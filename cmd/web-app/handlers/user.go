package handlers

import (
	"context"
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/notify"
	project_routes "geeks-accelerator/oss/saas-starter-kit/internal/project-routes"
	"net/http"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
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

// List returns all the existing users in the system.
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

			if err := webcontext.Validator().Struct(req); err != nil {
				if ne, ok := weberror.NewValidationError(ctx, err); ok {
					data["validationErrors"] = ne.(*weberror.Error)
					return nil
				} else {
					return err
				}
			}

			sessionTTL := time.Hour
			if req.RememberMe {
				sessionTTL = time.Hour * 36
			}

			// Authenticated the user.
			token, err := user.Authenticate(ctx, h.MasterDB, h.Authenticator, req.Email, req.Password, sessionTTL, ctxValues.Now)
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
			err = handleSessionToken(ctx, w, r, token)
			if err != nil {
				return err
			}

			// Redirect the user to the dashboard.
			http.Redirect(w, r, "/", http.StatusFound)
		}

		return nil
	}

	if err := f(); err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, tmplLayoutBase, tmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(UserLoginRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, tmplLayoutBase, "user-login.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// handleSessionToken persists the access token to the session for request authentication.
func handleSessionToken(ctx context.Context, w http.ResponseWriter, r *http.Request, token user.Token) error {
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

	sess = webcontext.SessionWithAccessToken(sess, token.AccessToken)

	if err := sess.Save(r, w); err != nil {
		return err
	}

	return nil
}

// Logout handles removing authentication for the user.
func (h *User) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	sess := webcontext.ContextSession(ctx)

	// Set the access token to empty to logout the user.
	sess = webcontext.SessionWithAccessToken(sess, "")

	if err := sess.Save(r, w); err != nil {
		return err
	}

	// Redirect the user to the root page.
	http.Redirect(w, r, "/", http.StatusFound)

	return nil
}

// List returns all the existing users in the system.
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

			if err := webcontext.Validator().Struct(req); err != nil {
				if ne, ok := weberror.NewValidationError(ctx, err); ok {
					data["validationErrors"] = ne.(*weberror.Error)
					return nil
				} else {
					return err
				}
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
		return web.RenderError(ctx, w, r, err, h.Renderer, tmplLayoutBase, tmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user.UserResetPasswordRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, tmplLayoutBase, "user-reset-password.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// List returns all the existing users in the system.
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

			if err := webcontext.Validator().Struct(req); err != nil {
				if ne, ok := weberror.NewValidationError(ctx, err); ok {
					data["validationErrors"] = ne.(*weberror.Error)
					return nil
				} else {
					return err
				}
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
			err = handleSessionToken(ctx, w, r, token)
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
		return web.RenderError(ctx, w, r, err, h.Renderer, tmplLayoutBase, tmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user.UserResetConfirmRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, tmplLayoutBase, "user-reset-confirm.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
