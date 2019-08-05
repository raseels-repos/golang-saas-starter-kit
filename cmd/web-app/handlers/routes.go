package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"geeks-accelerator/oss/saas-starter-kit/internal/mid"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/notify"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	project_routes "geeks-accelerator/oss/saas-starter-kit/internal/project-routes"
	"github.com/jmoiron/sqlx"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)

const (
	TmplLayoutBase          = "base.gohtml"
	tmplLayoutSite          = "site.gohtml"
	TmplContentErrorGeneric = "error-generic.gohtml"
)

// API returns a handler for a set of routes.
func APP(shutdown chan os.Signal, log *log.Logger, env webcontext.Env, staticDir, templateDir string, masterDB *sqlx.DB, redis *redis.Client, authenticator *auth.Authenticator, projectRoutes project_routes.ProjectRoutes, secretKey string, notifyEmail notify.Email, renderer web.Renderer, globalMids ...web.Middleware) http.Handler {

	// Define base middlewares applied to all requests.
	middlewares := []web.Middleware{
		mid.Trace(), mid.Logger(log), mid.Errors(log, renderer), mid.Metrics(), mid.Panics(),
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
		Redis:    redis,
		Renderer: renderer,
	}
	app.Handle("POST", "/projects/:project_id/update", p.Update, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/projects/:project_id/update", p.Update, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("POST", "/projects/:project_id", p.View, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/projects/:project_id", p.View, mid.AuthenticateSessionRequired(authenticator), mid.HasAuth())
	app.Handle("POST", "/projects/create", p.Create, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/projects/create", p.Create, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/projects", p.Index, mid.AuthenticateSessionRequired(authenticator), mid.HasAuth())

	// Register user management pages.
	us := Users{
		MasterDB:      masterDB,
		Redis:         redis,
		Renderer:      renderer,
		Authenticator: authenticator,
		ProjectRoutes: projectRoutes,
		NotifyEmail:   notifyEmail,
		SecretKey:     secretKey,
	}
	app.Handle("POST", "/users/:user_id/update", us.Update, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/users/:user_id/update", us.Update, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("POST", "/users/:user_id", us.View, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/users/:user_id", us.View, mid.AuthenticateSessionRequired(authenticator), mid.HasAuth())
	app.Handle("POST", "/users/invite/:hash", us.InviteAccept)
	app.Handle("GET", "/users/invite/:hash", us.InviteAccept)
	app.Handle("POST", "/users/invite", us.Invite, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/users/invite", us.Invite, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("POST", "/users/create", us.Create, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/users/create", us.Create, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/users", us.Index, mid.AuthenticateSessionRequired(authenticator), mid.HasAuth())

	// Register user management and authentication endpoints.
	u := User{
		MasterDB:      masterDB,
		Renderer:      renderer,
		Authenticator: authenticator,
		ProjectRoutes: projectRoutes,
		NotifyEmail:   notifyEmail,
		SecretKey:     secretKey,
	}
	app.Handle("POST", "/user/login", u.Login)
	app.Handle("GET", "/user/login", u.Login)
	app.Handle("GET", "/user/logout", u.Logout)
	app.Handle("POST", "/user/reset-password/:hash", u.ResetConfirm)
	app.Handle("GET", "/user/reset-password/:hash", u.ResetConfirm)
	app.Handle("POST", "/user/reset-password", u.ResetPassword)
	app.Handle("GET", "/user/reset-password", u.ResetPassword)
	app.Handle("POST", "/user/update", u.Update, mid.AuthenticateSessionRequired(authenticator), mid.HasAuth())
	app.Handle("GET", "/user/update", u.Update, mid.AuthenticateSessionRequired(authenticator), mid.HasAuth())
	app.Handle("GET", "/user/account", u.Account, mid.AuthenticateSessionRequired(authenticator), mid.HasAuth())
	app.Handle("GET", "/user/virtual-login/:user_id", u.VirtualLogin, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("POST", "/user/virtual-login", u.VirtualLogin, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/user/virtual-login", u.VirtualLogin, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/user/virtual-logout", u.VirtualLogout, mid.AuthenticateSessionRequired(authenticator), mid.HasAuth())
	app.Handle("GET", "/user/switch-account/:account_id", u.SwitchAccount, mid.AuthenticateSessionRequired(authenticator), mid.HasAuth())
	app.Handle("POST", "/user/switch-account", u.SwitchAccount, mid.AuthenticateSessionRequired(authenticator), mid.HasAuth())
	app.Handle("GET", "/user/switch-account", u.SwitchAccount, mid.AuthenticateSessionRequired(authenticator), mid.HasAuth())
	app.Handle("POST", "/user", u.View, mid.AuthenticateSessionRequired(authenticator), mid.HasAuth())
	app.Handle("GET", "/user", u.View, mid.AuthenticateSessionRequired(authenticator), mid.HasAuth())

	// Register account management endpoints.
	acc := Account{
		MasterDB:      masterDB,
		Renderer:      renderer,
		Authenticator: authenticator,
	}
	app.Handle("POST", "/account/update", acc.Update, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/account/update", acc.Update, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("POST", "/account", acc.View, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/account", acc.View, mid.AuthenticateSessionRequired(authenticator), mid.HasRole(auth.RoleAdmin))

	// Register user management and authentication endpoints.
	s := Signup{
		MasterDB:      masterDB,
		Renderer:      renderer,
		Authenticator: authenticator,
	}
	// This route is not authenticated
	app.Handle("POST", "/signup", s.Step1)
	app.Handle("GET", "/signup", s.Step1)

	// Register example endpoints.
	ex := Examples{
		Renderer: renderer,
	}
	app.Handle("POST", "/examples/flash-messages", ex.FlashMessages, mid.AuthenticateSessionOptional(authenticator))
	app.Handle("GET", "/examples/flash-messages", ex.FlashMessages, mid.AuthenticateSessionOptional(authenticator))
	app.Handle("GET", "/examples/images", ex.Images, mid.AuthenticateSessionOptional(authenticator))

	// Register geo
	g := Geo{
		MasterDB: masterDB,
		Redis:    redis,
	}
	app.Handle("GET", "/geo/regions/autocomplete", g.RegionsAutocomplete)
	app.Handle("GET", "/geo/postal_codes/autocomplete", g.PostalCodesAutocomplete)
	app.Handle("GET", "/geo/geonames/postal_code/:postalCode", g.GeonameByPostalCode)
	app.Handle("GET", "/geo/country/:countryCode/timezones", g.CountryTimezones)

	// Register root
	r := Root{
		MasterDB:      masterDB,
		Renderer:      renderer,
		ProjectRoutes: projectRoutes,
	}
	app.Handle("GET", "/api", r.SitePage)
	app.Handle("GET", "/pricing", r.SitePage)
	app.Handle("GET", "/support", r.SitePage)
	app.Handle("GET", "/legal/privacy", r.SitePage)
	app.Handle("GET", "/legal/terms", r.SitePage)
	app.Handle("GET", "/", r.Index, mid.AuthenticateSessionOptional(authenticator))
	app.Handle("GET", "/index.html", r.IndexHtml)
	app.Handle("GET", "/robots.txt", r.RobotTxt)

	// Register health check endpoint. This route is not authenticated.
	check := Check{
		MasterDB: masterDB,
		Redis:    redis,
		Renderer: renderer,
	}
	app.Handle("GET", "/v1/health", check.Health)

	// Handle static files/pages. Render a custom 404 page when file not found.
	static := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
		err := web.StaticHandler(ctx, w, r, params, staticDir, "")
		if err != nil {
			if os.IsNotExist(err) {
				rmsg := fmt.Sprintf("%s %s not found", r.Method, r.RequestURI)
				err = weberror.NewErrorMessage(ctx, err, http.StatusNotFound, rmsg)
			} else {
				err = weberror.NewError(ctx, err, http.StatusInternalServerError)
			}

			return web.RenderError(ctx, w, r, err, renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
		}

		return nil
	}

	// Static file server
	app.Handle("GET", "/*", static)

	return app
}
