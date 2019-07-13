package handlers

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user_account"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
	"net/http"
	"strconv"
	"strings"
)

// UserAccount represents the UserAccount API method handler set.
type UserAccount struct {
	MasterDB *sqlx.DB

	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

// Find godoc
// TODO: Need to implement unittests on user_accounts/find endpoint. There are none.
// @Summary List user accounts
// @Description Find returns the existing user accounts in the system.
// @Tags user_account
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param where				query string 	false	"Filter string, example: account_id = 'c4653bf9-5978-48b7-89c5-95704aebb7e2'"
// @Param order				query string   	false 	"Order columns separated by comma, example: created_at desc"
// @Param limit				query integer  	false 	"Limit, example: 10"
// @Param offset			query integer  	false 	"Offset, example: 20"
// @Param included-archived query boolean 	false 	"Included Archived, example: false"
// @Success 200 {array} user_account.UserAccountResponse
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /user_accounts [get]
func (u *UserAccount) Find(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req user_account.UserAccountFindRequest

	// Handle where query value if set.
	if v := r.URL.Query().Get("where"); v != "" {
		where, args, err := web.ExtractWhereArgs(v)
		if err != nil {
			return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusBadRequest))
		}
		req.Where = &where
		req.Args = args
	}

	// Handle order query value if set.
	if v := r.URL.Query().Get("order"); v != "" {
		for _, o := range strings.Split(v, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				req.Order = append(req.Order, o)
			}
		}
	}

	// Handle limit query value if set.
	if v := r.URL.Query().Get("limit"); v != "" {
		l, err := strconv.Atoi(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as int for limit param", v)
			return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusBadRequest))
		}
		ul := uint(l)
		req.Limit = &ul
	}

	// Handle offset query value if set.
	if v := r.URL.Query().Get("offset"); v != "" {
		l, err := strconv.Atoi(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as int for offset param", v)
			return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusBadRequest))
		}
		ul := uint(l)
		req.Limit = &ul
	}

	// Handle order query value if set.
	if v := r.URL.Query().Get("included-archived"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as boolean for included-archived param", v)
			return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusBadRequest))
		}
		req.IncludedArchived = b
	}

	//if err := web.Decode(r, &req); err != nil {
	//	if _, ok := errors.Cause(err).(*web.Error); !ok {
	//		err = web.NewRequestError(err, http.StatusBadRequest)
	//	}
	//	return  web.RespondJsonError(ctx, w, err)
	//}

	res, err := user_account.Find(ctx, claims, u.MasterDB, req)
	if err != nil {
		return err
	}

	var resp []*user_account.UserAccountResponse
	for _, m := range res {
		resp = append(resp, m.Response(ctx))
	}

	return web.RespondJson(ctx, w, resp, http.StatusOK)
}

// Read godoc
// @Summary Get user account by ID
// @Description Read returns the specified user account from the system.
// @Tags user_account
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param id path string true "UserAccount ID"
// @Success 200 {object} user_account.UserAccountResponse
// @Failure 400 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /user_accounts/{id} [get]
func (u *UserAccount) Read(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
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
			return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusBadRequest))
		}
		includeArchived = b
	}

	res, err := user_account.Read(ctx, claims, u.MasterDB, params["id"], includeArchived)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case user_account.ErrNotFound:
			return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusNotFound))
		default:
			return errors.Wrapf(err, "ID: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, res.Response(ctx), http.StatusOK)
}

// Create godoc
// @Summary Create new user account.
// @Description Create inserts a new user account into the system.
// @Tags user_account
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body user_account.UserAccountCreateRequest true "User Account details"
// @Success 201 {object} user_account.UserAccountResponse
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /user_accounts [post]
func (u *UserAccount) Create(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req user_account.UserAccountCreateRequest
	if err := web.Decode(r, &req); err != nil {
		if _, ok := errors.Cause(err).(*web.Error); !ok {
			err = web.NewRequestError(err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	res, err := user_account.Create(ctx, claims, u.MasterDB, req, v.Now)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case user_account.ErrForbidden:
			return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusBadRequest))
			}

			return errors.Wrapf(err, "User Account: %+v", &req)
		}
	}

	return web.RespondJson(ctx, w, res.Response(ctx), http.StatusCreated)
}

// Read godoc
// @Summary Update user account by user ID and account ID
// @Description Update updates the specified user account in the system.
// @Tags user
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body user_account.UserAccountUpdateRequest true "Update fields"
// @Success 204
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /user_accounts [patch]
func (u *UserAccount) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req user_account.UserAccountUpdateRequest
	if err := web.Decode(r, &req); err != nil {
		if _, ok := errors.Cause(err).(*web.Error); !ok {
			err = web.NewRequestError(err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	err := user_account.Update(ctx, claims, u.MasterDB, req, v.Now)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case user_account.ErrForbidden:
			return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusBadRequest))
			}

			return errors.Wrapf(err, "UserID: %s AccountID: %s  User Account: %+v", req.UserID, req.AccountID, &req)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Read godoc
// @Summary Archive user account by user ID and account ID
// @Description Archive soft-deletes the specified user account from the system.
// @Tags user
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body user_account.UserAccountArchiveRequest true "Update fields"
// @Success 204
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /user_accounts/archive [patch]
func (u *UserAccount) Archive(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req user_account.UserAccountArchiveRequest
	if err := web.Decode(r, &req); err != nil {
		if _, ok := errors.Cause(err).(*web.Error); !ok {
			err = web.NewRequestError(err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	err := user_account.Archive(ctx, claims, u.MasterDB, req, v.Now)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case user_account.ErrForbidden:
			return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusBadRequest))
			}

			return errors.Wrapf(err, "UserID: %s AccountID: %s  User Account: %+v", req.UserID, req.AccountID, &req)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Delete godoc
// @Summary Delete user account by user ID and account ID
// @Description Delete removes the specified user account from the system.
// @Tags user
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param id path string true "UserAccount ID"
// @Success 204
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /user_accounts [delete]
func (u *UserAccount) Delete(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req user_account.UserAccountDeleteRequest
	if err := web.Decode(r, &req); err != nil {
		if _, ok := errors.Cause(err).(*web.Error); !ok {
			err = web.NewRequestError(err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	err := user_account.Delete(ctx, claims, u.MasterDB, req)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case user_account.ErrForbidden:
			return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusBadRequest))
			}

			return errors.Wrapf(err, "UserID: %s, AccountID: %s", req.UserID, req.AccountID)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}
