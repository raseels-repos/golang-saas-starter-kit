package handlers

import (
	"log"
	"net/http"
	"os"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/mid"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
)

// API returns a handler for a set of routes.
func API(shutdown chan os.Signal, log *log.Logger, zipkinHost string, apiHost string) http.Handler {

	app := web.NewApp(shutdown, log, mid.Logger(log), mid.Errors(log))

	z := NewZipkin(zipkinHost, apiHost, time.Second)
	app.Handle("POST", "/v1/publish", z.Publish)

	h := Health{}
	app.Handle("GET", "/v1/health", h.Check)

	return app
}
