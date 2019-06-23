package user

import (
	"database/sql"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"time"

	"github.com/lib/pq"
)

// User represents someone with access to our system.
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`

	PasswordSalt  string         `json:"-"`
	PasswordHash  []byte         `json:"-"`
	PasswordReset sql.NullString `json:"-"`

	Timezone string `json:"timezone"`

	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
	ArchivedAt pq.NullTime `json:"archived_at"`
}

// CreateUserRequest contains information needed to create a new User.
type CreateUserRequest struct {
	Name            string  `json:"name" validate:"required"`
	Email           string  `json:"email" validate:"required,email,unique"`
	Password        string  `json:"password" validate:"required"`
	PasswordConfirm string  `json:"password_confirm" validate:"eqfield=Password"`
	Timezone        *string `json:"timezone" validate:"omitempty"`
}

// UpdateUserRequest defines what information may be provided to modify an existing
// User. All fields are optional so clients can send just the fields they want
// changed. It uses pointer fields so we can differentiate between a field that
// was not provided and a field that was provided as explicitly blank. Normally
// we do not want to use pointers to basic types but we make exceptions around
// marshalling/unmarshalling.
type UpdateUserRequest struct {
	ID       string  `validate:"required,uuid"`
	Name     *string `json:"name" validate:"omitempty"`
	Email    *string `json:"email" validate:"omitempty,email,unique"`
	Timezone *string `json:"timezone" validate:"omitempty"`
}

// UpdatePassword defines what information is required to update a user password.
type UpdatePasswordRequest struct {
	ID              string `validate:"required,uuid"`
	Password        string `json:"password" validate:"required"`
	PasswordConfirm string `json:"password_confirm" validate:"omitempty,eqfield=Password"`
}

// UserFindRequest defines the possible options to search for users. By default
// archived users will be excluded from response.
type UserFindRequest struct {
	Where            *string
	Args             []interface{}
	Order            []string
	Limit            *uint
	Offset           *uint
	IncludedArchived bool
}

// Token is the payload we deliver to users when they authenticate.
type Token struct {
	Token  string      `json:"token"`
	claims auth.Claims `json:"-"`
}
