package config

import (
	"log"
	"path/filepath"

	"encoding/json"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	"gitlab.com/geeks-accelerator/oss/devops/pkg/devdeploy"
)

// Function define the name of a function.
type Function = string

var (
	Function_Ddlogscollector       = "ddlogscollector"
	Function_YourNewFunction       = "your-new-function"
)

// List of function names used by main.go for help.
var FunctionNames = []Function{
	// Python Datadog Logs Collector
	Function_Ddlogscollector,
	Function_YourNewFunction,
}

// FunctionContext defines the flags for deploying a function.
type FunctionContext struct {
	// Required flags.
	Name              string                       `validate:"required" example:"aws-lambda-go-func"`
	AwsLambdaFunction *devdeploy.AwsLambdaFunction `validate:"required"`
	AwsIamRole        *devdeploy.AwsIamRole        `validate:"required"`
	AwsIamPolicy      *devdeploy.AwsIamPolicy      `validate:"required"`

	// Optional flags.
	FunctionDir        string `validate:"omitempty"`
	BuildDir           string `validate:"omitempty"`
	DockerBuildContext string `validate:"omitempty" example:"."`
	Dockerfile         string `validate:"required" example:"./cmd/web-api/Dockerfile"`
	ReleaseTag         string `validate:"required"`
	EnableVPC          bool   `validate:"omitempty" example:"false"`
}

// NewFunctionContext returns the FunctionContext.
func NewFunctionContext(funcName string, cfg *devdeploy.Config) (*FunctionContext, error) {

	ctx := &FunctionContext{
		Name: funcName,

		FunctionDir: filepath.Join(cfg.ProjectRoot, "examples", funcName),

		DockerBuildContext: ".",

		// Set the release tag for the image to use include env + service name + commit hash/tag.
		ReleaseTag: devdeploy.GitLabCiReleaseTag(cfg.Env, funcName),
	}

	switch funcName {
	case Function_YourNewFunction:
		// No additional settings for function.
	case Function_Ddlogscollector:

		// Python Datadog Logs Collector is
		ctx.FunctionDir = filepath.Join(cfg.ProjectRoot, "deployments/ddlogscollector")

		// Change the build directory to the function directory instead of project root.
		ctx.BuildDir = ctx.FunctionDir

		// AwsLambdaFunction defines the details needed to create an lambda function.
		ctx.AwsLambdaFunction = &devdeploy.AwsLambdaFunction{
			FunctionName: ctx.Name,
			Description:  "Ship logs from cloudwatch to datadog",

			Handler:    "lambda_function.lambda_handler",
			Runtime:    "python2.7",
			MemorySize: 512,

			Timeout: aws.Int64(300),
			Environment: map[string]string{
				"DD_API_KEY":  "",
				"LAMBDA_FUNC": ctx.Name,
			},
			Tags: []devdeploy.Tag{
				{Key: devdeploy.AwsTagNameProject, Value: cfg.ProjectName},
				{Key: devdeploy.AwsTagNameEnv, Value: cfg.Env},
			},
		}

		ctx.AwsIamRole = &devdeploy.AwsIamRole{
			RoleName:                 "DatadogAWSIntegrationLambdaRole",
			Description:              "Allows Datadog to run Lambda functions to call AWS services on your behalf.",
			AssumeRolePolicyDocument: "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":[\"lambda.amazonaws.com\"]},\"Action\":[\"sts:AssumeRole\"]}]}",
			Tags: []devdeploy.Tag{
				{Key: devdeploy.AwsTagNameProject, Value: cfg.ProjectName},
				{Key: devdeploy.AwsTagNameEnv, Value: cfg.Env},
			},
		}

		ctx.AwsIamPolicy = &devdeploy.AwsIamPolicy{
			PolicyName:  "DatadogAWSIntegrationPolicy",
			Description: "Provides Datadog Lambda function the ability to ship AWS service related logs back to Datadog.",
			PolicyDocument: devdeploy.AwsIamPolicyDocument{
				Version: "2012-10-17",
				Statement: []devdeploy.AwsIamStatementEntry{
					{
						Action: []string{
							"apigateway:GET",
							"autoscaling:Describe*",
							"budgets:ViewBudget",
							"cloudfront:GetDistributionConfig",
							"cloudfront:ListDistributions",
							"cloudtrail:DescribeTrails",
							"cloudtrail:GetTrailStatus",
							"cloudwatch:Describe*",
							"cloudwatch:Get*",
							"cloudwatch:List*",
							"codedeploy:List*",
							"codedeploy:BatchGet*",
							"directconnect:Describe*",
							"dynamodb:List*",
							"dynamodb:Describe*",
							"ec2:Describe*",
							"ecs:Describe*",
							"ecs:List*",
							"elasticache:Describe*",
							"elasticache:List*",
							"elasticfilesystem:DescribeFileSystems",
							"elasticfilesystem:DescribeTags",
							"elasticloadbalancing:Describe*",
							"elasticmapreduce:List*",
							"elasticmapreduce:Describe*",
							"es:ListTags",
							"es:ListDomainNames",
							"es:DescribeElasticsearchDomains",
							"health:DescribeEvents",
							"health:DescribeEventDetails",
							"health:DescribeAffectedEntities",
							"kinesis:List*",
							"kinesis:Describe*",
							"lambda:AddPermission",
							"lambda:GetPolicy",
							"lambda:List*",
							"lambda:RemovePermission",
							"logs:Get*",
							"logs:Describe*",
							"logs:FilterLogEvents",
							"logs:TestMetricFilter",
							"logs:PutSubscriptionFilter",
							"logs:DeleteSubscriptionFilter",
							"logs:DescribeSubscriptionFilters",
							"rds:Describe*",
							"rds:List*",
							"redshift:DescribeClusters",
							"redshift:DescribeLoggingStatus",
							"route53:List*",
							"s3:GetBucketLogging",
							"s3:GetBucketLocation",
							"s3:GetBucketNotification",
							"s3:GetBucketTagging",
							"s3:ListAllMyBuckets",
							"s3:PutBucketNotification",
							"ses:Get*",
							"sns:List*",
							"sns:Publish",
							"sqs:ListQueues",
							"support:*",
							"tag:GetResources",
							"tag:GetTagKeys",
							"tag:GetTagValues",
							"xray:BatchGetTraces",
							"xray:GetTraceSummaries",
							"lambda:List*",
							"logs:DescribeLogGroups",
							"logs:DescribeLogStreams",
							"logs:FilterLogEvents",
							"tag:GetResources",
							"cloudfront:GetDistributionConfig",
							"cloudfront:ListDistributions",
							"elasticloadbalancing:DescribeLoadBalancers",
							"elasticloadbalancing:DescribeLoadBalancerAttributes",
							"lambda:AddPermission",
							"lambda:GetPolicy",
							"lambda:RemovePermission",
							"redshift:DescribeClusters",
							"redshift:DescribeLoggingStatus",
							"s3:GetBucketLogging",
							"s3:GetBucketLocation",
							"s3:GetBucketNotification",
							"s3:ListAllMyBuckets",
							"s3:PutBucketNotification",
							"logs:PutSubscriptionFilter",
							"logs:DeleteSubscriptionFilter",
							"logs:DescribeSubscriptionFilters",
						},
						Effect:   "Allow",
						Resource: "*",
					},
				},
			},
		}
	default:
		return nil, errors.Wrapf(devdeploy.ErrInvalidFunction,
			"No function context defined for function '%s'",
			funcName)
	}

	// Append the datadog api key before execution.
	ctx.AwsLambdaFunction.UpdateEnvironment = func(vars map[string]string) error {
		datadogApiKey, err := getDatadogApiKey(cfg)
		if err != nil {
			return err
		}
		vars["DD_API_KEY"] = datadogApiKey
		return nil
	}

	// Set the docker file if no custom one has been defined for the service.
	if ctx.Dockerfile == "" {
		ctx.Dockerfile = filepath.Join(ctx.BuildDir, "Dockerfile")
	}

	return ctx, nil
}

// Build handles defining all the information needed to deploy a service to AWS ECS.
func (ctx *FunctionContext) Build(log *log.Logger, noCache, noPush bool) (*devdeploy.BuildLambda, error) {

	log.Printf("Define build for function '%s'.", ctx.Name)
	log.Printf("\tUsing release tag %s.", ctx.ReleaseTag)

	srv := &devdeploy.BuildLambda{
		FuncName:           ctx.Name,
		ReleaseTag:         ctx.ReleaseTag,
		BuildDir:           ctx.BuildDir,
		Dockerfile:         ctx.Dockerfile,
		DockerBuildContext: ctx.DockerBuildContext,
		NoCache:            noCache,
		NoPush:             noPush,
	}

	return srv, nil
}

// Deploy handles defining all the information needed to deploy a service to AWS ECS.
func (ctx *FunctionContext) Deploy(log *log.Logger) (*devdeploy.DeployLambda, error) {

	log.Printf("Define build for function '%s'.", ctx.Name)
	log.Printf("\tUsing release tag %s.", ctx.ReleaseTag)

	srv := &devdeploy.DeployLambda{
		FuncName:          ctx.Name,
		EnableVPC:         ctx.EnableVPC,
		AwsLambdaFunction: ctx.AwsLambdaFunction,
		AwsIamPolicy:      ctx.AwsIamPolicy,
		AwsIamRole:        ctx.AwsIamRole,
	}

	return srv, nil
}

// S3Location returns the s3 bucket and key used to upload the code to.
func (ctx *FunctionContext) S3Location(cfg *devdeploy.Config) (string, string) {
	s3Bucket := cfg.AwsS3BucketPrivate.BucketName
	s3Key := filepath.Join("src", "aws", "lambda", cfg.Env, ctx.Name, ctx.ReleaseTag+".zip")

	return s3Bucket, s3Key
}

// BuildFunctionForTargetEnv executes the build commands for a target function.
func BuildFunctionForTargetEnv(log *log.Logger, awsCredentials devdeploy.AwsCredentials, targetEnv Env, functionName, releaseTag string, dryRun, noCache, noPush bool) error {

	cfgCtx, err := NewConfigContext(targetEnv, awsCredentials)
	if err != nil {
		return err
	}

	cfg, err := cfgCtx.Config(log)
	if err != nil {
		return err
	}

	funcCtx, err := NewFunctionContext(functionName, cfg)
	if err != nil {
		return err
	}

	// Override the release tag if set.
	if releaseTag != "" {
		funcCtx.ReleaseTag = releaseTag
	}

	details, err := funcCtx.Build(log, noCache, noPush)
	if err != nil {
		return err
	}

	// Set the s3 bucket and s3 for uploading the zip file.
	details.CodeS3Bucket, details.CodeS3Key = funcCtx.S3Location(cfg)

	// funcPath is used to copy the service specific code in the Dockerfile.
	funcPath, err := filepath.Rel(cfg.ProjectRoot, funcCtx.FunctionDir)
	if err != nil {
		return err
	}

	// commitRef is used by main.go:build constant.
	commitRef := getCommitRef()
	if commitRef == "" {
		commitRef = funcCtx.ReleaseTag
	}

	details.BuildArgs = map[string]string{
		"func_path":  funcPath,
		"commit_ref": commitRef,
	}

	if dryRun {
		cfgJSON, err := json.MarshalIndent(cfg, "", "    ")
		if err != nil {
			log.Fatalf("BuildFunctionForTargetEnv : Marshalling config to JSON : %+v", err)
		}
		log.Printf("BuildFunctionForTargetEnv : config : %v\n", string(cfgJSON))

		detailsJSON, err := json.MarshalIndent(details, "", "    ")
		if err != nil {
			log.Fatalf("BuildFunctionForTargetEnv : Marshalling details to JSON : %+v", err)
		}
		log.Printf("BuildFunctionForTargetEnv : details : %v\n", string(detailsJSON))

		return nil
	}

	return devdeploy.BuildLambdaForTargetEnv(log, cfg, details)
}

// DeployFunctionForTargetEnv executes the deploy commands for a target function.
func DeployFunctionForTargetEnv(log *log.Logger, awsCredentials devdeploy.AwsCredentials, targetEnv Env, functionName, releaseTag string, dryRun bool) error {

	cfgCtx, err := NewConfigContext(targetEnv, awsCredentials)
	if err != nil {
		return err
	}

	cfg, err := cfgCtx.Config(log)
	if err != nil {
		return err
	}

	funcCtx, err := NewFunctionContext(functionName, cfg)
	if err != nil {
		return err
	}

	// Override the release tag if set.
	if releaseTag != "" {
		funcCtx.ReleaseTag = releaseTag
	}

	details, err := funcCtx.Deploy(log)
	if err != nil {
		return err
	}

	// Set the s3 bucket and s3 for uploading the zip file.
	details.CodeS3Bucket, details.CodeS3Key = funcCtx.S3Location(cfg)

	if dryRun {
		cfgJSON, err := json.MarshalIndent(cfg, "", "    ")
		if err != nil {
			log.Fatalf("DeployFunctionForTargetEnv : Marshalling config to JSON : %+v", err)
		}
		log.Printf("DeployFunctionForTargetEnv : config : %v\n", string(cfgJSON))

		detailsJSON, err := json.MarshalIndent(details, "", "    ")
		if err != nil {
			log.Fatalf("DeployFunctionForTargetEnv : Marshalling details to JSON : %+v", err)
		}
		log.Printf("DeployFunctionForTargetEnv : details : %v\n", string(detailsJSON))

		return nil
	}

	return devdeploy.DeployLambdaToTargetEnv(log, cfg, details)
}
