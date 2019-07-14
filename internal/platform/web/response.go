package web

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
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

	// If the error was of the type *Error, the handler has
	// a specific status code and error to return.
	webErr, ok := errors.Cause(err).(*Error)
	if !ok {
		webErr, ok = err.(*Error)
	}

	if ok {
		er := ErrorResponse{
			Error:  webErr.Err.Error(),
			Fields: webErr.Fields,
		}
		if err := RespondJson(ctx, w, er, webErr.Status); err != nil {
			return err
		}
		return nil
	}

	// If not, the handler sent any arbitrary error value so use 500.
	er := ErrorResponse{
		Error: http.StatusText(http.StatusInternalServerError),
	}
	if err := RespondJson(ctx, w, er, http.StatusInternalServerError); err != nil {
		return err
	}
	return nil
}

// RespondJson converts a Go value to JSON and sends it to the client.
// If code is StatusNoContent, v is expected to be nil.
func RespondJson(ctx context.Context, w http.ResponseWriter, data interface{}, statusCode int) error {

	// Set the status code for the request logger middleware.
	// If the context is missing this value, request the service
	// to be shutdown gracefully.
	v, ok := ctx.Value(KeyValues).(*Values)
	if !ok {
		return NewShutdownError("web value missing from context")
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
	return RespondErrorStatus(ctx, w, er, http.StatusInternalServerError)
}

// RespondErrorStatus sends an error back to the client as plain text with
// the specified HTTP status code.
func RespondErrorStatus(ctx context.Context, w http.ResponseWriter, er error, statusCode int) error {
	msg := fmt.Sprintf("%s", er)
	if err := Respond(ctx, w, []byte(msg), statusCode, MIMETextPlainCharsetUTF8); err != nil {
		return err
	}
	return nil
}


// RespondText sends text back to the client as plain text with the specified HTTP status code.
func RespondText(ctx context.Context, w http.ResponseWriter, text string, statusCode int) error {
	if err := Respond(ctx, w, []byte(text), statusCode, MIMETextPlainCharsetUTF8); err != nil {
		return err
	}
	return nil
}

// Respond writes the data to the client with the specified HTTP status code and
// content type.
func Respond(ctx context.Context, w http.ResponseWriter, data []byte, statusCode int, contentType string) error {

	// Set the status code for the request logger middleware.
	// If the context is missing this value, request the service
	// to be shutdown gracefully.
	v, ok := ctx.Value(KeyValues).(*Values)
	if !ok {
		return NewShutdownError("web value missing from context")
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
