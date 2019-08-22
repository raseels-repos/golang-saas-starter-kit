package handlers

import (
	"context"
	"net/http"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/account/account_preference"
	"geeks-accelerator/oss/saas-starter-kit/internal/geonames"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_auth"

	"github.com/gorilla/schema"
	"github.com/pkg/errors"
)

// Account represents the Account API method handler set.
type Account struct {
	AccountRepo     *account.Repository
	AccountPrefRepo *account_preference.Repository
	AuthRepo        *user_auth.Repository
	GeoRepo         *geonames.Repository
	Authenticator   *auth.Authenticator
	Renderer        web.Renderer
}

// View handles displaying the current account profile.
func (h *Account) View(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	data := make(map[string]interface{})
	f := func() error {

		claims, err := auth.ClaimsFromContext(ctx)
		if err != nil {
			return err
		}

		acc, err := h.AccountRepo.ReadByID(ctx, claims, claims.Audience)
		if err != nil {
			return err
		}

		data["account"] = acc.Response(ctx)

		return nil
	}

	if err := f(); err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "account-view.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

type AccountUpdateRequest struct {
	account.AccountUpdateRequest
	PreferenceDatetimeFormat string
	PreferenceDateFormat     string
	PreferenceTimeFormat     string
}

// Update handles allowing the current user to update their account.
func (h *Account) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	//
	req := new(AccountUpdateRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {

		claims, err := auth.ClaimsFromContext(ctx)
		if err != nil {
			return false, err
		}

		prefs, err := h.AccountPrefRepo.FindByAccountID(ctx, claims, account_preference.AccountPreferenceFindByAccountIDRequest{
			AccountID: claims.Audience,
		})
		if err != nil {
			return false, err
		}

		var (
			preferenceDatetimeFormat string
			preferenceDateFormat     string
			preferenceTimeFormat     string
		)

		for _, pref := range prefs {
			switch pref.Name {
			case account_preference.AccountPreference_Datetime_Format:
				preferenceDatetimeFormat = pref.Value
			case account_preference.AccountPreference_Date_Format:
				preferenceDateFormat = pref.Value
			case account_preference.AccountPreference_Time_Format:
				preferenceTimeFormat = pref.Value
			}
		}

		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return false, err
			}

			decoder := schema.NewDecoder()
			decoder.IgnoreUnknownKeys(true)

			if err := decoder.Decode(req, r.PostForm); err != nil {
				return false, err
			}
			req.ID = claims.Audience

			err = h.AccountRepo.Update(ctx, claims, req.AccountUpdateRequest, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}
			}

			var updateClaims bool
			if req.Timezone != nil && claims.Preferences.Timezone != *req.Timezone {
				claims.Preferences.Timezone = *req.Timezone
				updateClaims = true
			}

			if preferenceDatetimeFormat != req.PreferenceDatetimeFormat {
				err = h.AccountPrefRepo.Set(ctx, claims, account_preference.AccountPreferenceSetRequest{
					AccountID: claims.Audience,
					Name:      account_preference.AccountPreference_Datetime_Format,
					Value:     req.PreferenceDatetimeFormat,
				}, ctxValues.Now)
				if err != nil {
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}

				if claims.Preferences.DatetimeFormat != req.PreferenceDatetimeFormat {
					claims.Preferences.DatetimeFormat = req.PreferenceDatetimeFormat
					updateClaims = true
				}
			}

			if preferenceDateFormat != req.PreferenceDateFormat {
				err = h.AccountPrefRepo.Set(ctx, claims, account_preference.AccountPreferenceSetRequest{
					AccountID: claims.Audience,
					Name:      account_preference.AccountPreference_Date_Format,
					Value:     req.PreferenceDateFormat,
				}, ctxValues.Now)
				if err != nil {
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}

				if claims.Preferences.DateFormat != req.PreferenceDateFormat {
					claims.Preferences.DateFormat = req.PreferenceDateFormat
					updateClaims = true
				}
			}

			if preferenceTimeFormat != req.PreferenceTimeFormat {
				err = h.AccountPrefRepo.Set(ctx, claims, account_preference.AccountPreferenceSetRequest{
					AccountID: claims.Audience,
					Name:      account_preference.AccountPreference_Time_Format,
					Value:     req.PreferenceTimeFormat,
				}, ctxValues.Now)
				if err != nil {
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}

				if claims.Preferences.TimeFormat != req.PreferenceTimeFormat {
					claims.Preferences.TimeFormat = req.PreferenceTimeFormat
					updateClaims = true
				}
			}

			// Update the access token to include the updated claims.
			if updateClaims {
				ctx, err = updateContextClaims(ctx, h.Authenticator, claims)
				if err != nil {
					return false, err
				}
			}

			// Display a success message to the user.
			webcontext.SessionFlashSuccess(ctx,
				"Account Updated",
				"Account profile successfully updated.")

			return true, web.Redirect(ctx, w, r, "/account", http.StatusFound)
		}

		acc, err := h.AccountRepo.ReadByID(ctx, claims, claims.Audience)
		if err != nil {
			return false, err
		}

		if preferenceDatetimeFormat == "" {
			preferenceDatetimeFormat = account_preference.AccountPreference_Datetime_Format_Default
		}
		if preferenceDateFormat == "" {
			preferenceDateFormat = account_preference.AccountPreference_Date_Format_Default
		}
		if preferenceTimeFormat == "" {
			preferenceTimeFormat = account_preference.AccountPreference_Time_Format_Default
		}

		if req.ID == "" {
			req.Name = &acc.Name
			req.Address1 = &acc.Address1
			req.Address2 = &acc.Address2
			req.City = &acc.City
			req.Region = &acc.Region
			req.Country = &acc.Country
			req.Zipcode = &acc.Zipcode
			req.Timezone = &acc.Timezone
			req.PreferenceDatetimeFormat = preferenceDatetimeFormat
			req.PreferenceDateFormat = preferenceDateFormat
			req.PreferenceTimeFormat = preferenceTimeFormat
		}

		data["account"] = acc.Response(ctx)

		data["timezones"], err = h.GeoRepo.ListTimezones(ctx)
		if err != nil {
			return false, err
		}

		data["geonameCountries"] = geonames.ValidGeonameCountries(ctx)

		data["countries"], err = h.GeoRepo.FindCountries(ctx, "name", "")
		if err != nil {
			return false, err
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	data["form"] = req

	data["exampleDisplayTime"] = web.NewTimeResponse(ctx, time.Now().UTC())

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(account.AccountUpdateRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "account-update.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
