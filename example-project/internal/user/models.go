package user

import (
	"database/sql"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"time"

	"github.com/lib/pq"
)

// User represents someone with access to our system.
type User struct {
	ID    string `json:"id"  example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	Name  string `json:"name" validate:"required"  example:"Gabi May"`
	Email string `json:"email"  example:"gabi@geeksinthewoods.com"`

	PasswordSalt  string          `json:"-"`
	PasswordHash  []byte          `json:"-"`
	PasswordReset *sql.NullString `json:"-"`

	Timezone string `json:"timezone" example:"America/Anchorage"`

	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
	ArchivedAt *pq.NullTime `json:"archived_at,omitempty"`
}

// UserCreateRequest contains information needed to create a new User.
type UserCreateRequest struct {
	Name            string  `json:"name" validate:"required"  example:"Gabi May"`
	Email           string  `json:"email" validate:"required,email,unique"  example:"gabi@geeksinthewoods.com"`
	Password        string  `json:"password" validate:"required" example:"SecretString"`
	PasswordConfirm string  `json:"password_confirm" validate:"eqfield=Password" example:"SecretString"`
	Timezone        *string `json:"timezone,omitempty" validate:"omitempty" example:"America/Anchorage"`
}

// UserUpdateRequest defines what information may be provided to modify an existing
// User. All fields are optional so clients can send just the fields they want
// changed. It uses pointer fields so we can differentiate between a field that
// was not provided and a field that was provided as explicitly blank. Normally
// we do not want to use pointers to basic types but we make exceptions around
// marshalling/unmarshalling.
type UserUpdateRequest struct {
	ID       string  `json:"id" validate:"required,uuid"`
	Name     *string `json:"name,omitempty" validate:"omitempty"`
	Email    *string `json:"email,omitempty" validate:"omitempty,email,unique"`
	Timezone *string `json:"timezone,omitempty" validate:"omitempty"`
}

// UserUpdatePasswordRequest defines what information is required to update a user password.
type UserUpdatePasswordRequest struct {
	ID              string `json:"id" validate:"required,uuid"`
	Password        string `json:"password" validate:"required"`
	PasswordConfirm string `json:"password_confirm" validate:"omitempty,eqfield=Password"`
}

// UserFindRequest defines the possible options to search for users. By default
// archived users will be excluded from response.
type UserFindRequest struct {
	Where            *string       `json:"where"`
	Args             []interface{} `json:"args" swaggertype:"array,string"`
	Order            []string      `json:"order"`
	Limit            *uint         `json:"limit"`
	Offset           *uint         `json:"offset"`
	IncludedArchived bool          `json:"included-archived"`
}

// Token is the payload we deliver to users when they authenticate.
type Token struct {
	// AccessToken is the token that authorizes and authenticates
	// the requests.
	AccessToken string `json:"access_token"`
	// TokenType is the type of token.
	// The Type method returns either this or "Bearer", the default.
	TokenType string `json:"token_type,omitempty"`
	// Expiry is the optional expiration time of the access token.
	//
	// If zero, TokenSource implementations will reuse the same
	// token forever and RefreshToken or equivalent
	// mechanisms for that TokenSource will not be used.
	Expiry time.Time `json:"expiry,omitempty"`
	// contains filtered or unexported fields
	claims auth.Claims `json:"-"`
}
