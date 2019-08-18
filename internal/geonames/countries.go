package geonames

import (
	"context"

	"github.com/huandu/go-sqlbuilder"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	// The database table for Country
	countriesTableName = "countries"
)

// FindCountries ....
func (repo *Repository) FindCountries(ctx context.Context, orderBy, where string, args ...interface{}) ([]*Country, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.geonames.FindCountries")
	defer span.Finish()

	query := sqlbuilder.NewSelectBuilder()
	query.Select("code,iso_alpha3,name,capital,currency_code,currency_name,phone,postal_code_format,postal_code_regex")
	query.From(countriesTableName)

	if orderBy == "" {
		orderBy = "name"
	}
	query.OrderBy(orderBy)

	if where != "" {
		query.Where(where)
	}

	queryStr, queryArgs := query.Build()
	queryStr = repo.DbConn.Rebind(queryStr)
	args = append(args, queryArgs...)

	// fetch all places from the db
	rows, err := repo.DbConn.QueryContext(ctx, queryStr, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessage(err, "find countries failed")
		return nil, err
	}

	// iterate over each row
	resp := []*Country{}
	for rows.Next() {
		var (
			v   Country
			err error
		)
		err = rows.Scan(&v.Code, &v.IsoAlpha3, &v.Name, &v.Capital, &v.CurrencyCode, &v.CurrencyName, &v.Phone, &v.PostalCodeFormat, &v.PostalCodeRegex)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			return nil, err
		} else if v.Code == "" {
			continue
		}

		resp = append(resp, &v)
	}

	return resp, nil
}
