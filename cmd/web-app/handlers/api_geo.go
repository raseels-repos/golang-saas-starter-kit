package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/geonames"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/jmoiron/sqlx"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)

// Check provides support for orchestration geo endpoints.
type Geo struct {
	MasterDB *sqlx.DB
	Redis    *redis.Client
}

// GeonameByPostalCode...
func (h *Geo) GeonameByPostalCode(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	var filters []string
	var args []interface{}

	if qv := r.URL.Query().Get("country_code"); qv != "" {
		filters = append(filters, "country_code = ?")
		args = append(args, strings.ToUpper(qv))
	}

	if qv := r.URL.Query().Get("postal_code"); qv != "" {
		filters = append(filters, "postal_code = ?")
		args = append(args, strings.ToLower(qv))
	} else {
		filters = append(filters, "lower(postal_code) = ?")
		args = append(args, strings.ToLower(params["postalCode"]))
	}

	where := strings.Join(filters, " AND ")

	res, err := geonames.FindGeonames(ctx, h.MasterDB, "postal_code", where, args...)
	if err != nil {
		fmt.Printf("%+v", err)
		return web.RespondJsonError(ctx, w, err)
	}

	var resp interface{}
	if len(res) == 1 {
		resp = res[0]
	} else {
		// Autocomplete does not like null returned.
		resp = make(map[string]interface{})
	}

	return web.RespondJson(ctx, w, resp, http.StatusOK)
}

// PostalCodesAutocomplete...
func (h *Geo) PostalCodesAutocomplete(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	var filters []string
	var args []interface{}

	if qv := r.URL.Query().Get("country_code"); qv != "" {
		filters = append(filters, "country_code = ?")
		args = append(args, strings.ToUpper(qv))
	}

	if qv := r.URL.Query().Get("query"); qv != "" {
		filters = append(filters, "lower(postal_code) like ?")
		args = append(args, strings.ToLower(qv+"%"))
	}

	where := strings.Join(filters, " AND ")

	res, err := geonames.FindGeonamePostalCodes(ctx, h.MasterDB, where, args...)
	if err != nil {
		return web.RespondJsonError(ctx, w, err)
	}
	var list []string = res

	return web.RespondJson(ctx, w, list, http.StatusOK)
}

// RegionsAutocomplete...
func (h *Geo) RegionsAutocomplete(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	var filters []string
	var args []interface{}

	if qv := r.URL.Query().Get("country_code"); qv != "" {
		filters = append(filters, "country_code = ?")
		args = append(args, strings.ToUpper(qv))
	}

	if qv := r.URL.Query().Get("query"); qv != "" {
		filters = append(filters, "(lower(state_name) like ? or state_code like ?)")
		args = append(args, strings.ToLower(qv+"%"), strings.ToUpper(qv+"%"))
	}

	where := strings.Join(filters, " AND ")

	res, err := geonames.FindGeonameRegions(ctx, h.MasterDB, "state_name", where, args...)
	if err != nil {
		fmt.Printf("%+v", err)
		return web.RespondJsonError(ctx, w, err)
	}

	var resp interface{}
	if qv := r.URL.Query().Get("select"); qv != "" {
		list := []map[string]string{}
		for _, c := range res {
			list = append(list, map[string]string{
				"value": c.Code,
				"text":  c.Name,
			})
		}
		resp = list
	} else {
		list := []string{}
		for _, c := range res {
			list = append(list, c.Name)
		}
		resp = list
	}

	return web.RespondJson(ctx, w, resp, http.StatusOK)
}

// CountryTimezones....
func (h *Geo) CountryTimezones(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	var filters []string
	var args []interface{}

	if qv := r.URL.Query().Get("country_code"); qv != "" {
		filters = append(filters, "country_code = ?")
		args = append(args, strings.ToUpper(qv))
	} else {
		filters = append(filters, "country_code = ?")
		args = append(args, strings.ToUpper(params["countryCode"]))
	}

	where := strings.Join(filters, " AND ")

	res, err := geonames.FindCountryTimezones(ctx, h.MasterDB, "timezone_id", where, args...)
	if err != nil {
		return web.RespondJsonError(ctx, w, err)
	}

	list := []string{}
	for _, t := range res {
		list = append(list, t.TimezoneId)
	}

	return web.RespondJson(ctx, w, list, http.StatusOK)
}
