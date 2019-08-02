package cicd

import (
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/internal/schema"
	"geeks-accelerator/oss/saas-starter-kit/tools/devops/internal/retry"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/servicediscovery"
	"github.com/bobesa/go-domain-util/domainutil"
	"github.com/iancoleman/strcase"
	"github.com/lib/pq"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	sqltrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/database/sql"
	sqlxtrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/jmoiron/sqlx"
	"gopkg.in/go-playground/validator.v9"
)

// ServiceDeployFlags defines the flags used for executing a service deployment.
type ServiceDeployFlags struct {
	// Required flags.
	ServiceName string `validate:"required" example:"web-api"`
	Env         string `validate:"oneof=dev stage prod" example:"dev"`

	// Optional flags.
	EnableHTTPS              bool            `validate:"omitempty" example:"false"`
	ServiceHostPrimary       string          `validate:"omitempty" example:"example-project.com"`
	ServiceHostNames         cli.StringSlice `validate:"omitempty" example:"subdomain.example-project.com"`
	S3BucketPrivateName      string          `validate:"omitempty" example:"saas-example-project-private"`
	S3BucketPublicName       string          `validate:"omitempty" example:"saas-example-project-public"`
	S3BucketPublicCloudfront bool            `validate:"omitempty" example:"false"`

	ProjectRoot     string `validate:"omitempty" example:"."`
	ProjectName     string ` validate:"omitempty" example:"example-project"`
	DockerFile      string `validate:"omitempty" example:"./cmd/web-api/Dockerfile"`
	EnableLambdaVPC bool   `validate:"omitempty" example:"false"`
	EnableEcsElb    bool   `validate:"omitempty" example:"false"`

	StaticFilesS3Enable        bool `validate:"omitempty" example:"false"`
	StaticFilesImgResizeEnable bool `validate:"omitempty" example:"false"`

	RecreateService bool `validate:"omitempty" example:"false"`
}

// serviceDeployRequest defines the details needed to execute a service deployment.
type serviceDeployRequest struct {
	*serviceRequest

	EnableHTTPS        bool     `validate:"omitempty"`
	ServiceHostPrimary string   `validate:"omitempty,required_with=EnableHTTPS,fqdn"`
	ServiceHostNames   []string `validate:"omitempty,dive,fqdn"`

	EcrRepositoryName string `validate:"required"`

	EcsClusterName string `validate:"required"`
	EcsCluster     *ecs.CreateClusterInput

	EcsServiceName                          string `validate:"required"`
	EcsServiceDesiredCount                  int64  `validate:"required"`
	EcsServiceMinimumHealthyPercent         *int64 `validate:"omitempty"`
	EcsServiceMaximumPercent                *int64 `validate:"omitempty"`
	EscServiceHealthCheckGracePeriodSeconds *int64 `validate:"omitempty"`

	EcsExecutionRoleName       string `validate:"required"`
	EcsExecutionRole           *iam.CreateRoleInput
	EcsExecutionRolePolicyArns []string `validate:"required"`

	EcsTaskRoleName string `validate:"required"`
	EcsTaskRole     *iam.CreateRoleInput

	EcsTaskPolicyName     string `validate:"required"`
	EcsTaskPolicy         *iam.CreatePolicyInput
	EcsTaskPolicyDocument IamPolicyDocument

	Ec2SecurityGroupName string `validate:"required"`
	Ec2SecurityGroup     *ec2.CreateSecurityGroupInput

	GitlabRunnerEc2SecurityGroupName string `validate:"required"`

	CloudWatchLogGroupName string `validate:"required"`
	CloudWatchLogGroup     *cloudwatchlogs.CreateLogGroupInput

	S3BucketTempPrefix      string `validate:"required_with=S3BucketPrivateName S3BucketPublicName"`
	S3BucketPrivateName     string `validate:"omitempty"`
	S3BucketPublicName      string `validate:"omitempty"`
	S3BucketPublicKeyPrefix string `validate:"omitempty"`
	S3Buckets               []S3Bucket

	CloudfrontPublic *cloudfront.DistributionConfig

	StaticFilesS3Enable        bool   `validate:"omitempty"`
	StaticFilesS3Prefix        string `validate:"omitempty"`
	StaticFilesImgResizeEnable bool   `validate:"omitempty"`

	EnableEcsElb           bool   `validate:"omitempty"`
	ElbLoadBalancerName    string `validate:"omitempty"`
	ElbDeregistrationDelay *int   `validate:"omitempty"`
	ElbLoadBalancer        *elbv2.CreateLoadBalancerInput

	ElbTargetGroupName string `validate:"omitempty"`
	ElbTargetGroup     *elbv2.CreateTargetGroupInput

	VpcPublicName    string `validate:"omitempty"`
	VpcPublic        *ec2.CreateVpcInput
	VpcPublicSubnets []*ec2.CreateSubnetInput

	EnableLambdaVPC bool `validate:"omitempty"`
	RecreateService bool `validate:"omitempty"`

	SDNamepsace *servicediscovery.CreatePrivateDnsNamespaceInput
	SDService   *servicediscovery.CreateServiceInput

	CacheCluster          *elasticache.CreateCacheClusterInput
	CacheClusterParameter []*elasticache.ParameterNameValue

	DBCluster  *rds.CreateDBClusterInput
	DBInstance *rds.CreateDBInstanceInput

	flags ServiceDeployFlags
}

// NewServiceDeployRequest generates a new request for executing deployment of a single service for a given set of CLI flags.
func NewServiceDeployRequest(log *log.Logger, flags ServiceDeployFlags) (*serviceDeployRequest, error) {

	// Validates specified CLI flags map to struct successfully.
	log.Println("Validate flags.")
	{
		errs := validator.New().Struct(flags)
		if errs != nil {
			return nil, errs
		}
		log.Printf("\t%s\tFlags ok.", tests.Success)
	}

	// Generate a deploy request using CLI flags and AWS credentials.
	log.Println("Generate deploy request.")
	var req serviceDeployRequest
	{
		// Define new service request.
		sr := &serviceRequest{
			ServiceName: flags.ServiceName,
			Env:         flags.Env,
			ProjectRoot: flags.ProjectRoot,
			ProjectName: flags.ProjectName,
			DockerFile:  flags.DockerFile,
		}
		if err := sr.init(log); err != nil {
			return nil, err
		}

		req = serviceDeployRequest{
			serviceRequest: sr,

			EnableHTTPS:        flags.EnableHTTPS,
			ServiceHostPrimary: flags.ServiceHostPrimary,
			ServiceHostNames:   flags.ServiceHostNames,

			StaticFilesS3Enable:        flags.StaticFilesS3Enable,
			StaticFilesImgResizeEnable: flags.StaticFilesImgResizeEnable,

			S3BucketPrivateName: flags.S3BucketPrivateName,
			S3BucketPublicName:  flags.S3BucketPublicName,

			EnableLambdaVPC: flags.EnableLambdaVPC,
			EnableEcsElb:    flags.EnableEcsElb,
			RecreateService: flags.RecreateService,

			flags: flags,
		}

		// Set default configuration values. Primarily setting default values for all the AWS services:
		// - AWS S3 bucket settings
		// - AWS ECR repository settings
		// - AWS ECS cluster, service, task, and task policy settings
		// - AWS CloudWatch group settings
		// - AWS EC2 security groups
		// - AWS ECS settings and enable ELB
		// - AWS Elastic Cache settings for a Redis cache cluster
		// - AWS RDS configuration for Postgres via Aurora
		log.Println("\tSet defaults.")
		{
			// When only service host names are set, choose the first item as the primary host.
			if req.ServiceHostPrimary == "" && len(req.ServiceHostNames) > 0 {
				req.ServiceHostPrimary = req.ServiceHostNames[0]
				log.Printf("\t\t\tSet Service Primary Host to '%s'.", req.ServiceHostPrimary)
			}

			// S3 temp prefix used by services for short term storage. A lifecycle policy will be used for expiration.
			req.S3BucketTempPrefix = "tmp/"

			// Defines a life cycle policy to expire keys for the temp directory.
			bucketLifecycleTempRule := &s3.LifecycleRule{
				ID:     aws.String("Rule for : " + req.S3BucketTempPrefix),
				Status: aws.String("Enabled"),
				Filter: &s3.LifecycleRuleFilter{
					Prefix: aws.String(req.S3BucketTempPrefix),
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

			// Defines the S3 Buckets used for all services.
			// The public S3 Bucket used to serve static files and other assets.
			if req.S3BucketPublicName != "" {
				req.S3Buckets = append(req.S3Buckets,
					S3Bucket{
						Name: req.S3BucketPublicName,
						Input: &s3.CreateBucketInput{
							Bucket: aws.String(req.S3BucketPublicName),
						},
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
					})

				// The S3 key prefix used as the origin when cloud front is enabled.
				if req.S3BucketPublicKeyPrefix == "" {
					req.S3BucketPublicKeyPrefix = "/public"
				}

				if flags.S3BucketPublicCloudfront {
					allowedMethods := &cloudfront.AllowedMethods{
						Items: aws.StringSlice([]string{"HEAD", "GET"}),
					}
					allowedMethods.Quantity = aws.Int64(int64(len(allowedMethods.Items)))

					cacheMethods := &cloudfront.CachedMethods{
						Items: aws.StringSlice([]string{"HEAD", "GET"}),
					}
					cacheMethods.Quantity = aws.Int64(int64(len(cacheMethods.Items)))
					allowedMethods.SetCachedMethods(cacheMethods)

					domainId := "S3-" + req.S3BucketPublicName
					domainName := fmt.Sprintf("%s.s3.%s.amazonaws.com", req.S3BucketPublicName, req.AwsCreds.Region)

					origins := &cloudfront.Origins{
						Items: []*cloudfront.Origin{
							&cloudfront.Origin{
								Id:         aws.String(domainId),
								DomainName: aws.String(domainName),
								OriginPath: aws.String(req.S3BucketPublicKeyPrefix),
								S3OriginConfig: &cloudfront.S3OriginConfig{
									OriginAccessIdentity: aws.String(""),
								},
								CustomHeaders: &cloudfront.CustomHeaders{
									Quantity: aws.Int64(0),
								},
							},
						},
					}
					origins.Quantity = aws.Int64(int64(len(origins.Items)))

					req.CloudfrontPublic = &cloudfront.DistributionConfig{
						Comment:       aws.String(""),
						Enabled:       aws.Bool(true),
						HttpVersion:   aws.String("http2"),
						IsIPV6Enabled: aws.Bool(true),
						DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
							TargetOriginId: aws.String(domainId),
							AllowedMethods: allowedMethods,
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
						Origins: origins,
						ViewerCertificate: &cloudfront.ViewerCertificate{
							CertificateSource:            aws.String("cloudfront"),
							MinimumProtocolVersion:       aws.String("TLSv1"),
							CloudFrontDefaultCertificate: aws.Bool(true),
						},
						PriceClass:      aws.String("PriceClass_All"),
						CallerReference: aws.String("devops-deploy"),
					}
				}
			}

			// The private S3 Bucket used to persist data for services.
			if req.S3BucketPrivateName != "" {
				req.S3Buckets = append(req.S3Buckets,
					S3Bucket{
						Name: req.S3BucketPrivateName,
						Input: &s3.CreateBucketInput{
							Bucket: aws.String(req.S3BucketPrivateName),
						},
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
						Policy: func() string {
							// Add a bucket policy to enable exports from Cloudwatch Logs for the private S3 bucket.
							policyResource := strings.Trim(filepath.Join(req.S3BucketPrivateName, req.S3BucketTempPrefix), "/")
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
						}`, req.S3BucketPrivateName, req.AwsCreds.Region, policyResource, req.AwsCreds.Region)
						}(),
					})
			}

			// The S3 prefix used to upload static files served to public.
			if req.StaticFilesS3Prefix == "" {
				req.StaticFilesS3Prefix = filepath.Join(req.S3BucketPublicKeyPrefix, releaseTag(req.Env, req.ServiceName), "static")
			}

			// Set default AWS ECR Repository Name.
			req.EcrRepositoryName = ecrRepositoryName(req.ProjectName)
			log.Printf("\t\t\tSet ECR Repository Name to '%s'.", req.EcrRepositoryName)

			// Set default AWS ECS Cluster Name.
			req.EcsClusterName = req.ProjectName + "-" + req.Env
			req.EcsCluster = &ecs.CreateClusterInput{
				ClusterName: aws.String(req.EcsClusterName),
				Tags: []*ecs.Tag{
					&ecs.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
					&ecs.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
				},
			}
			log.Printf("\t\t\tSet ECS Cluster Name to '%s'.", req.EcsClusterName)

			// Set default AWS ECS Service Name.
			req.EcsServiceName = req.ServiceName + "-" + req.Env
			log.Printf("\t\t\tSet ECS Service Name to '%s'.", req.EcsServiceName)

			// Set default AWS ECS Execution Role Name.
			req.EcsExecutionRoleName = fmt.Sprintf("ecsExecutionRole%s%s", req.ProjectNameCamel(), strcase.ToCamel(req.Env))
			req.EcsExecutionRole = &iam.CreateRoleInput{
				RoleName:                 aws.String(req.EcsExecutionRoleName),
				Description:              aws.String(fmt.Sprintf("Provides access to other AWS service resources that are required to run Amazon ECS tasks for %s. ", req.ProjectName)),
				AssumeRolePolicyDocument: aws.String("{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":[\"ecs-tasks.amazonaws.com\"]},\"Action\":[\"sts:AssumeRole\"]}]}"),
				Tags: []*iam.Tag{
					&iam.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
					&iam.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
				},
			}
			req.EcsExecutionRolePolicyArns = []string{
				"arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy",
			}
			log.Printf("\t\t\tSet ECS Execution Role Name to '%s'.", req.EcsExecutionRoleName)

			// Set default AWS ECS Task Role Name.
			req.EcsTaskRoleName = fmt.Sprintf("ecsTaskRole%s%s", req.ProjectNameCamel(), strcase.ToCamel(req.Env))
			req.EcsTaskRole = &iam.CreateRoleInput{
				RoleName:                 aws.String(req.EcsTaskRoleName),
				Description:              aws.String(fmt.Sprintf("Allows ECS tasks for %s to call AWS services on your behalf.", req.ProjectName)),
				AssumeRolePolicyDocument: aws.String("{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":[\"ecs-tasks.amazonaws.com\"]},\"Action\":[\"sts:AssumeRole\"]}]}"),
				Tags: []*iam.Tag{
					&iam.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
					&iam.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
				},
			}
			log.Printf("\t\t\tSet ECS Task Role Name to '%s'.", req.EcsTaskRoleName)

			// Set default AWS ECS Task Policy Name.
			req.EcsTaskPolicyName = fmt.Sprintf("%s%sServices", req.ProjectNameCamel(), strcase.ToCamel(req.Env))
			req.EcsTaskPolicy = &iam.CreatePolicyInput{
				PolicyName:  aws.String(req.EcsTaskPolicyName),
				Description: aws.String(fmt.Sprintf("Defines access for %s services. ", req.ProjectName)),
			}
			log.Printf("\t\t\tSet ECS Task Policy Name to '%s'.", req.EcsTaskPolicyName)

			// EcsTaskPolicyDocument defines the default document policy used to create the AWS ECS Task Policy. If the
			// policy already exists, the permissions will be used to add new required actions, but not for removal.
			// The policy document grants the permissions required for deployed services to access AWS services.
			req.EcsTaskPolicyDocument = IamPolicyDocument{
				Version: "2012-10-17",
				Statement: []IamStatementEntry{
					IamStatementEntry{
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
					IamStatementEntry{
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
					IamStatementEntry{
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
			}

			// Set default Cloudwatch Log Group Name.
			req.CloudWatchLogGroupName = fmt.Sprintf("logs/env_%s/aws/ecs/cluster_%s/service_%s", req.Env, req.EcsClusterName, req.ServiceName)
			req.CloudWatchLogGroup = &cloudwatchlogs.CreateLogGroupInput{
				LogGroupName: aws.String(req.CloudWatchLogGroupName),
				Tags: map[string]*string{
					awsTagNameProject: aws.String(req.ProjectName),
					awsTagNameEnv:     aws.String(req.Env),
				},
			}
			log.Printf("\t\t\tSet CloudWatch Log Group Name to '%s'.", req.CloudWatchLogGroupName)

			// Set default EC2 Security Group Name.
			req.Ec2SecurityGroupName = req.EcsClusterName
			req.Ec2SecurityGroup = &ec2.CreateSecurityGroupInput{
				// The name of the security group.
				// Constraints: Up to 255 characters in length. Cannot start with sg-.
				// Constraints for EC2-Classic: ASCII characters
				// Constraints for EC2-VPC: a-z, A-Z, 0-9, spaces, and ._-:/()#,@[]+=&;{}!$*
				// GroupName is a required field
				GroupName: aws.String(req.Ec2SecurityGroupName),

				// A description for the security group. This is informational only.
				// Constraints: Up to 255 characters in length
				// Constraints for EC2-Classic: ASCII characters
				// Constraints for EC2-VPC: a-z, A-Z, 0-9, spaces, and ._-:/()#,@[]+=&;{}!$*
				// Description is a required field
				Description: aws.String(fmt.Sprintf("Security group for %s running on ECS cluster %s", req.ProjectName, req.EcsClusterName)),
			}
			log.Printf("\t\t\tSet ECS Security Group Name to '%s'.", req.Ec2SecurityGroupName)

			// Set the name of the EC2 Security Group used by the gitlab runner. This is used to ensure the security
			// group defined above has access to the RDS cluster/instance and can thus handle schema migrations.
			req.GitlabRunnerEc2SecurityGroupName = "gitlab-runner"

			// Set default ELB Load Balancer Name when ELB is enabled.
			if req.EnableEcsElb {
				if !strings.Contains(req.EcsClusterName, req.Env) && !strings.Contains(req.ServiceName, req.Env) {
					// When a custom cluster name is provided and/or service name, ensure the ELB contains the current env.
					req.ElbLoadBalancerName = fmt.Sprintf("%s-%s-%s", req.EcsClusterName, req.ServiceName, req.Env)
				} else {
					// Default value when when custom cluster/service name is supplied.
					req.ElbLoadBalancerName = fmt.Sprintf("%s-%s", req.EcsClusterName, req.ServiceName)
				}

				req.ElbLoadBalancer = &elbv2.CreateLoadBalancerInput{
					// The name of the load balancer.
					// This name must be unique per region per account, can have a maximum of 32
					// characters, must contain only alphanumeric characters or hyphens, must not
					// begin or end with a hyphen, and must not begin with "internal-".
					// Name is a required field
					Name: aws.String(req.ElbLoadBalancerName),
					// [Application Load Balancers] The type of IP addresses used by the subnets
					// for your load balancer. The possible values are ipv4 (for IPv4 addresses)
					// and dualstack (for IPv4 and IPv6 addresses).
					IpAddressType: aws.String("ipv4"),
					// The nodes of an Internet-facing load balancer have public IP addresses. The
					// DNS name of an Internet-facing load balancer is publicly resolvable to the
					// public IP addresses of the nodes. Therefore, Internet-facing load balancers
					// can route requests from clients over the internet.
					// The nodes of an internal load balancer have only private IP addresses. The
					// DNS name of an internal load balancer is publicly resolvable to the private
					// IP addresses of the nodes. Therefore, internal load balancers can only route
					// requests from clients with access to the VPC for the load balancer.
					Scheme: aws.String("internet-facing"),
					// The type of load balancer.
					Type: aws.String("application"),
					// One or more tags to assign to the load balancer.
					Tags: []*elbv2.Tag{
						&elbv2.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
						&elbv2.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
					},
				}
				log.Printf("\t\t\tSet ELB Name to '%s'.", req.ElbLoadBalancerName)

				req.ElbTargetGroupName = fmt.Sprintf("%s-http", req.EcsServiceName)
				req.ElbTargetGroup = &elbv2.CreateTargetGroupInput{
					// The name of the target group.
					// This name must be unique per region per account, can have a maximum of 32
					// characters, must contain only alphanumeric characters or hyphens, and must
					// not begin or end with a hyphen.
					// Name is a required field
					Name: aws.String(req.ElbTargetGroupName),

					// The port on which the targets receive traffic. This port is used unless you
					// specify a port override when registering the target. If the target is a Lambda
					// function, this parameter does not apply.
					Port: aws.Int64(80),

					// The protocol to use for routing traffic to the targets. For Application Load
					// Balancers, the supported protocols are HTTP and HTTPS. For Network Load Balancers,
					// the supported protocols are TCP, TLS, UDP, or TCP_UDP. A TCP_UDP listener
					// must be associated with a TCP_UDP target group. If the target is a Lambda
					// function, this parameter does not apply.
					Protocol: aws.String("HTTP"),

					// Indicates whether health checks are enabled. If the target type is lambda,
					// health checks are disabled by default but can be enabled. If the target type
					// is instance or ip, health checks are always enabled and cannot be disabled.
					HealthCheckEnabled: aws.Bool(true),

					// The approximate amount of time, in seconds, between health checks of an individual
					// target. For HTTP and HTTPS health checks, the range is 5–300 seconds. For
					// TCP health checks, the supported values are 10 and 30 seconds. If the target
					// type is instance or ip, the default is 30 seconds. If the target type is
					// lambda, the default is 35 seconds.
					HealthCheckIntervalSeconds: aws.Int64(30),

					// [HTTP/HTTPS health checks] The ping path that is the destination on the targets
					// for health checks. The default is /.
					HealthCheckPath: aws.String("/ping"),

					// The protocol the load balancer uses when performing health checks on targets.
					// For Application Load Balancers, the default is HTTP. For Network Load Balancers,
					// the default is TCP. The TCP protocol is supported for health checks only
					// if the protocol of the target group is TCP, TLS, UDP, or TCP_UDP. The TLS,
					// UDP, and TCP_UDP protocols are not supported for health checks.
					HealthCheckProtocol: aws.String("HTTP"),

					// The amount of time, in seconds, during which no response from a target means
					// a failed health check. For target groups with a protocol of HTTP or HTTPS,
					// the default is 5 seconds. For target groups with a protocol of TCP or TLS,
					// this value must be 6 seconds for HTTP health checks and 10 seconds for TCP
					// and HTTPS health checks. If the target type is lambda, the default is 30
					// seconds.
					HealthCheckTimeoutSeconds: aws.Int64(5),

					// The number of consecutive health checks successes required before considering
					// an unhealthy target healthy. For target groups with a protocol of HTTP or
					// HTTPS, the default is 5. For target groups with a protocol of TCP or TLS,
					// the default is 3. If the target type is lambda, the default is 5.
					HealthyThresholdCount: aws.Int64(3),

					// The number of consecutive health check failures required before considering
					// a target unhealthy. For target groups with a protocol of HTTP or HTTPS, the
					// default is 2. For target groups with a protocol of TCP or TLS, this value
					// must be the same as the healthy threshold count. If the target type is lambda,
					// the default is 2.
					UnhealthyThresholdCount: aws.Int64(3),

					// [HTTP/HTTPS health checks] The HTTP codes to use when checking for a successful
					// response from a target.
					Matcher: &elbv2.Matcher{
						HttpCode: aws.String("200"),
					},

					// The type of target that you must specify when registering targets with this
					// target group. You can't specify targets for a target group using more than
					// one target type.
					//
					//    * instance - Targets are specified by instance ID. This is the default
					//    value. If the target group protocol is UDP or TCP_UDP, the target type
					//    must be instance.
					//
					//    * ip - Targets are specified by IP address. You can specify IP addresses
					//    from the subnets of the virtual private cloud (VPC) for the target group,
					//    the RFC 1918 range (10.0.0.0/8, 172.16.0.0/12, and 192.168.0.0/16), and
					//    the RFC 6598 range (100.64.0.0/10). You can't specify publicly routable
					//    IP addresses.
					//
					//    * lambda - The target groups contains a single Lambda function.
					TargetType: aws.String("ip"),
				}
				log.Printf("\t\t\tSet ELB Target Group Name to '%s'.", req.ElbTargetGroupName)
			}

			// Set ECS configs based on specified env.
			if flags.Env == "prod" {
				req.EcsServiceMinimumHealthyPercent = aws.Int64(100)
				req.EcsServiceMaximumPercent = aws.Int64(200)

				req.ElbDeregistrationDelay = aws.Int(300)
			} else {
				req.EcsServiceMinimumHealthyPercent = aws.Int64(100)
				req.EcsServiceMaximumPercent = aws.Int64(200)

				// force staging to deploy immediately without waiting for connections to drain
				req.ElbDeregistrationDelay = aws.Int(0)
			}
			if req.EcsServiceDesiredCount == 0 {
				req.EcsServiceDesiredCount = 1
			}

			req.EscServiceHealthCheckGracePeriodSeconds = aws.Int64(60)

			// Service Discovery Namespace settings.
			req.SDNamepsace = &servicediscovery.CreatePrivateDnsNamespaceInput{
				Name:        aws.String(req.EcsClusterName),
				Description: aws.String(fmt.Sprintf("Private DNS namespace used for services running on the ECS Cluster %s", req.EcsClusterName)),

				// A unique string that identifies the request and that allows failed CreatePrivateDnsNamespace
				// requests to be retried without the risk of executing the operation twice.
				// CreatorRequestId can be any unique string, for example, a date/time stamp.
				CreatorRequestId: aws.String("devops-deploy"),
			}

			// Service Discovery Service settings.
			req.SDService = &servicediscovery.CreateServiceInput{
				Name:        aws.String(req.EcsServiceName),
				Description: aws.String(fmt.Sprintf("Service %s running on the ECS Cluster %s", req.EcsServiceName, req.EcsClusterName)),

				// A complex type that contains information about the Amazon Route 53 records
				// that you want AWS Cloud Map to create when you register an instance.
				DnsConfig: &servicediscovery.DnsConfig{
					DnsRecords: []*servicediscovery.DnsRecord{
						{
							// The amount of time, in seconds, that you want DNS resolvers to cache the
							// settings for this record.
							TTL: aws.Int64(300),

							// The type of the resource, which indicates the type of value that Route 53
							// returns in response to DNS queries.
							Type: aws.String("A"),
						},
					},
				},

				// A complex type that contains information about an optional custom health
				// check.
				//
				// If you specify a health check configuration, you can specify either HealthCheckCustomConfig
				// or HealthCheckConfig but not both.
				HealthCheckCustomConfig: &servicediscovery.HealthCheckCustomConfig{
					// The number of 30-second intervals that you want Cloud Map to wait after receiving
					// an UpdateInstanceCustomHealthStatus request before it changes the health
					// status of a service instance. For example, suppose you specify a value of
					// 2 for FailureTheshold, and then your application sends an UpdateInstanceCustomHealthStatus
					// request. Cloud Map waits for approximately 60 seconds (2 x 30) before changing
					// the status of the service instance based on that request.
					//
					// Sending a second or subsequent UpdateInstanceCustomHealthStatus request with
					// the same value before FailureThreshold x 30 seconds has passed doesn't accelerate
					// the change. Cloud Map still waits FailureThreshold x 30 seconds after the
					// first request to make the change.
					FailureThreshold: aws.Int64(3),
				},

				// A unique string that identifies the request and that allows failed CreatePrivateDnsNamespace
				// requests to be retried without the risk of executing the operation twice.
				// CreatorRequestId can be any unique string, for example, a date/time stamp.
				CreatorRequestId: aws.String("devops-deploy"),
			}

			// Elastic Cache settings for a Redis cache cluster. Could defined different settings by env.
			req.CacheCluster = &elasticache.CreateCacheClusterInput{
				AutoMinorVersionUpgrade: aws.Bool(true),
				CacheClusterId:          aws.String(req.ProjectName + "-" + req.Env),
				CacheNodeType:           aws.String("cache.t2.micro"),
				CacheSubnetGroupName:    aws.String("default"),
				Engine:                  aws.String("redis"),
				EngineVersion:           aws.String("5.0.4"),
				NumCacheNodes:           aws.Int64(1),
				Port:                    aws.Int64(6379),
				SnapshotRetentionLimit:  aws.Int64(7),
			}

			// Recommended to be set to allkeys-lru to avoid OOM since redis will be used as an ephemeral store.
			req.CacheClusterParameter = []*elasticache.ParameterNameValue{
				&elasticache.ParameterNameValue{
					ParameterName:  aws.String("maxmemory-policy"),
					ParameterValue: aws.String("allkeys-lru"),
				},
			}

			// RDS cluster is used for Aurora which is limited to regions and db instance types so not good for example.
			req.DBCluster = nil

			// RDS settings for a Postgres database Instance. Could defined different settings by env.
			req.DBInstance = &rds.CreateDBInstanceInput{
				DBInstanceIdentifier:      aws.String(dBInstanceIdentifier(req.ProjectName, req.Env)),
				DBName:                    aws.String("shared"),
				Engine:                    aws.String("postgres"),
				MasterUsername:            aws.String("god"),
				MasterUserPassword:        aws.String(uuid.NewRandom().String()),
				Port:                      aws.Int64(5432),
				DBInstanceClass:           aws.String("db.t2.small"),
				AllocatedStorage:          aws.Int64(20),
				MultiAZ:                   aws.Bool(false),
				PubliclyAccessible:        aws.Bool(false),
				StorageEncrypted:          aws.Bool(true),
				BackupRetentionPeriod:     aws.Int64(7),
				EnablePerformanceInsights: aws.Bool(false),
				AutoMinorVersionUpgrade:   aws.Bool(true),
				CopyTagsToSnapshot:        aws.Bool(true),
				Tags: []*rds.Tag{
					{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
					{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
				},
			}

			log.Printf("\t%s\tDefaults set.", tests.Success)
		}

		log.Println("\tValidate request.")
		errs := validator.New().Struct(req)
		if errs != nil {
			return nil, errs
		}

		log.Printf("\t%s\tNew request generated.", tests.Success)
	}

	return &req, nil
}

// Run is the main entrypoint for deploying a service for a given target environment.
func ServiceDeploy(log *log.Logger, req *serviceDeployRequest) error {

	startTime := time.Now()

	// Load the AWS ECR repository. Try to find by name else create new one.
	{
		log.Println("ECR - Get or create repository.")

		svc := ecr.New(req.awsSession())

		// First try to find ECR repository by name.
		var awsRepo *ecr.Repository
		descRes, err := svc.DescribeRepositories(&ecr.DescribeRepositoriesInput{
			RepositoryNames: []*string{aws.String(req.EcrRepositoryName)},
		})
		if err != nil {
			// The repository should have been created by build or manually created and should exist at this point.
			return errors.Wrapf(err, "Failed to describe repository '%s'.", req.EcrRepositoryName)
		} else if len(descRes.Repositories) > 0 {
			awsRepo = descRes.Repositories[0]
		}
		log.Printf("\t\tFound: %s.", *awsRepo.RepositoryArn)

		req.ReleaseImage = releaseImage(req.Env, req.ServiceName, *awsRepo.RepositoryUri)
		if err != nil {
			return err
		}

		log.Printf("\t\trelease image: %s", req.ReleaseImage)
		log.Printf("\t%s\tRelease image valid.", tests.Success)
	}

	// Try to find the Datadog API key, this value is optional.
	// If Datadog API key is not specified, then integration with Datadog for observability will not be active.
	var datadogApiKey string
	{
		log.Println("Datadog - Get API Key")

		// Load Datadog API key which can be either stored in an environment variable or in AWS Secrets Manager.
		// 1. Check env vars for [DEV|STAGE|PROD]_DD_API_KEY and DD_API_KEY
		datadogApiKey = getTargetEnv(req.Env, "DD_API_KEY")

		// 2. Check AWS Secrets Manager for datadog entry prefixed with target environment.
		if datadogApiKey == "" {
			prefixedSecretId := secretID(req.ProjectName, req.Env, "datadog")
			var err error
			datadogApiKey, err = GetAwsSecretValue(req.AwsCreds, prefixedSecretId)
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
			datadogApiKey, err = GetAwsSecretValue(req.AwsCreds, secretId)
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
	}

	// Helper function to tag ECS resources.
	ec2TagResource := func(resource, name string, tags ...*ec2.Tag) error {
		svc := ec2.New(req.awsSession())

		ec2Tags := []*ec2.Tag{
			{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
			{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
			{Key: aws.String(awsTagNameName), Value: aws.String(name)},
		}

		if tags != nil {
			for _, t := range tags {
				ec2Tags = append(ec2Tags, t)
			}
		}

		_, err := svc.CreateTags(&ec2.CreateTagsInput{
			Resources: aws.StringSlice([]string{resource}),
			Tags:      ec2Tags,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to create tags for %s", resource)
		}

		return nil
	}
	_ = ec2TagResource

	// Try to find the AWS Cloudwatch Log Group by name or create new one.
	{
		log.Println("CloudWatch Logs - Get or Create Log Group")

		svc := cloudwatchlogs.New(req.awsSession())

		// If no log group was found, create one.
		var err error
		_, err = svc.CreateLogGroup(req.CloudWatchLogGroup)
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != cloudwatchlogs.ErrCodeResourceAlreadyExistsException {
				return errors.Wrapf(err, "failed to create log group '%s'", req.CloudWatchLogGroupName)
			}

			log.Printf("\t\tFound: %s.", req.CloudWatchLogGroupName)
		} else {
			log.Printf("\t\tCreated: %s.", req.CloudWatchLogGroupName)
		}

		log.Printf("\t%s\tUsing Log Group '%s'.\n", tests.Success, req.CloudWatchLogGroupName)
	}

	// Try to find the AWS S3 Buckets by names or create new ones.
	{
		log.Println("S3 - Setup Buckets")

		svc := s3.New(req.awsSession())

		// Iterate through specified S3 buckets. Try to create S3 bucket for each.
		// Create bucket function will return record and not create it if it already exists.
		log.Println("\tGet or Create S3 Buckets")
		for _, bucket := range req.S3Buckets {
			_, err := svc.CreateBucket(bucket.Input)
			if err != nil {
				if aerr, ok := err.(awserr.Error); !ok || (aerr.Code() != s3.ErrCodeBucketAlreadyExists && aerr.Code() != s3.ErrCodeBucketAlreadyOwnedByYou) {
					return errors.Wrapf(err, "failed to create s3 bucket '%s'", bucket.Name)
				}

				// If bucket found during create, returns it.
				log.Printf("\t\tFound: %s.", bucket.Name)
			} else {

				// If no bucket found during create, create new one.
				log.Printf("\t\tCreated: %s.", bucket.Name)
			}
		}

		// S3 has a delay between when one is created vs when it is available to use.
		// Thus, need to iterate through each bucket and wait until it exists.
		log.Println("\tWait for S3 Buckets to exist")
		for _, bucket := range req.S3Buckets {
			log.Printf("\t\t%s", bucket.Name)

			err := svc.WaitUntilBucketExists(&s3.HeadBucketInput{
				Bucket: aws.String(bucket.Name),
			})
			if err != nil {
				return errors.Wrapf(err, "Failed to wait for s3 bucket '%s' to exist", bucket.Name)
			}
			log.Printf("\t\t\tExists")
		}

		// Loop through each S3 bucket and configure policies.
		log.Println("\tConfiguring each S3 Bucket")
		for _, bucket := range req.S3Buckets {
			log.Printf("\t\t%s", bucket.Name)

			// Add all the defined lifecycle rules for the bucket.
			if len(bucket.LifecycleRules) > 0 {
				_, err := svc.PutBucketLifecycleConfiguration(&s3.PutBucketLifecycleConfigurationInput{
					Bucket: aws.String(bucket.Name),
					LifecycleConfiguration: &s3.BucketLifecycleConfiguration{
						Rules: bucket.LifecycleRules,
					},
				})
				if err != nil {
					return errors.Wrapf(err, "Failed to configure lifecycle rule for s3 bucket '%s'", bucket.Name)
				}

				for _, r := range bucket.LifecycleRules {
					log.Printf("\t\t\tAdded lifecycle '%s'", *r.ID)
				}
			}

			// Add all the defined CORS rules for the bucket.
			if len(bucket.CORSRules) > 0 {
				_, err := svc.PutBucketCors(&s3.PutBucketCorsInput{
					Bucket: aws.String(bucket.Name),
					CORSConfiguration: &s3.CORSConfiguration{
						CORSRules: bucket.CORSRules,
					},
				})
				if err != nil {
					return errors.Wrapf(err, "Failed to put CORS on s3 bucket '%s'", bucket.Name)
				}
				log.Printf("\t\t\tUpdated CORS")
			}

			// Block public access for all non-public buckets.
			if bucket.PublicAccessBlock != nil {
				_, err := svc.PutPublicAccessBlock(&s3.PutPublicAccessBlockInput{
					Bucket:                         aws.String(bucket.Name),
					PublicAccessBlockConfiguration: bucket.PublicAccessBlock,
				})
				if err != nil {
					return errors.Wrapf(err, "Failed to block public access for s3 bucket '%s'", bucket.Name)
				}
				log.Printf("\t\t\tBlocked public access")
			}

			// Add the bucket policy if not empty.
			if bucket.Policy != "" {
				_, err := svc.PutBucketPolicy(&s3.PutBucketPolicyInput{
					Bucket: aws.String(bucket.Name),
					Policy: aws.String(bucket.Policy),
				})
				if err != nil {
					return errors.Wrapf(err, "Failed to put bucket policy for s3 bucket '%s'", bucket.Name)
				}
				log.Printf("\t\t\tUpdated bucket policy")
			}
		}
		log.Printf("\t%s\tS3 buckets configured successfully.\n", tests.Success)
	}

	if req.CloudfrontPublic != nil {
		log.Println("Cloudfront - Setup Distribution")

		svc := cloudfront.New(req.awsSession())

		_, err := svc.CreateDistribution(&cloudfront.CreateDistributionInput{
			DistributionConfig: req.CloudfrontPublic,
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || (aerr.Code() != cloudfront.ErrCodeDistributionAlreadyExists) {
				return errors.Wrapf(err, "Failed to create cloudfront distribution '%s'", *req.CloudfrontPublic.DefaultCacheBehavior.TargetOriginId)
			}

			// If bucket found during create, returns it.
			log.Printf("\t\tFound: %s.", *req.CloudfrontPublic.DefaultCacheBehavior.TargetOriginId)
		} else {

			// If no bucket found during create, create new one.
			log.Printf("\t\tCreated: %s.", *req.CloudfrontPublic.DefaultCacheBehavior.TargetOriginId)
		}
	}

	// Find the default VPC and associated subnets.
	// Custom subnets outside of the default VPC are not currently supported.
	var projectSubnetsIDs []string
	var projectVpcId string
	{
		log.Println("EC2 - Find Subnets")

		svc := ec2.New(req.awsSession())

		log.Println("\t\tFind all subnets are that default for each availability zone.")

		// Find all subnets that are default for each availability zone.
		var subnets []*ec2.Subnet
		err := svc.DescribeSubnetsPages(&ec2.DescribeSubnetsInput{}, func(res *ec2.DescribeSubnetsOutput, lastPage bool) bool {
			for _, s := range res.Subnets {
				if *s.DefaultForAz {
					subnets = append(subnets, s)
				}
			}
			return !lastPage
		})
		if err != nil {
			return errors.Wrap(err, "Failed to find default subnets")
		}

		// This deployment process requires at least one subnet.
		// Each AWS account gets a default VPC and default subnet for each availability zone.
		// Likely error with AWs is can not find at least one.
		if len(subnets) == 0 {
			return errors.New("Failed to find any subnets, expected at least 1")
		}

		// Iterate through subnets and make sure they belong to the same VPC as the project.
		for _, s := range subnets {
			if s.VpcId == nil {
				continue
			}
			if projectVpcId == "" {
				projectVpcId = *s.VpcId
			} else if projectVpcId != *s.VpcId {
				return errors.Errorf("Invalid subnet %s, all subnets should belong to the same VPC, expected %s, got %s", *s.SubnetId, projectVpcId, *s.VpcId)
			}

			projectSubnetsIDs = append(projectSubnetsIDs, *s.SubnetId)
			log.Printf("\t\t\t%s", *s.SubnetId)
		}

		log.Printf("\t\tFound %d subnets.\n", len(subnets))
	}

	// Try to find the AWS Security Group by name or create a new one.
	var securityGroupId string
	{
		log.Println("EC2 - Find Security Group")

		svc := ec2.New(req.awsSession())

		log.Printf("\t\tFind security group '%s'.\n", req.Ec2SecurityGroupName)

		// Link the ID of the VPC.
		req.Ec2SecurityGroup.VpcId = aws.String(projectVpcId)

		// Find all the security groups and then parse the group name to get the Id of the security group.
		var runnerSgId string
		err := svc.DescribeSecurityGroupsPages(&ec2.DescribeSecurityGroupsInput{
			GroupNames: aws.StringSlice([]string{req.Ec2SecurityGroupName, req.GitlabRunnerEc2SecurityGroupName}),
		}, func(res *ec2.DescribeSecurityGroupsOutput, lastPage bool) bool {
			for _, s := range res.SecurityGroups {
				if *s.GroupName == req.Ec2SecurityGroupName {
					securityGroupId = *s.GroupId
				} else if *s.GroupName == req.GitlabRunnerEc2SecurityGroupName {
					runnerSgId = *s.GroupId
				}
			}
			return !lastPage
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != "InvalidGroup.NotFound" {
				return errors.Wrapf(err, "Failed to find security group '%s'", req.Ec2SecurityGroupName)
			}
		}

		if securityGroupId == "" {
			// If no security group was found, create one.
			createRes, err := svc.CreateSecurityGroup(req.Ec2SecurityGroup)
			if err != nil {
				return errors.Wrapf(err, "Failed to create security group '%s'", req.Ec2SecurityGroupName)
			}
			securityGroupId = *createRes.GroupId

			log.Printf("\t\tCreated: %s.", req.Ec2SecurityGroupName)
		} else {
			log.Printf("\t\tFound: %s.", req.Ec2SecurityGroupName)
		}

		ingressInputs := []*ec2.AuthorizeSecurityGroupIngressInput{
			// Enable services to be publicly available via HTTP port 80
			&ec2.AuthorizeSecurityGroupIngressInput{
				IpProtocol: aws.String("tcp"),
				CidrIp:     aws.String("0.0.0.0/0"),
				FromPort:   aws.Int64(80),
				ToPort:     aws.Int64(80),
				GroupId:    aws.String(securityGroupId),
			},
			// Allow all services in the security group to access other services.
			&ec2.AuthorizeSecurityGroupIngressInput{
				SourceSecurityGroupName: aws.String(req.Ec2SecurityGroupName),
				GroupId:                 aws.String(securityGroupId),
			},
		}

		// When not using an Elastic Load Balancer, services need to support direct access via HTTPS.
		// HTTPS is terminated via the web server and not on the Load Balancer.
		if req.EnableHTTPS {
			// Enable services to be publicly available via HTTPS port 443.
			ingressInputs = append(ingressInputs, &ec2.AuthorizeSecurityGroupIngressInput{
				IpProtocol: aws.String("tcp"),
				CidrIp:     aws.String("0.0.0.0/0"),
				FromPort:   aws.Int64(443),
				ToPort:     aws.Int64(443),
				GroupId:    aws.String(securityGroupId),
			})
		}

		// When a db instance is defined, deploy needs access to the RDS instance to handle executing schema migration.
		if req.DBInstance != nil {
			// The gitlab runner security group is required when a db instance is defined.
			if runnerSgId == "" {
				return errors.Errorf("Failed to find security group '%s'", req.GitlabRunnerEc2SecurityGroupName)
			}

			// Enable GitLab runner to communicate with deployment created services.
			ingressInputs = append(ingressInputs, &ec2.AuthorizeSecurityGroupIngressInput{
				SourceSecurityGroupName: aws.String(req.GitlabRunnerEc2SecurityGroupName),
				GroupId:                 aws.String(securityGroupId),
			})
		}

		// Add all the default ingress to the security group.
		for _, ingressInput := range ingressInputs {
			_, err = svc.AuthorizeSecurityGroupIngress(ingressInput)
			if err != nil {
				if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != "InvalidPermission.Duplicate" {
					return errors.Wrapf(err, "Failed to add ingress for security group '%s'", req.Ec2SecurityGroupName)
				}
			}
		}

		log.Printf("\t%s\tUsing Security Group '%s'.\n", tests.Success, req.Ec2SecurityGroupName)
	}

	// This is only used when service uses Aurora via RDS for serverless Postgres and database cluster is defined.
	// Aurora Postgres is limited to specific AWS regions and thus not used by default.
	// If an Aurora Postgres cluster is defined, ensure it exists with RDS else create a new one.
	var dbCluster *rds.DBCluster
	if req.DBCluster != nil {
		log.Println("RDS - Get or Create Database Cluster")

		svc := rds.New(req.awsSession())

		// Try to find a RDS database cluster using cluster identifier.
		descRes, err := svc.DescribeDBClusters(&rds.DescribeDBClustersInput{
			DBClusterIdentifier: req.DBCluster.DBClusterIdentifier,
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != rds.ErrCodeDBClusterNotFoundFault {
				return errors.Wrapf(err, "Failed to describe database cluster '%s'", *req.DBCluster.DBClusterIdentifier)
			}
		} else if len(descRes.DBClusters) > 0 {
			dbCluster = descRes.DBClusters[0]
		}

		if dbCluster == nil {
			// If no cluster was found, create one.
			createRes, err := svc.CreateDBCluster(req.DBCluster)
			if err != nil {
				return errors.Wrapf(err, "Failed to create cluster '%s'", *req.DBCluster.DBClusterIdentifier)
			}
			dbCluster = createRes.DBCluster

			log.Printf("\t\tCreated: %s", *dbCluster.DBClusterArn)
		} else {
			log.Printf("\t\tFound: %s", *dbCluster.DBClusterArn)
		}

		// The status of the cluster.
		log.Printf("\t\t\tStatus: %s", *dbCluster.Status)

		log.Printf("\t%s\tUsing DB Cluster '%s'.\n", tests.Success, *dbCluster.DatabaseName)
	}

	// Regardless if deployment is using Aurora or not, still need to setup database instance.
	// If a database instance is defined, then ensure it exists with RDS in else create a new one.
	var db *DB
	if req.DBInstance != nil {
		log.Println("RDS - Get or Create Database Instance")

		// Secret ID used to store the DB username and password across deploys.
		dbSecretId := secretID(req.ProjectName, req.Env, *req.DBInstance.DBInstanceIdentifier)

		// Retrieve the current secret value if something is stored.
		{
			sm := secretsmanager.New(req.awsSession())
			res, err := sm.GetSecretValue(&secretsmanager.GetSecretValueInput{
				SecretId: aws.String(dbSecretId),
			})
			if err != nil {
				if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != secretsmanager.ErrCodeResourceNotFoundException {
					return errors.Wrapf(err, "Failed to get value for secret id %s", dbSecretId)
				}
			} else {
				err = json.Unmarshal([]byte(*res.SecretString), &db)
				if err != nil {
					return errors.Wrap(err, "Failed to json decode db credentials")
				}
			}
		}

		// Init a new RDS client.
		svc := rds.New(req.awsSession())

		// Always set the VPC Security Group ID dynamically with the one created/found previously.
		req.DBInstance.VpcSecurityGroupIds = aws.StringSlice([]string{securityGroupId})

		// When a DB cluster exists, add the identifier to the instance input. This is for creating databases with
		// the storage engine of AWS Aurora.
		if dbCluster != nil {
			req.DBInstance.DBClusterIdentifier = dbCluster.DBClusterIdentifier
		} else {
			req.DBInstance.DBClusterIdentifier = nil
		}

		// Try to find an existing DB instance with the same identifier.
		var dbInstance *rds.DBInstance
		descRes, err := svc.DescribeDBInstances(&rds.DescribeDBInstancesInput{
			DBInstanceIdentifier: req.DBInstance.DBInstanceIdentifier,
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != rds.ErrCodeDBInstanceNotFoundFault {
				return errors.Wrapf(err, "Failed to describe database instance '%s'", *req.DBInstance.DBInstanceIdentifier)
			}
		} else if len(descRes.DBInstances) > 0 {
			dbInstance = descRes.DBInstances[0]
		}

		// No DB instance was found, so create a new one.
		if dbInstance == nil {
			if db == nil {
				// If master password is not set, pull from cluster or generate random.
				if req.DBInstance.MasterUserPassword == nil {
					if req.DBCluster.MasterUserPassword != nil && *req.DBCluster.MasterUserPassword != "" {
						req.DBInstance.MasterUserPassword = req.DBCluster.MasterUserPassword
					} else {
						req.DBInstance.MasterUserPassword = aws.String(uuid.NewRandom().String())
					}
				}

				// Only set the password right now,
				// all other configuration details will be set after the database instance is created.
				db = &DB{
					Pass: *req.DBInstance.MasterUserPassword,
				}

				// Store the secret first in the event that create fails.
				{
					// Json encode the db details to be stored as secret text.
					dat, err := json.Marshal(db)
					if err != nil {
						return errors.Wrap(err, "Failed to marshal db credentials")
					}

					// Create the new entry in AWS Secret Manager with the database password.
					sm := secretsmanager.New(req.awsSession())
					_, err = sm.CreateSecret(&secretsmanager.CreateSecretInput{
						Name:         aws.String(dbSecretId),
						SecretString: aws.String(string(dat)),
					})
					if err != nil {
						/*
							if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != secretsmanager.ErrCodeResourceExistsException {
								return errors.Wrap(err, "failed to create new secret with db credentials")
							}
							_, err = sm.UpdateSecret(&secretsmanager.UpdateSecretInput{
								SecretId:         aws.String(dbSecretId),
								SecretString: aws.String(string(dat)),
							})
							if err != nil {
								return errors.Wrap(err, "failed to update secret with db credentials")
							}*/
						return errors.Wrap(err, "Failed to create new secret with db credentials")
					}
					log.Printf("\t\tStored Secret\n")
				}
			} else {
				req.DBInstance.MasterUserPassword = aws.String(db.Pass)
			}

			// If no cluster was found, create one.
			createRes, err := svc.CreateDBInstance(req.DBInstance)
			if err != nil {
				return errors.Wrapf(err, "Failed to create instance '%s'", *req.DBInstance.DBInstanceIdentifier)
			}
			dbInstance = createRes.DBInstance

			log.Printf("\t\tCreated: %s", *dbInstance.DBInstanceArn)
		} else {
			log.Printf("\t\tFound: %s", *dbInstance.DBInstanceArn)
		}

		// The status of the instance.
		log.Printf("\t\t\tStatus: %s", *dbInstance.DBInstanceStatus)

		// If the instance is not active because it was recently created, wait for it to become active.
		if *dbInstance.DBInstanceStatus != "available" {
			log.Printf("\t\tWait for instance to become available.")
			err = svc.WaitUntilDBInstanceAvailable(&rds.DescribeDBInstancesInput{
				DBInstanceIdentifier: dbInstance.DBInstanceIdentifier,
			})
			if err != nil {
				return errors.Wrapf(err, "Failed to wait for database instance '%s' to enter available state", *req.DBInstance.DBInstanceIdentifier)
			}
		}

		// Update the secret with the DB instance details. This happens after DB create to help address when the
		// DB instance was successfully created, but the secret failed to save. The DB details host should be empty or
		// match the current instance endpoint.
		curHost := fmt.Sprintf("%s:%d", *dbInstance.Endpoint.Address, *dbInstance.Endpoint.Port)
		if curHost != db.Host {

			// Copy the instance details to the DB struct.
			db.Host = curHost
			db.User = *dbInstance.MasterUsername
			db.Database = *dbInstance.DBName
			db.Driver = *dbInstance.Engine
			db.DisableTLS = false

			// Json encode the DB details to be stored as text via AWS Secrets Manager.
			dat, err := json.Marshal(db)
			if err != nil {
				return errors.Wrap(err, "Failed to marshal db credentials")
			}

			// Update the current AWS Secret.
			sm := secretsmanager.New(req.awsSession())
			_, err = sm.UpdateSecret(&secretsmanager.UpdateSecretInput{
				SecretId:     aws.String(dbSecretId),
				SecretString: aws.String(string(dat)),
			})
			if err != nil {
				return errors.Wrap(err, "Failed to update secret with db credentials")
			}
			log.Printf("\t\tUpdate Secret\n")

			// Ensure the newly created database is seeded.
			log.Printf("\t\tOpen database connection")
			// Register informs the sqlxtrace package of the driver that we will be using in our program.
			// It uses a default service name, in the below case "postgres.db". To use a custom service
			// name use RegisterWithServiceName.
			sqltrace.Register(db.Driver, &pq.Driver{}, sqltrace.WithServiceName("devops:migrate"))
			masterDb, err := sqlxtrace.Open(db.Driver, db.URL())
			if err != nil {
				return errors.WithStack(err)
			}
			defer masterDb.Close()

			// Start the database migrations.
			log.Printf("\t\tStart migrations.")
			if err = schema.Migrate(masterDb, log); err != nil {
				return errors.WithStack(err)
			}
			log.Printf("\t\tFinished migrations.")
		}

		log.Printf("\t%s\tUsing DB Instance '%s'.\n", tests.Success, *dbInstance.DBInstanceIdentifier)
	}

	// Setup AWS Elastic Cache cluster for Redis.
	var cacheCluster *elasticache.CacheCluster
	if req.CacheCluster != nil {
		log.Println("Elastic Cache - Get or Create Cache Cluster")

		// Set the security group of the cache cluster
		req.CacheCluster.SecurityGroupIds = aws.StringSlice([]string{securityGroupId})

		svc := elasticache.New(req.awsSession())

		// Find Elastic Cache cluster given Id.
		descRes, err := svc.DescribeCacheClusters(&elasticache.DescribeCacheClustersInput{
			CacheClusterId:    req.CacheCluster.CacheClusterId,
			ShowCacheNodeInfo: aws.Bool(true),
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != elasticache.ErrCodeCacheClusterNotFoundFault {
				return errors.Wrapf(err, "Failed to describe cache cluster '%s'", *req.CacheCluster.CacheClusterId)
			}
		} else if len(descRes.CacheClusters) > 0 {
			cacheCluster = descRes.CacheClusters[0]
		}

		if cacheCluster == nil {
			// If no repository was found, create one.
			createRes, err := svc.CreateCacheCluster(req.CacheCluster)
			if err != nil {
				return errors.Wrapf(err, "failed to create cluster '%s'", *req.CacheCluster.CacheClusterId)
			}
			cacheCluster = createRes.CacheCluster

			/*
				// TODO: Tag cache cluster, ARN for the cache cluster when it is not readily available.
				_, err = svc.AddTagsToResource(&elasticache.AddTagsToResourceInput{
					ResourceName: ???,
					Tags: []*elasticache.Tag{
						{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
						{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
					},
				})
				if err != nil {
					return errors.Wrapf(err, "failed to create cluster '%s'",  *req.CacheCluster.CacheClusterId)
				}
			*/

			log.Printf("\t\tCreated: %s", *cacheCluster.CacheClusterId)
		} else {
			log.Printf("\t\tFound: %s", *cacheCluster.CacheClusterId)
		}

		// The status of the cluster.
		log.Printf("\t\t\tStatus: %s", *cacheCluster.CacheClusterStatus)

		// If the cache cluster is not active because it was recently created, wait for it to become active.
		if *cacheCluster.CacheClusterStatus != "available" {
			log.Printf("\t\tWhat for cluster to become available.")
			err = svc.WaitUntilCacheClusterAvailable(&elasticache.DescribeCacheClustersInput{
				CacheClusterId: req.CacheCluster.CacheClusterId,
			})
			if err != nil {
				return errors.Wrapf(err, "Failed to wait for cache cluster '%s' to enter available state", req.CacheCluster.CacheClusterId)
			}
		}

		// If there are custom cache group parameters set, then create a new group and keep them modified.
		if len(req.CacheClusterParameter) > 0 {
			customCacheParameterGroupName := fmt.Sprintf("%s-%s%s", strings.ToLower(req.ProjectNameCamel()), *cacheCluster.Engine, *cacheCluster.EngineVersion)
			customCacheParameterGroupName = strings.Replace(customCacheParameterGroupName, ".", "-", -1)

			// If the cache cluster is using the default parameter group, create a new custom group.
			if strings.HasPrefix(*cacheCluster.CacheParameterGroup.CacheParameterGroupName, "default") {
				// Lookup the group family from the current cache parameter group.
				descRes, err := svc.DescribeCacheParameterGroups(&elasticache.DescribeCacheParameterGroupsInput{
					CacheParameterGroupName: cacheCluster.CacheParameterGroup.CacheParameterGroupName,
				})
				if err != nil {
					if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != elasticache.ErrCodeCacheClusterNotFoundFault {
						return errors.Wrapf(err, "Failed to describe cache parameter group '%s'", *req.CacheCluster.CacheClusterId)
					}
				}

				log.Printf("\t\tCreated custom Cache Parameter Group : %s", customCacheParameterGroupName)
				_, err = svc.CreateCacheParameterGroup(&elasticache.CreateCacheParameterGroupInput{
					CacheParameterGroupFamily: descRes.CacheParameterGroups[0].CacheParameterGroupFamily,
					CacheParameterGroupName:   aws.String(customCacheParameterGroupName),
					Description:               aws.String(fmt.Sprintf("Customized default parameter group for %s %s", *cacheCluster.Engine, *cacheCluster.EngineVersion)),
				})
				if err != nil {
					return errors.Wrapf(err, "Failed to cache parameter group '%s'", customCacheParameterGroupName)
				}

				log.Printf("\t\tSet Cache Parameter Group : %s", customCacheParameterGroupName)
				updateRes, err := svc.ModifyCacheCluster(&elasticache.ModifyCacheClusterInput{
					CacheClusterId:          cacheCluster.CacheClusterId,
					CacheParameterGroupName: aws.String(customCacheParameterGroupName),
				})
				if err != nil {
					return errors.Wrapf(err, "Failed modify cache parameter group '%s' for cache cluster '%s'", customCacheParameterGroupName, *cacheCluster.CacheClusterId)
				}
				cacheCluster = updateRes.CacheCluster
			}

			// Only modify the cache parameter group if the cache cluster is custom one created to allow other groups to
			// be set on the cache cluster but not modified.
			if *cacheCluster.CacheParameterGroup.CacheParameterGroupName == customCacheParameterGroupName {
				log.Printf("\t\tUpdating Cache Parameter Group : %s", *cacheCluster.CacheParameterGroup.CacheParameterGroupName)

				_, err = svc.ModifyCacheParameterGroup(&elasticache.ModifyCacheParameterGroupInput{
					CacheParameterGroupName: cacheCluster.CacheParameterGroup.CacheParameterGroupName,
					ParameterNameValues:     req.CacheClusterParameter,
				})
				if err != nil {
					return errors.Wrapf(err, "failed to modify cache parameter group '%s'", *cacheCluster.CacheParameterGroup.CacheParameterGroupName)
				}

				for _, p := range req.CacheClusterParameter {
					log.Printf("\t\t\tSet '%s' to '%s'", *p.ParameterName, *p.ParameterValue)
				}
			}
		}

		// Ensure cache nodes are set after updating parameters.
		if len(cacheCluster.CacheNodes) == 0 {
			// Find Elastic Cache cluster given Id.
			descRes, err := svc.DescribeCacheClusters(&elasticache.DescribeCacheClustersInput{
				CacheClusterId:    req.CacheCluster.CacheClusterId,
				ShowCacheNodeInfo: aws.Bool(true),
			})
			if err != nil {
				if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != elasticache.ErrCodeCacheClusterNotFoundFault {
					return errors.Wrapf(err, "Failed to describe cache cluster '%s'", *req.CacheCluster.CacheClusterId)
				}
			} else if len(descRes.CacheClusters) > 0 {
				cacheCluster = descRes.CacheClusters[0]
			}
		}

		log.Printf("\t%s\tDone setting up Cache Cluster '%s' successfully.\n", tests.Success, *cacheCluster.CacheClusterId)
	}

	// Route 53 zone lookup when hostname is set. Supports both top level domains or sub domains.
	var zoneArecNames = map[string][]string{}
	if req.ServiceHostPrimary != "" {
		log.Println("Route 53 - Get or create hosted zones.")

		svc := route53.New(req.awsSession())

		log.Println("\tList all hosted zones.")
		var zones []*route53.HostedZone
		err := svc.ListHostedZonesPages(&route53.ListHostedZonesInput{},
			func(res *route53.ListHostedZonesOutput, lastPage bool) bool {
				for _, z := range res.HostedZones {
					zones = append(zones, z)
				}
				return !lastPage
			})
		if err != nil {
			return errors.Wrap(err, "Failed list route 53 hosted zones")
		}

		// Generate a slice with the primary domain name and include all the alternative domain names.
		lookupDomains := []string{}
		if req.ServiceHostPrimary != "" {
			lookupDomains = append(lookupDomains, req.ServiceHostPrimary)
		}
		for _, dn := range req.ServiceHostNames {
			lookupDomains = append(lookupDomains, dn)
		}

		// Loop through all the defined domain names and find the associated zone even when they are a sub domain.
		for _, dn := range lookupDomains {
			log.Printf("\t\tFind zone for domain '%s'", dn)

			// Get the top level domain from url.
			zoneName := domainutil.Domain(dn)
			var subdomain string
			if zoneName == "" {
				// Handle domain names that have weird TDL: ie .tech
				zoneName = dn
				log.Printf("\t\t\tNon-standard Level Domain: '%s'", zoneName)
			} else {
				log.Printf("\t\t\tTop Level Domain: '%s'", zoneName)

				// Check if url has subdomain.
				if domainutil.HasSubdomain(dn) {
					subdomain = domainutil.Subdomain(dn)
					log.Printf("\t\t\tsubdomain: '%s'", subdomain)
				}
			}

			// Start at the top level domain and try to find a hosted zone. Search until a match is found or there are
			// no more domain levels to search for.
			var zoneId string
			for {
				log.Printf("\t\t\tChecking zone '%s' for associated hosted zone.", zoneName)

				// Loop over each one of hosted zones and try to find match.
				for _, z := range zones {
					zn := strings.TrimRight(*z.Name, ".")

					log.Printf("\t\t\t\tChecking if '%s' matches '%s'", zn, zoneName)
					if zn == zoneName {
						zoneId = *z.Id
						break
					}
				}

				if zoneId != "" || zoneName == dn {
					// Found a matching zone or have to search all possibilities!
					break
				}

				// If we have not found a hosted zone, append the next level from the domain to the zone.
				pts := strings.Split(subdomain, ".")
				subs := []string{}
				for idx, sn := range pts {
					if idx == len(pts)-1 {
						zoneName = sn + "." + zoneName
					} else {
						subs = append(subs, sn)
					}
				}
				subdomain = strings.Join(subs, ".")
			}

			var aName string
			if zoneId == "" {

				// Get the top level domain from url again.
				zoneName := domainutil.Domain(dn)
				if zoneName == "" {
					// Handle domain names that have weird TDL: ie .tech
					zoneName = dn
				}

				log.Printf("\t\t\tNo hosted zone found for '%s', create '%s'.", dn, zoneName)
				createRes, err := svc.CreateHostedZone(&route53.CreateHostedZoneInput{
					Name: aws.String(zoneName),
					HostedZoneConfig: &route53.HostedZoneConfig{
						Comment: aws.String(fmt.Sprintf("Public hosted zone created by saas-starter-kit.")),
					},

					// A unique string that identifies the request and that allows failed CreateHostedZone
					// requests to be retried without the risk of executing the operation twice.
					// You must use a unique CallerReference string every time you submit a CreateHostedZone
					// request. CallerReference can be any unique string, for example, a date/time
					// stamp.
					//
					// CallerReference is a required field
					CallerReference: aws.String("devops-deploy"),
				})
				if err != nil {
					return errors.Wrapf(err, "Failed to create route 53 hosted zone '%s' for domain '%s'", zoneName, dn)
				}
				zoneId = *createRes.HostedZone.Id

				log.Printf("\t\t\tCreated hosted zone '%s'", zoneId)

				// The fully qualified A record name.
				aName = dn
			} else {
				log.Printf("\t\t\tFound hosted zone '%s'", zoneId)

				// The fully qualified A record name.
				if subdomain != "" {
					aName = subdomain + "." + zoneName
				} else {
					aName = zoneName
				}
			}

			// Add the A record to be maintained for the zone.
			if _, ok := zoneArecNames[zoneId]; !ok {
				zoneArecNames[zoneId] = []string{}
			}
			zoneArecNames[zoneId] = append(zoneArecNames[zoneId], aName)

			log.Printf("\t%s\tZone '%s' found with A record name '%s'.\n", tests.Success, zoneId, aName)
		}
	}

	// Setup service discovery.
	var sdService *servicediscovery.Service
	{
		log.Println("SD - Get or Create Namespace")

		svc := servicediscovery.New(req.awsSession())

		log.Println("\t\tList all the private namespaces and try to find an existing entry.")

		listNamespaces := func() (*servicediscovery.NamespaceSummary, error) {
			var found *servicediscovery.NamespaceSummary
			err := svc.ListNamespacesPages(&servicediscovery.ListNamespacesInput{
				Filters: []*servicediscovery.NamespaceFilter{
					&servicediscovery.NamespaceFilter{
						Name:      aws.String("TYPE"),
						Condition: aws.String("EQ"),
						Values:    aws.StringSlice([]string{"DNS_PRIVATE"}),
					},
				},
			}, func(res *servicediscovery.ListNamespacesOutput, lastPage bool) bool {
				for _, n := range res.Namespaces {
					if *n.Name == *req.SDNamepsace.Name {
						found = n
						return false
					}
				}
				return !lastPage
			})
			if err != nil {
				return nil, errors.Wrap(err, "Failed to list namespaces")
			}

			return found, nil
		}

		sdNamespace, err := listNamespaces()
		if err != nil {
			return err
		}

		if sdNamespace == nil {
			// Link the namespace to the VPC.
			req.SDNamepsace.Vpc = aws.String(projectVpcId)

			log.Println("\t\tCreate private namespace.")

			// If no namespace was found, create one.
			createRes, err := svc.CreatePrivateDnsNamespace(req.SDNamepsace)
			if err != nil {
				return errors.Wrapf(err, "Failed to create namespace '%s'", *req.SDNamepsace.Name)
			}
			operationId := createRes.OperationId

			log.Println("\t\tWait for create operation to finish.")
			retryFunc := func() (bool, error) {
				opRes, err := svc.GetOperation(&servicediscovery.GetOperationInput{
					OperationId: operationId,
				})
				if err != nil {
					return true, err
				}

				log.Printf("\t\t\tStatus: %s.", *opRes.Operation.Status)

				// The status of the operation. Values include the following:
				//    * SUBMITTED: This is the initial state immediately after you submit a
				//    request.
				//    * PENDING: AWS Cloud Map is performing the operation.
				//    * SUCCESS: The operation succeeded.
				//    * FAIL: The operation failed. For the failure reason, see ErrorMessage.
				if *opRes.Operation.Status == "SUCCESS" {
					return true, nil
				} else if *opRes.Operation.Status == "FAIL" {
					err = errors.Errorf("Operation failed")
					err = awserr.New(*opRes.Operation.ErrorCode, *opRes.Operation.ErrorMessage, err)
					return true, err
				}

				return false, nil
			}
			err = retry.Retry(context.Background(), nil, retryFunc)
			if err != nil {
				return errors.Wrapf(err, "Failed to get operation for namespace '%s'", *req.SDNamepsace.Name)
			}

			// Now that the create operation is complete, try to find the namespace again.
			sdNamespace, err = listNamespaces()
			if err != nil {
				return err
			}

			log.Printf("\t\tCreated: %s.", *sdNamespace.Arn)
		} else {
			log.Printf("\t\tFound: %s.", *sdNamespace.Arn)

			// The number of services that are associated with the namespace.
			if sdNamespace.ServiceCount != nil {
				log.Printf("\t\t\tServiceCount: %d.", *sdNamespace.ServiceCount)
			}
		}

		log.Printf("\t%s\tUsing Service Discovery Namespace '%s'.\n", tests.Success, *sdNamespace.Id)

		// Try to find an existing entry for the current service.
		var existingService *servicediscovery.ServiceSummary
		err = svc.ListServicesPages(&servicediscovery.ListServicesInput{
			Filters: []*servicediscovery.ServiceFilter{
				&servicediscovery.ServiceFilter{
					Name:      aws.String("NAMESPACE_ID"),
					Condition: aws.String("EQ"),
					Values:    aws.StringSlice([]string{*sdNamespace.Id}),
				},
			},
		}, func(res *servicediscovery.ListServicesOutput, lastPage bool) bool {
			for _, n := range res.Services {
				if *n.Name == req.EcsServiceName {
					existingService = n
					return false
				}
			}
			return !lastPage
		})
		if err != nil {
			return errors.Wrapf(err, "failed to list services for namespace '%s'", *sdNamespace.Id)
		}

		if existingService == nil {
			// Link the service to the namespace.
			req.SDService.NamespaceId = sdNamespace.Id

			// If no namespace was found, create one.
			createRes, err := svc.CreateService(req.SDService)
			if err != nil {
				return errors.Wrapf(err, "failed to create service '%s'", *req.SDService.Name)
			}
			sdService = createRes.Service

			log.Printf("\t\tCreated: %s.", *sdService.Arn)
		} else {

			// If no namespace was found, create one.
			getRes, err := svc.GetService(&servicediscovery.GetServiceInput{
				Id: existingService.Id,
			})
			if err != nil {
				return errors.Wrapf(err, "failed to get service '%s'", *req.SDService.Name)
			}
			sdService = getRes.Service

			log.Printf("\t\tFound: %s.", *sdService.Arn)

			// The number of instances that are currently associated with the service. Instances
			// that were previously associated with the service but that have been deleted
			// are not included in the count.
			if sdService.InstanceCount != nil {
				log.Printf("\t\t\tInstanceCount: %d.", *sdService.InstanceCount)
			}

		}

		log.Printf("\t%s\tUsing Service Discovery Service '%s'.\n", tests.Success, *sdService.Id)
	}

	// If an Elastic Load Balancer is enabled, then ensure one exists else create one.
	var ecsELBs []*ecs.LoadBalancer
	var elb *elbv2.LoadBalancer
	if req.EnableEcsElb {

		// If HTTPS enabled on ELB, then need to find ARN certificates first.
		var certificateArn string
		if req.EnableHTTPS {
			log.Println("ACM - Find Elastic Load Balance")

			svc := acm.New(req.awsSession())

			err := svc.ListCertificatesPages(&acm.ListCertificatesInput{},
				func(res *acm.ListCertificatesOutput, lastPage bool) bool {
					for _, cert := range res.CertificateSummaryList {
						if *cert.DomainName == req.ServiceHostPrimary {
							certificateArn = *cert.CertificateArn
							return false
						}
					}
					return !lastPage
				})
			if err != nil {
				return errors.Wrapf(err, "failed to list certificates for '%s'", req.ServiceHostPrimary)
			}

			if certificateArn == "" {
				// Create hash of all the domain names to be used to mark unique requests.
				idempotencyToken := req.ServiceHostPrimary + "|" + strings.Join(req.ServiceHostNames, "|")
				idempotencyToken = fmt.Sprintf("%x", md5.Sum([]byte(idempotencyToken)))

				// If no certicate was found, create one.
				createRes, err := svc.RequestCertificate(&acm.RequestCertificateInput{
					// Fully qualified domain name (FQDN), such as www.example.com, that you want
					// to secure with an ACM certificate. Use an asterisk (*) to create a wildcard
					// certificate that protects several sites in the same domain. For example,
					// *.example.com protects www.example.com, site.example.com, and images.example.com.
					//
					// The first domain name you enter cannot exceed 63 octets, including periods.
					// Each subsequent Subject Alternative Name (SAN), however, can be up to 253
					// octets in length.
					//
					// DomainName is a required field
					DomainName: aws.String(req.ServiceHostPrimary),

					// Customer chosen string that can be used to distinguish between calls to RequestCertificate.
					// Idempotency tokens time out after one hour. Therefore, if you call RequestCertificate
					// multiple times with the same idempotency token within one hour, ACM recognizes
					// that you are requesting only one certificate and will issue only one. If
					// you change the idempotency token for each call, ACM recognizes that you are
					// requesting multiple certificates.
					IdempotencyToken: aws.String(idempotencyToken),

					// Currently, you can use this parameter to specify whether to add the certificate
					// to a certificate transparency log. Certificate transparency makes it possible
					// to detect SSL/TLS certificates that have been mistakenly or maliciously issued.
					// Certificates that have not been logged typically produce an error message
					// in a browser. For more information, see Opting Out of Certificate Transparency
					// Logging (https://docs.aws.amazon.com/acm/latest/userguide/acm-bestpractices.html#best-practices-transparency).
					Options: &acm.CertificateOptions{
						CertificateTransparencyLoggingPreference: aws.String("DISABLED"),
					},

					// Additional FQDNs to be included in the Subject Alternative Name extension
					// of the ACM certificate. For example, add the name www.example.net to a certificate
					// for which the DomainName field is www.example.com if users can reach your
					// site by using either name. The maximum number of domain names that you can
					// add to an ACM certificate is 100. However, the initial limit is 10 domain
					// names. If you need more than 10 names, you must request a limit increase.
					// For more information, see Limits (https://docs.aws.amazon.com/acm/latest/userguide/acm-limits.html).
					SubjectAlternativeNames: aws.StringSlice(req.ServiceHostNames),

					// The method you want to use if you are requesting a public certificate to
					// validate that you own or control domain. You can validate with DNS (https://docs.aws.amazon.com/acm/latest/userguide/gs-acm-validate-dns.html)
					// or validate with email (https://docs.aws.amazon.com/acm/latest/userguide/gs-acm-validate-email.html).
					// We recommend that you use DNS validation.
					ValidationMethod: aws.String("DNS"),
				})
				if err != nil {
					return errors.Wrapf(err, "failed to create certificate '%s'", req.ServiceHostPrimary)
				}
				certificateArn = *createRes.CertificateArn

				log.Printf("\t\tCreated certificate '%s'", req.ServiceHostPrimary)
			} else {
				log.Printf("\t\tFound certificate '%s'", req.ServiceHostPrimary)
			}

			descRes, err := svc.DescribeCertificate(&acm.DescribeCertificateInput{
				CertificateArn: aws.String(certificateArn),
			})
			if err != nil {
				return errors.Wrapf(err, "failed to describe certificate '%s'", certificateArn)
			}
			cert := descRes.Certificate

			log.Printf("\t\t\tStatus: %s", *cert.Status)

			if *cert.Status == "PENDING_VALIDATION" {
				svc := route53.New(req.awsSession())

				log.Println("\tList all hosted zones.")

				var zoneValOpts = map[string][]*acm.DomainValidation{}
				for _, opt := range cert.DomainValidationOptions {
					var found bool
					for zoneId, aNames := range zoneArecNames {
						for _, aName := range aNames {
							fmt.Println(*opt.DomainName, " ==== ", aName)

							if *opt.DomainName == aName {
								if _, ok := zoneValOpts[zoneId]; !ok {
									zoneValOpts[zoneId] = []*acm.DomainValidation{}
								}
								zoneValOpts[zoneId] = append(zoneValOpts[zoneId], opt)
								found = true
								break
							}
						}

						if found {
							break
						}
					}

					if !found {
						return errors.Errorf("Failed to find zone ID for '%s'", *opt.DomainName)
					}
				}

				for zoneId, opts := range zoneValOpts {
					for _, opt := range opts {
						if *opt.ValidationStatus == "SUCCESS" {
							continue
						}

						input := &route53.ChangeResourceRecordSetsInput{
							ChangeBatch: &route53.ChangeBatch{
								Changes: []*route53.Change{
									&route53.Change{
										Action: aws.String("UPSERT"),
										ResourceRecordSet: &route53.ResourceRecordSet{
											Name: opt.ResourceRecord.Name,
											ResourceRecords: []*route53.ResourceRecord{
												&route53.ResourceRecord{Value: opt.ResourceRecord.Value},
											},
											Type: opt.ResourceRecord.Type,
											TTL:  aws.Int64(60),
										},
									},
								},
							},
							HostedZoneId: aws.String(zoneId),
						}

						log.Printf("\tAdded verification record for '%s'.\n", *opt.ResourceRecord.Name)
						_, err := svc.ChangeResourceRecordSets(input)
						if err != nil {
							return errors.Wrapf(err, "failed to update A records for zone '%s'", zoneId)
						}
					}
				}
			}

			log.Printf("\t%s\tUsing ACM Certicate '%s'.\n", tests.Success, certificateArn)
		}

		log.Println("EC2 - Find Elastic Load Balance")
		{
			svc := elbv2.New(req.awsSession())

			// Try to find load balancer given a name.
			err := svc.DescribeLoadBalancersPages(&elbv2.DescribeLoadBalancersInput{
				Names: []*string{aws.String(req.ElbLoadBalancerName)},
			}, func(res *elbv2.DescribeLoadBalancersOutput, lastPage bool) bool {
				// Loop through the results to find the match ELB.
				for _, lb := range res.LoadBalancers {
					if *lb.LoadBalancerName == req.ElbLoadBalancerName {
						elb = lb
						return false
					}
				}
				return !lastPage
			})
			if err != nil {
				if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != elbv2.ErrCodeLoadBalancerNotFoundException {
					return errors.Wrapf(err, "Failed to describe load balancer '%s'", req.ElbLoadBalancerName)
				}
			}

			var curListeners []*elbv2.Listener
			if elb == nil {

				// Link the security group and subnets to the Load Balancer
				req.ElbLoadBalancer.SecurityGroups = aws.StringSlice([]string{securityGroupId})
				req.ElbLoadBalancer.Subnets = aws.StringSlice(projectSubnetsIDs)

				// If no repository was found, create one.
				createRes, err := svc.CreateLoadBalancer(req.ElbLoadBalancer)
				if err != nil {
					return errors.Wrapf(err, "Failed to create load balancer '%s'", req.ElbLoadBalancerName)
				}
				elb = createRes.LoadBalancers[0]

				log.Printf("\t\tCreated: %s.", *elb.LoadBalancerArn)
			} else {
				log.Printf("\t\tFound: %s.", *elb.LoadBalancerArn)

				// Search for existing listeners associated with the load balancer.
				res, err := svc.DescribeListeners(&elbv2.DescribeListenersInput{
					// The Amazon Resource Name (ARN) of the load balancer.
					LoadBalancerArn: elb.LoadBalancerArn,
					// There are two target groups, return both associated listeners if they exist.
					PageSize: aws.Int64(2),
				})
				if err != nil {
					return errors.Wrapf(err, "Failed to find listeners for load balancer '%s'", req.ElbLoadBalancerName)
				}
				curListeners = res.Listeners
			}

			// The state code. The initial state of the load balancer is provisioning. After
			// the load balancer is fully set up and ready to route traffic, its state is
			// active. If the load balancer could not be set up, its state is failed.
			log.Printf("\t\t\tState: %s.", *elb.State.Code)

			var targetGroup *elbv2.TargetGroup
			err = svc.DescribeTargetGroupsPages(&elbv2.DescribeTargetGroupsInput{
				LoadBalancerArn: elb.LoadBalancerArn,
			}, func(res *elbv2.DescribeTargetGroupsOutput, lastPage bool) bool {
				for _, tg := range res.TargetGroups {
					if *tg.TargetGroupName == req.ElbTargetGroupName {
						targetGroup = tg
						return false
					}
				}
				return !lastPage
			})
			if err != nil {
				if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != elbv2.ErrCodeTargetGroupNotFoundException {
					return errors.Wrapf(err, "Failed to describe target group '%s'", req.ElbTargetGroupName)
				}
			}

			if targetGroup == nil {
				// The identifier of the virtual private cloud (VPC). If the target is a Lambda
				// function, this parameter does not apply.
				req.ElbTargetGroup.VpcId = aws.String(projectVpcId)

				// If no target group was found, create one.
				createRes, err := svc.CreateTargetGroup(req.ElbTargetGroup)
				if err != nil {
					return errors.Wrapf(err, "Failed to create target group '%s'", req.ElbTargetGroupName)
				}
				targetGroup = createRes.TargetGroups[0]

				log.Printf("\t\tAdded target group: %s.", *targetGroup.TargetGroupArn)
			} else {
				log.Printf("\t\tHas target group: %s.", *targetGroup.TargetGroupArn)
			}

			if req.ElbDeregistrationDelay != nil {
				// If no target group was found, create one.
				_, err = svc.ModifyTargetGroupAttributes(&elbv2.ModifyTargetGroupAttributesInput{
					TargetGroupArn: targetGroup.TargetGroupArn,
					Attributes: []*elbv2.TargetGroupAttribute{
						&elbv2.TargetGroupAttribute{
							// The name of the attribute.
							Key: aws.String("deregistration_delay.timeout_seconds"),

							// The value of the attribute.
							Value: aws.String(strconv.Itoa(*req.ElbDeregistrationDelay)),
						},
					},
				})
				if err != nil {
					return errors.Wrapf(err, "Failed to modify target group '%s' attributes", req.ElbTargetGroupName)
				}

				log.Printf("\t\t\tSet sttributes.")
			}

			listenerPorts := map[string]int64{
				"HTTP": 80,
			}
			if req.EnableHTTPS {
				listenerPorts["HTTPS"] = 443
			}

			for listenerProtocol, listenerPort := range listenerPorts {

				var foundListener bool
				for _, cl := range curListeners {
					if *cl.Port == listenerPort {
						foundListener = true
						break
					}
				}

				if !foundListener {
					listenerInput := &elbv2.CreateListenerInput{
						// The actions for the default rule. The rule must include one forward action
						// or one or more fixed-response actions.
						//
						// If the action type is forward, you specify a target group. The protocol of
						// the target group must be HTTP or HTTPS for an Application Load Balancer.
						// The protocol of the target group must be TCP, TLS, UDP, or TCP_UDP for a
						// Network Load Balancer.
						//
						// DefaultActions is a required field
						DefaultActions: []*elbv2.Action{
							&elbv2.Action{
								// The type of action. Each rule must include exactly one of the following types
								// of actions: forward, fixed-response, or redirect.
								//
								// Type is a required field
								Type: aws.String("forward"),

								// The Amazon Resource Name (ARN) of the target group. Specify only when Type
								// is forward.
								TargetGroupArn: targetGroup.TargetGroupArn,
							},
						},

						// The Amazon Resource Name (ARN) of the load balancer.
						//
						// LoadBalancerArn is a required field
						LoadBalancerArn: elb.LoadBalancerArn,

						// The port on which the load balancer is listening.
						//
						// Port is a required field
						Port: aws.Int64(listenerPort),

						// The protocol for connections from clients to the load balancer. For Application
						// Load Balancers, the supported protocols are HTTP and HTTPS. For Network Load
						// Balancers, the supported protocols are TCP, TLS, UDP, and TCP_UDP.
						//
						// Protocol is a required field
						Protocol: aws.String(listenerProtocol),
					}

					if listenerProtocol == "HTTPS" {
						listenerInput.Certificates = append(listenerInput.Certificates, &elbv2.Certificate{
							CertificateArn: aws.String(certificateArn),
						})
					}

					// If no repository was found, create one.
					createRes, err := svc.CreateListener(listenerInput)
					if err != nil {
						return errors.Wrapf(err, "Failed to create listener '%s'", req.ElbLoadBalancerName)
					}

					log.Printf("\t\t\tAdded Listener: %s.", *createRes.Listeners[0].ListenerArn)
				}
			}

			ecsELBs = append(ecsELBs, &ecs.LoadBalancer{
				// The name of the container (as it appears in a container definition) to associate
				// with the load balancer.
				ContainerName: aws.String(req.EcsServiceName),
				// The port on the container to associate with the load balancer. This port
				// must correspond to a containerPort in the service's task definition. Your
				// container instances must allow ingress traffic on the hostPort of the port
				// mapping.
				ContainerPort: targetGroup.Port,
				// The full Amazon Resource Name (ARN) of the Elastic Load Balancing target
				// group or groups associated with a service or task set.
				TargetGroupArn: targetGroup.TargetGroupArn,
			})

			{
				log.Println("Ensure Load Balancer DNS name exists for hosted zones.")
				log.Printf("\t\tDNSName: '%s'.\n", *elb.DNSName)

				svc := route53.New(req.awsSession())

				for zoneId, aNames := range zoneArecNames {
					log.Printf("\tChange zone '%s'.\n", zoneId)

					input := &route53.ChangeResourceRecordSetsInput{
						ChangeBatch: &route53.ChangeBatch{
							Changes: []*route53.Change{},
						},
						HostedZoneId: aws.String(zoneId),
					}

					// Add all the A record names with the same set of public IPs.
					for _, aName := range aNames {
						log.Printf("\t\tAdd A record for '%s'.\n", aName)

						input.ChangeBatch.Changes = append(input.ChangeBatch.Changes, &route53.Change{
							Action: aws.String("UPSERT"),
							ResourceRecordSet: &route53.ResourceRecordSet{
								Name: aws.String(aName),
								Type: aws.String("A"),
								AliasTarget: &route53.AliasTarget{
									HostedZoneId:         elb.CanonicalHostedZoneId,
									DNSName:              elb.DNSName,
									EvaluateTargetHealth: aws.Bool(true),
								},
							},
						})
					}

					log.Printf("\tUpdated '%s'.\n", zoneId)
					_, err := svc.ChangeResourceRecordSets(input)
					if err != nil {
						return errors.Wrapf(err, "Failed to update A records for zone '%s'", zoneId)
					}
				}
			}

			log.Printf("\t%s\tUsing ELB '%s'.\n", tests.Success, *elb.LoadBalancerName)
		}
	}

	// Try to find AWS ECS Cluster by name or create new one.
	var ecsCluster *ecs.Cluster
	{
		log.Println("ECS - Get or Create Cluster")

		svc := ecs.New(req.awsSession())

		descRes, err := svc.DescribeClusters(&ecs.DescribeClustersInput{
			Clusters: []*string{aws.String(req.EcsClusterName)},
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != ecs.ErrCodeClusterNotFoundException {
				return errors.Wrapf(err, "Failed to describe cluster '%s'", req.EcsClusterName)
			}
		} else if len(descRes.Clusters) > 0 {
			ecsCluster = descRes.Clusters[0]
		}

		if ecsCluster == nil {
			// If no repository was found, create one.
			createRes, err := svc.CreateCluster(req.EcsCluster)
			if err != nil {
				return errors.Wrapf(err, "Failed to create cluster '%s'", req.EcsClusterName)
			}
			ecsCluster = createRes.Cluster

			log.Printf("\t\tCreated: %s.", *ecsCluster.ClusterArn)
		} else {
			log.Printf("\t\tFound: %s.", *ecsCluster.ClusterArn)

			// The number of services that are running on the cluster in an ACTIVE state.
			// You can view these services with ListServices.
			log.Printf("\t\t\tActiveServicesCount: %d.", *ecsCluster.ActiveServicesCount)
			// The number of tasks in the cluster that are in the PENDING state.
			log.Printf("\t\t\tPendingTasksCount: %d.", *ecsCluster.PendingTasksCount)
			// The number of container instances registered into the cluster. This includes
			// container instances in both ACTIVE and DRAINING status.
			log.Printf("\t\t\tRegisteredContainerInstancesCount: %d.", *ecsCluster.RegisteredContainerInstancesCount)
			// The number of tasks in the cluster that are in the RUNNING state.
			log.Printf("\t\t\tRunningTasksCount: %d.", *ecsCluster.RunningTasksCount)
		}

		// The status of the cluster. The valid values are ACTIVE or INACTIVE. ACTIVE
		// indicates that you can register container instances with the cluster and
		// the associated instances can accept tasks.
		log.Printf("\t\t\tStatus: %s.", *ecsCluster.Status)

		log.Printf("\t%s\tUsing ECS Cluster '%s'.\n", tests.Success, *ecsCluster.ClusterName)
	}

	// Register a new ECS task.
	var taskDef *ecs.TaskDefinition
	{
		log.Println("ECS - Register task definition")

		// List of placeholders that can be used in task definition and replaced on deployment.
		placeholders := map[string]string{
			"{SERVICE}":               req.ServiceName,
			"{RELEASE_IMAGE}":         req.ReleaseImage,
			"{ECS_CLUSTER}":           req.EcsClusterName,
			"{ECS_SERVICE}":           req.EcsServiceName,
			"{AWS_REGION}":            req.AwsCreds.Region,
			"{AWS_LOGS_GROUP}":        req.CloudWatchLogGroupName,
			"{AWS_S3_BUCKET_PRIVATE}": req.S3BucketPrivateName,
			"{AWS_S3_BUCKET_PUBLIC}":  req.S3BucketPublicName,
			"{ENV}":                   req.Env,
			"{DATADOG_APIKEY}":        datadogApiKey,
			"{DATADOG_ESSENTIAL}":     "true",
			"{HTTP_HOST}":             "0.0.0.0:80",
			"{HTTPS_HOST}":            "", // Not enabled by default
			"{HTTPS_ENABLED}":         "false",

			"{APP_PROJECT}":  req.ProjectName,
			"{APP_BASE_URL}": "", // Not set by default, requires a hostname to be defined.
			"{HOST_PRIMARY}": req.ServiceHostPrimary,
			"{HOST_NAMES}":   strings.Join(req.ServiceHostNames, ","),

			"{STATIC_FILES_S3_ENABLED}":         "false",
			"{STATIC_FILES_S3_PREFIX}":          req.StaticFilesS3Prefix,
			"{STATIC_FILES_CLOUDFRONT_ENABLED}": "false",
			"{STATIC_FILES_IMG_RESIZE_ENABLED}": "false",

			"{CACHE_HOST}": "", // Not enabled by default

			"{DB_HOST}":        "",
			"{DB_USER}":        "",
			"{DB_PASS}":        "",
			"{DB_DATABASE}":    "",
			"{DB_DRIVER}":      "",
			"{DB_DISABLE_TLS}": "",

			"{ROUTE53_ZONES}":           "",
			"{ROUTE53_UPDATE_TASK_IPS}": "false",

			// Directly map GitLab CICD env variables set during deploy.
			"{CI_COMMIT_REF_NAME}":     os.Getenv("CI_COMMIT_REF_NAME"),
			"{CI_COMMIT_REF_SLUG}":     os.Getenv("CI_COMMIT_REF_SLUG"),
			"{CI_COMMIT_SHA}":          os.Getenv("CI_COMMIT_SHA"),
			"{CI_COMMIT_TAG}":          os.Getenv("CI_COMMIT_TAG"),
			"{CI_COMMIT_TITLE}":        jsonEncodeStringValue(os.Getenv("CI_COMMIT_TITLE")),
			"{CI_COMMIT_DESCRIPTION}":  jsonEncodeStringValue(os.Getenv("CI_COMMIT_DESCRIPTION")),
			"{CI_COMMIT_JOB_ID}":       os.Getenv("CI_COMMIT_JOB_ID"),
			"{CI_COMMIT_JOB_URL}":      os.Getenv("CI_COMMIT_JOB_URL"),
			"{CI_COMMIT_PIPELINE_ID}":  os.Getenv("CI_COMMIT_PIPELINE_ID"),
			"{CI_COMMIT_PIPELINE_URL}": os.Getenv("CI_COMMIT_PIPELINE_URL"),
		}

		// When the datadog API key is empty, don't force the container to be essential have have the whole task fail.
		if datadogApiKey == "" {
			placeholders["{DATADOG_ESSENTIAL}"] = "false"
		}

		// For HTTPS support.
		if req.EnableHTTPS {
			placeholders["{HTTPS_ENABLED}"] = "true"

			// When there is no Elastic Load Balancer, we need to terminate HTTPS on the app.
			if !req.EnableEcsElb {
				placeholders["{HTTPS_HOST}"] = "0.0.0.0:443"
			}
		}

		// When a domain name if defined for the service, set the App Base URL. Default to HTTPS if enabled.
		if req.ServiceHostPrimary != "" {
			var appSchema string
			if req.EnableHTTPS {
				appSchema = "https"
			} else {
				appSchema = "http"
			}

			placeholders["{APP_BASE_URL}"] = fmt.Sprintf("%s://%s/", appSchema, req.ServiceHostPrimary)
		}

		// Static files served from S3.
		if req.StaticFilesS3Enable {
			placeholders["{STATIC_FILES_S3_ENABLED}"] = "true"
		}

		// Static files served from CloudFront.
		if req.CloudfrontPublic != nil {
			placeholders["{STATIC_FILES_CLOUDFRONT_ENABLED}"] = "true"
		}

		// Support for resizing static images files to be responsive.
		if req.StaticFilesImgResizeEnable {
			placeholders["{STATIC_FILES_IMG_RESIZE_ENABLED}"] = "true"
		}

		// When db is set, update the placeholders.
		if db != nil {
			placeholders["{DB_HOST}"] = db.Host
			placeholders["{DB_USER}"] = db.User
			placeholders["{DB_PASS}"] = db.Pass
			placeholders["{DB_DATABASE}"] = db.Database
			placeholders["{DB_DRIVER}"] = db.Driver

			if db.DisableTLS {
				placeholders["{DB_DISABLE_TLS}"] = "true"
			} else {
				placeholders["{DB_DISABLE_TLS}"] = "false"
			}
		}

		// When cache cluster is set, set the host and port.
		if cacheCluster != nil {
			var cacheHost string
			if cacheCluster.ConfigurationEndpoint != nil {
				// Works for memcache.
				cacheHost = fmt.Sprintf("%s:%d", *cacheCluster.ConfigurationEndpoint.Address, *cacheCluster.ConfigurationEndpoint.Port)
			} else if len(cacheCluster.CacheNodes) > 0 {
				// Works for redis.
				cacheHost = fmt.Sprintf("%s:%d", *cacheCluster.CacheNodes[0].Endpoint.Address, *cacheCluster.CacheNodes[0].Endpoint.Port)
			} else {
				return errors.New("Unable to determine cache host from cache cluster")
			}
			placeholders["{CACHE_HOST}"] = cacheHost
		}

		// Append the Route53 Zones as an env var to be used by the service for maintaining A records when new tasks
		// are spun up or down.
		if len(zoneArecNames) > 0 {
			dat, err := json.Marshal(zoneArecNames)
			if err != nil {
				return errors.Wrapf(err, "failed to json marshal zones")
			}

			placeholders["{ROUTE53_ZONES}"] = base64.RawURLEncoding.EncodeToString(dat)

			// When no Elastic Load Balance is used, tasks need to be able to directly update the Route 53 records.
			if !req.EnableEcsElb {
				placeholders["{ROUTE53_UPDATE_TASK_IPS}"] = "true"
			}
		}

		// Loop through all the placeholders and create a list of keys to search json.
		var pks []string
		for k, _ := range placeholders {
			pks = append(pks, k)
		}

		// Generate new regular expression for finding placeholders.
		expr := "(" + strings.Join(pks, "|") + ")"
		r, err := regexp.Compile(expr)
		if err != nil {
			return err
		}

		// Read the defined json task definition.
		dat, err := EcsReadTaskDefinition(req.ServiceDir, req.Env)
		if err != nil {
			return err
		}

		// Replace placeholders used in the JSON task definition.
		{
			jsonStr := string(dat)

			matches := r.FindAllString(jsonStr, -1)

			if len(matches) > 0 {
				log.Println("\t\tUpdating placeholders.")

				replaced := make(map[string]bool)
				for _, m := range matches {
					if replaced[m] {
						continue
					}
					replaced[m] = true

					newVal := placeholders[m]
					log.Printf("\t\t\t%s -> %s", m, newVal)
					jsonStr = strings.Replace(jsonStr, m, newVal, -1)
				}
			}

			dat = []byte(jsonStr)
		}

		log.Println("\t\tParse JSON to task definition.")
		taskDefInput, err := parseTaskDefinitionInput(dat)
		if err != nil {
			return err
		}

		// If a task definition value is empty, populate it with the default value.
		if taskDefInput.Family == nil || *taskDefInput.Family == "" {
			taskDefInput.Family = &req.ServiceName
		}
		if len(taskDefInput.ContainerDefinitions) > 0 {
			if taskDefInput.ContainerDefinitions[0].Name == nil || *taskDefInput.ContainerDefinitions[0].Name == "" {
				taskDefInput.ContainerDefinitions[0].Name = &req.EcsServiceName
			}
			if taskDefInput.ContainerDefinitions[0].Image == nil || *taskDefInput.ContainerDefinitions[0].Image == "" {
				taskDefInput.ContainerDefinitions[0].Image = &req.ReleaseImage
			}
		}

		log.Printf("\t\t\tFamily: %s", *taskDefInput.Family)
		log.Printf("\t\t\tExecutionRoleArn: %s", *taskDefInput.ExecutionRoleArn)

		if taskDefInput.TaskRoleArn != nil {
			log.Printf("\t\t\tTaskRoleArn: %s", *taskDefInput.TaskRoleArn)
		}
		if taskDefInput.NetworkMode != nil {
			log.Printf("\t\t\tNetworkMode: %s", *taskDefInput.NetworkMode)
		}
		log.Printf("\t\t\tTaskDefinitions: %d", len(taskDefInput.ContainerDefinitions))

		// If memory or cpu for the task is not set, need to compute from container definitions.
		if (taskDefInput.Cpu == nil || *taskDefInput.Cpu == "") || (taskDefInput.Memory == nil || *taskDefInput.Memory == "") {
			log.Println("\t\tCompute CPU and Memory for task definition.")

			var (
				totalMemory int64
				totalCpu    int64
			)
			for _, c := range taskDefInput.ContainerDefinitions {
				if c.Memory != nil {
					totalMemory = totalMemory + *c.Memory
				} else if c.MemoryReservation != nil {
					totalMemory = totalMemory + *c.MemoryReservation
				} else {
					totalMemory = totalMemory + 1
				}
				if c.Cpu != nil {
					totalCpu = totalCpu + *c.Cpu
				} else {
					totalCpu = totalCpu + 1
				}
			}

			log.Printf("\t\t\tContainer Definitions has defined total memory %d and cpu %d", totalMemory, totalCpu)

			// The selected memory and CPU for ECS Fargate is determined by the made available by AWS.
			// For more information, reference the section "Task and CPU Memory" on this page:
			// https://docs.aws.amazon.com/AmazonECS/latest/developerguide/AWS_Fargate.html

			// If your service deployment encounters the ECS error: Invalid CPU or Memory Value Specified
			// reference this page and the values below may need to be updated accordingly.
			// https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-cpu-memory-error.html

			var (
				selectedMemory int64
				selectedCpu    int64
			)
			if totalMemory < 8192 {
				if totalMemory > 7168 {
					selectedMemory = 8192

					if totalCpu >= 2048 {
						selectedCpu = 4096
					} else if totalCpu >= 1024 {
						selectedCpu = 2048
					} else {
						selectedCpu = 1024
					}
				} else if totalMemory > 6144 {
					selectedMemory = 7168

					if totalCpu >= 2048 {
						selectedCpu = 4096
					} else if totalCpu >= 1024 {
						selectedCpu = 2048
					} else {
						selectedCpu = 1024
					}
				} else if totalMemory > 5120 || totalCpu >= 1024 {
					selectedMemory = 6144

					if totalCpu >= 2048 {
						selectedCpu = 4096
					} else if totalCpu >= 1024 {
						selectedCpu = 2048
					} else {
						selectedCpu = 1024
					}
				} else if totalMemory > 4096 {
					selectedMemory = 5120

					if totalCpu >= 512 {
						selectedCpu = 1024
					} else {
						selectedCpu = 512
					}
				} else if totalMemory > 3072 {
					selectedMemory = 4096

					if totalCpu >= 512 {
						selectedCpu = 1024
					} else {
						selectedCpu = 512
					}
				} else if totalMemory > 2048 || totalCpu >= 512 {
					selectedMemory = 3072

					if totalCpu >= 512 {
						selectedCpu = 1024
					} else {
						selectedCpu = 512
					}
				} else if totalMemory > 1024 || totalCpu >= 256 {
					selectedMemory = 2048

					if totalCpu >= 256 {
						if totalCpu >= 512 {
							selectedCpu = 1024
						} else {
							selectedCpu = 512
						}
					} else {
						selectedCpu = 256
					}
				} else if totalMemory > 512 {
					selectedMemory = 1024

					if totalCpu >= 256 {
						selectedCpu = 512
					} else {
						selectedCpu = 256
					}
				} else {
					selectedMemory = 512
					selectedCpu = 256
				}
			}
			log.Printf("\t\t\tSelected memory %d and cpu %d", selectedMemory, selectedCpu)
			taskDefInput.Memory = aws.String(strconv.Itoa(int(selectedMemory)))
			taskDefInput.Cpu = aws.String(strconv.Itoa(int(selectedCpu)))
		}

		log.Printf("\t%s\tLoaded task definition complete.\n", tests.Success)

		// The execution role is the IAM role that executes ECS actions such as pulling the image and storing the
		// application logs in cloudwatch.
		if taskDefInput.ExecutionRoleArn == nil || *taskDefInput.ExecutionRoleArn == "" {

			svc := iam.New(req.awsSession())

			// Find or create role for ExecutionRoleArn.
			{
				log.Printf("\tAppend ExecutionRoleArn to task definition input for role %s.", req.EcsExecutionRoleName)

				res, err := svc.GetRole(&iam.GetRoleInput{
					RoleName: aws.String(req.EcsExecutionRoleName),
				})
				if err != nil {
					if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != iam.ErrCodeNoSuchEntityException {
						return errors.Wrapf(err, "Failed to find task role '%s'", req.EcsExecutionRoleName)
					}
				}

				if res.Role != nil {
					taskDefInput.ExecutionRoleArn = res.Role.Arn
					log.Printf("\t\t\tFound role '%s'", *taskDefInput.ExecutionRoleArn)
				} else {
					// If no repository was found, create one.
					res, err := svc.CreateRole(req.EcsExecutionRole)
					if err != nil {
						return errors.Wrapf(err, "Failed to create task role '%s'", req.EcsExecutionRoleName)
					}
					taskDefInput.ExecutionRoleArn = res.Role.Arn
					log.Printf("\t\t\tCreated role '%s'", *taskDefInput.ExecutionRoleArn)
				}

				for _, policyArn := range req.EcsExecutionRolePolicyArns {
					_, err = svc.AttachRolePolicy(&iam.AttachRolePolicyInput{
						PolicyArn: aws.String(policyArn),
						RoleName:  aws.String(req.EcsExecutionRoleName),
					})
					if err != nil {
						return errors.Wrapf(err, "Failed to attach policy '%s' to task role '%s'", policyArn, req.EcsExecutionRoleName)
					}
					log.Printf("\t\t\t\tAttached Policy '%s'", policyArn)
				}

				log.Printf("\t%s\tExecutionRoleArn updated.\n", tests.Success)
			}
		}

		// The task role is the IAM role used by the task itself to access other AWS Services. To access services
		// like S3, SQS, etc then those permissions would need to be covered by the TaskRole.
		if taskDefInput.TaskRoleArn == nil || *taskDefInput.TaskRoleArn == "" {
			svc := iam.New(req.awsSession())

			// Find or create the default service policy.
			var policyArn string
			{
				log.Printf("\tFind default service policy %s.", req.EcsTaskPolicyName)

				var policyVersionId string
				err = svc.ListPoliciesPages(&iam.ListPoliciesInput{}, func(res *iam.ListPoliciesOutput, lastPage bool) bool {
					for _, p := range res.Policies {
						if *p.PolicyName == req.EcsTaskPolicyName {
							policyArn = *p.Arn
							policyVersionId = *p.DefaultVersionId
							return false
						}
					}

					return !lastPage
				})
				if err != nil {
					return errors.Wrap(err, "Failed to list IAM policies")
				}

				if policyArn != "" {
					log.Printf("\t\t\tFound policy '%s' versionId '%s'", policyArn, policyVersionId)

					res, err := svc.GetPolicyVersion(&iam.GetPolicyVersionInput{
						PolicyArn: aws.String(policyArn),
						VersionId: aws.String(policyVersionId),
					})
					if err != nil {
						if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != iam.ErrCodeNoSuchEntityException {
							return errors.Wrapf(err, "Failed to read policy '%s' version '%s'", req.EcsTaskPolicyName, policyVersionId)
						}
					}

					// The policy document returned in this structure is URL-encoded compliant with
					// RFC 3986 (https://tools.ietf.org/html/rfc3986). You can use a URL decoding
					// method to convert the policy back to plain JSON text.
					curJson, err := url.QueryUnescape(*res.PolicyVersion.Document)
					if err != nil {
						return errors.Wrapf(err, "Failed to url unescape policy document - %s", string(*res.PolicyVersion.Document))
					}

					// Compare policy documents and add any missing actions for each statement by matching Sid.
					var curDoc IamPolicyDocument
					err = json.Unmarshal([]byte(curJson), &curDoc)
					if err != nil {
						return errors.Wrapf(err, "Failed to json decode policy document - %s", string(curJson))
					}

					var updateDoc bool
					for _, baseStmt := range req.EcsTaskPolicyDocument.Statement {
						var found bool
						for curIdx, curStmt := range curDoc.Statement {
							if baseStmt.Sid != curStmt.Sid {
								continue
							}

							found = true

							for _, baseAction := range baseStmt.Action {
								var hasAction bool
								for _, curAction := range curStmt.Action {
									if baseAction == curAction {
										hasAction = true
										break
									}
								}

								if !hasAction {
									log.Printf("\t\t\t\tAdded new action %s for '%s'", curStmt.Sid)
									curStmt.Action = append(curStmt.Action, baseAction)
									curDoc.Statement[curIdx] = curStmt
									updateDoc = true
								}
							}
						}

						if !found {
							log.Printf("\t\t\t\tAdded new statement '%s'", baseStmt.Sid)
							curDoc.Statement = append(curDoc.Statement, baseStmt)
							updateDoc = true
						}
					}

					if updateDoc {
						dat, err := json.Marshal(curDoc)
						if err != nil {
							return errors.Wrap(err, "Failed to json encode policy document")
						}

						_, err = svc.CreatePolicyVersion(&iam.CreatePolicyVersionInput{
							PolicyArn:      aws.String(policyArn),
							PolicyDocument: aws.String(string(dat)),
							SetAsDefault:   aws.Bool(true),
						})
						if err != nil {
							if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != iam.ErrCodeNoSuchEntityException {
								return errors.Wrapf(err, "Failed to read policy '%s' version '%s'", req.EcsTaskPolicyName, policyVersionId)
							}
						}
					}
				} else {
					dat, err := json.Marshal(req.EcsTaskPolicyDocument)
					if err != nil {
						return errors.Wrap(err, "Failed to json encode policy document")
					}
					req.EcsTaskPolicy.PolicyDocument = aws.String(string(dat))

					// If no repository was found, create one.
					res, err := svc.CreatePolicy(req.EcsTaskPolicy)
					if err != nil {
						return errors.Wrapf(err, "Failed to create task policy '%s'", req.EcsTaskPolicyName)
					}

					policyArn = *res.Policy.Arn

					log.Printf("\t\t\tCreated policy '%s'", policyArn)
				}

				log.Printf("\t%s\tConfigured default service policy.\n", tests.Success)
			}

			// Find or create role for TaskRoleArn.
			{
				log.Printf("\tAppend TaskRoleArn to task definition input for role %s.", req.EcsTaskRoleName)

				res, err := svc.GetRole(&iam.GetRoleInput{
					RoleName: aws.String(req.EcsTaskRoleName),
				})
				if err != nil {
					if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != iam.ErrCodeNoSuchEntityException {
						return errors.Wrapf(err, "Failed to find task role '%s'", req.EcsTaskRoleName)
					}
				}

				if res.Role != nil {
					taskDefInput.TaskRoleArn = res.Role.Arn
					log.Printf("\t\t\tFound role '%s'", *taskDefInput.TaskRoleArn)
				} else {
					// If no repository was found, create one.
					res, err := svc.CreateRole(req.EcsTaskRole)
					if err != nil {
						return errors.Wrapf(err, "Failed to create task role '%s'", req.EcsTaskRoleName)
					}
					taskDefInput.TaskRoleArn = res.Role.Arn
					log.Printf("\t\t\tCreated role '%s'", *taskDefInput.TaskRoleArn)

					//_, err = svc.UpdateAssumeRolePolicy(&iam.UpdateAssumeRolePolicyInput{
					//	PolicyDocument: ,
					//	RoleName:       aws.String(roleName),
					//})
					//if err != nil {
					//	return errors.Wrapf(err, "failed to create task role '%s'", roleName)
					//}
				}

				_, err = svc.AttachRolePolicy(&iam.AttachRolePolicyInput{
					PolicyArn: aws.String(policyArn),
					RoleName:  aws.String(req.EcsTaskRoleName),
				})
				if err != nil {
					return errors.Wrapf(err, "Failed to attach policy '%s' to task role '%s'", policyArn, req.EcsTaskRoleName)
				}

				log.Printf("\t%s\tTaskRoleArn updated.\n", tests.Success)
			}
		}

		log.Println("\tRegister new task definition.")
		{
			svc := ecs.New(req.awsSession())

			// Registers a new task.
			res, err := svc.RegisterTaskDefinition(taskDefInput)
			if err != nil {
				return errors.Wrapf(err, "Failed to register task definition '%s'", *taskDefInput.Family)
			}
			taskDef = res.TaskDefinition

			log.Printf("\t\tRegistered: %s.", *taskDef.TaskDefinitionArn)
			log.Printf("\t\t\tRevision: %d.", *taskDef.Revision)
			log.Printf("\t\t\tStatus: %s.", *taskDef.Status)

			log.Printf("\t%s\tTask definition registered.\n", tests.Success)
		}
	}

	// Try to find AWS ECS Service by name. This does not error on not found, but results are used to determine if
	// the full creation process of a service needs to be executed.
	var ecsService *ecs.Service
	{
		log.Println("ECS - Find Service")

		svc := ecs.New(req.awsSession())

		// Find service by ECS cluster and service name.
		res, err := svc.DescribeServices(&ecs.DescribeServicesInput{
			Cluster:  ecsCluster.ClusterArn,
			Services: []*string{aws.String(req.EcsServiceName)},
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != ecs.ErrCodeServiceNotFoundException {
				return errors.Wrapf(err, "Failed to describe service '%s'", req.EcsServiceName)
			}
		} else if len(res.Services) > 0 {
			ecsService = res.Services[0]

			log.Printf("\t\tFound: %s.", *ecsService.ServiceArn)

			// The desired number of instantiations of the task definition to keep running
			// on the service. This value is specified when the service is created with
			// CreateService, and it can be modified with UpdateService.
			log.Printf("\t\t\tDesiredCount: %d.", *ecsService.DesiredCount)
			// The number of tasks in the cluster that are in the PENDING state.
			log.Printf("\t\t\tPendingCount: %d.", *ecsService.PendingCount)
			// The number of tasks in the cluster that are in the RUNNING state.
			log.Printf("\t\t\tRunningCount: %d.", *ecsService.RunningCount)

			// The status of the service. The valid values are ACTIVE, DRAINING, or INACTIVE.
			log.Printf("\t\t\tStatus: %s.", *ecsService.Status)

			log.Printf("\t%s\tUsing ECS Service '%s'.\n", tests.Success, *ecsService.ServiceName)
		} else {
			log.Printf("\t%s\tExisting ECS Service not found.\n", tests.Success)
		}
	}

	// Check to see if the service should be re-created instead of updated.
	if ecsService != nil {
		var (
			recreateService bool
			forceDelete     bool
		)

		if req.RecreateService {
			// Flag was included to force recreate.
			recreateService = true
			forceDelete = true
		} else if req.EnableEcsElb && (ecsService.LoadBalancers == nil || len(ecsService.LoadBalancers) == 0) {
			// Service was created without ELB and now ELB is enabled.
			recreateService = true
		} else if !req.EnableEcsElb && (ecsService.LoadBalancers != nil && len(ecsService.LoadBalancers) > 0) {
			// Service was created with ELB and now ELB is disabled.
			recreateService = true
		} else if sdService != nil && sdService.Arn != nil && (ecsService.ServiceRegistries == nil || len(ecsService.ServiceRegistries) == 0) {
			// Service was created without Service Discovery and now Service Discovery is enabled.
			recreateService = true
		} else if (sdService == nil || sdService.Arn == nil) && (ecsService.ServiceRegistries != nil && len(ecsService.ServiceRegistries) > 0) {
			// Service was created with Service Discovery and now Service Discovery is disabled.
			recreateService = true
		}

		// If determined from above that service needs to be recreated.
		if recreateService {

			// Needs to delete any associated services on ECS first before it can be recreated.
			log.Println("ECS - Delete Service")

			svc := ecs.New(req.awsSession())

			// The service cannot be stopped while it is scaled above 0.
			if ecsService.DesiredCount != nil && *ecsService.DesiredCount > 0 {
				log.Println("\t\tScaling service down to zero.")
				_, err := svc.UpdateService(&ecs.UpdateServiceInput{
					Cluster:      ecsService.ClusterArn,
					Service:      ecsService.ServiceArn,
					DesiredCount: aws.Int64(int64(0)),
				})
				if err != nil {
					return errors.Wrapf(err, "Failed to update service '%s'", ecsService.ServiceName)
				}

				// It may take some time for the service to scale down, so need to wait.
				log.Println("\t\tWait for the service to scale down.")
				err = svc.WaitUntilServicesStable(&ecs.DescribeServicesInput{
					Cluster:  ecsCluster.ClusterArn,
					Services: aws.StringSlice([]string{*ecsService.ServiceArn}),
				})
				if err != nil {
					return errors.Wrapf(err, "Failed to wait for service '%s' to enter stable state", *ecsService.ServiceName)
				}
			}

			// Once task count is 0 for the service, then can delete it.
			log.Println("\t\tDelete Service.")
			res, err := svc.DeleteService(&ecs.DeleteServiceInput{
				Cluster: ecsService.ClusterArn,
				Service: ecsService.ServiceArn,

				// If true, allows you to delete a service even if it has not been scaled down
				// to zero tasks. It is only necessary to use this if the service is using the
				// REPLICA scheduling strategy.
				Force: aws.Bool(forceDelete),
			})
			if err != nil {
				return errors.Wrapf(err, "Failed to delete service '%s'", ecsService.ServiceName)
			}
			ecsService = res.Service

			log.Println("\t\tWait for the service to be deleted.")
			err = svc.WaitUntilServicesInactive(&ecs.DescribeServicesInput{
				Cluster:  ecsCluster.ClusterArn,
				Services: aws.StringSlice([]string{*ecsService.ServiceArn}),
			})
			if err != nil {
				return errors.Wrapf(err, "Failed to wait for service '%s' to enter stable state", *ecsService.ServiceName)
			}

			// Manually mark the ECS has inactive since WaitUntilServicesInactive was executed.
			ecsService.Status = aws.String("INACTIVE")

			log.Printf("\t%s\tDelete Service.\n", tests.Success)
		}
	}

	// If the service exists on ECS, update the service, else create a new service.
	if ecsService != nil && *ecsService.Status != "INACTIVE" {
		log.Println("ECS - Update Service")

		svc := ecs.New(req.awsSession())

		var desiredCount int64
		if req.EcsServiceDesiredCount > 0 {
			desiredCount = req.EcsServiceDesiredCount
		} else {
			// Maintain the current count set on the existing service.
			desiredCount := *ecsService.DesiredCount

			// If the desired count is zero because it was spun down for termination of staging env, update to launch
			// with at least once task running for the service.
			if desiredCount == 0 {
				desiredCount = 1
			}
		}

		updateRes, err := svc.UpdateService(&ecs.UpdateServiceInput{
			Cluster:                       ecsCluster.ClusterName,
			Service:                       ecsService.ServiceName,
			DesiredCount:                  aws.Int64(desiredCount),
			HealthCheckGracePeriodSeconds: ecsService.HealthCheckGracePeriodSeconds,
			TaskDefinition:                taskDef.TaskDefinitionArn,

			// Whether to force a new deployment of the service. Deployments are not forced
			// by default. You can use this option to trigger a new deployment with no service
			// definition changes. For example, you can update a service's tasks to use
			// a newer Docker image with the same image/tag combination (my_image:latest)
			// or to roll Fargate tasks onto a newer platform version.
			ForceNewDeployment: aws.Bool(false),
		})
		if err != nil {
			return errors.Wrapf(err, "Failed to update service '%s'", *ecsService.ServiceName)
		}
		ecsService = updateRes.Service

		log.Printf("\t%s\tUpdated ECS Service '%s'.\n", tests.Success, *ecsService.ServiceName)
	} else {

		// If not service exists on ECS, then create it.
		log.Println("ECS - Create Service")
		{

			svc := ecs.New(req.awsSession())

			var assignPublicIp *string
			var healthCheckGracePeriodSeconds *int64
			if len(ecsELBs) == 0 {
				assignPublicIp = aws.String("ENABLED")
			} else {
				assignPublicIp = aws.String("DISABLED")
				healthCheckGracePeriodSeconds = req.EscServiceHealthCheckGracePeriodSeconds
			}

			// When ELB is enabled and get the following error when using the default VPC.
			// 	Status reason 	CannotPullContainerError:
			// 		Error response from daemon:
			// 			Get https://888955683113.dkr.ecr.us-west-2.amazonaws.com/v2/:
			// 				net/http: request canceled while waiting for connection
			// 				(Client.Timeout exceeded while awaiting headers)
			assignPublicIp = aws.String("ENABLED")

			serviceInput := &ecs.CreateServiceInput{
				// The short name or full Amazon Resource Name (ARN) of the cluster that your
				// service is running on. If you do not specify a cluster, the default cluster
				// is assumed.
				Cluster: ecsCluster.ClusterName,

				// The name of your service. Up to 255 letters (uppercase and lowercase), numbers,
				// and hyphens are allowed. Service names must be unique within a cluster, but
				// you can have similarly named services in multiple clusters within a Region
				// or across multiple Regions.
				//
				// ServiceName is a required field
				ServiceName: aws.String(req.EcsServiceName),

				// Optional deployment parameters that control how many tasks run during the
				// deployment and the ordering of stopping and starting tasks.
				DeploymentConfiguration: &ecs.DeploymentConfiguration{
					// Refer to documentation for flags.ecsServiceMaximumPercent
					MaximumPercent: req.EcsServiceMaximumPercent,
					// Refer to documentation for flags.ecsServiceMinimumHealthyPercent
					MinimumHealthyPercent: req.EcsServiceMinimumHealthyPercent,
				},

				// Refer to documentation for flags.ecsServiceDesiredCount.
				DesiredCount: aws.Int64(req.EcsServiceDesiredCount),

				// Specifies whether to enable Amazon ECS managed tags for the tasks within
				// the service. For more information, see Tagging Your Amazon ECS Resources
				// (https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-using-tags.html)
				// in the Amazon Elastic Container Service Developer Guide.
				EnableECSManagedTags: aws.Bool(false),

				// The period of time, in seconds, that the Amazon ECS service scheduler should
				// ignore unhealthy Elastic Load Balancing target health checks after a task
				// has first started. This is only valid if your service is configured to use
				// a load balancer. If your service's tasks take a while to start and respond
				// to Elastic Load Balancing health checks, you can specify a health check grace
				// period of up to 2,147,483,647 seconds. During that time, the ECS service
				// scheduler ignores health check status. This grace period can prevent the
				// ECS service scheduler from marking tasks as unhealthy and stopping them before
				// they have time to come up.
				HealthCheckGracePeriodSeconds: healthCheckGracePeriodSeconds,

				// The launch type on which to run your service. For more information, see Amazon
				// ECS Launch Types (https://docs.aws.amazon.com/AmazonECS/latest/developerguide/launch_types.html)
				// in the Amazon Elastic Container Service Developer Guide.
				LaunchType: aws.String("FARGATE"),

				// A load balancer object representing the load balancer to use with your service.
				LoadBalancers: ecsELBs,

				// The network configuration for the service. This parameter is required for
				// task definitions that use the awsvpc network mode to receive their own elastic
				// network interface, and it is not supported for other network modes. For more
				// information, see Task Networking (https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-networking.html)
				// in the Amazon Elastic Container Service Developer Guide.
				NetworkConfiguration: &ecs.NetworkConfiguration{
					AwsvpcConfiguration: &ecs.AwsVpcConfiguration{
						// Whether the task's elastic network interface receives a public IP address.
						// The default value is DISABLED.
						AssignPublicIp: assignPublicIp,

						// The security groups associated with the task or service. If you do not specify
						// a security group, the default security group for the VPC is used. There is
						// a limit of 5 security groups that can be specified per AwsVpcConfiguration.
						// All specified security groups must be from the same VPC.
						SecurityGroups: aws.StringSlice([]string{securityGroupId}),

						// The subnets associated with the task or service. There is a limit of 16 subnets
						// that can be specified per AwsVpcConfiguration.
						// All specified subnets must be from the same VPC.
						// Subnets is a required field
						Subnets: aws.StringSlice(projectSubnetsIDs),
					},
				},

				// The family and revision (family:revision) or full ARN of the task definition
				// to run in your service. If a revision is not specified, the latest ACTIVE
				// revision is used. If you modify the task definition with UpdateService, Amazon
				// ECS spawns a task with the new version of the task definition and then stops
				// an old task after the new version is running.
				TaskDefinition: taskDef.TaskDefinitionArn,

				// The metadata that you apply to the service to help you categorize and organize
				// them. Each tag consists of a key and an optional value, both of which you
				// define. When a service is deleted, the tags are deleted as well. Tag keys
				// can have a maximum character length of 128 characters, and tag values can
				// have a maximum length of 256 characters.
				Tags: []*ecs.Tag{
					&ecs.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
					&ecs.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
				},
			}

			// Add the Service Discovery registry to the ECS service.
			if sdService != nil {
				if serviceInput.ServiceRegistries == nil {
					serviceInput.ServiceRegistries = []*ecs.ServiceRegistry{}
				}
				serviceInput.ServiceRegistries = append(serviceInput.ServiceRegistries, &ecs.ServiceRegistry{
					RegistryArn: sdService.Arn,
				})
			}

			createRes, err := svc.CreateService(serviceInput)

			// If tags aren't enabled for the account, try the request again without them.
			// https://aws.amazon.com/blogs/compute/migrating-your-amazon-ecs-deployment-to-the-new-arn-and-resource-id-format-2/
			if err != nil && strings.Contains(err.Error(), "ARN and resource ID format must be enabled") {
				serviceInput.Tags = nil
				createRes, err = svc.CreateService(serviceInput)
			}

			if err != nil {
				return errors.Wrapf(err, "Failed to create service '%s'", req.EcsServiceName)
			}
			ecsService = createRes.Service

			log.Printf("\t%s\tCreated ECS Service '%s'.\n", tests.Success, *ecsService.ServiceName)
		}
	}

	// When static files are enabled to be to stored on S3, we need to upload all of them.
	if req.StaticFilesS3Enable {
		log.Println("\tUpload static files to public S3 bucket")

		staticDir := filepath.Join(req.ServiceDir, "static")

		err := SyncPublicS3Files(req.awsSession(), req.S3BucketPublicName, req.StaticFilesS3Prefix, staticDir)
		if err != nil {
			return errors.Wrapf(err, "Failed to sync static files from %s to s3://%s/%s", staticDir, req.S3BucketPublicName, req.StaticFilesS3Prefix)
		}

		log.Printf("\t%s\tFiles uploaded to s3://%s/%s.\n", tests.Success, req.S3BucketPublicName, req.StaticFilesS3Prefix)
	}

	// Wait for the updated or created service to enter a stable state.
	{
		log.Println("\tWaiting for service to enter stable state.")

		// Helper method to get the logs from cloudwatch for a specific task ID.
		getTaskLogs := func(taskId string) ([]string, error) {
			if req.S3BucketPrivateName == "" {
				// No private S3 bucket defined so unable to export logs streams.
				return []string{}, nil
			}

			// Stream name generated by ECS for the awslogs driver.
			logStreamName := fmt.Sprintf("ecs/%s/%s", *ecsService.ServiceName, taskId)

			// Define S3 key prefix used to export the stream logs to.
			s3KeyPrefix := filepath.Join(req.S3BucketTempPrefix, "logs/cloudwatchlogs/exports", req.CloudWatchLogGroupName)

			var downloadPrefix string
			{
				svc := cloudwatchlogs.New(req.awsSession())

				createRes, err := svc.CreateExportTask(&cloudwatchlogs.CreateExportTaskInput{
					LogGroupName:        aws.String(req.CloudWatchLogGroupName),
					LogStreamNamePrefix: aws.String(logStreamName),
					//TaskName: aws.String(taskId),
					Destination:       aws.String(req.S3BucketPrivateName),
					DestinationPrefix: aws.String(s3KeyPrefix),
					From:              aws.Int64(startTime.UTC().AddDate(0, 0, -1).UnixNano() / int64(time.Millisecond)),
					To:                aws.Int64(time.Now().UTC().AddDate(0, 0, 1).UnixNano() / int64(time.Millisecond)),
				})
				if err != nil {
					return []string{}, errors.Wrapf(err, "Failed to create export task for from log group '%s' with stream name prefix '%s'", req.CloudWatchLogGroupName, logStreamName)
				}
				exportTaskId := *createRes.TaskId

				for {
					descRes, err := svc.DescribeExportTasks(&cloudwatchlogs.DescribeExportTasksInput{
						TaskId: aws.String(exportTaskId),
					})
					if err != nil {
						return []string{}, errors.Wrapf(err, "Failed to describe export task '%s' for from log group '%s' with stream name prefix '%s'", exportTaskId, req.CloudWatchLogGroupName, logStreamName)
					}
					taskStatus := *descRes.ExportTasks[0].Status.Code

					if taskStatus == "COMPLETED" {
						downloadPrefix = filepath.Join(s3KeyPrefix, exportTaskId) + "/"
						break
					} else if taskStatus == "CANCELLED" || taskStatus == "FAILED" {
						break
					}
					time.Sleep(time.Second * 5)
				}
			}

			// If downloadPrefix is set, then get logs from corresponding file for service.
			var logLines []string
			if downloadPrefix != "" {
				svc := s3.New(req.awsSession())

				var s3Keys []string
				err := svc.ListObjectsPages(&s3.ListObjectsInput{
					Bucket: aws.String(req.S3BucketPrivateName),
					Prefix: aws.String(downloadPrefix),
				},
					func(res *s3.ListObjectsOutput, lastPage bool) bool {
						for _, obj := range res.Contents {
							s3Keys = append(s3Keys, *obj.Key)
						}
						return !lastPage
					})
				if err != nil {
					return []string{}, errors.Wrapf(err, "Failed to list objects from s3 bucket '%s' with prefix '%s'", req.S3BucketPrivateName, downloadPrefix)
				}

				// Iterate trough S3 keys and get logs from file.
				for _, s3Key := range s3Keys {
					res, err := svc.GetObject(&s3.GetObjectInput{
						Bucket: aws.String(req.S3BucketPrivateName),
						Key:    aws.String(s3Key),
					})
					if err != nil {
						return []string{}, errors.Wrapf(err, "Failed to get object '%s' from s3 bucket", s3Key, req.S3BucketPrivateName)
					}
					r, _ := gzip.NewReader(res.Body)
					dat, err := ioutil.ReadAll(r)
					res.Body.Close()
					if err != nil {
						return []string{}, errors.Wrapf(err, "failed to read object '%s' from s3 bucket", s3Key, req.S3BucketPrivateName)
					}

					// Iterate through file by line break and add each line to array of logs.
					for _, l := range strings.Split(string(dat), "\n") {
						l = strings.TrimSpace(l)
						if l == "" {
							continue
						}
						logLines = append(logLines, l)
					}
				}
			}

			return logLines, nil
		}

		// Helper method to display tasks errors that failed to start while we wait for the service to stable state.
		taskLogLines := make(map[string][]string)
		checkTasks := func() (bool, error) {
			svc := ecs.New(req.awsSession())

			serviceTaskRes, err := svc.ListTasks(&ecs.ListTasksInput{
				Cluster:       aws.String(req.EcsClusterName),
				ServiceName:   aws.String(req.EcsServiceName),
				DesiredStatus: aws.String("STOPPED"),
			})
			if err != nil {
				return false, errors.Wrapf(err, "Failed to list tasks for cluster '%s' service '%s'", req.EcsClusterName, req.EcsServiceName)
			}

			if len(serviceTaskRes.TaskArns) == 0 {
				return false, nil
			}

			taskRes, err := svc.DescribeTasks(&ecs.DescribeTasksInput{
				Cluster: aws.String(req.EcsClusterName),
				Tasks:   serviceTaskRes.TaskArns,
			})
			if err != nil {
				return false, errors.Wrapf(err, "Failed to describe %d tasks for cluster '%s'", len(serviceTaskRes.TaskArns), req.EcsClusterName)
			}

			var failures []*ecs.Failure
			var stoppedCnt int64
			for _, t := range taskRes.Tasks {
				if *t.TaskDefinitionArn != *taskDef.TaskDefinitionArn || t.TaskArn == nil {
					continue
				}
				stoppedCnt = stoppedCnt + 1

				taskId := filepath.Base(*t.TaskArn)

				log.Printf("\t\t\tTask %s stopped\n", *t.TaskArn)
				for _, tc := range t.Containers {
					if tc.ExitCode != nil && tc.Reason != nil {
						log.Printf("\t\t\tContainer %s exited with %d - %s.\n", *tc.Name, *tc.ExitCode, *tc.Reason)
					} else if tc.ExitCode != nil {
						log.Printf("\t\t\tContainer %s exited with %d.\n", *tc.Name, *tc.ExitCode)
					} else {
						log.Printf("\t\t\tContainer %s exited.\n", *tc.Name)
					}
				}

				// Avoid exporting the logs multiple times.
				logLines, ok := taskLogLines[taskId]
				if !ok {
					logLines, err = getTaskLogs(taskId)
					if err != nil {
						return false, errors.Wrapf(err, "Failed to get logs for task %s for cluster '%s'", *t.TaskArn, req.EcsClusterName)
					}
					taskLogLines[taskId] = logLines
				}

				if len(logLines) > 0 {
					log.Printf("\t\t\tTask Logs:\n")
					for _, l := range logLines {
						log.Printf("\t\t\t\t%s\n", l)
					}
				}

				if t.StopCode != nil && t.StoppedReason != nil {
					log.Printf("\t%s\tTask %s stopped with %s - %s.\n", tests.Failed, *t.TaskArn, *t.StopCode, *t.StoppedReason)
				} else if t.StopCode != nil {
					log.Printf("\t%s\tTask %s stopped with %s.\n", tests.Failed, *t.TaskArn, *t.StopCode)
				} else {
					log.Printf("\t%s\tTask %s stopped.\n", tests.Failed, *t.TaskArn)
				}

				// Limit failures to only the current task definition.
				for _, f := range taskRes.Failures {
					if *f.Arn == *t.TaskArn {
						failures = append(failures, f)
					}
				}
			}

			if len(failures) > 0 {
				for _, t := range failures {
					log.Printf("\t%s\tTask %s failed with %s.\n", tests.Failed, *t.Arn, *t.Reason)
				}
			}

			// If the number of stopped tasks with the current task def match the desired count for the service,
			// then we no longer need to continue to check the status of the tasks.
			if stoppedCnt == *ecsService.DesiredCount {
				return true, nil
			}

			return false, nil
		}

		// New wait group with only a count of one, this will allow the first go worker to exit to cancel both.
		checkErr := make(chan error, 1)

		// Check the status of the service tasks and print out info for debugging.
		ticker := time.NewTicker(10 * time.Second)
		go func() {
			for {
				select {
				case <-ticker.C:
					stop, err := checkTasks()
					if err != nil {
						log.Printf("\t%s\tFailed to check tasks.\n%+v\n", tests.Failed, err)
					}

					if stop {
						checkErr <- errors.New("All tasks for service are stopped")
						return
					}
				}
			}
		}()

		// Use the AWS ECS method to check for the service to be stable.
		go func() {
			svc := ecs.New(req.awsSession())
			err := svc.WaitUntilServicesStable(&ecs.DescribeServicesInput{
				Cluster:  ecsCluster.ClusterArn,
				Services: aws.StringSlice([]string{*ecsService.ServiceArn}),
			})
			if err != nil {
				checkErr <- errors.Wrapf(err, "Failed to wait for service '%s' to enter stable state", *ecsService.ServiceName)
			} else {
				// All done.
				checkErr <- nil
			}
		}()

		if err := <-checkErr; err != nil {
			log.Printf("\t%s\tFailed to check tasks.\n%+v\n", tests.Failed, err)
			return err
		}

		// Wait for one of the methods to finish and then ensure the ticker is stopped.
		ticker.Stop()

		log.Printf("\t%s\tService running.\n", tests.Success)
	}

	return nil
}
