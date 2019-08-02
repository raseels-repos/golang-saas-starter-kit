package schema

import (
	"log"

	"github.com/jmoiron/sqlx"
)

// initSchema runs before any migrations are executed. This happens when no other migrations
// have previously been executed.
func initSchema(db *sqlx.DB, log *log.Logger, isUnittest bool) func(*sqlx.DB) error {
	f := func(db *sqlx.DB) error {
		return nil
	}

	return f
}

/*
// initGeonames populates countries and postal codes.
func initGeonamesOld(db *sqlx.DB) error {
	schemas := []string{
		`DROP TABLE IF EXISTS geoname`,
		`create table geoname (
			geonameid      int,
			name           varchar(200),
			asciiname      varchar(200),
			alternatenames text,
			latitude       float,
			longitude      float,
			fclass         char(1),
			fcode          varchar(10),
			country        varchar(2),
			cc2            varchar(600),
			admin1         varchar(20),
			admin2         varchar(80),
			admin3         varchar(20),
			admin4         varchar(20),
			population     bigint,
			elevation      int,
			gtopo30        int,
			timezone       varchar(40),
			moddate        date)`,
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
			postal               character varying(60),
			postal_format        character varying(200),
			postal_regex         character varying(200),
			languages            character varying(200),
			geonameId            int,
			neighbours           character varying(50),
			equivalent_fips_code character varying(3))`,
	}

	for _, q := range schemas {
		_, err := db.Exec(q)
		if err != nil {
			return errors.WithMessagef(err, "Failed to execute sql query '%s'", q)
		}
	}

	// Load the countryinfo table.
	if false {
		u := "http://download.geonames.org/export/dump/countryInfo.txt"
		resp, err := pester.Get(u)
		if err != nil {
			return errors.WithMessagef(err, "Failed to read country info from '%s'", u)
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
			if stmt == nil  {
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
					case "Postal":
						cn = "postal"
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
					default :
						return errors.Errorf("Failed to map column %s", fn)
					}
					columns = append(columns, cn)
				}

				placeholders := []string{}
				for i := 0; i < len(columns); i++ {
					placeholders = append(placeholders, "?")
				}

				q := "insert into countryinfo ("+strings.Join(columns, ",")+") values("+strings.Join(placeholders, ",")+")"
				q = db.Rebind(q)
				stmt, err = db.Prepare(q)
				if err != nil {
					return errors.WithMessagef(err, "Failed to prepare sql query '%s'", q)
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
	}

	// Load the geoname table.
	{
		u := "http://download.geonames.org/export/dump/allCountries.zip"
		resp, err := pester.Get(u)
		if err != nil {
			return errors.WithMessagef(err, "Failed to read countries from '%s'", u)
		}
		defer resp.Body.Close()

		br := bufio.NewReader(resp.Body)

		buff := bytes.NewBuffer([]byte{})
		size, err := io.Copy(buff, br)
		if err != nil {
			return err
		}

		b := bytes.NewReader(buff.Bytes())
		zr, err := zip.NewReader(b, size)
		if err != nil {
			return errors.WithStack(err)
		}

		q := "insert into geoname " +
			"(geonameid,name,asciiname,alternatenames,latitude,longitude,fclass,fcode,country,cc2,admin1,admin2,admin3,admin4,population,elevation,gtopo30,timezone,moddate) " +
			"values(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"
		q = db.Rebind(q)
		stmt, err := db.Prepare(q)
		if err != nil {
			return errors.WithMessagef(err, "Failed to prepare sql query '%s'", q)
		}

		for _, f := range zr.File {
			if f.Name == "readme.txt" {
				continue
			}

			fh, err := f.Open()
			if err != nil {
				return errors.WithStack(err)
			}

			scanner := bufio.NewScanner(fh)
			for scanner.Scan() {
				line := scanner.Text()

				// Skip comments.
				if strings.HasPrefix(line, "#") {
					continue
				}

				if strings.Contains(line, "\"") {
					line = strings.Replace(line, "\"", "\\\"", -1)
				}

				r := csv.NewReader(strings.NewReader(line))
				r.Comma = '\t' // Use tab-delimited instead of comma <---- here!
				r.LazyQuotes = true
				r.FieldsPerRecord = -1

				lines, err := r.ReadAll()
				if err != nil {
					return errors.WithStack(err)
				}

				for _, row := range lines {
					var args []interface{}
					for idx, v := range row {
						if v == "" {
							if idx == 0 || idx == 14 || idx == 15 {
								v = "0"
							}
						}
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
		}
	}


	return errors.New("not finished")


	queries := []string{
		// Countries...
		`DROP TABLE IF EXISTS countries`,
		`CREATE TABLE countries(
			  id  serial not null constraint countries_pkey primary key,
			  geoname_id int,
			  iso        char(2),
			  country    character varying(50),
			  capital    character varying(50),
			  created_at TIMESTAMP WITH TIME ZONE NOT NULL,
			  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
			  archived_at TIMESTAMP WITH TIME ZONE DEFAULT NULL)`,
		`create index idx_countries_deleted_at on countries (deleted_at)`,
		`insert into countries(geoname_id, iso, country, capital, created_at, updated_at)
			select geonameId, iso_alpha2, country, capital, NOW(), NOW()
			from countryinfo`,
		// Regions...
		`DROP TABLE IF EXISTS regions`,
		`CREATE TABLE regions (
			id serial not null constraint regions_pkey primary key,
			country_id int,
			geoname_id int,
			name       varchar(200),
			ascii_name varchar(200),
			adm        varchar(20),
			country    char(2),
			latitude   float,
			longitude  float,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
			archived_at TIMESTAMP WITH TIME ZONE DEFAULT NULL)`,
		`create index idx_regions_deleted_at on regions (deleted_at)`,
		`insert into regions(country_id, geoname_id, name, ascii_name, adm, country, latitude, longitude, created_at, updated_at)
			select c.id,
				   g.geonameid,
				   g.name,
				   g.asciiname,
				   g.admin1,
				   c.iso,
				   g.latitude,
				   g.longitude,
				   to_timestamp(TO_CHAR(g.moddate, 'YYYY-MM-DD'), 'YYYY-MM-DD'),
				   to_timestamp(TO_CHAR(g.moddate, 'YYYY-MM-DD'), 'YYYY-MM-DD')
			from countries as c
				   inner join geoname as g on c.iso = g.country and g.fcode like 'ADM1'`,
		// cities
		`DROP TABLE IF EXISTS cities`,
		`CREATE TABLE cities (
			id serial not null constraint cities_pkey primary key,
			country_id int,
			region_id  int,
			geoname_id int,
			name       varchar(200),
			ascii_name varchar(200),
			latitude   float,
			longitude  float,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
			archived_at TIMESTAMP WITH TIME ZONE DEFAULT NULL)`,
		`create index idx_cities_deleted_at on cities (deleted_at)`,
		`insert into cities(country_id, region_id, geoname_id, name, ascii_name, latitude, longitude, created_at, updated_at)
			select r.country_id,
				r.id,
				g.geonameid,
				g.name,
				g.asciiname,
				g.latitude,
				g.longitude,
				to_timestamp(TO_CHAR(g.moddate, 'YYYY-MM-DD'), 'YYYY-MM-DD'),
				to_timestamp(TO_CHAR(g.moddate, 'YYYY-MM-DD'), 'YYYY-MM-DD')
			from geoname as g
			join regions as r on r.adm = g.admin1
				and r.country = g.country
				and (g.fcode in ('PPLC', 'PPLA') or (g.fcode like 'PPLA%' and g.population >= 50000));`,

	}

	tx, err := db.Begin()
	if err != nil {
		return errors.WithStack(err)
	}

	for _, q := range queries {
		_, err = tx.Exec(q)
		if err != nil {
			return errors.WithMessagef(err, "Failed to execute sql query '%s'", q)
		}
	}

	err = tx.Commit()
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

*/
