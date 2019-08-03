package weberror

import (
	"context"
	"net/http"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// NewValidationError checks the error for validation errors and formats the correct response.
func NewValidationError(ctx context.Context, err error) (error, bool) {

	// Use a type assertion to get the real error value.
	verrors, ok := errors.Cause(err).(validator.ValidationErrors)
	if !ok {
		return err, false
	}

	// Load the translator from the context that will be set by query string or HTTP headers.
	trans := webcontext.ContextTranslator(ctx)

	//fields := make(map[string][]FieldError)
	var fields []FieldError
	for _, verror := range verrors {

		jsonKey := verror.Field()

		fieldName := jsonKey[2 : len(jsonKey)-2]
		localName, _ := trans.T(jsonKey)
		if localName == "" {
			localName = fieldName
		}

		namespace := strings.Replace(verror.Namespace(), "{{", "", -1)
		namespace = strings.Replace(namespace, "}}", "", -1)

		fieldErr := strings.Replace(verror.Translate(trans), jsonKey, localName, -1)
		fieldErr = strings.Replace(fieldErr, "{{", "", -1)
		fieldErr = strings.Replace(fieldErr, "}}", "", -1)

		field := FieldError{
			Field:     fieldName,
			Value:     verror.Value(),
			Tag:       verror.Tag(),
			Error:     fieldErr,
			FormField: FormField(verror.StructNamespace()),
			Display:   fieldErr,
		}

		//switch verror.Tag() {
		//case "required":
		//	field.Display = fmt.Sprintf("%s is required.", localName)
		//case "unique":
		//	field.Display = fmt.Sprintf("%s must be unique.", localName)
		//}

		/*
			fmt.Println("field", field.Error)
			fmt.Println("formField", field.FormField)
			fmt.Println("Namespace: " + verror.Namespace())
			fmt.Println("Field: " + verror.Field())
			fmt.Println("StructNamespace: " + verror.StructNamespace()) // can differ when a custom TagNameFunc is registered or
			fmt.Println("StructField: " + verror.StructField())         // by passing alt name to ReportError like below
			fmt.Println("Tag: " + verror.Tag())
			fmt.Println("ActualTag: " + verror.ActualTag())
			fmt.Println("Kind: ", verror.Kind())
			fmt.Println("Type: ", verror.Type())
			fmt.Println("Value: ", verror.Value())
			fmt.Println("Param: " + verror.Param())
			fmt.Println()
		*/

		fields = append(fields, field)
	}

	return &Error{
		Err:               err,
		Status:            http.StatusBadRequest,
		Fields:            fields,
		Cause:             err,
		Message:           "Field validation error",
		isValidationError: true,
	}, true
}

func FormField(namespace string) string {
	if !strings.Contains(namespace, ".") {
		return namespace
	}

	pts := strings.Split(namespace, ".")

	var newPts []string
	for i := 1; i < len(pts); i++ {
		newPts = append(newPts, strcase.ToCamel(pts[i]))
	}

	return strings.Join(newPts, ".")
}
