package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/datatable"
	"geeks-accelerator/oss/saas-starter-kit/internal/geonames"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/notify"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	project_routes "geeks-accelerator/oss/saas-starter-kit/internal/project-routes"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"github.com/gorilla/schema"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)

// Users represents the Users API method handler set.
type Users struct {
	MasterDB      *sqlx.DB
	Redis    *redis.Client
	Renderer      web.Renderer
	Authenticator *auth.Authenticator
	ProjectRoutes project_routes.ProjectRoutes
	NotifyEmail   notify.Email
	SecretKey     string
}

func UrlUsersView(userID string) string {
	return fmt.Sprintf("/users/%s", userID)
}

// Index handles listing all the users for the current account.
func (h *Users) Index(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	var statusValues []interface{}
	for _, v := range user_account.UserAccountStatus_Values {
		statusValues = append(statusValues, string(v))
	}

	statusOpts := web.NewEnumResponse(ctx, nil, statusValues...)

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
				v.Formatted = fmt.Sprintf("<a href='%s'>%s</a>", UrlUsersView(q.ID), v.Value)
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
			Order:      strings.Split(sorting, ","),
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
		"datatable":  dt.Response(),
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "users-index.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// View handles displaying a user.
func (h *Users) View(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	data := make(map[string]interface{})
	f := func() error {

		claims, err := auth.ClaimsFromContext(ctx)
		if err != nil {
			return err
		}

		usr, err := user.ReadByID(ctx, claims, h.MasterDB, claims.Subject)
		if err != nil {
			return err
		}

		data["user"] = usr.Response(ctx)

		usrAccs, err := user_account.FindByUserID(ctx, claims, h.MasterDB, claims.Subject, false)
		if err != nil {
			return err
		}

		for _, usrAcc := range usrAccs {
			if usrAcc.AccountID == claims.Audience {
				data["userAccount"] = usrAcc.Response(ctx)
				break
			}
		}

		return nil
	}

	if err := f(); err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "users-view.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Update handles updating a user for the account.
func (h *Users) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

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
			req.ID = claims.Subject

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
				pwdReq.ID = claims.Subject

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

			http.Redirect(w, r, "/users/"+req.ID, http.StatusFound)
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

	usr, err := user.ReadByID(ctx, claims, h.MasterDB, claims.Subject)
	if err != nil {
		return err
	}

	if req.ID == "" {
		req.FirstName = &usr.FirstName
		req.LastName = &usr.LastName
		req.Email = &usr.Email
		req.Timezone = &usr.Timezone
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

