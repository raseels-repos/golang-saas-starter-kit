package main

import (
	"database/sql"
	"log"

	"github.com/gitwak/sqlxmigrate"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
)

// migrationList returns a list of migrations to be executed. If the id of the
// migration already exists in the migrations table it will be skipped.
func migrationList(db *sqlx.DB, log *log.Logger) []*sqlxmigrate.Migration {
	return []*sqlxmigrate.Migration{
		// create table users
		{
			ID: "20190522-01a",
			Migrate: func(tx *sql.Tx) error {
				q := `CREATE TABLE IF NOT EXISTS users (
					  id char(36) NOT NULL,
					  email varchar(200) NOT NULL,
					  title varchar(100) NOT NULL DEFAULT '',
					  first_name varchar(200) NOT NULL DEFAULT '',
					  last_name varchar(200) NOT NULL DEFAULT '',
					  password_hash varchar(200) NOT NULL,
					  password_reset varchar(200) DEFAULT NULL,
					  password_salt varchar(200) NOT NULL,
					  phone varchar(20) NOT NULL DEFAULT '',
					  status enum('active','disabled') NOT NULL DEFAULT 'active',
					  timezone varchar(128) NOT NULL DEFAULT 'America/Anchorage',
					  created_at timestamp(0) NOT NULL DEFAULT CURRENT_TIMESTAMP,
					  updated_at timestamp(0) DEFAULT NULL,
					  deleted_at timestamp(0) DEFAULT NULL,
					  PRIMARY KEY (id),
					  CONSTRAINT email UNIQUE  (email)
					) ;`
				if _, err := tx.Exec(q); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q)
				}
				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				q := `DROP TABLE IF EXISTS users`
				if _, err := tx.Exec(q); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q)
				}
				return nil
			},
		},
		// create new table accounts
		{
			ID: "20190522-01b",
			Migrate: func(tx *sql.Tx) error {
				q := `CREATE TABLE IF NOT EXISTS accounts (
					  id char(36) NOT NULL,
					  name varchar(255) NOT NULL,
					  address1 varchar(255) NOT NULL DEFAULT '',
					  address2 varchar(255) NOT NULL DEFAULT '',
					  city varchar(100) NOT NULL DEFAULT '',
					  region varchar(255) NOT NULL DEFAULT '',
					  country varchar(255) NOT NULL DEFAULT '',
					  zipcode varchar(20) NOT NULL DEFAULT '',
					  status enum('active','pending','disabled') NOT NULL DEFAULT 'active',
					  timezone varchar(128) NOT NULL DEFAULT 'America/Anchorage',
					  signup_user_id char(36) DEFAULT NULL,
					  billing_user_id char(36) DEFAULT NULL,
					  created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
					  updated_at datetime DEFAULT NULL,
					  deleted_at datetime DEFAULT NULL,
					  PRIMARY KEY (id),
					  UNIQUE KEY name (name)
					) ENGINE=InnoDB DEFAULT CHARSET=utf8;`
				if _, err := tx.Exec(q); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q)
				}
				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				q := `DROP TABLE IF EXISTS accounts`
				if _, err := tx.Exec(q); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q)
				}
				return nil
			},
		},
		// create new table user_accounts
		{
			ID: "20190522-01c",
			Migrate: func(tx *sql.Tx) error {
				q1 := `CREATE TYPE IF NOT EXISTS role_t as enum('admin', 'user');`
				if _, err := tx.Exec(q1); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q1)
				}

				q2 := `CREATE TABLE IF NOT EXISTS users_accounts (
					  id char(36) NOT NULL,
					  account_id char(36) NOT NULL,
					  user_id ichar(36) NOT NULL,
					  roles role_t[] NOT NULL,
					  created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
					  updated_at datetime DEFAULT NULL,
					  deleted_at datetime DEFAULT NULL,
					  PRIMARY KEY (id),
					  UNIQUE KEY user_account (user_id,account_id)
					) ENGINE=InnoDB DEFAULT CHARSET=utf8;`
				if _, err := tx.Exec(q1); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q2)
				}

				return nil
			},
			Rollback: func(tx *sql.Tx) error {
				q1 := `DROP TYPE IF EXISTS role_t`
				if _, err := tx.Exec(q1); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q1)
				}

				q2 := `DROP TABLE IF EXISTS users_accounts`
				if _, err := tx.Exec(q2); err != nil {
					return errors.WithMessagef(err, "Query failed %s", q2)
				}

				return nil
			},
		},
	}
}
