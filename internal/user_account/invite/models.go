package invite

import (
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
)

// InviteUsersRequest defines the data needed to make an invite request.
type InviteUsersRequest struct {
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

// InviteAcceptRequest defines the fields need to complete an invite request.
type InviteAcceptRequest struct {
	InviteHash      string  `json:"invite_hash" validate:"required" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	FirstName       string  `json:"first_name" validate:"required" example:"Gabi"`
	LastName        string  `json:"last_name" validate:"required" example:"May"`
	Password        string  `json:"password" validate:"required" example:"SecretString"`
	PasswordConfirm string  `json:"password_confirm" validate:"required,eqfield=Password" example:"SecretString"`
	Timezone        *string `json:"timezone,omitempty" validate:"omitempty" example:"America/Anchorage"`
}
