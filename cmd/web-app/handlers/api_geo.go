package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/jmoiron/sqlx"
	"geeks-accelerator/oss/saas-starter-kit/internal/geonames"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)

// Check provides support for orchestration geo endpoints.
type Geo struct {
	MasterDB *sqlx.DB
	Redis    *redis.Client
}

// RegionsAutocomplete...
func (h *Geo) RegionsAutocomplete(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	var filters []string
	var args []interface{}

	if qv := r.URL.Query().Get("postal_code"); qv != "" {
		filters = append(filters,"postal_code like ?")
		args = append(args, qv+"%")
	}

	if qv := r.URL.Query().Get("query"); qv != "" {
		filters = append(filters,"(state_name like ? or state_code like ?)")
		args = append(args, qv+"%", qv+"%")
	}

	where := strings.Join(filters, " AND ")

	res, err := geonames.FindGeonameRegions(ctx, h.MasterDB, where, args)
	if err != nil {
		fmt.Printf("%+v", err)
		return web.RespondJsonError(ctx, w, err)
	}

	var list []string
	for _, c := range res {
		list = append(list, c.Name)
	}

	return web.RespondJson(ctx, w, list, http.StatusOK)
}

