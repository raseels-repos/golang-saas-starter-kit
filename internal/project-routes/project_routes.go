package project_routes

import (
	"github.com/pkg/errors"
	"net/url"
)

type ProjectRoutes struct {
	webAppUrl url.URL
	webApiUrl url.URL
}

func New(apiBaseUrl, appBaseUrl string) (ProjectRoutes, error) {
	var r ProjectRoutes

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

func (r ProjectRoutes) WebAppUrl(urlPath string) string {
	u := r.webAppUrl
	u.Path = urlPath
	return u.String()
}

func (r ProjectRoutes) WebApiUrl(urlPath string) string {
	u := r.webApiUrl
	u.Path = urlPath
	return u.String()
}

func (r ProjectRoutes) UserResetPassword(resetHash string) string {
	u := r.webAppUrl
	u.Path = "/user/reset-password/" + resetHash
	return u.String()
}

func (r ProjectRoutes) UserInviteAccept(inviteHash string) string {
	u := r.webAppUrl
	u.Path = "/users/invite/" + inviteHash
	return u.String()
}