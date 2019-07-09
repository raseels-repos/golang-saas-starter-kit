package devops

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/iancoleman/strcase"
)

// ServiceDeployFlags defines the flags used for executing a service deployment.
type ServiceDeployFlags struct {
	// Required flags.
	ServiceName string  `validate:"required" example:"web-api"`
	Env string `validate:"oneof=dev stage prod" example:"dev"`

	// Optional flags.
	ProjectRoot  string `validate:"omitempty" example:"."`
	ProjectName string ` validate:"omitempty" example:"example-project"`
	DockerFile string `validate:"omitempty" example:"./cmd/web-api/Dockerfile"`
	EnableLambdaVPC bool `validate:"omitempty" example:"false"`
	EnableEcsElb bool  `validate:"omitempty" example:"false"`
	NoBuild  bool  `validate:"omitempty" example:"false"`
	NoDeploy bool  `validate:"omitempty" example:"false"`
	NoCache bool `validate:"omitempty" example:"false"`
	NoPush bool `validate:"omitempty" example:"false"`
}

// serviceDeployRequest defines the details needed to execute a service deployment.
type serviceDeployRequest struct {
	// Required flags.
	serviceName string  `validate:"required"`
	serviceDir string `validate:"required"`
	env string `validate:"oneof=dev stage prod"`
	projectRoot  string  `validate:"required"`
	projectName string `validate:"required"`
	dockerFile string  `validate:"required"`
	goModFile string  `validate:"required"`
	goModName string  `validate:"required"`
	ecrRepositoryName string  `validate:"required"`
	ecsClusterName string   `validate:"required"`
	ecsServiceName string   `validate:"required"`
	ecsExecutionRoleArn string  `validate:"required"`
	ecsTaskRoleArn string  `validate:"required"`
	ecsServiceDesiredCount int64  `validate:"required"`

	ec2SecurityGroupName string  `validate:"required"`
	elbLoadBalancerName string  `validate:"required"`
	cloudWatchLogGroupName string   `validate:"required"`
	releaseImage string   `validate:"required"`
	buildTags []string   `validate:"required"`
	awsCreds *awsCredentials  `validate:"required,dive"`

	// Optional flags.
	ecrRepositoryMaxImages int `validate:"omitempty"`
	ecsServiceMinimumHealthyPercent  *int64 `validate:"omitempty"`
	ecsServiceMaximumPercent *int64 `validate:"omitempty"`
	escServiceHealthCheckGracePeriodSeconds *int64 `validate:"omitempty"`
	elbDeregistrationDelay *int  `validate:"omitempty"`
	enableLambdaVPC bool `validate:"omitempty"`
	enableEcsElb bool  `validate:"omitempty"`
	noBuild  bool  `validate:"omitempty"`
	noDeploy bool  `validate:"omitempty"`
	noCache bool `validate:"omitempty"`
	noPush bool `validate:"omitempty"`

	_awsSession *session.Session
}

// projectNameCamel takes a project name and returns the camel cased version.
func (r *serviceDeployRequest) projectNameCamel() string {
	s := strings.Replace(r.projectName, "_", " ", -1)
	s = strings.Replace(s, "-", " ", -1)
	s =  strcase.ToCamel(s)
	return s
}

// awsSession returns the current AWS session for the serviceDeployRequest.
func (r *serviceDeployRequest) awsSession() *session.Session {
	if r._awsSession == nil {
		r._awsSession = r.awsCreds.Session()
	}

	return r._awsSession
}

// AwsCredentials defines AWS credentials used for deployment. Unable to use roles when deploying
// using gitlab CI/CD pipeline.
type awsCredentials struct {
	accessKeyID string `validate:"required"`
	secretAccessKey string `validate:"required"`
	region string `validate:"required"`
}

// Session returns a new AWS Session used to access AWS services.
func (creds awsCredentials) Session() *session.Session {
	return session.New(
		&aws.Config{
			Region: aws.String(creds.region),
			Credentials: credentials.NewStaticCredentials(creds.accessKeyID, creds.secretAccessKey, ""),
		})
}

// IamPolicyDocument defines an AWS IAM policy used for defining access for IAM roles, users, and groups.
type IamPolicyDocument struct {
	Version   string  `json:"Version"`
	Statement []IamStatementEntry   `json:"Statement"`
}

// IamStatementEntry defines a single statement for an IAM policy.
type IamStatementEntry struct {
	Sid string `json:"Sid"`
	Effect   string `json:"Effect"`
	Action   []string  `json:"Action"`
	Resource interface{}  `json:"Resource"`
}
