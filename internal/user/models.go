package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/notify"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/sudo-suhas/symcrypto"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Repository defines the required dependencies for User.
type Repository struct {
	DbConn *sqlx.DB
	ResetUrl func(string) string
	Notify notify.Email
	SecretKey string
}

// NewRepository creates a new Repository that defines dependencies for User.
func NewRepository(db *sqlx.DB, resetUrl func(string) string, notify notify.Email, secretKey string) *Repository {
	return &Repository{
		DbConn: db,
		ResetUrl: resetUrl,
		Notify: notify,
		SecretKey: secretKey,
	}
}

// User represents someone with access to our system.
type User struct {
	ID            string          `json:"id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	FirstName     string          `json:"first_name" validate:"required" example:"Gabi"`
	LastName      string          `json:"last_name" validate:"required" example:"May"`
	Email         string          `json:"email" validate:"required,email,unique" example:"gabi@geeksinthewoods.com"`
	PasswordSalt  string          `json:"-" validate:"required"`
	PasswordHash  []byte          `json:"-" validate:"required"`
	PasswordReset *sql.NullString `json:"-"`
	Timezone      *string         `json:"timezone" validate:"omitempty" example:"America/Anchorage"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	ArchivedAt    *pq.NullTime    `json:"archived_at,omitempty"`
}

// UserResponse represents someone with access to our system that is returned for display.
type UserResponse struct {
	ID         string               `json:"id" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	Name       string               `json:"name" example:"Gabi"`
	FirstName  string               `json:"first_name" example:"Gabi"`
	LastName   string               `json:"last_name" example:"May"`
	Email      string               `json:"email" example:"gabi@geeksinthewoods.com"`
	Timezone   string               `json:"timezone" example:"America/Anchorage"`
	CreatedAt  web.TimeResponse     `json:"created_at"`            // CreatedAt contains multiple format options for display.
	UpdatedAt  web.TimeResponse     `json:"updated_at"`            // UpdatedAt contains multiple format options for display.
	ArchivedAt *web.TimeResponse    `json:"archived_at,omitempty"` // ArchivedAt contains multiple format options for display.
	Gravatar   web.GravatarResponse `json:"gravatar"`
}

// Response transforms User and UserResponse that is used for display.
// Additional filtering by context values or translations could be applied.
func (m *User) Response(ctx context.Context) *UserResponse {
	if m == nil {
		return nil
	}

	r := &UserResponse{
		ID:        m.ID,
		Name:      m.FirstName + " " + m.LastName,
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Email:     m.Email,
		CreatedAt: web.NewTimeResponse(ctx, m.CreatedAt),
		UpdatedAt: web.NewTimeResponse(ctx, m.UpdatedAt),
		Gravatar:  web.NewGravatarResponse(ctx, m.Email),
	}

	if m.Timezone != nil {
		r.Timezone = *m.Timezone
	}

	if m.ArchivedAt != nil && !m.ArchivedAt.Time.IsZero() {
		at := web.NewTimeResponse(ctx, m.ArchivedAt.Time)
		r.ArchivedAt = &at
	}

	return r
}

func (m *UserResponse) UnmarshalBinary(data []byte) error {
	if data == nil || len(data) == 0 {
		return nil
	}
	// convert data to yours, let's assume its json data
	return json.Unmarshal(data, m)
}

func (m *UserResponse) MarshalBinary() ([]byte, error) {
	return json.Marshal(m)
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

// UserCreateRequest contains information needed to create a new User.
type UserCreateRequest struct {
	FirstName       string  `json:"first_name" validate:"required" example:"Gabi"`
	LastName        string  `json:"last_name" validate:"required" example:"May"`
	Email           string  `json:"email" validate:"required,email,unique" example:"gabi@geeksinthewoods.com"`
	Password        string  `json:"password" validate:"required" example:"SecretString"`
	PasswordConfirm string  `json:"password_confirm" validate:"required,eqfield=Password" example:"SecretString"`
	Timezone        *string `json:"timezone,omitempty" validate:"omitempty" example:"America/Anchorage"`
}

// UserCreateInviteRequest contains information needed to create a new User.
type UserCreateInviteRequest struct {
	Email string `json:"email" validate:"required,email,unique" example:"gabi@geeksinthewoods.com"`
}

// UserReadRequest defines the information needed to read an user.
type UserReadRequest struct {
	ID              string `json:"id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	IncludeArchived bool   `json:"include-archived" example:"false"`
}

// UserUpdateRequest defines what information may be provided to modify an existing
// User. All fields are optional so clients can send just the fields they want
// changed. It uses pointer fields so we can differentiate between a field that
// was not provided and a field that was provided as explicitly blank. Normally
// we do not want to use pointers to basic types but we make exceptions around
// marshalling/unmarshalling.
type UserUpdateRequest struct {
	ID        string  `json:"id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	FirstName *string `json:"first_name,omitempty" validate:"omitempty" example:"Gabi May Not"`
	LastName  *string `json:"last_name,omitempty" validate:"omitempty" example:"Gabi May Not"`
	Email     *string `json:"email,omitempty" validate:"omitempty,email,unique" example:"gabi.may@geeksinthewoods.com"`
	Timezone  *string `json:"timezone,omitempty" validate:"omitempty" example:"America/Anchorage"`
}

// UserUpdatePasswordRequest defines what information is required to update a user password.
type UserUpdatePasswordRequest struct {
	ID              string `json:"id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	Password        string `json:"password" validate:"required" example:"NeverTellSecret"`
	PasswordConfirm string `json:"password_confirm" validate:"required,eqfield=Password" example:"NeverTellSecret"`
}

// UserArchiveRequest defines the information needed to archive an user. This will archive (soft-delete) the
// existing database entry.
type UserArchiveRequest struct {
	ID    string `json:"id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	force bool
}

// UserRestoreRequest defines the information needed to restore an user.
type UserRestoreRequest struct {
	ID string `json:"id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
}

// UserDeleteRequest defines the information needed to delete a user.
type UserDeleteRequest struct {
	ID    string `json:"id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	force bool
}

// UserFindRequest defines the possible options to search for users. By default
// archived users will be excluded from response.
type UserFindRequest struct {
	Where           string        `json:"where" example:"name = ? and email = ?"`
	Args            []interface{} `json:"args" swaggertype:"array,string" example:"Company Name,gabi.may@geeksinthewoods.com"`
	Order           []string      `json:"order" example:"created_at desc"`
	Limit           *uint         `json:"limit" example:"10"`
	Offset          *uint         `json:"offset" example:"20"`
	IncludeArchived bool          `json:"include-archived" example:"false"`
}

// UserResetPasswordRequest defines the fields need to reset a user password.
type UserResetPasswordRequest struct {
	Email string        `json:"email" validate:"required,email" example:"gabi.may@geeksinthewoods.com"`
	TTL   time.Duration `json:"ttl,omitempty" `
}

// ResetHash
type ResetHash struct {
	ResetID   string `json:"reset_id" validate:"required" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	CreatedAt int    `json:"created_at" validate:"required"`
	ExpiresAt int    `json:"expires_at" validate:"required"`
	RequestIP string `json:"request_ip" validate:"required,ip" example:"69.56.104.36"`
}

// UserResetConfirmRequest defines the fields need to reset a user password.
type UserResetConfirmRequest struct {
	ResetHash       string `json:"reset_hash" validate:"required" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	Password        string `json:"password" validate:"required" example:"SecretString"`
	PasswordConfirm string `json:"password_confirm" validate:"required,eqfield=Password" example:"SecretString"`
}

// NewResetHash generates a new encrypt reset hash that is web safe for use in URLs.
func NewResetHash(ctx context.Context, secretKey, resetId, requestIp string, ttl time.Duration, now time.Time) (string, error) {

	// Generate a string that embeds additional information.
	hashPts := []string{
		resetId,
		strconv.Itoa(int(now.UTC().Unix())),
		strconv.Itoa(int(now.UTC().Add(ttl).Unix())),
		requestIp,
	}
	hashStr := strings.Join(hashPts, "|")

	// This returns the nonce appended with the encrypted string.
	crypto, err := symcrypto.New(secretKey)
	if err != nil {
		return "", errors.WithStack(err)
	}
	encrypted, err := crypto.Encrypt(hashStr)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return encrypted, nil
}

// ParseResetHash extracts the details encrypted in the hash string.
func ParseResetHash(ctx context.Context, secretKey string, str string, now time.Time) (*ResetHash, error) {

	crypto, err := symcrypto.New(secretKey)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	hashStr, err := crypto.Decrypt(str)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	hashPts := strings.Split(hashStr, "|")

	var hash ResetHash
	if len(hashPts) == 4 {
		hash.ResetID = hashPts[0]
		hash.CreatedAt, _ = strconv.Atoi(hashPts[1])
		hash.ExpiresAt, _ = strconv.Atoi(hashPts[2])
		hash.RequestIP = hashPts[3]
	}

	// Validate the hash.
	err = webcontext.Validator().StructCtx(ctx, hash)
	if err != nil {
		return nil, err
	}

	if int64(hash.ExpiresAt) < now.UTC().Unix() {
		err = errors.WithMessage(ErrResetExpired, "Password reset has expired.")
		return nil, err
	}

	return &hash, nil
}
