package handlers

import (
	"log"
	"net/http"
	"os"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/mid"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"github.com/jmoiron/sqlx"
)

const baseLayoutTmpl = "base.tmpl"

// API returns a handler for a set of routes.
func APP(shutdown chan os.Signal, log *log.Logger, staticDir, templateDir string, masterDB *sqlx.DB, authenticator *auth.Authenticator, renderer web.Renderer, globalMids ...web.Middleware) http.Handler {

	// Define base middlewares applied to all requests.
	middlewares := []web.Middleware{
		mid.Trace(), mid.Logger(log), mid.Errors(log), mid.Metrics(), mid.Panics(),
	}

	// Append any global middlewares if they were included.
	if len(globalMids) > 0 {
		middlewares = append(middlewares, globalMids...)
	}

	// Construct the web.App which holds all routes as well as common Middleware.
	app := web.NewApp(shutdown, log, middlewares...)

	// Register health check endpoint. This route is not authenticated.
	check := Check{
		MasterDB: masterDB,
		Renderer: renderer,
	}
	app.Handle("GET", "/v1/health", check.Health)

	// Register user management and authentication endpoints.
	u := User{
		MasterDB: masterDB,
		Renderer: renderer,
	}

	// This route is not authenticated
	app.Handle("POST", "/users/login", u.Login)
	app.Handle("GET", "/users/login", u.Login)

	// Register root
	r := Root{
		MasterDB: masterDB,
		Renderer: renderer,
	}
	// This route is not authenticated
	app.Handle("GET", "/index.html", r.Index)
	app.Handle("GET", "/", r.Index)

	// Static file server
	app.Handle("GET", "/*", web.Static(staticDir, ""))

	return app
}
