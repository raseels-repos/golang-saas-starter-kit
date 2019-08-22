package schema

import (
	"context"
	"log"

	"github.com/jmoiron/sqlx"
)

// initSchema runs before any migrations are executed. This happens when no other migrations
// have previously been executed.
func initSchema(ctx context.Context, db *sqlx.DB, log *log.Logger, isUnittest bool) func(*sqlx.DB) error {
	f := func(db *sqlx.DB) error {
		return nil
	}

	return f
}
