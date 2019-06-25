package handlers

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/signup"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"net/http"
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
// @Success 200 {object} signup.SignupResponse
// @Header 200 {string} Token "qwerty"
// @Failure 400 {object} web.Error
// @Failure 403 {object} web.Error
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
		return errors.Wrap(err, "")
	}

	res, err := signup.Signup(ctx, claims, c.MasterDB, req, v.Now)
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
