package schema

import (
	"context"
	"log"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"github.com/geeks-accelerator/sqlxmigrate"
	"github.com/jmoiron/sqlx"
)

// Migrate is the entry point for performing init schema and running all the migrations.
func Migrate(ctx context.Context, targetEnv webcontext.Env, masterDb *sqlx.DB, log *log.Logger, isUnittest bool) error {

	// Set the context with the required values to
	// process the request.
	v := webcontext.Values{
		Now: time.Now(),
		Env: targetEnv,
	}
	ctx = context.WithValue(ctx, webcontext.KeyValues, &v)

	// Load list of Schema migrations and init new sqlxmigrate client
	migrations := migrationList(ctx, masterDb, log, isUnittest)
	m := sqlxmigrate.New(masterDb, sqlxmigrate.DefaultOptions, migrations)
	m.SetLogger(log)

	// Append any schema that need to be applied if this is a fresh migration
	// ie. the migrations database table does not exist.
	m.InitSchema(initSchema(ctx, masterDb, log, isUnittest))

	// Execute the migrations
	return m.Migrate()
}
