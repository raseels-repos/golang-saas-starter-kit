package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/geonames"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/datatable"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account/invite"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_auth"
	"github.com/dustin/go-humanize/english"
	"github.com/gorilla/schema"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)

// Users represents the Users API method handler set.
type Users struct {
	UserRepo        *user.Repository
	UserAccountRepo *user_account.Repository
	AuthRepo        *user_auth.Repository
	InviteRepo      *invite.Repository
	MasterDB        *sqlx.DB
	Redis           *redis.Client
	Renderer        web.Renderer
}

func urlUsersIndex() string {
	return fmt.Sprintf("/users")
}

func urlUsersCreate() string {
	return fmt.Sprintf("/users/create")
}

func urlUsersInvite() string {
	return fmt.Sprintf("/users/invite")
}

func urlUsersView(userID string) string {
	return fmt.Sprintf("/users/%s", userID)
}

func urlUsersUpdate(userID string) string {
	return fmt.Sprintf("/users/%s/update", userID)
}

// UserCreateRequest extends the UserCreateRequest with a list of roles.
type UserCreateRequest struct {
	user.UserCreateRequest
	Roles user_account.UserAccountRoles `json:"roles" validate:"required,dive,oneof=admin user" enums:"admin,user" swaggertype:"array,string" example:"admin"`
}

// UserUpdateRequest extends the UserUpdateRequest with a list of roles.
type UserUpdateRequest struct {
	user.UserUpdateRequest
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
				if strings.TrimSpace(q.Name) == "" {
					v.Value = q.Email
				} else {
					v.Value = q.Name
				}
				v.Formatted = fmt.Sprintf("<a href='%s'>%s</a>", urlUsersView(q.ID), v.Value)
			case "status":
				v.Value = q.Status.String()

				var subStatusClass string
				var subStatusIcon string
				switch q.Status {
				case user_account.UserAccountStatus_Active:
					subStatusClass = "text-green"
					subStatusIcon = "fas fa-circle"
				case user_account.UserAccountStatus_Invited:
					subStatusClass = "text-aqua"
					subStatusIcon = "far fa-dot-circle"
				case user_account.UserAccountStatus_Disabled:
					subStatusClass = "text-orange"
					subStatusIcon = "fas fa-circle-notch"
				}

				v.Formatted = fmt.Sprintf("<span class='cell-font-status %s'><i class='%s mr-1'></i>%s</span>", subStatusClass, subStatusIcon, web.EnumValueTitle(v.Value))
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
		res, err := h.UserAccountRepo.UserFindByAccount(ctx, claims, user_account.UserFindByAccountRequest{
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
		"urlUsersInvite": urlUsersInvite(),
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

			usr, err := h.UserRepo.Create(ctx, claims, req.UserCreateRequest, ctxValues.Now)
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
			_, err = h.UserAccountRepo.Create(ctx, claims, user_account.UserAccountCreateRequest{
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

			return true, web.Redirect(ctx, w, r, urlUsersView(usr.ID), http.StatusFound)
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

	var selectedRoles []interface{}
	for _, r := range req.Roles {
		selectedRoles = append(selectedRoles, r.String())
	}
	data["roles"] = web.NewEnumMultiResponse(ctx, selectedRoles, user_account.UserAccountRole_ValuesInterface()...)

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
				err = h.UserRepo.Archive(ctx, claims, user.UserArchiveRequest{
					ID: userID,
				}, ctxValues.Now)
				if err != nil {
					return false, err
				}

				webcontext.SessionFlashSuccess(ctx,
					"User Archive",
					"User successfully archive.")

				return true, web.Redirect(ctx, w, r, urlUsersIndex(), http.StatusFound)
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

	usr, err := h.UserRepo.ReadByID(ctx, claims, userID)
	if err != nil {
		return err
	}

	data["user"] = usr.Response(ctx)

	usrAccs, err := h.UserAccountRepo.FindByUserID(ctx, claims, userID, false)
	if err != nil {
		return err
	}

	for _, usrAcc := range usrAccs {
		if usrAcc.AccountID == claims.Audience {
			data["userAccount"] = usrAcc.Response(ctx)
			break
		}
	}

	data["urlUsersView"] = urlUsersView(userID)
	data["urlUsersUpdate"] = urlUsersUpdate(userID)
	data["urlUserVirtualLogin"] = urlUserVirtualLogin(userID)

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
	req := new(UserUpdateRequest)
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

			err = h.UserRepo.Update(ctx, claims, req.UserUpdateRequest, ctxValues.Now)
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

			if req.Roles != nil {
				err = h.UserAccountRepo.Update(ctx, claims, user_account.UserAccountUpdateRequest{
					UserID:    userID,
					AccountID: claims.Audience,
					Roles:     &req.Roles,
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
			}

			if r.PostForm.Get("Password") != "" {
				pwdReq := new(user.UserUpdatePasswordRequest)

				if err := decoder.Decode(pwdReq, r.PostForm); err != nil {
					return false, err
				}
				pwdReq.ID = userID

				err = h.UserRepo.UpdatePassword(ctx, claims, *pwdReq, ctxValues.Now)
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

			return true, web.Redirect(ctx, w, r, urlUsersView(req.ID), http.StatusFound)
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	usr, err := h.UserRepo.ReadByID(ctx, claims, userID)
	if err != nil {
		return err
	}

	usrAcc, err := h.UserAccountRepo.Read(ctx, claims, user_account.UserAccountReadRequest{
		UserID:    userID,
		AccountID: claims.Audience,
	})
	if err != nil {
		return err
	}

	if req.ID == "" {
		req.FirstName = &usr.FirstName
		req.LastName = &usr.LastName
		req.Email = &usr.Email
		req.Timezone = usr.Timezone
		req.Roles = usrAcc.Roles
	}

	data["user"] = usr.Response(ctx)

	data["timezones"], err = geonames.ListTimezones(ctx, h.MasterDB)
	if err != nil {
		return err
	}

	var selectedRoles []interface{}
	for _, r := range req.Roles {
		selectedRoles = append(selectedRoles, r.String())
	}
	data["roles"] = web.NewEnumMultiResponse(ctx, selectedRoles, user_account.UserAccountRole_ValuesInterface()...)

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(UserUpdateRequest{})); ok {
		data["userValidationDefaults"] = verr.(*weberror.Error)
	}

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user.UserUpdatePasswordRequest{})); ok {
		data["passwordValidationDefaults"] = verr.(*weberror.Error)
	}

	data["urlUsersView"] = urlUsersView(userID)

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

			res, err := h.InviteRepo.SendUserInvites(ctx, claims, *req, ctxValues.Now)
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

			return true, web.Redirect(ctx, w, r, urlUsersIndex(), http.StatusFound)
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
	req := new(invite.AcceptInviteUserRequest)
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

			hash, err := h.InviteRepo.AcceptInviteUser(ctx, *req, ctxValues.Now)
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
						"The user is already is already active for the account. Try to login or use forgot password.")

					return true, web.Redirect(ctx, w, r, "/user/login", http.StatusFound)

				case invite.ErrNoPendingInvite:
					webcontext.SessionFlashError(ctx,
						"Invite Accepted",
						"The invite has already been accepted. Try to login or use forgot password.")

					return true, web.Redirect(ctx, w, r, "/user/login", http.StatusFound)

				case user_account.ErrNotFound:
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
			usr, err := h.UserRepo.ReadByID(ctx, auth.Claims{}, hash.UserID)
			if err != nil {
				return false, err
			}

			// Authenticated the user. Probably should use the default session TTL from UserLogin.
			token, err := h.AuthRepo.Authenticate(ctx, user_auth.AuthenticateRequest{
				Email:     usr.Email,
				Password:  req.Password,
				AccountID: hash.AccountID,
			}, time.Hour, ctxValues.Now)
			if err != nil {
				if verr, ok := weberror.NewValidationError(ctx, err); ok {
					data["validationErrors"] = verr.(*weberror.Error)
					return false, nil
				} else {
					return false, err
				}
			}

			// Add the token to the users session.
			err = handleSessionToken(ctx, w, r, token)
			if err != nil {
				return false, err
			}

			// Redirect the user to the dashboard.
			return true, web.Redirect(ctx, w, r, "/", http.StatusFound)
		}

		usrAcc, err := h.InviteRepo.AcceptInvite(ctx, invite.AcceptInviteRequest{
			InviteHash: inviteHash,
		}, ctxValues.Now)
		if err != nil {

			switch errors.Cause(err) {
			case invite.ErrInviteExpired:
				webcontext.SessionFlashError(ctx,
					"Invite Expired",
					"The invite has expired.")

				return true, web.Redirect(ctx, w, r, "/user/login", http.StatusFound)

			case invite.ErrUserAccountActive:
				webcontext.SessionFlashError(ctx,
					"User already Active",
					"The user is already is already active for the account. Try to login or use forgot password.")

				return true, web.Redirect(ctx, w, r, "/user/login", http.StatusFound)

			case invite.ErrNoPendingInvite:
				webcontext.SessionFlashError(ctx,
					"Invite Accepted",
					"The invite has already been accepted. Try to login or use forgot password.")

				return true, web.Redirect(ctx, w, r, "/user/login", http.StatusFound)

			case user_account.ErrNotFound:
				return false, err
			default:
				if verr, ok := weberror.NewValidationError(ctx, err); ok {
					data["validationErrors"] = verr.(*weberror.Error)

					return false, nil
				} else {
					return false, err
				}
			}
		} else if usrAcc.Status == user_account.UserAccountStatus_Active {
			webcontext.SessionFlashError(ctx,
				"Invite Accepted",
				"The invite has been accepted. Login to continue.")

			return true, web.Redirect(ctx, w, r, "/user/login", http.StatusFound)
		}

		// Read user by ID with no claims.
		usr, err := h.UserRepo.ReadByID(ctx, auth.Claims{}, usrAcc.UserID)
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

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(invite.AcceptInviteUserRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "users-invite-accept.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
