package handlers

import (
	"context"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"github.com/jmoiron/sqlx"
)

// Check provides support for orchestration health checks.
type Check struct {
	MasterDB *sqlx.DB

	// ADD OTHER STATE LIKE THE LOGGER IF NEEDED.
}

// Health validates the service is healthy and ready to accept requests.
func (c *Check) Health(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	_, err := c.MasterDB.Exec("SELECT 1")
	if err != nil {
		return err
	}

	status := struct {
		Status string `json:"status"`
	}{
		Status: "ok",
	}

	return web.RespondJson(ctx, w, status, http.StatusOK)
}
