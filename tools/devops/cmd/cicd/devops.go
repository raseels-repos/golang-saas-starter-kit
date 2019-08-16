package cicd

import (
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"gitlab.com/geeks-accelerator/oss/devops/pkg/devdeploy"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// DeployContext defines the flags for defining the deployment env.
type DeployContext struct {
	// Env is the target environment used for the deployment.
	Env         string `validate:"oneof=dev stage prod"`

	// AwsCredentials defines the credentials used for deployment.
	AwsCredentials devdeploy.AwsCredentials `validate:"required,dive,required"`
}

// DefineDeploymentEnv handles defining all the information needed to setup the target env including RDS and cache.
func DefineDeploymentEnv(log *log.Logger, ctx DeployContext) (*devdeploy.DeploymentEnv, error) {

	// If AWS Credentials are not set and use role is not enabled, try to load the credentials from env vars.
	if ctx.AwsCredentials.UseRole == false && ctx.AwsCredentials.AccessKeyID == "" {
		var err error
		ctx.AwsCredentials, err = devdeploy.GetAwsCredentialsFromEnv(ctx.Env)
		if err != nil {
			return nil, err
		}
	}

	targetEnv := &devdeploy.DeploymentEnv{
		Env: ctx.Env,
		AwsCredentials: ctx.AwsCredentials,
	}

	// Get the current working directory. This should be somewhere contained within the project.
	workDir, err := os.Getwd()
	if err != nil {
		return targetEnv, errors.WithMessage(err, "Failed to get current working directory.")
	}

	// Set the project root directory and project name. This is current set by finding the go.mod file for the project
	// repo. Project name is the directory name.
	modDetails, err := devdeploy.LoadModuleDetails(workDir)
	if err != nil {
		return targetEnv, err
	}

	// ProjectRoot should be the root directory for the project.
	targetEnv.ProjectRoot = modDetails.ProjectRoot

	// ProjectName will be used for prefixing AWS resources. This could be changed as needed or manually defined.
	targetEnv.ProjectName = modDetails.ProjectName

	// Set default AWS ECR Repository Name.
	targetEnv.AwsEcrRepository = &devdeploy.AwsEcrRepository{
		RepositoryName: targetEnv.ProjectName,
		Tags: []devdeploy.Tag{
			{Key: awsTagNameProject, Value:  targetEnv.ProjectName},
			{Key: awsTagNameEnv, Value: targetEnv.Env},
		},
	}

	// Set the deployment to use the default VPC for the region.
	targetEnv.AwsEc2Vpc = &devdeploy.AwsEc2Vpc{
		IsDefault : true,
	}

	// Set the security group to use for the deployed services, database and cluster. This will used the VPC ID defined
	// for the deployment.
	targetEnv.AwsEc2SecurityGroup =  &devdeploy.AwsEc2SecurityGroup{
		GroupName: targetEnv.ProjectName + "-" + ctx.Env,
		Description: fmt.Sprintf("Security group for %s services running on ECS", targetEnv.ProjectName),
	}

	// Set the name of the EC2 Security Group used by the gitlab runner. This is used to ensure the security
	// group defined above has access to the RDS cluster/instance and can thus handle schema migrations.
	targetEnv.GitlabRunnerEc2SecurityGroupName = "gitlab-runner"

	// Set the s3 buckets used by the deployed services.
	// S3 temp prefix used by services for short term storage. A lifecycle policy will be used for expiration.
	s3BucketTempPrefix := "tmp/"

	// Defines a life cycle policy to expire keys for the temp directory.
	bucketLifecycleTempRule := &s3.LifecycleRule{
		ID:     aws.String("Rule for : " + s3BucketTempPrefix),
		Status: aws.String("Enabled"),
		Filter: &s3.LifecycleRuleFilter{
			Prefix: aws.String(s3BucketTempPrefix),
		},
		Expiration: &s3.LifecycleExpiration{
			// Indicates the lifetime, in days, of the objects that are subject to the rule.
			// The value must be a non-zero positive integer.
			Days: aws.Int64(1),
		},
		// Specifies the days since the initiation of an incomplete multipart upload
		// that Amazon S3 will wait before permanently removing all parts of the upload.
		// For more information, see Aborting Incomplete Multipart Uploads Using a Bucket
		// Lifecycle Policy (https://docs.aws.amazon.com/AmazonS3/latest/dev/mpuoverview.html#mpu-abort-incomplete-mpu-lifecycle-config)
		// in the Amazon Simple Storage Service Developer Guide.
		AbortIncompleteMultipartUpload: &s3.AbortIncompleteMultipartUpload{
			DaysAfterInitiation: aws.Int64(1),
		},
	}

	// Define the public S3 bucket used to serve static files for all the services.
	targetEnv.AwsS3BucketPublic = &devdeploy.AwsS3Bucket{
		BucketName: targetEnv.ProjectName+"-public",
		IsPublic: true,
		TempPrefix: s3BucketTempPrefix,
		LocationConstraint: &ctx.AwsCredentials.Region,
		LifecycleRules: []*s3.LifecycleRule{bucketLifecycleTempRule},
		CORSRules: []*s3.CORSRule{
			&s3.CORSRule{
				// Headers that are specified in the Access-Control-Request-Headers header.
				// These headers are allowed in a preflight OPTIONS request. In response to
				// any preflight OPTIONS request, Amazon S3 returns any requested headers that
				// are allowed.
				// AllowedHeaders: aws.StringSlice([]string{}),

				// An HTTP method that you allow the origin to execute. Valid values are GET,
				// PUT, HEAD, POST, and DELETE.
				//
				// AllowedMethods is a required field
				AllowedMethods: aws.StringSlice([]string{"GET", "POST"}),

				// One or more origins you want customers to be able to access the bucket from.
				//
				// AllowedOrigins is a required field
				AllowedOrigins: aws.StringSlice([]string{"*"}),

				// One or more headers in the response that you want customers to be able to
				// access from their applications (for example, from a JavaScript XMLHttpRequest
				// object).
				// ExposeHeaders: aws.StringSlice([]string{}),

				// The time in seconds that your browser is to cache the preflight response
				// for the specified resource.
				// MaxAgeSeconds: aws.Int64(),
			},
		},
	}

	// The base s3 key prefix used to upload static files.
	targetEnv.AwsS3BucketPublicKeyPrefix = "/public"

	// For production, enable Cloudfront CND for all static files to avoid serving them from the slower S3 option.
	if ctx.Env == webcontext.Env_Prod {
		targetEnv.AwsS3BucketPublic.CloudFront =  &devdeploy.AwsS3BucketCloudFront{
			// S3 key prefix to request your content from a directory in your Amazon S3 bucket.
			OriginPath : targetEnv.AwsS3BucketPublicKeyPrefix ,

			// A complex type that controls whether CloudFront caches the response to requests.
			CachedMethods: []string{"HEAD", "GET"},

			// The distribution's configuration information.
			DistributionConfig: &cloudfront.DistributionConfig{
				Comment:       aws.String(""),
				Enabled:       aws.Bool(true),
				HttpVersion:   aws.String("http2"),
				IsIPV6Enabled: aws.Bool(true),
				DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
					Compress:       aws.Bool(true),
					DefaultTTL:     aws.Int64(1209600),
					MinTTL:         aws.Int64(604800),
					MaxTTL:         aws.Int64(31536000),
					ForwardedValues: &cloudfront.ForwardedValues{
						QueryString: aws.Bool(true),
						Cookies: &cloudfront.CookiePreference{
							Forward: aws.String("none"),
						},
					},
					TrustedSigners: &cloudfront.TrustedSigners{
						Enabled:  aws.Bool(false),
						Quantity: aws.Int64(0),
					},
					ViewerProtocolPolicy: aws.String("allow-all"),
				},
				ViewerCertificate: &cloudfront.ViewerCertificate{
					CertificateSource:            aws.String("cloudfront"),
					MinimumProtocolVersion:       aws.String("TLSv1"),
					CloudFrontDefaultCertificate: aws.Bool(true),
				},
				PriceClass:      aws.String("PriceClass_All"),
				CallerReference: aws.String("devops-deploy"),
			},
		}
	}

	// Define the private S3 bucket used for long term file storage including but not limited to: log exports,
	// AWS Lambda code, application caching.
	targetEnv.AwsS3BucketPrivate = &devdeploy.AwsS3Bucket{
		BucketName: targetEnv.ProjectName+"-private",
		IsPublic: false,
		TempPrefix: s3BucketTempPrefix,
		LocationConstraint: &ctx.AwsCredentials.Region,
		LifecycleRules: []*s3.LifecycleRule{bucketLifecycleTempRule},
		PublicAccessBlock: &s3.PublicAccessBlockConfiguration{
			// Specifies whether Amazon S3 should block public access control lists (ACLs)
			// for this bucket and objects in this bucket. Setting this element to TRUE
			// causes the following behavior:
			//
			//    * PUT Bucket acl and PUT Object acl calls fail if the specified ACL is
			//    public.
			//
			//    * PUT Object calls fail if the request includes a public ACL.
			//
			// Enabling this setting doesn't affect existing policies or ACLs.
			BlockPublicAcls: aws.Bool(true),

			// Specifies whether Amazon S3 should block public bucket policies for this
			// bucket. Setting this element to TRUE causes Amazon S3 to reject calls to
			// PUT Bucket policy if the specified bucket policy allows public access.
			//
			// Enabling this setting doesn't affect existing bucket policies.
			BlockPublicPolicy: aws.Bool(true),

			// Specifies whether Amazon S3 should restrict public bucket policies for this
			// bucket. Setting this element to TRUE restricts access to this bucket to only
			// AWS services and authorized users within this account if the bucket has a
			// public policy.
			//
			// Enabling this setting doesn't affect previously stored bucket policies, except
			// that public and cross-account access within any public bucket policy, including
			// non-public delegation to specific accounts, is blocked.
			RestrictPublicBuckets: aws.Bool(true),

			// Specifies whether Amazon S3 should ignore public ACLs for this bucket and
			// objects in this bucket. Setting this element to TRUE causes Amazon S3 to
			// ignore all public ACLs on this bucket and objects in this bucket.
			//
			// Enabling this setting doesn't affect the persistence of any existing ACLs
			// and doesn't prevent new public ACLs from being set.
			IgnorePublicAcls: aws.Bool(true),
		},
	}

	// Add a bucket policy to enable exports from Cloudwatch Logs for the private S3 bucket.
	targetEnv.AwsS3BucketPrivate.Policy = func() string {
		policyResource := strings.Trim(filepath.Join(targetEnv.AwsS3BucketPrivate.BucketName, targetEnv.AwsS3BucketPrivate.TempPrefix), "/")
		return fmt.Sprintf(`{
				"Version": "2012-10-17",
				"Statement": [
				  {
					  "Action": "s3:GetBucketAcl",
					  "Effect": "Allow",
					  "Resource": "arn:aws:s3:::%s",
					  "Principal": { "Service": "logs.%s.amazonaws.com" }
				  },
				  {
					  "Action": "s3:PutObject" ,
					  "Effect": "Allow",
					  "Resource": "arn:aws:s3:::%s/*",
					  "Condition": { "StringEquals": { "s3:x-amz-acl": "bucket-owner-full-control" } },
					  "Principal": { "Service": "logs.%s.amazonaws.com" }
				  }
				]
			}`, targetEnv.AwsS3BucketPrivate.BucketName, ctx.AwsCredentials.Region, policyResource, ctx.AwsCredentials.Region)
	}()

	// Define the Redis Cache cluster used for ephemeral storage.
	targetEnv.AwsElasticCacheCluster = &devdeploy.AwsElasticCacheCluster{
		CacheClusterId:          targetEnv.ProjectName + "-" + ctx.Env,
		CacheNodeType:           "cache.t2.micro",
		CacheSubnetGroupName:    "default",
		Engine:                  "redis",
		EngineVersion:          "5.0.4",
		NumCacheNodes:           1,
		Port:                   6379,
		AutoMinorVersionUpgrade: aws.Bool(true),
		SnapshotRetentionLimit:  aws.Int64(7),
		ParameterNameValues: []devdeploy.AwsElasticCacheParameter{
			devdeploy.AwsElasticCacheParameter{
				ParameterName:"maxmemory-policy",
				ParameterValue: "allkeys-lru",
			},
		},
	}

	// Define the RDS Database instance for transactional data. A random one will be generated for any created instance.
	targetEnv.AwsRdsDBInstance = &devdeploy.AwsRdsDBInstance{
		DBInstanceIdentifier:     targetEnv.ProjectName + "-" + ctx.Env,
		DBName:                   "shared",
		Engine:                   "postgres",
		MasterUsername:            "god",
		Port:                     5432,
		DBInstanceClass:       "db.t2.small",
		AllocatedStorage:          20,
		CharacterSetName: aws.String("UTF8"),
		PubliclyAccessible:   false,
		BackupRetentionPeriod:     aws.Int64(7),
		AutoMinorVersionUpgrade:   true,
		CopyTagsToSnapshot:        aws.Bool(true),
		Tags: []devdeploy.Tag{
			{Key: awsTagNameProject, Value:  targetEnv.ProjectName},
			{Key: awsTagNameEnv, Value: targetEnv.Env},
		},
	}

	return targetEnv, nil
}

// ServiceContext defines the flags for deploying a service.
type ServiceContext struct {
	// Required flags.
	ServiceName string `validate:"required" example:"web-api"`

	// Optional flags.
	EnableHTTPS              bool            `validate:"omitempty" example:"false"`
	EnableElb    bool   `validate:"omitempty" example:"false"`
	ServiceHostPrimary       string          `validate:"omitempty" example:"example-project.com"`
	ServiceHostNames         cli.StringSlice `validate:"omitempty" example:"subdomain.example-project.com"`
	DesiredCount int `validate:"omitempty" example:"2"`
	Dockerfile      string `validate:"omitempty" example:"./cmd/web-api/Dockerfile"`
	ServiceDir      string `validate:"omitempty" example:"./cmd/web-api"`

	StaticFilesS3Enable        bool `validate:"omitempty" example:"false"`
	StaticFilesImgResizeEnable bool `validate:"omitempty" example:"false"`

	RecreateService bool `validate:"omitempty" example:"false"`
}

// DefineDeployService handles defining all the information needed to deploy a service to AWS ECS.
func DefineDeployService(log *log.Logger, ctx ServiceContext, targetEnv *devdeploy.DeploymentEnv) (*devdeploy.DeployService, error) {

	log.Printf("\tDefine deploy for service '%s'.", ctx.ServiceName)

	// Start to define all the information for the service from the service context.
	srv := &devdeploy.DeployService{
		DeploymentEnv: targetEnv,
		ServiceName: ctx.ServiceName,
		EnableHTTPS: ctx.EnableHTTPS,
		ServiceHostPrimary: ctx.ServiceHostPrimary,
		ServiceHostNames: ctx.ServiceHostNames,
		StaticFilesImgResizeEnable: ctx.StaticFilesImgResizeEnable,
	}

	// When only service host names are set, choose the first item as the primary host.
	if srv.ServiceHostPrimary == "" && len(srv.ServiceHostNames) > 0 {
		srv.ServiceHostPrimary = srv.ServiceHostNames[0]
		log.Printf("\t\tSet Service Primary Host to '%s'.", srv.ServiceHostPrimary)
	}

	// Set the release tag for the image to use include env + service name + commit hash/tag.
	srv.ReleaseTag = devdeploy.GitLabCiReleaseTag(targetEnv.Env, srv.ServiceName)
	log.Printf("\t\tSet ReleaseTag '%s'.", srv.ReleaseTag)

	// The S3 prefix used to upload static files served to public.
	if ctx.StaticFilesS3Enable {
		srv.StaticFilesS3Prefix = filepath.Join(targetEnv.AwsS3BucketPublicKeyPrefix, srv.ReleaseTag, "static")
	}

	// Determine the Dockerfile for the service.
	if  ctx.Dockerfile != "" {
		srv.Dockerfile = ctx.Dockerfile
		log.Printf("\t\tUsing docker file '%s'.", srv.Dockerfile)
	} else {
		var err error
		srv.Dockerfile, err = devdeploy.FindServiceDockerFile(targetEnv.ProjectRoot, srv.ServiceName)
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
		ClusterName: targetEnv.ProjectName + "-" + targetEnv.Env,
		Tags: []devdeploy.Tag{
			{Key: awsTagNameProject, Value:  targetEnv.ProjectName},
			{Key: awsTagNameEnv, Value: targetEnv.Env},
		},
	}

	// Define the ECS task execution role. This role executes ECS actions such as pulling the image and storing the
	// application logs in cloudwatch.
	srv.AwsEcsExecutionRole = &devdeploy.AwsIamRole{
		RoleName: fmt.Sprintf("ecsExecutionRole%s%s", targetEnv.ProjectNameCamel(), strcase.ToCamel(targetEnv.Env)),
		Description:             fmt.Sprintf("Provides access to other AWS service resources that are required to run Amazon ECS tasks for %s. ", targetEnv.ProjectName),
		AssumeRolePolicyDocument: "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":[\"ecs-tasks.amazonaws.com\"]},\"Action\":[\"sts:AssumeRole\"]}]}",
		Tags: []devdeploy.Tag{
			{Key: awsTagNameProject, Value:  targetEnv.ProjectName},
			{Key: awsTagNameEnv, Value: targetEnv.Env},
		},
		AttachRolePolicyArns: []string{"arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"},
	}
	log.Printf("\t\tSet ECS Execution Role Name to '%s'.", srv.AwsEcsExecutionRole)

	// Define the ECS task role. This role is used by the task itself for calling other AWS services.
	srv.AwsEcsTaskRole = &devdeploy.AwsIamRole{
		RoleName: fmt.Sprintf("ecsTaskRole%s%s", targetEnv.ProjectNameCamel(), strcase.ToCamel(targetEnv.Env)),
		Description:             fmt.Sprintf("Allows ECS tasks for %s to call AWS services on your behalf.", targetEnv.ProjectName),
		AssumeRolePolicyDocument:"{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":[\"ecs-tasks.amazonaws.com\"]},\"Action\":[\"sts:AssumeRole\"]}]}",
		Tags: []devdeploy.Tag{
			{Key: awsTagNameProject, Value:  targetEnv.ProjectName},
			{Key: awsTagNameEnv, Value: targetEnv.Env},
		},
	}
	log.Printf("\t\tSet ECS Task Role Name to '%s'.", srv.AwsEcsTaskRole)

	// AwsEcsTaskPolicy defines the name and policy that will be attached to the task role. The policy document grants
	// the permissions required for deployed services to access AWS services. If the policy already exists, the
	// statements will be used to add new required actions, but not for removal.
	srv.AwsEcsTaskPolicy  = &devdeploy.AwsIamPolicy{
		PolicyName: fmt.Sprintf("%s%sServices", targetEnv.ProjectNameCamel(), strcase.ToCamel(targetEnv.Env)),
		Description: fmt.Sprintf("Defines access for %s services. ", targetEnv.ProjectName),
		PolicyDocument:  devdeploy.AwsIamPolicyDocument{
			Version: "2012-10-17",
			Statement: []devdeploy.AwsIamStatementEntry{
				{
					Sid:    "DefaultServiceAccess",
					Effect: "Allow",
					Action: []string{
						"s3:HeadBucket",
						"s3:ListObjects",
						"s3:PutObject",
						"s3:PutObjectAcl",
						"cloudfront:ListDistributions",
						"ec2:DescribeNetworkInterfaces",
						"ec2:DeleteNetworkInterface",
						"ecs:ListTasks",
						"ecs:DescribeServices",
						"ecs:DescribeTasks",
						"ec2:DescribeNetworkInterfaces",
						"route53:ListHostedZones",
						"route53:ListResourceRecordSets",
						"route53:ChangeResourceRecordSets",
						"ecs:UpdateService",
						"ses:SendEmail",
						"ses:ListIdentities",
						"secretsmanager:ListSecretVersionIds",
						"secretsmanager:GetSecretValue",
						"secretsmanager:CreateSecret",
						"secretsmanager:UpdateSecret",
						"secretsmanager:RestoreSecret",
						"secretsmanager:DeleteSecret",
					},
					Resource: "*",
				},
				{
					Sid:    "ServiceInvokeLambda",
					Effect: "Allow",
					Action: []string{
						"iam:GetRole",
						"lambda:InvokeFunction",
						"lambda:ListVersionsByFunction",
						"lambda:GetFunction",
						"lambda:InvokeAsync",
						"lambda:GetFunctionConfiguration",
						"iam:PassRole",
						"lambda:GetAlias",
						"lambda:GetPolicy",
					},
					Resource: []string{
						"arn:aws:iam:::role/*",
						"arn:aws:lambda:::function:*",
					},
				},
				{
					Sid:    "datadoglambda",
					Effect: "Allow",
					Action: []string{
						"cloudwatch:Get*",
						"cloudwatch:List*",
						"ec2:Describe*",
						"support:*",
						"tag:GetResources",
						"tag:GetTagKeys",
						"tag:GetTagValues",
					},
					Resource: "*",
				},
			},
		},
	}
	log.Printf("\t\tSet ECS Task Policy Name to '%s'.", srv.AwsEcsTaskPolicy.PolicyName)

	// AwsCloudWatchLogGroup defines the name of the cloudwatch log group that will be used to store logs for the ECS tasks.
	srv.AwsCloudWatchLogGroup = &devdeploy.AwsCloudWatchLogGroup {
		LogGroupName: fmt.Sprintf("logs/env_%s/aws/ecs/cluster_%s/service_%s", targetEnv.Env, srv.AwsEcsCluster.ClusterName, srv.ServiceName),
		Tags: []devdeploy.Tag{
			{Key: awsTagNameProject, Value:  targetEnv.ProjectName},
			{Key: awsTagNameEnv, Value: targetEnv.Env},
		},
	}
	log.Printf("\t\tSet AWS Log Group Name to '%s'.", srv.AwsCloudWatchLogGroup.LogGroupName)

	// AwsSdPrivateDnsNamespace defines the service discovery group.
	srv.AwsSdPrivateDnsNamespace = &devdeploy.AwsSdPrivateDnsNamespace{
		Name:        srv.AwsEcsCluster.ClusterName,
		Description: fmt.Sprintf("Private DNS namespace used for services running on the ECS Cluster %s", srv.AwsEcsCluster.ClusterName),
		Service: &devdeploy.AwsSdService{
				Name:        ctx.ServiceName,
				Description: fmt.Sprintf("Service %s running on the ECS Cluster %s",ctx.ServiceName, srv.AwsEcsCluster.ClusterName),
				DnsRecordTTL: 300,
				HealthCheckFailureThreshold: 3,
		},
	}
	log.Printf("\t\tSet AWS Service Discovery Namespace to '%s'.", srv.AwsSdPrivateDnsNamespace.Name)

	// If the service is requested to use an elastic load balancer then define.
	if ctx.EnableElb {
		// AwsElbLoadBalancer defines if the service should use an elastic load balancer.
		srv.AwsElbLoadBalancer = &devdeploy.AwsElbLoadBalancer{
			Name: fmt.Sprintf("%s-%s-%s", targetEnv.Env, srv.AwsEcsCluster.ClusterName, srv.ServiceName),
			IpAddressType: "ipv4",
			Scheme: "internet-facing",
			Type: "application",
			Tags: []devdeploy.Tag{
				{Key: awsTagNameProject, Value:  targetEnv.ProjectName},
				{Key: awsTagNameEnv, Value: targetEnv.Env},
			},
		}
		log.Printf("\t\tSet ELB Name to '%s'.", srv.AwsElbLoadBalancer.Name)

		// Define the target group for service to receive HTTP traffic from the load balancer.
		srv.AwsElbLoadBalancer.TargetGroup  = &devdeploy.AwsElbTargetGroup{
			Name: fmt.Sprintf("%s-http", srv.ServiceName),
			Port: 80,
			Protocol:"HTTP",
			TargetType: "ip",
			HealthCheckEnabled: true,
			HealthCheckIntervalSeconds: 30,
			HealthCheckPath: "/ping",
			HealthCheckProtocol: "HTTP",
			HealthCheckTimeoutSeconds: 5,
			HealthyThresholdCount: 3,
			UnhealthyThresholdCount: 3,
			Matcher: "200",
		}
		log.Printf("\t\t\tSet ELB Target Group Name for %s to '%s'.",
			srv.AwsElbLoadBalancer.TargetGroup.Protocol,
			srv.AwsElbLoadBalancer.TargetGroup .Name)

		// Set ECS configs based on specified env.
		if targetEnv.Env == "prod" {
			srv.AwsElbLoadBalancer.EcsTaskDeregistrationDelay = 300
		} else {
			// Force staging to deploy immediately without waiting for connections to drain
			srv.AwsElbLoadBalancer.EcsTaskDeregistrationDelay =0
		}
	}

	// AwsEcsService defines the details for the ecs service.
	srv.AwsEcsService = &devdeploy.AwsEcsService{
		ServiceName: ctx.ServiceName,
		DesiredCount: int64(ctx.DesiredCount),
		EnableECSManagedTags: false,
		HealthCheckGracePeriodSeconds: 60,
		LaunchType: "FARGATE",
	}

	// Ensure when deploying a new service there is always at-least one running.
	if srv.AwsEcsService.DesiredCount == 0 {
		srv.AwsEcsService.DesiredCount = 1
	}

	// Set ECS configs based on specified env.
	if targetEnv.Env == "prod" {
		srv.AwsEcsService .DeploymentMinimumHealthyPercent =100
		srv.AwsEcsService .DeploymentMaximumPercent = 200
	} else {
		srv.AwsEcsService .DeploymentMinimumHealthyPercent = 100
		srv.AwsEcsService .DeploymentMaximumPercent = 200
	}


	// Read the defined json task definition for the service.
	dat, err := devdeploy.EcsReadTaskDefinition(ctx.ServiceDir, targetEnv.Env)
	if err != nil {
		return  srv, err
	}

	// JSON decode the task definition.
	taskDef, err := devdeploy.ParseTaskDefinitionInput(dat)
	if err != nil {
		return  srv, err
	}

	// AwsEcsTaskDefinition defines the details for registering a new ECS task definition.
	srv.AwsEcsTaskDefinition = &devdeploy.AwsEcsTaskDefinition{
		RegisterInput: taskDef,
		UpdatePlaceholders: func(placeholders map[string]string) error {

			// Try to find the Datadog API key, this value is optional.
			// If Datadog API key is not specified, then integration with Datadog for observability will not be active.
			{
				// Load Datadog API key which can be either stored in an environment variable or in AWS Secrets Manager.
				// 1. Check env vars for [DEV|STAGE|PROD]_DD_API_KEY and DD_API_KEY
				datadogApiKey := devdeploy.GetTargetEnv(targetEnv.Env, "DD_API_KEY")

				// 2. Check AWS Secrets Manager for datadog entry prefixed with target environment.
				if datadogApiKey == "" {
					prefixedSecretId := secretID(targetEnv.ProjectName, targetEnv.Env, "datadog")
					var err error
					datadogApiKey, err = devdeploy.GetAwsSecretValue(targetEnv.AwsCredentials, prefixedSecretId)
					if err != nil {
						if aerr, ok := errors.Cause(err).(awserr.Error); !ok || aerr.Code() != secretsmanager.ErrCodeResourceNotFoundException {
							return err
						}
					}
				}

				// 3. Check AWS Secrets Manager for Datadog entry.
				if datadogApiKey == "" {
					secretId := "DATADOG"
					var err error
					datadogApiKey, err = devdeploy.GetAwsSecretValue(targetEnv.AwsCredentials, secretId)
					if err != nil {
						if aerr, ok := errors.Cause(err).(awserr.Error); !ok || aerr.Code() != secretsmanager.ErrCodeResourceNotFoundException {
							return err
						}
					}
				}

				if datadogApiKey != "" {
					log.Printf("\t%s\tAPI Key set.\n", tests.Success)
				} else {
					log.Printf("\t%s\tAPI Key NOT set.\n", tests.Failed)
				}

				placeholders["{DATADOG_APIKEY}"] =       datadogApiKey



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

	return srv, nil
}

// FunctionContext defines the flags for deploying a function.
type FunctionContext struct {
	EnableVPC bool   `validate:"omitempty" example:"false"`
}
