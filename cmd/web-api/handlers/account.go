package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	accountref "geeks-accelerator/oss/saas-starter-kit/internal/account/account_preference"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"

	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// Account represents the Account API method handler set.
type Accounts struct {
	Repository AccountRepository

	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

type AccountRepository interface {
	//CanReadAccount(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, accountID string) error
	Find(ctx context.Context, claims auth.Claims, req account.AccountFindRequest) (account.Accounts, error)
	Create(ctx context.Context, claims auth.Claims, req account.AccountCreateRequest, now time.Time) (*account.Account, error)
	ReadByID(ctx context.Context, claims auth.Claims, id string) (*account.Account, error)
	Read(ctx context.Context, claims auth.Claims, req account.AccountReadRequest) (*account.Account, error)
	Update(ctx context.Context, claims auth.Claims, req account.AccountUpdateRequest, now time.Time) error
	Archive(ctx context.Context, claims auth.Claims, req account.AccountArchiveRequest, now time.Time) error
	Delete(ctx context.Context, claims auth.Claims, req account.AccountDeleteRequest) error
}
type AccountPrefRepository interface {
	Find(ctx context.Context, claims auth.Claims, req accountref.AccountPreferenceFindRequest) ([]*accountref.AccountPreference, error)
	FindByAccountID(ctx context.Context, claims auth.Claims, req accountref.AccountPreferenceFindByAccountIDRequest) ([]*accountref.AccountPreference, error)
	Read(ctx context.Context, claims auth.Claims, req accountref.AccountPreferenceReadRequest) (*accountref.AccountPreference, error)
	Set(ctx context.Context, claims auth.Claims, req accountref.AccountPreferenceSetRequest, now time.Time) error
	Archive(ctx context.Context, claims auth.Claims, req accountref.AccountPreferenceArchiveRequest, now time.Time) error
	Delete(ctx context.Context, claims auth.Claims, req accountref.AccountPreferenceDeleteRequest) error
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
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 404 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /accounts/{id} [get]
func (h *Accounts) Read(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	// Handle include-archived query value if set.
	var includeArchived bool
	if v := r.URL.Query().Get("include-archived"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as boolean for include-archived param", v)
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
		}
		includeArchived = b
	}

	res, err := h.Repository.Read(ctx, claims, account.AccountReadRequest{
		ID:              params["id"],
		IncludeArchived: includeArchived,
	})
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case account.ErrNotFound:
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
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 403 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /accounts [patch]
func (h *Accounts) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

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

	err = h.Repository.Update(ctx, claims, req, v.Now)
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
