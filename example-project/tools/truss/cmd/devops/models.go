package devops

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/iancoleman/strcase"
	"github.com/urfave/cli"
)

// ServiceDeployFlags defines the flags used for executing a service deployment.
type ServiceDeployFlags struct {
	// Required flags.
	ServiceName string `validate:"required" example:"web-api"`
	Env         string `validate:"oneof=dev stage prod" example:"dev"`

	// Optional flags.
	EnableHTTPS              bool            `validate:"omitempty" example:"false"`
	ServiceDomainName        string          `validate:"omitempty" example:"example-project.com"`
	ServiceDomainNameAliases cli.StringSlice `validate:"omitempty" example:"subdomain.example-project.com"`
	ProjectRoot              string          `validate:"omitempty" example:"."`
	ProjectName              string          ` validate:"omitempty" example:"example-project"`
	DockerFile               string          `validate:"omitempty" example:"./cmd/web-api/Dockerfile"`
	EnableLambdaVPC          bool            `validate:"omitempty" example:"false"`
	EnableEcsElb             bool            `validate:"omitempty" example:"false"`
	NoBuild                  bool            `validate:"omitempty" example:"false"`
	NoDeploy                 bool            `validate:"omitempty" example:"false"`
	NoCache                  bool            `validate:"omitempty" example:"false"`
	NoPush                   bool            `validate:"omitempty" example:"false"`
	RecreateService          bool            `validate:"omitempty" example:"false"`
}

// serviceDeployRequest defines the details needed to execute a service deployment.
type serviceDeployRequest struct {
	// Required flags.
	ServiceName            string         `validate:"required"`
	ServiceDir             string         `validate:"required"`
	Env                    string         `validate:"oneof=dev stage prod"`
	ProjectRoot            string         `validate:"required"`
	ProjectName            string         `validate:"required"`
	DockerFile             string         `validate:"required"`
	GoModFile              string         `validate:"required"`
	GoModName              string         `validate:"required"`
	EcrRepositoryName      string         `validate:"required"`
	EcsClusterName         string         `validate:"required"`
	EcsServiceName         string         `validate:"required"`
	EcsExecutionRoleName   string         `validate:"required"`
	EcsTaskRoleName        string         `validate:"required"`
	EcsTaskPolicyName      string         `validate:"required"`
	EcsServiceDesiredCount int64          `validate:"required"`
	Ec2SecurityGroupName   string         `validate:"required"`
	CloudWatchLogGroupName string         `validate:"required"`
	AwsCreds               awsCredentials `validate:"required,dive,required"`

	// Optional flags.
	EnableHTTPS                             bool     `validate:"omitempty"`
	ServiceDomainName                       string   `validate:"omitempty,required_with=EnableHTTPS,fqdn"`
	ServiceDomainNameAliases                []string `validate:"omitempty,dive,fqdn"`
	EcrRepositoryMaxImages                  int      `validate:"omitempty"`
	EcsServiceMinimumHealthyPercent         *int64   `validate:"omitempty"`
	EcsServiceMaximumPercent                *int64   `validate:"omitempty"`
	EscServiceHealthCheckGracePeriodSeconds *int64   `validate:"omitempty"`
	ElbDeregistrationDelay                  *int     `validate:"omitempty"`
	EnableLambdaVPC                         bool     `validate:"omitempty"`
	EnableEcsElb                            bool     `validate:"omitempty"`
	ElbLoadBalancerName                     string   `validate:"omitempty"`
	NoBuild                                 bool     `validate:"omitempty"`
	NoDeploy                                bool     `validate:"omitempty"`
	NoCache                                 bool     `validate:"omitempty"`
	NoPush                                  bool     `validate:"omitempty"`
	RecreateService                         bool     `validate:"omitempty"`

	ReleaseImage string
	BuildTags    []string
	flags        ServiceDeployFlags
	_awsSession  *session.Session
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
