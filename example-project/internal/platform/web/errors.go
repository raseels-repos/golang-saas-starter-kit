package web

import (
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
	"net/http"
)

// FieldError is used to indicate an error with a specific request field.
type FieldError struct {
	Field string `json:"field"`
	Error string `json:"error"`
}

// ErrorResponse is the form used for API responses from failures in the API.
type ErrorResponse struct {
	Error  string       `json:"error"`
	Fields []FieldError `json:"fields,omitempty"`
}

// Error is used to pass an error during the request through the
// application with web specific context.
type Error struct {
	Err    error
	Status int
	Fields []FieldError
}

// NewRequestError wraps a provided error with an HTTP status code. This
// function should be used when handlers encounter expected errors.
func NewRequestError(err error, status int) error {

	// if its a validation error then
	if verr, ok := NewValidationError(err); ok {
		return verr
	}

	return &Error{err, status, nil}
}

// Error implements the error interface. It uses the default message of the
// wrapped error. This is what will be shown in the services' logs.
func (err *Error) Error() string {
	return err.Err.Error()
}

// shutdown is a type used to help with the graceful termination of the service.
type shutdown struct {
	Message string
}

// Error is the implementation of the error interface.
func (s *shutdown) Error() string {
	return s.Message
}

// NewShutdownError returns an error that causes the framework to signal
// a graceful shutdown.
func NewShutdownError(message string) error {
	return &shutdown{message}
}

// NewValidationError checks the error for validation errors and formats the correct response.
func NewValidationError(err error) (error, bool) {

	// Use a type assertion to get the real error value.
	verrors, ok := errors.Cause(err).(validator.ValidationErrors)
	if !ok {
		return err, false
	}

	// lang controls the language of the error messages. You could look at the
	// Accept-Language header if you intend to support multiple languages.
	lang, _ := translator.GetTranslator("en")

	var fields []FieldError
	for _, verror := range verrors {
		field := FieldError{
			Field: verror.Field(),
			Error: verror.Translate(lang),
		}
		fields = append(fields, field)
	}

	return &Error{
		Err:    errors.New("field validation error"),
		Status: http.StatusBadRequest,
		Fields: fields,
	}, true
}

// IsShutdown checks to see if the shutdown error is contained
// in the specified error value.
func IsShutdown(err error) bool {
	if _, ok := errors.Cause(err).(*shutdown); ok {
		return true
	}
	return false
}
