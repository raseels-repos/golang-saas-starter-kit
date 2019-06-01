package user

import (
	"context"
	"gopkg.in/go-playground/validator.v9"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// TokenGenerator is the behavior we need in our Authenticate to generate tokens for
// authenticated users.
type TokenGenerator interface {
	GenerateToken(auth.Claims) (string, error)
	ParseClaims(string) (auth.Claims, error)
}

// Authenticate finds a user by their email and verifies their password. On success
// it returns a Token that can be used to authenticate access to the application in
// the future.
func Authenticate(ctx context.Context, dbConn *sqlx.DB, tknGen TokenGenerator, email, password string, expires time.Duration, now time.Time) (Token, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Authenticate")
	defer span.Finish()

	// Generate sql query to select user by email address.
	query := sqlbuilder.NewSelectBuilder()
	query.Where(query.Equal("email", email))

	// Run the find, use empty claims to bypass ACLs since this in an internal request
	// and the current user is not authenticated at this point. If the email is
	// invalid, return the same error as when an invalid password is supplied.
	res, err := find(ctx, auth.Claims{}, dbConn, query, []interface{}{}, false)
	if err != nil {
		return Token{}, err
	} else if res == nil || len(res) == 0 {
		err = errors.WithStack(ErrAuthenticationFailure)
		return Token{}, err
	}
	u := res[0]

	// Append the salt from the user record to the supplied password.
	saltedPassword := password + u.PasswordSalt

	// Compare the provided password with the saved hash. Use the bcrypt comparison
	// function so it is cryptographically secure. Return authentication error for
	// invalid password.
	if err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(saltedPassword)); err != nil {
		err = errors.WithStack(ErrAuthenticationFailure)
		return Token{}, err
	}

	// The user is successfully authenticated with the supplied email and password.
	return generateToken(ctx, dbConn, tknGen, auth.Claims{}, u.ID, "", expires, now)
}

// Authenticate finds a user by their email and verifies their password. On success
// it returns a Token that can be used to authenticate access to the application in
// the future.
func SwitchAccount(ctx context.Context, dbConn *sqlx.DB, tknGen TokenGenerator, claims auth.Claims, accountID string, expires time.Duration, now time.Time) (Token, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.SwitchAccount")
	defer span.Finish()

	// Defines struct to apply validation for the supplied claims and account ID.
	req := struct {
		UserID    string `validate:"required,uuid"`
		AccountID string `validate:"required,uuid"`
	}{
		UserID:    claims.Subject,
		AccountID: accountID,
	}

	// Validate the request.
	err := validator.New().Struct(req)
	if err != nil {
		return Token{}, err
	}

	// Generate a token for the user ID in supplied in claims as the Subject. Pass
	// in the supplied claims as well to enforce ACLs when finding the current
	// list of accounts for the user.
	return generateToken(ctx, dbConn, tknGen, claims, req.UserID, req.AccountID, expires, now)
}

// generateToken generates claims for the supplied user ID and account ID and then
// returns the token for the generated claims used for authentication.
func generateToken(ctx context.Context, dbConn *sqlx.DB, tknGen TokenGenerator, claims auth.Claims, userID, accountID string, expires time.Duration, now time.Time) (Token, error) {
	// Get a list of all the accounts associated with the user.
	accounts, err := FindAccountsByUserID(ctx, auth.Claims{}, dbConn, userID, false)
	if err != nil {
		return Token{}, err
	}

	// Load the user account entry for the specifed account ID. If none provided,
	// choose the first.
	var account *UserAccount
	if accountID == "" {
		// Select the first account associated with the user. For the login flow,
		// users could be forced to select a specific account to override this.
		if len(accounts) > 0 {
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
		if account == nil {
			err = errors.WithStack(ErrAuthenticationFailure)
			return Token{}, err
		}
	}

	// Generate list of user defined roles for accessing the account.
	var roles []string
	if account != nil {
		for _, r := range account.Roles {
			roles = append(roles, r.String())
		}
	}

	// Generate a list of all the account IDs associated with the user so the use
	// has the ability to switch between accounts.
	var accountIds []string
	for _, a := range accounts {
		accountIds = append(accountIds, a.AccountID)
	}

	// JWT claims requires both an audience and a subject. For this application:
	// 	Subject: The ID of the user authenticated.
	// 	Audience: The ID of the account the user is accessing. A list of account IDs
	// 			  will also be included to support the user switching between them.
	claims = auth.NewClaims(userID, accountID, accountIds, roles, now, expires)

	// Generate a token for the user with the defined claims.
	tkn, err := tknGen.GenerateToken(claims)
	if err != nil {
		return Token{}, errors.Wrap(err, "generating token")
	}

	return Token{Token: tkn, claims: claims}, nil
}
