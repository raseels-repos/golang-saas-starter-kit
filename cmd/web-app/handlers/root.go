package handlers

import (
	"context"
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
	data := map[string]interface{}{
		"imgSizes": []int{100, 200, 300, 400, 500},
	}

	return u.Renderer.Render(ctx, w, r, baseLayoutTmpl, "root-index.tmpl", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
