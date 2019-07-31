package handlers

import (
	"context"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/jmoiron/sqlx"
)

// User represents the User API method handler set.
type Projects struct {
	MasterDB *sqlx.DB
	Renderer web.Renderer
	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

// List returns all the existing users in the system.
func (p *Projects) Index(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	return p.Renderer.Render(ctx, w, r, tmplLayoutBase, "projects-index.tmpl", web.MIMETextHTMLCharsetUTF8, http.StatusOK, nil)
}
