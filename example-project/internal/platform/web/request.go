package web

import (
	"encoding/json"
	"net"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/gorilla/schema"
	"github.com/pkg/errors"
	"github.com/xwb1989/sqlparser"
	"github.com/xwb1989/sqlparser/dependency/querypb"
	"gopkg.in/go-playground/validator.v9"
	en_translations "gopkg.in/go-playground/validator.v9/translations/en"
)

// Headers
const (
	HeaderUpgrade = "Upgrade"
	HeaderXForwardedFor       = "X-Forwarded-For"
	HeaderXForwardedProto     = "X-Forwarded-Proto"
	HeaderXForwardedProtocol  = "X-Forwarded-Protocol"
	HeaderXForwardedSsl       = "X-Forwarded-Ssl"
	HeaderXUrlScheme          = "X-Url-Scheme"
	HeaderXHTTPMethodOverride = "X-HTTP-Method-Override"
	HeaderXRealIP             = "X-Real-IP"
	HeaderXRequestID          = "X-Request-ID"
	HeaderXRequestedWith      = "X-Requested-With"
	HeaderServer              = "Server"
	HeaderOrigin              = "Origin"
)

// validate holds the settings and caches for validating request struct values.
var validate = validator.New()

// translator is a cache of locale and translation information.
var translator *ut.UniversalTranslator

func init() {

	// Instantiate the english locale for the validator library.
	enLocale := en.New()

	// Create a value using English as the fallback locale (first argument).
	// Provide one or more arguments for additional supported locales.
	translator = ut.New(enLocale, enLocale)

	// Register the english error messages for validation errors.
	lang, _ := translator.GetTranslator("en")
	en_translations.RegisterDefaultTranslations(validate, lang)

	// Use JSON tag names for errors instead of Go struct names.
	validate = NewValidator()

	// Empty method that can be overwritten in business logic packages to prevent web.Decode from failing.
	f := func(fl validator.FieldLevel) bool {
		return true
	}
	validate.RegisterValidation("unique", f)
}

// NewValidator inits a new validator with custom settings.
func NewValidator() *validator.Validate {
	var v = validator.New()

	// Use JSON tag names for errors instead of Go struct names.
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	return v
}

// Decode reads the body of an HTTP request looking for a JSON document. The
// body is decoded into the provided value.
//
// If the provided value is a struct then it is checked for validation tags.
func Decode(r *http.Request, val interface{}) error {

	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(val); err != nil {
			err = errors.Wrap(err, "decode request body failed")
			return NewRequestError(err, http.StatusBadRequest)
		}
	} else {
		decoder := schema.NewDecoder()
		if err := decoder.Decode(val, r.URL.Query()); err != nil {
			err = errors.Wrap(err, "decode request query failed")
			return NewRequestError(err, http.StatusBadRequest)
		}
	}

	if err := validate.Struct(val); err != nil {
		verr, _ := NewValidationError(err)
		return verr
	}

	return nil
}

// ExtractWhereArgs extracts the sql args from where. This allows requests to accept sql queries for filters and
// then replaces the raw values with placeholders. The resulting query will then be executed with bind vars.
func ExtractWhereArgs(where string) (string, []interface{}, error) {
	// Create a full select sql query.
	query := "select `t` from test where " + where

	// Parse the query.
	stmt, err := sqlparser.Parse(query)
	if err != nil {
		return "", nil, errors.WithMessagef(err, "Failed to parse query - %s", where)
	}

	// Normalize changes the query statement to use bind values, and updates the bind vars to those values. The
	// supplied prefix is used to generate the bind var names.
	bindVars := make(map[string]*querypb.BindVariable)
	sqlparser.Normalize(stmt, bindVars, "redacted")

	// Loop through all the bind vars and append to the response args list.
	var vals []interface{}
	for _, bv := range bindVars {
		if bv.Values != nil {
			var l []interface{}
			for _, v := range bv.Values {
				l = append(l, string(v.Value))
			}
			vals = append(vals, l)
		} else {
			vals = append(vals, string(bv.Value))
		}
	}

	// Update the original query to include the redacted values.
	query = sqlparser.String(stmt)

	// Parse out the updated where.
	where = strings.Split(query, " where ")[1]

	return where, vals, nil
}

func RequestIsJson(r *http.Request) bool {
	if r == nil {
		return false
	}
	if v := r.Header.Get("Content-type"); v != "" {
		for _, hv := range strings.Split(v, ";") {
			if strings.ToLower(hv) == "application/json" {
				return true
			}
		}
	}

	if v := r.URL.Query().Get("ResponseFormat"); v != "" {
		if strings.ToLower(v) == "json" {
			return true
		}
	}

	if strings.HasSuffix(r.URL.Path, ".json") {
		return true
	}

	return false
}

func RequestIsTLS(r *http.Request) bool {
	return r.TLS != nil
}

func RequestIsWebSocket(r *http.Request) bool {
	upgrade := r.Header.Get(HeaderUpgrade)
	return strings.ToLower(upgrade) == "websocket"
}

func RequestScheme(r *http.Request) string {
	// Can't use `r.Request.URL.Scheme`
	// See: https://groups.google.com/forum/#!topic/golang-nuts/pMUkBlQBDF0
	if RequestIsTLS(r) {
		return "https"
	}
	if scheme := r.Header.Get(HeaderXForwardedProto); scheme != "" {
		return scheme
	}
	if scheme := r.Header.Get(HeaderXForwardedProtocol); scheme != "" {
		return scheme
	}
	if ssl := r.Header.Get(HeaderXForwardedSsl); ssl == "on" {
		return "https"
	}
	if scheme := r.Header.Get(HeaderXUrlScheme); scheme != "" {
		return scheme
	}
	return "http"
}

func RequestRealIP(r *http.Request) string {
	if ip := r.Header.Get(HeaderXForwardedFor); ip != "" {
		return strings.Split(ip, ", ")[0]
	}
	if ip := r.Header.Get(HeaderXRealIP); ip != "" {
		return ip
	}
	ra, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ra
}
