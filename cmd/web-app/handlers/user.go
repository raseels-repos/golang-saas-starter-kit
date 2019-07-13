package handlers

import (
	"context"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"github.com/jmoiron/sqlx"
)

// User represents the User API method handler set.
type User struct {
	MasterDB *sqlx.DB
	Renderer web.Renderer
	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

// List returns all the existing users in the system.
func (u *User) Login(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	return u.Renderer.Render(ctx, w, r, baseLayoutTmpl, "user-login.tmpl", web.MIMETextHTMLCharsetUTF8, http.StatusOK, nil)
}
