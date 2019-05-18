package handlers

import (
	"context"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/db"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"go.opencensus.io/trace"
)

// User represents the User API method handler set.
type Root struct {
	MasterDB       *db.DB
	Renderer web.Renderer
	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

// List returns all the existing users in the system.
func (u *Root) Index(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	ctx, span := trace.StartSpan(ctx, "handlers.Root.Index")
	defer span.End()

	return u.Renderer.Render(ctx, w, r, baseLayoutTmpl, "root-index.tmpl", web.MIMETextHTMLCharsetUTF8, http.StatusOK, nil)
}
