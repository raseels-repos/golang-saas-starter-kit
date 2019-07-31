package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"geeks-accelerator/oss/saas-starter-kit/internal/mid"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"github.com/jmoiron/sqlx"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)

const (
	tmplLayoutBase          = "base.tmpl"
	tmplContentErrorGeneric = "error-generic.gohtml"
)

// API returns a handler for a set of routes.
func APP(shutdown chan os.Signal, log *log.Logger, env webcontext.Env, staticDir, templateDir string, masterDB *sqlx.DB, redis *redis.Client, authenticator *auth.Authenticator, renderer web.Renderer, globalMids ...web.Middleware) http.Handler {

	// Define base middlewares applied to all requests.
	middlewares := []web.Middleware{
		mid.Trace(), mid.Logger(log), mid.Errors(log), mid.Metrics(), mid.Panics(),
	}

	// Append any global middlewares if they were included.
	if len(globalMids) > 0 {
		middlewares = append(middlewares, globalMids...)
	}

	// Construct the web.App which holds all routes as well as common Middleware.
	app := web.NewApp(shutdown, log, env, middlewares...)

	// Register project management pages.
	p := Projects{
		MasterDB: masterDB,
		Renderer: renderer,
	}
	app.Handle("GET", "/projects", p.Index, mid.HasAuth())

	// Register user management and authentication endpoints.
	u := User{
		MasterDB:      masterDB,
		Renderer:      renderer,
		Authenticator: authenticator,
	}
	// This route is not authenticated
	app.Handle("POST", "/user/login", u.Login)
	app.Handle("GET", "/user/login", u.Login)
	app.Handle("GET", "/user/logout", u.Logout)
	app.Handle("POST", "/user/forgot-password", u.ForgotPassword)
	app.Handle("GET", "/user/forgot-password", u.ForgotPassword)

	// Register user management and authentication endpoints.
	s := Signup{
		MasterDB:      masterDB,
		Renderer:      renderer,
		Authenticator: authenticator,
	}
	// This route is not authenticated
	app.Handle("POST", "/signup", s.Step1)
	app.Handle("GET", "/signup", s.Step1)

	// Register root
	r := Root{
		MasterDB: masterDB,
		Renderer: renderer,
	}
	// This route is not authenticated
	app.Handle("GET", "/index.html", r.Index)
	app.Handle("GET", "/", r.Index)

	// Register health check endpoint. This route is not authenticated.
	check := Check{
		MasterDB: masterDB,
		Redis:    redis,
		Renderer: renderer,
	}
	app.Handle("GET", "/v1/health", check.Health)

	static := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
		err := web.StaticHandler(ctx, w, r, params, staticDir, "")
		if err != nil {
			if os.IsNotExist(err) {
				rmsg := fmt.Sprintf("%s %s not found", r.Method, r.RequestURI)
				err = weberror.NewErrorMessage(ctx, err, http.StatusNotFound, rmsg)
			} else {
				err = weberror.NewError(ctx, err, http.StatusInternalServerError)
			}

			return web.RenderError(ctx, w, r, err, renderer, tmplLayoutBase, tmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
		}

		return nil
	}

	// Static file server
	app.Handle("GET", "/*", static)

	return app
}
