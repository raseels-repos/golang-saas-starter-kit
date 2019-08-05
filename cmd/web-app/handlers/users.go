package handlers

import (
	"context"
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/internal/geonames"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/datatable"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/notify"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	project_routes "geeks-accelerator/oss/saas-starter-kit/internal/project-routes"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account/invite"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_auth"
	"github.com/dustin/go-humanize/english"
	"github.com/gorilla/schema"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
	"net/http"
	"strings"
	"time"
)

// Users represents the Users API method handler set.
type Users struct {
	MasterDB      *sqlx.DB
	Redis         *redis.Client
	Renderer      web.Renderer
	Authenticator *auth.Authenticator
	ProjectRoutes project_routes.ProjectRoutes
	NotifyEmail   notify.Email
	SecretKey     string
}

func urlUsersIndex() string {
	return fmt.Sprintf("/users")
}

func urlUsersCreate() string {
	return fmt.Sprintf("/users/create")
}

func urlUsersView(userID string) string {
	return fmt.Sprintf("/users/%s", userID)
}

func urlUsersUpdate(userID string) string {
	return fmt.Sprintf("/users/%s/update", userID)
}

// UserLoginRequest extends the AuthenicateRequest with the RememberMe flag.
type UserCreateRequest struct {
	user.UserCreateRequest
	Roles user_account.UserAccountRoles `json:"roles" validate:"required,dive,oneof=admin user" enums:"admin,user" swaggertype:"array,string" example:"admin"`
}

// Index handles listing all the users for the current account.
func (h *Users) Index(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	statusOpts := web.NewEnumResponse(ctx, nil, user_account.UserAccountStatus_ValuesInterface()...)

	statusFilterItems := []datatable.FilterOptionItem{}
	for _, opt := range statusOpts.Options {
		statusFilterItems = append(statusFilterItems, datatable.FilterOptionItem{
			Display: opt.Title,
			Value:   opt.Value,
		})
	}

	fields := []datatable.DisplayField{
		datatable.DisplayField{Field: "id", Title: "ID", Visible: false, Searchable: true, Orderable: true, Filterable: false},
		datatable.DisplayField{Field: "name", Title: "User", Visible: true, Searchable: true, Orderable: true, Filterable: true, FilterPlaceholder: "filter Name"},
		datatable.DisplayField{Field: "status", Title: "Status", Visible: true, Searchable: true, Orderable: true, Filterable: true, FilterPlaceholder: "All Statuses", FilterItems: statusFilterItems},
		datatable.DisplayField{Field: "updated_at", Title: "Last Updated", Visible: true, Searchable: true, Orderable: true, Filterable: false},
		datatable.DisplayField{Field: "created_at", Title: "Created", Visible: true, Searchable: true, Orderable: true, Filterable: false},
	}

	mapFunc := func(q *user_account.User, cols []datatable.DisplayField) (resp []datatable.ColumnValue, err error) {
		for i := 0; i < len(cols); i++ {
			col := cols[i]
			var v datatable.ColumnValue
			switch col.Field {
			case "id":
				v.Value = fmt.Sprintf("%d", q.ID)
			case "name":
				v.Value = q.Name
				v.Formatted = fmt.Sprintf("<a href='%s'>%s</a>", urlUsersView(q.ID), v.Value)
			case "status":
				v.Value = q.Status.String()

				var subStatusClass string
				var subStatusIcon string
				switch q.Status {
				case user_account.UserAccountStatus_Active:
					subStatusClass = "text-green"
					subStatusIcon = "far fa-dot-circle"
				case user_account.UserAccountStatus_Invited:
					subStatusClass = "text-blue"
					subStatusIcon = "far fa-unicorn"
				case user_account.UserAccountStatus_Disabled:
					subStatusClass = "text-orange"
					subStatusIcon = "far fa-circle"
				}

				v.Formatted = fmt.Sprintf("<span class='cell-font-status %s'><i class='%s'></i>%s</span>", subStatusClass, subStatusIcon, web.EnumValueTitle(v.Value))
			case "created_at":
				dt := web.NewTimeResponse(ctx, q.CreatedAt)
				v.Value = dt.Local
				v.Formatted = fmt.Sprintf("<span class='cell-font-date'>%s</span>", v.Value)
			case "updated_at":
				dt := web.NewTimeResponse(ctx, q.UpdatedAt)
				v.Value = dt.Local
				v.Formatted = fmt.Sprintf("<span class='cell-font-date'>%s</span>", v.Value)
			default:
				return resp, errors.Errorf("Failed to map value for %s.", col.Field)
			}
			resp = append(resp, v)
		}

		return resp, nil
	}

	loadFunc := func(ctx context.Context, sorting string, fields []datatable.DisplayField) (resp [][]datatable.ColumnValue, err error) {
		res, err := user_account.UserFindByAccount(ctx, claims, h.MasterDB, user_account.UserFindByAccountRequest{
			AccountID: claims.Audience,
			Order:     strings.Split(sorting, ","),
		})
		if err != nil {
			return resp, err
		}

		for _, a := range res {
			l, err := mapFunc(a, fields)
			if err != nil {
				return resp, errors.Wrapf(err, "Failed to map user for display.")
			}

			resp = append(resp, l)
		}

		return resp, nil
	}

	dt, err := datatable.New(ctx, w, r, h.Redis, fields, loadFunc)
	if err != nil {
		return err
	}

	if dt.HasCache() {
		return nil
	}

	if ok, err := dt.Render(); ok {
		if err != nil {
			return err
		}
		return nil
	}

	data := map[string]interface{}{
		"datatable":      dt.Response(),
		"urlUsersCreate": urlUsersCreate(),
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "users-index.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Create handles creating a new user for the account.
func (h *Users) Create(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	//
	req := new(UserCreateRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {
		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return false, err
			}

			decoder := schema.NewDecoder()
			decoder.IgnoreUnknownKeys(true)

			if err := decoder.Decode(req, r.PostForm); err != nil {
				return false, err
			}

			// Bypass the uniq check on email here for the moment, it will be caught before the user_account is
			// created by user.Create.
			ctx = context.WithValue(ctx, webcontext.KeyTagUnique, true)

			// Validate the request.
			err = webcontext.Validator().StructCtx(ctx, req)
			if err != nil {
				if verr, ok := weberror.NewValidationError(ctx, err); ok {
					data["validationErrors"] = verr.(*weberror.Error)
					return false, nil
				} else {
					return false, err
				}
			}

			usr, err := user.Create(ctx, claims, h.MasterDB, req.UserCreateRequest, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}
			}

			uaStatus := user_account.UserAccountStatus_Active
			_, err = user_account.Create(ctx, claims, h.MasterDB, user_account.UserAccountCreateRequest{
				UserID:    usr.ID,
				AccountID: claims.Audience,
				Roles:     req.Roles,
				Status:    &uaStatus,
			}, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}
			}

			// Display a success message to the user.
			webcontext.SessionFlashSuccess(ctx,
				"User Created",
				"User successfully created.")
			err = webcontext.ContextSession(ctx).Save(r, w)
			if err != nil {
				return false, err
			}

			http.Redirect(w, r, urlUsersView(usr.ID), http.StatusFound)
			return true, nil
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	data["timezones"], err = geonames.ListTimezones(ctx, h.MasterDB)
	if err != nil {
		return err
	}

	var roleValues []interface{}
	for _, v := range user_account.UserAccountRole_Values {
		roleValues = append(roleValues, string(v))
	}
	data["roles"] = web.NewEnumResponse(ctx, nil, roleValues...)

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(UserCreateRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "users-create.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// View handles displaying a user.
func (h *Users) View(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	userID := params["user_id"]

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	data := make(map[string]interface{})
	f := func() (bool, error) {
		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return false, err
			}

			switch r.PostForm.Get("action") {
			case "archive":
				err = user.Archive(ctx, claims, h.MasterDB, user.UserArchiveRequest{
					ID: userID,
				}, ctxValues.Now)
				if err != nil {
					return false, err
				}

				webcontext.SessionFlashSuccess(ctx,
					"User Archive",
					"User successfully archive.")
				err = webcontext.ContextSession(ctx).Save(r, w)
				if err != nil {
					return false, err
				}

				http.Redirect(w, r, urlUsersIndex(), http.StatusFound)
				return true, nil
			}
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	usr, err := user.ReadByID(ctx, claims, h.MasterDB, userID)
	if err != nil {
		return err
	}

	data["user"] = usr.Response(ctx)

	usrAccs, err := user_account.FindByUserID(ctx, claims, h.MasterDB, userID, false)
	if err != nil {
		return err
	}

	for _, usrAcc := range usrAccs {
		if usrAcc.AccountID == claims.Audience {
			data["userAccount"] = usrAcc.Response(ctx)
			break
		}
	}

	data["urlUsersUpdate"] = urlUsersUpdate(userID)

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "users-view.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Update handles updating a user for the account.
func (h *Users) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	userID := params["user_id"]

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	//
	req := new(user.UserUpdateRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {
		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return false, err
			}

			decoder := schema.NewDecoder()
			decoder.IgnoreUnknownKeys(true)

			if err := decoder.Decode(req, r.PostForm); err != nil {
				return false, err
			}
			req.ID = userID

			err = user.Update(ctx, claims, h.MasterDB, *req, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}
			}

			if r.PostForm.Get("Password") != "" {
				pwdReq := new(user.UserUpdatePasswordRequest)

				if err := decoder.Decode(pwdReq, r.PostForm); err != nil {
					return false, err
				}
				pwdReq.ID = userID

				err = user.UpdatePassword(ctx, claims, h.MasterDB, *pwdReq, ctxValues.Now)
				if err != nil {
					switch errors.Cause(err) {
					default:
						if verr, ok := weberror.NewValidationError(ctx, err); ok {
							data["validationErrors"] = verr.(*weberror.Error)
							return false, nil
						} else {
							return false, err
						}
					}
				}
			}

			// Display a success message to the user.
			webcontext.SessionFlashSuccess(ctx,
				"User Updated",
				"User successfully updated.")
			err = webcontext.ContextSession(ctx).Save(r, w)
			if err != nil {
				return false, err
			}

			http.Redirect(w, r, urlUsersView(req.ID), http.StatusFound)
			return true, nil
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	usr, err := user.ReadByID(ctx, claims, h.MasterDB, userID)
	if err != nil {
		return err
	}

	if req.ID == "" {
		req.FirstName = &usr.FirstName
		req.LastName = &usr.LastName
		req.Email = &usr.Email
		req.Timezone = usr.Timezone
	}

	data["user"] = usr.Response(ctx)

	data["timezones"], err = geonames.ListTimezones(ctx, h.MasterDB)
	if err != nil {
		return err
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user.UserUpdateRequest{})); ok {
		data["userValidationDefaults"] = verr.(*weberror.Error)
	}

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user.UserUpdatePasswordRequest{})); ok {
		data["passwordValidationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "users-update.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Invite handles sending invites for users to the account.
func (h *Users) Invite(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	//
	req := new(invite.SendUserInvitesRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {
		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return false, err
			}

			decoder := schema.NewDecoder()
			if err := decoder.Decode(req, r.PostForm); err != nil {
				return false, err
			}

			req.UserID = claims.Subject
			req.AccountID = claims.Audience

			res, err := invite.SendUserInvites(ctx, claims, h.MasterDB, h.ProjectRoutes.UserInviteAccept, h.NotifyEmail, *req, h.SecretKey, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}
			}

			// Display a success message to the user.
			inviteCnt := len(res)
			if inviteCnt > 0 {
				webcontext.SessionFlashSuccess(ctx,
					fmt.Sprintf("%s Invited", english.PluralWord(inviteCnt, "User", "")),
					fmt.Sprintf("%s successfully invited. %s been sent to them to join your account.",
						english.Plural(inviteCnt, "user", ""),
						english.PluralWord(inviteCnt, "An email has", "Emails have")))
			} else {
				webcontext.SessionFlashWarning(ctx,
					"Users not Invited",
					"No users were invited.")
			}

			err = webcontext.ContextSession(ctx).Save(r, w)
			if err != nil {
				return false, err
			}

			http.Redirect(w, r, urlUsersIndex(), http.StatusFound)
			return true, nil
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	var selectedRoles []interface{}
	for _, r := range req.Roles {
		selectedRoles = append(selectedRoles, r.String())
	}
	data["roles"] = web.NewEnumMultiResponse(ctx, selectedRoles, user_account.UserAccountRole_ValuesInterface()...)

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(invite.SendUserInvitesRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "users-invite.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Invite handles sending invites for users to the account.
func (h *Users) InviteAccept(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	inviteHash := params["hash"]

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	//
	req := new(invite.AcceptInviteRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {

		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return false, err
			}

			decoder := schema.NewDecoder()
			if err := decoder.Decode(req, r.PostForm); err != nil {
				return false, err
			}

			// Append the query param value to the request.
			req.InviteHash = inviteHash

			userID, err := invite.AcceptInvite(ctx, h.MasterDB, *req, h.SecretKey, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				case invite.ErrInviteExpired:
					webcontext.SessionFlashError(ctx,
						"Invite Expired",
						"The invite has expired.")
					return false, nil
				case invite.ErrUserAccountActive:
					webcontext.SessionFlashError(ctx,
						"User already Active",
						"The user already is already active for the account. Try to login or use forgot password.")
					http.Redirect(w, r, "/user/login", http.StatusFound)
					return true, nil
				case invite.ErrInviteUserPasswordSet:
					webcontext.SessionFlashError(ctx,
						"Invite already Accepted",
						"The invite has already been accepted. Try to login or use forgot password.")
					http.Redirect(w, r, "/user/login", http.StatusFound)
					return true, nil
				case user_account.ErrNotFound:
					return false, err
				case invite.ErrNoPendingInvite:
					return false, err
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}
			}

			// Load the user without any claims applied.
			usr, err := user.ReadByID(ctx, auth.Claims{}, h.MasterDB, userID)
			if err != nil {
				return false, err
			}

			// Authenticated the user. Probably should use the default session TTL from UserLogin.
			token, err := user_auth.Authenticate(ctx, h.MasterDB, h.Authenticator, usr.Email, req.Password, time.Hour, ctxValues.Now)
			if err != nil {
				if verr, ok := weberror.NewValidationError(ctx, err); ok {
					data["validationErrors"] = verr.(*weberror.Error)
					return false, nil
				} else {
					return false, err
				}
			}

			// Add the token to the users session.
			err = handleSessionToken(ctx, h.MasterDB, w, r, token)
			if err != nil {
				return false, err
			}

			// Redirect the user to the dashboard.
			http.Redirect(w, r, "/", http.StatusFound)
			return true, nil
		}

		hash, err := invite.ParseInviteHash(ctx, h.SecretKey, inviteHash, ctxValues.Now)
		if err != nil {
			switch errors.Cause(err) {
			case invite.ErrInviteExpired:
				webcontext.SessionFlashError(ctx,
					"Invite Expired",
					"The invite has expired.")
				return false, nil
			case invite.ErrInviteUserPasswordSet:
				webcontext.SessionFlashError(ctx,
					"Invite already Accepted",
					"The invite has already been accepted. Try to login or use forgot password.")
				http.Redirect(w, r, "/user/login", http.StatusFound)
				return true, nil
			default:
				if verr, ok := weberror.NewValidationError(ctx, err); ok {
					data["validationErrors"] = verr.(*weberror.Error)
					return false, nil
				} else {
					return false, err
				}
			}
		}

		// Read user by ID with no claims.
		usr, err := user.ReadByID(ctx, auth.Claims{}, h.MasterDB, hash.UserID)
		if err != nil {
			return false, err
		}
		data["user"] = usr.Response(ctx)

		if req.Email == "" {
			req.FirstName = usr.FirstName
			req.LastName = usr.LastName
			req.Email = usr.Email
			req.Timezone = usr.Timezone
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	data["timezones"], err = geonames.ListTimezones(ctx, h.MasterDB)
	if err != nil {
		return err
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(invite.AcceptInviteRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "users-invite-accept.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
