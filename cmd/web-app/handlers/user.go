package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/geonames"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_auth"
	"github.com/gorilla/schema"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// User represents the User API method handler set.
type User struct {
	UserRepo        *user.Repository
	AuthRepo        *user_auth.Repository
	UserAccountRepo *user_account.Repository
	AccountRepo     *account.Repository
	MasterDB        *sqlx.DB
	Renderer        web.Renderer
}

func urlUserVirtualLogin(userID string) string {
	return fmt.Sprintf("/user/virtual-login/%s", userID)
}

// UserLoginRequest extends the AuthenicateRequest with the RememberMe flag.
type UserLoginRequest struct {
	user_auth.AuthenticateRequest
	RememberMe bool
}

// Login handles authenticating a user into the system.
func (h *User) Login(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	//
	req := new(UserLoginRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {

		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return false, err
			}

			decoder := schema.NewDecoder()
			if err := decoder.Decode(req, r.PostForm); err != nil {
				return false, err
			}

			sessionTTL := time.Hour
			if req.RememberMe {
				sessionTTL = time.Hour * 36
			}

			// Authenticated the user.
			token, err := h.AuthRepo.Authenticate(ctx, user_auth.AuthenticateRequest{
				Email:    req.Email,
				Password: req.Password,
			}, sessionTTL, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				case user.ErrForbidden:
					return false, web.RespondError(ctx, w, weberror.NewError(ctx, err, http.StatusForbidden))
				case user_auth.ErrAuthenticationFailure:
					data["error"] = weberror.NewErrorMessage(ctx, err, http.StatusUnauthorized, "Authentication failure. Try again.")
					return false, nil
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}
			}

			// Add the token to the users session.
			err = handleSessionToken(ctx, w, r, token)
			if err != nil {
				return false, err
			}

			redirectUri := "/"
			if qv := r.URL.Query().Get("redirect"); qv != "" {
				redirectUri, err = url.QueryUnescape(qv)
				if err != nil {
					return false, err
				}
			}

			// Redirect the user to the dashboard.
			return true, web.Redirect(ctx, w, r, redirectUri, http.StatusFound)
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

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(UserLoginRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-login.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Logout handles removing authentication for the user.
func (h *User) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	sess := webcontext.ContextSession(ctx)

	// Set the access token to empty to logout the user.
	sess = webcontext.SessionDestroy(sess)

	if err := sess.Save(r, w); err != nil {
		return err
	}

	// Redirect the user to the root page.
	return web.Redirect(ctx, w, r, "/", http.StatusFound)
}

// ResetPassword allows a user to perform forgot password.
func (h *User) ResetPassword(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	//
	req := new(user.UserResetPasswordRequest)
	data := make(map[string]interface{})
	f := func() error {

		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return err
			}

			decoder := schema.NewDecoder()
			if err := decoder.Decode(req, r.PostForm); err != nil {
				return err
			}

			_, err = h.UserRepo.ResetPassword(ctx, *req, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return nil
					} else {
						return err
					}
				}
			}

			// Display a success message to the user to check their email.
			webcontext.SessionFlashSuccess(ctx,
				"Check your email",
				fmt.Sprintf("An email was sent to '%s'. Click on the link in the email to finish resetting your password.", req.Email))

		}

		return nil
	}

	if err := f(); err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user.UserResetPasswordRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-reset-password.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// ResetConfirm handles changing a users password after they have clicked on the link emailed.
func (h *User) ResetConfirm(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	resetHash := params["hash"]

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	//
	req := new(user.UserResetConfirmRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {

		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				return false, err
			}

			decoder := schema.NewDecoder()
			if err := decoder.Decode(req, r.PostForm); err != nil {
				return false, err
			}

			// Append the query param value to the request.
			req.ResetHash = resetHash

			u, err := h.UserRepo.ResetConfirm(ctx, *req, ctxValues.Now)
			if err != nil {
				switch errors.Cause(err) {
				case user.ErrResetExpired:
					webcontext.SessionFlashError(ctx,
						"Reset Expired",
						"The reset has expired.")
					return false, nil
				default:
					if verr, ok := weberror.NewValidationError(ctx, err); ok {
						data["validationErrors"] = verr.(*weberror.Error)
						return false, nil
					} else {
						return false, err
					}
				}
			}

			// Authenticated the user. Probably should use the default session TTL from UserLogin.
			token, err := h.AuthRepo.Authenticate(ctx, user_auth.AuthenticateRequest{
				Email:    u.Email,
				Password: req.Password,
			}, time.Hour, ctxValues.Now)
			if err != nil {
				if verr, ok := weberror.NewValidationError(ctx, err); ok {
					data["validationErrors"] = verr.(*weberror.Error)
					return false, nil
				} else {
					return false, err
				}
			}

			// Add the token to the users session.
			err = handleSessionToken(ctx, w, r, token)
			if err != nil {
				return false, err
			}

			// Redirect the user to the dashboard.
			return true, web.Redirect(ctx, w, r, "/", http.StatusFound)
		}

		_, err = h.UserRepo.ParseResetHash(ctx, resetHash, ctxValues.Now)
		if err != nil {
			switch errors.Cause(err) {
			case user.ErrResetExpired:
				webcontext.SessionFlashError(ctx,
					"Reset Expired",
					"The reset has expired.")
				return false, nil
			default:
				if verr, ok := weberror.NewValidationError(ctx, err); ok {
					data["validationErrors"] = verr.(*weberror.Error)
					return false, nil
				} else {
					return false, err
				}
			}
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

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user.UserResetConfirmRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-reset-confirm.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// View handles displaying the current user profile.
func (h *User) View(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	data := make(map[string]interface{})
	f := func() error {

		claims, err := auth.ClaimsFromContext(ctx)
		if err != nil {
			return err
		}

		usr, err := h.UserRepo.ReadByID(ctx, claims, claims.Subject)
		if err != nil {
			return err
		}

		data["user"] = usr.Response(ctx)

		usrAccs, err := h.UserAccountRepo.FindByUserID(ctx, claims, claims.Subject, false)
		if err != nil {
			return err
		}

		for _, usrAcc := range usrAccs {
			if usrAcc.AccountID == claims.Audience {
				data["userAccount"] = usrAcc.Response(ctx)
				break
			}
		}

		return nil
	}

	if err := f(); err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-view.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Update handles allowing the current user to update their profile.
func (h *User) Update(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	//
	req := new(user.UserUpdateRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {
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
			req.ID = claims.Subject

			err = h.UserRepo.Update(ctx, claims, *req, ctxValues.Now)
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

			if r.PostForm.Get("Password") != "" {
				pwdReq := new(user.UserUpdatePasswordRequest)

				if err := decoder.Decode(pwdReq, r.PostForm); err != nil {
					return false, err
				}
				pwdReq.ID = claims.Subject

				err = h.UserRepo.UpdatePassword(ctx, claims, *pwdReq, ctxValues.Now)
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
			}

			// Display a success message to the user.
			webcontext.SessionFlashSuccess(ctx,
				"Profile Updated",
				"User profile successfully updated.")

			return true, web.Redirect(ctx, w, r, "/user", http.StatusFound)
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	usr, err := h.UserRepo.ReadByID(ctx, claims, claims.Subject)
	if err != nil {
		return err
	}

	if req.ID == "" {
		req.FirstName = &usr.FirstName
		req.LastName = &usr.LastName
		req.Email = &usr.Email
		req.Timezone = usr.Timezone
	}

	data["user"] = usr.Response(ctx)

	data["timezones"], err = geonames.ListTimezones(ctx, h.MasterDB)
	if err != nil {
		return err
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user.UserUpdateRequest{})); ok {
		data["userValidationDefaults"] = verr.(*weberror.Error)
	}

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user.UserUpdatePasswordRequest{})); ok {
		data["passwordValidationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-update.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// Account handles displaying the Account for the current user.
func (h *User) Account(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

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

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-account.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// VirtualLogin handles switching the scope of the context to another user.
func (h *User) VirtualLogin(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	//
	req := new(user_auth.VirtualLoginRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {
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
		} else {
			if pv, ok := params["user_id"]; ok && pv != "" {
				req.UserID = pv
			}
		}

		if qv := r.URL.Query().Get("account_id"); qv != "" {
			req.AccountID = qv
		} else {
			req.AccountID = claims.Audience
		}

		if req.UserID != "" {
			sess := webcontext.ContextSession(ctx)
			var expires time.Duration
			if sess != nil && sess.Options != nil {
				expires = time.Second * time.Duration(sess.Options.MaxAge)
			} else {
				expires = time.Hour
			}

			// Perform the account switch.
			tkn, err := h.AuthRepo.VirtualLogin(ctx, claims, *req, expires, ctxValues.Now)
			if err != nil {
				if verr, ok := weberror.NewValidationError(ctx, err); ok {
					data["validationErrors"] = verr.(*weberror.Error)
					return false, nil
				} else {
					return false, err
				}
			}

			// Update the access token in the session.
			sess = webcontext.SessionUpdateAccessToken(sess, tkn.AccessToken)

			// Read the account for a flash message.
			usr, err := h.UserRepo.ReadByID(ctx, claims, tkn.UserID)
			if err != nil {
				return false, err
			}
			webcontext.SessionFlashSuccess(ctx,
				"User Switched",
				fmt.Sprintf("You are now virtually logged into user %s.",
					usr.Response(ctx).Name))

			// Redirect the user to the dashboard with the new credentials.
			return true, web.Redirect(ctx, w, r, "/", http.StatusFound)
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	usrAccs, err := h.UserAccountRepo.Find(ctx, claims, user_account.UserAccountFindRequest{
		Where: "account_id = ?",
		Args:  []interface{}{claims.Audience},
	})
	if err != nil {
		return err
	}

	var userIDs []interface{}
	var userPhs []string
	for _, usrAcc := range usrAccs {
		if usrAcc.UserID == claims.Subject {
			// Skip the current authenticated user.
			continue
		}
		userIDs = append(userIDs, usrAcc.UserID)
		userPhs = append(userPhs, "?")
	}

	if len(userIDs) == 0 {
		userIDs = append(userIDs, "")
		userPhs = append(userPhs, "?")
	}

	users, err := h.UserRepo.Find(ctx, claims, user.UserFindRequest{
		Where: fmt.Sprintf("id IN (%s)",
			strings.Join(userPhs, ", ")),
		Args: userIDs,
	})
	if err != nil {
		return err
	}
	data["users"] = users.Response(ctx)

	if req.AccountID == "" {
		req.AccountID = claims.Audience
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user_auth.VirtualLoginRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-virtual-login.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// VirtualLogout handles switching the scope back to the user who initiated the virtual login.
func (h *User) VirtualLogout(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	sess := webcontext.ContextSession(ctx)

	var expires time.Duration
	if sess != nil && sess.Options != nil {
		expires = time.Second * time.Duration(sess.Options.MaxAge)
	} else {
		expires = time.Hour
	}

	tkn, err := h.AuthRepo.VirtualLogout(ctx, claims, expires, ctxValues.Now)
	if err != nil {
		return err
	}

	// Update the access token in the session.
	sess = webcontext.SessionUpdateAccessToken(sess, tkn.AccessToken)

	// Display a success message to verify the user has switched contexts.
	if claims.Subject != tkn.UserID && claims.Audience != tkn.AccountID {
		usr, err := h.UserRepo.ReadByID(ctx, claims, tkn.UserID)
		if err != nil {
			return err
		}
		acc, err := h.AccountRepo.ReadByID(ctx, claims, tkn.AccountID)
		if err != nil {
			return err
		}
		webcontext.SessionFlashSuccess(ctx,
			"Context Switched",
			fmt.Sprintf("You are now virtually logged back into account %s user %s.",
				acc.Response(ctx).Name, usr.Response(ctx).Name))
	} else if claims.Audience != tkn.AccountID {
		acc, err := h.AccountRepo.ReadByID(ctx, claims, tkn.AccountID)
		if err != nil {
			return err
		}
		webcontext.SessionFlashSuccess(ctx,
			"Context Switched",
			fmt.Sprintf("You are now virtually logged back into account %s.",
				acc.Response(ctx).Name))
	} else {
		usr, err := h.UserRepo.ReadByID(ctx, claims, tkn.UserID)
		if err != nil {
			return err
		}
		webcontext.SessionFlashSuccess(ctx,
			"Context Switched",
			fmt.Sprintf("You are now virtually logged back into user %s.",
				usr.Response(ctx).Name))
	}

	// Write the session to the client.
	err = webcontext.ContextSession(ctx).Save(r, w)
	if err != nil {
		return err
	}

	// Redirect the user to the dashboard with the new credentials.
	return web.Redirect(ctx, w, r, "/", http.StatusFound)
}

// VirtualLogin handles switching the scope of the context to another user.
func (h *User) SwitchAccount(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	ctxValues, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	claims, err := auth.ClaimsFromContext(ctx)
	if err != nil {
		return err
	}

	//
	req := new(user_auth.SwitchAccountRequest)
	data := make(map[string]interface{})
	f := func() (bool, error) {

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
		} else {
			if pv, ok := params["account_id"]; ok && pv != "" {
				req.AccountID = pv
			} else if qv := r.URL.Query().Get("account_id"); qv != "" {
				req.AccountID = qv
			}
		}

		if req.AccountID != "" {
			sess := webcontext.ContextSession(ctx)
			var expires time.Duration
			if sess != nil && sess.Options != nil {
				expires = time.Second * time.Duration(sess.Options.MaxAge)
			} else {
				expires = time.Hour
			}

			// Perform the account switch.
			tkn, err := h.AuthRepo.SwitchAccount(ctx, claims, *req, expires, ctxValues.Now)
			if err != nil {
				if verr, ok := weberror.NewValidationError(ctx, err); ok {
					data["validationErrors"] = verr.(*weberror.Error)
					return false, nil
				} else {
					return false, err
				}
			}

			// Update the access token in the session.
			sess = webcontext.SessionUpdateAccessToken(sess, tkn.AccessToken)

			// Read the account for a flash message.
			acc, err := h.AccountRepo.ReadByID(ctx, claims, tkn.AccountID)
			if err != nil {
				return false, err
			}
			webcontext.SessionFlashSuccess(ctx,
				"Account Switched",
				fmt.Sprintf("You are now logged into account %s.",
					acc.Response(ctx).Name))

			// Redirect the user to the dashboard with the new credentials.
			return true, web.Redirect(ctx, w, r, "/", http.StatusFound)
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	} else if end {
		return nil
	}

	accounts, err := h.AccountRepo.Find(ctx, claims, account.AccountFindRequest{
		Order: []string{"name"},
	})
	if err != nil {
		return err
	}
	data["accounts"] = accounts.Response(ctx)

	if req.AccountID == "" {
		req.AccountID = claims.Audience
	}

	data["form"] = req

	if verr, ok := weberror.NewValidationError(ctx, webcontext.Validator().Struct(user_auth.SwitchAccountRequest{})); ok {
		data["validationDefaults"] = verr.(*weberror.Error)
	}

	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "user-switch-account.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}

// handleSessionToken persists the access token to the session for request authentication.
func handleSessionToken(ctx context.Context, w http.ResponseWriter, r *http.Request, token user_auth.Token) error {
	if token.AccessToken == "" {
		return errors.New("accessToken is required.")
	}

	sess := webcontext.ContextSession(ctx)

	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   int(token.TTL.Seconds()),
		HttpOnly: false,
	}

	sess = webcontext.SessionInit(sess,
		token.AccessToken)
	if err := sess.Save(r, w); err != nil {
		return err
	}

	return nil
}

// updateContextClaims updates the claims in the context.
func updateContextClaims(ctx context.Context, authenticator *auth.Authenticator, claims auth.Claims) (context.Context, error) {
	tkn, err := authenticator.GenerateToken(claims)
	if err != nil {
		return ctx, err
	}

	sess := webcontext.ContextSession(ctx)
	sess = webcontext.SessionUpdateAccessToken(sess, tkn)

	ctx = context.WithValue(ctx, auth.Key, claims)

	return ctx, nil
}
