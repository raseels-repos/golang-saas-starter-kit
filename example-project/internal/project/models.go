package project

import (
	"database/sql/driver"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
	"time"
)

// Project represents a workflow.
type Project struct {
	ID         string        `json:"id" validate:"required,uuid"`
	AccountID  string        `json:"account_id" validate:"required,uuid" truss:"api-create"`
	Name       string        `json:"name"  validate:"required"`
	Status     ProjectStatus `json:"status" validate:"omitempty,oneof=active disabled"`
	CreatedAt  time.Time     `json:"created_at" truss:"api-read"`
	UpdatedAt  time.Time     `json:"updated_at" truss:"api-read"`
	ArchivedAt pq.NullTime   `json:"archived_at" truss:"api-hide"`
}

// ProjectCreateRequest contains information needed to create a new Project.
type ProjectCreateRequest struct {
	AccountID string         `json:"account_id" validate:"required,uuid"`
	Name      string         `json:"name" validate:"required"`
	Status    *ProjectStatus `json:"status" validate:"omitempty,oneof=active disabled"`
}

// ProjectUpdateRequest defines what information may be provided to modify an existing
// Project. All fields are optional so clients can send just the fields they want
// changed. It uses pointer fields so we can differentiate between a field that
// was not provided and a field that was provided as explicitly blank.
type ProjectUpdateRequest struct {
	ID     string         `json:"id" validate:"required,uuid"`
	Name   *string        `json:"name" validate:"omitempty"`
	Status *ProjectStatus `json:"status" validate:"omitempty,oneof=active disabled"`
}

// ProjectFindRequest defines the possible options to search for projects. By default
// archived project will be excluded from response.
type ProjectFindRequest struct {
	Where            *string
	Args             []interface{}
	Order            []string
	Limit            *uint
	Offset           *uint
	IncludedArchived bool
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
