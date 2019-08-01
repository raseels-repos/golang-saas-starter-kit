package mid

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/gorilla/sessions"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

func Session(store sessions.Store, sessionName string) web.Middleware {

	// This is the actual middleware function to be executed.
	f := func(after web.Handler) web.Handler {

		// Wrap this handler around the next one provided.
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
			span, ctx := tracer.StartSpanFromContext(ctx, "internal.mid.Session")
			defer span.Finish()

			// Get a session. We're ignoring the error resulted from decoding an
			// existing session: Get() always returns a session, even if empty.
			session, _ := store.Get(r, sessionName)

			// Append the session to the context.
			ctx = webcontext.ContextWithSession(ctx, session)

			return after(ctx, w, r, params)
		}

		return h
	}

	return f
}
