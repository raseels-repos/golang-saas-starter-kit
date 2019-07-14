package deploy

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/servicediscovery"
	"github.com/iancoleman/strcase"
	"github.com/urfave/cli"
)

// ServiceDeployFlags defines the flags used for executing a service deployment.
type ServiceDeployFlags struct {
	// Required flags.
	ServiceName string `validate:"required" example:"web-api"`
	Env         string `validate:"oneof=dev stage prod" example:"dev"`

	// Optional flags.
	EnableHTTPS         bool            `validate:"omitempty" example:"false"`
	ServiceHostPrimary  string          `validate:"omitempty" example:"example-project.com"`
	ServiceHostNames    cli.StringSlice `validate:"omitempty" example:"subdomain.example-project.com"`
	S3BucketPrivateName string          `validate:"omitempty" example:"saas-example-project-private"`
	S3BucketPublicName  string          `validate:"omitempty" example:"saas-example-project-public"`

	ProjectRoot     string `validate:"omitempty" example:"."`
	ProjectName     string ` validate:"omitempty" example:"example-project"`
	DockerFile      string `validate:"omitempty" example:"./cmd/web-api/Dockerfile"`
	EnableLambdaVPC bool   `validate:"omitempty" example:"false"`
	EnableEcsElb    bool   `validate:"omitempty" example:"false"`
	NoBuild         bool   `validate:"omitempty" example:"false"`
	NoDeploy        bool   `validate:"omitempty" example:"false"`
	NoCache         bool   `validate:"omitempty" example:"false"`
	NoPush          bool   `validate:"omitempty" example:"false"`
	RecreateService bool   `validate:"omitempty" example:"false"`
}

// serviceDeployRequest defines the details needed to execute a service deployment.
type serviceDeployRequest struct {
	ServiceName string `validate:"required"`
	ServiceDir  string `validate:"required"`
	Env         string `validate:"oneof=dev stage prod"`
	ProjectRoot string `validate:"required"`
	ProjectName string `validate:"required"`
	DockerFile  string `validate:"required"`
	GoModFile   string `validate:"required"`
	GoModName   string `validate:"required"`

	EnableHTTPS        bool     `validate:"omitempty"`
	ServiceHostPrimary string   `validate:"omitempty,required_with=EnableHTTPS,fqdn"`
	ServiceHostNames   []string `validate:"omitempty,dive,fqdn"`

	AwsCreds awsCredentials `validate:"required,dive,required"`

	EcrRepositoryName      string `validate:"required"`
	EcrRepository          *ecr.CreateRepositoryInput
	EcrRepositoryMaxImages int `validate:"omitempty"`

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

	CloudWatchLogGroupName string `validate:"required"`
	CloudWatchLogGroup     *cloudwatchlogs.CreateLogGroupInput

	S3BucketTempPrefix  string `validate:"required_with=S3BucketPrivateName S3BucketPublicName"`
	S3BucketPrivateName string `validate:"omitempty"`
	S3BucketPublicName  string `validate:"omitempty"`
	S3Buckets           []S3Bucket

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
	NoBuild         bool `validate:"omitempty"`
	NoDeploy        bool `validate:"omitempty"`
	NoCache         bool `validate:"omitempty"`
	NoPush          bool `validate:"omitempty"`
	RecreateService bool `validate:"omitempty"`

	SDNamepsace *servicediscovery.CreatePrivateDnsNamespaceInput
	SDService   *servicediscovery.CreateServiceInput

	CacheCluster          *elasticache.CreateCacheClusterInput
	CacheClusterParameter []*elasticache.ParameterNameValue

	DBCluster  *rds.CreateDBClusterInput
	DBInstance *rds.CreateDBInstanceInput

	ReleaseImage string
	BuildTags    []string
	flags        ServiceDeployFlags
	_awsSession  *session.Session
}

type S3Bucket struct {
	Name              string `validate:"omitempty"`
	Input             *s3.CreateBucketInput
	LifecycleRules    []*s3.LifecycleRule
	CORSRules         []*s3.CORSRule
	PublicAccessBlock *s3.PublicAccessBlockConfiguration
	Policy            string
}

// DB mimics the general info needed for services used to define placeholders.
type DB struct {
	Host       string
	User       string
	Pass       string
	Database   string
	Driver     string
	DisableTLS bool
}

// projectNameCamel takes a project name and returns the camel cased version.
func (r *serviceDeployRequest) ProjectNameCamel() string {
	s := strings.Replace(r.ProjectName, "_", " ", -1)
	s = strings.Replace(s, "-", " ", -1)
	s = strcase.ToCamel(s)
	return s
}

// awsSession returns the current AWS session for the serviceDeployRequest.
func (r *serviceDeployRequest) awsSession() *session.Session {
	if r._awsSession == nil {
		r._awsSession = r.AwsCreds.Session()
	}

	return r._awsSession
}

// AwsCredentials defines AWS credentials used for deployment. Unable to use roles when deploying
// using gitlab CI/CD pipeline.
type awsCredentials struct {
	AccessKeyID     string `validate:"required"`
	SecretAccessKey string `validate:"required"`
	Region          string `validate:"required"`
}

// Session returns a new AWS Session used to access AWS services.
func (creds awsCredentials) Session() *session.Session {
	return session.New(
		&aws.Config{
			Region:      aws.String(creds.Region),
			Credentials: credentials.NewStaticCredentials(creds.AccessKeyID, creds.SecretAccessKey, ""),
		})
}

// IamPolicyDocument defines an AWS IAM policy used for defining access for IAM roles, users, and groups.
type IamPolicyDocument struct {
	Version   string              `json:"Version"`
	Statement []IamStatementEntry `json:"Statement"`
}

// IamStatementEntry defines a single statement for an IAM policy.
type IamStatementEntry struct {
	Sid      string      `json:"Sid"`
	Effect   string      `json:"Effect"`
	Action   []string    `json:"Action"`
	Resource interface{} `json:"Resource"`
}
