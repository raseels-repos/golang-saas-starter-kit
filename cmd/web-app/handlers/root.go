package handlers

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/jmoiron/sqlx"
)

// User represents the User API method handler set.
type Root struct {
	MasterDB *sqlx.DB
	Renderer web.Renderer
	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

// List returns all the existing users in the system.
func (u *Root) Index(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	// Force users to login to access the index page.
	if claims, err := auth.ClaimsFromContext(ctx); err != nil || !claims.HasAuth() {
		http.Redirect(w, r, "/user/login", http.StatusFound)
		return nil
	}

	data := map[string]interface{}{
		"imgSizes": []int{100, 200, 300, 400, 500},
	}

	return u.Renderer.Render(ctx, w, r, tmplLayoutBase, "root-index.tmpl", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
