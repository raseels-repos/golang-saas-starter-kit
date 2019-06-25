package account

import (
	"database/sql"
	"database/sql/driver"
	"time"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// Account represents someone with access to our system.
type Account struct {
	ID            string          `json:"id" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name          string          `json:"name" example:"Company Name"`
	Address1      string          `json:"address1" example:"221 Tatitlek Ave"`
	Address2      string          `json:"address2" example:"Box #1832"`
	City          string          `json:"city" example:"Valdez"`
	Region        string          `json:"region" example:"AK"`
	Country       string          `json:"country" example:"USA"`
	Zipcode       string          `json:"zipcode" example:"99686"`
	Status        AccountStatus   `json:"status" swaggertype:"string" example:"active"`
	Timezone      string          `json:"timezone" example:"America/Anchorage"`
	SignupUserID  *sql.NullString `json:"signup_user_id,omitempty" swaggertype:"string"`
	BillingUserID *sql.NullString `json:"billing_user_id,omitempty" swaggertype:"string"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	ArchivedAt    *pq.NullTime    `json:"archived_at,omitempty"`
}

// AccountCreateRequest contains information needed to create a new Account.
type AccountCreateRequest struct {
	Name          string         `json:"name" validate:"required,unique" example:"Company Name"`
	Address1      string         `json:"address1" validate:"required" example:"221 Tatitlek Ave"`
	Address2      string         `json:"address2" validate:"omitempty" example:"Box #1832"`
	City          string         `json:"city" validate:"required" example:"Valdez"`
	Region        string         `json:"region" validate:"required" example:"AK"`
	Country       string         `json:"country" validate:"required" example:"USA"`
	Zipcode       string         `json:"zipcode" validate:"required" example:"99686"`
	Status        *AccountStatus `json:"status,omitempty" validate:"omitempty,oneof=active pending disabled" swaggertype:"string" enums:"active,pending,disabled" example:"active"`
	Timezone      *string        `json:"timezone,omitempty" validate:"omitempty" example:"America/Anchorage"`
	SignupUserID  *string        `json:"signup_user_id,omitempty" validate:"omitempty,uuid" swaggertype:"string"`
	BillingUserID *string        `json:"billing_user_id,omitempty" validate:"omitempty,uuid" swaggertype:"string"`
}

// AccountUpdateRequest defines what information may be provided to modify an existing
// Account. All fields are optional so clients can send just the fields they want
// changed. It uses pointer fields so we can differentiate between a field that
// was not provided and a field that was provided as explicitly blank. Normally
// we do not want to use pointers to basic types but we make exceptions around
// marshalling/unmarshalling.
type AccountUpdateRequest struct {
	ID            string         `json:"id" validate:"required,uuid"`
	Name          *string        `json:"name,omitempty" validate:"omitempty,unique"`
	Address1      *string        `json:"address1,omitempty" validate:"omitempty"`
	Address2      *string        `json:"address2,omitempty" validate:"omitempty"`
	City          *string        `json:"city,omitempty" validate:"omitempty"`
	Region        *string        `json:"region,omitempty" validate:"omitempty"`
	Country       *string        `json:"country,omitempty" validate:"omitempty"`
	Zipcode       *string        `json:"zipcode,omitempty" validate:"omitempty"`
	Status        *AccountStatus `json:"status,omitempty" validate:"omitempty,oneof=active pending disabled" swaggertype:"string" enums:"active,pending,disabled"`
	Timezone      *string        `json:"timezone,omitempty" validate:"omitempty"`
	SignupUserID  *string        `json:"signup_user_id,omitempty" validate:"omitempty,uuid" swaggertype:"string"`
	BillingUserID *string        `json:"billing_user_id,omitempty" validate:"omitempty,uuid" swaggertype:"string"`
}

// AccountFindRequest defines the possible options to search for accounts. By default
// archived accounts will be excluded from response.
type AccountFindRequest struct {
	Where            *string       `json:"where"`
	Args             []interface{} `json:"args" swaggertype:"array,string"`
	Order            []string      `json:"order"`
	Limit            *uint         `json:"limit"`
	Offset           *uint         `json:"offset"`
	IncludedArchived bool          `json:"included-archived"`
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
