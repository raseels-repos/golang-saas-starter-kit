package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_auth"

	"github.com/gorilla/schema"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// sessionTtl defines the auth token expiration.
var sessionTtl = time.Hour * 24

// User represents the User API method handler set.
type Users struct {
	AuthRepo *user_auth.Repository
	UserRepo *user.Repository

	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

// Find godoc
// TODO: Need to implement unittests on users/find endpoint. There are none.
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
// @Param include-archived query boolean 	false 	"Included Archived, example: false"
// @Success 200 {array} user.UserResponse
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /users [get]
func (h *Users) Find(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, ok := ctx.Value(auth.Key).(auth.Claims)
	if !ok {
		return errors.New("claims missing from context")
	}

	var req user.UserFindRequest

	// Handle where query value if set.
	if v := r.URL.Query().Get("where"); v != "" {
		where, args, err := web.ExtractWhereArgs(v)
		if err != nil {
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
		}
		req.Where = where
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
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
		}
		ul := uint(l)
		req.Limit = &ul
	}

	// Handle offset query value if set.
	if v := r.URL.Query().Get("offset"); v != "" {
		l, err := strconv.Atoi(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as int for offset param", v)
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
		}
		ul := uint(l)
		req.Limit = &ul
	}

	// Handle include-archived query value if set.
	if v := r.URL.Query().Get("include-archived"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			err = errors.WithMessagef(err, "unable to parse %s as boolean for include-archived param", v)
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
		}
		req.IncludeArchived = b
	}

	//if err := web.Decode(r, &req); err != nil {
	//	if _, ok := errors.Cause(err).(*web.Error); !ok {
	//		err = weberror.NewError(ctx, err, http.StatusBadRequest)
	//	}
	//	return  web.RespondJsonError(ctx, w, err)
	//}

	res, err := h.UserRepo.Find(ctx, claims, req)
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
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 404 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /users/{id} [get]
func (h *Users) Read(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
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

	res, err := h.UserRepo.Read(ctx, claims, user.UserReadRequest{
		ID:              params["id"],
		IncludeArchived: includeArchived,
	})
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case user.ErrNotFound:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusNotFound))
		default:
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
// @Success 201 {object} user.UserResponse
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 403 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /users [post]
func (h *Users) Create(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	var req user.UserCreateRequest
	if err := web.Decode(ctx, r, &req); err != nil {
		if _, ok := errors.Cause(err).(*weberror.Error); !ok {
			err = weberror.NewError(ctx, err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	usr, err := h.UserRepo.Create(ctx, claims, req, v.Now)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case user.ErrForbidden:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
			}

			return errors.Wrapf(err, "User: %+v", &req)
		}
	}

	return web.RespondJson(ctx, w, usr.Response(ctx), http.StatusCreated)
}

// Read godoc
// @Summary Update user by ID
// @Description Update updates the specified user in the system.
// @Tags user
// @Accept  json
// @Produce  json
// @Security OAuth2Password
// @Param data body user.UserUpdateRequest true "Update fields"
// @Success 204
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 403 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /users [patch]
func (h *Users) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	var req user.UserUpdateRequest
	if err := web.Decode(ctx, r, &req); err != nil {
		if _, ok := errors.Cause(err).(*weberror.Error); !ok {
			err = weberror.NewError(ctx, err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	err = h.UserRepo.Update(ctx, claims, req, v.Now)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case user.ErrForbidden:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
			}

			return errors.Wrapf(err, "Id: %s User: %+v", req.ID, &req)
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
// @Success 204
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 403 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /users/password [patch]
func (h *Users) UpdatePassword(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	var req user.UserUpdatePasswordRequest
	if err := web.Decode(ctx, r, &req); err != nil {
		if _, ok := errors.Cause(err).(*weberror.Error); !ok {
			err = weberror.NewError(ctx, err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	err = h.UserRepo.UpdatePassword(ctx, claims, req, v.Now)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case user.ErrNotFound:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusNotFound))
		case user.ErrForbidden:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
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
// @Success 204
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 403 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /users/archive [patch]
func (h *Users) Archive(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	var req user.UserArchiveRequest
	if err := web.Decode(ctx, r, &req); err != nil {
		if _, ok := errors.Cause(err).(*weberror.Error); !ok {
			err = weberror.NewError(ctx, err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	err = h.UserRepo.Archive(ctx, claims, req, v.Now)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case user.ErrForbidden:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
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
// @Success 204
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 403 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /users/{id} [delete]
func (h *Users) Delete(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	err = h.UserRepo.Delete(ctx, claims,
		user.UserDeleteRequest{ID: params["id"]})
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case user.ErrForbidden:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
			}

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
// @Success 200
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 401 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /users/switch-account/{account_id} [patch]
func (h *Users) SwitchAccount(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	tkn, err := h.AuthRepo.SwitchAccount(ctx, claims, user_auth.SwitchAccountRequest{
		AccountID: params["account_id"],
	}, sessionTtl, v.Now)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case user_auth.ErrAuthenticationFailure:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusUnauthorized))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
			}

			return errors.Wrap(err, "switch account")
		}
	}

	return web.RespondJson(ctx, w, tkn, http.StatusOK)
}

// Token godoc
// @Summary Token handles a request to authenticate a user.
// @Description Token generates an oauth2 accessToken using Basic Auth with a user's email and password.
// @Tags user
// @Accept  x-www-form-urlencoded
// @Produce  json
// @Param username formData string true "Email"
// @Param password formData string true "Password"
// @Param account_id formData string false "Account ID"
// @Param scope formData string false "Scope" Enums(user, admin)
// @Success 200
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 401 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /oauth/token [post]
func (h *Users) Token(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	var authReq user_auth.AuthenticateRequest
	var scopes []string

	email, pass, ok := r.BasicAuth()
	if !ok || email == "" {
		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return err
			}

			decoder := schema.NewDecoder()
			decoder.IgnoreUnknownKeys(true)

			var req user_auth.OAuth2PasswordRequest
			if err := decoder.Decode(&req, r.PostForm); err != nil {
				if _, ok := errors.Cause(err).(*weberror.Error); !ok {
					err = weberror.NewError(ctx, err, http.StatusBadRequest)
				}
				return web.RespondJsonError(ctx, w, err)
			}

			// Validate the request.
			err = webcontext.Validator().StructCtx(ctx, req)
			if err != nil {
				return err
			}

			authReq.Email = req.Username
			authReq.Password = req.Password
			scopes = req.Scope
		} else {
			err := errors.New("must provide email and password in Basic auth")
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusUnauthorized))
		}
	} else {
		authReq.Email = email
		authReq.Password = pass
	}

	if qv := r.URL.Query().Get("account_id"); qv != "" {
		authReq.AccountID = qv
	}

	// Optional to include scope.
	if qv := r.URL.Query().Get("scope"); qv != "" {
		scopes = strings.Split(qv, ",")
	}

	tkn, err := h.AuthRepo.Authenticate(ctx, authReq, sessionTtl, v.Now, scopes...)
	if err != nil {
		cause := errors.Cause(err)
		switch cause {
		case user_auth.ErrAuthenticationFailure:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusUnauthorized))
		default:
			_, ok := cause.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
			}

			return errors.Wrap(err, "authenticating")
		}
	}

	return web.RespondJson(ctx, w, tkn, http.StatusOK)
}
