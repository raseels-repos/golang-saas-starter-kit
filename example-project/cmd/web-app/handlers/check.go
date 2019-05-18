package handlers

import (
	"context"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/db"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"go.opencensus.io/trace"
)

// Check provides support for orchestration health checks.
type Check struct {
	MasterDB *db.DB
	Renderer web.Renderer

	// ADD OTHER STATE LIKE THE LOGGER IF NEEDED.
}

// Health validates the service is healthy and ready to accept requests.
func (c *Check) Health(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	ctx, span := trace.StartSpan(ctx, "handlers.Check.Health")
	defer span.End()

	dbConn := c.MasterDB.Copy()
	defer dbConn.Close()

	if err := dbConn.StatusCheck(ctx); err != nil {
		return err
	}

	data := map[string]interface{}{
		"Status": "ok",
	}

	return c.Renderer.Render(ctx, w, r, baseLayoutTmpl, "health.tmpl", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
