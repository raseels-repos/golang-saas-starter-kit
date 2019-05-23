package main

import (
	"encoding/json"
	"expvar"
	"github.com/lib/pq"
	"log"
	"net/url"
	"os"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/flag"
	"github.com/gitwak/sqlxmigrate"
	"github.com/kelseyhightower/envconfig"
	_ "github.com/lib/pq"
	sqltrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/database/sql"
	sqlxtrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/jmoiron/sqlx"
)

// build is the git version of this program. It is set using build flags in the makefile.
var build = "develop"

func main() {
	// =========================================================================
	// Logging

	log := log.New(os.Stdout, "Schema : ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	// =========================================================================
	// Configuration
	var cfg struct {
		Env string `default:"dev" envconfig:"ENV"`
		DB  struct {
			Host       string `default:"127.0.0.1:5433" envconfig:"DB_HOST"`
			User       string `default:"postgres" envconfig:"DB_USER"`
			Pass       string `default:"postgres" envconfig:"DB_PASS" json:"-"` // don't print
			Database   string `default:"shared" envconfig:"DB_DATABASE"`
			Driver     string `default:"postgres" envconfig:"DB_DRIVER"`
			Timezone   string `default:"utc" envconfig:"DB_TIMEZONE"`
			DisableTLS bool   `default:"false" envconfig:"DB_DISABLE_TLS"`
		}
	}

	// The prefix used for loading env variables.
	// ie: export SCHEMA_ENV=dev
	envKeyPrefix := "SCHEMA"

	// For additional details refer to https://github.com/kelseyhightower/envconfig
	if err := envconfig.Process(envKeyPrefix, &cfg); err != nil {
		log.Fatalf("main : Parsing Config : %v", err)
	}

	if err := flag.Process(&cfg); err != nil {
		if err != flag.ErrHelp {
			log.Fatalf("main : Parsing Command Line : %v", err)
		}
		return // We displayed help.
	}

	// =========================================================================
	// Log App Info

	// Print the build version for our logs. Also expose it under /debug/vars.
	expvar.NewString("build").Set(build)
	log.Printf("main : Started : Application Initializing version %q", build)
	defer log.Println("main : Completed")

	// Print the config for our logs. It's important to any credentials in the config
	// that could expose a security risk are excluded from being json encoded by
	// applying the tag `json:"-"` to the struct var.
	{
		cfgJSON, err := json.MarshalIndent(cfg, "", "    ")
		if err != nil {
			log.Fatalf("main : Marshalling Config to JSON : %v", err)
		}
		log.Printf("main : Config : %v\n", string(cfgJSON))
	}

	// =========================================================================
	// Start Database
	var dbUrl url.URL
	{
		// Query parameters.
		var q url.Values

		// Handle SSL Mode
		if cfg.DB.DisableTLS {
			q.Set("sslmode", "disable")
		} else {
			q.Set("sslmode", "require")
		}

		q.Set("timezone", cfg.DB.Timezone)

		// Construct url.
		dbUrl = url.URL{
			Scheme:   cfg.DB.Driver,
			User:     url.UserPassword(cfg.DB.User, cfg.DB.Pass),
			Host:     cfg.DB.Host,
			Path:     cfg.DB.Database,
			RawQuery: q.Encode(),
		}
	}

	// Register informs the sqlxtrace package of the driver that we will be using in our program.
	// It uses a default service name, in the below case "postgres.db". To use a custom service
	// name use RegisterWithServiceName.
	sqltrace.Register(cfg.DB.Driver, &pq.Driver{}, sqltrace.WithServiceName("my-service"))
	masterDb, err := sqlxtrace.Open(cfg.DB.Driver, dbUrl.String())
	if err != nil {
		log.Fatalf("main : Register DB : %s : %v", cfg.DB.Driver, err)
	}
	defer masterDb.Close()

	// =========================================================================
	// Start Migrations

	// Load list of Schema migrations and init new sqlxmigrate client
	migrations := migrationList(masterDb, log)
	m := sqlxmigrate.New(masterDb, sqlxmigrate.DefaultOptions, migrations)

	// Append any schema that need to be applied if this is a fresh migration
	// ie. the migrations database table does not exist.
	m.InitSchema(initSchema(masterDb, log))

	// Execute the migrations
	if err = m.Migrate(); err != nil {
		log.Fatalf("main : Migrate : %v", err)
	}
	log.Printf("main : Migrate : Completed")
}
