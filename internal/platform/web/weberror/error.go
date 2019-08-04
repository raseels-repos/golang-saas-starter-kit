package weberror

import (
	"context"
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

// Error is used to pass an error during the request through the
// application with web specific context.
type Error struct {
	Err               error
	Status            int
	Fields            []FieldError
	Cause             error
	Message           string
	isValidationError bool
}

// FieldError is used to indicate an error with a specific request field.
type FieldError struct {
	Field     string      `json:"field"`
	FormField string      `json:"-"`
	Value     interface{} `json:"value"`
	Tag       string      `json:"tag"`
	Error     string      `json:"error"`
	Display   string      `json:"display"`
}

// NewError wraps a provided error with an HTTP status code. This
// function should be used when handlers encounter expected errors.
func NewError(ctx context.Context, er error, status int) error {
	webErr, ok := er.(*Error)
	if ok {
		return webErr
	}

	// If the error was of the type *Error, the handler has
	// a specific status code and error to return.
	webErr, ok = errors.Cause(er).(*Error)
	if ok {
		return webErr
	}

	// Ensure the error is not a validation error.
	if ne, ok := NewValidationError(ctx, er); ok {
		return ne
	}

	if er == webcontext.ErrContextRequired {
		return NewShutdownError(er.Error())
	}

	// If not, the handler sent any arbitrary error value so use 500.
	if status == 0 {
		status = http.StatusInternalServerError
	}

	cause := errors.Cause(er)
	if cause == nil {
		cause = er
	}

	return &Error{er, status, nil, cause, "", false}
}

// Error implements the error interface. It uses the default message of the
// wrapped error. This is what will be shown in the services' logs.
func (err *Error) Error() string {
	if err.Err != nil {
		return err.Err.Error()
	} else if err.Cause != nil {
		return err.Cause.Error()
	}
	return err.Message
}

// Display renders an error that can be returned as ErrorResponse to the user via the API.
func (er *Error) Response(ctx context.Context, htmlEntities bool) ErrorResponse {
	var r ErrorResponse

	r.StatusCode = er.Status

	if er.Message != "" {
		r.Error = er.Message
	} else {
		r.Error = http.StatusText(er.Status)
	}

	if len(er.Fields) > 0 {
		r.Fields = er.Fields
	}

	switch webcontext.ContextEnv(ctx) {
	case webcontext.Env_Dev, webcontext.Env_Stage:
		r.Details = fmt.Sprintf("%v", er.Err)

		if er.Cause != nil && er.Cause.Error() != er.Err.Error() {
			r.StackTrace = fmt.Sprintf("%+v", er.Cause)
		} else {
			r.StackTrace = fmt.Sprintf("%+v", er.Err)
		}
	}

	if htmlEntities {
		r.Details = strings.Replace(r.Details, "\n", "<br/>", -1)
		r.StackTrace = strings.Replace(r.StackTrace, "\n", "<br/>", -1)
	}

	return r
}

// ErrorResponse is the form used for API responses from failures in the API.
type ErrorResponse struct {
	StatusCode int          `json:"status_code"`
	Error      string       `json:"error"`
	Details    string       `json:"details,omitempty"`
	StackTrace string       `json:"stack_trace,omitempty"`
	Fields     []FieldError `json:"fields,omitempty"`
}

// String returns the ErrorResponse formatted as a string.
func (er ErrorResponse) String() string {
	str := er.Error

	if len(er.Fields) > 0 {
		for _, f := range er.Fields {
			str = str + "\t" + f.Error + "\n"
		}
	}

	return str
}

// NewErrorMessage wraps a provided error with an HTTP status code and message. The
// message value is given priority and returned as the error message.
func NewErrorMessage(ctx context.Context, er error, status int, msg string) error {
	return WithMessage(ctx, NewError(ctx, er, status), msg)
}

// WithMessage appends the error with a message.
func WithMessage(ctx context.Context, er error, msg string) error {
	weberr := NewError(ctx, er, 0).(*Error)
	weberr.Message = msg
	return weberr
}

// SessionFlashError
func SessionFlashError(ctx context.Context, er error) {

	webErr := NewError(ctx, er, 0).(*Error)

	resp := webErr.Response(ctx, true)

	msg := webcontext.FlashMsg{
		Type:  webcontext.FlashType_Error,
		Title: resp.Error,
	}

	if webErr.isValidationError {
		for _, f := range resp.Fields {
			msg.Items = append(msg.Items, f.Display)
		}
	} else {
		msg.Text = resp.Details
		msg.Details = resp.StackTrace
	}

	if pts := strings.Split(msg.Details, "<br/>"); len(pts) > 3 {
		msg.Details = strings.Join(pts[0:3], "<br/>")
	}

	webcontext.SessionAddFlash(ctx, msg)
}
