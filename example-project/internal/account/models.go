package account

import (
	"database/sql"
	"database/sql/driver"
	"time"

	"github.com/lib/pq"
	"gopkg.in/go-playground/validator.v9"
	"github.com/pkg/errors"
)

// Account represents someone with access to our system.
type Account struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Address1 string `json:"address1"`
	Address2 string `json:"address2"`
	City string `json:"city"`
	Region string `json:"region"`
	Country string `json:"country"`
	Zipcode string `json:"zipcode"`
	Status     AccountStatus `json:"status"`
	Timezone string `json:"timezone"`
	SignupUserID sql.NullString `json:"signup_user_id"`
	BillingUserID sql.NullString `json:"billing_user_id"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
	ArchivedAt pq.NullTime `json:"archived_at"`
}

// CreateAccountRequest contains information needed to create a new Account.
type CreateAccountRequest struct {
	Name            string  `json:"name" validate:"required,unique"`
	Address1        string  `json:"address1" validate:"required"`
	Address2        string `json:"address2" validate:"omitempty"`
	City string `json:"city" validate:"required"`
	Region string `json:"region" validate:"required"`
	Country string `json:"country" validate:"required"`
	Zipcode string `json:"zipcode" validate:"required"`
	Status    *AccountStatus `json:"status" validate:"omitempty,oneof=active pending disabled"`
	Timezone        *string `json:"timezone" validate:"omitempty"`
	SignupUserID *string `json:"signup_user_id" validate:"omitempty,uuid"`
	BillingUserID *string `json:"billing_user_id" validate:"omitempty,uuid"`
}

// UpdateAccountRequest defines what information may be provided to modify an existing
// Account. All fields are optional so clients can send just the fields they want
// changed. It uses pointer fields so we can differentiate between a field that
// was not provided and a field that was provided as explicitly blank. Normally
// we do not want to use pointers to basic types but we make exceptions around
// marshalling/unmarshalling.
type UpdateAccountRequest struct {
	ID       string  `validate:"required,uuid"`
	Name        *string  `json:"name" validate:"omitempty,unique"`
	Address1        *string  `json:"address1" validate:"omitempty"`
	Address2        *string `json:"address2" validate:"omitempty"`
	City *string `json:"city" validate:"omitempty"`
	Region *string `json:"region" validate:"omitempty"`
	Country *string `json:"country" validate:"omitempty"`
	Zipcode *string `json:"zipcode" validate:"omitempty"`
	Status    *AccountStatus `json:"status" validate:"omitempty,oneof=active pending disabled"`
	Timezone        *string `json:"timezone" validate:"omitempty"`
	SignupUserID *string `json:"signup_user_id" validate:"omitempty,uuid"`
	BillingUserID *string `json:"billing_user_id" validate:"omitempty,uuid"`
}

// AccountFindRequest defines the possible options to search for accounts. By default
// archived accounts will be excluded from response.
type AccountFindRequest struct {
	Where            *string
	Args             []interface{}
	Order            []string
	Limit            *uint
	Offset           *uint
	IncludedArchived bool
}

// AccountStatus represents the status of an account.
type AccountStatus string

// AccountStatus values define the status field of a user account.
const (
	// AccountStatus_Active defines the state when a user can access an account.
	AccountStatus_Active AccountStatus = "active"
	// AccountStatus_Pending defined the state when an account was created but
	// not activated.
	AccountStatus_Pending AccountStatus = "pending"
	// AccountStatus_Disabled defines the state when a user has been disabled from
	// accessing an account.
	AccountStatus_Disabled AccountStatus = "disabled"
)

// AccountStatus_Values provides list of valid AccountStatus values.
var AccountStatus_Values = []AccountStatus{
	AccountStatus_Active,
	AccountStatus_Pending,
	AccountStatus_Disabled,
}

// Scan supports reading the AccountStatus value from the database.
func (s *AccountStatus) Scan(value interface{}) error {
	asBytes, ok := value.([]byte)
	if !ok {
		return errors.New("Scan source is not []byte")
	}
	*s = AccountStatus(string(asBytes))
	return nil
}

// Value converts the AccountStatus value to be stored in the database.
func (s AccountStatus) Value() (driver.Value, error) {
	v := validator.New()

	errs := v.Var(s, "required,oneof=active invited disabled")
	if errs != nil {
		return nil, errs
	}

	return string(s), nil
}

// String converts the AccountStatus value to a string.
func (s AccountStatus) String() string {
	return string(s)
}

