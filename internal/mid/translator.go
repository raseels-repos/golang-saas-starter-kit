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

func Translator(utrans *ut.UniversalTranslator) web.Middleware {

	// This is the actual middleware function to be executed.
	f := func(after web.Handler) web.Handler {

		// Wrap this handler around the next one provided.
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
			span, ctx := tracer.StartSpanFromContext(ctx, "internal.mid.Translator")
			defer span.Finish()

			m := func() error {
				locale, _ := params["locale"]

				var t ut.Translator
				if len(locale) > 0 {

					var found bool

					if t, found = utrans.GetTranslator(locale); found {
						goto END
					}
				}

				// get and parse the "Accept-Language" http header and return an array
				t, _ = utrans.FindTranslator(httpext.AcceptedLanguages(r)...)
			END:

				ctx = webcontext.ContextWithTranslator(ctx, t)

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
