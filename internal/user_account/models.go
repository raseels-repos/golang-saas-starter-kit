package user_account

import (
	"context"
	"strings"
	"time"

	"database/sql/driver"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// Repository defines the required dependencies for UserAccount.
type Repository struct {
	DbConn *sqlx.DB
}

// NewRepository creates a new Repository that defines dependencies for UserAccount.
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{
		DbConn: db,
	}
}

// UserAccount defines the one to many relationship of an user to an account. This
// will enable a single user access to multiple accounts without having duplicate
// users. Each association of a user to an account has a set of roles and a status
// defined for the user. The roles will be applied to enforce ACLs across the
// application. The status will allow users to be managed on by account with users
// being global to the application.
type UserAccount struct {
	//ID         string            `json:"id" validate:"required,uuid" example:"72938896-a998-4258-a17b-6418dcdb80e3"`
	UserID     string            `json:"user_id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	AccountID  string            `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Roles      UserAccountRoles  `json:"roles" validate:"required,dive,oneof=admin user" enums:"admin,user" swaggertype:"array,string" example:"admin"`
	Status     UserAccountStatus `json:"status" validate:"omitempty,oneof=active invited disabled" enums:"active,invited,disabled" swaggertype:"string" example:"active"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	ArchivedAt *pq.NullTime      `json:"archived_at,omitempty"`
}

// UserAccountResponse defines the one to many relationship of an user to an account that is returned for display.
type UserAccountResponse struct {
	//ID         string            `json:"id" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	UserID     string                `json:"user_id" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	AccountID  string                `json:"account_id" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Roles      web.EnumMultiResponse `json:"roles" validate:"required,dive,oneof=admin user" enums:"admin,user" swaggertype:"array,string" example:"admin"`
	Status     web.EnumResponse      `json:"status"`                // Status is enum with values [active, invited, disabled].
	CreatedAt  web.TimeResponse      `json:"created_at"`            // CreatedAt contains multiple format options for display.
	UpdatedAt  web.TimeResponse      `json:"updated_at"`            // UpdatedAt contains multiple format options for display.
	ArchivedAt *web.TimeResponse     `json:"archived_at,omitempty"` // ArchivedAt contains multiple format options for display.
}

// Response transforms UserAccount and UserAccountResponse that is used for display.
// Additional filtering by context values or translations could be applied.
func (m *UserAccount) Response(ctx context.Context) *UserAccountResponse {
	if m == nil {
		return nil
	}

	r := &UserAccountResponse{
		//ID:        m.ID,
		UserID:    m.UserID,
		AccountID: m.AccountID,
		Status:    web.NewEnumResponse(ctx, m.Status, UserAccountStatus_ValuesInterface()...),
		CreatedAt: web.NewTimeResponse(ctx, m.CreatedAt),
		UpdatedAt: web.NewTimeResponse(ctx, m.UpdatedAt),
	}

	var selectedRoles []interface{}
	for _, r := range m.Roles {
		selectedRoles = append(selectedRoles, r.String())
	}
	r.Roles = web.NewEnumMultiResponse(ctx, selectedRoles, UserAccountRole_ValuesInterface()...)

	if m.ArchivedAt != nil && !m.ArchivedAt.Time.IsZero() {
		at := web.NewTimeResponse(ctx, m.ArchivedAt.Time)
		r.ArchivedAt = &at
	}

	return r
}

// HasRole checks if the entry has a role.
func (m *UserAccount) HasRole(role UserAccountRole) bool {
	if m == nil {
		return false
	}
	for _, r := range m.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// UserAccounts a list of UserAccounts.
type UserAccounts []*UserAccount

// Response transforms a list of UserAccounts to a list of UserAccountResponses.
func (m *UserAccounts) Response(ctx context.Context) []*UserAccountResponse {
	var l []*UserAccountResponse
	if m != nil && len(*m) > 0 {
		for _, n := range *m {
			l = append(l, n.Response(ctx))
		}
	}

	return l
}

// UserAccountCreateRequest defines the information is needed to associate a user to an
// account. Users are global to the application and each users access can be managed
// on an account level. If a current entry exists in the database but is archived,
// it will be un-archived.
type UserAccountCreateRequest struct {
	UserID    string             `json:"user_id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	AccountID string             `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Roles     UserAccountRoles   `json:"roles" validate:"required,dive,oneof=admin user" enums:"admin,user" swaggertype:"array,string" example:"admin"`
	Status    *UserAccountStatus `json:"status,omitempty" validate:"omitempty,oneof=active invited disabled" enums:"active,invited,disabled" swaggertype:"string" example:"active"`
}

// UserAccountReadRequest defines the information needed to read a user account.
type UserAccountReadRequest struct {
	UserID          string `json:"user_id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	AccountID       string `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	IncludeArchived bool   `json:"include-archived" example:"false"`
}

// UserAccountUpdateRequest defines the information needed to update the roles or the
// status for an existing user account.
type UserAccountUpdateRequest struct {
	UserID    string             `json:"user_id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	AccountID string             `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Roles     *UserAccountRoles  `json:"roles,omitempty" validate:"omitempty,dive,oneof=admin user" enums:"admin,user" swaggertype:"array,string" example:"user"`
	Status    *UserAccountStatus `json:"status,omitempty" validate:"omitempty,oneof=active invited disabled" enums:"active,invited,disabled" swaggertype:"string" example:"disabled"`
	unArchive bool               `json:"-"` // Internal use only.
}

// UserAccountArchiveRequest defines the information needed to remove an existing account
// for a user. This will archive (soft-delete) the existing database entry.
type UserAccountArchiveRequest struct {
	UserID    string `json:"user_id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	AccountID string `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
}

// UserAccountDeleteRequest defines the information needed to delete an existing account
// for a user. This will hard delete the existing database entry.
type UserAccountDeleteRequest struct {
	UserID    string `json:"user_id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	AccountID string `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
}

// UserAccountFindRequest defines the possible options to search for users accounts.
// By default archived user accounts will be excluded from response.
type UserAccountFindRequest struct {
	Where           string        `json:"where" example:"user_id = ? and account_id = ?"`
	Args            []interface{} `json:"args" swaggertype:"array,string" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2,c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Order           []string      `json:"order" example:"created_at desc"`
	Limit           *uint         `json:"limit" example:"10"`
	Offset          *uint         `json:"offset" example:"20"`
	IncludeArchived bool          `json:"include-archived" example:"false"`
}

// UserAccountStatus represents the status of a user for an account.
type UserAccountStatus string

// UserAccountStatus values define the status field of a user account.
const (
	// UserAccountStatus_Active defines the state when a user can access an account.
	UserAccountStatus_Active UserAccountStatus = "active"
	// UserAccountStatus_Invited defined the state when a user has been invited to an
	// account.
	UserAccountStatus_Invited UserAccountStatus = "invited"
	// UserAccountStatus_Disabled defines the state when a user has been disabled from
	// accessing an account.
	UserAccountStatus_Disabled UserAccountStatus = "disabled"
)

// UserAccountStatus_Values provides list of valid UserAccountStatus values.
var UserAccountStatus_Values = []UserAccountStatus{
	UserAccountStatus_Active,
	UserAccountStatus_Invited,
	UserAccountStatus_Disabled,
}

// UserAccountStatus_ValuesInterface returns the UserAccountStatus options as a slice interface.
func UserAccountStatus_ValuesInterface() []interface{} {
	var l []interface{}
	for _, v := range UserAccountStatus_Values {
		l = append(l, v.String())
	}
	return l
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

	errs := v.Var(s, "required,oneof=active invited disabled")
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

// UserAccountRole values define the role field of a user account.
const (
	// UserAccountRole_Admin defines the state of a user when they have admin
	// privileges for accessing an account. This role provides a user with full
	// access to an account.
	UserAccountRole_Admin UserAccountRole = auth.RoleAdmin
	// UserAccountRole_User defines the state of a user when they have basic
	// privileges for accessing an account. This role provies a user with the most
	// limited access to an account.
	UserAccountRole_User UserAccountRole = auth.RoleUser
)

// UserAccountRole_Values provides list of valid UserAccountRole values.
var UserAccountRole_Values = []UserAccountRole{
	UserAccountRole_Admin,
	UserAccountRole_User,
}

// UserAccountRole_ValuesInterface returns the UserAccountRole options as a slice interface.
func UserAccountRole_ValuesInterface() []interface{} {
	var l []interface{}
	for _, v := range UserAccountRole_Values {
		l = append(l, v.String())
	}
	return l
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

// User represents someone with access to our system.
type User struct {
	ID         string            `json:"id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	Name       string            `json:"name"  validate:"required" example:"Gabi May"`
	FirstName  string            `json:"first_name" validate:"required" example:"Gabi"`
	LastName   string            `json:"last_name" validate:"required" example:"May"`
	Email      string            `json:"email" validate:"required,email,unique" example:"gabi@geeksinthewoods.com"`
	Timezone   *string           `json:"timezone" validate:"omitempty" example:"America/Anchorage"`
	AccountID  string            `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Roles      UserAccountRoles  `json:"roles" validate:"required,dive,oneof=admin user" enums:"admin,user" swaggertype:"array,string" example:"admin"`
	Status     UserAccountStatus `json:"status" validate:"omitempty,oneof=active invited disabled" enums:"active,invited,disabled" swaggertype:"string" example:"active"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	ArchivedAt *pq.NullTime      `json:"archived_at,omitempty"`
}

// UserResponse represents someone with access to our system that is returned for display.
type UserResponse struct {
	ID         string                `json:"id" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	Name       string                `json:"name" example:"Gabi"`
	FirstName  string                `json:"first_name" example:"Gabi"`
	LastName   string                `json:"last_name" example:"May"`
	Email      string                `json:"email" example:"gabi@geeksinthewoods.com"`
	Timezone   string                `json:"timezone" example:"America/Anchorage"`
	AccountID  string                `json:"account_id" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Roles      web.EnumMultiResponse `json:"roles" validate:"required,dive,oneof=admin user" enums:"admin,user" swaggertype:"array,string" example:"admin"`
	Status     web.EnumResponse      `json:"status"`                // Status is enum with values [active, invited, disabled].
	CreatedAt  web.TimeResponse      `json:"created_at"`            // CreatedAt contains multiple format options for display.
	UpdatedAt  web.TimeResponse      `json:"updated_at"`            // UpdatedAt contains multiple format options for display.
	ArchivedAt *web.TimeResponse     `json:"archived_at,omitempty"` // ArchivedAt contains multiple format options for display.
	Gravatar   web.GravatarResponse  `json:"gravatar"`
}

// Response transforms User and UserResponse that is used for display.
// Additional filtering by context values or translations could be applied.
func (m *User) Response(ctx context.Context) *UserResponse {
	if m == nil {
		return nil
	}

	r := &UserResponse{
		ID:        m.ID,
		Name:      m.Name,
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Email:     m.Email,
		AccountID: m.AccountID,
		Status:    web.NewEnumResponse(ctx, m.Status, UserAccountStatus_Values),
		CreatedAt: web.NewTimeResponse(ctx, m.CreatedAt),
		UpdatedAt: web.NewTimeResponse(ctx, m.UpdatedAt),
		Gravatar:  web.NewGravatarResponse(ctx, m.Email),
	}

	var selectedRoles []interface{}
	for _, r := range m.Roles {
		selectedRoles = append(selectedRoles, r.String())
	}
	r.Roles = web.NewEnumMultiResponse(ctx, selectedRoles, UserAccountRole_ValuesInterface()...)

	if m.Timezone != nil {
		r.Timezone = *m.Timezone
	}

	if strings.TrimSpace(r.Name) == "" {
		r.Name = r.Email
	}

	if m.ArchivedAt != nil && !m.ArchivedAt.Time.IsZero() {
		at := web.NewTimeResponse(ctx, m.ArchivedAt.Time)
		r.ArchivedAt = &at
	}

	return r
}

// Users a list of Users.
type Users []*User

// Response transforms a list of Users to a list of UserResponses.
func (m *Users) Response(ctx context.Context) []*UserResponse {
	var l []*UserResponse
	if m != nil && len(*m) > 0 {
		for _, n := range *m {
			l = append(l, n.Response(ctx))
		}
	}

	return l
}

// UserFindByAccountRequest defines the possible options to search for users by account ID.
// By default archived users will be excluded from response.
type UserFindByAccountRequest struct {
	AccountID       string        `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Where           string        `json:"where" example:"name = ? and email = ?"`
	Args            []interface{} `json:"args" swaggertype:"array,string" example:"Company Name,gabi.may@geeksinthewoods.com"`
	Order           []string      `json:"order" example:"created_at desc"`
	Limit           *uint         `json:"limit" example:"10"`
	Offset          *uint         `json:"offset" example:"20"`
	IncludeArchived bool          `json:"include-archived" example:"false"`
}
