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

// API returns a handler for a set of routes.
func API(shutdown chan os.Signal, log *log.Logger, masterDB *sqlx.DB, authenticator *auth.Authenticator) http.Handler {

	// Construct the web.App which holds all routes as well as common Middleware.
	app := web.NewApp(shutdown, log, mid.Logger(log), mid.Errors(log), mid.Metrics(), mid.Panics())

	// Register health check endpoint. This route is not authenticated.
	check := Check{
		MasterDB: masterDB,
	}
	app.Handle("GET", "/v1/health", check.Health)

	// Register user management and authentication endpoints.
	u := User{
		MasterDB:       masterDB,
		TokenGenerator: authenticator,
	}
	app.Handle("GET", "/v1/users", u.List, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("POST", "/v1/users", u.Create, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/v1/users/:id", u.Retrieve, mid.Authenticate(authenticator))
	app.Handle("PUT", "/v1/users/:id", u.Update, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("DELETE", "/v1/users/:id", u.Delete, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))

	// This route is not authenticated
	app.Handle("GET", "/v1/users/token", u.Token)

	// Register project and sale endpoints.
	p := Project{
		MasterDB: masterDB,
	}
	app.Handle("GET", "/v1/projects", p.List, mid.Authenticate(authenticator))
	app.Handle("POST", "/v1/projects", p.Create, mid.Authenticate(authenticator))
	app.Handle("GET", "/v1/projects/:id", p.Retrieve, mid.Authenticate(authenticator))
	app.Handle("PUT", "/v1/projects/:id", p.Update, mid.Authenticate(authenticator))
	app.Handle("DELETE", "/v1/projects/:id", p.Delete, mid.Authenticate(authenticator))

	return app
}
