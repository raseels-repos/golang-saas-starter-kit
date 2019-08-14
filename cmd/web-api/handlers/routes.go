package handlers

import (
	"log"
	"net/http"
	"os"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/account/account_preference"
	"geeks-accelerator/oss/saas-starter-kit/internal/mid"
	saasSwagger "geeks-accelerator/oss/saas-starter-kit/internal/mid/saas-swagger"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	_ "geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"geeks-accelerator/oss/saas-starter-kit/internal/project"
	"geeks-accelerator/oss/saas-starter-kit/internal/signup"
	_ "geeks-accelerator/oss/saas-starter-kit/internal/signup"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account/invite"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_auth"
	"github.com/jmoiron/sqlx"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)

type AppContext struct {
	Log               *log.Logger
	Env               webcontext.Env
	MasterDB          *sqlx.DB
	Redis             *redis.Client
	UserRepo          *user.Repository
	UserAccountRepo   *user_account.Repository
	AccountRepo       *account.Repository
	AccountPrefRepo   *account_preference.Repository
	AuthRepo          *user_auth.Repository
	SignupRepo        *signup.Repository
	InviteRepo        *invite.Repository
	ProjectRepo       *project.Repository
	Authenticator     *auth.Authenticator
	PreAppMiddleware  []web.Middleware
	PostAppMiddleware []web.Middleware
}

// API returns a handler for a set of routes.
func API(shutdown chan os.Signal, appCtx *AppContext) http.Handler {

	// Include the pre middlewares first.
	middlewares := appCtx.PreAppMiddleware

	// Define app middlewares applied to all requests.
	middlewares = append(middlewares,
		mid.Trace(),
		mid.Logger(appCtx.Log),
		mid.Errors(appCtx.Log, nil),
		mid.Metrics(),
		mid.Panics())

	// Append any global middlewares that should be included after the app middlewares.
	if len(appCtx.PostAppMiddleware) > 0 {
		middlewares = append(middlewares, appCtx.PostAppMiddleware...)
	}

	// Construct the web.App which holds all routes as well as common Middleware.
	app := web.NewApp(shutdown, appCtx.Log, appCtx.Env, middlewares...)

	// Register health check endpoint. This route is not authenticated.
	check := Check{
		MasterDB: appCtx.MasterDB,
		Redis:    appCtx.Redis,
	}
	app.Handle("GET", "/v1/health", check.Health)
	app.Handle("GET", "/ping", check.Ping)

	// Register example endpoints.
	ex := Example{
		Project: appCtx.ProjectRepo,
	}
	app.Handle("GET", "/v1/examples/error-response", ex.ErrorResponse)

	// Register user management and authentication endpoints.
	u := User{
		Repository: appCtx.UserRepo,
		Auth:       appCtx.AuthRepo,
	}
	app.Handle("GET", "/v1/users", u.Find, mid.AuthenticateHeader(appCtx.Authenticator))
	app.Handle("POST", "/v1/users", u.Create, mid.AuthenticateHeader(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/v1/users/:id", u.Read, mid.AuthenticateHeader(appCtx.Authenticator))
	app.Handle("PATCH", "/v1/users", u.Update, mid.AuthenticateHeader(appCtx.Authenticator))
	app.Handle("PATCH", "/v1/users/password", u.UpdatePassword, mid.AuthenticateHeader(appCtx.Authenticator))
	app.Handle("PATCH", "/v1/users/archive", u.Archive, mid.AuthenticateHeader(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("DELETE", "/v1/users/:id", u.Delete, mid.AuthenticateHeader(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("PATCH", "/v1/users/switch-account/:account_id", u.SwitchAccount, mid.AuthenticateHeader(appCtx.Authenticator))

	// This route is not authenticated
	app.Handle("POST", "/v1/oauth/token", u.Token)

	// Register user account management endpoints.
	ua := UserAccount{
		Repository: appCtx.UserAccountRepo,
	}
	app.Handle("GET", "/v1/user_accounts", ua.Find, mid.AuthenticateHeader(appCtx.Authenticator))
	app.Handle("POST", "/v1/user_accounts", ua.Create, mid.AuthenticateHeader(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/v1/user_accounts/:user_id/:account_id", ua.Read, mid.AuthenticateHeader(appCtx.Authenticator))
	app.Handle("PATCH", "/v1/user_accounts", ua.Update, mid.AuthenticateHeader(appCtx.Authenticator))
	app.Handle("PATCH", "/v1/user_accounts/archive", ua.Archive, mid.AuthenticateHeader(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("DELETE", "/v1/user_accounts", ua.Delete, mid.AuthenticateHeader(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))

	// Register account endpoints.
	a := Account{
		Repository: appCtx.AccountRepo,
	}
	app.Handle("GET", "/v1/accounts/:id", a.Read, mid.AuthenticateHeader(appCtx.Authenticator))
	app.Handle("PATCH", "/v1/accounts", a.Update, mid.AuthenticateHeader(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))

	// Register signup endpoints.
	s := Signup{
		Repository: appCtx.SignupRepo,
	}
	app.Handle("POST", "/v1/signup", s.Signup)

	// Register project.
	p := Project{
		Repository: appCtx.ProjectRepo,
	}
	app.Handle("GET", "/v1/projects", p.Find, mid.AuthenticateHeader(appCtx.Authenticator))
	app.Handle("POST", "/v1/projects", p.Create, mid.AuthenticateHeader(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/v1/projects/:id", p.Read, mid.AuthenticateHeader(appCtx.Authenticator))
	app.Handle("PATCH", "/v1/projects", p.Update, mid.AuthenticateHeader(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("PATCH", "/v1/projects/archive", p.Archive, mid.AuthenticateHeader(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("DELETE", "/v1/projects/:id", p.Delete, mid.AuthenticateHeader(appCtx.Authenticator), mid.HasRole(auth.RoleAdmin))

	// Register swagger documentation.
	// TODO: Add authentication. Current authenticator requires an Authorization header
	// 		 which breaks the browser experience.
	app.Handle("GET", "/docs/", saasSwagger.WrapHandler)
	app.Handle("GET", "/docs/*", saasSwagger.WrapHandler)

	return app
}

// Types godoc
// @Summary List of types.
// @Param data body weberror.FieldError false "Field Error"
// @Param data body web.TimeResponse false "Time Response"
// @Param data body web.EnumResponse false "Enum Response"
// @Param data body web.EnumMultiResponse false "Enum Multi Response"
// @Param data body web.EnumOption false "Enum Option"
// @Param data body signup.SignupAccount false "SignupAccount"
// @Param data body signup.SignupUser false "SignupUser"
// To support nested types not parsed by swag.
func Types() {}
