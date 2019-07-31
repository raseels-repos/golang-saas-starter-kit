package weberror

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"net/http"

	"github.com/pkg/errors"
)

// Error is used to pass an error during the request through the
// application with web specific context.
type Error struct {
	Err     error
	Status  int
	Fields  []FieldError
	Cause   error
	Message string
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

	return &Error{er, status, nil, cause, ""}
}

// Error implements the error interface. It uses the default message of the
// wrapped error. This is what will be shown in the services' logs.
func (err *Error) Error() string {
	return err.Err.Error()
}

// Display renders an error that can be returned as ErrorResponse to the user via the API.
func (er *Error) Display(ctx context.Context) ErrorResponse {
	var r ErrorResponse

	if er.Message != "" {
		r.Error = er.Message
	} else {
		r.Error = er.Error()
	}

	if len(er.Fields) > 0 {
		r.Fields = er.Fields
	}

	return r
}

// ErrorResponse is the form used for API responses from failures in the API.
type ErrorResponse struct {
	Error  string       `json:"error"`
	Fields []FieldError `json:"fields,omitempty"`
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
