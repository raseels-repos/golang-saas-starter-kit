package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// sessionTtl defines the auth token expiration.
var sessionTtl = time.Hour * 24

// User represents the User API method handler set.
type User struct {
	MasterDB       *sqlx.DB
	TokenGenerator user.TokenGenerator

	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

// List returns all the existing users in the system.
func (u *User) Find(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req user.UserFindRequest
	if err := web.Decode(r, &req); err != nil {
		return errors.Wrap(err, "")
	}

	res, err := user.Find(ctx, claims, u.MasterDB, req)
	if err != nil {
		return err
	}

	return web.RespondJson(ctx, w, res, http.StatusOK)
}

// Read godoc
// @Summary Read returns the specified user from the system.
// @Description get string by ID
// @Tags user
// @ID get-string-by-int
// @Accept  json
// @Produce  json
// @Param id path int true "User ID"
// @Success 200 {object} user.User
// @Header 200 {string} Token "qwerty"
// @Failure 400 {object} web.Error
// @Failure 403 {object} web.Error
// @Failure 404 {object} web.Error
// @Router /users/{id} [get]
func (u *User) Read(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var includeArchived bool
	if qv := r.URL.Query().Get("include-archived"); qv != "" {
		var err error
		includeArchived, err = strconv.ParseBool(qv)
		if err != nil {
			return errors.Wrapf(err, "Invalid value for include-archived : %s", qv)
		}
	}

	res, err := user.Read(ctx, claims, u.MasterDB, params["id"], includeArchived)
	if err != nil {
		switch err {
		case user.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case user.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case user.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "ID: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, res, http.StatusOK)
}

// Create inserts a new user into the system.
func (u *User) Create(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req user.UserCreateRequest
	if err := web.Decode(r, &req); err != nil {
		return errors.Wrap(err, "")
	}

	res, err := user.Create(ctx, claims, u.MasterDB, req, v.Now)
	if err != nil {
		switch err {
		case user.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "User: %+v", &req)
		}
	}

	return web.RespondJson(ctx, w, res, http.StatusCreated)
}

// Update updates the specified user in the system.
func (u *User) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req user.UserUpdateRequest
	if err := web.Decode(r, &req); err != nil {
		return errors.Wrap(err, "")
	}
	req.ID = params["id"]

	err := user.Update(ctx, claims, u.MasterDB, req, v.Now)
	if err != nil {
		switch err {
		case user.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case user.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case user.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "Id: %s  User: %+v", params["id"], &req)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Update updates the password for a specified user in the system.
func (u *User) UpdatePassword(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req user.UserUpdatePasswordRequest
	if err := web.Decode(r, &req); err != nil {
		return errors.Wrap(err, "")
	}
	req.ID = params["id"]

	err := user.UpdatePassword(ctx, claims, u.MasterDB, req, v.Now)
	if err != nil {
		switch err {
		case user.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case user.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case user.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "Id: %s  User: %+v", params["id"], &req)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Archive soft-deletes the specified user from the system.
func (u *User) Archive(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	err := user.Archive(ctx, claims, u.MasterDB, params["id"], v.Now)
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

// Delete removes the specified user from the system.
func (u *User) Delete(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	err := user.Delete(ctx, claims, u.MasterDB, params["id"])
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

// SwitchAccount updates the claims.
func (u *User) SwitchAccount(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	tkn, err := user.SwitchAccount(ctx, u.MasterDB, u.TokenGenerator, claims, params["accountId"], sessionTtl, v.Now)
	if err != nil {
		switch err {
		case user.ErrAuthenticationFailure:
			return web.NewRequestError(err, http.StatusUnauthorized)
		default:
			return errors.Wrap(err, "switch account")
		}
	}

	return web.RespondJson(ctx, w, tkn, http.StatusNoContent)
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

	tkn, err := user.Authenticate(ctx, u.MasterDB, u.TokenGenerator, email, pass, sessionTtl, v.Now)
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
