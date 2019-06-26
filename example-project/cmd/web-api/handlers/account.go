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
	"gopkg.in/go-playground/validator.v9"
)

// Account represents the Account API method handler set.
type Account struct {
	MasterDB *sqlx.DB

	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

// Read godoc
// @Summary Get account by ID
// @Description Read returns the specified account from the system.
// @Tags account
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param id path string true "Account ID"
// @Success 200 {object} account.AccountResponse
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
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

	return web.RespondJson(ctx, w, res.Response(ctx), http.StatusOK)
}

// Read godoc
// @Summary Update account by ID
// @Description Update updates the specified account in the system.
// @Tags account
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body account.AccountUpdateRequest true "Update fields"
// @Success 201
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /accounts [patch]
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
		err = errors.WithStack(err)

		_, ok := err.(validator.ValidationErrors)
		if ok {
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		return err
	}

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
			_, ok := err.(validator.ValidationErrors)
			if ok {
				return web.NewRequestError(err, http.StatusBadRequest)
			}

			return errors.Wrapf(err, "Id: %s Account: %+v", params["id"], &req)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}
