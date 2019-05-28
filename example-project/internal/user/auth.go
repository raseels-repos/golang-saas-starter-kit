package user

import (
	"context"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// TokenGenerator is the behavior we need in our Authenticate to generate
// tokens for authenticated users.
type TokenGenerator interface {
	GenerateToken(auth.Claims) (string, error)
}

// Authenticate finds a user by their email and verifies their password. On
// success it returns a Token that can be used to authenticate in the future.
func Authenticate(ctx context.Context, dbConn *sqlx.DB, tknGen TokenGenerator, now time.Time, email, password string) (Token, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Authenticate")
	defer span.Finish()

	// Generate sql query to select user by email address
	query := sqlbuilder.NewSelectBuilder()
	query.Where(query.Equal("email", email))

	// Run the find, use empty claims to bypass ACLs
	res, err := find(ctx, auth.Claims{}, dbConn, query, []interface{}{}, false)
	if err != nil {
		return Token{}, err
	} else if res == nil || len(res) == 0 {
		err = errors.WithStack(ErrAuthenticationFailure)
		return Token{}, err
	}
	u := res[0]

	// Append the salt from the user record to the supplied password.
	saltedPassword := password + string(u.PasswordSalt)

	// Compare the provided password with the saved hash. Use the bcrypt
	// comparison function so it is cryptographically secure.
	if err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(saltedPassword)); err != nil {
		err = errors.WithStack(ErrAuthenticationFailure)
		return Token{}, err
	}

	// Get a list of all the account ids associated with the user.
	accounts, err := FindAccountsByUserID(ctx, auth.Claims{}, dbConn, u.ID, false)
	if err != nil {
		return Token{}, err
	}

	// Claims needs an audience, select the first account associated with
	// the user.
	var (
		accountId string
		roles     []string
	)
	if len(accounts) > 0 {
		accountId = accounts[0].AccountID
		for _, r := range accounts[0].Roles {
			roles = append(roles, r.String())
		}
	}

	// Generate a list of all the account IDs associated with the user so
	// the use has the ability to switch between accounts.
	accountIds := []string{}
	for _, a := range accounts {
		accountIds = append(accountIds, a.AccountID)
	}

	// If we are this far the request is valid. Create some claims for the user.
	claims := auth.NewClaims(u.ID, accountId, accountIds, roles, now, time.Hour)

	// Generate a token for the user with the defined claims.
	tkn, err := tknGen.GenerateToken(claims)
	if err != nil {
		return Token{}, errors.Wrap(err, "generating token")
	}

	return Token{Token: tkn, claims: claims}, nil
}
