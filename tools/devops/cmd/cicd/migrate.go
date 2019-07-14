package cicd

import (
	"encoding/json"
	"log"
	"net/url"
	"path/filepath"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/internal/schema"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	sqltrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/database/sql"
	sqlxtrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/jmoiron/sqlx"
	"gopkg.in/go-playground/validator.v9"
)

// MigrateFlags defines the flags used for executing schema migration.
type MigrateFlags struct {
	// Required flags.
	Env string `validate:"oneof=dev stage prod" example:"dev"`

	// Optional flags.
	ProjectRoot string `validate:"omitempty" example:"."`
	ProjectName string ` validate:"omitempty" example:"example-project"`
}

// migrateRequest defines the details needed to execute a service build.
type migrateRequest struct {
	Env         string `validate:"oneof=dev stage prod"`
	ProjectRoot string `validate:"required"`
	ProjectName string `validate:"required"`
	GoModFile   string `validate:"required"`
	GoModName   string `validate:"required"`

	AwsCreds    awsCredentials `validate:"required,dive,required"`
	_awsSession *session.Session

	flags MigrateFlags
}

// awsSession returns the current AWS session for the serviceDeployRequest.
func (r *migrateRequest) awsSession() *session.Session {
	if r._awsSession == nil {
		r._awsSession = r.AwsCreds.Session()
	}

	return r._awsSession
}

// NewMigrateRequest generates a new request for executing schema migration for a given set of CLI flags.
func NewMigrateRequest(log *log.Logger, flags MigrateFlags) (*migrateRequest, error) {

	// Validates specified CLI flags map to struct successfully.
	log.Println("Validate flags.")
	{
		errs := validator.New().Struct(flags)
		if errs != nil {
			return nil, errs
		}
		log.Printf("\t%s\tFlags ok.", tests.Success)
	}

	// Generate a migrate request using CLI flags and AWS credentials.
	log.Println("Generate migrate request.")
	var req migrateRequest
	{

		// Define new migrate request.
		req = migrateRequest{
			Env:         flags.Env,
			ProjectRoot: flags.ProjectRoot,
			ProjectName: flags.ProjectName,

			flags: flags,
		}

		// When project root directory is empty or set to current working path, then search for the project root by locating
		// the go.mod file.
		log.Println("\tDetermining the project root directory.")
		{
			if req.ProjectRoot == "" || req.ProjectRoot == "." {
				log.Println("\tAttempting to location project root directory from current working directory.")

				var err error
				req.GoModFile, err = findProjectGoModFile()
				if err != nil {
					return nil, err
				}
				req.ProjectRoot = filepath.Dir(req.GoModFile)
			} else {
				log.Printf("\t\tUsing supplied project root directory '%s'.\n", req.ProjectRoot)
				req.GoModFile = filepath.Join(req.ProjectRoot, "go.mod")
			}
			log.Printf("\t\t\tproject root: %s", req.ProjectRoot)
			log.Printf("\t\t\tgo.mod: %s", req.GoModFile)
		}

		log.Println("\tExtracting go module name from go.mod.")
		{
			var err error
			req.GoModName, err = loadGoModName(req.GoModFile)
			if err != nil {
				return nil, err
			}
			log.Printf("\t\t\tmodule name: %s", req.GoModName)
		}

		log.Println("\tDetermining the project name.")
		{
			if req.ProjectName != "" {
				log.Printf("\t\tUse provided value.")
			} else {
				req.ProjectName = filepath.Base(req.GoModName)
				log.Printf("\t\tSet from go module.")
			}
			log.Printf("\t\t\tproject name: %s", req.ProjectName)
		}

		// Verifies AWS credentials specified as environment variables.
		log.Println("\tVerify AWS credentials.")
		{
			var err error
			req.AwsCreds, err = GetAwsCredentials(req.Env)
			if err != nil {
				return nil, err
			}
			if req.AwsCreds.UseRole {
				log.Printf("\t\t\tUsing role")
			} else {
				log.Printf("\t\t\tAccessKeyID: '%s'", req.AwsCreds.AccessKeyID)
			}

			log.Printf("\t\t\tRegion: '%s'", req.AwsCreds.Region)
			log.Printf("\t%s\tAWS credentials valid.", tests.Success)
		}
	}

	return &req, nil
}

// Run is the main entrypoint for migration of database schema for a given target environment.
func Migrate(log *log.Logger, req *migrateRequest) error {

	// Load the database details.
	var db DB
	{
		log.Println("Get Database Details from AWS Secret Manager")

		dbId := dBInstanceIdentifier(req.ProjectName, req.Env)

		// Secret ID used to store the DB username and password across deploys.
		dbSecretId := secretID(req.ProjectName, req.Env, dbId)

		// Retrieve the current secret value if something is stored.
		{
			sm := secretsmanager.New(req.awsSession())
			res, err := sm.GetSecretValue(&secretsmanager.GetSecretValueInput{
				SecretId: aws.String(dbSecretId),
			})
			if err != nil {
				if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != secretsmanager.ErrCodeResourceNotFoundException {
					return errors.Wrapf(err, "Failed to get value for secret id %s", dbSecretId)
				}
			} else {
				err = json.Unmarshal([]byte(*res.SecretString), &db)
				if err != nil {
					return errors.Wrap(err, "Failed to json decode db credentials")
				}
			}

			log.Printf("\t%s\tDatabase credentials found.", tests.Success)
		}
	}

	// Start Database and run the migration.
	{
		log.Println("Proceed with schema migration")

		var dbUrl url.URL
		{
			// Query parameters.
			var q url.Values = make(map[string][]string)

			// Handle SSL Mode
			if db.DisableTLS {
				q.Set("sslmode", "disable")
			} else {
				q.Set("sslmode", "require")
			}

			// Construct url.
			dbUrl = url.URL{
				Scheme:   db.Driver,
				User:     url.UserPassword(db.User, db.Pass),
				Host:     db.Host,
				Path:     db.Database,
				RawQuery: q.Encode(),
			}
		}

		log.Printf("\t\tOpen database connection")
		// Register informs the sqlxtrace package of the driver that we will be using in our program.
		// It uses a default service name, in the below case "postgres.db". To use a custom service
		// name use RegisterWithServiceName.
		sqltrace.Register(db.Driver, &pq.Driver{}, sqltrace.WithServiceName("devops:migrate"))
		masterDb, err := sqlxtrace.Open(db.Driver, dbUrl.String())
		if err != nil {
			return errors.WithStack(err)
		}
		defer masterDb.Close()

		// Start Migrations
		log.Printf("\t\tStart migrations.")
		if err = schema.Migrate(masterDb, log); err != nil {
			return errors.WithStack(err)
		}

		log.Printf("\t%s\tMigrate complete.", tests.Success)
	}

	return nil
}
