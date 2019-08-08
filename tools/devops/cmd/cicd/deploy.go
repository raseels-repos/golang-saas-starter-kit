package cicd

import "github.com/aws/aws-sdk-go/service/ec2"

// deployRequest defines the details needed to execute a service deployment.
type deployRequest struct {
	*serviceRequest

	EcrRepositoryName string `validate:"required"`

	Ec2SecurityGroupName string `validate:"required"`
	Ec2SecurityGroup     *ec2.CreateSecurityGroupInput

	GitlabRunnerEc2SecurityGroupName string `validate:"required"`

	S3BucketTempPrefix      string `validate:"required_with=S3BucketPrivateName S3BucketPublicName"`
	S3BucketPrivateName     string `validate:"omitempty"`
	S3Buckets               []S3Bucket


	EcsService *deployEcsServiceRequest
	LambdaFunction *deployLambdaFuncRequest
}

