package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/schema"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/urfave/cli"
	sqltrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/database/sql"
	sqlxtrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/jmoiron/sqlx"
)

// service is the name of the program used for logging, tracing and the
// the prefix used for loading env variables
// ie: export SCHEMA_ENV=dev
var service = "SCHEMA"

// DB defines the database credentials stored in AWS Secrets Manager as defined by devops.
type DB struct {
	Host       string
	User       string
	Pass       string
	Database   string
	Driver     string
	DisableTLS bool
}

func main() {

	// =========================================================================
	// Logging
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	log.SetPrefix(service + " : ")
	log := log.New(os.Stdout, log.Prefix(), log.Flags())

	// =========================================================================
	// New CLI application.
	app := cli.NewApp()
	app.Name = "schema"
	app.Version = "1.0.0"
	app.Author = "Lee Brown"
	app.Email = "lee@geeksinthewoods.com"

	app.Commands = []cli.Command{
		{
			Name:    "migrate",
			Aliases: []string{"m"},
			Usage:   "run schema migration",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "env",
					Usage: fmt.Sprintf("target environment, one of [%s]",
						strings.Join(webcontext.EnvNames, ", ")),
					Value:  "dev",
					EnvVar: "ENV",
				},
				cli.StringFlag{
					Name:   "host",
					Usage:  "host",
					Value:  "127.0.0.1:5433",
					EnvVar: "SCHEMA_DB_HOST",
				},
				cli.StringFlag{
					Name:   "user",
					Usage:  "username",
					Value:  "postgres",
					EnvVar: "SCHEMA_DB_USER",
				},
				cli.StringFlag{
					Name:   "pass",
					Usage:  "password",
					Value:  "postgres",
					EnvVar: "SCHEMA_DB_PASS",
				},
				cli.StringFlag{
					Name:   "database",
					Usage:  "name of the default",
					Value:  "shared",
					EnvVar: "SCHEMA_DB_DATABASE",
				},
				cli.StringFlag{
					Name:   "driver",
					Usage:  "database drive to use for connection",
					Value:  "postgres",
					EnvVar: "SCHEMA_DB_DRIVER",
				},
				cli.BoolTFlag{
					Name:   "disable-tls",
					Usage:  "disable TLS for the database connection",
					EnvVar: "SCHEMA_DB_DISABLE_TLS",
				},
			},
			Action: func(c *cli.Context) error {
				targetEnv := c.String("env")
				var dbInfo = DB{
					Host:     c.String("host"),
					User:     c.String("user"),
					Pass:     c.String("pass"),
					Database: c.String("database"),

					Driver:     c.String("driver"),
					DisableTLS: c.Bool("disable-tls"),
				}

				return runMigrate(log, targetEnv, dbInfo)
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("%+v", err)
	}
}

// runMigrate executes the schema migration against the provided database connection details.
func runMigrate(log *log.Logger, targetEnv string, dbInfo DB) error {
	// =========================================================================
	// Start Database
	var dbUrl url.URL
	{
		// Query parameters.
		var q url.Values = make(map[string][]string)

		// Handle SSL Mode
		if dbInfo.DisableTLS {
			q.Set("sslmode", "disable")
		} else {
			q.Set("sslmode", "require")
		}

		// Construct url.
		dbUrl = url.URL{
			Scheme:   dbInfo.Driver,
			User:     url.UserPassword(dbInfo.User, dbInfo.Pass),
			Host:     dbInfo.Host,
			Path:     dbInfo.Database,
			RawQuery: q.Encode(),
		}
	}

	// Register informs the sqlxtrace package of the driver that we will be using in our program.
	// It uses a default service name, in the below case "postgres.db". To use a custom service
	// name use RegisterWithServiceName.
	sqltrace.Register(dbInfo.Driver, &pq.Driver{}, sqltrace.WithServiceName(service))
	masterDb, err := sqlxtrace.Open(dbInfo.Driver, dbUrl.String())
	if err != nil {
		log.Fatalf("main : Register DB : %s : %v", dbInfo.Driver, err)
	}
	defer masterDb.Close()

	// =========================================================================
	// Start Migrations

	ctx := context.Background()

	// Execute the migrations
	if err = schema.Migrate(ctx, targetEnv, masterDb, log, false); err != nil {
		return err
	}

	log.Printf("main : Migrate : Completed")
	return nil
}
