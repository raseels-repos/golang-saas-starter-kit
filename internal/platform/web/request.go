package web

import (
	"context"
	"encoding/json"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/schema"
	"github.com/pkg/errors"
	"github.com/xwb1989/sqlparser"
	"github.com/xwb1989/sqlparser/dependency/querypb"
)

// Headers
const (
	HeaderUpgrade             = "Upgrade"
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

// Decode reads the body of an HTTP request looking for a JSON document. The
// body is decoded into the provided value.
//
// If the provided value is a struct then it is checked for validation tags.
func Decode(ctx context.Context, r *http.Request, val interface{}) error {

	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(val); err != nil {
			return weberror.NewErrorMessage(ctx, err, http.StatusBadRequest, "decode request body failed")
		}
	} else {
		decoder := schema.NewDecoder()
		if err := decoder.Decode(val, r.URL.Query()); err != nil {
			err = errors.Wrap(err, "decode request query failed")
			return weberror.NewErrorMessage(ctx, err, http.StatusBadRequest, "decode request query failed")
		}
	}

	if err := webcontext.Validator().Struct(val); err != nil {
		verr, _ := weberror.NewValidationError(ctx, err)
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
