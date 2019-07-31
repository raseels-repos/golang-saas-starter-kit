package handlers

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/jmoiron/sqlx"
)

// User represents the User API method handler set.
type User struct {
	MasterDB      *sqlx.DB
	Renderer      web.Renderer
	Authenticator *auth.Authenticator
}

// List returns all the existing users in the system.
func (u *User) Login(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	return u.Renderer.Render(ctx, w, r, tmplLayoutBase, "user-login.tmpl", web.MIMETextHTMLCharsetUTF8, http.StatusOK, nil)
}

// List returns all the existing users in the system.
func (u *User) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	return u.Renderer.Render(ctx, w, r, tmplLayoutBase, "user-logout.tmpl", web.MIMETextHTMLCharsetUTF8, http.StatusOK, nil)
}

// List returns all the existing users in the system.
func (u *User) ForgotPassword(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	return u.Renderer.Render(ctx, w, r, tmplLayoutBase, "user-forgot-password.tmpl", web.MIMETextHTMLCharsetUTF8, http.StatusOK, nil)
}
