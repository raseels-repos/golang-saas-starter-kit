package mid

import (
	"context"
	"net/http"
	"regexp"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/tdewolff/minify"
	"github.com/tdewolff/minify/css"
	"github.com/tdewolff/minify/html"
	"github.com/tdewolff/minify/js"
	"github.com/tdewolff/minify/json"
	"github.com/tdewolff/minify/svg"
	"github.com/tdewolff/minify/xml"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// Minify provides minification
func Minify() web.Middleware {

	m := minify.New()
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("image/svg+xml", svg.Minify)
	m.AddFuncRegexp(regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"), js.Minify)
	m.AddFuncRegexp(regexp.MustCompile("[/+]json$"), json.Minify)
	m.AddFuncRegexp(regexp.MustCompile("[/+]xml$"), xml.Minify)

	m.AddFunc(web.MIMEApplicationJSON, json.Minify)
	m.AddFunc(web.MIMEApplicationJSONCharsetUTF8, json.Minify)
	m.AddFunc(web.MIMETextHTML, html.Minify)
	m.AddFunc(web.MIMETextHTMLCharsetUTF8, html.Minify)
	m.AddFunc(web.MIMETextPlain, html.Minify)
	m.AddFunc(web.MIMETextPlainCharsetUTF8, html.Minify)

	// This is the actual middleware function to be executed.
	f := func(after web.Handler) web.Handler {

		// Wrap this handler around the next one provided.
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
			span, ctx := tracer.StartSpanFromContext(ctx, "internal.mid.minify")
			defer span.Finish()

			mw := m.ResponseWriter(w, r)
			defer mw.Close()

			return after(ctx, mw, r, params)
		}

		return h
	}

	return f
}
