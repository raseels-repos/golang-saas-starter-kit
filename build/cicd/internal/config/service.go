package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/applicationautoscaling"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
	"gitlab.com/geeks-accelerator/oss/devops/pkg/devdeploy"
	"gopkg.in/go-playground/validator.v9"
)

const (
	// EnableServiceElb will enable all services to be deployed with an ELB (Elastic Load Balancer).
	// This will only be applied to the prod env, but the logic can be changed in the code below.
	//
	// When enabled each service will require it's own ELB and therefore will add $20~ month per service when
	// this is enabled. The hostnames defined for the service will be updated in Route53 to resolve to the ELB.
	// If HTTPS is enabled, the ELB will be created with an AWS ACM certificate that will support SSL termination on
	// the ELB, all traffic will be sent to the container as HTTP.
	// This can be configured on a by service basis.
	//
	// When not enabled, tasks will be auto assigned a public IP. As ECS tasks for the service are launched/terminated,
	// the task will update the hostnames defined for the service in Route53 to either add/remove its public IP. This
	// option is good for services that only need one container running.
	EnableServiceElb = false

	// EnableServiceAutoscaling will enable all services to be deployed with an application scaling policy. This should
	// typically be enabled for front end services that have an ELB enabled.
	EnableServiceAutoscaling = false
)

// Service define the name of a service.
type Service = string

var (
	ServiceWebApi Service = "web-api"
	ServiceWebApp Service = "web-app"
)

// List of service names used by main.go for help.
var ServiceNames = []Service{
	ServiceWebApi,
	ServiceWebApp,
}

// ServiceContext defines the settings for a service.
type ServiceContext struct {
	// Required flags.
	Name               string `validate:"required" example:"web-api"`
	ServiceHostPrimary string `validate:"required" example:"example-project.com"`
	DesiredCount       int64  `validate:"required" example:"2"`
	ServiceDir         string `validate:"required"`
	Dockerfile         string `validate:"required" example:"./cmd/web-api/Dockerfile"`
	ReleaseTag         string `validate:"required"`

	// Optional flags.
	ServiceHostNames    []string `validate:"omitempty" example:"subdomain.example-project.com"`
	EnableHTTPS         bool     `validate:"omitempty" example:"false"`
	StaticFilesS3Enable bool     `validate:"omitempty" example:"false"`
	DockerBuildDir      string   `validate:"omitempty"`
	DockerBuildContext  string   `validate:"omitempty" example:"."`
}

// NewServiceContext returns the Service for a service that is configured for the target deployment env.
func NewServiceContext(serviceName string, cfg *devdeploy.Config) (ServiceContext, error) {

	// =========================================================================
	// New service context.
	srv := ServiceContext{
		Name:               serviceName,
		DesiredCount:       1,
		DockerBuildContext: ".",
		ServiceDir:         filepath.Join(cfg.ProjectRoot, "cmd", serviceName),

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
			srv.ServiceHostPrimary = "example.saasstartupkit.com"

			// Any hostname listed here that doesn't match the primary hostname will be updated in Route 53 but the
			// service itself will redirect any requests back to the primary hostname.
			srv.ServiceHostNames = []string{
				fmt.Sprintf("%s.example.saasstartupkit.com", cfg.Env),
			}
		} else {
			srv.ServiceHostPrimary = fmt.Sprintf("%s.example.saasstartupkit.com", cfg.Env)
		}

	case ServiceWebApi:

		// Set the hostnames for the service.
		if cfg.Env == EnvProd {
			srv.ServiceHostPrimary = "api.example.saasstartupkit.com"
		} else {
			srv.ServiceHostPrimary = fmt.Sprintf("api.%s.example.saasstartupkit.com", cfg.Env)
		}

	default:
		return ServiceContext{}, errors.Wrapf(devdeploy.ErrInvalidService,
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
func (c ServiceContext) BaseUrl() string {
	var schema string
	if c.EnableHTTPS {
		schema = "https"
	} else {
		schema = "http"
	}
	return fmt.Sprintf("%s://%s/", schema, c.ServiceHostPrimary)
}

// NewService returns the ProjectService for a service that is configured for the target deployment env.
func NewService(serviceName string, cfg *devdeploy.Config) (*devdeploy.ProjectService, error) {

	ctx, err := NewServiceContext(serviceName, cfg)
	if err != nil {
		return nil, err
	}

	// =========================================================================
	// New project service.
	srv := &devdeploy.ProjectService{
		Name:               serviceName,
		CodeDir:            filepath.Join(cfg.ProjectRoot, "cmd", serviceName),
		DockerBuildDir:     ctx.DockerBuildDir,
		DockerBuildContext: ".",
		EnableHTTPS:        ctx.EnableHTTPS,

		ServiceHostPrimary: ctx.ServiceHostPrimary,
		ServiceHostNames:   ctx.ServiceHostNames,
		ReleaseTag:         ctx.ReleaseTag,

		DockerBuildArgs: make(map[string]string),
	}

	if srv.DockerBuildDir == "" {
		srv.DockerBuildDir = cfg.ProjectRoot
	}

	// Sync static files to S3 will be enabled when the S3 prefix is defined.
	if ctx.StaticFilesS3Enable {
		srv.StaticFilesS3Prefix = filepath.Join(cfg.AwsS3BucketPublicKeyPrefix, ctx.ReleaseTag, "static")
	}

	// =========================================================================
	// Service settings based on target env.
	var enableElb bool
	if cfg.Env == EnvStage || cfg.Env == EnvProd {
		if cfg.Env == EnvProd && EnableServiceElb {
			enableElb = true
		}
	}

	// =========================================================================
	// Shared details that could be applied to all task definitions.

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
		LogGroupName: fmt.Sprintf("logs/env_%s/aws/ecs/cluster_%s/service_%s", cfg.Env, srv.AwsEcsCluster.ClusterName, srv.Name),
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
	if enableElb {
		// AwsElbLoadBalancer defines if the service should use an elastic load balancer.
		srv.AwsElbLoadBalancer = &devdeploy.AwsElbLoadBalancer{
			Name:          fmt.Sprintf("%s-%s", cfg.Env, ctx.Name),
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
		srv.AwsElbLoadBalancer.TargetGroups = []*devdeploy.AwsElbTargetGroup{
			&devdeploy.AwsElbTargetGroup{
				Name:                       fmt.Sprintf("%s-http", ctx.Name),
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
			},
		}
		log.Printf("\t\t\tSet ELB Target Group Name for %s to '%s'.",
			srv.AwsElbLoadBalancer.TargetGroups[0].Protocol,
			srv.AwsElbLoadBalancer.TargetGroups[0].Name)

		// Set ECS configs based on specified env.
		if cfg.Env == EnvProd {
			srv.AwsElbLoadBalancer.EcsTaskDeregistrationDelay = 300
		} else {
			// Force staging to deploy immediately without waiting for connections to drain
			srv.AwsElbLoadBalancer.EcsTaskDeregistrationDelay = 0
		}
	}

	// AwsEcsService defines the details for the ecs service.
	srv.AwsEcsService = &devdeploy.AwsEcsService{
		ServiceName:                   ctx.Name,
		DesiredCount:                  ctx.DesiredCount,
		EnableECSManagedTags:          false,
		HealthCheckGracePeriodSeconds: 60,
		LaunchType:                    "FARGATE",
	}

	// Set ECS configs based on specified env.
	if cfg.Env == EnvProd {
		srv.AwsEcsService.DeploymentMinimumHealthyPercent = 100
		srv.AwsEcsService.DeploymentMaximumPercent = 200
	} else {
		srv.AwsEcsService.DeploymentMinimumHealthyPercent = 0
		srv.AwsEcsService.DeploymentMaximumPercent = 100
	}

	if EnableServiceAutoscaling {
		srv.AwsAppAutoscalingPolicy = &devdeploy.AwsAppAutoscalingPolicy{
			// The name of the scaling policy.
			PolicyName: srv.AwsEcsService.ServiceName,

			// The policy type. This parameter is required if you are creating a scaling
			// policy.
			//
			// The following policy types are supported:
			//
			// TargetTrackingScaling—Not supported for Amazon EMR or AppStream
			//
			// StepScaling—Not supported for Amazon DynamoDB
			//
			// For more information, see Step Scaling Policies for Application Auto Scaling
			// (https://docs.aws.amazon.com/autoscaling/application/userguide/application-auto-scaling-step-scaling-policies.html)
			// and Target Tracking Scaling Policies for Application Auto Scaling (https://docs.aws.amazon.com/autoscaling/application/userguide/application-auto-scaling-target-tracking.html)
			// in the Application Auto Scaling User Guide.
			PolicyType: "TargetTrackingScaling",

			// The minimum value to scale to in response to a scale-in event. MinCapacity
			// is required to register a scalable target.
			MinCapacity: ctx.DesiredCount,

			// The maximum value to scale to in response to a scale-out event. MaxCapacity
			// is required to register a scalable target.
			MaxCapacity: ctx.DesiredCount * 2,

			// A target tracking scaling policy. Includes support for predefined or customized metrics.
			TargetTrackingScalingPolicyConfiguration: &applicationautoscaling.TargetTrackingScalingPolicyConfiguration{

				// A predefined metric. You can specify either a predefined metric or a customized
				// metric.
				PredefinedMetricSpecification: &applicationautoscaling.PredefinedMetricSpecification{
					// The metric type. The following predefined metrics are available:
					//
					//    * ASGAverageCPUUtilization - Average CPU utilization of the Auto Scaling
					//    group.
					//
					//    * ASGAverageNetworkIn - Average number of bytes received on all network
					//    interfaces by the Auto Scaling group.
					//
					//    * ASGAverageNetworkOut - Average number of bytes sent out on all network
					//    interfaces by the Auto Scaling group.
					//
					//    * ALBRequestCountPerTarget - Number of requests completed per target in
					//    an Application Load Balancer target group. ResourceLabel will be auto populated.
					//
					PredefinedMetricType: aws.String("ECSServiceAverageCPUUtilization"),
				},

				// The target value for the metric. The range is 8.515920e-109 to 1.174271e+108
				// (Base 10) or 2e-360 to 2e360 (Base 2).
				TargetValue: aws.Float64(70.0),

				// The amount of time, in seconds, after a scale-in activity completes before
				// another scale in activity can start.
				//
				// The cooldown period is used to block subsequent scale-in requests until it
				// has expired. The intention is to scale in conservatively to protect your
				// application's availability. However, if another alarm triggers a scale-out
				// policy during the cooldown period after a scale-in, Application Auto Scaling
				// scales out your scalable target immediately.
				ScaleInCooldown: aws.Int64(300),

				// The amount of time, in seconds, after a scale-out activity completes before
				// another scale-out activity can start.
				//
				// While the cooldown period is in effect, the capacity that has been added
				// by the previous scale-out event that initiated the cooldown is calculated
				// as part of the desired capacity for the next scale out. The intention is
				// to continuously (but not excessively) scale out.
				ScaleOutCooldown: aws.Int64(300),

				// Indicates whether scale in by the target tracking scaling policy is disabled.
				// If the value is true, scale in is disabled and the target tracking scaling
				// policy won't remove capacity from the scalable resource. Otherwise, scale
				// in is enabled and the target tracking scaling policy can remove capacity
				// from the scalable resource. The default value is false.
				DisableScaleIn: aws.Bool(false),
			},
		}
	}

	// Load the web-app config for the web-api can reference it's hostname.
	webAppCtx, err := NewServiceContext(ServiceWebApp, cfg)
	if err != nil {
		return nil, err
	}

	// Load the web-api config for the web-app can reference it's hostname.
	webApiCtx, err := NewServiceContext(ServiceWebApi, cfg)
	if err != nil {
		return nil, err
	}

	// Try to find the Datadog API key, this value is optional.
	// If Datadog API key is not specified, then integration with Datadog for observability will not be active.
	datadogApiKey, err := getDatadogApiKey(cfg)
	if err != nil {
		return srv, err
	}

	// Add the Datadog container to the task definition if an API Key is set.
	var ddContainer *ecs.ContainerDefinition
	if datadogApiKey != "" {
		ddTags := []string{
			"source:docker",
			"service:" + srv.AwsEcsService.ServiceName,
			"service_name:" + ctx.Name,
			"cluster:" + srv.AwsEcsCluster.ClusterName,
			"env:" + cfg.Env,
		}

		// Defined a container definition for the specific service.
		ddContainer = &ecs.ContainerDefinition{
			Name:      aws.String("datadog-agent"),
			Image:     aws.String(srv.ReleaseImage),
			Essential: aws.Bool(true),
			PortMappings: []*ecs.PortMapping{
				&ecs.PortMapping{
					ContainerPort: aws.Int64(8125),
				},
				&ecs.PortMapping{
					ContainerPort: aws.Int64(8126),
				},
			},
			Cpu:               aws.Int64(128),
			MemoryReservation: aws.Int64(256),
			Environment: []*ecs.KeyValuePair{
				ecsKeyValuePair("DD_API_KEY", datadogApiKey),
				ecsKeyValuePair("DD_LOGS_ENABLED", "true"),
				ecsKeyValuePair("DD_APM_ENABLED", "true"),
				ecsKeyValuePair("DD_RECEIVER_PORT", "8126"),
				ecsKeyValuePair("DD_APM_NON_LOCAL_TRAFFIC", "true"),
				ecsKeyValuePair("DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL", "true"),
				ecsKeyValuePair("DD_TAGS", strings.Join(ddTags, " ")),
				ecsKeyValuePair("DD_DOGSTATSD_ORIGIN_DETECTION", "true"),
				ecsKeyValuePair("DD_DOGSTATSD_NON_LOCAL_TRAFFIC", "true"),
				ecsKeyValuePair("ECS_FARGATE", "true"),
			},
		}

	}

	// Define a base set of environment variables that can be assigned to individual container definitions.
	baseEnvVals := func() []*ecs.KeyValuePair {

		var ciJobURL string
		if id := os.Getenv("CI_JOB_ID"); id != "" {
			ciJobURL = strings.TrimRight(GitLabProjectBaseUrl, "/") + "/-/jobs/" + os.Getenv("CI_JOB_ID")
		}

		var ciPipelineURL string
		if id := os.Getenv("CI_PIPELINE_ID"); id != "" {
			ciPipelineURL = strings.TrimRight(GitLabProjectBaseUrl, "/") + "/pipelines/" + os.Getenv("CI_PIPELINE_ID")
		}

		envVars := []*ecs.KeyValuePair{
			ecsKeyValuePair(devdeploy.ENV_KEY_ECS_CLUSTER, srv.AwsEcsCluster.ClusterName),
			ecsKeyValuePair(devdeploy.ENV_KEY_ECS_SERVICE, srv.AwsEcsService.ServiceName),
			ecsKeyValuePair("AWS_DEFAULT_REGION", cfg.AwsCredentials.Region),
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
			ecsKeyValuePair("WEB_APP_BASE_URL", webAppCtx.BaseUrl()),
			ecsKeyValuePair("WEB_API_BASE_URL", webApiCtx.BaseUrl()),
			ecsKeyValuePair("EMAIL_SENDER", "lee+saas-starter-kit@geeksinthewoods.com"),
		}

		if datadogApiKey != "" {
			envVars = append(envVars, ecsKeyValuePair("DATADOG_ADDR", "127.0.0.1:8125"),
				ecsKeyValuePair("DD_API_KEY", datadogApiKey),
				ecsKeyValuePair("DD_TRACE_AGENT_PORT", "8126"),
				ecsKeyValuePair("DD_SERVICE_NAME", srv.AwsEcsService.ServiceName),
				ecsKeyValuePair("DD_ENV", cfg.Env))
		}

		return envVars
	}

	// =========================================================================
	// Service dependant settings.
	switch serviceName {

	// Define the ServiceContext for the web-app that will be used for build and deploy.
	case ServiceWebApp:

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
			Environment:       baseEnvVals(),
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
		if ctx.EnableHTTPS && !enableElb {
			container1.PortMappings = append(container1.PortMappings, &ecs.PortMapping{
				HostPort:      aws.Int64(443),
				Protocol:      aws.String("tcp"),
				ContainerPort: aws.Int64(443),
			})
		}

		// Define the full task definition for the service.
		taskDef := &ecs.RegisterTaskDefinitionInput{
			Family:      aws.String(fmt.Sprintf("%s-%s-%s", cfg.Env, srv.AwsEcsCluster.ClusterName, ctx.Name)),
			NetworkMode: aws.String("awsvpc"),
			ContainerDefinitions: []*ecs.ContainerDefinition{
				// Include the single container definition for the service. Additional definitions could be added
				// here like one for datadog.
				container1,
			},
			RequiresCompatibilities: aws.StringSlice([]string{"FARGATE"}),
		}

		// Append the datadog container if defined.
		if ddContainer != nil {
			taskDef.ContainerDefinitions = append(taskDef.ContainerDefinitions, ddContainer)
		}

		srv.AwsEcsTaskDefinition = &devdeploy.AwsEcsTaskDefinition{
			RegisterInput: taskDef,
			PreRegister: func(input *ecs.RegisterTaskDefinitionInput, vars devdeploy.AwsEcsServiceDeployVariables) error {
				// Append env vars for the service task.
				input.ContainerDefinitions[0].Environment = append(input.ContainerDefinitions[0].Environment,
					ecsKeyValuePair("SERVICE_NAME", ctx.Name),
					ecsKeyValuePair("PROJECT_NAME", cfg.ProjectName),

					// Use placeholders for these environment variables that will be replaced with devdeploy.DeployServiceToTargetEnv
					ecsKeyValuePair("WEB_APP_HTTP_HOST", vars.HTTPHost),
					ecsKeyValuePair("WEB_APP_HTTPS_HOST", vars.HTTPSHost),
					ecsKeyValuePair("WEB_APP_SERVICE_ENABLE_HTTPS", strconv.FormatBool(vars.HTTPSEnabled)),
					ecsKeyValuePair("WEB_APP_SERVICE_BASE_URL", vars.ServiceBaseUrl),
					ecsKeyValuePair("WEB_APP_SERVICE_HOST_NAMES", strings.Join(vars.AlternativeHostnames, ",")),
					ecsKeyValuePair("WEB_APP_SERVICE_STATICFILES_S3_ENABLED", strconv.FormatBool(vars.StaticFilesS3Enabled)),
					ecsKeyValuePair("WEB_APP_SERVICE_STATICFILES_S3_PREFIX", vars.StaticFilesS3Prefix),
					ecsKeyValuePair("WEB_APP_SERVICE_STATICFILES_CLOUDFRONT_ENABLED", strconv.FormatBool(vars.StaticFilesCloudfrontEnabled)),
					ecsKeyValuePair("WEB_APP_REDIS_HOST", vars.CacheHost),
					ecsKeyValuePair("WEB_APP_DB_HOST", vars.DbHost),
					ecsKeyValuePair("WEB_APP_DB_USERNAME", vars.DbUser),
					ecsKeyValuePair("WEB_APP_DB_PASSWORD", vars.DbPass),
					ecsKeyValuePair("WEB_APP_DB_DATABASE", vars.DbName),
					ecsKeyValuePair("WEB_APP_DB_DRIVER", vars.DbDriver),
					ecsKeyValuePair("WEB_APP_DB_DISABLE_TLS", strconv.FormatBool(vars.DbDisableTLS)),
					ecsKeyValuePair("WEB_APP_AWS_S3_BUCKET_PRIVATE", vars.AwsS3BucketNamePrivate),
					ecsKeyValuePair("WEB_APP_AWS_S3_BUCKET_PUBLIC", vars.AwsS3BucketNamePublic),
				)

				// When no Elastic Load Balance is used, tasks need to be able to directly update the Route 53 records.
				if vars.AwsElbLoadBalancer == nil {
					input.ContainerDefinitions[0].Environment = append(input.ContainerDefinitions[0].Environment,
						ecsKeyValuePair(devdeploy.ENV_KEY_ROUTE53_ZONES, vars.EncodeRoute53Zones()),
						ecsKeyValuePair(devdeploy.ENV_KEY_ROUTE53_UPDATE_TASK_IPS, "true"))
				}

				return nil
			},
		}

	// Define the ServiceContext for the web-api that will be used for build and deploy.
	case ServiceWebApi:

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
			Environment:       baseEnvVals(),
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
		if ctx.EnableHTTPS && !enableElb {
			container1.PortMappings = append(container1.PortMappings, &ecs.PortMapping{
				HostPort:      aws.Int64(443),
				Protocol:      aws.String("tcp"),
				ContainerPort: aws.Int64(443),
			})
		}

		// Define the full task definition for the service.
		taskDef := &ecs.RegisterTaskDefinitionInput{
			Family:      aws.String(fmt.Sprintf("%s-%s-%s", cfg.Env, srv.AwsEcsCluster.ClusterName, ctx.Name)),
			NetworkMode: aws.String("awsvpc"),
			ContainerDefinitions: []*ecs.ContainerDefinition{
				// Include the single container definition for the service. Additional definitions could be added
				// here like one for datadog.
				container1,
			},
			RequiresCompatibilities: aws.StringSlice([]string{"FARGATE"}),
		}

		// Append the datadog container if defined.
		if ddContainer != nil {
			taskDef.ContainerDefinitions = append(taskDef.ContainerDefinitions, ddContainer)
		}

		srv.AwsEcsTaskDefinition = &devdeploy.AwsEcsTaskDefinition{
			RegisterInput: taskDef,
			PreRegister: func(input *ecs.RegisterTaskDefinitionInput, vars devdeploy.AwsEcsServiceDeployVariables) error {
				// Append env vars for the service task.
				input.ContainerDefinitions[0].Environment = append(input.ContainerDefinitions[0].Environment,
					ecsKeyValuePair("SERVICE_NAME", ctx.Name),
					ecsKeyValuePair("PROJECT_NAME", cfg.ProjectName),

					// Use placeholders for these environment variables that will be replaced with devdeploy.DeployServiceToTargetEnv
					ecsKeyValuePair("WEB_API_HTTP_HOST", vars.HTTPHost),
					ecsKeyValuePair("WEB_API_HTTPS_HOST", vars.HTTPSHost),
					ecsKeyValuePair("WEB_API_SERVICE_ENABLE_HTTPS", strconv.FormatBool(vars.HTTPSEnabled)),
					ecsKeyValuePair("WEB_API_SERVICE_BASE_URL", vars.ServiceBaseUrl),
					ecsKeyValuePair("WEB_API_SERVICE_HOST_NAMES", strings.Join(vars.AlternativeHostnames, ",")),
					ecsKeyValuePair("WEB_API_SERVICE_STATICFILES_S3_ENABLED", strconv.FormatBool(vars.StaticFilesS3Enabled)),
					ecsKeyValuePair("WEB_API_SERVICE_STATICFILES_S3_PREFIX", vars.StaticFilesS3Prefix),
					ecsKeyValuePair("WEB_API_SERVICE_STATICFILES_CLOUDFRONT_ENABLED", strconv.FormatBool(vars.StaticFilesCloudfrontEnabled)),
					ecsKeyValuePair("WEB_API_REDIS_HOST", vars.CacheHost),
					ecsKeyValuePair("WEB_API_DB_HOST", vars.DbHost),
					ecsKeyValuePair("WEB_API_DB_USERNAME", vars.DbUser),
					ecsKeyValuePair("WEB_API_DB_PASSWORD", vars.DbPass),
					ecsKeyValuePair("WEB_API_DB_DATABASE", vars.DbName),
					ecsKeyValuePair("WEB_API_DB_DRIVER", vars.DbDriver),
					ecsKeyValuePair("WEB_API_DB_DISABLE_TLS", strconv.FormatBool(vars.DbDisableTLS)),
					ecsKeyValuePair("WEB_API_AWS_S3_BUCKET_PRIVATE", vars.AwsS3BucketNamePrivate),
					ecsKeyValuePair("WEB_API_AWS_S3_BUCKET_PUBLIC", vars.AwsS3BucketNamePublic),
				)

				// When no Elastic Load Balance is used, tasks need to be able to directly update the Route 53 records.
				if vars.AwsElbLoadBalancer == nil {
					input.ContainerDefinitions[0].Environment = append(input.ContainerDefinitions[0].Environment,
						ecsKeyValuePair(devdeploy.ENV_KEY_ROUTE53_ZONES, vars.EncodeRoute53Zones()),
						ecsKeyValuePair(devdeploy.ENV_KEY_ROUTE53_UPDATE_TASK_IPS, "true"))
				}

				return nil
			},
		}

		srv.DockerBuildArgs["swagInit"] = "1"

	default:
		return nil, errors.Wrapf(devdeploy.ErrInvalidService,
			"No service context defined for service '%s'",
			serviceName)
	}

	// Set the docker file if no custom one has been defined for the service.
	if srv.Dockerfile == "" {
		srv.Dockerfile = filepath.Join(srv.CodeDir, "Dockerfile")
	}

	if srv.StaticFilesDir == "" {
		srv.StaticFilesDir = filepath.Join(srv.CodeDir, "static")
	}

	// When only service host names are set, choose the first item as the primary host.
	if srv.ServiceHostPrimary == "" && len(srv.ServiceHostNames) > 0 {
		srv.ServiceHostPrimary = srv.ServiceHostNames[0]
		log.Printf("\t\tSet Service Primary Host to '%s'.", srv.ServiceHostPrimary)
	}

	return srv, nil
}

// BuildServiceForTargetEnv executes the build commands for a target service.
func BuildServiceForTargetEnv(log *log.Logger, awsCredentials devdeploy.AwsCredentials, targetEnv Env, serviceName, releaseTag string, dryRun, noCache, noPush bool) error {

	cfg, err := NewConfig(log, targetEnv, awsCredentials)
	if err != nil {
		return err
	}

	targetSvc, err := NewService(serviceName, cfg)
	if err != nil {
		return err
	}

	// Override the release tag if set.
	if releaseTag != "" {
		targetSvc.ReleaseTag = releaseTag
	}

	// Append build args to be used for all services.
	if targetSvc.DockerBuildArgs == nil {
		targetSvc.DockerBuildArgs = make(map[string]string)
	}

	// servicePath is used to copy the service specific code in the Dockerfile.
	codePath, err := filepath.Rel(cfg.ProjectRoot, targetSvc.CodeDir)
	if err != nil {
		return err
	}
	targetSvc.DockerBuildArgs["code_path"] = codePath

	// commitRef is used by main.go:build constant.
	commitRef := getCommitRef()
	if commitRef == "" {
		commitRef = targetSvc.ReleaseTag
	}
	targetSvc.DockerBuildArgs["commit_ref"] = commitRef

	if dryRun {
		cfgJSON, err := json.MarshalIndent(cfg, "", "    ")
		if err != nil {
			log.Fatalf("BuildServiceForTargetEnv : Marshalling config to JSON : %+v", err)
		}
		log.Printf("BuildServiceForTargetEnv : config : %v\n", string(cfgJSON))

		detailsJSON, err := json.MarshalIndent(targetSvc, "", "    ")
		if err != nil {
			log.Fatalf("BuildServiceForTargetEnv : Marshalling details to JSON : %+v", err)
		}
		log.Printf("BuildServiceForTargetEnv : details : %v\n", string(detailsJSON))

		return nil
	}

	return devdeploy.BuildServiceForTargetEnv(log, cfg, targetSvc, noCache, noPush)
}

// DeployServiceForTargetEnv executes the build commands for a target service.
func DeployServiceForTargetEnv(log *log.Logger, awsCredentials devdeploy.AwsCredentials, targetEnv Env, serviceName, releaseTag string, dryRun bool) error {

	cfg, err := NewConfig(log, targetEnv, awsCredentials)
	if err != nil {
		return err
	}

	targetSvc, err := NewService(serviceName, cfg)
	if err != nil {
		return err
	}

	// Override the release tag if set.
	if releaseTag != "" {
		targetSvc.ReleaseTag = releaseTag
	}

	return devdeploy.DeployServiceToTargetEnv(log, cfg, targetSvc)
}

// ecsKeyValuePair returns an *ecs.KeyValuePair
func ecsKeyValuePair(name, value string) *ecs.KeyValuePair {
	return &ecs.KeyValuePair{
		Name:  aws.String(name),
		Value: aws.String(value),
	}
}
