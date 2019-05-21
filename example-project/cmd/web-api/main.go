package main

import (
	"context"
	"encoding/json"
	"expvar"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/cmd/web-api/handlers"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/db"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/flag"
	itrace "geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/trace"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/kelseyhightower/envconfig"
	"go.opencensus.io/trace"
	awstrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/aws/aws-sdk-go/aws"
)

/*
ZipKin: http://localhost:9411
AddLoad: hey -m GET -c 10 -n 10000 "http://localhost:3000/v1/users"
expvarmon -ports=":3001" -endpoint="/metrics" -vars="requests,goroutines,errors,mem:memstats.Alloc"
*/

/*
Need to figure out timeouts for http service.
You might want to reset your DB_HOST env var during test tear down.
Service should start even without a DB running yet.
symbols in profiles: https://github.com/golang/go/issues/23376 / https://github.com/google/pprof/pull/366
*/

// build is the git version of this program. It is set using build flags in the makefile.
var build = "develop"

func main() {

	// =========================================================================
	// Logging

	log := log.New(os.Stdout, "WEB_API : ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	// =========================================================================
	// Configuration
	var cfg struct {
		Env  string `default:"dev" envconfig:"ENV"`
		HTTP struct {
			Host            string        `default:"0.0.0.0:3001" envconfig:"HTTP_HOST"`
			DebugHost       string        `default:"0.0.0.0:4000" envconfig:"HTTP_DEBUG_HOST"`
			ReadTimeout     time.Duration `default:"5s" envconfig:"HTTP_READ_TIMEOUT"`
			WriteTimeout    time.Duration `default:"5s" envconfig:"HTTP_WRITE_TIMEOUT"`
			ShutdownTimeout time.Duration `default:"5s" envconfig:"HTTP_SHUTDOWN_TIMEOUT"`
		}
		App struct {
			Name string `default:"web-app" envconfig:"APP_NAME"`
		}
		BuildInfo struct {
			CiCommitRefName     string `envconfig:"CI_COMMIT_REF_NAME"`
			CiCommitRefSlug     string `envconfig:"CI_COMMIT_REF_SLUG"`
			CiCommitSha         string `envconfig:"CI_COMMIT_SHA"`
			CiCommitTag         string `envconfig:"CI_COMMIT_TAG"`
			CiCommitTitle       string `envconfig:"CI_COMMIT_TITLE"`
			CiCommitDescription string `envconfig:"CI_COMMIT_DESCRIPTION"`
			CiJobId             string `envconfig:"CI_COMMIT_JOB_ID"`
			CiJobUrl            string `envconfig:"CI_COMMIT_JOB_URL"`
			CiPipelineId        string `envconfig:"CI_COMMIT_PIPELINE_ID"`
			CiPipelineUrl       string `envconfig:"CI_COMMIT_PIPELINE_URL"`
		}
		DB struct {
			DialTimeout time.Duration `default:"5s" envconfig:"DB_DIAL_TIMEOUT"`
			Host        string        `default:"mongo:27017/gotraining" envconfig:"DB_HOST"`
		}
		Trace struct {
			Host         string        `default:"http://tracer:3002/v1/publish" envconfig:"TRACE_HOST"`
			BatchSize    int           `default:"1000" envconfig:"TRACE_BATCH_SIZE"`
			SendInterval time.Duration `default:"15s" envconfig:"TRACE_SEND_INTERVAL"`
			SendTimeout  time.Duration `default:"500ms" envconfig:"TRACE_SEND_TIMEOUT"`
		}
		AwsAccount struct {
			AccessKeyID     string `envconfig:"AWS_ACCESS_KEY_ID"`
			SecretAccessKey string `envconfig:"AWS_SECRET_ACCESS_KEY"`
			Region          string `default:"us-east-1" envconfig:"AWS_REGION"`

			// Get an AWS session from an implicit source if no explicit
			// configuration is provided. This is useful for taking advantage of
			// EC2/ECS instance roles.
			UseRole bool `envconfig:"AWS_USE_ROLE"`
		}
		Auth struct {
			AwsSecretID   string        `default:"auth-secret-key" envconfig:"AUTH_AWS_SECRET_ID"`
			KeyExpiration time.Duration `default:"3600s" envconfig:"AUTH_KEY_EXPIRATION"`
		}
	}

	// !!! This prefix seems buried, if you copy and paste this main.go
	// 		file, its likely you will forget to update this.
	if err := envconfig.Process("WEB_API", &cfg); err != nil {
		log.Fatalf("main : Parsing Config : %v", err)
	}

	if err := flag.Process(&cfg); err != nil {
		if err != flag.ErrHelp {
			log.Fatalf("main : Parsing Command Line : %v", err)
		}
		return // We displayed help.
	}

	// =========================================================================
	// App Starting

	// Print the build version for our logs. Also expose it under /debug/vars.
	expvar.NewString("build").Set(build)
	log.Printf("main : Started : Application Initializing version %q", build)
	defer log.Println("main : Completed")

	cfgJSON, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		log.Fatalf("main : Marshalling Config to JSON : %v", err)
	}

	// TODO: Validate what is being written to the logs. We don't
	// want to leak credentials or anything that can be a security risk.
	log.Printf("main : Config : %v\n", string(cfgJSON))

	// =========================================================================
	// Init AWS Session
	var awsSession *session.Session
	if cfg.AwsAccount.UseRole {
		// Get an AWS session from an implicit source if no explicit
		// configuration is provided. This is useful for taking advantage of
		// EC2/ECS instance roles.
		awsSession = session.Must(session.NewSession())
	} else {
		creds := credentials.NewStaticCredentials(cfg.AwsAccount.AccessKeyID, cfg.AwsAccount.SecretAccessKey, "")
		awsSession = session.New(&aws.Config{Region: aws.String(cfg.AwsAccount.Region), Credentials: creds})
	}
	awsSession = awstrace.WrapSession(awsSession)

	// =========================================================================
	// Load auth keys from AWS and init new Authenticator
	authenticator, err := auth.NewAuthenticator(awsSession, cfg.Auth.AwsSecretID, time.Now().UTC(), cfg.Auth.KeyExpiration)
	if err != nil {
		log.Fatalf("main : Constructing authenticator : %v", err)
	}

	// =========================================================================
	// Start Mongo

	log.Println("main : Started : Initialize Mongo")
	masterDB, err := db.New(cfg.DB.Host, cfg.DB.DialTimeout)
	if err != nil {
		log.Fatalf("main : Register DB : %v", err)
	}
	defer masterDB.Close()

	// =========================================================================
	// Start Tracing Support

	logger := func(format string, v ...interface{}) {
		log.Printf(format, v...)
	}

	log.Printf("main : Tracing Started : %s", cfg.Trace.Host)
	exporter, err := itrace.NewExporter(logger, cfg.Trace.Host, cfg.Trace.BatchSize, cfg.Trace.SendInterval, cfg.Trace.SendTimeout)
	if err != nil {
		log.Fatalf("main : RegiTracingster : ERROR : %v", err)
	}
	defer func() {
		log.Printf("main : Tracing Stopping : %s", cfg.Trace.Host)
		batch, err := exporter.Close()
		if err != nil {
			log.Printf("main : Tracing Stopped : ERROR : Batch[%d] : %v", batch, err)
		} else {
			log.Printf("main : Tracing Stopped : Flushed Batch[%d]", batch)
		}
	}()

	trace.RegisterExporter(exporter)
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})

	// =========================================================================
	// Start Debug Service. Not concerned with shutting this down when the
	// application is being shutdown.
	//
	// /debug/vars - Added to the default mux by the expvars package.
	// /debug/pprof - Added to the default mux by the net/http/pprof package.
	if cfg.HTTP.DebugHost != "" {
		go func() {
			log.Printf("main : Debug Listening %s", cfg.HTTP.DebugHost)
			log.Printf("main : Debug Listener closed : %v", http.ListenAndServe(cfg.HTTP.DebugHost, http.DefaultServeMux))
		}()
	}

	// =========================================================================
	// Start API Service

	// Make a channel to listen for an interrupt or terminate signal from the OS.
	// Use a buffered channel because the signal package requires it.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	api := http.Server{
		Addr:           cfg.HTTP.Host,
		Handler:        handlers.API(shutdown, log, masterDB, authenticator),
		ReadTimeout:    cfg.HTTP.ReadTimeout,
		WriteTimeout:   cfg.HTTP.WriteTimeout,
		MaxHeaderBytes: 1 << 20,
	}

	// Make a channel to listen for errors coming from the listener. Use a
	// buffered channel so the goroutine can exit if we don't collect this error.
	serverErrors := make(chan error, 1)

	// Start the service listening for requests.
	go func() {
		log.Printf("main : API Listening %s", cfg.HTTP.Host)
		serverErrors <- api.ListenAndServe()
	}()

	// =========================================================================
	// Shutdown

	// Blocking main and waiting for shutdown.
	select {
	case err := <-serverErrors:
		log.Fatalf("main : Error starting server: %v", err)

	case sig := <-shutdown:
		log.Printf("main : %v : Start shutdown..", sig)

		// Create context for Shutdown call.
		ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()

		// Asking listener to shutdown and load shed.
		err := api.Shutdown(ctx)
		if err != nil {
			log.Printf("main : Graceful shutdown did not complete in %v : %v", cfg.HTTP.ShutdownTimeout, err)
			err = api.Close()
		}

		// Log the status of this shutdown.
		switch {
		case sig == syscall.SIGSTOP:
			log.Fatal("main : Integrity issue caused shutdown")
		case err != nil:
			log.Fatalf("main : Could not stop server gracefully : %v", err)
		}
	}
}
