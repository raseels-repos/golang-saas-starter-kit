package geonames

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"github.com/huandu/go-sqlbuilder"
	"github.com/pkg/errors"
	"github.com/sethgrid/pester"
	"github.com/shopspring/decimal"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	// The database table for Geoname
	geonamesTableName = "geonames"
)

// List of country codes that will geonames will be downloaded for.
func ValidGeonameCountries(ctx context.Context) []string {
	if webcontext.ContextEnv(ctx) == webcontext.Env_Dev {
		return []string{"US"}
	}
	return []string{
		"AD", "AR", "AS", "AT", "AU", "AX", "BD", "BE", "BG",
		"BR", "CA", "CH", "CZ", "DE", "DK", "DO",
		"DZ", "ES", "FI", "FO", "FR", "GB", "GF", "GG", "GL", "GP",
		"GT", "GU", "HR", "HU", "IM", "IN", "IS", "IT", "JE",
		"JP", "LI", "LK", "LT", "LU", "MC", "MD", "MH", "MK",
		"MP", "MQ", "MX", "MY", "NL", "NO", "NZ", "PH",
		"PK", "PL", "PM", "PR", "PT", "RE", "RO", "RU", "SE", "SI",
		"SJ", "SK", "SM", "TH", "TR", "US", "VA", "VI",
		"YT", "ZA"}
}

// FindGeonames ....
func (repo *Repository) FindGeonames(ctx context.Context, orderBy, where string, args ...interface{}) ([]*Geoname, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.geonames.FindGeonames")
	defer span.Finish()

	query := sqlbuilder.NewSelectBuilder()
	query.Select("country_code,postal_code,place_name,state_name,state_code,county_name,county_code,community_name,community_code,latitude,longitude,accuracy")
	query.From(geonamesTableName)

	if orderBy == "" {
		orderBy = "postal_code"
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
		err = errors.WithMessage(err, "find regions failed")
		return nil, err
	}

	// iterate over each row
	resp := []*Geoname{}
	for rows.Next() {
		var (
			v   Geoname
			err error
		)
		err = rows.Scan(&v.CountryCode, &v.PostalCode, &v.PlaceName, &v.StateName, &v.StateCode, &v.CountyName, &v.CountyCode, &v.CommunityName, &v.CommunityCode, &v.Latitude, &v.Longitude, &v.Accuracy)
		if err != nil {
			return nil, errors.WithStack(err)
		} else if v.PostalCode == "" {
			continue
		}

		resp = append(resp, &v)
	}

	return resp, nil
}

// FindGeonamePostalCodes ....
func (repo *Repository) FindGeonamePostalCodes(ctx context.Context, where string, args ...interface{}) ([]string, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.geonames.FindGeonamePostalCodes")
	defer span.Finish()

	query := sqlbuilder.NewSelectBuilder()
	query.Select("postal_code")
	query.From(geonamesTableName)

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
		err = errors.WithMessage(err, "find regions failed")
		return nil, err
	}

	// iterate over each row
	resp := []string{}
	for rows.Next() {
		var (
			v   string
			err error
		)
		err = rows.Scan(&v)
		if err != nil {
			return nil, errors.WithStack(err)
		} else if v == "" {
			continue
		}

		resp = append(resp, v)
	}

	return resp, nil
}

// FindGeonameRegions ....
func (repo *Repository) FindGeonameRegions(ctx context.Context, orderBy, where string, args ...interface{}) ([]*Region, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.geonames.FindGeonameRegions")
	defer span.Finish()

	query := sqlbuilder.NewSelectBuilder()
	query.Select("distinct state_code", "state_name")
	query.From(geonamesTableName)

	if orderBy == "" {
		orderBy = "state_name"
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
		err = errors.WithMessage(err, "find regions failed")
		return nil, err
	}

	// iterate over each row
	resp := []*Region{}
	for rows.Next() {
		var (
			v   Region
			err error
		)
		err = rows.Scan(&v.Code, &v.Name)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			return nil, err
		} else if v.Code == "" || v.Name == "" {
			continue
		}

		resp = append(resp, &v)
	}

	return resp, nil
}

// GetGeonameCountry downloads geoname data for the country.
// Parses data and returns slice of Geoname
func (repo *Repository) GetGeonameCountry(ctx context.Context, country string) ([]Geoname, error) {
	res := make([]Geoname, 0)
	var err error
	var resp *http.Response

	u := fmt.Sprintf("http://www.geonames.org/export/zip/%s.zip", country)

	h := fmt.Sprintf("%x", md5.Sum([]byte(u)))
	cp := filepath.Join(os.TempDir(), h+".zip")

	if _, err := os.Stat(cp); err != nil {
		resp, err = pester.Get(u)
		if err != nil {
			// Add re-try three times after failing first time
			// This reduces the risk when network is lagy, we still have chance to re-try.
			for i := 0; i < 3; i++ {
				resp, err = pester.Get(u)
				if err == nil {
					break
				}
				time.Sleep(time.Second * 1)
			}
			if err != nil {
				err = errors.WithMessagef(err, "Failed to read countries from '%s'", u)
				return res, err
			}
		}
		defer resp.Body.Close()

		// Create the file
		out, err := os.Create(cp)
		if err != nil {
			return nil, err
		}
		defer out.Close()

		// Write the body to file
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			return nil, err
		}

		out.Close()
	}

	f, err := os.Open(cp)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReader(f)

	buff := bytes.NewBuffer([]byte{})
	size, err := io.Copy(buff, br)
	if err != nil {
		err = errors.WithStack(err)
		return res, err
	}

	b := bytes.NewReader(buff.Bytes())
	zr, err := zip.NewReader(b, size)
	if err != nil {
		err = errors.WithStack(err)
		return res, err
	}

	for _, f := range zr.File {
		if f.Name == "readme.txt" {
			continue
		}

		fh, err := f.Open()
		if err != nil {
			err = errors.WithStack(err)
			return res, err
		}

		scanner := bufio.NewScanner(fh)
		for scanner.Scan() {
			line := scanner.Text()

			if strings.Contains(line, "\"") {
				line = strings.Replace(line, "\"", "\\\"", -1)
			}

			r := csv.NewReader(strings.NewReader(line))
			r.Comma = '\t' // Use tab-delimited instead of comma <---- here!
			r.LazyQuotes = true
			r.FieldsPerRecord = -1

			lines, err := r.ReadAll()
			if err != nil {
				err = errors.WithStack(err)
				continue
			}

			for _, row := range lines {

				gn := Geoname{
					CountryCode:   row[0],
					PostalCode:    row[1],
					PlaceName:     row[2],
					StateName:     row[3],
					StateCode:     row[4],
					CountyName:    row[5],
					CountyCode:    row[6],
					CommunityName: row[7],
					CommunityCode: row[8],
				}
				if row[9] != "" {
					gn.Latitude, err = decimal.NewFromString(row[9])
					if err != nil {
						err = errors.WithStack(err)
					}
				}

				if row[10] != "" {
					gn.Longitude, err = decimal.NewFromString(row[10])
					if err != nil {
						err = errors.WithStack(err)
					}
				}

				if row[11] != "" {
					gn.Accuracy, err = strconv.Atoi(row[11])
					if err != nil {
						err = errors.WithStack(err)
					}
				}

				res = append(res, gn)
			}
		}

		if err := scanner.Err(); err != nil {
			err = errors.WithStack(err)
		}
	}

	return res, err
}
