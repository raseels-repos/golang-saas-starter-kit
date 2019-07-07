package handlers

import (
	"log"
	"net/http"
	"os"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/mid"
	saasSwagger "geeks-accelerator/oss/saas-starter-kit/example-project/internal/mid/saas-swagger"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"github.com/jmoiron/sqlx"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
	_ "geeks-accelerator/oss/saas-starter-kit/example-project/internal/signup"
)

// API returns a handler for a set of routes.
func API(shutdown chan os.Signal, log *log.Logger, masterDB *sqlx.DB, redis *redis.Client, authenticator *auth.Authenticator) http.Handler {

	// Construct the web.App which holds all routes as well as common Middleware.
	app := web.NewApp(shutdown, log, mid.Trace(), mid.Logger(log), mid.Errors(log), mid.Metrics(), mid.Panics())

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
	app.Handle("GET", "/v1/users", u.Find, mid.Authenticate(authenticator))
	app.Handle("POST", "/v1/users", u.Create, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/v1/users/:id", u.Read, mid.Authenticate(authenticator))
	app.Handle("PATCH", "/v1/users", u.Update, mid.Authenticate(authenticator))
	app.Handle("PATCH", "/v1/users/password", u.UpdatePassword, mid.Authenticate(authenticator))
	app.Handle("PATCH", "/v1/users/archive", u.Archive, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("DELETE", "/v1/users/:id", u.Delete, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("PATCH", "/v1/users/switch-account/:account_id", u.SwitchAccount, mid.Authenticate(authenticator))

	// This route is not authenticated
	app.Handle("POST", "/v1/oauth/token", u.Token)

	// Register user account management endpoints.
	ua := UserAccount{
		MasterDB: masterDB,
	}
	app.Handle("GET", "/v1/user_accounts", ua.Find, mid.Authenticate(authenticator))
	app.Handle("POST", "/v1/user_accounts", ua.Create, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/v1/user_accounts/:id", ua.Read, mid.Authenticate(authenticator))
	app.Handle("PATCH", "/v1/user_accounts", ua.Update, mid.Authenticate(authenticator))
	app.Handle("PATCH", "/v1/user_accounts/archive", ua.Archive, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("DELETE", "/v1/user_accounts", ua.Delete, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))

	// Register account endpoints.
	a := Account{
		MasterDB: masterDB,
	}
	app.Handle("GET", "/v1/accounts/:id", a.Read, mid.Authenticate(authenticator))
	app.Handle("PATCH", "/v1/accounts", a.Update, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))

	// Register signup endpoints.
	s := Signup{
		MasterDB: masterDB,
	}
	app.Handle("POST", "/v1/signup", s.Signup)

	// Register project.
	p := Project{
		MasterDB: masterDB,
	}
	app.Handle("GET", "/v1/projects", p.Find, mid.Authenticate(authenticator))
	app.Handle("POST", "/v1/projects", p.Create, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("GET", "/v1/projects/:id", p.Read, mid.Authenticate(authenticator))
	app.Handle("PATCH", "/v1/projects", p.Update, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("PATCH", "/v1/projects/archive", p.Archive, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle("DELETE", "/v1/projects/:id", p.Delete, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))

	// Register swagger documentation.
	// TODO: Add authentication. Current authenticator requires an Authorization header
	// 		 which breaks the browser experience.
	app.Handle("GET", "/swagger/", saasSwagger.WrapHandler)
	app.Handle("GET", "/swagger/*", saasSwagger.WrapHandler)

	return app
}

// Types godoc
// @Summary List of types.
// @Param data body web.FieldError false "Field Error"
// @Param data body web.TimeResponse false "Time Response"
// @Param data body web.EnumResponse false "Enum Response"
// @Param data body web.EnumOption false "Enum Option"
// @Param data body signup.SignupAccount false "SignupAccount"
// @Param data body signup.SignupUser false "SignupUser"
// To support nested types not parsed by swag.
func Types() {}
