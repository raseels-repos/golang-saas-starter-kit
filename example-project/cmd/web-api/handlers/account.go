package handlers

import (
	"context"
	"net/http"
	"strconv"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// Account represents the Account API method handler set.
type Account struct {
	MasterDB *sqlx.DB

	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

// List returns all the existing accounts in the system.
func (a *Account) Find(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req account.AccountFindRequest
	if err := web.Decode(r, &req); err != nil {
		return errors.Wrap(err, "")
	}

	res, err := account.Find(ctx, claims, a.MasterDB, req)
	if err != nil {
		return err
	}

	return web.RespondJson(ctx, w, res, http.StatusOK)
}

// Read godoc
// @Summary Read returns the specified account from the system.
// @Description get string by ID
// @Tags account
// @ID get-string-by-int
// @Accept  json
// @Produce  json
// @Param id path int true "Account ID"
// @Success 200 {object} account.Account
// @Header 200 {string} Token "qwerty"
// @Failure 400 {object} web.Error
// @Failure 403 {object} web.Error
// @Failure 404 {object} web.Error
// @Router /accounts/{id} [get]
func (a *Account) Read(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
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

	res, err := account.Read(ctx, claims, a.MasterDB, params["id"], includeArchived)
	if err != nil {
		switch err {
		case account.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case account.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case account.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "ID: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, res, http.StatusOK)
}

// Create inserts a new account into the system.
func (a *Account) Create(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req account.AccountCreateRequest
	if err := web.Decode(r, &req); err != nil {
		return errors.Wrap(err, "")
	}

	res, err := account.Create(ctx, claims, a.MasterDB, req, v.Now)
	if err != nil {
		switch err {
		case account.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "User: %+v", &req)
		}
	}

	return web.RespondJson(ctx, w, res, http.StatusCreated)
}

// Update updates the specified account in the system.
func (a *Account) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req account.AccountUpdateRequest
	if err := web.Decode(r, &req); err != nil {
		return errors.Wrap(err, "")
	}
	req.ID = params["id"]

	err := account.Update(ctx, claims, a.MasterDB, req, v.Now)
	if err != nil {
		switch err {
		case account.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case account.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case account.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "Id: %s Account: %+v", params["id"], &req)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Archive soft-deletes the specified account from the system.
func (a *Account) Archive(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	err := account.Archive(ctx, claims, a.MasterDB, params["id"], v.Now)
	if err != nil {
		switch err {
		case account.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case account.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case account.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "Id: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Delete removes the specified account from the system.
func (a *Account) Delete(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	err := account.Delete(ctx, claims, a.MasterDB, params["id"])
	if err != nil {
		switch err {
		case account.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case account.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case account.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			return errors.Wrapf(err, "Id: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}
