package main

import (
	"encoding/json"
	"expvar"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/example-project/tools/truss/cmd/dbtable2crud"
	"github.com/kelseyhightower/envconfig"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	sqltrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/database/sql"
	sqlxtrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/jmoiron/sqlx"
)

// build is the git version of this program. It is set using build flags in the makefile.
var build = "develop"

// service is the name of the program used for logging, tracing and the
// the prefix used for loading env variables
// ie: export TRUSS_ENV=dev
var service = "TRUSS"

func main() {
	// =========================================================================
	// Logging

	log := log.New(os.Stdout, service+" : ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	// =========================================================================
	// Configuration
	var cfg struct {
		DB struct {
			Host       string `default:"127.0.0.1:5433" envconfig:"HOST"`
			User       string `default:"postgres" envconfig:"USER"`
			Pass       string `default:"postgres" envconfig:"PASS" json:"-"` // don't print
			Database   string `default:"shared" envconfig:"DATABASE"`
			Driver     string `default:"postgres" envconfig:"DRIVER"`
			Timezone   string `default:"utc" envconfig:"TIMEZONE"`
			DisableTLS bool   `default:"false" envconfig:"DISABLE_TLS"`
		}
	}

	// For additional details refer to https://github.com/kelseyhightower/envconfig
	if err := envconfig.Process(service, &cfg); err != nil {
		log.Fatalf("main : Parsing Config : %v", err)
	}

	// TODO: can't use flag.Process here since it doesn't support nested arg options
	//if err := flag.Process(&cfg); err != nil {
	///	if err != flag.ErrHelp {
	//		log.Fatalf("main : Parsing Command Line : %v", err)
	//	}
	//	return // We displayed help.
	//}

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

	// Register informs the sqlxtrace package of the driver that we will be using in our program.
	// It uses a default service name, in the below case "postgres.db". To use a custom service
	// name use RegisterWithServiceName.
	sqltrace.Register(cfg.DB.Driver, &pq.Driver{}, sqltrace.WithServiceName(service))
	masterDb, err := sqlxtrace.Open(cfg.DB.Driver, dbUrl.String())
	if err != nil {
		log.Fatalf("main : Register DB : %s : %v", cfg.DB.Driver, err)
	}
	defer masterDb.Close()

	// =========================================================================
	// Start Truss

	app := cli.NewApp()
	app.Commands = []cli.Command{
		{
			Name:    "dbtable2crud",
			Aliases: []string{},
			Usage:   "-table=projects -file=../../internal/project/models.go -model=Project [-dbtable=TABLE] [-templateDir=DIR] [-projectPath=DIR] [-saveChanges=false] ",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "dbtable, table"},
				cli.StringFlag{Name: "modelFile, modelfile, file"},
				cli.StringFlag{Name: "modelName, modelname, model"},
				cli.StringFlag{Name: "templateDir, templates", Value: "./templates/dbtable2crud"},
				cli.StringFlag{Name: "projectPath"},
				cli.BoolFlag{Name: "saveChanges, save"},
			},
			Action: func(c *cli.Context) error {
				dbTable := strings.TrimSpace(c.String("dbtable"))
				modelFile := strings.TrimSpace(c.String("modelFile"))
				modelName := strings.TrimSpace(c.String("modelName"))
				templateDir := strings.TrimSpace(c.String("templateDir"))
				projectPath := strings.TrimSpace(c.String("projectPath"))

				pwd, err := os.Getwd()
				if err != nil {
					return errors.WithMessage(err, "Failed to get current working directory")
				}

				if !path.IsAbs(templateDir) {
					templateDir = filepath.Join(pwd, templateDir)
				}
				ok, err := exists(templateDir)
				if err != nil {
					return errors.WithMessage(err, "Failed to load template directory")
				} else if !ok {
					return errors.Errorf("Template directory %s does not exist", templateDir)
				}

				if modelFile == "" {
					return errors.Errorf("Model file path is required")
				}

				if !path.IsAbs(modelFile) {
					modelFile = filepath.Join(pwd, modelFile)
				}
				ok, err = exists(modelFile)
				if err != nil {
					return errors.WithMessage(err, "Failed to load model file")
				} else if !ok {
					return errors.Errorf("Model file %s does not exist", modelFile)
				}

				// Load the project path from go.mod if not set.
				if projectPath == "" {
					goModFile := filepath.Join(pwd, "../../go.mod")
					ok, err = exists(goModFile)
					if err != nil {
						return errors.WithMessage(err, "Failed to load go.mod for project")
					} else if !ok {
						return errors.Errorf("Failed to locate project go.mod at %s", goModFile)
					}

					b, err := ioutil.ReadFile(goModFile)
					if err != nil {
						return errors.WithMessagef(err, "Failed to read go.mod at %s", goModFile)
					}

					lines := strings.Split(string(b), "\n")
					for _, l := range lines {
						if strings.HasPrefix(l, "module ") {
							projectPath = strings.TrimSpace(strings.Split(l, " ")[1])
							break
						}
					}
				}

				if modelName == "" {
					modelName = strings.Split(filepath.Base(modelFile), ".")[0]
					modelName = strings.Replace(modelName, "_", " ", -1)
					modelName = strings.Replace(modelName, "-", " ", -1)
					modelName = strings.Title(modelName)
					modelName = strings.Replace(modelName, " ", "", -1)
				}

				return dbtable2crud.Run(masterDb, log, cfg.DB.Database, dbTable, modelFile, modelName, templateDir, projectPath, c.Bool("saveChanges"))
			},
		},
	}

	err = app.Run(os.Args)
	if err != nil {
		log.Fatalf("main : Truss : %+v", err)
	}

	log.Printf("main : Truss : Completed")
}

// exists returns a bool as to whether a file path exists.
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}
