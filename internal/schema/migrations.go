package schema

import (
	"database/sql"
	"log"

	"github.com/geeks-accelerator/sqlxmigrate"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
)

// migrationList returns a list of migrations to be executed. If the id of the
// migration already exists in the migrations table it will be skipped.
func migrationList(db *sqlx.DB, log *log.Logger) []*sqlxmigrate.Migration {
	return []*sqlxmigrate.Migration{
		// Create table users.
		{
			ID: "20190522-01a",
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
					return errors.WithMessagef(err, "Query failed %s", q1)
				}
				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				q1 := `DROP TABLE IF EXISTS users`
				if _, err := tx.Exec(q1); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q1)
				}
				return nil
			},
		},
		// Create new table accounts.
		{
			ID: "20190522-01b",
			Migrate: func(tx *sql.Tx) error {
				q1 := `CREATE TYPE account_status_t as enum('active','pending','disabled')`
				if _, err := tx.Exec(q1); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q1)
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
					return errors.WithMessagef(err, "Query failed %s", q2)
				}
				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				q1 := `DROP TYPE account_status_t`
				if _, err := tx.Exec(q1); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q1)
				}

				q2 := `DROP TABLE IF EXISTS accounts`
				if _, err := tx.Exec(q2); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q2)
				}
				return nil
			},
		},
		// Create new table user_accounts.
		{
			ID: "20190522-01d",
			Migrate: func(tx *sql.Tx) error {
				q1 := `CREATE TYPE user_account_role_t as enum('admin', 'user')`
				if _, err := tx.Exec(q1); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q1)
				}

				q2 := `CREATE TYPE user_account_status_t as enum('active', 'invited','disabled')`
				if _, err := tx.Exec(q2); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q2)
				}

				q3 := `CREATE TABLE IF NOT EXISTS users_accounts (
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
				if _, err := tx.Exec(q3); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q3)
				}

				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				q1 := `DROP TYPE user_account_role_t`
				if _, err := tx.Exec(q1); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q1)
				}

				q2 := `DROP TYPE userr_account_status_t`
				if _, err := tx.Exec(q2); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q2)
				}

				q3 := `DROP TABLE IF EXISTS users_accounts`
				if _, err := tx.Exec(q3); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q3)
				}

				return nil
			},
		},
		// Create new table projects.
		{
			ID: "20190622-01",
			Migrate: func(tx *sql.Tx) error {
				q1 := `CREATE TYPE project_status_t as enum('active','disabled')`
				if _, err := tx.Exec(q1); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q1)
				}

				q2 := `CREATE TABLE IF NOT EXISTS projects (
					  id char(36) NOT NULL,
					  account_id char(36) NOT NULL REFERENCES accounts(id) ON DELETE SET NULL,
					  name varchar(255) NOT NULL,
					  status project_status_t NOT NULL DEFAULT 'active',
					  created_at TIMESTAMP WITH TIME ZONE NOT NULL,
					  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
					  archived_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
					  PRIMARY KEY (id)
					)`
				if _, err := tx.Exec(q2); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q2)
				}
				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				q1 := `DROP TYPE project_status_t`
				if _, err := tx.Exec(q1); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q1)
				}

				q2 := `DROP TABLE IF EXISTS projects`
				if _, err := tx.Exec(q2); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q2)
				}
				return nil
			},
		},
		// Split users.name into first_name and last_name columns.
		{
			ID: "201907-29-01a",
			Migrate: func(tx *sql.Tx) error {
				q1 := `ALTER TABLE users 
					  RENAME COLUMN name to first_name;`
				if _, err := tx.Exec(q1); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q1)
				}

				q2 := `ALTER TABLE users 
					  ADD last_name varchar(200) NOT NULL DEFAULT '';`
				if _, err := tx.Exec(q2); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q2)
				}

				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				q1 := `DROP TABLE IF EXISTS users`
				if _, err := tx.Exec(q1); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q1)
				}
				return nil
			},
		},
	}
}
