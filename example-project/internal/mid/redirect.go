package mid

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"net/http"
)

type (
	// Skipper defines a function to skip middleware. Returning true skips processing
	// the middleware.
	Skipper func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) bool

	// RedirectConfig defines the config for Redirect middleware.
	RedirectConfig struct {
		// Skipper defines a function to skip middleware.
		Skipper

		// Status code to be used when redirecting the request.
		// Optional. Default value http.StatusMovedPermanently.
		Code int
	}

	// DomainNameRedirectConfig defines the details needed to apply redirects based on domain names.
	DomainNameRedirectConfig struct {
		RedirectConfig
		DomainName       	string
		HTTPSEnabled bool
	}

	// redirectLogic represents a function that given a scheme, host and uri
	// can both: 1) determine if redirect is needed (will set ok accordingly) and
	// 2) return the appropriate redirect url.
	redirectLogic func(scheme, host, uri string) (ok bool, url string)
)

const www = "www."

// DefaultRedirectConfig is the default Redirect middleware config.
var DefaultRedirectConfig = RedirectConfig{
	Skipper: DefaultSkipper,
	Code:    http.StatusMovedPermanently,
}

// DefaultSkipper returns false which processes the middleware.
func DefaultSkipper(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) bool {
	return false
}

// HTTPSRedirectWithConfig returns an HTTPSRedirect middleware with config.
// See `HTTPSRedirect()`.
func DomainNameRedirect(config DomainNameRedirectConfig) web.Middleware  {
	return redirect(config.RedirectConfig, func(scheme, host, uri string) (ok bool, url string) {

		// Redirects http requests to https.
		if config.HTTPSEnabled {
			if ok = scheme != "https"; ok {
				url = "https://" + host + uri

				scheme = "https"
			}
		}

		// Redirects all domain name alternatives to the primary hostname.
		if host != config.DomainName {
			host = config.DomainName
		}


		url = scheme + "://" + host + uri

		return
	})
}


// HTTPSRedirect redirects http requests to https.
// For example, http://geeksinthewoods.com will be redirect to https://geeksinthewoods.com.
func HTTPSRedirect() web.Middleware {
	return HTTPSRedirectWithConfig(DefaultRedirectConfig)
}

// HTTPSRedirectWithConfig returns an HTTPSRedirect middleware with config.
// See `HTTPSRedirect()`.
func HTTPSRedirectWithConfig(config RedirectConfig)web.Middleware  {
	return redirect(config, func(scheme, host, uri string) (ok bool, url string) {
		if ok = scheme != "https"; ok {
			url = "https://" + host + uri
		}
		return
	})
}

// HTTPSWWWRedirect redirects http requests to https www.
// For example, http://geeksinthewoods.com will be redirect to https://www.geeksinthewoods.com.
func HTTPSWWWRedirect() web.Middleware {
	return HTTPSWWWRedirectWithConfig(DefaultRedirectConfig)
}

// HTTPSWWWRedirectWithConfig returns an HTTPSRedirect middleware with config.
// See `HTTPSWWWRedirect()`.
func HTTPSWWWRedirectWithConfig(config RedirectConfig) web.Middleware  {
	return redirect(config, func(scheme, host, uri string) (ok bool, url string) {
		if ok = scheme != "https" && host[:3] != www; ok {
			url = "https://www." + host + uri
		}
		return
	})
}

// HTTPSNonWWWRedirect redirects http requests to https non www.
// For example, http://www.geeksinthewoods.com will be redirect to https://geeksinthewoods.com.
func HTTPSNonWWWRedirect() web.Middleware {
	return HTTPSNonWWWRedirectWithConfig(DefaultRedirectConfig)
}

// HTTPSNonWWWRedirectWithConfig returns an HTTPSRedirect middleware with config.
// See `HTTPSNonWWWRedirect()`.
func HTTPSNonWWWRedirectWithConfig(config RedirectConfig) web.Middleware  {
	return redirect(config, func(scheme, host, uri string) (ok bool, url string) {
		if ok = scheme != "https"; ok {
			if host[:3] == www {
				host = host[4:]
			}
			url = "https://" + host + uri
		}
		return
	})
}

// WWWRedirect redirects non www requests to www.
// For example, http://geeksinthewoods.com will be redirect to http://www.geeksinthewoods.com.
func WWWRedirect() web.Middleware {
	return WWWRedirectWithConfig(DefaultRedirectConfig)
}

// WWWRedirectWithConfig returns an HTTPSRedirect middleware with config.
// See `WWWRedirect()`.
func WWWRedirectWithConfig(config RedirectConfig) web.Middleware {
	return redirect(config, func(scheme, host, uri string) (ok bool, url string) {
		if ok = host[:3] != www; ok {
			url = scheme + "://www." + host + uri
		}
		return
	})
}

// NonWWWRedirect redirects www requests to non www.
// For example, http://www.geeksinthewoods.com will be redirect to http://geeksinthewoods.com.
func NonWWWRedirect() web.Middleware {
	return NonWWWRedirectWithConfig(DefaultRedirectConfig)
}

// NonWWWRedirectWithConfig returns an HTTPSRedirect middleware with config.
// See `NonWWWRedirect()`.
func NonWWWRedirectWithConfig(config RedirectConfig)  web.Middleware {
	return redirect(config, func(scheme, host, uri string) (ok bool, url string) {
		if ok = host[:3] == www; ok {
			url = scheme + "://" + host[4:] + uri
		}
		return
	})
}

func redirect(config RedirectConfig, cb redirectLogic) web.Middleware {
	if config.Skipper == nil {
		config.Skipper = DefaultSkipper
	}
	if config.Code == 0 {
		config.Code = DefaultRedirectConfig.Code
	}

	// This is the actual middleware function to be executed.
	f := func(after web.Handler) web.Handler {

		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
			span, ctx := tracer.StartSpanFromContext(ctx, "internal.mid.redirect")
			defer span.Finish()

			if config.Skipper(ctx, w, r, params) {
				return after(ctx, w, r, params)
			}

			scheme := web.RequestScheme(r)
			if ok, url := cb(scheme, r.Host, r.RequestURI); ok {
				http.Redirect(w, r, url, config.Code)
				return nil
			}

			return after(ctx, w, r, params)
		}

		return h
	}

	return f
}
