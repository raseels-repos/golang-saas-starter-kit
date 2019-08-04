package account_preference

import (
	"context"
	"github.com/pkg/errors"
	"time"

	"database/sql/driver"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/lib/pq"
	"gopkg.in/go-playground/validator.v9"
)

// AccountPreference represents an account setting.
type AccountPreference struct {
	AccountID  string                `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name       AccountPreferenceName `json:"name" validate:"required,oneof=datetime_format date_format time_format" swaggertype:"string" enums:"datetime_format,date_format,time_format" example:"datetime_format"`
	Value      string                `json:"value" validate:"required,preference_value" example:"2006-01-02 at 3:04PM MST"`
	CreatedAt  time.Time             `json:"created_at"`
	UpdatedAt  time.Time             `json:"updated_at"`
	ArchivedAt *pq.NullTime          `json:"archived_at,omitempty"`
}

// AccountPreferenceResponse represents an account setting that is returned for display.
type AccountPreferenceResponse struct {
	AccountID  string            `json:"account_id" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name       web.EnumResponse  `json:"name" example:"datetime_format"`
	Value      string            `json:"value"  example:"2006-01-02 at 3:04PM MST"`
	CreatedAt  web.TimeResponse  `json:"created_at"`            // CreatedAt contains multiple format options for display.
	UpdatedAt  web.TimeResponse  `json:"updated_at"`            // UpdatedAt contains multiple format options for display.
	ArchivedAt *web.TimeResponse `json:"archived_at,omitempty"` // ArchivedAt contains multiple format options for display.
}

// Response transforms AccountPreference and AccountPreferenceResponse that is used for display.
// Additional filtering by context values or translations could be applied.
func (m *AccountPreference) Response(ctx context.Context) *AccountPreferenceResponse {
	if m == nil {
		return nil
	}

	r := &AccountPreferenceResponse{
		AccountID: m.AccountID,
		Name:      web.NewEnumResponse(ctx, m.Name, AccountPreferenceName_Values),
		Value:     m.Value,
		CreatedAt: web.NewTimeResponse(ctx, m.CreatedAt),
		UpdatedAt: web.NewTimeResponse(ctx, m.UpdatedAt),
	}

	if m.ArchivedAt != nil && !m.ArchivedAt.Time.IsZero() {
		at := web.NewTimeResponse(ctx, m.ArchivedAt.Time)
		r.ArchivedAt = &at
	}

	return r
}

// AccountPreferenceReadRequest contains information needed to read an Account Preference.
type AccountPreferenceReadRequest struct {
	AccountID       string                `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name            AccountPreferenceName `json:"name" validate:"required,oneof=datetime_format date_format time_format" swaggertype:"string" enums:"datetime_format,date_format,time_format" example:"datetime_format"`
	IncludeArchived bool                  `json:"include-archived" example:"false"`
}

// AccountPreferenceSetRequest contains information needed to create a new Account Preference.
type AccountPreferenceSetRequest struct {
	AccountID string                `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name      AccountPreferenceName `json:"name" validate:"required,oneof=datetime_format date_format time_format" swaggertype:"string" enums:"datetime_format,date_format,time_format" example:"datetime_format"`
	Value     string                `json:"value" validate:"required,preference_value" example:"2006-01-02 at 3:04PM MST"`
}

// AccountPreferenceArchiveRequest defines the information needed to archive an account preference.
// This will archive (soft-delete) the existing database entry.
type AccountPreferenceArchiveRequest struct {
	AccountID string                `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name      AccountPreferenceName `json:"name" validate:"required,oneof=datetime_format date_format time_format" swaggertype:"string" enums:"datetime_format,date_format,time_format" example:"datetime_format"`
}

// AccountPreferenceDeleteRequest defines the information needed to delete an account preference.
type AccountPreferenceDeleteRequest struct {
	AccountID string                `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name      AccountPreferenceName `json:"name" validate:"required,oneof=datetime_format date_format time_format" swaggertype:"string" enums:"datetime_format,date_format,time_format" example:"datetime_format"`
}

// AccountPreferenceFindRequest defines the possible options to search for accounts. By default
// archived accounts will be excluded from response.
type AccountPreferenceFindRequest struct {
	Where           *string       `json:"where" example:"name = ?"`
	Args            []interface{} `json:"args" swaggertype:"array,string" example:"Company Name,active"`
	Order           []string      `json:"order" example:"created_at desc"`
	Limit           *uint         `json:"limit" example:"10"`
	Offset          *uint         `json:"offset" example:"20"`
	IncludeArchived bool          `json:"include-archived" example:"false"`
}

// AccountPreferenceFindByAccountIDRequest defines the possible options to search for accounts. By default
// archived account preferences will be excluded from response.
type AccountPreferenceFindByAccountIDRequest struct {
	AccountID       string   `json:"id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Order           []string `json:"order" example:"created_at desc"`
	Limit           *uint    `json:"limit" example:"10"`
	Offset          *uint    `json:"offset" example:"20"`
	IncludeArchived bool     `json:"include-archived" example:"false"`
}

// AccountPreferenceName represents the name of an account preference.
type AccountPreferenceName string

// Account Preference Datetime Format
var (
	AccountPreference_Datetime_Format         AccountPreferenceName = "datetime_format"
	AccountPreference_Date_Format             AccountPreferenceName = "date_format"
	AccountPreference_Time_Format             AccountPreferenceName = "time_format"
	AccountPreference_Datetime_Format_Default                       = "2006-01-02 at 3:04PM MST"
	AccountPreference_Date_Format_Default                           = "2006-01-02"
	AccountPreference_Time_Format_Default                           = "3:04PM MST"
)

// AccountPreferenceName_Values provides list of valid AccountPreferenceName values.
var AccountPreferenceName_Values = []AccountPreferenceName{
	AccountPreference_Datetime_Format,
	AccountPreference_Date_Format,
	AccountPreference_Time_Format,
}

// Scan supports reading the AccountPreferenceName value from the database.
func (s *AccountPreferenceName) Scan(value interface{}) error {
	asBytes, ok := value.(string)
	if !ok {
		return errors.New("Scan source is not []byte")
	}
	*s = AccountPreferenceName(string(asBytes))
	return nil
}

// Value converts the AccountPreferenceName value to be stored in the database.
func (s AccountPreferenceName) Value() (driver.Value, error) {
	v := validator.New()

	errs := v.Var(s, "required,oneof=datetime_format date_format time_format")
	if errs != nil {
		return nil, errs
	}

	return string(s), nil
}

// String converts the AccountPreferenceName value to a string.
func (s AccountPreferenceName) String() string {
	return string(s)
}
