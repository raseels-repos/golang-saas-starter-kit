package user

import (
	"context"
	"crypto/rsa"
	"github.com/dgrijalva/jwt-go"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/go-playground/validator.v9"
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

	type userAccount struct {
		AccountID       string
		Roles           pq.StringArray
		UserStatus      string
		UserArchived    pq.NullTime
		AccountStatus   string
		AccountArchived pq.NullTime
	}

	// Build select statement for users_accounts table to find all the user accounts for the user
	f := func() ([]userAccount, error) {
		query := sqlbuilder.NewSelectBuilder().Select("ua.account_id, ua.roles, ua.status as userStatus, ua.archived_at userArchived, a.status as accountStatus, a.archived_at as accountArchived").
			From(userAccountTableName+" ua").
			Join(accountTableName+" a", "a.id = ua.account_id")
		query.Where(query.And(
			query.Equal("ua.user_id", userID),
		))
		query.OrderBy("ua.status, a.status, ua.created_at")

		// fetch all places from the db
		queryStr, queryArgs := query.Build()
		queryStr = dbConn.Rebind(queryStr)
		rows, err := dbConn.QueryContext(ctx, queryStr, queryArgs...)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			return nil, err
		}

		// iterate over each row
		var resp []userAccount
		for rows.Next() {
			var ua userAccount
			err = rows.Scan(&ua.AccountID, &ua.Roles, &ua.UserStatus, &ua.UserArchived, &ua.AccountStatus, &ua.AccountArchived)
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

	// JWT claims requires both an audience and a subject. For this application:
	// 	Subject: The ID of the user authenticated.
	// 	Audience: The ID of the account the user is accessing. A list of account IDs
	// 			  will also be included to support the user switching between them.
	claims = auth.NewClaims(userID, accountID, accountIds, account.Roles, now, expires)

	// Generate a token for the user with the defined claims.
	tknStr, err := tknGen.GenerateToken(claims)
	if err != nil {
		return Token{}, errors.Wrap(err, "generating token")
	}

	tkn := Token{
		AccessToken: tknStr,
		TokenType:   "Bearer",
		claims:      claims,
	}

	if expires.Seconds() > 0 {
		tkn.Expiry = now.Add(expires)
	}

	return tkn, nil
}

// mockTokenGenerator is used for testing that Authenticate calls its provided
// token generator in a specific way.
type MockTokenGenerator struct {
	// Private key generated by GenerateToken that is need for ParseClaims
	key *rsa.PrivateKey
	// algorithm is the method used to generate the private key.
	algorithm string
}

// GenerateToken implements the TokenGenerator interface. It returns a "token"
// that includes some information about the claims it was passed.
func (g *MockTokenGenerator) GenerateToken(claims auth.Claims) (string, error) {
	privateKey, err := auth.KeyGen()
	if err != nil {
		return "", err
	}

	g.key, err = jwt.ParseRSAPrivateKeyFromPEM(privateKey)
	if err != nil {
		return "", err
	}

	g.algorithm = "RS256"
	method := jwt.GetSigningMethod(g.algorithm)

	tkn := jwt.NewWithClaims(method, claims)
	tkn.Header["kid"] = "1"

	str, err := tkn.SignedString(g.key)
	if err != nil {
		return "", err
	}

	return str, nil
}

// ParseClaims recreates the Claims that were used to generate a token. It
// verifies that the token was signed using our key.
func (g *MockTokenGenerator) ParseClaims(tknStr string) (auth.Claims, error) {
	parser := jwt.Parser{
		ValidMethods: []string{g.algorithm},
	}

	if g.key == nil {
		return auth.Claims{}, errors.New("Private key is empty.")
	}

	f := func(t *jwt.Token) (interface{}, error) {
		return g.key.Public().(*rsa.PublicKey), nil
	}

	var claims auth.Claims
	tkn, err := parser.ParseWithClaims(tknStr, &claims, f)
	if err != nil {
		return auth.Claims{}, errors.Wrap(err, "parsing token")
	}

	if !tkn.Valid {
		return auth.Claims{}, errors.New("Invalid token")
	}

	return claims, nil
}
