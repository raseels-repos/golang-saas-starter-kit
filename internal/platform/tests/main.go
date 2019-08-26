package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/docker"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/schema"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/jmoiron/sqlx"
	"github.com/kelseyhightower/envconfig"
)

// Success and failure markers.
const (
	Success = "\u2713"
	Failed  = "\u2717"
)

// Test owns state for running/shutting down tests.
type Test struct {
	Log        *log.Logger
	MasterDB   *sqlx.DB
	container  *docker.Container
	AwsSession *session.Session
}

// Flag used to disable setting up database.
var DisableDb bool

// New is the entry point for tests.
func New() *Test {

	// =========================================================================
	// Logging

	log := log.New(os.Stdout, "TEST : ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	// =========================================================================
	// Configuration
	var cfg struct {
		Aws struct {
			AccessKeyID     string `envconfig:"AWS_ACCESS_KEY_ID"`              // WEB_API_AWS_AWS_ACCESS_KEY_ID or AWS_ACCESS_KEY_ID
			SecretAccessKey string `envconfig:"AWS_SECRET_ACCESS_KEY" json:"-"` // don't print
			Region          string `default:"us-west-2" envconfig:"AWS_REGION"`
			UseRole         bool   `envconfig:"AWS_USE_ROLE"`
		}
	}

	// For additional details refer to https://github.com/kelseyhightower/envconfig
	if err := envconfig.Process("TESTS", &cfg); err != nil {
		log.Fatalf("startup : Parsing Config : %+v", err)
	}

	// AWS access keys are required, if roles are enabled, remove any placeholders.
	if cfg.Aws.UseRole {
		cfg.Aws.AccessKeyID = ""
		cfg.Aws.SecretAccessKey = ""

		// Get an AWS session from an implicit source if no explicit
		// configuration is provided. This is useful for taking advantage of
		// EC2/ECS instance roles.
		if cfg.Aws.Region == "" {
			sess := session.Must(session.NewSession())
			md := ec2metadata.New(sess)

			var err error
			cfg.Aws.Region, err = md.Region()
			if err != nil {
				log.Fatalf("startup : Load region of ecs metadata : %+v", err)
			}
		}
	}

	// Print the config for our logs. It's important to any credentials in the config
	// that could expose a security risk are excluded from being json encoded by
	// applying the tag `json:"-"` to the struct var.
	{
		cfgJSON, err := json.MarshalIndent(cfg, "", "    ")
		if err != nil {
			log.Fatalf("startup : Marshalling Config to JSON : %+v", err)
		}
		log.Printf("startup : Config : %v\n", string(cfgJSON))
	}

	// ============================================================
	// Init AWS Session
	var awsSession *session.Session
	if cfg.Aws.UseRole {
		// Get an AWS session from an implicit source if no explicit
		// configuration is provided. This is useful for taking advantage of
		// EC2/ECS instance roles.
		awsSession = session.Must(session.NewSession())
		if cfg.Aws.Region != "" {
			awsSession.Config.WithRegion(cfg.Aws.Region)
		}

		log.Printf("startup : AWS : Using role.\n")
	} else if cfg.Aws.AccessKeyID != "" {
		creds := credentials.NewStaticCredentials(cfg.Aws.AccessKeyID, cfg.Aws.SecretAccessKey, "")
		awsSession = session.New(&aws.Config{Region: aws.String(cfg.Aws.Region), Credentials: creds})

		log.Printf("startup : AWS : Using static credentials\n")
	}

	// ============================================================
	// Startup Postgres container

	var (
		masterDB  *sqlx.DB
		container *docker.Container
	)
	if !DisableDb {
		var err error
		container, err = docker.StartPostgres(log)
		if err != nil {
			log.Fatalln(err)
		}

		// ============================================================
		// Configuration

		dbHost := fmt.Sprintf("postgres://%s:%s@127.0.0.1:%s/%s?timezone=UTC&sslmode=disable", container.User, container.Pass, container.Port, container.Database)

		// ============================================================
		// Start Postgres

		log.Println("main : Started : Initialize Postgres")
		for i := 0; i <= 20; i++ {
			masterDB, err = sqlx.Open("postgres", dbHost)
			if err != nil {
				break
			}

			// Make sure the database is ready for queries.
			_, err = masterDB.Exec("SELECT 1")
			if err != nil {
				if err != io.EOF && !strings.Contains(err.Error(), "connection reset by peer") {
					break
				}
				time.Sleep(time.Second)
			} else {
				break
			}
		}

		if err != nil {
			log.Fatalf("startup : Register DB : %v", err)
		}

		// Set the context with the required values to
		// process the request.
		v := webcontext.Values{
			Now: time.Now(),
			Env: webcontext.Env_Dev,
		}
		ctx := context.WithValue(context.Background(), webcontext.KeyValues, &v)

		// Execute the migrations
		if err = schema.Migrate(ctx, v.Env, masterDB, log, true); err != nil {
			log.Fatalf("main : Migrate : %v", err)
		}
		log.Printf("main : Migrate : Completed")
	}

	return &Test{log, masterDB, container, awsSession}
}

// TearDown is used for shutting down tests. Calling this should be
// done in a defer immediately after calling New.
func (t *Test) TearDown() {
	if t.MasterDB != nil {
		t.MasterDB.Close()
	}

	if t.container != nil {
		if err := docker.StopPostgres(t.Log, t.container); err != nil {
			t.Log.Println(err)
		}
	}
}

// Recover is used to prevent panics from allowing the test to cleanup.
func Recover(t *testing.T) {
	if r := recover(); r != nil {
		t.Fatal("Unhandled Exception:", string(debug.Stack()))
	}
}

// Context returns an app level context for testing.
func Context() context.Context {
	values := webcontext.Values{
		TraceID:   uint64(time.Now().UnixNano()),
		Now:       time.Now(),
		RequestIP: "68.69.35.104",
		Env:       "dev",
	}

	return context.WithValue(context.Background(), webcontext.KeyValues, &values)
}
