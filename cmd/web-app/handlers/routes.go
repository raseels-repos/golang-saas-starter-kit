package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/account/account_preference"
	"geeks-accelerator/oss/saas-starter-kit/internal/geonames"
	"geeks-accelerator/oss/saas-starter-kit/internal/mid"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"geeks-accelerator/oss/saas-starter-kit/internal/project"
	"geeks-accelerator/oss/saas-starter-kit/internal/project_route"
	"geeks-accelerator/oss/saas-starter-kit/internal/signup"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account/invite"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_auth"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/ikeikeikeike/go-sitemap-generator/v2/stm"
	"github.com/jmoiron/sqlx"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)

const (
	TmplLayoutBase          = "base.gohtml"
	tmplLayoutSite          = "site.gohtml"
	TmplContentErrorGeneric = "error-generic.gohtml"
)

type AppContext struct {
	Log               *log.Logger
	Env               webcontext.Env
	MasterDB          *sqlx.DB
	MasterDbHost      string
	Redis             *redis.Client
	UserRepo          *user.Repository
	UserAccountRepo   *user_account.Repository
	AccountRepo       *account.Repository
	AccountPrefRepo   *account_preference.Repository
	AuthRepo          *user_auth.Repository
	SignupRepo        *signup.Repository
	InviteRepo        *invite.Repository
	ProjectRepo       *project.Repository
	GeoRepo           *geonames.Repository
	Authenticator     *auth.Authenticator
	StaticDir         string
	TemplateDir       string
	Renderer          web.Renderer
	ProjectRoute      project_route.ProjectRoute
	PreAppMiddleware  []web.Middleware
	PostAppMiddleware []web.Middleware
	AwsSession        *session.Session
}

// API returns a handler for a set of routes.
func APP(shutdown chan os.Signal, appCtx *AppContext) http.Handler {

	// Include the pre middlewares first.
	middlewares := appCtx.PreAppMiddleware

	// Define app middlewares applied to all requests.
	middlewares = append(middlewares,
		mid.Trace(),
		mid.Logger(appCtx.Log),
		mid.Errors(appCtx.Log, appCtx.Renderer),
		mid.Metrics(),
		mid.Panics())

	// Append any global middlewares that should be included after the app middlewares.
	if len(appCtx.PostAppMiddleware) > 0 {
		middlewares = append(middlewares, appCtx.PostAppMiddleware...)
	}

	// Construct the web.App which holds all routes as well as common Middleware.
	app := web.NewApp(shutdown, appCtx.Log, appCtx.Env, middlewares...)

	// Register serverless endpoint. This route is not authenticated.
	serverless := Serverless{
		MasterDB:     appCtx.MasterDB,
		MasterDbHost: appCtx.MasterDbHost,
		AwsSession:   appCtx.AwsSession,
		Renderer:     appCtx.Renderer,
	}
	app.Handle("GET", "/serverless/pending", serverless.Pending)

	// waitDbMid ensures the database is active before allowing the user to access the requested URI.
	waitDbMid := mid.WaitForDbResumed(mid.WaitForDbResumedConfig{
		// Database handle to be used to ensure its online.
		DB: appCtx.MasterDB,

		// WaitHandler defines the handler to render for the user to when the database is being resumed.
		WaitHandler: serverless.Pending,
	})

	// Build a sitemap.
	sm := stm.NewSitemap(1)
	sm.SetVerbose(false)
	sm.SetDefaultHost(appCtx.ProjectRoute.WebAppUrl(""))
	sm.Create()

	smLocAddModified := func(loc stm.URL, filename string) {
		contentPath := filepath.Join(appCtx.TemplateDir, "content", filename)

		file, err := os.Stat(contentPath)
		if err != nil {
			log.Fatalf("main : Add sitemap file modified for %s: %+v", filename, err)
		}

		lm := []interface{}{"lastmod", file.ModTime().Format(time.RFC3339)}
		loc = append(loc, lm)
		sm.Add(loc)
	}

	// Register project management pages.
	p := Projects{
		ProjectRepo: appCtx.ProjectRepo,
		Redis:       appCtx.Redis,
		Renderer:    appCtx.Renderer,
	}
	app.Handle("POST", "/projects/:project_id/update", p.Update, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/projects/:project_id/update", p.Update, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("POST", "/projects/:project_id", p.View, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/projects/:project_id", p.View, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasAuth())
	app.Handle("POST", "/projects/create", p.Create, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/projects/create", p.Create, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/projects", p.Index, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasAuth())

	// Register user management pages.
	us := Users{
		UserRepo:        appCtx.UserRepo,
		UserAccountRepo: appCtx.UserAccountRepo,
		AuthRepo:        appCtx.AuthRepo,
		InviteRepo:      appCtx.InviteRepo,
		GeoRepo:         appCtx.GeoRepo,
		Redis:           appCtx.Redis,
		Renderer:        appCtx.Renderer,
	}
	app.Handle("POST", "/users/:user_id/update", us.Update, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/users/:user_id/update", us.Update, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("POST", "/users/:user_id", us.View, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/users/:user_id", us.View, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasAuth())
	app.Handle("POST", "/users/invite/:hash", us.InviteAccept)
	app.Handle("GET", "/users/invite/:hash", us.InviteAccept)
	app.Handle("POST", "/users/invite", us.Invite, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/users/invite", us.Invite, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("POST", "/users/create", us.Create, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/users/create", us.Create, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/users", us.Index, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasAuth())

	// Register user management and authentication endpoints.
	u := UserRepos{
		UserRepo:        appCtx.UserRepo,
		UserAccountRepo: appCtx.UserAccountRepo,
		AccountRepo:     appCtx.AccountRepo,
		AuthRepo:        appCtx.AuthRepo,
		GeoRepo:         appCtx.GeoRepo,
		Renderer:        appCtx.Renderer,
	}
	app.Handle("POST", "/user/login", u.Login)
	app.Handle("GET", "/user/login", u.Login, waitDbMid)
	app.Handle("GET", "/user/logout", u.Logout)
	app.Handle("POST", "/user/reset-password/:hash", u.ResetConfirm)
	app.Handle("GET", "/user/reset-password/:hash", u.ResetConfirm)
	app.Handle("POST", "/user/reset-password", u.ResetPassword)
	app.Handle("GET", "/user/reset-password", u.ResetPassword)
	app.Handle("POST", "/user/update", u.Update, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasAuth())
	app.Handle("GET", "/user/update", u.Update, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasAuth())
	app.Handle("GET", "/user/account", u.Account, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasAuth())
	app.Handle("GET", "/user/virtual-login/:user_id", u.VirtualLogin, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("POST", "/user/virtual-login", u.VirtualLogin, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/user/virtual-login", u.VirtualLogin, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/user/virtual-logout", u.VirtualLogout, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasAuth())
	app.Handle("GET", "/user/switch-account/:account_id", u.SwitchAccount, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasAuth())
	app.Handle("POST", "/user/switch-account", u.SwitchAccount, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasAuth())
	app.Handle("GET", "/user/switch-account", u.SwitchAccount, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasAuth())
	app.Handle("POST", "/user", u.View, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasAuth())
	app.Handle("GET", "/user", u.View, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasAuth())

	// Register account management endpoints.
	acc := Account{
		AccountRepo:     appCtx.AccountRepo,
		AccountPrefRepo: appCtx.AccountPrefRepo,
		AuthRepo:        appCtx.AuthRepo,
		Authenticator:   appCtx.Authenticator,
		GeoRepo:         appCtx.GeoRepo,
		Renderer:        appCtx.Renderer,
	}
	app.Handle("POST", "/account/update", acc.Update, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/account/update", acc.Update, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("POST", "/account", acc.View, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/account", acc.View, mid.AuthenticateSessionRequired(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))

	// Register signup endpoints.
	s := Signup{
		SignupRepo: appCtx.SignupRepo,
		AuthRepo:   appCtx.AuthRepo,
		GeoRepo:    appCtx.GeoRepo,
		Renderer:   appCtx.Renderer,
	}
	// This route is not authenticated
	app.Handle("POST", "/signup", s.Step1)
	app.Handle("GET", "/signup", s.Step1, waitDbMid)

	// Register example endpoints.
	ex := Examples{
		Renderer: appCtx.Renderer,
	}
	app.Handle("POST", "/examples/flash-messages", ex.FlashMessages, mid.AuthenticateSessionOptional(appCtx.Authenticator))
	app.Handle("GET", "/examples/flash-messages", ex.FlashMessages, mid.AuthenticateSessionOptional(appCtx.Authenticator))
	app.Handle("GET", "/examples/images", ex.Images, mid.AuthenticateSessionOptional(appCtx.Authenticator))

	// Register geo
	g := Geo{
		GeoRepo: appCtx.GeoRepo,
		Redis:   appCtx.Redis,
	}
	app.Handle("GET", "/geo/regions/autocomplete", g.RegionsAutocomplete)
	app.Handle("GET", "/geo/postal_codes/autocomplete", g.PostalCodesAutocomplete)
	app.Handle("GET", "/geo/geonames/postal_code/:postalCode", g.GeonameByPostalCode)
	app.Handle("GET", "/geo/country/:countryCode/timezones", g.CountryTimezones)

	// Register root
	r := Root{
		Renderer:     appCtx.Renderer,
		ProjectRoute: appCtx.ProjectRoute,
		Sitemap:      sm,
	}
	app.Handle("GET", "/api", r.SitePage)
	app.Handle("GET", "/pricing", r.SitePage)
	app.Handle("GET", "/support", r.SitePage)
	app.Handle("GET", "/legal/privacy", r.SitePage)
	app.Handle("GET", "/legal/terms", r.SitePage)
	app.Handle("GET", "/", r.Index, mid.AuthenticateSessionOptional(appCtx.Authenticator))
	app.Handle("GET", "/index.html", r.IndexHtml)
	app.Handle("GET", "/robots.txt", r.RobotTxt)
	app.Handle("GET", "/sitemap.xml", r.SitemapXml)

	// Register health check endpoint. This route is not authenticated.
	check := Check{
		MasterDB: appCtx.MasterDB,
		Redis:    appCtx.Redis,
	}

	app.Handle("GET", "/v1/health", check.Health)
	app.Handle("GET", "/ping", check.Ping)

	// Add sitemap entries for Root.
	smLocAddModified(stm.URL{{"loc", "/"}, {"changefreq", "weekly"}, {"mobile", true}, {"priority", 0.9}}, "site-index.gohtml")
	smLocAddModified(stm.URL{{"loc", "/pricing"}, {"changefreq", "monthly"}, {"mobile", true}, {"priority", 0.8}}, "site-pricing.gohtml")
	smLocAddModified(stm.URL{{"loc", "/support"}, {"changefreq", "monthly"}, {"mobile", true}, {"priority", 0.8}}, "site-support.gohtml")
	smLocAddModified(stm.URL{{"loc", "/api"}, {"changefreq", "monthly"}, {"mobile", true}, {"priority", 0.7}}, "site-api.gohtml")
	smLocAddModified(stm.URL{{"loc", "/legal/privacy"}, {"changefreq", "monthly"}, {"mobile", true}, {"priority", 0.5}}, "legal-privacy.gohtml")
	smLocAddModified(stm.URL{{"loc", "/legal/terms"}, {"changefreq", "monthly"}, {"mobile", true}, {"priority", 0.5}}, "legal-terms.gohtml")

	// Handle static files/pages. Render a custom 404 page when file not found.
	static := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
		err := web.StaticHandler(ctx, w, r, params, appCtx.StaticDir, "")
		if err != nil {
			if os.IsNotExist(err) {
				rmsg := fmt.Sprintf("%s %s not found", r.Method, r.RequestURI)
				err = weberror.NewErrorMessage(ctx, err, http.StatusNotFound, rmsg)
			} else {
				err = weberror.NewError(ctx, err, http.StatusInternalServerError)
			}

			return web.RenderError(ctx, w, r, err, appCtx.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
		}

		return nil
	}

	// Static file server
	app.Handle("GET", "/*", static)

	return app
}
