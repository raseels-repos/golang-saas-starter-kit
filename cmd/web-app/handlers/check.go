package handlers

import (
	"context"
	"net/http"
	"os"

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

	data := struct {
		Status          string `json:"status"`
		CiCommitRefName string `json:"ci-commit-ref-name,omitempty"`
		CiCommitShortSha string `json:"ci-commit-short-sha,omitempty"`
		CiCommitSha     string `json:"ci-commit-sha,omitempty"`
		CiCommitTag     string `json:"ci-commit-tag,omitempty"`
		CiCommitTitle   string `json:"ci-commit-title,omitempty"`
		CiJobId         string `json:"ci-commit-job-id,omitempty"`
		CiPipelineId    string `json:"ci-commit-pipeline-id,omitempty"`
	}{
		Status:          "ok",
		CiCommitRefName: os.Getenv("CI_COMMIT_REF_NAME"),
		CiCommitShortSha : os.Getenv("CI_COMMIT_SHORT_SHA"),
		CiCommitSha:     os.Getenv("CI_COMMIT_SHA"),
		CiCommitTag:     os.Getenv("CI_COMMIT_TAG"),
		CiJobId:         os.Getenv("CI_JOB_ID"),
		CiPipelineId:    os.Getenv("CI_PIPELINE_ID"),
	}

	return web.RespondJson(ctx, w, data, http.StatusOK)
}

// Ping validates the service is ready to accept requests.
func (c *Check) Ping(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	status := "pong"

	return web.RespondText(ctx, w, status, http.StatusOK)
}
