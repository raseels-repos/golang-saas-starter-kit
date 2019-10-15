package project_route

import (
	"fmt"
	"github.com/pkg/errors"
	"net"
	"net/url"
	"os"
	"strings"
)

type ProjectRoute struct {
	webAppUrl url.URL
	webApiUrl url.URL
}

func New(apiBaseUrl, appBaseUrl string) (ProjectRoute, error) {
	var r ProjectRoute

	apiUrl, err := url.Parse(apiBaseUrl)
	if err != nil {
		return r, errors.WithMessagef(err, "Failed to parse api base URL '%s'", apiBaseUrl)
	}
	r.webApiUrl = *apiUrl

	appUrl, err := url.Parse(appBaseUrl)
	if err != nil {
		return r, errors.WithMessagef(err, "Failed to parse app base URL '%s'", appBaseUrl)
	}
	r.webAppUrl = *appUrl

	return r, nil
}

func (r ProjectRoute) WebAppUrl(urlPath string) string {
	u := r.webAppUrl
	u.Path = urlPath
	return u.String()
}

func (r ProjectRoute) WebApiUrl(urlPath string) string {
	u := r.webApiUrl
	u.Path = urlPath
	return u.String()
}

func (r ProjectRoute) UserResetPassword(resetHash string) string {
	u := r.webAppUrl
	u.Path = "/user/reset-password/" + resetHash
	return u.String()
}

func (r ProjectRoute) UserInviteAccept(inviteHash string) string {
	u := r.webAppUrl
	u.Path = "/users/invite/" + inviteHash
	return u.String()
}

func (r ProjectRoute) ApiDocs() string {
	u := r.webApiUrl
	u.Path = "/docs"
	return u.String()
}

func (r ProjectRoute) ApiDocsJson(internal bool) string {
	u := r.webApiUrl

	if ev := os.Getenv("USE_NETWORK_ALIAS"); ev != "" {
		if internal && strings.Contains(u.Host, ":") {
			h, p, _ := net.SplitHostPort(u.Host)
			if h == "127.0.0.1" || h == "localhost" {
				u.Host = fmt.Sprintf("web-api:%s", p)
			}
		}
	}

	u.Path = "/docs/doc.json"
	return u.String()
}
