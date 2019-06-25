package signup

import (
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user"
)

// SignupRequest contains information needed perform signup.
type SignupRequest struct {
	Account account.AccountCreateRequest `json:"account" validate:"required"`
	User    user.UserCreateRequest       `json:"user" validate:"required"`
}

// SignupResponse contains information needed perform signup.
type SignupResponse struct {
	Account *account.Account `json:"account"`
	User    *user.User       `json:"user"`
}
