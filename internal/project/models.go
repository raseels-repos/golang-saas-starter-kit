package project

import (
	"context"
	"database/sql/driver"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
	"time"
)

// Project represents a workflow.
type Project struct {
	ID         string        `json:"id" validate:"required,uuid" example:"985f1746-1d9f-459f-a2d9-fc53ece5ae86"`
	AccountID  string        `json:"account_id" validate:"required,uuid" truss:"api-create"`
	Name       string        `json:"name"  validate:"required" example:"Rocket Launch"`
	Status     ProjectStatus `json:"status" validate:"omitempty,oneof=active disabled" enums:"active,disabled" swaggertype:"string" example:"active"`
	CreatedAt  time.Time     `json:"created_at" truss:"api-read"`
	UpdatedAt  time.Time     `json:"updated_at" truss:"api-read"`
	ArchivedAt *pq.NullTime  `json:"archived_at,omitempty" truss:"api-hide"`
}

// ProjectResponse represents a workflow that is returned for display.
type ProjectResponse struct {
	ID         string            `json:"id" validate:"required,uuid" example:"985f1746-1d9f-459f-a2d9-fc53ece5ae86"`
	AccountID  string            `json:"account_id" validate:"required,uuid" truss:"api-create" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name       string            `json:"name"  validate:"required" example:"Rocket Launch"`
	Status     web.EnumResponse  `json:"status"`                // Status is enum with values [active, disabled].
	CreatedAt  web.TimeResponse  `json:"created_at"`            // CreatedAt contains multiple format options for display.
	UpdatedAt  web.TimeResponse  `json:"updated_at"`            // UpdatedAt contains multiple format options for display.
	ArchivedAt *web.TimeResponse `json:"archived_at,omitempty"` // ArchivedAt contains multiple format options for display.
}

// Response transforms Project and ProjectResponse that is used for display.
// Additional filtering by context values or translations could be applied.
func (m *Project) Response(ctx context.Context) *ProjectResponse {
	if m == nil {
		return nil
	}

	r := &ProjectResponse{
		ID:        m.ID,
		AccountID: m.AccountID,
		Name:      m.Name,
		Status:    web.NewEnumResponse(ctx, m.Status, ProjectStatus_Values),
		CreatedAt: web.NewTimeResponse(ctx, m.CreatedAt),
		UpdatedAt: web.NewTimeResponse(ctx, m.UpdatedAt),
	}

	if m.ArchivedAt != nil && !m.ArchivedAt.Time.IsZero() {
		at := web.NewTimeResponse(ctx, m.ArchivedAt.Time)
		r.ArchivedAt = &at
	}

	return r
}

// Projects a list of Projects.
type Projects []*Project

// Response transforms a list of Projects to a list of ProjectResponses.
func (m *Projects) Response(ctx context.Context) []*ProjectResponse {
	var l []*ProjectResponse
	if m != nil && len(*m) > 0 {
		for _, n := range *m {
			l = append(l, n.Response(ctx))
		}
	}

	return l
}

// ProjectCreateRequest contains information needed to create a new Project.
type ProjectCreateRequest struct {
	AccountID string         `json:"account_id" validate:"required,uuid"  example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Name      string         `json:"name" validate:"required"  example:"Rocket Launch"`
	Status    *ProjectStatus `json:"status,omitempty" validate:"omitempty,oneof=active disabled" enums:"active,disabled" swaggertype:"string" example:"active"`
}

// ProjectReadRequest defines the information needed to read a project.
type ProjectReadRequest struct {
	ID              string `json:"id" validate:"required,uuid" example:"985f1746-1d9f-459f-a2d9-fc53ece5ae86"`
	IncludeArchived bool   `json:"include-archived" example:"false"`
}

// ProjectUpdateRequest defines what information may be provided to modify an existing
// Project. All fields are optional so clients can send just the fields they want
// changed. It uses pointer fields so we can differentiate between a field that
// was not provided and a field that was provided as explicitly blank.
type ProjectUpdateRequest struct {
	ID     string         `json:"id" validate:"required,uuid" example:"985f1746-1d9f-459f-a2d9-fc53ece5ae86"`
	Name   *string        `json:"name,omitempty" validate:"omitempty" example:"Rocket Launch to Moon"`
	Status *ProjectStatus `json:"status,omitempty" validate:"omitempty,oneof=active disabled" enums:"active,disabled" swaggertype:"string" example:"disabled"`
}

// ProjectArchiveRequest defines the information needed to archive a project. This will archive (soft-delete) the
// existing database entry.
type ProjectArchiveRequest struct {
	ID string `json:"id" validate:"required,uuid" example:"985f1746-1d9f-459f-a2d9-fc53ece5ae86"`
}

// ProjectDeleteRequest defines the information needed to delete a project.
type ProjectDeleteRequest struct {
	ID string `json:"id" validate:"required,uuid" example:"985f1746-1d9f-459f-a2d9-fc53ece5ae86"`
}

// ProjectFindRequest defines the possible options to search for projects. By default
// archived project will be excluded from response.
type ProjectFindRequest struct {
	Where           *string       `json:"where" example:"name = ? and status = ?"`
	Args            []interface{} `json:"args" swaggertype:"array,string" example:"Moon Launch,active"`
	Order           []string      `json:"order" example:"created_at desc"`
	Limit           *uint         `json:"limit" example:"10"`
	Offset          *uint         `json:"offset" example:"20"`
	IncludeArchived bool          `json:"include-archived" example:"false"`
}

// ProjectStatus represents the status of project.
type ProjectStatus string

// ProjectStatus values define the status field of project.
const (
	// ProjectStatus_Active defines the status of active for project.
	ProjectStatus_Active ProjectStatus = "active"
	// ProjectStatus_Disabled defines the status of disabled for project.
	ProjectStatus_Disabled ProjectStatus = "disabled"
)

// ProjectStatus_Values provides list of valid ProjectStatus values.
var ProjectStatus_Values = []ProjectStatus{
	ProjectStatus_Active,
	ProjectStatus_Disabled,
}

// Scan supports reading the ProjectStatus value from the database.
func (s *ProjectStatus) Scan(value interface{}) error {
	asBytes, ok := value.([]byte)
	if !ok {
		return errors.New("Scan source is not []byte")
	}

	*s = ProjectStatus(string(asBytes))
	return nil
}

// Value converts the ProjectStatus value to be stored in the database.
func (s ProjectStatus) Value() (driver.Value, error) {
	v := validator.New()
	errs := v.Var(s, "required,oneof=active disabled")
	if errs != nil {
		return nil, errs
	}

	return string(s), nil
}

// String converts the ProjectStatus value to a string.
func (s ProjectStatus) String() string {
	return string(s)
}
