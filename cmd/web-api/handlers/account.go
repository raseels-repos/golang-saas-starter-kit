package handlers

import (
	"context"
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"net/http"
	"strconv"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
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
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /accounts/{id} [get]
func (a *Account) Read(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	// Handle included-archived query value if set.
	var includeArchived bool
	if v := r.URL.Query().Get("included-archived"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as boolean for included-archived param", v)
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
		}
		includeArchived = b
	}

	res, err := account.Read(ctx, claims, a.MasterDB, params["id"], includeArchived)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case account.ErrNotFound:

			fmt.Println("HERE!!!!! account.ErrNotFound")

			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusNotFound))
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
// @Success 204
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /accounts [patch]
func (a *Account) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req account.AccountUpdateRequest
	if err := web.Decode(ctx, r, &req); err != nil {
		if _, ok := errors.Cause(err).(*weberror.Error); !ok {
			err = weberror.NewError(ctx, err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	err = account.Update(ctx, claims, a.MasterDB, req, v.Now)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case account.ErrForbidden:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
			}

			return errors.Wrapf(err, "Id: %s Account: %+v", req.ID, &req)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}
