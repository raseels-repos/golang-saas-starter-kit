package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/urfave/cli"
	"geeks-accelerator/oss/saas-starter-kit/build/cicd/internal/config"
	"gitlab.com/geeks-accelerator/oss/devops/pkg/devdeploy"
)

// service is the name of the program used for logging, tracing, etc.
var service = "CICD"

func main() {

	// =========================================================================
	// Logging
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	log.SetPrefix(service + " : ")
	log := log.New(os.Stdout, log.Prefix(), log.Flags())

	// =========================================================================
	// New CLI application.
	app := cli.NewApp()
	app.Name = "cicd"
	app.Usage = "Provides build and deploy for GitLab to Amazon AWS"
	app.Version = "1.0"
	app.Author = "Lee Brown"
	app.Email = "lee@geeksinthewoods.com"

	// Define global CLI flags.
	var awsCredentials devdeploy.AwsCredentials
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "env",
			Usage: fmt.Sprintf("target environment, one of [%s]",
				strings.Join(config.EnvNames, ", ")),
			Required: true,
		},
		cli.StringFlag{
			Name:        "aws-access-key",
			Usage:       "AWS Access Key",
			EnvVar:      "AWS_ACCESS_KEY_ID",
			Destination: &awsCredentials.AccessKeyID,
		},
		cli.StringFlag{
			Name:        "aws-secret-key",
			Usage:       "AWS Secret Key",
			EnvVar:      "AWS_SECRET_ACCESS_KEY",
			Destination: &awsCredentials.SecretAccessKey,
		},
		cli.StringFlag{
			Name:        "aws-region",
			Usage:       "AWS Region",
			EnvVar:      "AWS_REGION",
			Destination: &awsCredentials.Region,
		},
		cli.BoolFlag{
			Name:        "aws-use-role",
			Usage:       "Use an IAM Role else AWS Access/Secret Keys are required",
			EnvVar:      "AWS_USE_ROLE",
			Destination: &awsCredentials.UseRole,
		},
	}

	app.Commands = []cli.Command{
		// Build command for services and functions.
		{
			Name:    "build",
			Aliases: []string{"b"},
			Usage:   "build a service or function",
			Subcommands: []cli.Command{
				{
					Name:  "service",
					Usage: "build a service",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name: "name, n",
							Usage: fmt.Sprintf("target service, one of [%s]",
								strings.Join(config.ServiceNames, ", ")),
							Required: true,
						},
						cli.StringFlag{
							Name:  "release-tag, tag",
							Usage: "optional tag to override default CI_COMMIT_SHORT_SHA",
						},
						cli.BoolFlag{
							Name:  "dry-run",
							Usage: "print out the build details",
						},
						cli.BoolFlag{
							Name:  "no-cache",
							Usage: "skip caching for the docker build",
						},
						cli.BoolFlag{
							Name:  "no-push",
							Usage: "disable pushing release image to remote repository",
						},
					},
					Action: func(c *cli.Context) error {
						targetEnv := c.GlobalString("env")
						serviceName := c.String("name")
						releaseTag := c.String("release-tag")
						dryRun := c.Bool("dry-run")
						noCache := c.Bool("no-cache")
						noPush := c.Bool("no-push")

						return config.BuildServiceForTargetEnv(log, awsCredentials, targetEnv, serviceName, releaseTag, dryRun, noCache, noPush)
					},
				},
				{
					Name:  "function",
					Usage: "build a function",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name: "name, n",
							Usage: fmt.Sprintf("target function, one of [%s]",
								strings.Join(config.FunctionNames, ", ")),
							Required: true,
						},
						cli.StringFlag{
							Name:  "release-tag, tag",
							Usage: "optional tag to override default CI_COMMIT_SHORT_SHA",
						},
						cli.BoolFlag{
							Name:  "dry-run",
							Usage: "print out the build details",
						},
						cli.BoolFlag{
							Name:  "no-cache",
							Usage: "skip caching for the docker build",
						},
						cli.BoolFlag{
							Name:  "no-push",
							Usage: "disable pushing release image to remote repository",
						},
					},
					Action: func(c *cli.Context) error {
						targetEnv := c.GlobalString("env")
						funcName := c.String("name")
						releaseTag := c.String("release-tag")
						dryRun := c.Bool("dry-run")
						noCache := c.Bool("no-cache")
						noPush := c.Bool("no-push")

						return config.BuildFunctionForTargetEnv(log, awsCredentials, targetEnv, funcName, releaseTag, dryRun, noCache, noPush)
					},
				},
			},
		},

		// deploy command for services and functions.
		{
			Name:    "deploy",
			Aliases: []string{"d"},
			Usage:   "deploy a service or function",
			Subcommands: []cli.Command{
				{
					Name:  "service",
					Usage: "deploy a service",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name: "name, n",
							Usage: fmt.Sprintf("target service, one of [%s]",
								strings.Join(config.ServiceNames, ", ")),
							Required: true,
						},
						cli.StringFlag{
							Name:  "release-tag, tag",
							Usage: "optional tag to override default CI_COMMIT_SHORT_SHA",
						},
						cli.BoolFlag{
							Name:  "dry-run",
							Usage: "print out the deploy details",
						},
					},
					Action: func(c *cli.Context) error {
						targetEnv := c.GlobalString("env")
						serviceName := c.String("name")
						releaseTag := c.String("release-tag")
						dryRun := c.Bool("dry-run")

						return config.DeployServiceForTargetEnv(log, awsCredentials, targetEnv, serviceName, releaseTag, dryRun)
					},
				},
				{
					Name:  "function",
					Usage: "deploy a function",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name: "name, n",
							Usage: fmt.Sprintf("target function, one of [%s]",
								strings.Join(config.FunctionNames, ", ")),
							Required: true,
						},
						cli.StringFlag{
							Name:  "release-tag, tag",
							Usage: "optional tag to override default CI_COMMIT_SHORT_SHA",
						},
						cli.BoolFlag{
							Name:  "dry-run",
							Usage: "print out the deploy details",
						},
					},
					Action: func(c *cli.Context) error {
						targetEnv := c.GlobalString("env")
						funcName := c.String("name")
						releaseTag := c.String("release-tag")
						dryRun := c.Bool("dry-run")

						return config.DeployFunctionForTargetEnv(log, awsCredentials, targetEnv, funcName, releaseTag, dryRun)
					},
				},
			},
		},

		// schema command used to run database schema migrations.
		{
			Name:    "schema",
			Aliases: []string{"s"},
			Usage:   "manage the database schema",
			Subcommands: []cli.Command{
				{
					Name:  "migrate",
					Usage: "run the schema migrations",
					Flags: []cli.Flag{
						cli.BoolFlag{
							Name:  "unittest",
							Usage: "print out the build details",
						},
					},
					Action: func(c *cli.Context) error {
						targetEnv := c.GlobalString("env")
						isUnittest := c.Bool("unittest")

						return config.RunSchemaMigrationsForTargetEnv(log, awsCredentials, targetEnv, isUnittest)
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("%+v", err)
	}
}
