package mid

import (
	"context"
	"net/http"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/jmoiron/sqlx"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// ctxServerlessKey represents the type of value for the context key.
type ctxServerlessKey int

// Key is used to store/retrieve a Serverless value from a context.Context.
const ServerlessKey ctxServerlessKey = 1

type (
	// WaitForDbResumedConfig defines the config for WaitForDbResumed middleware.
	WaitForDbResumedConfig struct {
		RedirectConfig

		// Database handle to be used to ensure its online.
		DB *sqlx.DB

		// WaitHandler defines the handler to render for the user to when the database is being resumed.
		WaitHandler web.Handler
	}
)

// WaitForDbResumed returns an middleware with for ensuring an serverless database is resumed.
func WaitForDbResumed(config WaitForDbResumedConfig) web.Middleware {

	if config.Skipper == nil {
		config.Skipper = DefaultSkipper
	}
	if config.Code == 0 {
		config.Code = DefaultRedirectConfig.Code
	}

	verifyDb := func() error {
		// When the database is paused, Postgres will return the error, "Canceling statement due to user request"

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
		defer cancel()

		_, err := config.DB.ExecContext(ctx, "SELECT NULL")
		if err != nil {
			return err
		}

		return nil
	}

	// This is the actual middleware function to be executed.
	f := func(after web.Handler) web.Handler {

		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
			span, ctx := tracer.StartSpanFromContext(ctx, "internal.mid.serverless")
			defer span.Finish()

			if config.Skipper(ctx, w, r, params) {
				return after(ctx, w, r, params)
			}

			if err := verifyDb(); err != nil {
				ctx = context.WithValue(ctx, ServerlessKey, err)
				return config.WaitHandler(ctx, w, r, params)
			}

			return after(ctx, w, r, params)
		}

		return h
	}

	return f
}
