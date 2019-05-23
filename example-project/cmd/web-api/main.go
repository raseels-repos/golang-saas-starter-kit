package main

import (
	"context"
	"encoding/json"
	"expvar"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/cmd/web-api/handlers"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/flag"
	itrace "geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/trace"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-redis/redis"
	"github.com/kelseyhightower/envconfig"
	"github.com/lib/pq"
	"go.opencensus.io/trace"
	awstrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/aws/aws-sdk-go/aws"
	sqltrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/database/sql"
	redistrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
	sqlxtrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/jmoiron/sqlx"
)

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
			Host         string        `default:"0.0.0.0:3000" envconfig:"HTTP_HOST"`
			ReadTimeout  time.Duration `default:"10s" envconfig:"HTTP_READ_TIMEOUT"`
			WriteTimeout time.Duration `default:"10s" envconfig:"HTTP_WRITE_TIMEOUT"`
		}
		HTTPS struct {
			Host         string        `default:"" envconfig:"HTTPS_HOST"`
			ReadTimeout  time.Duration `default:"5s" envconfig:"HTTPS_READ_TIMEOUT"`
			WriteTimeout time.Duration `default:"5s" envconfig:"HTTPS_WRITE_TIMEOUT"`
		}
		App struct {
			Name            string        `default:"web-api" envconfig:"APP_NAME"`
			BaseUrl         string        `default:"" envconfig:"APP_BASE_URL"`
			TemplateDir     string        `default:"./templates" envconfig:"APP_TEMPLATE_DIR"`
			DebugHost       string        `default:"0.0.0.0:4000" envconfig:"APP_DEBUG_HOST"`
			ShutdownTimeout time.Duration `default:"5s" envconfig:"APP_SHUTDOWN_TIMEOUT"`
		}
		Redis struct {
			DialTimeout     time.Duration `default:"5s" envconfig:"REDIS_DIAL_TIMEOUT"`
			Host            string        `default:":6379" envconfig:"REDIS_HOST"`
			DB              int           `default:"1" envconfig:"REDIS_DB"`
			MaxmemoryPolicy string        `envconfig:"REDIS_MAXMEMORY_POLICY"`
		}
		DB struct {
			Host       string `default:"127.0.0.1:5433" envconfig:"DB_HOST"`
			User       string `default:"postgres" envconfig:"DB_USER"`
			Pass       string `default:"postgres" envconfig:"DB_PASS" json:"-"` // don't print
			Database   string `default:"shared" envconfig:"DB_DATABASE"`
			Driver     string `default:"postgres" envconfig:"DB_DRIVER"`
			Timezone   string `default:"utc" envconfig:"DB_TIMEZONE"`
			DisableTLS bool   `default:"false" envconfig:"DB_DISABLE_TLS"`
		}
		Trace struct {
			Host         string        `default:"http://tracer:3002/v1/publish" envconfig:"TRACE_HOST"`
			BatchSize    int           `default:"1000" envconfig:"TRACE_BATCH_SIZE"`
			SendInterval time.Duration `default:"15s" envconfig:"TRACE_SEND_INTERVAL"`
			SendTimeout  time.Duration `default:"500ms" envconfig:"TRACE_SEND_TIMEOUT"`
		}
		AwsAccount struct {
			AccessKeyID     string `envconfig:"AWS_ACCESS_KEY_ID"`
			SecretAccessKey string `envconfig:"AWS_SECRET_ACCESS_KEY" json:"-"` // don't print
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
	}

	// The prefix used for loading env variables.
	// ie: export WEB_API_ENV=dev
	envKeyPrefix := "WEB_API"

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
	// Config Validation & Defaults

	// If base URL is empty, set the default value from the HTTP Host
	if cfg.App.BaseUrl == "" {
		baseUrl := cfg.HTTP.Host
		if !strings.HasPrefix(baseUrl, "http") {
			if strings.HasPrefix(baseUrl, "0.0.0.0:") {
				pts := strings.Split(baseUrl, ":")
				pts[0] = "127.0.0.1"
				baseUrl = strings.Join(pts, ":")
			} else if strings.HasPrefix(baseUrl, ":") {
				baseUrl = "127.0.0.1" + baseUrl
			}
			baseUrl = "http://" + baseUrl
		}
		cfg.App.BaseUrl = baseUrl
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
	// Start Redis
	// Ensure the eviction policy on the redis cluster is set correctly.
	// 		AWS Elastic cache redis clusters by default have the volatile-lru.
	// 			volatile-lru: evict keys by trying to remove the less recently used (LRU) keys first, but only among keys that have an expire set, in order to make space for the new data added.
	// 			allkeys-lru: evict keys by trying to remove the less recently used (LRU) keys first, in order to make space for the new data added.
	//		Recommended to have eviction policy set to allkeys-lru
	log.Println("main : Started : Initialize Redis")
	redisClient := redistrace.NewClient(&redis.Options{
		Addr:        cfg.Redis.Host,
		DB:          cfg.Redis.DB,
		DialTimeout: cfg.Redis.DialTimeout,
	})
	defer redisClient.Close()

	evictPolicyConfigKey := "maxmemory-policy"

	// if the maxmemory policy is set for redis, make sure its set on the cluster
	// default not set and will based on the redis config values defined on the server
	if cfg.Redis.MaxmemoryPolicy != "" {
		err := redisClient.ConfigSet(evictPolicyConfigKey, cfg.Redis.MaxmemoryPolicy).Err()
		if err != nil {
			log.Fatalf("main : redis : ConfigSet maxmemory-policy : %v", err)
		}
	} else {
		evictPolicy, err := redisClient.ConfigGet(evictPolicyConfigKey).Result()
		if err != nil {
			log.Fatalf("main : redis : ConfigGet maxmemory-policy : %v", err)
		}

		if evictPolicy[1] != "allkeys-lru" {
			log.Printf("main : redis : ConfigGet maxmemory-policy : recommended to be set to allkeys-lru to avoid OOM")
		}
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
	// Load auth keys from AWS and init new Authenticator
	authenticator, err := auth.NewAuthenticator(awsSession, cfg.Auth.AwsSecretID, time.Now().UTC(), cfg.Auth.KeyExpiration)
	if err != nil {
		log.Fatalf("main : Constructing authenticator : %v", err)
	}

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
	if cfg.App.DebugHost != "" {
		go func() {
			log.Printf("main : Debug Listening %s", cfg.App.DebugHost)
			log.Printf("main : Debug Listener closed : %v", http.ListenAndServe(cfg.App.DebugHost, http.DefaultServeMux))
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
		Handler:        handlers.API(shutdown, log, masterDb, authenticator),
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
		ctx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
		defer cancel()

		// Asking listener to shutdown and load shed.
		err := api.Shutdown(ctx)
		if err != nil {
			log.Printf("main : Graceful shutdown did not complete in %v : %v", cfg.App.ShutdownTimeout, err)
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
