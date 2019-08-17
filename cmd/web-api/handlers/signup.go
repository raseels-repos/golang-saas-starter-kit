package handlers

import (
	"context"
	"net/http"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"geeks-accelerator/oss/saas-starter-kit/internal/signup"

	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// Signup represents the Signup API method handler set.
type Signup struct {
	Repository SignupRepository

	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

type SignupRepository interface {
	Signup(ctx context.Context, claims auth.Claims, req signup.SignupRequest, now time.Time) (*signup.SignupResult, error)
}

// Signup godoc
// @Summary Signup handles new account creation.
// @Description Signup creates a new account and user in the system.
// @Tags signup
// @Accept  json
// @Produce  json
// @Param data body signup.SignupRequest true "Signup details"
// @Success 201 {object} signup.SignupResponse
// @Failure 400 {object} weberror.ErrorResponse
// @Failure 500 {object} weberror.ErrorResponse
// @Router /signup [post]
func (h *Signup) Signup(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	// Claims are optional as authentication is not required ATM for this method.
	claims, _ := auth.ClaimsFromContext(ctx)

	var req signup.SignupRequest
	if err := web.Decode(ctx, r, &req); err != nil {
		if _, ok := errors.Cause(err).(*weberror.Error); !ok {
			err = weberror.NewError(ctx, err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	res, err := h.Repository.Signup(ctx, claims, req, v.Now)
	if err != nil {
		switch errors.Cause(err) {
		case account.ErrForbidden:
			return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
		default:
			_, ok := err.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, weberror.NewError(ctx, err, http.StatusBadRequest))
			}

			return errors.Wrapf(err, "Signup: %+v", &req)
		}
	}

	return web.RespondJson(ctx, w, res.Response(ctx), http.StatusCreated)
}
