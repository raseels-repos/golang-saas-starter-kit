package handlers

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user_account"
	"net/http"
	"strconv"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// sessionTtl defines the auth token expiration.
var sessionTtl = time.Hour * 24

// User represents the User API method handler set.
type User struct {
	MasterDB       *sqlx.DB
	TokenGenerator user.TokenGenerator

	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

// Find godoc
// @Summary List users
// @Description Find returns the existing users in the system.
// @Tags user
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param where				query string 	false	"Filter string, example: name = 'Company Name' and email = 'gabi.may@geeksinthewoods.com'"
// @Param order				query string   	false 	"Order columns separated by comma, example: created_at desc"
// @Param limit				query integer  	false 	"Limit, example: 10"
// @Param offset			query integer  	false 	"Offset, example: 20"
// @Param included-archived query boolean 	false 	"Included Archived, example: false"
// @Success 200 {array} user.UserResponse
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /users [get]
func (u *User) Find(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req user.UserFindRequest

	// Handle where query value if set.
	if v := r.URL.Query().Get("where"); v != "" {
		where, args, err := web.ExtractWhereArgs(v)
		if err != nil {
			return web.NewRequestError(err, http.StatusBadRequest)
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
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		ul := uint(l)
		req.Limit = &ul
	}

	// Handle offset query value if set.
	if v := r.URL.Query().Get("offset"); v != "" {
		l, err := strconv.Atoi(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as int for offset param", v)
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		ul := uint(l)
		req.Limit = &ul
	}

	// Handle order query value if set.
	if v := r.URL.Query().Get("included-archived"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as boolean for included-archived param", v)
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		req.IncludedArchived = b
	}

	if err := web.Decode(r, &req); err != nil {
		err = errors.WithStack(err)

		_, ok := err.(validator.ValidationErrors)
		if ok {
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		return err
	}

	res, err := user.Find(ctx, claims, u.MasterDB, req)
	if err != nil {
		return err
	}

	var resp []*user.UserResponse
	for _, m := range res {
		resp = append(resp, m.Response(ctx))
	}

	return web.RespondJson(ctx, w, resp, http.StatusOK)
}

// Read godoc
// @Summary Get user by ID
// @Description Read returns the specified user from the system.
// @Tags user
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param id path string true "User ID"
// @Success 200 {object} user.UserResponse
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
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
			_, ok := err.(validator.ValidationErrors)
			if ok {
				return web.NewRequestError(err, http.StatusBadRequest)
			}

			return errors.Wrapf(err, "ID: %s", params["id"])
		}
	}

	return web.RespondJson(ctx, w, res.Response(ctx), http.StatusOK)
}

// Create godoc
// @Summary Create new user.
// @Description Create inserts a new user into the system.
// @Tags user
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body user.UserCreateRequest true "User details"
// @Success 200 {object} user.UserResponse
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /users [post]
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
		err = errors.WithStack(err)

		_, ok := err.(validator.ValidationErrors)
		if ok {
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		return err
	}

	res, err := user.Create(ctx, claims, u.MasterDB, req, v.Now)
	if err != nil {
		switch err {
		case user.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			_, ok := err.(validator.ValidationErrors)
			if ok {
				return web.NewRequestError(err, http.StatusBadRequest)
			}

			return errors.Wrapf(err, "User: %+v", &req)
		}
	}

	if claims.Audience != "" {
		uaReq := user_account.UserAccountCreateRequest{
			UserID:    resp.User.ID,
			AccountID: resp.Account.ID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole_Admin},
			//Status:  Use default value
		}
		_, err = user_account.Create(ctx, claims, u.MasterDB, uaReq, v.Now)
		if err != nil {
			switch err {
			case user.ErrForbidden:
				return web.NewRequestError(err, http.StatusForbidden)
			default:
				_, ok := err.(validator.ValidationErrors)
				if ok {
					return web.NewRequestError(err, http.StatusBadRequest)
				}

				return errors.Wrapf(err, "User account: %+v", &req)
			}
		}
	}

	return web.RespondJson(ctx, w, res.Response(ctx), http.StatusCreated)
}

// Read godoc
// @Summary Update user by ID
// @Description Update updates the specified user in the system.
// @Tags user
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body user.UserUpdateRequest true "Update fields"
// @Success 201
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /users [patch]
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
		err = errors.WithStack(err)

		_, ok := err.(validator.ValidationErrors)
		if ok {
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		return err
	}

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
			_, ok := err.(validator.ValidationErrors)
			if ok {
				return web.NewRequestError(err, http.StatusBadRequest)
			}

			return errors.Wrapf(err, "Id: %s  User: %+v", req.ID, &req)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Read godoc
// @Summary Update user password by ID
// @Description Update updates the password for a specified user in the system.
// @Tags user
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body user.UserUpdatePasswordRequest true "Update fields"
// @Success 201
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /users/password [patch]
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
		err = errors.WithStack(err)

		_, ok := err.(validator.ValidationErrors)
		if ok {
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		return err
	}

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
			_, ok := err.(validator.ValidationErrors)
			if ok {
				return web.NewRequestError(err, http.StatusBadRequest)
			}

			return errors.Wrapf(err, "Id: %s  User: %+v", req.ID, &req)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Read godoc
// @Summary Archive user by ID
// @Description Archive soft-deletes the specified user from the system.
// @Tags user
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body user.UserArchiveRequest true "Update fields"
// @Success 201
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /users/archive [patch]
func (u *User) Archive(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req user.UserArchiveRequest
	if err := web.Decode(r, &req); err != nil {
		err = errors.WithStack(err)

		_, ok := err.(validator.ValidationErrors)
		if ok {
			return web.NewRequestError(err, http.StatusBadRequest)
		}
		return err
	}

	err := user.Archive(ctx, claims, u.MasterDB, req, v.Now)
	if err != nil {
		switch err {
		case user.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		case user.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case user.ErrForbidden:
			return web.NewRequestError(err, http.StatusForbidden)
		default:
			_, ok := err.(validator.ValidationErrors)
			if ok {
				return web.NewRequestError(err, http.StatusBadRequest)
			}

			return errors.Wrapf(err, "Id: %s", req.ID)
		}
	}

	return web.RespondJson(ctx, w, nil, http.StatusNoContent)
}

// Delete godoc
// @Summary Delete user by ID
// @Description Delete removes the specified user from the system.
// @Tags user
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param id path string true "User ID"
// @Success 201
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /users/{id} [delete]
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

// SwitchAccount godoc
// @Summary Switch account.
// @Description SwitchAccount updates the auth claims to a new account.
// @Tags user
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param account_id path int true "Account ID"
// @Success 201
// @Failure 400 {object} web.ErrorResponse
// @Failure 403 {object} web.ErrorResponse
// @Failure 404 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /users/switch-account/{account_id} [patch]
func (u *User) SwitchAccount(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	tkn, err := user.SwitchAccount(ctx, u.MasterDB, u.TokenGenerator, claims, params["account_id"], sessionTtl, v.Now)
	if err != nil {
		switch err {
		case user.ErrAuthenticationFailure:
			return web.NewRequestError(err, http.StatusUnauthorized)
		default:
			_, ok := err.(validator.ValidationErrors)
			if ok {
				return web.NewRequestError(err, http.StatusBadRequest)
			}

			return errors.Wrap(err, "switch account")
		}
	}

	return web.RespondJson(ctx, w, tkn, http.StatusNoContent)
}

// Token godoc
// @Summary Token handles a request to authenticate a user.
// @Description Token generates an oauth2 accessToken using Basic Auth with a user's email and password.
// @Tags user
// @Accept  json
// @Produce  json
// @Security BasicAuth
// @Param scope query string false "Scope" Enums(user, admin)
// @Success 200 {object} user.Token
// @Header 200 {string} Token "qwerty"
// @Failure 400 {object} web.Error
// @Failure 403 {object} web.Error
// @Failure 404 {object} web.Error
// @Router /oauth/token [post]
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

	// Optional to include scope.
	scope := r.URL.Query().Get("scope")

	tkn, err := user.Authenticate(ctx, u.MasterDB, u.TokenGenerator, email, pass, sessionTtl, v.Now, scope)
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
