package main

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"
	"html/template"

	template_renderer "geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web/template-renderer"
	lru "github.com/hashicorp/golang-lru"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/example-project/cmd/web-app/handlers"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/flag"
	itrace "geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/trace"
	"github.com/kelseyhightower/envconfig"
	"go.opencensus.io/trace"
)

// build is the git version of this program. It is set using build flags in the makefile.
var build = "develop"

const LRU_CACHE_ITEMS = 128

var (
	localCache         *lru.Cache
)

func init() {
	localCache, _ = lru.New(LRU_CACHE_ITEMS)
}

func main() {

	// =========================================================================
	// Logging

	log := log.New(os.Stdout, "WEB_APP : ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	// =========================================================================
	// Configuration
	var cfg struct {
		Env            string        `default:"dev" envconfig:"ENV"`
		HTTP struct {
			Host            string        `default:"0.0.0.0:3000" envconfig:"HTTP_HOST"`
			DebugHost       string        `default:"0.0.0.0:4000" envconfig:"DEBUG_HOST"`
			ReadTimeout     time.Duration `default:"5s" envconfig:"READ_TIMEOUT"`
			WriteTimeout    time.Duration `default:"5s" envconfig:"WRITE_TIMEOUT"`
			ShutdownTimeout time.Duration `default:"5s" envconfig:"SHUTDOWN_TIMEOUT"`
			TemplateDir       string        `default:"./templates" envconfig:"TEMPLATE_DIR"`
			StaticDir       string        `default:"./static" envconfig:"STATIC_DIR"`
		}
		App struct {
			Name             string       `default:"web-app" envconfig:"APP_NAME"`
			StaticS3 struct {
				S3Bucket       string        `envconfig:"APP_STATIC_S3_BUCKET"`
				S3KeyPrefix       string        `envconfig:"APP_STATIC_S3_KEY_PREFIX"`
				EnableCloudFront       bool        `envconfig:"APP_STATIC_S3_ENABLE_CLOUDFRONT"`
			}
		}
		BuildInfo struct {
			CiCommitRefName    string `envconfig:"CI_COMMIT_REF_NAME"`
			CiCommitRefSlug    string `envconfig:"CI_COMMIT_REF_SLUG"`
			CiCommitSha        string `envconfig:"CI_COMMIT_SHA"`
			CiCommitTag        string `envconfig:"CI_COMMIT_TAG"`
			CiCommitTitle      string `envconfig:"CI_COMMIT_TITLE"`
			CiCommitDescription string `envconfig:"CI_COMMIT_DESCRIPTION"`
			CiJobId            string `envconfig:"CI_COMMIT_JOB_ID"`
			CiJobUrl           string `envconfig:"CI_COMMIT_JOB_URL"`
			CiPipelineId       string `envconfig:"CI_COMMIT_PIPELINE_ID"`
			CiPipelineUrl      string `envconfig:"CI_COMMIT_PIPELINE_URL"`
		}
		DB struct {
			DialTimeout time.Duration `default:"5s" envconfig:"DIAL_TIMEOUT"`
			Host        string        `default:"mongo:27017/gotraining" envconfig:"HOST"`
		}
		Trace struct {
			Host         string        `default:"http://tracer:3002/v1/publish" envconfig:"HOST"`
			BatchSize    int           `default:"1000" envconfig:"BATCH_SIZE"`
			SendInterval time.Duration `default:"15s" envconfig:"SEND_INTERVAL"`
			SendTimeout  time.Duration `default:"500ms" envconfig:"SEND_TIMEOUT"`
		}
		Auth struct {
			KeyID          string `envconfig:"KEY_ID"`
			PrivateKeyFile string `default:"/app/private.pem" envconfig:"PRIVATE_KEY_FILE"`
			Algorithm      string `default:"RS256" envconfig:"ALGORITHM"`
		}
	}

	if err := envconfig.Process("WEB_APP", &cfg); err != nil {
		log.Fatalf("main : Parsing Config : %v", err)
	}

	if err := flag.Process(&cfg); err != nil {
		if err != flag.ErrHelp {
			log.Fatalf("main : Parsing Command Line : %v", err)
		}
		return // We displayed help.
	}

	// =========================================================================
	// App Starting

	// Print the build version for our logs. Also expose it under /debug/vars.
	expvar.NewString("build").Set(build)
	log.Printf("main : Started : Application Initializing version %q", build)
	defer log.Println("main : Completed")

	cfgJSON, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		log.Fatalf("main : Marshalling Config to JSON : %v", err)
	}

	// TODO: Validate what is being written to the logs. We don't
	// want to leak credentials or anything that can be a security risk.
	log.Printf("main : Config : %v\n", string(cfgJSON))

	// =========================================================================
	// Template Renderer
	// Implements interface web.Renderer to support alternative renderer

	var (
		staticS3BaseUrl string
		staticS3CloudFrontOriginPrefix string
	)
	if cfg.App.StaticS3.S3Bucket != "" {
		// TODO: lookup s3 url/cloud front distribution based on s3 bucket
	}

	// Append query string value to break browser cache used for services
	// that render responses for a browser with the following:
	// 	1. when env=dev, the current timestamp will be used to ensure every
	// 		request will skip browser cache.
	// 	2. all other envs, ie stage and prod. The commit hash will be used to
	// 		ensure that all cache will be reset with each new deployment.
	browserCacheBusterQueryString := func() string {
		var v string
		if cfg.Env == "dev" {
			// On dev always break cache.
			v = fmt.Sprintf("%d", time.Now().UTC().Unix())
		} else {
			// All other envs, use the current commit hash for the build
			v = cfg.BuildInfo.CiCommitSha
		}
		return v
	}

	// Helper method for appending the browser cache buster as a query string to
	// support breaking browser cache when necessary
	browserCacheBusterFunc := browserCacheBuster(browserCacheBusterQueryString)

	// Need defined functions below since they require config values, able to add additional functions
	// here to extend functionality.
	tmplFuncs := template.FuncMap{
		"BuildInfo": func(k string) string {
			r := reflect.ValueOf(cfg.BuildInfo)
			f := reflect.Indirect(r).FieldByName(k)
			return f.String()
		},
		"SiteBaseUrl": func(p string) string {
			u, err := url.Parse(cfg.HTTP.Host)
			if err != nil {
				return "?"
			}
			u.Path = p
			return u.String()
		},
		"AssetUrl": func(p string) string {
			var u string
			if staticS3BaseUrl != "" {
				u = template_renderer.S3Url(staticS3BaseUrl, staticS3CloudFrontOriginPrefix, p)
			} else {
				if !strings.HasPrefix(p, "/") {
					p = "/" + p
				}
				u = p
			}

			u = browserCacheBusterFunc( u)

			return u
		},
		"SiteAssetUrl": func(p string) string {
			var u string
			if staticS3BaseUrl != "" {
				u = template_renderer.S3Url(staticS3BaseUrl, staticS3CloudFrontOriginPrefix, filepath.Join(cfg.App.Name, p))
			} else {
				if !strings.HasPrefix(p, "/") {
					p = "/" + p
				}
				u = p
			}

			u = browserCacheBusterFunc( u)

			return u
		},
		"SiteS3Url": func(p string) string {
			var u string
			if staticS3BaseUrl != "" {
				u = template_renderer.S3Url(staticS3BaseUrl, staticS3CloudFrontOriginPrefix, filepath.Join(cfg.App.Name, p))
			} else {
				u = p
			}
			return u
		},
		"S3Url": func(p string) string {
			var u string
			if staticS3BaseUrl != "" {
				u = template_renderer.S3Url(staticS3BaseUrl, staticS3CloudFrontOriginPrefix, p)
			} else {
				u = p
			}
			return u
		},
	}

	//
	t := template_renderer.NewTemplate(tmplFuncs)

	// global variables exposed for rendering of responses with templates
	gvd := map[string]interface{}{
		"_App": map[string]interface{}{
			"ENV":            cfg.Env,
			"BuildInfo":      cfg.BuildInfo,
			"BuildVersion": build,
		},
	}

	// Custom error handler to support rendering user friendly error page for improved web experience.
	eh := func(ctx context.Context, w http.ResponseWriter, r *http.Request, renderer web.Renderer, statusCode int, er error) error {
		data := map[string]interface{}{}

		return renderer.Render(ctx, w, r,
			"base.tmpl", // base layout file to be used for rendering of errors
			"error.tmpl", // generic format for errors, could select based on status code
								web.MIMETextHTMLCharsetUTF8,
								http.StatusOK,
								data,
		)
	}

	// Enable template renderer to reload and parse template files when generating a response of dev
	// for a more developer friendly process. Any changes to the template files will be included
	// without requiring re-build/re-start of service.
	// This only supports files that already exist, if a new template file is added, then the
	// serivce needs to be restarted, but not rebuilt.
	enableHotReload := cfg.Env == "dev"

	// Template Renderer used to generate HTML response for web experience.
	renderer, err := template_renderer.NewTemplateRenderer(cfg.HTTP.TemplateDir, enableHotReload, gvd, t, eh)
	if err != nil {
		log.Fatalf("main : Marshalling Config to JSON : %v", err)
	}

	// =========================================================================
	// Start Tracing Support

	logger := func(format string, v ...interface{}) {
		log.Printf(format, v...)
	}

	log.Printf("main : Tracing Started : %s", cfg.Trace.Host)
	exporter, err := itrace.NewExporter(logger, cfg.Trace.Host, cfg.Trace.BatchSize, cfg.Trace.SendInterval, cfg.Trace.SendTimeout)
	if err != nil {
		log.Fatalf("main : RegiTracingster : ERROR : %v", err)
	}
	defer func() {
		log.Printf("main : Tracing Stopping : %s", cfg.Trace.Host)
		batch, err := exporter.Close()
		if err != nil {
			log.Printf("main : Tracing Stopped : ERROR : Batch[%d] : %v", batch, err)
		} else {
			log.Printf("main : Tracing Stopped : Flushed Batch[%d]", batch)
		}
	}()

	trace.RegisterExporter(exporter)
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})

	// =========================================================================
	// Start Debug Service. Not concerned with shutting this down when the
	// application is being shutdown.
	//
	// /debug/vars - Added to the default mux by the expvars package.
	// /debug/pprof - Added to the default mux by the net/http/pprof package.
	if cfg.HTTP.DebugHost != "" {
		go func() {
			log.Printf("main : Debug Listening %s", cfg.HTTP.DebugHost)
			log.Printf("main : Debug Listener closed : %v", http.ListenAndServe(cfg.HTTP.DebugHost, http.DefaultServeMux))
		}()
	}

	// =========================================================================
	// Start APP Service

	// Make a channel to listen for an interrupt or terminate signal from the OS.
	// Use a buffered channel because the signal package requires it.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	api := http.Server{
		Addr:           cfg.HTTP.Host,
		Handler:        handlers.APP(shutdown, log, cfg.HTTP.StaticDir, cfg.HTTP.TemplateDir, nil, nil, renderer),
		ReadTimeout:    cfg.HTTP.ReadTimeout,
		WriteTimeout:   cfg.HTTP.WriteTimeout,
		MaxHeaderBytes: 1 << 20,
	}

	// Make a channel to listen for errors coming from the listener. Use a
	// buffered channel so the goroutine can exit if we don't collect this error.
	serverErrors := make(chan error, 1)

	// Start the service listening for requests.
	go func() {
		log.Printf("main : APP Listening %s", cfg.HTTP.Host)
		serverErrors <- api.ListenAndServe()
	}()

	// =========================================================================
	// Shutdown

	// Blocking main and waiting for shutdown.
	select {
	case err := <-serverErrors:
		log.Fatalf("main : Error starting server: %v", err)

	case sig := <-shutdown:
		log.Printf("main : %v : Start shutdown..", sig)

		// Create context for Shutdown call.
		ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()

		// Asking listener to shutdown and load shed.
		err := api.Shutdown(ctx)
		if err != nil {
			log.Printf("main : Graceful shutdown did not complete in %v : %v", cfg.HTTP.ShutdownTimeout, err)
			err = api.Close()
		}

		// Log the status of this shutdown.
		switch {
		case sig == syscall.SIGSTOP:
			log.Fatal("main : Integrity issue caused shutdown")
		case err != nil:
			log.Fatalf("main : Could not stop server gracefully : %v", err)
		}
	}
}

// browserCacheBuster appends a the query string param v to a given url with
// a value based on the value returned from cacheBusterValueFunc
func browserCacheBuster(cacheBusterValueFunc func() string) func(uri string) string {
	f := func(uri string) string {
		v := cacheBusterValueFunc()
		if v == "" {
			return uri
		}

		u, err := url.Parse(uri)
		if err != nil {
			return ""
		}
		q := u.Query()
		q.Set("v", v)
		u.RawQuery = q.Encode()

		return u.String()
	}

	return f
}

/*
	"S3ImgSrcLarge": func(p string) template.HTMLAttr {
			res, _ := blower_display.S3ImgSrc(cfg, site, p, []int{320, 480, 800}, true)
			return template.HTMLAttr(res)
		},
		"S3ImgThumbSrcLarge": func(p string) template.HTMLAttr {
			res, _ := blower_display.S3ImgSrc(cfg, site, p, []int{320, 480, 800}, false)
			return template.HTMLAttr(res)
		},
		"S3ImgSrcMedium": func(p string) template.HTMLAttr {
			res, _ := blower_display.S3ImgSrc(cfg, site, p, []int{320, 640}, true)
			return template.HTMLAttr(res)
		},
		"S3ImgThumbSrcMedium": func(p string) template.HTMLAttr {
			res, _ := blower_display.S3ImgSrc(cfg, site, p, []int{320, 640}, false)
			return template.HTMLAttr(res)
		},
		"S3ImgSrcSmall": func(p string) template.HTMLAttr {
			res, _ := blower_display.S3ImgSrc(cfg, site, p, []int{320}, true)
			return template.HTMLAttr(res)
		},
		"S3ImgThumbSrcSmall": func(p string) template.HTMLAttr {
			res, _ := blower_display.S3ImgSrc(cfg, site, p, []int{320}, false)
			return template.HTMLAttr(res)
		},
		"S3ImgSrc": func(p string, sizes []int) template.HTMLAttr {
			res, _ := blower_display.S3ImgSrc(cfg, site, p, sizes, true)
			return template.HTMLAttr(res)
		},
		"S3ImgUrl": func(p string, size int) string {
			res, _ := blower_display.S3ImgUrl(cfg, site, p, size)
			return res
		},
 */