package logger

import (
	"context"
	"fmt"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
)

// WithContext manual injects context values to log message including Trace ID
func WithContext(ctx context.Context, msg string) string {
	v, ok := ctx.Value(webcontext.KeyValues).(*webcontext.Values)
	if !ok {
		return msg
	}

	cm := fmt.Sprintf("dd.trace_id=%d dd.span_id=%d", v.TraceID, v.SpanID)

	return cm + ": " + msg
}
