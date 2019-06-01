package handlers

import (
	"context"
	"github.com/jmoiron/sqlx"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
)

// Check provides support for orchestration health checks.
type Check struct {
	MasterDB *sqlx.DB
	Renderer web.Renderer

	// ADD OTHER STATE LIKE THE LOGGER IF NEEDED.
}

// Health validates the service is healthy and ready to accept requests.
func (c *Check) Health(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	// check postgres
	_, err := c.MasterDB.Exec("SELECT 1")
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"Status": "ok",
	}

	return c.Renderer.Render(ctx, w, r, baseLayoutTmpl, "health.tmpl", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
