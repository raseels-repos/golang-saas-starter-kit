package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"expvar"
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/cmd/web-api/docs"
	"geeks-accelerator/oss/saas-starter-kit/cmd/web-api/handlers"
	"geeks-accelerator/oss/saas-starter-kit/internal/mid"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/devops"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/flag"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-redis/redis"
	"github.com/kelseyhightower/envconfig"
	"github.com/lib/pq"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	awstrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/aws/aws-sdk-go/aws"
	sqltrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/database/sql"
	redistrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
	sqlxtrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/jmoiron/sqlx"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// build is the git version of this program. It is set using build flags in the makefile.
var build = "develop"

// service is the name of the program used for logging, tracing and the
// the prefix used for loading env variables
// ie: export WEB_API_ENV=dev
var service = "WEB_API"

// @title SaaS Example API

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @securityDefinitions.basic BasicAuth

// @securitydefinitions.oauth2.password OAuth2Password
// @tokenUrl /v1/oauth/token
// @scope.user Grants basic privileges with role of user.
// @scope.admin Grants administrative privileges with role of admin.

func main() {

	// =========================================================================
	// Logging

	log := log.New(os.Stdout, service+" : ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	// =========================================================================
	// Configuration
	var cfg struct {
		Env  string `default:"dev" envconfig:"ENV"`
		HTTP struct {
			Host         string        `default:"0.0.0.0:3001" envconfig:"HOST"`
			ReadTimeout  time.Duration `default:"10s" envconfig:"READ_TIMEOUT"`
			WriteTimeout time.Duration `default:"10s" envconfig:"WRITE_TIMEOUT"`
		}
		HTTPS struct {
			Host         string        `default:"" envconfig:"HOST"`
			ReadTimeout  time.Duration `default:"5s" envconfig:"READ_TIMEOUT"`
			WriteTimeout time.Duration `default:"5s" envconfig:"WRITE_TIMEOUT"`
			DisableHTTP2 bool          `default:"false" envconfig:"DISABLE_HTTP2"`
		}
		Service struct {
			Name            string        `default:"web-api" envconfig:"NAME"`
			Project         string        `default:"" envconfig:"PROJECT"`
			BaseUrl         string        `default:"" envconfig:"BASE_URL"  example:"http://api.example.saasstartupkit.com"`
			HostNames       []string      `envconfig:"HOST_NAMES" example:"alternative-subdomain.example.saasstartupkit.com"`
			EnableHTTPS     bool          `default:"false" envconfig:"ENABLE_HTTPS"`
			TemplateDir     string        `default:"./templates" envconfig:"TEMPLATE_DIR"`
			WebAppBaseUrl   string        `default:"http://127.0.0.1:3000" envconfig:"WEB_APP_BASE_URL" example:"www.example.saasstartupkit.com"`
			DebugHost       string        `default:"0.0.0.0:4000" envconfig:"DEBUG_HOST"`
			ShutdownTimeout time.Duration `default:"5s" envconfig:"SHUTDOWN_TIMEOUT"`
		}
		Redis struct {
			Host            string        `default:":6379" envconfig:"HOST"`
			DB              int           `default:"1" envconfig:"DB"`
			DialTimeout     time.Duration `default:"5s" envconfig:"DIAL_TIMEOUT"`
			MaxmemoryPolicy string        `envconfig:"MAXMEMORY_POLICY"`
		}
		DB struct {
			Host       string `default:"127.0.0.1:5433" envconfig:"HOST"`
			User       string `default:"postgres" envconfig:"USER"`
			Pass       string `default:"postgres" envconfig:"PASS" json:"-"` // don't print
			Database   string `default:"shared" envconfig:"DATABASE"`
			Driver     string `default:"postgres" envconfig:"DRIVER"`
			Timezone   string `default:"utc" envconfig:"TIMEZONE"`
			DisableTLS bool   `default:"true" envconfig:"DISABLE_TLS"`
		}
		Trace struct {
			Host          string  `default:"127.0.0.1" envconfig:"DD_TRACE_AGENT_HOSTNAME"`
			Port          int     `default:"8126" envconfig:"DD_TRACE_AGENT_PORT"`
			AnalyticsRate float64 `default:"0.10" envconfig:"ANALYTICS_RATE"`
		}
		Aws struct {
			AccessKeyID                string `envconfig:"AWS_ACCESS_KEY_ID"`              // WEB_API_AWS_AWS_ACCESS_KEY_ID or AWS_ACCESS_KEY_ID
			SecretAccessKey            string `envconfig:"AWS_SECRET_ACCESS_KEY" json:"-"` // don't print
			Region                     string `default:"us-west-2" envconfig:"AWS_REGION"`
			S3BucketPrivate            string `envconfig:"S3_BUCKET_PRIVATE"`
			S3BucketPublic             string `envconfig:"S3_BUCKET_PUBLIC"`
			SecretsManagerConfigPrefix string `default:"" envconfig:"SECRETS_MANAGER_CONFIG_PREFIX"`

			// Get an AWS session from an implicit source if no explicit
			// configuration is provided. This is useful for taking advantage of
			// EC2/ECS instance roles.
			UseRole bool `envconfig:"AWS_USE_ROLE"`
		}
		Auth struct {
			UseAwsSecretManager bool          `default:"false" envconfig:"USE_AWS_SECRET_MANAGER"`
			KeyExpiration       time.Duration `default:"3600s" envconfig:"KEY_EXPIRATION"`
		}
		BuildInfo struct {
			CiCommitRefName  string `envconfig:"CI_COMMIT_REF_NAME"`
			CiCommitShortSha string `envconfig:"CI_COMMIT_SHORT_SHA"`
			CiCommitSha      string `envconfig:"CI_COMMIT_SHA"`
			CiCommitTag      string `envconfig:"CI_COMMIT_TAG"`
			CiJobId          string `envconfig:"CI_JOB_ID"`
			CiJobUrl         string `envconfig:"CI_JOB_URL"`
			CiPipelineId     string `envconfig:"CI_PIPELINE_ID"`
			CiPipelineUrl    string `envconfig:"CI_PIPELINE_URL"`
		}
	}

	// For additional details refer to https://github.com/kelseyhightower/envconfig
	if err := envconfig.Process(service, &cfg); err != nil {
		log.Fatalf("main : Parsing Config : %+v", err)
	}

	if err := flag.Process(&cfg); err != nil {
		if err != flag.ErrHelp {
			log.Fatalf("main : Parsing Command Line : %+v", err)
		}
		return // We displayed help.
	}

	// =========================================================================
	// Config Validation & Defaults

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
				log.Fatalf("main : Load region of ecs metadata : %+v", err)
			}
		}
	}

	// Set the default AWS Secrets Manager prefix used for name to store config files that will be persisted across
	// deployments and distributed to each instance of the service running.
	if cfg.Aws.SecretsManagerConfigPrefix == "" {
		var pts []string
		if cfg.Service.Project != "" {
			pts = append(pts, cfg.Service.Project)
		}
		pts = append(pts, cfg.Env, cfg.Service.Name)

		cfg.Aws.SecretsManagerConfigPrefix = filepath.Join(pts...)
	}

	// If base URL is empty, set the default value from the HTTP Host
	if cfg.Service.BaseUrl == "" {
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
		cfg.Service.BaseUrl = baseUrl
	}

	// When HTTPS is not specifically enabled, but an HTTP host is set, enable HTTPS.
	if !cfg.Service.EnableHTTPS && cfg.HTTPS.Host != "" {
		cfg.Service.EnableHTTPS = true
	}

	// Determine the primary host by parsing host from the base app URL.
	baseSiteUrl, err := url.Parse(cfg.Service.BaseUrl)
	if err != nil {
		log.Fatalf("main : Parse service base URL : %s : %+v", cfg.Service.BaseUrl, err)
	}

	// Drop any ports from the base app URL.
	var primaryServiceHost string
	if strings.Contains(baseSiteUrl.Host, ":") {
		primaryServiceHost, _, err = net.SplitHostPort(baseSiteUrl.Host)
		if err != nil {
			log.Fatalf("main : SplitHostPort : %s : %+v", baseSiteUrl.Host, err)
		}
	} else {
		primaryServiceHost = baseSiteUrl.Host
	}

	// =========================================================================
	// Log Service Info

	// Print the build version for our logs. Also expose it under /debug/vars.
	expvar.NewString("build").Set(build)
	log.Printf("main : Started : Service Initializing version %q", build)
	defer log.Println("main : Completed")

	// Print the config for our logs. It's important to any credentials in the config
	// that could expose a security risk are excluded from being json encoded by
	// applying the tag `json:"-"` to the struct var.
	{
		cfgJSON, err := json.MarshalIndent(cfg, "", "    ")
		if err != nil {
			log.Fatalf("main : Marshalling Config to JSON : %+v", err)
		}
		log.Printf("main : Config : %v\n", string(cfgJSON))
	}

	// =========================================================================
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

		log.Printf("main : AWS : Using role.\n")

	} else if cfg.Aws.AccessKeyID != "" {
		creds := credentials.NewStaticCredentials(cfg.Aws.AccessKeyID, cfg.Aws.SecretAccessKey, "")
		awsSession = session.New(&aws.Config{Region: aws.String(cfg.Aws.Region), Credentials: creds})

		log.Printf("main : AWS : Using static credentials\n")
	}

	// Wrap the AWS session to enable tracing.
	if awsSession != nil {
		awsSession = awstrace.WrapSession(awsSession)
	}

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
		if err != nil && !strings.Contains(err.Error(), "unknown command") {
			log.Fatalf("main : redis : ConfigSet maxmemory-policy : %+v", err)
		}
	} else {
		evictPolicy, err := redisClient.ConfigGet(evictPolicyConfigKey).Result()
		if err != nil && !strings.Contains(err.Error(), "unknown command") {
			log.Fatalf("main : redis : ConfigGet maxmemory-policy : %+v", err)
		} else if evictPolicy != nil && len(evictPolicy) > 0 && evictPolicy[1] != "allkeys-lru" {
			log.Printf("main : redis : ConfigGet maxmemory-policy : recommended to be set to allkeys-lru to avoid OOM")
		}
	}

	// =========================================================================
	// Start Database
	var dbUrl url.URL
	{
		// Query parameters.
		var q url.Values = make(map[string][]string)

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
	log.Println("main : Started : Initialize Database")

	// Register informs the sqlxtrace package of the driver that we will be using in our program.
	// It uses a default service name, in the below case "postgres.db". To use a custom service
	// name use RegisterWithServiceName.
	sqltrace.Register(cfg.DB.Driver, &pq.Driver{}, sqltrace.WithServiceName(service))
	masterDb, err := sqlxtrace.Open(cfg.DB.Driver, dbUrl.String())
	if err != nil {
		log.Fatalf("main : Register DB : %s : %+v", cfg.DB.Driver, err)
	}
	defer masterDb.Close()

	// =========================================================================
	// Init new Authenticator
	var authenticator *auth.Authenticator
	if cfg.Auth.UseAwsSecretManager {
		secretName := filepath.Join(cfg.Aws.SecretsManagerConfigPrefix, "authenticator")
		authenticator, err = auth.NewAuthenticatorAws(awsSession, secretName, time.Now().UTC(), cfg.Auth.KeyExpiration)
	} else {
		authenticator, err = auth.NewAuthenticatorFile("", time.Now().UTC(), cfg.Auth.KeyExpiration)
	}
	if err != nil {
		log.Fatalf("main : Constructing authenticator : %+v", err)
	}

	// =========================================================================
	// Load middlewares that need to be configured specific for the service.
	var serviceMiddlewares = []web.Middleware{
		mid.Translator(webcontext.UniversalTranslator()),
	}

	// Init redirect middleware to ensure all requests go to the primary domain contained in the base URL.
	if primaryServiceHost != "127.0.0.1" && primaryServiceHost != "localhost" {
		redirect := mid.DomainNameRedirect(mid.DomainNameRedirectConfig{
			RedirectConfig: mid.RedirectConfig{
				Code: http.StatusMovedPermanently,
				Skipper: func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) bool {
					if r.URL.Path == "/ping" {
						return true
					}
					return false
				},
			},
			DomainName:   primaryServiceHost,
			HTTPSEnabled: cfg.Service.EnableHTTPS,
		})
		serviceMiddlewares = append(serviceMiddlewares, redirect)
	}

	// =========================================================================
	// Start Tracing Support
	th := fmt.Sprintf("%s:%d", cfg.Trace.Host, cfg.Trace.Port)
	log.Printf("main : Tracing Started : %s", th)
	sr := tracer.NewRateSampler(cfg.Trace.AnalyticsRate)
	tracer.Start(tracer.WithAgentAddr(th), tracer.WithSampler(sr))
	defer tracer.Stop()

	// =========================================================================
	// Start Debug Service. Not concerned with shutting this down when the
	// application is being shutdown.
	//
	// /debug/vars - Added to the default mux by the expvars package.
	// /debug/pprof - Added to the default mux by the net/http/pprof package.
	if cfg.Service.DebugHost != "" {
		go func() {
			log.Printf("main : Debug Listening %s", cfg.Service.DebugHost)
			log.Printf("main : Debug Listener closed : %v", http.ListenAndServe(cfg.Service.DebugHost, http.DefaultServeMux))
		}()
	}

	// =========================================================================
	// ECS Task registration for services that don't use an AWS Elastic Load Balancer.
	err = devops.EcsServiceTaskInit(log, awsSession)
	if err != nil {
		log.Fatalf("main : Ecs Service Task init : %+v", err)
	}

	// =========================================================================
	// Start API Service

	// Programmatically set swagger info.
	{
		docs.SwaggerInfo.Version = build

		u, err := url.Parse(cfg.Service.BaseUrl)
		if err != nil {
			log.Fatalf("main : Parse app base url %s : %+v", cfg.Service.BaseUrl, err)
		}

		docs.SwaggerInfo.Host = u.Host
		docs.SwaggerInfo.BasePath = "/v1"
	}

	// Make a channel to listen for an interrupt or terminate signal from the OS.
	// Use a buffered channel because the signal package requires it.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Make a channel to listen for errors coming from the listener. Use a
	// buffered channel so the goroutine can exit if we don't collect this error.
	serverErrors := make(chan error, 1)

	// Make an list of HTTP servers for both HTTP and HTTPS requests.
	var httpServers []http.Server

	// Start the HTTP service listening for requests.
	if cfg.HTTP.Host != "" {
		api := http.Server{
			Addr:           cfg.HTTP.Host,
			Handler:        handlers.API(shutdown, log, cfg.Env, masterDb, redisClient, authenticator, serviceMiddlewares...),
			ReadTimeout:    cfg.HTTP.ReadTimeout,
			WriteTimeout:   cfg.HTTP.WriteTimeout,
			MaxHeaderBytes: 1 << 20,
		}
		httpServers = append(httpServers, api)

		go func() {
			log.Printf("main : API Listening %s", cfg.HTTP.Host)
			serverErrors <- api.ListenAndServe()
		}()
	}

	// Start the HTTPS service listening for requests with an SSL Cert auto generated with Let's Encrypt.
	if cfg.HTTPS.Host != "" {
		api := http.Server{
			Addr:           cfg.HTTPS.Host,
			Handler:        handlers.API(shutdown, log, cfg.Env, masterDb, redisClient, authenticator, serviceMiddlewares...),
			ReadTimeout:    cfg.HTTPS.ReadTimeout,
			WriteTimeout:   cfg.HTTPS.WriteTimeout,
			MaxHeaderBytes: 1 << 20,
		}

		// Generate a unique list of hostnames.
		var hosts []string
		if primaryServiceHost != "" {
			hosts = append(hosts, primaryServiceHost)
		}
		for _, h := range cfg.Service.HostNames {
			h = strings.TrimSpace(h)
			if h != "" && h != primaryServiceHost {
				hosts = append(hosts, h)
			}
		}

		// Enable autocert to store certs via Secret Manager.
		secretPrefix := filepath.Join(cfg.Aws.SecretsManagerConfigPrefix, "autocert")

		// Local file cache to reduce requests hitting Secret Manager.
		localCache := autocert.DirCache(os.TempDir())

		cache, err := devops.NewSecretManagerAutocertCache(log, awsSession, secretPrefix, localCache)
		if err != nil {
			log.Fatalf("main : HTTPS : %+v", err)
		}

		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(hosts...),
			Cache:      cache,
		}
		api.TLSConfig = &tls.Config{GetCertificate: m.GetCertificate}
		api.TLSConfig.NextProtos = append(api.TLSConfig.NextProtos, acme.ALPNProto)
		if !cfg.HTTPS.DisableHTTP2 {
			api.TLSConfig.NextProtos = append(api.TLSConfig.NextProtos, "h2")
		}

		httpServers = append(httpServers, api)

		go func() {
			log.Printf("main : API Listening %s with SSL cert for hosts %s", cfg.HTTPS.Host, strings.Join(hosts, ", "))
			serverErrors <- api.ListenAndServeTLS("", "")
		}()
	}

	// =========================================================================
	// Shutdown

	// Blocking main and waiting for shutdown.
	select {
	case err := <-serverErrors:
		log.Fatalf("main : Error starting server: %+v", err)

	case sig := <-shutdown:
		log.Printf("main : %v : Start shutdown..", sig)

		// Ensure the public IP address for the task is removed from Route53.
		err = devops.EcsServiceTaskTaskShutdown(log, awsSession)
		if err != nil {
			log.Fatalf("main : Ecs Service Task shutdown : %+v", err)
		}

		// Create context for Shutdown call.
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Service.ShutdownTimeout)
		defer cancel()

		// Handle closing connections for both possible HTTP servers.
		for _, api := range httpServers {

			// Asking listener to shutdown and load shed.
			err := api.Shutdown(ctx)
			if err != nil {
				log.Printf("main : Graceful shutdown did not complete in %v : %v", cfg.Service.ShutdownTimeout, err)
				err = api.Close()
			}
		}

		// Log the status of this shutdown.
		switch {
		case sig == syscall.SIGSTOP:
			log.Fatal("main : Integrity issue caused shutdown")
		case err != nil:
			log.Fatalf("main : Could not stop server gracefully : %+v", err)
		}
	}
}
