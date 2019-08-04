package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
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
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
)

// User represents the User API method handler set.
type User struct {
	MasterDB      *sqlx.DB
	Renderer      web.Renderer
	Authenticator *auth.Authenticator
	ProjectRoutes project_routes.ProjectRoutes
	NotifyEmail   notify.Email
	SecretKey     string
}

type UserLoginRequest struct {
	user.AuthenticateRequest
	RememberMe bool
}

// Login handles authenticating a user into the system.
func (h *User) Login(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	//
	req := new(UserLoginRequest)
	data := make(map[string]interface{})
	f := func() error {

		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return err
			}

			decoder := schema.NewDecoder()
			if err := decoder.Decode(req, r.PostForm); err != nil {
				return err
			}

			sessionTTL := time.Hour
			if req.RememberMe {
				sessionTTL = time.Hour * 36
			}

			// Authenticated the user.
			token, err := user.Authenticate(ctx, h.MasterDB, h.Authenticator, req.Email, req.Password, sessionTTL, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				case user.ErrForbidden:
					return web.RespondError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return nil
					} else {
						return err
					}
				}
			}

			// Add the token to the users session.
			err = handleSessionToken(ctx, h.MasterDB, w, r, token)
			if err != nil {
				return err
			}

			// Redirect the user to the dashboard.
			http.Redirect(w, r, "/", http.StatusFound)
		}

		return nil
	}

	if err := f(); err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(UserLoginRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-login.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// handleSessionToken persists the access token to the session for request authentication.
func handleSessionToken(ctx context.Context, db *sqlx.DB, w http.ResponseWriter, r *http.Request, token user.Token) error {
	if token.AccessToken == "" {
		return errors.New("accessToken is required.")
	}

	sess := webcontext.ContextSession(ctx)

	if sess.IsNew {
		sess.ID = uuid.NewRandom().String()
	}

	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   int(token.TTL.Seconds()),
		HttpOnly: false,
	}

	sess = webcontext.SessionInit(sess,
		token.AccessToken)
	if err := sess.Save(r, w); err != nil {
		return err
	}

	return nil
}

// Logout handles removing authentication for the user.
func (h *User) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	sess := webcontext.ContextSession(ctx)

	// Set the access token to empty to logout the user.
	sess = webcontext.SessionDestroy(sess)

	if err := sess.Save(r, w); err != nil {
		return err
	}

	// Redirect the user to the root page.
	http.Redirect(w, r, "/", http.StatusFound)

	return nil
}

// ResetPassword allows a user to perform forgot password.
func (h *User) ResetPassword(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	//
	req := new(user.UserResetPasswordRequest)
	data := make(map[string]interface{})
	f := func() error {

		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return err
			}

			decoder := schema.NewDecoder()
			if err := decoder.Decode(req, r.PostForm); err != nil {
				return err
			}

			_, err = user.ResetPassword(ctx, h.MasterDB, h.ProjectRoutes.UserResetPassword, h.NotifyEmail, *req, h.SecretKey, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return nil
					} else {
						return err
					}
				}
			}

			// Display a success message to the user to check their email.
			webcontext.SessionFlashSuccess(ctx,
				"Check your email",
				fmt.Sprintf("An email was sent to '%s'. Click on the link in the email to finish resetting your password.", req.Email))

		}

		return nil
	}

	if err := f(); err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user.UserResetPasswordRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-reset-password.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// ResetConfirm handles changing a users password after they have clicked on the link emailed.
func (h *User) ResetConfirm(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	//
	req := new(user.UserResetConfirmRequest)
	data := make(map[string]interface{})
	f := func() error {

		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return err
			}

			decoder := schema.NewDecoder()
			if err := decoder.Decode(req, r.PostForm); err != nil {
				return err
			}

			u, err := user.ResetConfirm(ctx, h.MasterDB, *req, h.SecretKey, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return nil
					} else {
						return err
					}
				}
			}

			// Authenticated the user. Probably should use the default session TTL from UserLogin.
			token, err := user.Authenticate(ctx, h.MasterDB, h.Authenticator, u.Email, req.Password, time.Hour, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				case account.ErrForbidden:
					return web.RespondError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return nil
					} else {
						return err
					}
				}
			}

			// Add the token to the users session.
			err = handleSessionToken(ctx, h.MasterDB, w, r, token)
			if err != nil {
				return err
			}

			// Redirect the user to the dashboard.
			http.Redirect(w, r, "/", http.StatusFound)
		} else {
			req.ResetHash = params["hash"]
		}

		return nil
	}

	if err := f(); err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user.UserResetConfirmRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-reset-confirm.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// View handles displaying the current user profile.
func (h *User) View(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	data := make(map[string]interface{})
	f := func() error {

		claims, err := auth.ClaimsFromContext(ctx)
		if err != nil {
			return err
		}

		usr, err := user.Read(ctx, claims, h.MasterDB, claims.Subject, false)
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

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-view.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Update handles allowing the current user to update their profile.
func (h *User) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	//
	req := new(user.UserUpdateRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {

		claims, err := auth.ClaimsFromContext(ctx)
		if err != nil {
			return false, err
		}

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
				"Profile Updated",
				"User profile successfully updated.")
			err = webcontext.ContextSession(ctx).Save(r, w)
			if err != nil {
				return false, err
			}

			http.Redirect(w, r, "/user", http.StatusFound)
			return true, nil
		}

		usr, err := user.Read(ctx, claims, h.MasterDB, claims.Subject, false)
		if err != nil {
			return false, err
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
			return false, err
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user.UserUpdateRequest{})); ok {
		data["userValidationDefaults"] = verr.(*weberror.Error)
	}

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user.UserUpdatePasswordRequest{})); ok {
		data["passwordValidationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-update.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Account handles displaying the Account for the current user.
func (h *User) Account(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	data := make(map[string]interface{})
	f := func() error {

		claims, err := auth.ClaimsFromContext(ctx)
		if err != nil {
			return err
		}

		acc, err := account.Read(ctx, claims, h.MasterDB, claims.Audience, false)
		if err != nil {
			return err
		}
		data["account"] = acc.Response(ctx)

		return nil
	}

	if err := f(); err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-account.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
