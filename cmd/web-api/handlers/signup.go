package handlers

import (
	"context"
	"net/http"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/signup"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// Signup represents the Signup API method handler set.
type Signup struct {
	MasterDB *sqlx.DB

	// ADD OTHER STATE LIKE THE LOGGER AND CONFIG HERE.
}

// Signup godoc
// @Summary Signup handles new account creation.
// @Description Signup creates a new account and user in the system.
// @Tags signup
// @Accept  json
// @Produce  json
// @Param data body signup.SignupRequest true "Signup details"
// @Success 201 {object} signup.SignupResponse
// @Failure 400 {object} web.ErrorResponse
// @Failure 500 {object} web.ErrorResponse
// @Router /signup [post]
func (c *Signup) Signup(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
	v, ok := ctx.Value(web.KeyValues).(*web.Values)
	if !ok {
		return web.NewShutdownError("web value missing from context")
	}

	// Claims are optional as authentication is not required ATM for this method.
	claims, _ := ctx.Value(auth.Key).(auth.Claims)

	var req signup.SignupRequest
	if err := web.Decode(r, &req); err != nil {
		if _, ok := errors.Cause(err).(*web.Error); !ok {
			err = web.NewRequestError(err, http.StatusBadRequest)
		}
		return web.RespondJsonError(ctx, w, err)
	}

	res, err := signup.Signup(ctx, claims, c.MasterDB, req, v.Now)
	if err != nil {
		switch errors.Cause(err) {
		case account.ErrForbidden:
			return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusForbidden))
		default:
			_, ok := err.(validator.ValidationErrors)
			if ok {
				return web.RespondJsonError(ctx, w, web.NewRequestError(err, http.StatusBadRequest))
			}

			return errors.Wrapf(err, "Signup: %+v", &req)
		}
	}

	return web.RespondJson(ctx, w, res.Response(ctx), http.StatusCreated)
}
