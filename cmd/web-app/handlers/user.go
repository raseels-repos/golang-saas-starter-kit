package handlers

import (
	"context"
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
func (h *User) ForgotPassword(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	return h.Renderer.Render(ctx, w, r, tmplLayoutBase, "user-forgot-password.tmpl", web.MIMETextHTMLCharsetUTF8, http.StatusOK, nil)
}
