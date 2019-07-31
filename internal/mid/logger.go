package mid

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"log"
	"net/http"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// Logger writes some information about the request to the logs in the
// format: TraceID : (200) GET /foo -> IP ADDR (latency)
func Logger(log *log.Logger) web.Middleware {

	// This is the actual middleware function to be executed.
	f := func(before web.Handler) web.Handler {

		// Create the handler that will be attached in the middleware chain.
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
			span, ctx := tracer.StartSpanFromContext(ctx, "internal.mid.Logger")
			defer span.Finish()

			v, err := webcontext.ContextValues(ctx)
			if err != nil {
				return err
			}

			err = before(ctx, w, r, params)

			log.Printf("%d : (%d) : %s %s -> %s (%s)\n",
				span.Context().TraceID(),
				v.StatusCode,
				r.Method, r.URL.Path,
				r.RemoteAddr, time.Since(v.Now),
			)

			// Return the error so it can be handled further up the chain.
			return err
		}

		return h
	}

	return f
}
