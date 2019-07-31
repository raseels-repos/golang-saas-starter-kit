package webcontext

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// ctxKey represents the type of value for the context key.
type ctxKey int

// KeyValues is how request values or stored/retrieved.
const KeyValues ctxKey = 1

var ErrContextRequired = errors.New("web value missing from context")

// Values represent state for each request.
type Values struct {
	Now        time.Time
	TraceID    uint64
	SpanID     uint64
	StatusCode int
	Env        Env
}

func ContextValues(ctx context.Context) (*Values, error) {
	// If the context is missing this value, request the service
	// to be shutdown gracefully.
	v, ok := ctx.Value(KeyValues).(*Values)
	if !ok {
		e := Values{}
		return &e, ErrContextRequired
	}

	return v, nil
}

type Env = string

var (
	Env_Dev   Env = "dev"
	Env_Stage Env = "stage"
	Env_Prod  Env = "prod"
)

func ContextEnv(ctx context.Context) string {
	cv := ctx.Value(KeyValues).(*Values)
	if cv != nil {
		return cv.Env
	}
	return ""
}
