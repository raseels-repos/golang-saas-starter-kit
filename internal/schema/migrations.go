package schema

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/geonames"
	"github.com/geeks-accelerator/sqlxmigrate"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/sethgrid/pester"
)

// migrationList returns a list of migrations to be executed. If the id of the
// migration already exists in the migrations table it will be skipped.
func migrationList(ctx context.Context, db *sqlx.DB, log *log.Logger, isUnittest bool) []*sqlxmigrate.Migration {
	geoRepo := geonames.NewRepository(db)

	return []*sqlxmigrate.Migration{
		// Create table users.
		{
			ID: "20190522-01b",
			Migrate: func(tx *sql.Tx) error {
				q1 := `CREATE TABLE IF NOT EXISTS users (
					  id char(36) NOT NULL,
					  email varchar(200) NOT NULL,
					  name varchar(200) NOT NULL DEFAULT '',
					  password_hash varchar(256) NOT NULL,
					  password_salt varchar(36) NOT NULL,
					  password_reset varchar(36) DEFAULT NULL,
					  timezone varchar(128) NOT NULL DEFAULT 'America/Anchorage',
					  created_at TIMESTAMP WITH TIME ZONE NOT NULL,
					  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
					  archived_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
					  PRIMARY KEY (id),
					  CONSTRAINT email UNIQUE  (email)
					) ;`
				if _, err := tx.Exec(q1); err != nil {
					return errors.Wrapf(err, "Query failed %s", q1)
				}
				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				q1 := `DROP TABLE IF EXISTS users`
				if _, err := tx.Exec(q1); err != nil {
					return errors.Wrapf(err, "Query failed %s", q1)
				}
				return nil
			},
		},
		// Create new table accounts.
		{
			ID: "20190522-01h",
			Migrate: func(tx *sql.Tx) error {
				if err := createTypeIfNotExists(tx, "account_status_t", "enum('active','pending','disabled')"); err != nil {
					return err
				}

				q2 := `CREATE TABLE IF NOT EXISTS accounts (
					  id char(36) NOT NULL,
					  name varchar(255) NOT NULL,
					  address1 varchar(255) NOT NULL DEFAULT '',
					  address2 varchar(255) NOT NULL DEFAULT '',
					  city varchar(100) NOT NULL DEFAULT '',
					  region varchar(255) NOT NULL DEFAULT '',
					  country varchar(255) NOT NULL DEFAULT '',
					  zipcode varchar(20) NOT NULL DEFAULT '',
					  status account_status_t NOT NULL DEFAULT 'active',
					  timezone varchar(128) NOT NULL DEFAULT 'America/Anchorage',
					  signup_user_id char(36) DEFAULT NULL REFERENCES users(id) ON DELETE SET NULL,
					  billing_user_id char(36) DEFAULT NULL REFERENCES users(id) ON DELETE SET NULL,
					  created_at TIMESTAMP WITH TIME ZONE NOT NULL,
					  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
					  archived_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
					  PRIMARY KEY (id),
					  CONSTRAINT name UNIQUE  (name)
					)`
				if _, err := tx.Exec(q2); err != nil {
					return errors.Wrapf(err, "Query failed %s", q2)
				}
				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				q1 := `DROP TYPE IF EXISTS account_status_t`
				if _, err := tx.Exec(q1); err != nil {
					return errors.Wrapf(err, "Query failed %s", q1)
				}

				q2 := `DROP TABLE IF EXISTS accounts`
				if _, err := tx.Exec(q2); err != nil {
					return errors.Wrapf(err, "Query failed %s", q2)
				}
				return nil
			},
		},
		// Create new table user_accounts.
		{
			ID: "20190522-01e",
			Migrate: func(tx *sql.Tx) error {
				if err := createTypeIfNotExists(tx, "user_account_role_t", "enum('admin', 'user')"); err != nil {
					return err
				}

				if err := createTypeIfNotExists(tx, "user_account_status_t", "enum('active', 'invited','disabled')"); err != nil {
					return err
				}

				q1 := `CREATE TABLE IF NOT EXISTS users_accounts (
					  id char(36) NOT NULL,
					  account_id char(36) NOT NULL  REFERENCES accounts(id) ON DELETE NO ACTION,
					  user_id char(36) NOT NULL  REFERENCES users(id) ON DELETE NO ACTION,
					  roles user_account_role_t[] NOT NULL,
					  status user_account_status_t NOT NULL DEFAULT 'active',
					  created_at TIMESTAMP WITH TIME ZONE NOT NULL,
					  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
					  archived_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
					  PRIMARY KEY (id),
					  CONSTRAINT user_account UNIQUE (user_id,account_id) 
					)`
				if _, err := tx.Exec(q1); err != nil {
					return errors.Wrapf(err, "Query failed %s", q1)
				}

				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				q1 := `DROP TYPE IF EXISTS user_account_role_t`
				if _, err := tx.Exec(q1); err != nil {
					return errors.Wrapf(err, "Query failed %s", q1)
				}

				q2 := `DROP TYPE IF EXISTS user_account_status_t`
				if _, err := tx.Exec(q2); err != nil {
					return errors.Wrapf(err, "Query failed %s", q2)
				}

				q3 := `DROP TABLE IF EXISTS users_accounts`
				if _, err := tx.Exec(q3); err != nil {
					return errors.Wrapf(err, "Query failed %s", q3)
				}

				return nil
			},
		},
		// Create new table projects.
		{
			ID: "20190622-01",
			Migrate: func(tx *sql.Tx) error {
				if err := createTypeIfNotExists(tx, "project_status_t", "enum('active','disabled')"); err != nil {
					return err
				}

				q1 := `CREATE TABLE IF NOT EXISTS projects (
					  id char(36) NOT NULL,
					  account_id char(36) NOT NULL REFERENCES accounts(id) ON DELETE SET NULL,
					  name varchar(255) NOT NULL,
					  status project_status_t NOT NULL DEFAULT 'active',
					  created_at TIMESTAMP WITH TIME ZONE NOT NULL,
					  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
					  archived_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
					  PRIMARY KEY (id)
					)`
				if _, err := tx.Exec(q1); err != nil {
					return errors.Wrapf(err, "Query failed %s", q1)
				}
				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				q1 := `DROP TYPE IF EXISTS project_status_t`
				if _, err := tx.Exec(q1); err != nil && !errorIsAlreadyExists(err) {
					return errors.Wrapf(err, "Query failed %s", q1)
				}

				q2 := `DROP TABLE IF EXISTS projects`
				if _, err := tx.Exec(q2); err != nil && !errorIsAlreadyExists(err) {
					return errors.Wrapf(err, "Query failed %s", q2)
				}
				return nil
			},
		},
		// Split users.name into first_name and last_name columns.
		{
			ID: "20190729-01a",
			Migrate: func(tx *sql.Tx) error {
				q1 := `ALTER TABLE users 
					  RENAME COLUMN name to first_name;`
				if _, err := tx.Exec(q1); err != nil {
					return errors.Wrapf(err, "Query failed %s", q1)
				}

				q2 := `ALTER TABLE users 
					  ADD last_name varchar(200) NOT NULL DEFAULT '';`
				if _, err := tx.Exec(q2); err != nil {
					return errors.Wrapf(err, "Query failed %s", q2)
				}

				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				q1 := `DROP TABLE IF EXISTS users`
				if _, err := tx.Exec(q1); err != nil {
					return errors.Wrapf(err, "Query failed %s", q1)
				}
				return nil
			},
		},
		// Load new geonames table.
		{
			ID: "20190731-02l",
			Migrate: func(tx *sql.Tx) error {

				schemas := []string{
					`DROP TABLE IF EXISTS geonames`,
					`CREATE TABLE geonames (
						country_code char(2),
						postal_code character varying(60),
						place_name character varying(200),
						state_name character varying(200),
						state_code character varying(10),
						county_name character varying(200),
						county_code character varying(10),
						community_name character varying(200),
						community_code character varying(10),
						latitude float,
						longitude float,
						accuracy int)`,
				}

				for _, q := range schemas {
					_, err := tx.Exec(q)
					if err != nil {
						return errors.Wrapf(err, "Failed to execute sql query '%s'", q)
					}
				}

				countries := geonames.ValidGeonameCountries(ctx)
				if isUnittest {
					countries = []string{"US"}
				}

				ncol := 12
				fn := func(geoNames []geonames.Geoname) error {
					valueStrings := make([]string, 0, len(geoNames))
					valueArgs := make([]interface{}, 0, len(geoNames)*ncol)
					for _, geoname := range geoNames {
						valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")

						valueArgs = append(valueArgs, geoname.CountryCode)
						valueArgs = append(valueArgs, geoname.PostalCode)
						valueArgs = append(valueArgs, geoname.PlaceName)

						valueArgs = append(valueArgs, geoname.StateName)
						valueArgs = append(valueArgs, geoname.StateCode)
						valueArgs = append(valueArgs, geoname.CountyName)

						valueArgs = append(valueArgs, geoname.CountyCode)
						valueArgs = append(valueArgs, geoname.CommunityName)
						valueArgs = append(valueArgs, geoname.CommunityCode)

						valueArgs = append(valueArgs, geoname.Latitude)
						valueArgs = append(valueArgs, geoname.Longitude)
						valueArgs = append(valueArgs, geoname.Accuracy)
					}
					insertStmt := fmt.Sprintf("insert into geonames "+
						"(country_code,postal_code,place_name,state_name,state_code,county_name,county_code,community_name,community_code,latitude,longitude,accuracy) "+
						"VALUES %s", strings.Join(valueStrings, ","))
					insertStmt = db.Rebind(insertStmt)

					_, err := tx.Exec(insertStmt, valueArgs...)
					if err != nil {
						return errors.Wrapf(err, "Failed to execute sql query '%s'", insertStmt)
					}

					return nil
				}
				start := time.Now()
				for _, country := range countries {
					//fmt.Println("LoadGeonames: start country: ", country)
					v, err := geoRepo.GetGeonameCountry(context.Background(), country)
					if err != nil {
						return errors.WithStack(err)
					}
					//fmt.Println("Geoname records: ", len(v))
					// Max argument values of Postgres is about 54460. So the batch size for bulk insert is selected 4500*12 (ncol)
					batch := 1000
					n := len(v) / batch

					//fmt.Println("Number of batch: ", n)

					if n == 0 {
						err := fn(v)
						if err != nil {
							return errors.WithStack(err)
						}
					} else {
						for i := 0; i < n; i++ {
							vn := v[i*batch : (i+1)*batch]
							err := fn(vn)
							if err != nil {
								return errors.WithStack(err)
							}
							if n > 0 && n%25 == 0 {
								time.Sleep(200)
							}
						}
						if len(v)%batch > 0 {
							fmt.Println("Remain part: ", len(v)-n*batch)
							vn := v[n*batch:]
							err := fn(vn)
							if err != nil {
								return errors.WithStack(err)
							}
						}
					}

					//fmt.Println("Insert Geoname took: ", time.Since(start))
					//fmt.Println("LoadGeonames: end country: ", country)
				}
				log.Println("Total Geonames population took: ", time.Since(start))

				queries := []string{
					`create index idx_geonames_country_code on geonames (country_code)`,
					`create index idx_geonames_postal_code on geonames (postal_code)`,
				}

				for _, q := range queries {
					_, err := tx.Exec(q)
					if err != nil {
						return errors.Wrapf(err, "Failed to execute sql query '%s'", q)
					}
				}

				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				return nil
			},
		},
		// Load new countries table.
		{
			ID: "20190731-02f",
			Migrate: func(tx *sql.Tx) error {

				schemas := []string{
					// Countries...
					`DROP TABLE IF EXISTS countries`,
					`CREATE TABLE countries(
						code           char(2) not null constraint countries_pkey primary key,
						iso_alpha3           char(3),
						name    character varying(50),
						capital    character varying(50),
						currency_code        char(3),
						currency_name        CHAR(20),
						phone                character varying(20),
						postal_code_format        character varying(200),
						postal_code_regex         character varying(200))`,
				}

				for _, q := range schemas {
					_, err := tx.Exec(q)
					if err != nil {
						return errors.Wrapf(err, "Failed to execute sql query '%s'", q)
					}
				}

				if isUnittest {
					// `insert into countries(code, iso_alpha3, name, capital, currency_code, currency_name, phone, postal_code_format, postal_code_regex)

				} else {
					prep := []string{
						`DROP TABLE IF EXISTS countryinfo`,
						`CREATE TABLE countryinfo (
						iso_alpha2           char(2),
						iso_alpha3           char(3),
						iso_numeric          integer,
						fips_code            character varying(3),
						country              character varying(200),
						capital              character varying(200),
						areainsqkm           double precision,
						population           integer,
						continent            char(2),
						tld                  CHAR(10),
						currency_code        char(3),
						currency_name        CHAR(20),
						phone                character varying(20),
						postal_format        character varying(200),
						postal_regex         character varying(200),
						languages            character varying(200),
						geonameId            int,
						neighbours           character varying(50),
						equivalent_fips_code character varying(3))`,
					}

					for _, q := range prep {
						_, err := tx.Exec(q)
						if err != nil {
							return errors.Wrapf(err, "Failed to execute sql query '%s'", q)
						}
					}

					u := "http://download.geonames.org/export/dump/countryInfo.txt"
					resp, err := pester.Get(u)
					if err != nil {
						return errors.Wrapf(err, "Failed to read country info from '%s'", u)
					}
					defer resp.Body.Close()

					scanner := bufio.NewScanner(resp.Body)
					var prevLine string
					var stmt *sql.Stmt
					for scanner.Scan() {
						line := scanner.Text()

						// Skip comments.
						if strings.HasPrefix(line, "#") {
							prevLine = line
							continue
						}

						// Pull the last comment to load the fields.
						if stmt == nil {
							prevLine = strings.TrimPrefix(prevLine, "#")
							r := csv.NewReader(strings.NewReader(prevLine))
							r.Comma = '\t' // Use tab-delimited instead of comma <---- here!
							r.FieldsPerRecord = -1

							lines, err := r.ReadAll()
							if err != nil {
								return errors.WithStack(err)
							}
							var columns []string

							for _, fn := range lines[0] {
								var cn string
								switch fn {
								case "ISO":
									cn = "iso_alpha2"
								case "ISO3":
									cn = "iso_alpha3"
								case "ISO-Numeric":
									cn = "iso_numeric"
								case "fips":
									cn = "fips_code"
								case "Country":
									cn = "country"
								case "Capital":
									cn = "capital"
								case "Area(in sq km)":
									cn = "areainsqkm"
								case "Population":
									cn = "population"
								case "Continent":
									cn = "continent"
								case "tld":
									cn = "tld"
								case "CurrencyCode":
									cn = "currency_code"
								case "CurrencyName":
									cn = "currency_name"
								case "Phone":
									cn = "phone"
								case "Postal Code Format":
									cn = "postal_format"
								case "Postal Code Regex":
									cn = "postal_regex"
								case "Languages":
									cn = "languages"
								case "geonameid":
									cn = "geonameId"
								case "neighbours":
									cn = "neighbours"
								case "EquivalentFipsCode":
									cn = "equivalent_fips_code"
								default:
									return errors.Errorf("Failed to map column %s", fn)
								}
								columns = append(columns, cn)
							}

							placeholders := []string{}
							for i := 0; i < len(columns); i++ {
								placeholders = append(placeholders, "?")
							}

							q := "insert into countryinfo (" + strings.Join(columns, ",") + ") values(" + strings.Join(placeholders, ",") + ")"
							q = db.Rebind(q)
							stmt, err = tx.Prepare(q)
							if err != nil {
								return errors.Wrapf(err, "Failed to prepare sql query '%s'", q)
							}
						}

						r := csv.NewReader(strings.NewReader(line))
						r.Comma = '\t' // Use tab-delimited instead of comma <---- here!
						r.FieldsPerRecord = -1

						lines, err := r.ReadAll()
						if err != nil {
							return errors.WithStack(err)
						}

						for _, row := range lines {
							var args []interface{}
							for _, v := range row {
								args = append(args, v)
							}

							_, err = stmt.Exec(args...)
							if err != nil {
								return errors.WithStack(err)
							}
						}
					}

					if err := scanner.Err(); err != nil {
						return errors.WithStack(err)
					}

					queries := []string{
						`insert into countries(code, iso_alpha3, name, capital, currency_code, currency_name, phone, postal_code_format, postal_code_regex)
						select iso_alpha2, iso_alpha3, country, capital, currency_code, currency_name, phone, postal_format, postal_regex
						from countryinfo`,
						`DROP TABLE IF EXISTS countryinfo`,
					}

					for _, q := range queries {
						_, err := tx.Exec(q)
						if err != nil {
							return errors.Wrapf(err, "Failed to execute sql query '%s'", q)
						}
					}
				}

				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				return nil
			},
		},
		// Load new country_timezones table.
		{
			ID: "20190731-03e",
			Migrate: func(tx *sql.Tx) error {

				queries := []string{
					`DROP TABLE IF EXISTS country_timezones`,
					`CREATE TABLE country_timezones(
						country_code           char(2) not null,
						timezone_id    character varying(50) not null,
						CONSTRAINT country_timezones_pkey UNIQUE (country_code, timezone_id))`,
				}

				for _, q := range queries {
					_, err := tx.Exec(q)
					if err != nil {
						return errors.Wrapf(err, "Failed to execute sql query '%s'", q)
					}
				}

				if isUnittest {

				} else {
					u := "http://download.geonames.org/export/dump/timeZones.txt"
					resp, err := pester.Get(u)
					if err != nil {
						return errors.Wrapf(err, "Failed to read timezones info from '%s'", u)
					}
					defer resp.Body.Close()

					q := "insert into country_timezones (country_code,timezone_id) values(?, ?)"
					q = db.Rebind(q)
					stmt, err := tx.Prepare(q)
					if err != nil {
						return errors.Wrapf(err, "Failed to prepare sql query '%s'", q)
					}

					scanner := bufio.NewScanner(resp.Body)
					for scanner.Scan() {
						line := scanner.Text()

						// Skip comments.
						if strings.HasPrefix(line, "CountryCode") {
							continue
						}

						r := csv.NewReader(strings.NewReader(line))
						r.Comma = '\t' // Use tab-delimited instead of comma <---- here!
						r.FieldsPerRecord = -1

						lines, err := r.ReadAll()
						if err != nil {
							return errors.WithStack(err)
						}

						for _, row := range lines {
							_, err = stmt.Exec(row[0], row[1])
							if err != nil {
								return errors.WithStack(err)
							}
						}
					}

					if err := scanner.Err(); err != nil {
						return errors.WithStack(err)
					}
				}

				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				return nil
			},
		},
		// Create new table account_preferences.
		{
			ID: "20190801-01",
			Migrate: func(tx *sql.Tx) error {

				q := `CREATE TABLE IF NOT EXISTS account_preferences (
					  account_id char(36) NOT NULL  REFERENCES accounts(id) ON DELETE NO ACTION,
					  name varchar(200) NOT NULL DEFAULT '',
					  value varchar(200) NOT NULL DEFAULT '',
					  created_at TIMESTAMP WITH TIME ZONE NOT NULL,
					  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
					  archived_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
					  CONSTRAINT account_preferences_pkey UNIQUE (account_id,name) 
					)`
				if _, err := tx.Exec(q); err != nil {
					return errors.Wrapf(err, "Query failed %s", q)
				}

				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				return nil
			},
		},
		// Remove default value for users.timezone.
		{
			ID: "20190805-01",
			Migrate: func(tx *sql.Tx) error {
				q1 := `ALTER TABLE users ALTER COLUMN timezone DROP DEFAULT`
				if _, err := tx.Exec(q1); err != nil {
					return errors.Wrapf(err, "Query failed %s", q1)
				}

				q2 := `ALTER TABLE users ALTER COLUMN timezone DROP NOT NULL`
				if _, err := tx.Exec(q2); err != nil {
					return errors.Wrapf(err, "Query failed %s", q2)
				}

				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				return nil
			},
		},
	}
}

// dropTypeIfExists executes drop type.
func dropTypeIfExists(tx *sql.Tx, name string) error {
	q := "DROP TYPE IF EXISTS " + name
	if _, err := tx.Exec(q); err != nil && !errorIsAlreadyExists(err) {
		return errors.Wrapf(err, "Query failed %s", q)
	}
	return nil
}

// createTypeIfNotExists checks to ensure a type doesn't exist before creating.
func createTypeIfNotExists(tx *sql.Tx, name, val string) error {

	q1 := "select exists (select 1 from pg_type where typname = '" + name + "')"
	rows, err := tx.Query(q1)
	if err != nil {
		return errors.Wrapf(err, "Query failed %s", q1)
	}
	defer rows.Close()

	var exists bool
	for rows.Next() {
		err := rows.Scan(&exists)
		if err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return err
	}

	if exists {
		return nil
	}

	q2 := "CREATE TYPE " + name + " AS " + val
	if _, err := tx.Exec(q2); err != nil && !errorIsAlreadyExists(err) {
		return errors.Wrapf(err, "Query failed %s", q2)
	}

	return nil
}

// errorIsAlreadyExists checks an error message for the error "already exists"
func errorIsAlreadyExists(err error) bool {
	if strings.Contains(err.Error(), "already exists") {
		return true
	}
	return false
}
