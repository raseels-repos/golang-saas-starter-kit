package checklist

import (
	"context"
	"time"

	"database/sql/driver"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// Repository defines the required dependencies for Checklist.
type Repository struct {
	DbConn *sqlx.DB
}

// NewRepository creates a new Repository that defines dependencies for Checklist.
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{
		DbConn: db,
	}
}

// Checklist represents a workflow.
type Checklist struct {
	ID         string          `json:"id" validate:"required,uuid" example:"985f1746-1d9f-459f-a2d9-fc53ece5ae86"`
	AccountID  string          `json:"account_id" validate:"required,uuid" truss:"api-create"`
	Name       string          `json:"name"  validate:"required" example:"Rocket Launch"`
	Status     ChecklistStatus `json:"status" validate:"omitempty,oneof=active disabled" enums:"active,disabled" swaggertype:"string" example:"active"`
	CreatedAt  time.Time       `json:"created_at" truss:"api-read"`
	UpdatedAt  time.Time       `json:"updated_at" truss:"api-read"`
	ArchivedAt *pq.NullTime    `json:"archived_at,omitempty" truss:"api-hide"`
}

// ChecklistResponse represents a workflow that is returned for display.
type ChecklistResponse struct {
	ID         string            `json:"id" validate:"required,uuid" example:"985f1746-1d9f-459f-a2d9-fc53ece5ae86"`
	AccountID  string            `json:"account_id" validate:"required,uuid" truss:"api-create" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name       string            `json:"name"  validate:"required" example:"Rocket Launch"`
	Status     web.EnumResponse  `json:"status"`                // Status is enum with values [active, disabled].
	CreatedAt  web.TimeResponse  `json:"created_at"`            // CreatedAt contains multiple format options for display.
	UpdatedAt  web.TimeResponse  `json:"updated_at"`            // UpdatedAt contains multiple format options for display.
	ArchivedAt *web.TimeResponse `json:"archived_at,omitempty"` // ArchivedAt contains multiple format options for display.
}

// Response transforms Checklist and ChecklistResponse that is used for display.
// Additional filtering by context values or translations could be applied.
func (m *Checklist) Response(ctx context.Context) *ChecklistResponse {
	if m == nil {
		return nil
	}

	r := &ChecklistResponse{
		ID:        m.ID,
		AccountID: m.AccountID,
		Name:      m.Name,
		Status:    web.NewEnumResponse(ctx, m.Status, ChecklistStatus_ValuesInterface()...),
		CreatedAt: web.NewTimeResponse(ctx, m.CreatedAt),
		UpdatedAt: web.NewTimeResponse(ctx, m.UpdatedAt),
	}

	if m.ArchivedAt != nil && !m.ArchivedAt.Time.IsZero() {
		at := web.NewTimeResponse(ctx, m.ArchivedAt.Time)
		r.ArchivedAt = &at
	}

	return r
}

// Checklists a list of Checklists.
type Checklists []*Checklist

// Response transforms a list of Checklists to a list of ChecklistResponses.
func (m *Checklists) Response(ctx context.Context) []*ChecklistResponse {
	var l []*ChecklistResponse
	if m != nil && len(*m) > 0 {
		for _, n := range *m {
			l = append(l, n.Response(ctx))
		}
	}

	return l
}

// ChecklistCreateRequest contains information needed to create a new Checklist.
type ChecklistCreateRequest struct {
	AccountID string           `json:"account_id" validate:"required,uuid"  example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name      string           `json:"name" validate:"required"  example:"Rocket Launch"`
	Status    *ChecklistStatus `json:"status,omitempty" validate:"omitempty,oneof=active disabled" enums:"active,disabled" swaggertype:"string" example:"active"`
}

// ChecklistReadRequest defines the information needed to read a checklist.
type ChecklistReadRequest struct {
	ID              string `json:"id" validate:"required,uuid" example:"985f1746-1d9f-459f-a2d9-fc53ece5ae86"`
	IncludeArchived bool   `json:"include-archived" example:"false"`
}

// ChecklistUpdateRequest defines what information may be provided to modify an existing
// Checklist. All fields are optional so clients can send just the fields they want
// changed. It uses pointer fields so we can differentiate between a field that
// was not provided and a field that was provided as explicitly blank.
type ChecklistUpdateRequest struct {
	ID     string           `json:"id" validate:"required,uuid" example:"985f1746-1d9f-459f-a2d9-fc53ece5ae86"`
	Name   *string          `json:"name,omitempty" validate:"omitempty" example:"Rocket Launch to Moon"`
	Status *ChecklistStatus `json:"status,omitempty" validate:"omitempty,oneof=active disabled" enums:"active,disabled" swaggertype:"string" example:"disabled"`
}

// ChecklistArchiveRequest defines the information needed to archive a checklist. This will archive (soft-delete) the
// existing database entry.
type ChecklistArchiveRequest struct {
	ID string `json:"id" validate:"required,uuid" example:"985f1746-1d9f-459f-a2d9-fc53ece5ae86"`
}

// ChecklistDeleteRequest defines the information needed to delete a checklist.
type ChecklistDeleteRequest struct {
	ID string `json:"id" validate:"required,uuid" example:"985f1746-1d9f-459f-a2d9-fc53ece5ae86"`
}

// ChecklistFindRequest defines the possible options to search for checklists. By default
// archived checklist will be excluded from response.
type ChecklistFindRequest struct {
	Where           string        `json:"where" example:"name = ? and status = ?"`
	Args            []interface{} `json:"args" swaggertype:"array,string" example:"Moon Launch,active"`
	Order           []string      `json:"order" example:"created_at desc"`
	Limit           *uint         `json:"limit" example:"10"`
	Offset          *uint         `json:"offset" example:"20"`
	IncludeArchived bool          `json:"include-archived" example:"false"`
}

// ChecklistStatus represents the status of checklist.
type ChecklistStatus string

// ChecklistStatus values define the status field of checklist.
const (
	// ChecklistStatus_Active defines the status of active for checklist.
	ChecklistStatus_Active ChecklistStatus = "active"
	// ChecklistStatus_Disabled defines the status of disabled for checklist.
	ChecklistStatus_Disabled ChecklistStatus = "disabled"
)

// ChecklistStatus_Values provides list of valid ChecklistStatus values.
var ChecklistStatus_Values = []ChecklistStatus{
	ChecklistStatus_Active,
	ChecklistStatus_Disabled,
}

// ChecklistStatus_ValuesInterface returns the ChecklistStatus options as a slice interface.
func ChecklistStatus_ValuesInterface() []interface{} {
	var l []interface{}
	for _, v := range ChecklistStatus_Values {
		l = append(l, v.String())
	}
	return l
}

// Scan supports reading the ChecklistStatus value from the database.
func (s *ChecklistStatus) Scan(value interface{}) error {
	asBytes, ok := value.([]byte)
	if !ok {
		return errors.New("Scan source is not []byte")
	}

	*s = ChecklistStatus(string(asBytes))
	return nil
}

// Value converts the ChecklistStatus value to be stored in the database.
func (s ChecklistStatus) Value() (driver.Value, error) {
	v := validator.New()
	errs := v.Var(s, "required,oneof=active disabled")
	if errs != nil {
		return nil, errs
	}

	return string(s), nil
}

// String converts the ChecklistStatus value to a string.
func (s ChecklistStatus) String() string {
	return string(s)
}
