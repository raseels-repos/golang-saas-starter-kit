package main

import (
	"expvar"
	"log"
	"os"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/tools/devops/cmd/cicd"
	_ "github.com/lib/pq"
	"github.com/urfave/cli"
)

// build is the git version of this program. It is set using build flags in the makefile.
var build = "develop"

// service is the name of the program used for logging, tracing and the
// the prefix used for loading env variables
// ie: export TRUSS_ENV=dev
var service = "DEVOPS"

func main() {
	// =========================================================================
	// Logging

	log := log.New(os.Stdout, service+" : ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	// =========================================================================
	// Log App Info

	// Print the build version for our logs. Also expose it under /debug/vars.
	expvar.NewString("build").Set(build)
	log.Printf("main : Started : Application Initializing version %q", build)
	defer log.Println("main : Completed")

	log.Printf("main : Args: %s", strings.Join(os.Args, " "))

	// =========================================================================
	// Start Truss

	var (
		buildFlags  cicd.ServiceBuildFlags
		deployFlags cicd.ServiceDeployFlags
	)

	app := cli.NewApp()
	app.Commands = []cli.Command{
		{
			Name:  "build",
			Usage: "-service=web-api -env=dev",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "service", Usage: "name of cmd", Destination: &buildFlags.ServiceName},
				cli.StringFlag{Name: "env", Usage: "dev, stage, or prod", Destination: &buildFlags.Env},
				cli.StringFlag{Name: "dockerfile", Usage: "DockerFile for service", Destination: &buildFlags.DockerFile},
				cli.StringFlag{Name: "root", Usage: "project root directory", Destination: &buildFlags.ProjectRoot},
				cli.StringFlag{Name: "project", Usage: "name of project", Destination: &buildFlags.ProjectName},
				cli.BoolFlag{Name: "no_cache", Usage: "skip docker cache", Destination: &buildFlags.NoCache},
				cli.BoolFlag{Name: "no_push", Usage: "skip docker push after build", Destination: &buildFlags.NoPush},
			},
			Action: func(c *cli.Context) error {
				req, err := cicd.NewServiceBuildRequest(log, buildFlags)
				if err != nil {
					return err
				}
				return cicd.ServiceBuild(log, req)
			},
		},
		{
			Name:  "deploy",
			Usage: "-service=web-api -env=dev",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "service", Usage: "name of cmd", Destination: &deployFlags.ServiceName},
				cli.StringFlag{Name: "env", Usage: "dev, stage, or prod", Destination: &deployFlags.Env},
				cli.BoolFlag{Name: "enable_https", Usage: "enable HTTPS", Destination: &deployFlags.EnableHTTPS},
				cli.StringFlag{Name: "primary_host", Usage: "dev, stage, or prod", Destination: &deployFlags.ServiceHostPrimary},
				cli.StringSliceFlag{Name: "host_names", Usage: "dev, stage, or prod", Value: &deployFlags.ServiceHostNames},
				cli.StringFlag{Name: "private_bucket", Usage: "dev, stage, or prod", Destination: &deployFlags.S3BucketPrivateName},
				cli.StringFlag{Name: "public_bucket", Usage: "dev, stage, or prod", Destination: &deployFlags.S3BucketPublicName},
				cli.StringFlag{Name: "dockerfile", Usage: "DockerFile for service", Destination: &deployFlags.DockerFile},
				cli.StringFlag{Name: "root", Usage: "project root directory", Destination: &deployFlags.ProjectRoot},
				cli.StringFlag{Name: "project", Usage: "name of project", Destination: &deployFlags.ProjectName},
				cli.BoolFlag{Name: "enable_elb", Usage: "enable deployed to use Elastic Load Balancer", Destination: &deployFlags.EnableEcsElb},
				cli.BoolTFlag{Name: "lambda_vpc", Usage: "deploy lambda behind VPC", Destination: &deployFlags.EnableLambdaVPC},
				cli.BoolFlag{Name: "recreate_service", Usage: "skip docker push after build", Destination: &deployFlags.RecreateService},
			},
			Action: func(c *cli.Context) error {
				if len(deployFlags.ServiceHostNames.Value()) == 1 {
					var hostNames []string
					for _, inpVal := range deployFlags.ServiceHostNames.Value() {
						pts := strings.Split(inpVal, ",")

						for _, h := range pts {
							h = strings.TrimSpace(h)
							if h != "" {
								hostNames = append(hostNames, h)
							}
						}
					}

					deployFlags.ServiceHostNames = hostNames
				}

				req, err := cicd.NewServiceDeployRequest(log, deployFlags)
				if err != nil {
					return err
				}
				return cicd.ServiceDeploy(log, req)
			},
		},
		{
			Name:  "migrate",
			Usage: "-env=dev",
			Flags: []cli.Flag{},
			Action: func(c *cli.Context) error {

				return nil
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("main : Truss : %+v", err)
	}

	log.Printf("main : Truss : Completed")
}
