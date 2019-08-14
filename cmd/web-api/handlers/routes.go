package handlers

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"log"
	"net/http"
	"os"

	"geeks-accelerator/oss/saas-starter-kit/internal/mid"
	saasSwagger "geeks-accelerator/oss/saas-starter-kit/internal/mid/saas-swagger"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	_ "geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"geeks-accelerator/oss/saas-starter-kit/internal/project"
	_ "geeks-accelerator/oss/saas-starter-kit/internal/signup"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)


type AppContext struct {
	Log *log.Logger
	Env webcontext.Env
	Repo *user.Repository
	MasterDB *sqlx.DB
	Redis *redis.Client
	Authenticator *auth.Authenticator
	PreAppMiddleware []web.Middleware
	PostAppMiddleware []web.Middleware
}


// API returns a handler for a set of routes.
func API(shutdown chan os.Signal, appContext *AppContext ) http.Handler {

	// Include the pre middlewares first.
	middlewares := appContext.PreAppMiddleware

	// Define app middlewares applied to all requests.
	middlewares = append(middlewares,
		mid.Trace(),
		mid.Logger(appContext.Log),
		mid.Errors(appContext.Log, nil),
		mid.Metrics(),
		mid.Panics())

	// Append any global middlewares that should be included after the app middlewares.
	if len(appContext.PostAppMiddleware) > 0 {
		middlewares = append(middlewares, appContext.PostAppMiddleware...)
	}

	// Construct the web.App which holds all routes as well as common Middleware.
	app := web.NewApp(shutdown, appContext.Log, appContext.Env, middlewares...)

	// Register health check endpoint. This route is not authenticated.
	check := Check{
		MasterDB: appContext.MasterDB,
		Redis:    appContext.Redis,
	}
	app.Handle("GET", "/v1/health", check.Health)
	app.Handle("GET", "/ping", check.Ping)

	// Register user management and authentication endpoints.
	u := User{
		MasterDB:       appContext.MasterDB,
		TokenGenerator: authenticator,
	}
	app.Handle("GET", "/v1/users", u.Find, mid.AuthenticateHeader(authenticator))
	app.Handle("POST", "/v1/users", u.Create, mid.AuthenticateHeader(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/v1/users/:id", u.Read, mid.AuthenticateHeader(authenticator))
	app.Handle("PATCH", "/v1/users", u.Update, mid.AuthenticateHeader(authenticator))
	app.Handle("PATCH", "/v1/users/password", u.UpdatePassword, mid.AuthenticateHeader(authenticator))
	app.Handle("PATCH", "/v1/users/archive", u.Archive, mid.AuthenticateHeader(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("DELETE", "/v1/users/:id", u.Delete, mid.AuthenticateHeader(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("PATCH", "/v1/users/switch-account/:account_id", u.SwitchAccount, mid.AuthenticateHeader(authenticator))

	// This route is not authenticated
	app.Handle("POST", "/v1/oauth/token", u.Token)

	// Register user account management endpoints.
	ua := UserAccount{
		MasterDB: masterDB,
	}
	app.Handle("GET", "/v1/user_accounts", ua.Find, mid.AuthenticateHeader(authenticator))
	app.Handle("POST", "/v1/user_accounts", ua.Create, mid.AuthenticateHeader(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/v1/user_accounts/:user_id/:account_id", ua.Read, mid.AuthenticateHeader(authenticator))
	app.Handle("PATCH", "/v1/user_accounts", ua.Update, mid.AuthenticateHeader(authenticator))
	app.Handle("PATCH", "/v1/user_accounts/archive", ua.Archive, mid.AuthenticateHeader(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("DELETE", "/v1/user_accounts", ua.Delete, mid.AuthenticateHeader(authenticator), mid.HasRole(auth.RoleAdmin))

	// Register account endpoints.
	a := Account{
		MasterDB: masterDB,
	}
	app.Handle("GET", "/v1/accounts/:id", a.Read, mid.AuthenticateHeader(authenticator))
	app.Handle("PATCH", "/v1/accounts", a.Update, mid.AuthenticateHeader(authenticator), mid.HasRole(auth.RoleAdmin))

	// Register signup endpoints.
	s := Signup{
		MasterDB: masterDB,
	}
	app.Handle("POST", "/v1/signup", s.Signup)

	// Register project.
	p := Project{
		MasterDB: masterDB,
	}
	app.Handle("GET", "/v1/projects", p.Find, mid.AuthenticateHeader(authenticator))
	app.Handle("POST", "/v1/projects", p.Create, mid.AuthenticateHeader(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/v1/projects/:id", p.Read, mid.AuthenticateHeader(authenticator))
	app.Handle("PATCH", "/v1/projects", p.Update, mid.AuthenticateHeader(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("PATCH", "/v1/projects/archive", p.Archive, mid.AuthenticateHeader(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("DELETE", "/v1/projects/:id", p.Delete, mid.AuthenticateHeader(authenticator), mid.HasRole(auth.RoleAdmin))

	app.Handle("GET", "/v1/examples/error-response", ExampleErrorResponse)

	// Register swagger documentation.
	// TODO: Add authentication. Current authenticator requires an Authorization header
	// 		 which breaks the browser experience.
	app.Handle("GET", "/docs/", saasSwagger.WrapHandler)
	app.Handle("GET", "/docs/*", saasSwagger.WrapHandler)

	return app
}

// ExampleErrorResponse returns example error messages.
func ExampleErrorResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	if qv := r.URL.Query().Get("test-validation-error"); qv != "" {
		_, err := project.Create(ctx, auth.Claims{}, nil, project.ProjectCreateRequest{}, v.Now)
		return web.RespondJsonError(ctx, w, err)

	}

	if qv := r.URL.Query().Get("test-web-error"); qv != "" {
		terr := errors.New("Some random error")
		terr = errors.WithMessage(terr, "Actual error message")
		rerr := weberror.NewError(ctx, terr, http.StatusBadRequest).(*weberror.Error)
		rerr.Message = "Test Web Error Message"
		return web.RespondJsonError(ctx, w, rerr)
	}

	if qv := r.URL.Query().Get("test-error"); qv != "" {
		terr := errors.New("Test error")
		terr = errors.WithMessage(terr, "Error message")
		return web.RespondJsonError(ctx, w, terr)
	}

	return nil
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
