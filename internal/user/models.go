package user

import (
	"context"
	"database/sql"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"time"

	"github.com/lib/pq"
)

// User represents someone with access to our system.
type User struct {
	ID            string          `json:"id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	Name          string          `json:"name" validate:"required" example:"Gabi May"`
	Email         string          `json:"email" validate:"required,email,unique" example:"gabi@geeksinthewoods.com"`
	PasswordSalt  string          `json:"-" validate:"required"`
	PasswordHash  []byte          `json:"-" validate:"required"`
	PasswordReset *sql.NullString `json:"-"`
	Timezone      string          `json:"timezone" validate:"omitempty" example:"America/Anchorage"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	ArchivedAt    *pq.NullTime    `json:"archived_at,omitempty"`
}

// UserResponse represents someone with access to our system that is returned for display.
type UserResponse struct {
	ID         string            `json:"id" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	Name       string            `json:"name" example:"Gabi May"`
	Email      string            `json:"email" example:"gabi@geeksinthewoods.com"`
	Timezone   string            `json:"timezone" example:"America/Anchorage"`
	CreatedAt  web.TimeResponse  `json:"created_at"`            // CreatedAt contains multiple format options for display.
	UpdatedAt  web.TimeResponse  `json:"updated_at"`            // UpdatedAt contains multiple format options for display.
	ArchivedAt *web.TimeResponse `json:"archived_at,omitempty"` // ArchivedAt contains multiple format options for display.
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
		Email:     m.Email,
		Timezone:  m.Timezone,
		CreatedAt: web.NewTimeResponse(ctx, m.CreatedAt),
		UpdatedAt: web.NewTimeResponse(ctx, m.UpdatedAt),
	}

	if m.ArchivedAt != nil && !m.ArchivedAt.Time.IsZero() {
		at := web.NewTimeResponse(ctx, m.ArchivedAt.Time)
		r.ArchivedAt = &at
	}

	return r
}

// UserCreateRequest contains information needed to create a new User.
type UserCreateRequest struct {
	Name            string  `json:"name" validate:"required" example:"Gabi May"`
	Email           string  `json:"email" validate:"required,email,unique" example:"gabi@geeksinthewoods.com"`
	Password        string  `json:"password" validate:"required" example:"SecretString"`
	PasswordConfirm string  `json:"password_confirm" validate:"eqfield=Password" example:"SecretString"`
	Timezone        *string `json:"timezone,omitempty" validate:"omitempty" example:"America/Anchorage"`
}

// UserUpdateRequest defines what information may be provided to modify an existing
// User. All fields are optional so clients can send just the fields they want
// changed. It uses pointer fields so we can differentiate between a field that
// was not provided and a field that was provided as explicitly blank. Normally
// we do not want to use pointers to basic types but we make exceptions around
// marshalling/unmarshalling.
type UserUpdateRequest struct {
	ID       string  `json:"id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	Name     *string `json:"name,omitempty" validate:"omitempty" example:"Gabi May Not"`
	Email    *string `json:"email,omitempty" validate:"omitempty,email,unique" example:"gabi.may@geeksinthewoods.com"`
	Timezone *string `json:"timezone,omitempty" validate:"omitempty" example:"America/Anchorage"`
}

// UserUpdatePasswordRequest defines what information is required to update a user password.
type UserUpdatePasswordRequest struct {
	ID              string `json:"id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	Password        string `json:"password" validate:"required" example:"NeverTellSecret"`
	PasswordConfirm string `json:"password_confirm" validate:"omitempty,eqfield=Password" example:"NeverTellSecret"`
}

// UserArchiveRequest defines the information needed to archive an user. This will archive (soft-delete) the
// existing database entry.
type UserArchiveRequest struct {
	ID string `json:"id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
}

// UserFindRequest defines the possible options to search for users. By default
// archived users will be excluded from response.
type UserFindRequest struct {
	Where            *string       `json:"where" example:"name = ? and email = ?"`
	Args             []interface{} `json:"args" swaggertype:"array,string" example:"Company Name,gabi.may@geeksinthewoods.com"`
	Order            []string      `json:"order" example:"created_at desc"`
	Limit            *uint         `json:"limit" example:"10"`
	Offset           *uint         `json:"offset" example:"20"`
	IncludedArchived bool          `json:"included-archived" example:"false"`
}

// Token is the payload we deliver to users when they authenticate.
type Token struct {
	// AccessToken is the token that authorizes and authenticates
	// the requests.
	AccessToken string `json:"access_token"`
	// TokenType is the type of token.
	// The Type method returns either this or "Bearer", the default.
	TokenType string `json:"token_type,omitempty"`
	// Expiry is the optional expiration time of the access token.
	//
	// If zero, TokenSource implementations will reuse the same
	// token forever and RefreshToken or equivalent
	// mechanisms for that TokenSource will not be used.
	Expiry time.Time `json:"expiry,omitempty"`
	// contains filtered or unexported fields
	claims auth.Claims `json:"-"`
}
