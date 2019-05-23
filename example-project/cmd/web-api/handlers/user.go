package handlers

import (
	"context"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// User represents the User API method handler set.
type User struct {
	MasterDB       *sqlx.DB
	TokenGenerator user.TokenGenerator

	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

// List returns all the existing users in the system.
func (u *User) List(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	usrs, err := user.List(ctx, u.MasterDB)
	if err != nil {
		return err
	}

	return web.RespondJson(ctx, w, usrs, http.StatusOK)
}

// Retrieve returns the specified user from the system.
func (u *User) Retrieve(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	usr, err := user.Retrieve(ctx, claims, u.MasterDB, params["id"])
	if err != nil {
		switch err {
		case user.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case user.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case user.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "Id: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, usr, http.StatusOK)
}

// Create inserts a new user into the system.
func (u *User) Create(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	var newU user.NewUser
	if err := web.Decode(r, &newU); err != nil {
		return errors.Wrap(err, "")
	}

	usr, err := user.Create(ctx, u.MasterDB, &newU, v.Now)
	if err != nil {
		return errors.Wrapf(err, "User: %+v", &usr)
	}

	return web.RespondJson(ctx, w, usr, http.StatusCreated)
}

// Update updates the specified user in the system.
func (u *User) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	var upd user.UpdateUser
	if err := web.Decode(r, &upd); err != nil {
		return errors.Wrap(err, "")
	}

	err := user.Update(ctx, u.MasterDB, params["id"], &upd, v.Now)
	if err != nil {
		switch err {
		case user.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case user.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case user.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "Id: %s  User: %+v", params["id"], &upd)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Delete removes the specified user from the system.
func (u *User) Delete(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	err := user.Delete(ctx, u.MasterDB, params["id"])
	if err != nil {
		switch err {
		case user.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case user.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case user.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "Id: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Token handles a request to authenticate a user. It expects a request using
// Basic Auth with a user's email and password. It responds with a JWT.
func (u *User) Token(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	email, pass, ok := r.BasicAuth()
	if !ok {
		err := errors.New("must provide email and password in Basic auth")
		return web.NewRequestError(err, http.StatusUnauthorized)
	}

	tkn, err := user.Authenticate(ctx, u.MasterDB, u.TokenGenerator, v.Now, email, pass)
	if err != nil {
		switch err {
		case user.ErrAuthenticationFailure:
			return web.NewRequestError(err, http.StatusUnauthorized)
		default:
			return errors.Wrap(err, "authenticating")
		}
	}

	return web.RespondJson(ctx, w, tkn, http.StatusOK)
}
