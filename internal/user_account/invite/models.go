package invite

import (
	"context"
	"strconv"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"github.com/pkg/errors"
	"github.com/sudo-suhas/symcrypto"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
)

// SendUserInvitesRequest defines the data needed to make an invite request.
type SendUserInvitesRequest struct {
	AccountID string                         `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	UserID    string                         `json:"user_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
	Emails    []string                       `json:"emails" validate:"required,dive,email"`
	Roles     []user_account.UserAccountRole `json:"roles" validate:"required"`
	TTL       time.Duration                  `json:"ttl,omitempty" `
}

// InviteHash
type InviteHash struct {
	UserID    string `json:"user_id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	CreatedAt int    `json:"created_at" validate:"required"`
	ExpiresAt int    `json:"expires_at" validate:"required"`
	RequestIP string `json:"request_ip" validate:"required,ip" example:"69.56.104.36"`
}

// AcceptInviteRequest defines the fields need to complete an invite request.
type AcceptInviteRequest struct {
	InviteHash      string  `json:"invite_hash" validate:"required" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	Email      string            `json:"email" validate:"required,email" example:"gabi@geeksinthewoods.com"`
	FirstName       string  `json:"first_name" validate:"required" example:"Gabi"`
	LastName        string  `json:"last_name" validate:"required" example:"May"`
	Password        string  `json:"password" validate:"required" example:"SecretString"`
	PasswordConfirm string  `json:"password_confirm" validate:"required,eqfield=Password" example:"SecretString"`
	Timezone        *string `json:"timezone,omitempty" validate:"omitempty" example:"America/Anchorage"`
}

// NewInviteHash generates a new encrypt invite hash that is web safe for use in URLs.
func NewInviteHash(ctx context.Context, secretKey string, userID, requestIp string, ttl time.Duration, now time.Time) (string, error) {
	// Generate a string that embeds additional information.
	hashPts := []string{
		userID,
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

// ParseInviteHash extracts the details encrypted in the hash string.
func ParseInviteHash(ctx context.Context, secretKey string, str string, now time.Time) (*InviteHash, error) {
	crypto, err := symcrypto.New(secretKey)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	hashStr, err := crypto.Decrypt(str)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	hashPts := strings.Split(hashStr, "|")

	var hash InviteHash
	if len(hashPts) == 4 {
		hash.UserID = hashPts[0]
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
		err = errors.WithMessage(ErrInviteExpired, "Invite has expired.")
		return nil, err
	}

	return &hash, nil
}
