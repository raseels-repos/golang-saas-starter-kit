package mid

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"net/http"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// ErrorForbidden is returned when an authenticated user does not have a
// sufficient role for an action.
func ErrorForbidden(ctx context.Context) error {
	return weberror.NewError(ctx,
		errors.New("you are not authorized for that action"),
		http.StatusForbidden,
	)
}

// Authenticate validates a JWT from the `Authorization` header.
func Authenticate(authenticator *auth.Authenticator) web.Middleware {

	// This is the actual middleware function to be executed.
	f := func(after web.Handler) web.Handler {

		// Wrap this handler around the next one provided.
		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
			span, ctx := tracer.StartSpanFromContext(ctx, "internal.mid.Authenticate")
			defer span.Finish()

			m := func() error {
				authHdr := r.Header.Get("Authorization")
				if authHdr == "" {
					err := errors.New("missing Authorization header")
					return weberror.NewError(ctx, err, http.StatusUnauthorized)
				}

				tknStr, err := parseAuthHeader(authHdr)
				if err != nil {
					return weberror.NewError(ctx, err, http.StatusUnauthorized)
				}

				claims, err := authenticator.ParseClaims(tknStr)
				if err != nil {
					return weberror.NewError(ctx, err, http.StatusUnauthorized)
				}

				// Add claims to the context so they can be retrieved later.
				ctx = context.WithValue(ctx, auth.Key, claims)

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

// HasAuth validates the current user is an authenticated user,
func HasAuth() web.Middleware {

	// This is the actual middleware function to be executed.
	f := func(after web.Handler) web.Handler {

		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
			span, ctx := tracer.StartSpanFromContext(ctx, "internal.mid.HasAuth")
			defer span.Finish()

			m := func() error {
				claims, err := auth.ClaimsFromContext(ctx)
				if err != nil {
					return err
				}

				if !claims.HasAuth() {
					return ErrorForbidden(ctx)
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

// HasRole validates that an authenticated user has at least one role from a
// specified list. This method constructs the actual function that is used.
func HasRole(roles ...string) web.Middleware {

	// This is the actual middleware function to be executed.
	f := func(after web.Handler) web.Handler {

		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
			span, ctx := tracer.StartSpanFromContext(ctx, "internal.mid.HasRole")
			defer span.Finish()

			m := func() error {
				claims, err := auth.ClaimsFromContext(ctx)
				if err != nil {
					return err
				}

				if !claims.HasRole(roles...) {
					return ErrorForbidden(ctx)
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

// parseAuthHeader parses an authorization header. Expected header is of
// the format `Bearer <token>`.
func parseAuthHeader(bearerStr string) (string, error) {
	split := strings.Split(bearerStr, " ")
	if len(split) != 2 || strings.ToLower(split[0]) != "bearer" {
		return "", errors.New("Expected Authorization header format: Bearer <token>")
	}

	return split[1], nil
}
