package auth

import (
	"context"
	"fmt"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
)

// These are the expected values for Claims.Roles.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// ctxKey represents the type of value for the context key.
type ctxKey int

// Key is used to store/retrieve a Claims value from a context.Context.
const Key ctxKey = 1

// Claims represents the authorization claims transmitted via a JWT.
type Claims struct {
	AccountIds []string `json:"accounts"`
	Roles      []string `json:"roles"`
	Timezone   string   `json:"timezone"`
	tz         *time.Location
	jwt.StandardClaims
}

// NewClaims constructs a Claims value for the identified user. The Claims
// expire within a specified duration of the provided time. Additional fields
// of the Claims can be set after calling NewClaims is desired.
func NewClaims(userId, accountId string, accountIds []string, roles []string, userTimezone *time.Location, now time.Time, expires time.Duration) Claims {
	c := Claims{
		AccountIds: accountIds,
		Roles:      roles,
		StandardClaims: jwt.StandardClaims{
			Subject:   userId,
			Audience:  accountId,
			IssuedAt:  now.Unix(),
			ExpiresAt: now.Add(expires).Unix(),
		},
	}

	if userTimezone != nil {
		c.Timezone = userTimezone.String()
	}

	return c
}

// Valid is called during the parsing of a token.
func (c Claims) Valid() error {
	for _, r := range c.Roles {
		switch r {
		case RoleAdmin, RoleUser: // Role is valid.
		default:
			return fmt.Errorf("invalid role %q", r)
		}
	}
	if err := c.StandardClaims.Valid(); err != nil {
		return errors.Wrap(err, "validating standard claims")
	}
	return nil
}

// HasAuth returns true the user is authenticated.
func (c Claims) HasAuth() bool {
	if c.Subject != "" {
		return true
	}
	return false
}

// HasRole returns true if the claims has at least one of the provided roles.
func (c Claims) HasRole(roles ...string) bool {
	for _, has := range c.Roles {
		for _, want := range roles {
			if has == want {
				return true
			}
		}
	}
	return false
}

// TimeLocation returns the timezone used to format datetimes for the user.
func (c Claims) TimeLocation() *time.Location {
	if c.tz == nil && c.Timezone != "" {
		c.tz, _ = time.LoadLocation(c.Timezone)
	}
	return c.tz
}

// ClaimsFromContext loads the claims from context.
func ClaimsFromContext(ctx context.Context) (Claims, error) {
	claims, ok := ctx.Value(Key).(Claims)
	if !ok {
		// TODO(jlw) should this be a web.Shutdown?
		return Claims{}, errors.New("claims missing from context: HasRole called without/before Authenticate")
	}

	return claims, nil
}
