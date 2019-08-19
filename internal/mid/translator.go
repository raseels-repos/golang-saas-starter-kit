package mid

import (
	"context"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"

	httpext "github.com/go-playground/pkg/net/http"
	ut "github.com/go-playground/universal-translator"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// Translator enables configuration of a language translator configured by
// query parameter or accept language header.
func Translator(utrans *ut.UniversalTranslator) web.Middleware {

	// This is the actual middleware function to be executed.
	f := func(after web.Handler) web.Handler {

		// Wrap this handler around the next one provided.
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
			span, ctx := tracer.StartSpanFromContext(ctx, "internal.mid.Translator")
			defer span.Finish()

			var t ut.Translator
			var queryLocaleFound bool
			if locale := r.URL.Query().Get("locale"); locale != "" {
				t, queryLocaleFound = utrans.GetTranslator(locale)
			}

			if !queryLocaleFound {
				t, _ = utrans.FindTranslator(httpext.AcceptedLanguages(r)...)
			}

			ctx = webcontext.ContextWithTranslator(ctx, t)

			return after(ctx, w, r, params)
		}

		return h
	}

	return f
}
