package mid

import (
	"context"
	"log"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// Errors handles errors coming out of the call chain. It detects normal
// application errors which are used to respond to the client in a uniform way.
// Unexpected errors (status >= 500) are logged.
func Errors(log *log.Logger, renderer web.Renderer) web.Middleware {

	// This is the actual middleware function to be executed.
	f := func(before web.Handler) web.Handler {

		// Create the handler that will be attached in the middleware chain.
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
			span, ctx := tracer.StartSpanFromContext(ctx, "internal.mid.Errors")
			defer span.Finish()

			if er := before(ctx, w, r, params); er != nil {

				// Log the error.
				log.Printf("%d : ERROR : %+v", span.Context().TraceID(), er)

				// Respond to the error.
				if web.RequestIsJson(r) {
					if err := web.RespondJsonError(ctx, w, er); err != nil {
						return err
					}
				} else if renderer != nil {
					v, err := webcontext.ContextValues(ctx)
					if err != nil {
						return err
					}

					if err := renderer.Error(ctx, w, r, v.StatusCode, er); err != nil {
						return err
					}
				} else {
					if err := web.RespondError(ctx, w, er); err != nil {
						return err
					}
				}

				// If we receive the shutdown err we need to return it
				// back to the base handler to shutdown the service.
				if ok := weberror.IsShutdown(er); ok {
					return er
				}
			}

			// The error has been handled so we can stop propagating it.
			return nil
		}

		return h
	}

	return f
}
