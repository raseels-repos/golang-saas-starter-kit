package user_auth

import (
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
)

// AuthenticateRequest defines what information is required to authenticate a user.
type AuthenticateRequest struct {
	Email     string `json:"email" validate:"required,email" example:"gabi.may@geeksinthewoods.com"`
	Password  string `json:"password" validate:"required" example:"NeverTellSecret"`
	AccountID string `json:"account_id" validate:"omitempty,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
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
	Expiry time.Time     `json:"expiry,omitempty"`
	TTL    time.Duration `json:"ttl,omitempty"`
	// contains filtered or unexported fields
	claims auth.Claims `json:"-"`
	// UserId is the ID of the user authenticated.
	UserID string `json:"user_id" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	// AccountID is the ID of the account for the user authenticated.
	AccountID string `json:"account_id"example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
}

// SwitchAccountRequest defines the information for the current user to switch between their accounts
type SwitchAccountRequest struct {
	AccountID string `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
}

// VirtualLoginRequest defines the information virtual login to a user / account.
type VirtualLoginRequest struct {
	UserID    string `json:"user_id" validate:"required,uuid" example:"d69bdef7-173f-4d29-b52c-3edc60baf6a2"`
	AccountID string `json:"account_id" validate:"required,uuid" example:"c4653bf9-5978-48b7-89c5-95704aebb7e2"`
}

// AuthorizationHeader returns the header authorization value.
func (t Token) AuthorizationHeader() string {
	return "Bearer " + t.AccessToken
}

// TokenGenerator is the behavior we need in our Authenticate to generate tokens for
// authenticated users.
type TokenGenerator interface {
	GenerateToken(auth.Claims) (string, error)
	ParseClaims(string) (auth.Claims, error)
}
