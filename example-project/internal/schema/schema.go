package schema

import (
	"log"

	"github.com/geeks-accelerator/sqlxmigrate"
	"github.com/jmoiron/sqlx"
)

func Migrate(masterDb *sqlx.DB, log *log.Logger) error {
	// Load list of Schema migrations and init new sqlxmigrate client
	migrations := migrationList(masterDb, log)
	m := sqlxmigrate.New(masterDb, sqlxmigrate.DefaultOptions, migrations)
	m.SetLogger(log)

	// Append any schema that need to be applied if this is a fresh migration
	// ie. the migrations database table does not exist.
	m.InitSchema(initSchema(masterDb, log))

	// Execute the migrations
	return m.Migrate()
}
