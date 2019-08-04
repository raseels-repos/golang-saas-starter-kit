package web

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"log"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/dimfeld/httptreemux"
)

// A Handler is a type that handles an http request within our own little mini
// framework.
type Handler func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error

// App is the entrypoint into our application and what configures our context
// object for each of our http handlers. Feel free to add any configuration
// data/logic on this App struct
type App struct {
	*httptreemux.TreeMux
	shutdown chan os.Signal
	log      *log.Logger
	env      webcontext.Env
	mw       []Middleware
}

// NewApp creates an App value that handle a set of routes for the application.
func NewApp(shutdown chan os.Signal, log *log.Logger, env webcontext.Env, mw ...Middleware) *App {
	app := App{
		TreeMux:  httptreemux.New(),
		shutdown: shutdown,
		log:      log,
		env:      env,
		mw:       mw,
	}

	return &app
}

// SignalShutdown is used to gracefully shutdown the app when an integrity
// issue is identified.
func (a *App) SignalShutdown() bool {
	if a.shutdown == nil {
		return false
	}
	a.log.Println("error returned from handler indicated integrity issue, shutting down service")
	a.shutdown <- syscall.SIGSTOP
	return true
}

// Handle is our mechanism for mounting Handlers for a given HTTP verb and path
// pair, this makes for really easy, convenient routing.
func (a *App) Handle(verb, path string, handler Handler, mw ...Middleware) {

	// First wrap handler specific middleware around this handler.
	handler = wrapMiddleware(mw, handler)

	// Add the application's general middleware to the handler chain.
	handler = wrapMiddleware(a.mw, handler)

	// The function to execute for each request.
	h := func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		// Set the context with the required values to
		// process the request.
		v := webcontext.Values{
			Now:       time.Now(),
			Env:       a.env,
			RequestIP: RequestRealIP(r),
		}
		ctx := context.WithValue(r.Context(), webcontext.KeyValues, &v)

		// Call the wrapped handler functions.
		err := handler(ctx, w, r, params)
		if err != nil {

			// If we have specifically handled the error, then no need
			// to initiate a shutdown.
			if webErr, ok := err.(*weberror.Error); ok {
				// Render an error response.
				if rerr := RespondErrorStatus(ctx, w, webErr.Err, webErr.Status); rerr == nil {
					// If there was not error rending the error, then no need to continue.
					return
				}
			}

			a.log.Printf("*****> critical shutdown error: %v", err)
			if ok := a.SignalShutdown(); !ok {
				// When shutdown chan is nil, in the case of unit testing
				// we need to force display of the error.
				panic(err)
			}
			return
		}
	}

	// Add this handler for the specified verb and route.
	a.TreeMux.Handle(verb, path, h)
}
