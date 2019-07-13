package account

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"time"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// Account represents someone with access to our system.
type Account struct {
	ID            string          `json:"id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name          string          `json:"name" validate:"required,unique" example:"Company Name"`
	Address1      string          `json:"address1" validate:"required" example:"221 Tatitlek Ave"`
	Address2      string          `json:"address2" validate:"omitempty" example:"Box #1832"`
	City          string          `json:"city" validate:"required" example:"Valdez"`
	Region        string          `json:"region" validate:"required" example:"AK"`
	Country       string          `json:"country" validate:"required" example:"USA"`
	Zipcode       string          `json:"zipcode" validate:"required" example:"99686"`
	Status        AccountStatus   `json:"status" validate:"omitempty,oneof=active pending disabled" swaggertype:"string" enums:"active,pending,disabled" example:"active"`
	Timezone      string          `json:"timezone" validate:"omitempty" example:"America/Anchorage"`
	SignupUserID  *sql.NullString `json:"signup_user_id,omitempty" validate:"omitempty,uuid" swaggertype:"string" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	BillingUserID *sql.NullString `json:"billing_user_id,omitempty" validate:"omitempty,uuid" swaggertype:"string" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	ArchivedAt    *pq.NullTime    `json:"archived_at,omitempty"`
}

// AccountResponse represents someone with access to our system that is returned for display.
type AccountResponse struct {
	ID            string            `json:"id" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name          string            `json:"name" example:"Company Name"`
	Address1      string            `json:"address1" example:"221 Tatitlek Ave"`
	Address2      string            `json:"address2" example:"Box #1832"`
	City          string            `json:"city" example:"Valdez"`
	Region        string            `json:"region" example:"AK"`
	Country       string            `json:"country" example:"USA"`
	Zipcode       string            `json:"zipcode" example:"99686"`
	Status        web.EnumResponse  `json:"status"` // Status is enum with values [active, pending, disabled].
	Timezone      string            `json:"timezone" example:"America/Anchorage"`
	SignupUserID  *string           `json:"signup_user_id,omitempty" swaggertype:"string" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	BillingUserID *string           `json:"billing_user_id,omitempty" swaggertype:"string" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	CreatedAt     web.TimeResponse  `json:"created_at"`            // CreatedAt contains multiple format options for display.
	UpdatedAt     web.TimeResponse  `json:"updated_at"`            // UpdatedAt contains multiple format options for display.
	ArchivedAt    *web.TimeResponse `json:"archived_at,omitempty"` // ArchivedAt contains multiple format options for display.
}

// Response transforms Account and AccountResponse that is used for display.
// Additional filtering by context values or translations could be applied.
func (m *Account) Response(ctx context.Context) *AccountResponse {
	if m == nil {
		return nil
	}

	r := &AccountResponse{
		ID:        m.ID,
		Name:      m.Name,
		Address1:  m.Address1,
		Address2:  m.Address2,
		City:      m.City,
		Region:    m.Region,
		Country:   m.Country,
		Zipcode:   m.Zipcode,
		Timezone:  m.Timezone,
		Status:    web.NewEnumResponse(ctx, m.Status, AccountStatus_Values),
		CreatedAt: web.NewTimeResponse(ctx, m.CreatedAt),
		UpdatedAt: web.NewTimeResponse(ctx, m.UpdatedAt),
	}

	if m.SignupUserID != nil {
		r.SignupUserID = &m.SignupUserID.String
	}
	if m.BillingUserID != nil {
		r.BillingUserID = &m.BillingUserID.String
	}

	if m.ArchivedAt != nil && !m.ArchivedAt.Time.IsZero() {
		at := web.NewTimeResponse(ctx, m.ArchivedAt.Time)
		r.ArchivedAt = &at
	}

	return r
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
	SignupUserID  *string        `json:"signup_user_id,omitempty" validate:"omitempty,uuid" swaggertype:"string" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	BillingUserID *string        `json:"billing_user_id,omitempty" validate:"omitempty,uuid" swaggertype:"string" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
}

// AccountUpdateRequest defines what information may be provided to modify an existing
// Account. All fields are optional so clients can send just the fields they want
// changed. It uses pointer fields so we can differentiate between a field that
// was not provided and a field that was provided as explicitly blank. Normally
// we do not want to use pointers to basic types but we make exceptions around
// marshalling/unmarshalling.
type AccountUpdateRequest struct {
	ID            string         `json:"id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name          *string        `json:"name,omitempty" validate:"omitempty,unique" example:"Company Name"`
	Address1      *string        `json:"address1,omitempty" validate:"omitempty" example:"221 Tatitlek Ave"`
	Address2      *string        `json:"address2,omitempty" validate:"omitempty" example:"Box #1832"`
	City          *string        `json:"city,omitempty" validate:"omitempty" example:"Valdez"`
	Region        *string        `json:"region,omitempty" validate:"omitempty" example:"AK"`
	Country       *string        `json:"country,omitempty" validate:"omitempty" example:"USA"`
	Zipcode       *string        `json:"zipcode,omitempty" validate:"omitempty" example:"99686"`
	Status        *AccountStatus `json:"status,omitempty" validate:"omitempty,oneof=active pending disabled" swaggertype:"string" enums:"active,pending,disabled" example:"disabled"`
	Timezone      *string        `json:"timezone,omitempty" validate:"omitempty" example:"America/Anchorage"`
	SignupUserID  *string        `json:"signup_user_id,omitempty" validate:"omitempty,uuid" swaggertype:"string" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	BillingUserID *string        `json:"billing_user_id,omitempty" validate:"omitempty,uuid" swaggertype:"string" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
}

// AccountArchiveRequest defines the information needed to archive an account. This will archive (soft-delete) the
// existing database entry.
type AccountArchiveRequest struct {
	ID string `json:"id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
}

// AccountFindRequest defines the possible options to search for accounts. By default
// archived accounts will be excluded from response.
type AccountFindRequest struct {
	Where            *string       `json:"where" example:"name = ? and status = ?"`
	Args             []interface{} `json:"args" swaggertype:"array,string" example:"Company Name,active"`
	Order            []string      `json:"order" example:"created_at desc"`
	Limit            *uint         `json:"limit" example:"10"`
	Offset           *uint         `json:"offset" example:"20"`
	IncludedArchived bool          `json:"included-archived" example:"false"`
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
