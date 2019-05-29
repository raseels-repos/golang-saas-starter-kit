package user

import (
	"database/sql"
	"database/sql/driver"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
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
	Timezone        *string     `json:"timezone" validate:"omitempty"`
}

// UpdateUserRequest defines what information may be provided to modify an existing
// User. All fields are optional so clients can send just the fields they want
// changed. It uses pointer fields so we can differentiate between a field that
// was not provided and a field that was provided as explicitly blank. Normally
// we do not want to use pointers to basic types but we make exceptions around
// marshalling/unmarshalling.
type UpdateUserRequest struct {
	ID       string      `validate:"required,uuid"`
	Name     *string     `json:"name" validate:"omitempty"`
	Email    *string     `json:"email" validate:"omitempty,email,unique"`
	Timezone *string     `json:"timezone" validate:"omitempty"`
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
	ID         string           `db:"id" json:"id"`
	UserID     string           `db:"user_id" json:"user_id"`
	AccountID  string           `db:"account_id" json:"account_id"`
	Roles      UserAccountRoles `db:"roles" json:"roles"`
	Status   UserAccountStatus `db:"status" json:"status"`
	CreatedAt  time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time        `db:"updated_at" json:"updated_at"`
	ArchivedAt pq.NullTime      `db:"archived_at" json:"archived_at"`
}

// AddAccountRequest defines the information needed to add a new account to a user.
type AddAccountRequest struct {
	UserID    string           `validate:"required,uuid"`
	AccountID string           `validate:"required,uuid"`
	Roles     UserAccountRoles `json:"roles" validate:"required,dive,oneof=admin user"`
	Status          *UserAccountStatus `json:"status" validate:"omitempty,oneof=active disabled"`
}

// UpdateAccountRequest defines the information needed to update the roles for
// an existing user account.
type UpdateAccountRequest struct {
	UserID    string           `validate:"required,uuid"`
	AccountID string           `validate:"required,uuid"`
	Roles     *UserAccountRoles `json:"roles" validate:"required,dive,oneof=admin user"`
	Status   *UserAccountStatus `json:"status" validate:"omitempty,oneof=active disabled"`
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

// UserAccountStatus represents the status of a user.
type UserAccountStatus string

// UserAccountStatus values
const (
	UserAccountStatus_Active   UserAccountStatus = "active"
	UserAccountStatus_Disabled UserAccountStatus = "disabled"
)

// UserAccountStatus_Values provides list of valid UserAccountStatus values
var UserAccountStatus_Values = []UserAccountStatus{
	UserAccountStatus_Active,
	UserAccountStatus_Disabled,
}

// Scan supports reading the UserAccountStatus value from the database.
func (s *UserAccountStatus) Scan(value interface{}) error {
	asBytes, ok := value.([]byte)
	if !ok {
		return errors.New("Scan source is not []byte")
	}
	*s = UserAccountStatus(string(asBytes))
	return nil
}

// Value converts the UserAccountStatus value to be stored in the database.
func (s UserAccountStatus) Value() (driver.Value, error) {
	v := validator.New()

	errs := v.Var(s, "required,oneof=active disabled")
	if errs != nil {
		return nil, errs
	}

	return string(s), nil
}

// String converts the UserAccountStatus value to a string.
func (s UserAccountStatus) String() string {
	return string(s)
}

// UserAccountRole represents the role of a user for an account.
type UserAccountRole string

// UserAccountRole values
const (
	UserAccountRole_Admin UserAccountRole = auth.RoleAdmin
	UserAccountRole_User  UserAccountRole = auth.RoleUser
)

// UserAccountRole_Values provides list of valid UserAccountRole values
var UserAccountRole_Values = []UserAccountRole{
	UserAccountRole_Admin,
	UserAccountRole_User,
}

// String converts the UserAccountRole value to a string.
func (s UserAccountRole) String() string {
	return string(s)
}

// UserAccountRoles represents a set of roles for a user for an account.
type UserAccountRoles []UserAccountRole

// Scan supports reading the UserAccountRole value from the database.
func (s *UserAccountRoles) Scan(value interface{}) error {
	arr := &pq.StringArray{}
	if err := arr.Scan(value); err != nil {
		return err
	}

	for _, v := range *arr {
		*s = append(*s, UserAccountRole(v))
	}

	return nil
}

// Value converts the UserAccountRole value to be stored in the database.
func (s UserAccountRoles) Value() (driver.Value, error) {
	v := validator.New()

	var arr pq.StringArray
	for _, r := range s {
		errs := v.Var(r, "required,oneof=admin user")
		if errs != nil {
			return nil, errs
		}
		arr = append(arr, r.String())
	}

	return arr.Value()
}

// Token is the payload we deliver to users when they authenticate.
type Token struct {
	Token  string      `json:"token"`
	claims auth.Claims `json:"-"`
}
