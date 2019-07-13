package handlers

import (
	"context"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)

// Check provides support for orchestration health checks.
type Check struct {
	MasterDB *sqlx.DB
	Redis    *redis.Client

	// ADD OTHER STATE LIKE THE LOGGER IF NEEDED.
}

// Health validates the service is healthy and ready to accept requests.
func (c *Check) Health(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	// check postgres
	_, err := c.MasterDB.Exec("SELECT 1")
	if err != nil {
		return errors.Wrap(err, "Postgres failed")
	}

	// check redis
	err = c.Redis.Ping().Err()
	if err != nil {
		return errors.Wrap(err, "Redis failed")
	}

	status := struct {
		Status string `json:"status"`
	}{
		Status: "ok",
	}

	return web.RespondJson(ctx, w, status, http.StatusOK)
}

// Ping validates the service is ready to accept requests.
func (c *Check) Ping(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	status := "pong"

	return web.RespondJson(ctx, w, status, http.StatusOK)
}
