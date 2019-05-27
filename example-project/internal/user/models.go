package user

import (
	"database/sql"
	"database/sql/driver"
	"time"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// User represents someone with access to our system.
type User struct {
	ID    string `db:"id" json:"id"`
	Name  string `db:"name" json:"name"`
	Email string `db:"email" json:"email"`

	PasswordSalt  string         `db:"password_salt" json:"-"`
	PasswordHash  []byte         `db:"password_hash" json:"-"`
	PasswordReset sql.NullString `db:"password_reset" json:"-"`

	Status   UserStatus `db:"status" json:"status"`
	Timezone string     `db:"timezone" json:"timezone"`

	CreatedAt  time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time   `db:"updated_at" json:"updated_at"`
	ArchivedAt pq.NullTime `db:"archived_at" json:"archived_at"`
}

// CreateUserRequest contains information needed to create a new User.
type CreateUserRequest struct {
	Name            string      `json:"name" validate:"required"`
	Email           string      `json:"email" validate:"required,email,unique"`
	Password        string      `json:"password" validate:"required"`
	PasswordConfirm string      `json:"password_confirm" validate:"eqfield=Password"`
	Status          *UserStatus `json:"status" validate:"oneof=active disabled"`
	Timezone        *string     `json:"timezone"`
}

// UpdateUserRequest defines what information may be provided to modify an existing
// User. All fields are optional so clients can send just the fields they want
// changed. It uses pointer fields so we can differentiate between a field that
// was not provided and a field that was provided as explicitly blank. Normally
// we do not want to use pointers to basic types but we make exceptions around
// marshalling/unmarshalling.
type UpdateUserRequest struct {
	ID       string      `validate:"required,uuid"`
	Name     *string     `json:"name"`
	Email    *string     `json:"email" validate:"email,unique"`
	Status   *UserStatus `json:"status" validate:"oneof=active disabled"`
	Timezone *string     `json:"timezone"`
}

// UpdatePassword defines what information may be provided to update user password.
type UpdatePasswordRequest struct {
	ID              string `validate:"required,uuid"`
	Password        string `json:"password" validate:"required"`
	PasswordConfirm string `json:"password_confirm" validate:"omitempty,eqfield=Password"`
}

// UserFindRequest defines the possible options for search for users
type UserFindRequest struct {
	Where            *string
	Args             []interface{}
	Order            []string
	Limit            *uint
	Offset           *uint
	IncludedArchived bool
}

// UserAccount defines the one to many relationship of an user to an account.
// Each association of an user to an account has a set of roles defined for the user
// that will be applied when accessing the account.
type UserAccount struct {
	ID         string      `db:"id" json:"id"`
	UserID     string      `db:"user_id" json:"user_id"`
	AccountID  string      `db:"account_id" json:"account_id"`
	Roles      []string    `db:"roles" json:"roles"`
	CreatedAt  time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time   `db:"updated_at" json:"updated_at"`
	ArchivedAt pq.NullTime `db:"archived_at" json:"archived_at"`
}

// AddAccountRequest defines the information needed to add a new account to a user.
type AddAccountRequest struct {
	UserID    string   `validate:"required,uuid"`
	AccountID string   `validate:"required,uuid"`
	Roles     []string `json:"roles" validate:"oneof=admin user"`
}

// UpdateAccountRequest defines the information needed to update the roles for
// an existing user account.
type UpdateAccountRequest struct {
	UserID    string   `validate:"required,uuid"`
	AccountID string   `validate:"required,uuid"`
	Roles     []string `json:"roles" validate:"oneof=admin user"`
	unArchive bool
}

// RemoveAccountRequest defines the information needed to remove an existing
// account for a user. This will archive (soft-delete) the existing database entry.
type RemoveAccountRequest struct {
	UserID    string `validate:"required,uuid"`
	AccountID string `validate:"required,uuid"`
}

// DeleteAccountRequest defines the information needed to delete an existing
// account for a user. This will hard delete the existing database entry.
type DeleteAccountRequest struct {
	UserID    string `validate:"required,uuid"`
	AccountID string `validate:"required,uuid"`
}

// UserAccountFindRequest defines the possible options for search for users accounts
type UserAccountFindRequest struct {
	Where            *string
	Args             []interface{}
	Order            []string
	Limit            *uint
	Offset           *uint
	IncludedArchived bool
}

// UserStatus values
const (
	UserStatus_Active   UserStatus = "active"
	UserStatus_Disabled UserStatus = "disabled"
)

// UserStatus_Values provides list of valid UserStatus values
var UserStatus_Values = []UserStatus{
	UserStatus_Active,
	UserStatus_Disabled,
}

// UserStatus represents the status of a user.
type UserStatus string

// Scan supports reading the UserStatus value from the database.
func (s *UserStatus) Scan(value interface{}) error {
	asBytes, ok := value.([]byte)
	if !ok {
		return errors.New("Scan source is not []byte")
	}
	*s = UserStatus(string(asBytes))
	return nil
}

// Value converts the UserStatus value to be stored in the database.
func (s UserStatus) Value() (driver.Value, error) {
	v := validator.New()

	errs := v.Var(s, "required,oneof=active disabled")
	if errs != nil {
		return nil, errs
	}

	// validation would go here
	return string(s), nil
}

// String converts the UserStatus value to a string.
func (s UserStatus) String() string {
	return string(s)
}

// Token is the payload we deliver to users when they authenticate.
type Token struct {
	Token string `json:"token"`
}
