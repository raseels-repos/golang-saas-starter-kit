package dbtable2crud

import (
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"
)

type psqlColumn struct {
	Table                   string
	Column                  string
	ColumnId                int64
	NotNull                 bool
	DataTypeFull            string
	DataTypeName            string
	DataTypeLength          *int
	NumericPrecision        *int
	NumericScale            *int
	IsPrimaryKey            bool
	PrimaryKeyName          *string
	IsUniqueKey             bool
	UniqueKeyName           *string
	IsForeignKey            bool
	ForeignKeyName          *string
	ForeignKeyColumnId      pq.Int64Array
	ForeignKeyTable         *string
	ForeignKeyLocalColumnId pq.Int64Array
	DefaultFull             *string
	DefaultValue            *string
	IsEnum                  bool
	EnumTypeId              *string
	EnumValues              []string
}

// descTable lists all the columns for a table.
func descTable(db *sqlx.DB, dbName, dbTable string) ([]psqlColumn, error) {

	queryStr := fmt.Sprintf(`SELECT
		c.relname as table,
		f.attname as column,
		f.attnum as columnId,
		f.attnotnull as not_null,
		pg_catalog.format_type(f.atttypid,f.atttypmod) AS data_type_full,
		t.typname AS data_type_name,
		CASE WHEN f.atttypmod >= 0 AND t.typname <> 'numeric'THEN (f.atttypmod - 4) --first 4 bytes are for storing actual length of data
			END AS data_type_length,
		CASE WHEN t.typname = 'numeric' THEN (((f.atttypmod - 4) >> 16) & 65535)
			END AS numeric_precision,
		CASE WHEN t.typname = 'numeric' THEN ((f.atttypmod - 4)& 65535 )
			END AS numeric_scale,
		CASE WHEN p.contype = 'p' THEN true ELSE false
			END AS is_primary_key,
		CASE WHEN p.contype = 'p' THEN p.conname
			END AS primary_key_name,
		CASE WHEN p.contype = 'u' THEN true ELSE false
			END AS is_unique_key,
		CASE WHEN p.contype = 'u' THEN p.conname
			END AS unique_key_name,
		CASE WHEN p.contype = 'f' THEN true ELSE false
			END AS is_foreign_key,
		CASE WHEN p.contype = 'f' THEN p.conname
			END AS foreignkey_name,
		CASE WHEN p.contype = 'f' THEN p.confkey
			END AS foreign_key_columnid,
		CASE WHEN p.contype = 'f' THEN g.relname
			END AS foreign_key_table,
		CASE WHEN p.contype = 'f' THEN p.conkey
			END AS foreign_key_local_column_id,
		CASE WHEN f.atthasdef = 't' THEN d.adsrc
			END AS default_value,
		CASE WHEN t.typtype = 'e' THEN true ELSE false
			END AS is_enum,
		CASE WHEN t.typtype = 'e' THEN t.oid 
			END AS enum_type_id
	FROM pg_attribute f
	JOIN pg_class c ON c.oid = f.attrelid
	JOIN pg_type t ON t.oid = f.atttypid
	LEFT JOIN pg_attrdef d ON d.adrelid = c.oid AND d.adnum = f.attnum
	LEFT JOIN pg_namespace n ON n.oid = c.relnamespace
	LEFT JOIN pg_constraint p ON p.conrelid = c.oid AND f.attnum = ANY (p.conkey)
	LEFT JOIN pg_class AS g ON p.confrelid = g.oid
	WHERE c.relkind = 'r'::char
	AND f.attisdropped = false
		AND c.relname = '%s' 
		AND f.attnum > 0 
	ORDER BY f.attnum
	;`, dbTable) // AND n.nspname = '%s'

	rows, err := db.Query(queryStr)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", queryStr)
		return nil, err
	}

	// iterate over each row
	var resp []psqlColumn
	for rows.Next() {
		var c psqlColumn
		err = rows.Scan(&c.Table, &c.Column, &c.ColumnId, &c.NotNull, &c.DataTypeFull, &c.DataTypeName, &c.DataTypeLength, &c.NumericPrecision, &c.NumericScale, &c.IsPrimaryKey, &c.PrimaryKeyName, &c.IsUniqueKey, &c.UniqueKeyName, &c.IsForeignKey, &c.ForeignKeyName, &c.ForeignKeyColumnId, &c.ForeignKeyTable, &c.ForeignKeyLocalColumnId, &c.DefaultFull, &c.IsEnum, &c.EnumTypeId)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", queryStr)
			return nil, err
		}

		if c.DefaultFull != nil {
			defaultValue := *c.DefaultFull

			// "'active'::project_status_t"
			defaultValue = strings.Split(defaultValue, "::")[0]
			c.DefaultValue = &defaultValue
		}

		resp = append(resp, c)
	}

	for colIdx, dbCol := range resp {
		if !dbCol.IsEnum {
			continue
		}

		queryStr := fmt.Sprintf(`SELECT e.enumlabel
		  FROM pg_enum AS e
		 WHERE e.enumtypid = '%s'
		 ORDER BY e.enumsortorder`, *dbCol.EnumTypeId)

		rows, err := db.Query(queryStr)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", queryStr)
			return nil, err
		}

		for rows.Next() {
			var v string
			err = rows.Scan(&v)
			if err != nil {
				err = errors.Wrapf(err, "query - %s", queryStr)
				return nil, err
			}
			dbCol.EnumValues = append(dbCol.EnumValues, v)
		}

		resp[colIdx] = dbCol
	}

	return resp, nil
}
