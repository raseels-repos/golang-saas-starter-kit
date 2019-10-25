package template_renderer

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"

	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var (
	errInvalidTemplate = errors.New("Invalid template")
)

type Template struct {
	Funcs        template.FuncMap
	mainTemplate *template.Template
}

// NewTemplate defines a base set of functions that will be applied to all templates
// being rendered.
func NewTemplate(templateFuncs template.FuncMap) *Template {
	t := &Template{}

	// Default functions are defined and available for all templates being rendered.
	// These base function help with provided basic formatting so don't have to use javascript/jquery,
	// transformation happens server-side instead of client-side to provide base-level consistency.
	// Any defined function below will be overwritten if a matching function key is included.
	t.Funcs = template.FuncMap{
		// probably could provide examples of each of these
		"Minus": func(a, b int) int {
			return a - b
		},
		"Add": func(a, b int) int {
			return a + b
		},
		"Mod": func(a, b int) int {
			return int(math.Mod(float64(a), float64(b)))
		},
		"AssetUrl": func(p string) string {
			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			return p
		},
		"AppAssetUrl": func(p string) string {
			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			return p
		},
		"SiteS3Url": func(p string) string {
			return p
		},
		"S3Url": func(p string) string {
			return p
		},
		"AppBaseUrl": func(p string) string {
			return p
		},
		"Http2Https": func(u string) string {
			return strings.Replace(u, "http:", "https:", 1)
		},
		"StringHasPrefix": func(str, match string) bool {
			if strings.HasPrefix(str, match) {
				return true
			}
			return false
		},
		"StringHasSuffix": func(str, match string) bool {
			if strings.HasSuffix(str, match) {
				return true
			}
			return false
		},
		"StringContains": func(str, match string) bool {
			if strings.Contains(str, match) {
				return true
			}
			return false
		},
		"NavPageClass": func(uri, uriMatch, uriClass string) string {
			u, err := url.Parse(uri)
			if err != nil {
				return "?"
			}
			if strings.HasPrefix(u.Path, uriMatch) {
				return uriClass
			}
			return ""
		},
		"UrlEncode": func(k string) string {
			return url.QueryEscape(k)
		},
		"html": func(value interface{}) template.HTML {
			return template.HTML(fmt.Sprint(value))
		},
		"HasAuth": func(ctx context.Context) bool {
			claims, err := auth.ClaimsFromContext(ctx)
			if err != nil {
				return false
			}
			return claims.HasAuth()
		},
		"HasRole": func(ctx context.Context, roles ...string) bool {
			claims, err := auth.ClaimsFromContext(ctx)
			if err != nil {
				return false
			}
			return claims.HasRole(roles...)
		},

		"CmpString": func(str1 string, str2Ptr *string) bool {
			var str2 string
			if str2Ptr != nil {
				str2 = *str2Ptr
			}
			if str1 == str2 {
				return true
			}
			return false
		},
		"HasField": func(v interface{}, name string) bool {
			rv := reflect.ValueOf(v)
			if rv.Kind() == reflect.Ptr {
				rv = rv.Elem()
			}
			if rv.Kind() != reflect.Struct {
				return false
			}
			return rv.FieldByName(name).IsValid()
		},
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values) == 0 {
				return nil, errors.New("invalid dict call")
			}

			dict := make(map[string]interface{})

			for i := 0; i < len(values); i++ {
				key, isset := values[i].(string)
				if !isset {
					if reflect.TypeOf(values[i]).Kind() == reflect.Map {
						m := values[i].(map[string]interface{})
						for i, v := range m {
							dict[i] = v
						}
					} else {
						return nil, errors.New("dict values must be maps")
					}
				} else {
					i++
					if i == len(values) {
						return nil, errors.New("specify the key for non array values")
					}
					dict[key] = values[i]
				}

			}
			return dict, nil
		},
	}
	for fn, f := range templateFuncs {
		t.Funcs[fn] = f
	}

	return t
}

// TemplateRenderer is a custom html/template renderer for Echo framework
type TemplateRenderer struct {
	templateDir string
	// has to be map so can know the name and map the name to the  location / file path
	layoutFiles     map[string]string
	contentFiles    map[string]string
	partialFiles    map[string]string
	enableHotReload bool
	templates       map[string]*template.Template
	globalViewData  map[string]interface{}
	mainTemplate    *template.Template
	errorHandler    func(ctx context.Context, w http.ResponseWriter, req *http.Request, renderer web.Renderer, statusCode int, er error) error
}

// NewTemplateRenderer implements the interface web.Renderer allowing for execution of
// nested html templates. The templateDir should include three directories:
// 	1. layouts: base layouts defined for the entire application
//  2. content: page specific templates that will be nested instead of a layout template
//  3. partials: templates used by multiple layout or content templates
func NewTemplateRenderer(templateDir string, enableHotReload bool, globalViewData map[string]interface{}, tmpl *Template, errorHandler func(ctx context.Context, w http.ResponseWriter, req *http.Request, renderer web.Renderer, statusCode int, er error) error) (*TemplateRenderer, error) {
	r := &TemplateRenderer{
		templateDir:     templateDir,
		layoutFiles:     make(map[string]string),
		contentFiles:    make(map[string]string),
		partialFiles:    make(map[string]string),
		enableHotReload: enableHotReload,
		templates:       make(map[string]*template.Template),
		globalViewData:  globalViewData,
		errorHandler:    errorHandler,
	}

	// Recursively loop through all folders/files in the template directory and group them by their
	// template type. They are filename / filepath for lookup on render.
	err := filepath.Walk(templateDir, func(path string, info os.FileInfo, err error) error {
		dir := filepath.Base(filepath.Dir(path))

		// Skip directories.
		if info.IsDir() {
			return nil
		}

		baseName := filepath.Base(path)

		if dir == "content" {
			r.contentFiles[baseName] = path
		} else if dir == "layouts" {
			r.layoutFiles[baseName] = path
		} else if dir == "partials" {
			r.partialFiles[baseName] = path
		}

		return err
	})
	if err != nil {
		return r, err
	}

	// Main template used to render execute all templates against.
	r.mainTemplate = template.New("main")
	r.mainTemplate, _ = r.mainTemplate.Parse(`{{define "main" }}{{ template "base" . }}{{ end }}`)
	r.mainTemplate.Funcs(tmpl.Funcs)

	// Ensure all layout files render successfully with no errors.
	for _, f := range r.layoutFiles {
		t, err := r.mainTemplate.Clone()
		if err != nil {
			return r, err
		}
		template.Must(t.ParseFiles(f))
	}

	// Ensure all partial files render successfully with no errors.
	for _, f := range r.partialFiles {
		t, err := r.mainTemplate.Clone()
		if err != nil {
			return r, err
		}
		template.Must(t.ParseFiles(f))
	}

	// Ensure all content files render successfully with no errors.
	for _, f := range r.contentFiles {
		t, err := r.mainTemplate.Clone()
		if err != nil {
			return r, err
		}
		template.Must(t.ParseFiles(f))
	}

	return r, nil
}

// Render executes the nested templates and returns the result to the client.
// contentType: supports any content type to allow for rendering text, emails and other formats
// statusCode: the error method calls this function so allow the HTTP Status Code to be set
// data: map[string]interface{} to allow including additional request and globally defined values.
func (r *TemplateRenderer) Render(ctx context.Context, w http.ResponseWriter, req *http.Request, templateLayoutName, templateContentName, contentType string, statusCode int, data map[string]interface{}) error {
	// Not really anyway to render an image response using a template.
	if web.RequestIsImage(req) {
		return nil
	}

	// If the template has not been rendered yet or hot reload is enabled,
	// then parse the template files.
	t, ok := r.templates[templateContentName]
	if !ok || r.enableHotReload {
		var err error
		t, err = r.mainTemplate.Clone()
		if err != nil {
			return err
		}

		// Load the base template file path.
		layoutFile, ok := r.layoutFiles[templateLayoutName]
		if !ok {
			return errors.Wrapf(errInvalidTemplate, "template layout file for %s does not exist", templateLayoutName)
		}
		// The base layout will be the first template.
		files := []string{layoutFile}

		// Append all of the partials that are defined. Not an easy way to determine if the
		// layout or content template contain any references to a partial so load all of them.
		// This assumes that all partial templates should be uniquely named and not conflict with
		// and base layout or content definitions.
		for _, f := range r.partialFiles {
			files = append(files, f)
		}

		// Load the content template file path.
		contentFile, ok := r.contentFiles[templateContentName]
		if !ok {
			return errors.Wrapf(errInvalidTemplate, "template content file for %s does not exist", templateContentName)
		}
		files = append(files, contentFile)

		// Render all of template files
		t = template.Must(t.ParseFiles(files...))
		r.templates[templateContentName] = t
	}

	opts := []ddtrace.StartSpanOption{
		tracer.SpanType(ext.SpanTypeWeb),
		tracer.ResourceName(templateContentName),
	}

	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "web.Render", opts...)
	defer span.Finish()

	// Specific new data map for render to allow values to be overwritten on a request
	// basis.
	// append the global key/pairs
	renderData := make(map[string]interface{}, len(r.globalViewData))
	for k, v := range r.globalViewData {
		renderData[k] = v
	}

	// Add Request URL to render data
	reqData := map[string]interface{}{
		"Url": "",
		"Uri": "",
	}
	if req != nil {
		reqData["Url"] = req.URL.String()
		reqData["Uri"] = req.URL.RequestURI()
	}
	renderData["_Request"] = reqData

	// Add context to render data, this supports template functions having the ability
	// to define context.Context as an argument
	renderData["_Ctx"] = ctx

	if qv := req.URL.Query().Get("test-validation-error"); qv != "" {
		data["validationErrors"] = data["validationDefaults"]
	}

	if qv := req.URL.Query().Get("test-web-error"); qv != "" {
		terr := errors.New("Some random error")
		terr = errors.WithMessage(terr, "Actual error message")
		rerr := weberror.NewError(ctx, terr, http.StatusBadRequest).(*weberror.Error)
		rerr.Message = "Test Web Error Message"
		data["error"] = rerr
	}

	if qv := req.URL.Query().Get("test-error"); qv != "" {
		terr := errors.New("Test error")
		terr = errors.WithMessage(terr, "Error message")
		data["error"] = terr
	}

	// Append request data map to render data last so any previous value can be overwritten.
	if data != nil {
		for k, v := range data {
			renderData[k] = v
		}
	}

	// If there is a session, check for flashes and ensure the session is saved.
	sess := webcontext.ContextSession(ctx)
	if sess != nil {
		// Load any flash messages and append to response data to be included in the rendered template.
		if msgs := sess.Flashes(); len(msgs) > 0 {
			var flashes []webcontext.FlashMsgResponse
			for _, mv := range msgs {
				dat, ok := mv.([]byte)
				if !ok {
					continue
				}
				var msg webcontext.FlashMsgResponse
				if err := json.Unmarshal(dat, &msg); err != nil {
					continue
				}
				flashes = append(flashes, msg)
			}

			renderData["flashes"] = flashes
		}

		// Save the session before writing to the response for the session cookie to be sent to the client.
		if err := sess.Save(req, w); err != nil {
			return errors.WithStack(err)
		}
	}

	// Render template with data.
	if err := t.Execute(w, renderData); err != nil {
		type stackTracer interface {
			StackTrace() errors.StackTrace
		}

		if st, ok := err.(stackTracer); !ok ||st == nil || st.StackTrace() == nil  {
			err = errors.WithStack(err)
		}

		return err
	}

	return nil
}

// Error formats an error and returns the result to the client.
func (r *TemplateRenderer) Error(ctx context.Context, w http.ResponseWriter, req *http.Request, statusCode int, er error) error {
	// If error handler was defined to support formatted response for web, used it.
	if r.errorHandler != nil {
		return r.errorHandler(ctx, w, req, r, statusCode, er)
	}

	// Default response text response of error.
	return web.RespondError(ctx, w, er)
}

// Static serves files from the local file exist.
// If an error is encountered, it will handled by TemplateRenderer.Error
func (tr *TemplateRenderer) Static(rootDir, prefix string) web.Handler {
	h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
		err := web.StaticHandler(ctx, w, r, params, rootDir, prefix)
		if err != nil {
			return tr.Error(ctx, w, r, http.StatusNotFound, err)
		}
		return nil
	}
	return h
}
