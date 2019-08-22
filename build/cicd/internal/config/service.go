package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/iancoleman/strcase"
	"gitlab.com/geeks-accelerator/oss/devops/pkg/devdeploy"
	"gopkg.in/go-playground/validator.v9"
)

// Service define the name of a service.
type Service = string

var (
	ServiceWebApi = "web-api"
	ServiceWebApp = "web-app"
)

// List of service names used by main.go for help.
var ServiceNames = []Service{
	ServiceWebApi,
	ServiceWebApp,
}

// ServiceConfig defines the settings for a service.
type ServiceConfig struct {
	// Required flags.
	Name                 string `validate:"required" example:"web-api"`
	ServiceHostPrimary  string   `validate:"required" example:"example-project.com"`
	DesiredCount        int      `validate:"required" example:"2"`
	ServiceDir          string   `validate:"required"`
	Dockerfile          string   `validate:"required" example:"./cmd/web-api/Dockerfile"`
	ReleaseTag          string   `validate:"required"`

	// Optional flags.
	ServiceHostNames    []string `validate:"omitempty" example:"subdomain.example-project.com"`
	EnableHTTPS         bool     `validate:"omitempty" example:"false"`
	EnableElb           bool     `validate:"omitempty" example:"false"`
	StaticFilesS3Enable bool     `validate:"omitempty" example:"false"`
	BuildDir            string   `validate:"omitempty"`
	DockerBuildContext  string   `validate:"omitempty" example:"."`
}

// ServiceContext includes the config and task definition for building and deploying a service.
type ServiceContext struct {
	ServiceConfig

	// AwsEcsTaskDefinition defines the ECS task definition based on the service configs.
	AwsEcsTaskDefinition func(cfg *devdeploy.Config, srv *devdeploy.DeployService) (*ecs.RegisterTaskDefinitionInput, error)
}

// NewServiceConfig returns the Service for a service that is configured for the target deployment env.
func NewServiceConfig(serviceName string, cfg *devdeploy.Config) (ServiceConfig, error) {

	// =========================================================================
	// New service context.
	srv := ServiceConfig{
		Name:               serviceName,
		DesiredCount:       1,
		DockerBuildContext: ".",
		ServiceDir:         filepath.Join(cfg.ProjectRoot, "examples", serviceName),

		// Set the release tag for the image to use include env + service name + commit hash/tag.
		ReleaseTag: devdeploy.GitLabCiReleaseTag(cfg.Env, serviceName),
	}

	// =========================================================================
	// Context settings based on target env.
	if cfg.Env == EnvStage || cfg.Env == EnvProd {
		srv.EnableHTTPS = true
		srv.StaticFilesS3Enable = true
	} else {
		srv.EnableHTTPS = false
		srv.StaticFilesS3Enable = false
	}

	// =========================================================================
	// Service dependant settings.
	switch serviceName {
	case ServiceWebApp:

		// Set the hostnames for the service.
		if cfg.Env == EnvProd {
			srv.ServiceHostPrimary  = "example.saasstartupkit.com"

			// Any hostname listed here that doesn't match the primary hostname will be updated in Route 53 but the
			// service itself will redirect any requests back to the primary hostname.
			srv.ServiceHostNames = []string{
				fmt.Sprintf("%s.example.saasstartupkit.com", cfg.Env),
			}
		} else {
			srv.ServiceHostPrimary  = fmt.Sprintf("%s.example.saasstartupkit.com", cfg.Env)
		}

	case ServiceWebApi:

		// Set the hostnames for the service.
		if cfg.Env == EnvProd {
			srv.ServiceHostPrimary = "api.example.saasstartupkit.com"
		} else {
			srv.ServiceHostPrimary = fmt.Sprintf("api.%s.example.saasstartupkit.com", cfg.Env)
		}

	default:
		return ServiceConfig{}, errors.Wrapf(devdeploy.ErrInvalidService,
		"No service config defined for service '%s'",
		serviceName)
	}

	// Set the docker file if no custom one has been defined for the service.
	if srv.Dockerfile == "" {
		srv.Dockerfile = filepath.Join(srv.ServiceDir, "Dockerfile")
	}

	// Ensure the config is valid.
	errs := validator.New().Struct(cfg)
	if errs != nil {
		return srv, errs
	}

	return srv, nil
}

// BaseUrl returns the base url for a specific service.
func (c ServiceConfig) BaseUrl() string {
	var schema string
	if c.EnableHTTPS {
		schema = "https"
	} else {
		schema = "http"
	}
	return fmt.Sprintf("%s://%s/", schema, c.ServiceHostPrimary)
}


// NewServiceContext returns the ServiceContext for a service that is configured for the target deployment env.
func NewServiceContext(serviceName Service, cfg *devdeploy.Config) (*ServiceContext, error) {

	// =========================================================================
	// Shared details that could be applied to all task definitions.

	// Load the web-app config for the web-api can reference it's hostname.
	webAppCfg, err := NewServiceConfig(ServiceWebApp, cfg)
	if err != nil {
		return nil, err
	}

	// Load the web-api config for the web-app can reference it's hostname.
	webApiCfg, err := NewServiceConfig(ServiceWebApi, cfg)
	if err != nil {
		return nil, err
	}

	// Define a base set of environment variables that can be assigned to individual container definitions.
	baseEnvVals := func(cfg *devdeploy.Config, srv *devdeploy.DeployService) []*ecs.KeyValuePair {

		var ciJobURL string
		if id := os.Getenv("CI_JOB_ID"); id != "" {
			ciJobURL = strings.TrimRight(GitLabProjectBaseUrl, "/") + "/-/jobs/" + os.Getenv("CI_JOB_ID")
		}

		var ciPipelineURL string
		if id := os.Getenv("CI_PIPELINE_ID"); id != "" {
			ciPipelineURL = strings.TrimRight(GitLabProjectBaseUrl, "/") + "/pipelines/" + os.Getenv("CI_PIPELINE_ID")
		}

		return []*ecs.KeyValuePair{
			ecsKeyValuePair(devdeploy.ENV_KEY_ECS_CLUSTER, srv.AwsEcsCluster.ClusterName),
			ecsKeyValuePair(devdeploy.ENV_KEY_ECS_SERVICE, srv.AwsEcsService.ServiceName),
			ecsKeyValuePair("AWS_REGION", cfg.AwsCredentials.Region),
			ecsKeyValuePair("AWS_USE_ROLE", "true"),
			ecsKeyValuePair("AWSLOGS_GROUP", srv.AwsCloudWatchLogGroup.LogGroupName),
			ecsKeyValuePair("ECS_ENABLE_CONTAINER_METADATA", "true"),
			ecsKeyValuePair("CI_COMMIT_REF_NAME", os.Getenv("CI_COMMIT_REF_NAME")),
			ecsKeyValuePair("CI_COMMIT_SHORT_SHA", os.Getenv("CI_COMMIT_SHORT_SHA")),
			ecsKeyValuePair("CI_COMMIT_SHA", os.Getenv("CI_COMMIT_SHA")),
			ecsKeyValuePair("CI_COMMIT_TAG", os.Getenv("CI_COMMIT_TAG")),
			ecsKeyValuePair("CI_JOB_ID", os.Getenv("CI_JOB_ID")),
			ecsKeyValuePair("CI_PIPELINE_ID", os.Getenv("CI_PIPELINE_ID")),
			ecsKeyValuePair("CI_JOB_URL", ciJobURL),
			ecsKeyValuePair("CI_PIPELINE_URL", ciPipelineURL),
			ecsKeyValuePair("WEB_APP_BASE_URL", webAppCfg.BaseUrl()),
			ecsKeyValuePair("WEB_API_BASE_URL", webApiCfg.BaseUrl()),
			ecsKeyValuePair("EMAIL_SENDER", "lee+saas-starter-kit@geeksinthewoods.com"),
		}
	}


	// =========================================================================
	// Service dependant settings.

	var ctx *ServiceContext
	switch serviceName {

	// Define the ServiceContext for the web-app that will be used for build and deploy.
	case ServiceWebApp:

		ctx = &ServiceContext{
			ServiceConfig: webAppCfg,
		}

		// Define the service task definition with a function to enable use of config and deploy details.
		ctx.AwsEcsTaskDefinition = func(cfg *devdeploy.Config, srv *devdeploy.DeployService) (*ecs.RegisterTaskDefinitionInput, error) {

			// Defined a container definition for the specific service.
			container1 := &ecs.ContainerDefinition{
				Name:      aws.String(ctx.Name),
				Image:     aws.String(srv.ReleaseImage),
				Essential: aws.Bool(true),
				LogConfiguration: &ecs.LogConfiguration{
					LogDriver: aws.String("awslogs"),
					Options: map[string]*string{
						"awslogs-group":         aws.String(srv.AwsCloudWatchLogGroup.LogGroupName),
						"awslogs-region":        aws.String(cfg.AwsCredentials.Region),
						"awslogs-stream-prefix": aws.String("ecs"),
					},
				},
				PortMappings: []*ecs.PortMapping{
					&ecs.PortMapping{
						HostPort:      aws.Int64(80),
						Protocol:      aws.String("tcp"),
						ContainerPort: aws.Int64(80),
					},
				},
				Cpu:               aws.Int64(128),
				MemoryReservation: aws.Int64(128),
				Environment:       baseEnvVals(cfg, srv),
				HealthCheck: &ecs.HealthCheck{
					Retries: aws.Int64(3),
					Command: aws.StringSlice([]string{
						"CMD-SHELL",
						"curl -f http://localhost/ping || exit 1",
					}),
					Timeout:     aws.Int64(5),
					Interval:    aws.Int64(60),
					StartPeriod: aws.Int64(60),
				},
				Ulimits: []*ecs.Ulimit{
					&ecs.Ulimit{
						Name:      aws.String("nofile"),
						SoftLimit: aws.Int64(987654),
						HardLimit: aws.Int64(999999),
					},
				},
			}

			// If the service has HTTPS enabled with the use of an AWS Elastic Load Balancer, then need to enable
			// traffic for port 443 for SSL traffic to get terminated on the deployed tasks.
			if ctx.EnableHTTPS && !ctx.EnableElb {
				container1.PortMappings = append(container1.PortMappings, &ecs.PortMapping{
					HostPort:      aws.Int64(443),
					Protocol:      aws.String("tcp"),
					ContainerPort: aws.Int64(443),
				})
			}

			// Append env vars for the service task.
			container1.Environment = append(container1.Environment,
				ecsKeyValuePair("SERVICE_NAME", srv.ServiceName),
				ecsKeyValuePair("PROJECT_NAME", cfg.ProjectName),

				// Use placeholders for these environment variables that will be replaced with devdeploy.DeployServiceToTargetEnv
				ecsKeyValuePair("WEB_API_SERVICE_HOST", "{HTTP_HOST}"),
				ecsKeyValuePair("WEB_API_SERVICE_HTTPS_HOST", "{HTTPS_HOST}"),
				ecsKeyValuePair("WEB_API_SERVICE_ENABLE_HTTPS", "{HTTPS_ENABLED}"),
				ecsKeyValuePair("WEB_API_SERVICE_BASE_URL", "{APP_BASE_URL}"),
				ecsKeyValuePair("WEB_API_SERVICE_HOST_NAMES", "{HOST_NAMES}"),
				ecsKeyValuePair("WEB_API_SERVICE_STATICFILES_S3_ENABLED", "{STATIC_FILES_S3_ENABLED}"),
				ecsKeyValuePair("WEB_API_SERVICE_STATICFILES_S3_PREFIX", "{STATIC_FILES_S3_PREFIX}"),
				ecsKeyValuePair("WEB_API_SERVICE_STATICFILES_CLOUDFRONT_ENABLED", "{STATIC_FILES_CLOUDFRONT_ENABLED}"),
				ecsKeyValuePair("WEB_API_REDIS_HOST", "{CACHE_HOST}"),
				ecsKeyValuePair("WEB_API_DB_HOST", "{DB_HOST}"),
				ecsKeyValuePair("WEB_API_DB_USERNAME", "{DB_USER}"),
				ecsKeyValuePair("WEB_API_DB_PASSWORD", "{DB_PASS}"),
				ecsKeyValuePair("WEB_API_DB_DATABASE", "{DB_DATABASE}"),
				ecsKeyValuePair("WEB_API_DB_DRIVER", "{DB_DRIVER}"),
				ecsKeyValuePair("WEB_API_DB_DISABLE_TLS", "{DB_DISABLE_TLS}"),
				ecsKeyValuePair("WEB_API_AWS_S3_BUCKET_PRIVATE", "{AWS_S3_BUCKET_PRIVATE}"),
				ecsKeyValuePair("WEB_API_AWS_S3_BUCKET_PUBLIC", "{AWS_S3_BUCKET_PUBLIC}"),
				ecsKeyValuePair(devdeploy.ENV_KEY_ROUTE53_UPDATE_TASK_IPS, "{ROUTE53_UPDATE_TASK_IPS}"),
				ecsKeyValuePair(devdeploy.ENV_KEY_ROUTE53_ZONES, "{ROUTE53_ZONES}"),
			)

			// Define the full task definition for the service.
			def := &ecs.RegisterTaskDefinitionInput{
				Family:           aws.String(srv.ServiceName),
				ExecutionRoleArn: aws.String(srv.AwsEcsExecutionRole.Arn()),
				TaskRoleArn:      aws.String(srv.AwsEcsTaskRole.Arn()),
				NetworkMode:      aws.String("awsvpc"),
				ContainerDefinitions: []*ecs.ContainerDefinition{
					// Include the single container definition for the service. Additional definitions could be added
					// here like one for datadog.
					container1,
				},
				RequiresCompatibilities: aws.StringSlice([]string{"FARGATE"}),
			}

			return def, nil
		}


	// Define the ServiceContext for the web-api that will be used for build and deploy.
	case ServiceWebApi:

		ctx = &ServiceContext{
			ServiceConfig: webApiCfg,
		}

		// Define the service task definition with a function to enable use of config and deploy details.
		ctx.AwsEcsTaskDefinition = func(cfg *devdeploy.Config, srv *devdeploy.DeployService) (*ecs.RegisterTaskDefinitionInput, error) {

			// Defined a container definition for the specific service.
			container1 := &ecs.ContainerDefinition{
				Name:      aws.String(ctx.Name),
				Image:     aws.String(srv.ReleaseImage),
				Essential: aws.Bool(true),
				LogConfiguration: &ecs.LogConfiguration{
					LogDriver: aws.String("awslogs"),
					Options: map[string]*string{
						"awslogs-group":         aws.String(srv.AwsCloudWatchLogGroup.LogGroupName),
						"awslogs-region":        aws.String(cfg.AwsCredentials.Region),
						"awslogs-stream-prefix": aws.String("ecs"),
					},
				},
				PortMappings: []*ecs.PortMapping{
					&ecs.PortMapping{
						HostPort:      aws.Int64(80),
						Protocol:      aws.String("tcp"),
						ContainerPort: aws.Int64(80),
					},
				},
				Cpu:               aws.Int64(128),
				MemoryReservation: aws.Int64(128),
				Environment:       baseEnvVals(cfg, srv),
				HealthCheck: &ecs.HealthCheck{
					Retries: aws.Int64(3),
					Command: aws.StringSlice([]string{
						"CMD-SHELL",
						"curl -f http://localhost/ping || exit 1",
					}),
					Timeout:     aws.Int64(5),
					Interval:    aws.Int64(60),
					StartPeriod: aws.Int64(60),
				},
				Ulimits: []*ecs.Ulimit{
					&ecs.Ulimit{
						Name:      aws.String("nofile"),
						SoftLimit: aws.Int64(987654),
						HardLimit: aws.Int64(999999),
					},
				},
			}

			// If the service has HTTPS enabled with the use of an AWS Elastic Load Balancer, then need to enable
			// traffic for port 443 for SSL traffic to get terminated on the deployed tasks.
			if ctx.EnableHTTPS && !ctx.EnableElb {
				container1.PortMappings = append(container1.PortMappings, &ecs.PortMapping{
					HostPort:      aws.Int64(443),
					Protocol:      aws.String("tcp"),
					ContainerPort: aws.Int64(443),
				})
			}

			// Append env vars for the service task.
			container1.Environment = append(container1.Environment,
				ecsKeyValuePair("SERVICE_NAME", srv.ServiceName),
				ecsKeyValuePair("PROJECT_NAME", cfg.ProjectName),

				// Use placeholders for these environment variables that will be replaced with devdeploy.DeployServiceToTargetEnv
				ecsKeyValuePair("WEB_API_SERVICE_HOST", "{HTTP_HOST}"),
				ecsKeyValuePair("WEB_API_SERVICE_HTTPS_HOST", "{HTTPS_HOST}"),
				ecsKeyValuePair("WEB_API_SERVICE_ENABLE_HTTPS", "{HTTPS_ENABLED}"),
				ecsKeyValuePair("WEB_API_SERVICE_BASE_URL", "{APP_BASE_URL}"),
				ecsKeyValuePair("WEB_API_SERVICE_HOST_NAMES", "{HOST_NAMES}"),
				ecsKeyValuePair("WEB_API_SERVICE_STATICFILES_S3_ENABLED", "{STATIC_FILES_S3_ENABLED}"),
				ecsKeyValuePair("WEB_API_SERVICE_STATICFILES_S3_PREFIX", "{STATIC_FILES_S3_PREFIX}"),
				ecsKeyValuePair("WEB_API_SERVICE_STATICFILES_CLOUDFRONT_ENABLED", "{STATIC_FILES_CLOUDFRONT_ENABLED}"),
				ecsKeyValuePair("WEB_API_REDIS_HOST", "{CACHE_HOST}"),
				ecsKeyValuePair("WEB_API_DB_HOST", "{DB_HOST}"),
				ecsKeyValuePair("WEB_API_DB_USERNAME", "{DB_USER}"),
				ecsKeyValuePair("WEB_API_DB_PASSWORD", "{DB_PASS}"),
				ecsKeyValuePair("WEB_API_DB_DATABASE", "{DB_DATABASE}"),
				ecsKeyValuePair("WEB_API_DB_DRIVER", "{DB_DRIVER}"),
				ecsKeyValuePair("WEB_API_DB_DISABLE_TLS", "{DB_DISABLE_TLS}"),
				ecsKeyValuePair("WEB_API_AWS_S3_BUCKET_PRIVATE", "{AWS_S3_BUCKET_PRIVATE}"),
				ecsKeyValuePair("WEB_API_AWS_S3_BUCKET_PUBLIC", "{AWS_S3_BUCKET_PUBLIC}"),
				ecsKeyValuePair(devdeploy.ENV_KEY_ROUTE53_UPDATE_TASK_IPS, "{ROUTE53_UPDATE_TASK_IPS}"),
				ecsKeyValuePair(devdeploy.ENV_KEY_ROUTE53_ZONES, "{ROUTE53_ZONES}"),
			)

			// Define the full task definition for the service.
			def := &ecs.RegisterTaskDefinitionInput{
				Family:           aws.String(srv.ServiceName),
				ExecutionRoleArn: aws.String(srv.AwsEcsExecutionRole.Arn()),
				TaskRoleArn:      aws.String(srv.AwsEcsTaskRole.Arn()),
				NetworkMode:      aws.String("awsvpc"),
				ContainerDefinitions: []*ecs.ContainerDefinition{
					// Include the single container definition for the service. Additional definitions could be added
					// here like one for datadog.
					container1,
				},
				RequiresCompatibilities: aws.StringSlice([]string{"FARGATE"}),
			}

			return def, nil
		}

	default:
		return nil, errors.Wrapf(devdeploy.ErrInvalidService,
			"No service context defined for service '%s'",
			serviceName)
	}

	return ctx, nil
}

// BuildService handles defining all the information needed to a service with docker and push to AWS ECR.
func (ctx *ServiceContext) Build(log *log.Logger, noCache, noPush bool) (*devdeploy.BuildService, error) {

	log.Printf("Define build for service '%s'.", ctx.Name)
	log.Printf("\tUsing release tag %s.", ctx.ReleaseTag)

	srv := &devdeploy.BuildService{
		ServiceName:        ctx.Name,
		ReleaseTag:         ctx.ReleaseTag,
		BuildDir:           ctx.BuildDir,
		Dockerfile:         ctx.Dockerfile,
		DockerBuildContext: ctx.DockerBuildContext,
		NoCache:            noCache,
		NoPush:             noPush,
	}

	return srv, nil
}

// DeployService handles defining all the information needed to deploy a service to AWS ECS.
func (ctx *ServiceContext) Deploy(log *log.Logger, cfg *devdeploy.Config) (*devdeploy.DeployService, error) {

	log.Printf("Define deploy for service '%s'.", ctx.Name)
	log.Printf("\tUsing release tag %s.", ctx.ReleaseTag)

	// Start to define all the information for the service from the service context.
	srv := &devdeploy.DeployService{
		ServiceName:        ctx.Name,
		ReleaseTag:         ctx.ReleaseTag,
		EnableHTTPS:        ctx.EnableHTTPS,
		ServiceHostPrimary: ctx.ServiceHostPrimary,
		ServiceHostNames:   ctx.ServiceHostNames,
	}

	// When only service host names are set, choose the first item as the primary host.
	if srv.ServiceHostPrimary == "" && len(srv.ServiceHostNames) > 0 {
		srv.ServiceHostPrimary = srv.ServiceHostNames[0]
		log.Printf("\t\tSet Service Primary Host to '%s'.", srv.ServiceHostPrimary)
	}

	// The S3 prefix used to upload static files served to public.
	if ctx.StaticFilesS3Enable {
		srv.StaticFilesS3Prefix = filepath.Join(cfg.AwsS3BucketPublicKeyPrefix, srv.ReleaseTag, "static")
	}

	// Determine the Dockerfile for the service.
	if ctx.Dockerfile != "" {
		srv.Dockerfile = ctx.Dockerfile
		log.Printf("\t\tUsing docker file '%s'.", srv.Dockerfile)
	} else {
		var err error
		srv.Dockerfile, err = devdeploy.FindServiceDockerFile(cfg.ProjectRoot, srv.ServiceName)
		if err != nil {
			return nil, err
		}
		log.Printf("\t\tFound service docker file '%s'.", srv.Dockerfile)
	}

	// Set the service directory.
	if ctx.ServiceDir == "" {
		ctx.ServiceDir = filepath.Dir(srv.Dockerfile)
	}
	srv.StaticFilesDir = filepath.Join(ctx.ServiceDir, "static")

	// Define the ECS Cluster used to host the serverless fargate tasks.
	srv.AwsEcsCluster = &devdeploy.AwsEcsCluster{
		ClusterName: cfg.ProjectName + "-" + cfg.Env,
		Tags: []devdeploy.Tag{
			{Key: devdeploy.AwsTagNameProject, Value: cfg.ProjectName},
			{Key: devdeploy.AwsTagNameEnv, Value: cfg.Env},
		},
	}

	// Define the ECS task execution role. This role executes ECS actions such as pulling the image and storing the
	// application logs in cloudwatch.
	srv.AwsEcsExecutionRole = &devdeploy.AwsIamRole{
		RoleName:                 fmt.Sprintf("ecsExecutionRole%s%s", cfg.ProjectNameCamel(), strcase.ToCamel(cfg.Env)),
		Description:              fmt.Sprintf("Provides access to other AWS service resources that are required to run Amazon ECS tasks for %s. ", cfg.ProjectName),
		AssumeRolePolicyDocument: "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":[\"ecs-tasks.amazonaws.com\"]},\"Action\":[\"sts:AssumeRole\"]}]}",
		Tags: []devdeploy.Tag{
			{Key: devdeploy.AwsTagNameProject, Value: cfg.ProjectName},
			{Key: devdeploy.AwsTagNameEnv, Value: cfg.Env},
		},
		AttachRolePolicyArns: []string{"arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"},
	}
	log.Printf("\t\tSet ECS Execution Role Name to '%s'.", srv.AwsEcsExecutionRole.RoleName)

	// Define the ECS task role. This role is used by the task itself for calling other AWS services.
	srv.AwsEcsTaskRole = &devdeploy.AwsIamRole{
		RoleName:                 fmt.Sprintf("ecsTaskRole%s%s", cfg.ProjectNameCamel(), strcase.ToCamel(cfg.Env)),
		Description:              fmt.Sprintf("Allows ECS tasks for %s to call AWS services on your behalf.", cfg.ProjectName),
		AssumeRolePolicyDocument: "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":[\"ecs-tasks.amazonaws.com\"]},\"Action\":[\"sts:AssumeRole\"]}]}",
		Tags: []devdeploy.Tag{
			{Key: devdeploy.AwsTagNameProject, Value: cfg.ProjectName},
			{Key: devdeploy.AwsTagNameEnv, Value: cfg.Env},
		},
	}
	log.Printf("\t\tSet ECS Task Role Name to '%s'.", srv.AwsEcsTaskRole.RoleName)

	// AwsCloudWatchLogGroup defines the name of the cloudwatch log group that will be used to store logs for the ECS tasks.
	srv.AwsCloudWatchLogGroup = &devdeploy.AwsCloudWatchLogGroup{
		LogGroupName: fmt.Sprintf("logs/env_%s/aws/ecs/cluster_%s/service_%s", cfg.Env, srv.AwsEcsCluster.ClusterName, srv.ServiceName),
		Tags: []devdeploy.Tag{
			{Key: devdeploy.AwsTagNameProject, Value: cfg.ProjectName},
			{Key: devdeploy.AwsTagNameEnv, Value: cfg.Env},
		},
	}
	log.Printf("\t\tSet AWS Log Group Name to '%s'.", srv.AwsCloudWatchLogGroup.LogGroupName)

	// AwsSdPrivateDnsNamespace defines the service discovery group.
	srv.AwsSdPrivateDnsNamespace = &devdeploy.AwsSdPrivateDnsNamespace{
		Name:        srv.AwsEcsCluster.ClusterName,
		Description: fmt.Sprintf("Private DNS namespace used for services running on the ECS Cluster %s", srv.AwsEcsCluster.ClusterName),
		Service: &devdeploy.AwsSdService{
			Name:                        ctx.Name,
			Description:                 fmt.Sprintf("Service %s running on the ECS Cluster %s", ctx.Name, srv.AwsEcsCluster.ClusterName),
			DnsRecordTTL:                300,
			HealthCheckFailureThreshold: 3,
		},
	}
	log.Printf("\t\tSet AWS Service Discovery Namespace to '%s'.", srv.AwsSdPrivateDnsNamespace.Name)

	// If the service is requested to use an elastic load balancer then define.
	if ctx.EnableElb {
		// AwsElbLoadBalancer defines if the service should use an elastic load balancer.
		srv.AwsElbLoadBalancer = &devdeploy.AwsElbLoadBalancer{
			Name:          fmt.Sprintf("%s-%s-%s", cfg.Env, srv.AwsEcsCluster.ClusterName, srv.ServiceName),
			IpAddressType: "ipv4",
			Scheme:        "internet-facing",
			Type:          "application",
			Tags: []devdeploy.Tag{
				{Key: devdeploy.AwsTagNameProject, Value: cfg.ProjectName},
				{Key: devdeploy.AwsTagNameEnv, Value: cfg.Env},
			},
		}
		log.Printf("\t\tSet ELB Name to '%s'.", srv.AwsElbLoadBalancer.Name)

		// Define the target group for service to receive HTTP traffic from the load balancer.
		srv.AwsElbLoadBalancer.TargetGroup = &devdeploy.AwsElbTargetGroup{
			Name:                       fmt.Sprintf("%s-http", srv.ServiceName),
			Port:                       80,
			Protocol:                   "HTTP",
			TargetType:                 "ip",
			HealthCheckEnabled:         true,
			HealthCheckIntervalSeconds: 30,
			HealthCheckPath:            "/ping",
			HealthCheckProtocol:        "HTTP",
			HealthCheckTimeoutSeconds:  5,
			HealthyThresholdCount:      3,
			UnhealthyThresholdCount:    3,
			Matcher:                    "200",
		}
		log.Printf("\t\t\tSet ELB Target Group Name for %s to '%s'.",
			srv.AwsElbLoadBalancer.TargetGroup.Protocol,
			srv.AwsElbLoadBalancer.TargetGroup.Name)

		// Set ECS configs based on specified env.
		if cfg.Env == "prod" {
			srv.AwsElbLoadBalancer.EcsTaskDeregistrationDelay = 300
		} else {
			// Force staging to deploy immediately without waiting for connections to drain
			srv.AwsElbLoadBalancer.EcsTaskDeregistrationDelay = 0
		}
	}

	// AwsEcsService defines the details for the ecs service.
	srv.AwsEcsService = &devdeploy.AwsEcsService{
		ServiceName:                   ctx.Name,
		DesiredCount:                  int64(ctx.DesiredCount),
		EnableECSManagedTags:          false,
		HealthCheckGracePeriodSeconds: 60,
		LaunchType:                    "FARGATE",
	}

	// Ensure when deploying a new service there is always at-least one running.
	if srv.AwsEcsService.DesiredCount == 0 {
		srv.AwsEcsService.DesiredCount = 1
	}

	// Set ECS configs based on specified env.
	if cfg.Env == "prod" {
		srv.AwsEcsService.DeploymentMinimumHealthyPercent = 100
		srv.AwsEcsService.DeploymentMaximumPercent = 200
	} else {
		srv.AwsEcsService.DeploymentMinimumHealthyPercent = 100
		srv.AwsEcsService.DeploymentMaximumPercent = 200
	}

	// AwsEcsTaskDefinition defines the details for registering a new ECS task definition.
	taskDef, err := ctx.AwsEcsTaskDefinition(cfg, srv)
	if err != nil {
		return nil, err
	}

	srv.AwsEcsTaskDefinition = &devdeploy.AwsEcsTaskDefinition{
		RegisterInput: taskDef,
		UpdatePlaceholders: func(placeholders map[string]string) error {

			// Try to find the Datadog API key, this value is optional.
			// If Datadog API key is not specified, then integration with Datadog for observability will not be active.
			{
				datadogApiKey, err := getDatadogApiKey(cfg)
				if err != nil {
					return err
				}

				if datadogApiKey != "" {
					log.Println("DATADOG API Key set.")
				} else {
					log.Printf("DATADOG API Key NOT set.")
				}

				placeholders["{DATADOG_APIKEY}"] = datadogApiKey

				// When the datadog API key is empty, don't force the container to be essential have have the whole task fail.
				if datadogApiKey != "" {
					placeholders["{DATADOG_ESSENTIAL}"] = "true"
				} else {
					placeholders["{DATADOG_ESSENTIAL}"] = "false"
				}
			}

			return nil
		},
	}

	log.Printf("\t\tDeploying task to '%s'.", ctx.ServiceHostPrimary)

	return srv, nil
}

// BuildServiceForTargetEnv executes the build commands for a target service.
func BuildServiceForTargetEnv(log *log.Logger, awsCredentials devdeploy.AwsCredentials, targetEnv Env, serviceName, releaseTag string, dryRun, noCache, noPush bool) error {

	cfgCtx, err := NewConfigContext(targetEnv, awsCredentials)
	if err != nil {
		return err
	}

	cfg, err := cfgCtx.Config(log)
	if err != nil {
		return err
	}

	srvCtx, err := NewServiceContext(serviceName, cfg)
	if err != nil {
		return err
	}

	// Override the release tag if set.
	if releaseTag != "" {
		srvCtx.ReleaseTag = releaseTag
	}

	details, err := srvCtx.Build(log, noCache, noPush)
	if err != nil {
		return err
	}

	// servicePath is used to copy the service specific code in the Dockerfile.
	servicePath, err := filepath.Rel(cfg.ProjectRoot, srvCtx.ServiceDir)
	if err != nil {
		return err
	}

	// commitRef is used by main.go:build constant.
	commitRef := getCommitRef()
	if commitRef == "" {
		commitRef = srvCtx.ReleaseTag
	}

	details.BuildArgs = map[string]string{
		"service_path": servicePath,
		"commit_ref":   commitRef,
	}

	if dryRun {
		cfgJSON, err := json.MarshalIndent(cfg, "", "    ")
		if err != nil {
			log.Fatalf("BuildServiceForTargetEnv : Marshalling config to JSON : %+v", err)
		}
		log.Printf("BuildServiceForTargetEnv : config : %v\n", string(cfgJSON))

		detailsJSON, err := json.MarshalIndent(details, "", "    ")
		if err != nil {
			log.Fatalf("BuildServiceForTargetEnv : Marshalling details to JSON : %+v", err)
		}
		log.Printf("BuildServiceForTargetEnv : details : %v\n", string(detailsJSON))

		return nil
	}

	return devdeploy.BuildServiceForTargetEnv(log, cfg, details)
}

// DeployServiceForTargetEnv executes the build commands for a target service.
func DeployServiceForTargetEnv(log *log.Logger, awsCredentials devdeploy.AwsCredentials, targetEnv Env, serviceName, releaseTag string, dryRun bool) error {

	cfgCtx, err := NewConfigContext(targetEnv, awsCredentials)
	if err != nil {
		return err
	}

	cfg, err := cfgCtx.Config(log)
	if err != nil {
		return err
	}

	srvCtx, err := NewServiceContext(serviceName, cfg)
	if err != nil {
		return err
	}

	// Override the release tag if set.
	if releaseTag != "" {
		srvCtx.ReleaseTag = releaseTag
	}

	details, err := srvCtx.Deploy(log, cfg)
	if err != nil {
		return err
	}

	return devdeploy.DeployServiceToTargetEnv(log, cfg, details)
}

// ecsKeyValuePair returns an *ecs.KeyValuePair
func ecsKeyValuePair(name, value string) *ecs.KeyValuePair {
	return &ecs.KeyValuePair{
		Name:  aws.String(name),
		Value: aws.String(value),
	}
}
