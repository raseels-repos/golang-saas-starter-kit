package template_renderer

import (
	"context"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var (
	errInvalidTemplate = errors.New("Invalid template")

	// Base template to support applying custom
	// TODO try to remove this
	//mainTmpl           = `{{define "main" }} {{ template "base" . }} {{ end }}`
)

type Template struct {
	Funcs template.FuncMap
	mainTemplate *template.Template
}


func NewTemplate(templateFuncs template.FuncMap) *Template {
	t := &Template{}

	// these functions are used and rendered on run-time of web page so don't have to use javascript/jquery
	// to for basic template formatting. transformation happens server-side instead of client-side to
	// provide base-level consistency.
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
	layoutFiles map[string]string
	contentFiles map[string]string
	partialFiles map[string]string
	enableHotReload bool
	templates              map[string]*template.Template
	globalViewData         map[string]interface{}
	//mainTemplate *template.Template
	errorHandler func(ctx context.Context, w http.ResponseWriter, req *http.Request, renderer web.Renderer, statusCode int, er error) error
}

func NewTemplateRenderer(templateDir string, enableHotReload bool, globalViewData map[string]interface{}, tmpl *Template, errorHandler func(ctx context.Context, w http.ResponseWriter, req *http.Request, renderer web.Renderer, statusCode int, er error) error) (*TemplateRenderer, error) {
	r := &TemplateRenderer{
		templateDir: templateDir,
		layoutFiles: make( map[string]string),
		contentFiles: make( map[string]string),
		partialFiles: make( map[string]string),
		enableHotReload: enableHotReload,
		templates: make(map[string]*template.Template),
		globalViewData:globalViewData,
		errorHandler: errorHandler,
	}

	//r.mainTemplate = template.New("main")
	//r.mainTemplate, _ = r.mainTemplate.Parse(mainTmpl)
	//r.mainTemplate.Funcs(tmpl.Funcs)

	err := filepath.Walk(templateDir, func(path string, info os.FileInfo, err error) error {
		dir := filepath.Base(filepath.Dir(path))

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

	// Ensure all layout files render successfully with no errors.
	for _, f := range r.layoutFiles {
		//t, err := r.mainTemplate.Clone()
		//if err != nil {
		//	return r, err
		//}
		t := template.New("main")
		t.Funcs(tmpl.Funcs)
		template.Must(t.ParseFiles(f))
	}

	// Ensure all partial files render successfully with no errors.
	for _, f := range r.partialFiles {
		//t, err := r.mainTemplate.Clone()
		//if err != nil {
		//	return r, err
		//}
		t := template.New("partial")
		t.Funcs(tmpl.Funcs)
		template.Must(t.ParseFiles(f))
	}

	// Ensure all content files render successfully with no errors.
	for _, f := range r.contentFiles {
		//t, err := r.mainTemplate.Clone()
		//if err != nil {
		//	return r, err
		//}
		t := template.New("content")
		t.Funcs(tmpl.Funcs)
		template.Must(t.ParseFiles(f))
	}

	return r, nil
}

// Render renders a template document
func (r *TemplateRenderer) Render(ctx context.Context, w http.ResponseWriter, req *http.Request, templateLayoutName, templateContentName, contentType string, statusCode int, data map[string]interface{}) error {

	t, ok := r.templates[templateContentName]
	if !ok || r.enableHotReload {
		layoutFile, ok := r.layoutFiles[templateLayoutName]
		if !ok {
			return errors.Wrapf(errInvalidTemplate, "template layout file for %s does not exist", templateLayoutName)
		}
		files := []string{layoutFile}

		for _, f := range r.partialFiles {
			files = append(files, f)
		}

		contentFile, ok := r.contentFiles[templateContentName]
		if !ok {
			return errors.Wrapf(errInvalidTemplate, "template content file for %s does not exist", templateContentName)
		}
		files = append(files, contentFile)

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
	renderData := r.globalViewData
	if renderData == nil {
		renderData = make(map[string]interface{})
	}

	// Add Request URL to render data
	reqData := map[string]interface{}{
		"Url": "",
		"Uri":  "",
	}
	if req != nil {
		reqData["Url"] = req.URL.String()
		reqData["Uri"] = req.URL.RequestURI()
	}
	renderData["_Request"] = reqData

	// Add context to render data, this supports template functions having the ability
	// to define context.Context as an argument
	renderData["_Ctx"] = ctx


	// Append request data map to render data last so any previous value can be overwritten.
	if data != nil {
		for k, v := range data {
			renderData[k] = v
		}
	}

	// Render template with data.
	err := t.Execute(w, renderData)
	if err != nil {
		return err
	}

	return nil
}

func (r *TemplateRenderer) Error(ctx context.Context, w http.ResponseWriter, req *http.Request, statusCode int, er error) error {
	// If error hander was defined to support formated response for web, used it.
	if r.errorHandler != nil {
		return  r.errorHandler(ctx, w, req, r, statusCode, er)
	}

	// Default response text response of error.
	return web.RespondError(ctx, w, er)
}

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

func S3Url(baseS3Url, baseS3Origin, p string) string {
	if strings.HasPrefix(p, "http") {
		return p
	}
	org := strings.TrimRight(baseS3Origin, "/")
	if org != "" {
		p = strings.Replace(p, org+"/", "", 1)
	}

	pts := strings.Split(p, "?")
	p = pts[0]

	var rq string
	if len(pts) > 1 {
		rq = pts[1]
	}

	p = strings.TrimLeft(p, "/")

	baseUrl := baseS3Url

	u, err := url.Parse(baseUrl)
	if err != nil {
		return "?"
	}
	ldir := filepath.Base(u.Path)

	if strings.HasPrefix(p, ldir) {
		p = strings.Replace(p, ldir+"/", "", 1)
	}

	u.Path = filepath.Join(u.Path, p)
	u.RawQuery = rq

	return u.String()
}
