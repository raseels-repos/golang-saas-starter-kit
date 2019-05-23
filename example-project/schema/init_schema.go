package main

import (
	"github.com/jmoiron/sqlx"
	"log"
)

// initSchema runs before any migrations are executed. This happens when no other migrations
// have previously been executed.
func initSchema(db *sqlx.DB, log *log.Logger) func(*sqlx.DB) error {
	f := func(*sqlx.DB) error {

		return nil
	}

	return f
}
