package handlers

import (
	"context"
	"net/http"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/geonames"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"geeks-accelerator/oss/saas-starter-kit/internal/signup"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_auth"
	"github.com/gorilla/schema"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// Signup represents the Signup API method handler set.
type Signup struct {
	SignupRepo *signup.Repository
	AuthRepo   *user_auth.Repository
	MasterDB   *sqlx.DB
	Renderer   web.Renderer
}

// Step1 handles collecting the first detailed needed to create a new account.
func (h *Signup) Step1(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	//
	req := new(signup.SignupRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {
		claims, _ := auth.ClaimsFromContext(ctx)

		if r.Method == http.MethodPost {

			err := r.ParseForm()
			if err != nil {
				return false, err
			}

			decoder := schema.NewDecoder()
			if err := decoder.Decode(req, r.PostForm); err != nil {
				return false, err
			}

			// Execute the account / user signup.
			_, err = h.SignupRepo.Signup(ctx, claims, *req, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				case account.ErrForbidden:
					return false, web.RespondError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}
			}

			// Authenticated the new user.
			token, err := h.AuthRepo.Authenticate(ctx, user_auth.AuthenticateRequest{
				Email:    req.User.Email,
				Password: req.User.Password,
			}, time.Hour, ctxValues.Now)
			if err != nil {
				return false, err
			}

			// Add the token to the users session.
			err = handleSessionToken(ctx, w, r, token)
			if err != nil {
				return false, err
			}

			// Display a welcome message to the user.
			webcontext.SessionFlashSuccess(ctx,
				"Thank you for Joining",
				"You workflow will be a breeze starting today.")

			// Redirect the user to the dashboard.
			return true, web.Redirect(ctx, w, r, "/", http.StatusFound)
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		err = webcontext.ContextSession(ctx).Save(r, w)
		if err != nil {
			return err
		}
		return nil
	}

	data["geonameCountries"] = geonames.ValidGeonameCountries(ctx)

	data["countries"], err = geonames.FindCountries(ctx, h.MasterDB, "name", "")
	if err != nil {
		return err
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(signup.SignupRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "signup-step1.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
