package session

import (
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
)

// ctxKey represents the type of value for the context key.
type ctxKey int

// Key is used to store/retrieve a Claims value from a context.Context.
const Key ctxKey = 1

// Session represents a user with authentication.
type Session struct {
	Claims auth.Claims `json:"claims"`
}
