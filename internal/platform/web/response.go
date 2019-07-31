package web

import (
	"context"
	"encoding/json"
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"html/template"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
)

const (
	charsetUTF8 = "charset=UTF-8"
)

// MIME types
const (
	MIMEApplicationJSON            = "application/json"
	MIMEApplicationJSONCharsetUTF8 = MIMEApplicationJSON + "; " + charsetUTF8
	MIMETextHTML                   = "text/html"
	MIMETextHTMLCharsetUTF8        = MIMETextHTML + "; " + charsetUTF8
	MIMETextPlain                  = "text/plain"
	MIMETextPlainCharsetUTF8       = MIMETextPlain + "; " + charsetUTF8
	MIMEOctetStream                = "application/octet-stream"
)

// RespondJsonError sends an error formatted as JSON response back to the client.
func RespondJsonError(ctx context.Context, w http.ResponseWriter, err error) error {

	// Set the status code for the request logger middleware.
	// If the context is missing this value, request the service
	// to be shutdown gracefully.
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	// If the error was of the type *Error, the handler has
	// a specific status code and error to return.
	webErr := weberror.NewError(ctx, err, v.StatusCode).(*weberror.Error)
	v.StatusCode = webErr.Status

	return RespondJson(ctx, w, webErr.Display(ctx), webErr.Status)
}

// RespondJson converts a Go value to JSON and sends it to the client.
// If code is StatusNoContent, v is expected to be nil.
func RespondJson(ctx context.Context, w http.ResponseWriter, data interface{}, statusCode int) error {

	// Set the status code for the request logger middleware.
	// If the context is missing this value, request the service
	// to be shutdown gracefully.
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}
	v.StatusCode = statusCode

	// If there is nothing to marshal then set status code and return.
	if statusCode == http.StatusNoContent {
		w.WriteHeader(statusCode)
		return nil
	}

	// Check to see if the json has already been encoded.
	jsonData, ok := data.([]byte)
	if !ok {
		// Convert the response value to JSON.
		var err error
		jsonData, err = json.Marshal(data)
		if err != nil {
			return err
		}
	}

	// Set the content type and headers once we know marshaling has succeeded.
	w.Header().Set("Content-Type", MIMEApplicationJSONCharsetUTF8)

	// Write the status code to the response.
	w.WriteHeader(statusCode)

	// Send the result back to the client.
	if _, err := w.Write(jsonData); err != nil {
		return err
	}

	return nil
}

// RespondError sends an error back to the client as plain text with
// the status code 500 Internal Service Error
func RespondError(ctx context.Context, w http.ResponseWriter, er error) error {
	return RespondErrorStatus(ctx, w, er, 0)
}

// RespondErrorStatus sends an error back to the client as plain text with
// the specified HTTP status code.
func RespondErrorStatus(ctx context.Context, w http.ResponseWriter, er error, statusCode int) error {

	// Set the status code for the request logger middleware.
	// If the context is missing this value, request the service
	// to be shutdown gracefully.
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	// If the error was of the type *Error, the handler has
	// a specific status code and error to return.
	webErr := weberror.NewError(ctx, er, v.StatusCode).(*weberror.Error)
	v.StatusCode = webErr.Status

	respErr := webErr.Display(ctx).String()

	switch webcontext.ContextEnv(ctx) {
	case webcontext.Env_Dev, webcontext.Env_Stage:
		respErr = respErr + fmt.Sprintf("\n%s\n%+v", webErr.Error(), webErr.Cause)
	}

	return RespondText(ctx, w, respErr, statusCode)
}

// RespondText sends text back to the client as plain text with the specified HTTP status code.
func RespondText(ctx context.Context, w http.ResponseWriter, text string, statusCode int) error {
	return Respond(ctx, w, []byte(text), statusCode, MIMETextPlainCharsetUTF8)
}

// Respond writes the data to the client with the specified HTTP status code and
// content type.
func Respond(ctx context.Context, w http.ResponseWriter, data []byte, statusCode int, contentType string) error {

	// Set the status code for the request logger middleware.
	// If the context is missing this value, request the service
	// to be shutdown gracefully.
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}
	v.StatusCode = statusCode

	// If there is nothing to marshal then set status code and return.
	if statusCode == http.StatusNoContent {
		w.WriteHeader(statusCode)
		return nil
	}

	// Set the content type and headers once we know marshaling has succeeded.
	w.Header().Set("Content-Type", contentType)

	// Write the status code to the response.
	w.WriteHeader(statusCode)

	// Send the result back to the client.
	if _, err := w.Write(data); err != nil {
		return err
	}

	return nil
}

// RenderError sends an error back to the client as html with
// the specified HTTP status code.
func RenderError(ctx context.Context, w http.ResponseWriter, r *http.Request, er error, renderer Renderer, templateLayoutName, templateContentName, contentType string) error {

	// Set the status code for the request logger middleware.
	// If the context is missing this value, request the service
	// to be shutdown gracefully.
	v, err := webcontext.ContextValues(ctx)
	if err != nil {
		return err
	}

	// If the error was of the type *Error, the handler has
	// a specific status code and error to return.
	webErr := weberror.NewError(ctx, er, v.StatusCode).(*weberror.Error)
	v.StatusCode = webErr.Status

	respErr := webErr.Display(ctx)

	var fullError string
	switch webcontext.ContextEnv(ctx) {
	case webcontext.Env_Dev, webcontext.Env_Stage:
		if webErr.Cause != nil && webErr.Cause.Error() != webErr.Err.Error() {
			fullError = fmt.Sprintf("\n%s\n%+v", webErr.Error(), webErr.Cause)
		} else {
			fullError = fmt.Sprintf("%+v", webErr.Err)
		}

		fullError = strings.Replace(fullError, "\n", "<br/>", -1)
	}

	data := map[string]interface{}{
		"statusCode":   webErr.Status,
		"errorMessage": respErr.Error,
		"fullError":    template.HTML(fullError),
	}

	return renderer.Render(ctx, w, r, templateLayoutName, templateContentName, contentType, webErr.Status, data)
}

// Static registers a new route with path prefix to serve static files from the
// provided root directory. All errors will result in 404 File Not Found.
func Static(rootDir, prefix string) Handler {
	h := func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {
		err := StaticHandler(ctx, w, r, params, rootDir, prefix)
		if err != nil {
			return RespondErrorStatus(ctx, w, err, http.StatusNotFound)
		}
		return nil
	}
	return h
}

// StaticHandler sends a static file wo the client. The error is returned directly
// from this function allowing it to be wrapped by a Handler. The handler then was the
// the ability to format/display the error before responding to the client.
func StaticHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string, rootDir, prefix string) error {
	// Parse the URL from the http request.
	urlPath := path.Clean("/" + r.URL.Path) // "/"+ for security
	urlPath = strings.TrimLeft(urlPath, "/")

	// Remove the static directory name from the url
	rootDirName := filepath.Base(rootDir)
	if strings.HasPrefix(urlPath, rootDirName) {
		urlPath = strings.Replace(urlPath, rootDirName, "", 1)
	}

	// Also remove the URL prefix used to serve the static file since
	// this does not need to match any existing directory structure.
	if prefix != "" {
		urlPath = strings.TrimLeft(urlPath, prefix)
	}

	// Resolve the root directory to an absolute path
	sd, err := filepath.Abs(rootDir)
	if err != nil {
		return err
	}

	// Append the requested file to the root directory
	filePath := filepath.Join(sd, urlPath)

	// Make sure the file exists before attempting to serve it so
	// have the opportunity to handle the when a file does not exist.
	if _, err := os.Stat(filePath); err != nil {
		return err
	}

	// Serve the file from the local file system.
	http.ServeFile(w, r, filePath)

	return nil
}
