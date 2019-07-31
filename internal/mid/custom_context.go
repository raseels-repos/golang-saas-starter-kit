package mid

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"net/http"
)

// CustomContext sets a default set of context values.
func CustomContext(values map[interface{}]interface{}) web.Middleware {

	// This is the actual middleware function to be executed.
	f := func(after web.Handler) web.Handler {

		// Wrap this handler around the next one provided.
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
			span, ctx := tracer.StartSpanFromContext(ctx, "internal.mid.CustomContext")
			defer span.Finish()

			m := func() error {
				for k, v := range values {
					if cv := ctx.Value(k); cv == nil {
						ctx = context.WithValue(ctx, k, v)
					}
				}

				return nil
			}

			if err := m(); err != nil {
				if web.RequestIsJson(r) {
					return web.RespondJsonError(ctx, w, err)
				}
				return err
			}

			return after(ctx, w, r, params)
		}

		return h
	}

	return f
}
