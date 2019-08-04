package geonames

import (
	"context"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	// The database table for CountryTimezone
	countrieTimezonesTableName = "country_timezones"
)

// FindCountryTimezones ....
func FindCountryTimezones(ctx context.Context, dbConn *sqlx.DB, orderBy, where string, args ...interface{}) ([]*CountryTimezone, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.geonames.FindCountryTimezones")
	defer span.Finish()

	query := sqlbuilder.NewSelectBuilder()
	query.Select("country_code,timezone_id")
	query.From(countrieTimezonesTableName)

	if orderBy == "" {
		orderBy = "timezone_id"
	}
	query.OrderBy(orderBy)

	if where != "" {
		query.Where(where)
	}

	queryStr, queryArgs := query.Build()
	queryStr = dbConn.Rebind(queryStr)
	args = append(args, queryArgs...)

	// Fetch all country timezones from the db.
	rows, err := dbConn.QueryContext(ctx, queryStr, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessage(err, "find country timezones failed")
		return nil, err
	}

	// iterate over each row
	resp := []*CountryTimezone{}
	for rows.Next() {
		var (
			v   CountryTimezone
			err error
		)
		err = rows.Scan(&v.CountryCode, &v.TimezoneId)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			return nil, err
		} else if v.CountryCode == "" || v.TimezoneId == "" {
			continue
		}

		resp = append(resp, &v)
	}

	return resp, nil
}

func ListTimezones(ctx context.Context, dbConn *sqlx.DB) ([]string, error) {
	res, err := FindCountryTimezones(ctx, dbConn, "timezone_id", "")
	if err != nil {
		return nil, err
	}

	resp := []string{}
	for _, ct := range res {
		var exists bool
		for _, t := range resp {
			if ct.TimezoneId == t {
				exists = true
				break
			}
		}

		if !exists {
			resp = append(resp, ct.TimezoneId)
		}
	}

	return resp, nil
}
