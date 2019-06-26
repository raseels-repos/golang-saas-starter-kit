package web

import (
	"encoding/json"
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
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	f := func(fl validator.FieldLevel) bool {
		return true
	}
	validate.RegisterValidation("unique", f)
}

// Decode reads the body of an HTTP request looking for a JSON document. The
// body is decoded into the provided value.
//
// If the provided value is a struct then it is checked for validation tags.
func Decode(r *http.Request, val interface{}) error {

	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(val); err != nil {
			return NewRequestError(err, http.StatusBadRequest)
		}
	} else {
		decoder := schema.NewDecoder()
		if err := decoder.Decode(val, r.URL.Query()); err != nil {
			return NewRequestError(err, http.StatusBadRequest)
		}
	}

	if err := validate.Struct(val); err != nil {

		// Use a type assertion to get the real error value.
		verrors, ok := err.(validator.ValidationErrors)
		if !ok {
			return err
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
		}
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
