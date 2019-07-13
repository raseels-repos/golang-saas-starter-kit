package tests

import (
	"context"
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/internal/schema"
	"io"
	"log"
	"os"
	"runtime/debug"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/docker"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/jmoiron/sqlx"
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

	// ============================================================
	// Init AWS Session

	awsSession := session.Must(session.NewSession())

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
				if err != io.EOF {
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

		// Execute the migrations
		if err = schema.Migrate(masterDB, log); err != nil {
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
	values := web.Values{
		TraceID: uint64(time.Now().UnixNano()),
		Now:     time.Now(),
	}

	return context.WithValue(context.Background(), web.KeyValues, &values)
}
