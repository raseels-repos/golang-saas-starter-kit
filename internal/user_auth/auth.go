package user_auth

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account/account_preference"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"github.com/huandu/go-sqlbuilder"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var (
	// ErrAuthenticationFailure occurs when a user attempts to authenticate but
	// anything goes wrong.
	ErrAuthenticationFailure = errors.New("Authentication failed")

	// ErrForbidden occurs when a user tries to do something that is forbidden to them according to our access control policies.
	ErrForbidden = errors.New("Attempted action is not allowed")
)

const (
	// The database table for User
	userTableName = "users"
	// The database table for Account
	accountTableName = "accounts"
	// The database table for User Account
	userAccountTableName = "users_accounts"
)

// Authenticate finds a user by their email and verifies their password. On success
// it returns a Token that can be used to authenticate access to the application in
// the future.
func (repo *Repository) Authenticate(ctx context.Context, req AuthenticateRequest, expires time.Duration, now time.Time, scopes ...string) (Token, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_auth.Authenticate")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return Token{}, err
	}

	u, err := repo.User.ReadByEmail(ctx, auth.Claims{}, req.Email, false)
	if err != nil {
		if errors.Cause(err) == user.ErrNotFound {
			err = errors.WithStack(ErrAuthenticationFailure)
			return Token{}, err
		} else {
			return Token{}, err
		}
	}

	// Append the salt from the user record to the supplied password.
	saltedPassword := req.Password + u.PasswordSalt

	// Compare the provided password with the saved hash. Use the bcrypt comparison
	// function so it is cryptographically secure. Return authentication error for
	// invalid password.
	if err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(saltedPassword)); err != nil {
		err = errors.WithStack(ErrAuthenticationFailure)
		return Token{}, err
	}

	// The user is successfully authenticated with the supplied email and password.
	return repo.generateToken(ctx, auth.Claims{}, u.ID, req.AccountID, expires, now, scopes...)
}

// SwitchAccount allows users to switch between multiple accounts, this changes the claim audience.
func (repo *Repository) SwitchAccount(ctx context.Context, claims auth.Claims, req SwitchAccountRequest, expires time.Duration, now time.Time, scopes ...string) (Token, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_auth.SwitchAccount")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return Token{}, err
	}

	claims.RootAccountID = req.AccountID

	if claims.RootUserID == "" {
		claims.RootUserID = claims.Subject
	}

	// Generate a token for the user ID in supplied in claims as the Subject. Pass
	// in the supplied claims as well to enforce ACLs when finding the current
	// list of accounts for the user.
	return repo.generateToken(ctx, claims, claims.Subject, req.AccountID, expires, now, scopes...)
}

// VirtualLogin allows users to mock being logged in as other users.
func (repo *Repository) VirtualLogin(ctx context.Context, claims auth.Claims, req VirtualLoginRequest, expires time.Duration, now time.Time, scopes ...string) (Token, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_auth.VirtualLogin")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return Token{}, err
	}

	// Find all the accounts that the current user has access to.
	usrAccs, err := repo.UserAccount.FindByUserID(ctx, claims, claims.Subject, false)
	if err != nil {
		return Token{}, err
	}

	// The user must have the role of admin to login any other user.
	var hasAccountAdminRole bool
	for _, usrAcc := range usrAccs {
		if usrAcc.HasRole(user_account.UserAccountRole_Admin) {
			if usrAcc.AccountID == req.AccountID {
				hasAccountAdminRole = true
				break
			}
		}
	}
	if !hasAccountAdminRole {
		return Token{}, errors.WithMessagef(ErrForbidden, "User %s does not have correct access to account %s ", claims.Subject, req.AccountID)
	}

	if claims.RootAccountID == "" {
		claims.RootAccountID = claims.Audience
	}
	if claims.RootUserID == "" {
		claims.RootUserID = claims.Subject
	}

	// Generate a token for the user ID in supplied in claims as the Subject. Pass
	// in the supplied claims as well to enforce ACLs when finding the current
	// list of accounts for the user.
	return repo.generateToken(ctx, claims, req.UserID, req.AccountID, expires, now, scopes...)
}

// VirtualLogout allows switch back to their root user/account.
func (repo *Repository) VirtualLogout(ctx context.Context, claims auth.Claims, expires time.Duration, now time.Time, scopes ...string) (Token, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_auth.VirtualLogout")
	defer span.Finish()

	// Generate a token for the user ID in supplied in claims as the Subject. Pass
	// in the supplied claims as well to enforce ACLs when finding the current
	// list of accounts for the user.
	return repo.generateToken(ctx, claims, claims.RootUserID, claims.RootAccountID, expires, now, scopes...)
}

// generateToken generates claims for the supplied user ID and account ID and then
// returns the token for the generated claims used for authentication.
func (repo *Repository) generateToken(ctx context.Context, claims auth.Claims, userID, accountID string, expires time.Duration, now time.Time, scopes ...string) (Token, error) {

	type userAccount struct {
		AccountID       string
		Roles           pq.StringArray
		UserStatus      string
		UserArchived    pq.NullTime
		AccountStatus   string
		AccountArchived pq.NullTime
		AccountTimezone sql.NullString
		UserTimezone    sql.NullString
	}

	// Build select statement for users_accounts table to find all the user accounts for the user
	f := func() ([]userAccount, error) {
		query := sqlbuilder.NewSelectBuilder().Select("ua.account_id, ua.roles, ua.status as userStatus, ua.archived_at userArchived, a.status as accountStatus, a.archived_at, a.timezone, u.timezone as userTimezone").
			From(userAccountTableName+" ua").
			Join(accountTableName+" a", "a.id = ua.account_id").
			Join(userTableName+" u", "u.id = ua.user_id")
		query.Where(query.And(
			query.Equal("ua.user_id", userID),
		))
		query.OrderBy("ua.status, a.status, ua.created_at")

		// fetch all places from the db
		queryStr, queryArgs := query.Build()
		queryStr = repo.DbConn.Rebind(queryStr)
		rows, err := repo.DbConn.QueryContext(ctx, queryStr, queryArgs...)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			return nil, err
		}

		// iterate over each row
		var resp []userAccount
		for rows.Next() {
			var ua userAccount
			err = rows.Scan(&ua.AccountID, &ua.Roles, &ua.UserStatus, &ua.UserArchived, &ua.AccountStatus, &ua.AccountArchived, &ua.AccountTimezone, &ua.UserTimezone)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			if err != nil {
				err = errors.Wrapf(err, "query - %s", query.String())
				return nil, err
			}

			resp = append(resp, ua)
		}

		return resp, nil
	}

	accounts, err := f()
	if err != nil {
		err = errors.WithStack(ErrAuthenticationFailure)
		return Token{}, err
	}

	// Load the user account entry for the specified account ID. If none provided,
	// choose the first.
	var account userAccount
	if accountID == "" {
		// Try to choose the first active user account that has not been archived.
		for _, a := range accounts {
			if a.AccountArchived.Valid && !a.AccountArchived.Time.IsZero() {
				continue
			} else if a.UserArchived.Valid && !a.UserArchived.Time.IsZero() {
				continue
			} else if a.AccountStatus != "active" {
				continue
			} else if a.UserStatus != "active" {
				continue
			}

			account = accounts[0]
			accountID = account.AccountID
			break
		}

		// Select the first account associated with the user. For the login flow,
		// users could be forced to select a specific account to override this.
		if accountID == "" && len(accounts) > 0 {
			account = accounts[0]
			accountID = account.AccountID
		}
	} else {
		// Loop through all the accounts found for the user and select the specified
		// account.
		for _, a := range accounts {
			if a.AccountID == accountID {
				account = a
				break
			}
		}

		// If no matching entry was found for the specified account ID throw an error.
		if account.AccountID == "" {
			err = errors.WithStack(ErrAuthenticationFailure)
			return Token{}, err
		}
	}

	// Validate the user account is completely active.
	if account.AccountArchived.Valid && !account.AccountArchived.Time.IsZero() {
		err = errors.WithMessage(ErrAuthenticationFailure, "account is archived")
		return Token{}, err
	} else if account.UserArchived.Valid && !account.UserArchived.Time.IsZero() {
		err = errors.WithMessage(ErrAuthenticationFailure, "user account is archived")
		return Token{}, err
	} else if account.AccountStatus != "active" {
		err = errors.WithMessagef(ErrAuthenticationFailure, "account is not active with status of %s", account.AccountStatus)
		return Token{}, err
	} else if account.UserStatus != "active" {
		err = errors.WithMessagef(ErrAuthenticationFailure, "user account is not active with status of %s", account.UserStatus)
		return Token{}, err
	}

	// Generate a list of all the account IDs associated with the user so the use
	// has the ability to switch between accounts.
	var accountIds []string
	for _, a := range accounts {
		accountIds = append(accountIds, a.AccountID)
	}

	// Allow the scope to be defined for the claims. This enables testing via the API when a user has the role of admin
	// and would like to limit their role to user.
	var roles []string
	{
		if len(scopes) > 0 && scopes[0] != "" {
			// Parse scopes, handle when one value has a list of scopes
			// separated by a space.
			var scopeList []string
			for _, vs := range scopes {
				for _, v := range strings.Split(vs, " ") {
					v = strings.TrimSpace(v)
					if v == "" {
						continue
					}
					scopeList = append(scopeList, v)
				}
			}

			for _, s := range scopeList {
				var scopeValid bool
				for _, r := range account.Roles {
					if r == s || (s == auth.RoleUser && r == auth.RoleAdmin) {
						scopeValid = true
						break
					}
				}

				if scopeValid {
					roles = append(roles, s)
				} else {
					err := errors.Wrapf(ErrForbidden, "invalid scope '%s'", s)
					return Token{}, err
				}
			}
		} else {
			roles = account.Roles
		}

		if len(roles) == 0 {
			err := errors.Wrapf(ErrForbidden, "no roles defined for user")
			return Token{}, err
		}
	}

	var claimPref auth.ClaimPreferences
	{
		// Set the timezone if one is specifically set on the user.
		var tz *time.Location
		if account.UserTimezone.Valid && account.UserTimezone.String != "" {
			tz, _ = time.LoadLocation(account.UserTimezone.String)
		}

		// If user timezone failed to parse or none is set, check the timezone set on the account.
		if tz == nil && account.AccountTimezone.Valid && account.AccountTimezone.String != "" {
			tz, _ = time.LoadLocation(account.AccountTimezone.String)
		}

		prefs, err := repo.AccountPreference.FindByAccountID(ctx, auth.Claims{}, account_preference.AccountPreferenceFindByAccountIDRequest{
			AccountID: accountID,
		})
		if err != nil {
			return Token{}, err
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

		if preferenceDatetimeFormat == "" {
			preferenceDatetimeFormat = account_preference.AccountPreference_Datetime_Format_Default
		}
		if preferenceDateFormat == "" {
			preferenceDateFormat = account_preference.AccountPreference_Date_Format_Default
		}
		if preferenceTimeFormat == "" {
			preferenceTimeFormat = account_preference.AccountPreference_Time_Format_Default
		}

		claimPref = auth.NewClaimPreferences(tz, preferenceDatetimeFormat, preferenceDateFormat, preferenceTimeFormat)
	}

	// Ensure the current claims has the root values set.
	if (claims.RootAccountID == "" && claims.Audience != "") || (claims.RootUserID == "" && claims.Subject != "") {
		claims.RootAccountID = claims.Audience
		claims.RootUserID = claims.Subject
	}

	// JWT claims requires both an audience and a subject. For this application:
	// 	Subject: The ID of the user authenticated.
	// 	Audience: The ID of the account the user is accessing. A list of account IDs
	// 			  will also be included to support the user switching between them.
	newClaims := auth.NewClaims(userID, accountID, accountIds, roles, claimPref, now, expires)

	// Copy the original root account/user ID.
	newClaims.RootAccountID = claims.RootAccountID
	newClaims.RootUserID = claims.RootUserID

	// Generate a token for the user with the defined claims.
	tknStr, err := repo.TknGen.GenerateToken(newClaims)
	if err != nil {
		return Token{}, errors.Wrap(err, "generating token")
	}

	tkn := Token{
		AccessToken: tknStr,
		TokenType:   "Bearer",
		claims:      newClaims,
		UserID:      newClaims.Subject,
		AccountID:   newClaims.Audience,
	}

	if expires.Seconds() > 0 {
		tkn.Expiry = now.Add(expires)
		tkn.TTL = expires
	}

	return tkn, nil
}
