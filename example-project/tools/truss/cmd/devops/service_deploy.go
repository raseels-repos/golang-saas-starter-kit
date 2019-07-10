package devops

import (
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/bobesa/go-domain-util/domainutil"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	dockerTypes "github.com/docker/docker/api/types"
	dockerClient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// baseServicePolicyDocument defines the default permissions required to access AWS services for all deployed services.
var baseServicePolicyDocument = IamPolicyDocument{
	Version: "2012-10-17",
	Statement: []IamStatementEntry{
		IamStatementEntry{
			Sid:    "DefaultServiceAccess",
			Effect: "Allow",
			Action: []string{
				"s3:HeadBucket",
				"ec2:DescribeNetworkInterfaces",
				"ec2:DeleteNetworkInterface",
				"ecs:ListTasks",
				"ecs:DescribeTasks",
				"ec2:DescribeNetworkInterfaces",
				"route53:ListHostedZones",
				"route53:ListResourceRecordSets",
				"route53:ChangeResourceRecordSets",
				"ecs:UpdateService",
				"ses:SendEmail",
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

/*
// requiredCmdsBuild proves a list of required executables for completing build.
var requiredCmdsDeploy = [][]string{
	[]string{"docker", "version", "-f", "{{.Client.Version}}"},
}
*/

// NewServiceDeployRequest generated a new request for executing deploy for a given set of flags.
func NewServiceDeployRequest(log *log.Logger, flags ServiceDeployFlags) (*serviceDeployRequest, error) {

	log.Println("Validate flags.")
	{
		errs := validator.New().Struct(flags)
		if errs != nil {
			return nil, errs
		}
		log.Printf("\t%s\tFlags ok.", tests.Success)
	}

	log.Println("\tVerify AWS credentials.")
	var awsCreds awsCredentials
	{
		var err error
		awsCreds, err = GetAwsCredentials(flags.Env)
		if err != nil {
			return nil, err
		}
		log.Printf("\t\t\tAccessKeyID: '%s'", awsCreds.AccessKeyID)
		log.Printf("\t\t\tRegion: '%s'", awsCreds.Region)
		log.Printf("\t%s\tAWS credentials valid.", tests.Success)
	}

	log.Println("Generate deploy request.")
	var req serviceDeployRequest
	{
		req = serviceDeployRequest{
			// Required flags.
			ServiceName: flags.ServiceName,
			Env:         flags.Env,
			AwsCreds:    awsCreds,

			// Optional flags.
			ProjectRoot:     flags.ProjectRoot,
			ProjectName:     flags.ProjectName,
			DockerFile:      flags.DockerFile,
			EnableHTTPS:      flags.EnableHTTPS,
			ServiceDomainName:      flags.ServiceDomainName,
			ServiceDomainNameAliases:      flags.ServiceDomainNameAliases,
			S3BucketPrivateName :      flags.S3BucketPrivateName,
			S3BucketPublicName:      flags.S3BucketPublicName,
			EnableLambdaVPC: flags.EnableLambdaVPC,
			EnableEcsElb:    flags.EnableEcsElb,
			NoBuild:         flags.NoBuild,
			NoDeploy:        flags.NoDeploy,
			NoCache:         flags.NoCache,
			NoPush:          flags.NoPush,
			RecreateService: flags.RecreateService,

			flags: flags,
		}

		// When project root directory is empty or set to current working path, then search for the project root by locating
		// the go.mod file.
		log.Println("\tDetermining the project root directory.")
		{
			if flags.ProjectRoot == "" || flags.ProjectRoot == "." {
				log.Println("\tAttempting to location project root directory from current working directory.")

				var err error
				req.GoModFile, err = findProjectGoModFile()
				if err != nil {
					return nil, err
				}
				req.ProjectRoot = filepath.Dir(req.GoModFile)
			} else {
				log.Println("\t\tUsing supplied project root directory.")
				req.GoModFile = filepath.Join(flags.ProjectRoot, "go.mod")
			}
			log.Printf("\t\t\tproject root: %s", req.ProjectRoot)
			log.Printf("\t\t\tgo.mod: %s", req.GoModFile)
		}

		log.Println("\tExtracting go module name from go.mod.")
		{
			var err error
			req.GoModName, err = loadGoModName(req.GoModFile)
			if err != nil {
				return nil, err
			}
			log.Printf("\t\t\tmodule name: %s", req.GoModName)
		}

		log.Println("\tDetermining the project name.")
		{
			if flags.ProjectName != "" {
				req.ProjectName = flags.ProjectName
				log.Printf("\t\tUse provided value.")
			} else {
				req.ProjectName = filepath.Base(req.GoModName)
				log.Printf("\t\tSet from go module.")
			}
			log.Printf("\t\t\tproject name: %s", req.ProjectName)
		}

		log.Println("\tAttempting to locate service directory from project root directory.")
		{
			if flags.DockerFile != "" {
				req.DockerFile = flags.DockerFile
				log.Printf("\t\tUse provided value.")

			} else {
				log.Printf("\t\tFind from project root looking for Dockerfile.")
				var err error
				req.DockerFile, err = findServiceDockerFile(req.ProjectRoot, req.ServiceName)
				if err != nil {
					return nil, err
				}
			}

			req.ServiceDir = filepath.Dir(req.DockerFile)

			log.Printf("\t\t\tservice directory: %s", req.ServiceDir)
			log.Printf("\t\t\tdockerfile: %s", req.DockerFile)
		}

		log.Println("\tSet defaults not defined in env vars.")
		{
			// Set default AWS ECR Repository Name.
			req.EcrRepositoryName = req.ProjectName
			log.Printf("\t\t\tSet ECR Repository Name to '%s'.", req.EcrRepositoryName)

			// Set default AWS ECR Regsistry Max Images.
			req.EcrRepositoryMaxImages = defaultAwsRegistryMaxImages
			log.Printf("\t\t\tSet ECR Regsistry Max Images to '%d'.", req.EcrRepositoryMaxImages)

			// Set default AWS ECS Cluster Name.
			req.EcsClusterName = req.ProjectName + "-" + req.Env
			log.Printf("\t\t\tSet ECS Cluster Name to '%s'.", req.EcsClusterName)

			// Set default AWS ECS Service Name.
			req.EcsServiceName = req.ServiceName + "-" + req.Env
			log.Printf("\t\t\tSet ECS Service Name to '%s'.", req.EcsServiceName)

			// Set default AWS ECS Execution Role Name.
			req.EcsExecutionRoleName = fmt.Sprintf("ecsExecutionRole%s%s", req.ProjectNameCamel(), strcase.ToCamel(req.Env))
			log.Printf("\t\t\tSet ECS Execution Role Name to '%s'.", req.EcsExecutionRoleName)

			// Set default AWS ECS Task Role Name.
			req.EcsTaskRoleName = fmt.Sprintf("ecsTaskRole%s%s", req.ProjectNameCamel(), strcase.ToCamel(req.Env))
			log.Printf("\t\t\tSet ECS Task Role Name to '%s'.", req.EcsTaskRoleName)

			// Set default AWS ECS Task Policy Name.
			req.EcsTaskPolicyName = fmt.Sprintf("%s%sServices", req.ProjectNameCamel(), strcase.ToCamel(req.Env))
			log.Printf("\t\t\tSet ECS Task Policy Name to '%s'.", req.EcsTaskPolicyName)

			// Set default Cloudwatch Log Group Name.
			req.CloudWatchLogGroupName = fmt.Sprintf("logs/env_%s/aws/ecs/cluster_%s/service_%s", req.Env, req.EcsClusterName, req.ServiceName)
			log.Printf("\t\t\tSet CloudWatch Log Group Name to '%s'.", req.CloudWatchLogGroupName)

			// Set default EC2 Security Group Name.
			req.Ec2SecurityGroupName = req.EcsClusterName
			log.Printf("\t\t\tSet ECS Security Group Name to '%s'.", req.Ec2SecurityGroupName)

			// Set default ELB Load Balancer Name when ELB is enabled.
			if req.EnableEcsElb {
				if !strings.Contains(req.EcsClusterName, req.Env) && !strings.Contains(req.ServiceName, req.Env) {
					// When a custom cluster name is provided and/or service name, ensure the ELB contains the current env.
					req.ElbLoadBalancerName = fmt.Sprintf("%s-%s-%s", req.EcsClusterName, req.ServiceName, req.Env)
				} else {
					// Default value when when custom cluster/service name is supplied.
					req.ElbLoadBalancerName = fmt.Sprintf("%s-%s", req.EcsClusterName, req.ServiceName)
				}
				log.Printf("\t\t\tSet ELB Name to '%s'.", req.ElbLoadBalancerName)
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
			req.EcsServiceDesiredCount = 1
			req.EscServiceHealthCheckGracePeriodSeconds = aws.Int64(60)

			// S3 temp prefix, a life cycle policy will be applied to this.
			req.S3BucketTempPrefix = "tmp/"

			// Elastic Cache settings for a Redis cache cluster. Could defined different settings by env.
			req.CacheCluster = &elasticache.CreateCacheClusterInput{
				AutoMinorVersionUpgrade:   aws.Bool(true),
				CacheClusterId:            aws.String(req.ProjectName+"-"+req.Env),
				CacheNodeType:             aws.String("cache.t2.micro"),
				CacheSubnetGroupName:      aws.String("default"),
				Engine:                    aws.String("redis"),
				EngineVersion:             aws.String("5.0.4"),
				NumCacheNodes:             aws.Int64(1),
				Port:                      aws.Int64(6379),
				SnapshotRetentionLimit:    aws.Int64(7),
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

// Run is the main entrypoint for deploying a service for a given target env.
func ServiceDeploy(log *log.Logger, req *serviceDeployRequest) error {

	/*
		log.Println("Verify required commands are installed.")
		for _, cmdVals := range requiredCmdsDeploy {
			cmd := exec.Command(cmdVals[0], cmdVals[1:]...)
			cmd.Env = os.Environ()

			out, err := cmd.CombinedOutput()
			if err != nil {
				return errors.WithMessagef(err, "failed to execute %s - %s\n%s", strings.Join(cmdVals, " "), string(out))
			}

			log.Printf("\t%s\t%s - %s", tests.Success, cmdVals[0], string(out))
		}

		// Pull the current env variables to be passed in for command execution.
		envVars := EnvVars(os.Environ())

	*/

	startTime := time.Now()

	// Load the ECR repository.
	log.Println("ECR - Get or create repository.")
	var docker *dockerClient.Client
	var registryAuth string
	{
		svc := ecr.New(req.awsSession())

		var awsRepo *ecr.Repository
		descRes, err := svc.DescribeRepositories(&ecr.DescribeRepositoriesInput{
			RepositoryNames: []*string{aws.String(req.EcrRepositoryName)},
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != ecr.ErrCodeRepositoryNotFoundException {
				return errors.Wrapf(err, "failed to describe repository '%s'", req.EcrRepositoryName)
			}
		} else if len(descRes.Repositories) > 0 {
			awsRepo = descRes.Repositories[0]
		}

		if awsRepo == nil {
			// If no repository was found, create one.
			createRes, err := svc.CreateRepository(&ecr.CreateRepositoryInput{
				RepositoryName: aws.String(req.EcrRepositoryName),
				Tags: []*ecr.Tag{
					&ecr.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
					&ecr.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
				},
			})
			if err != nil {
				return errors.Wrapf(err, "failed to create repository '%s'", req.EcrRepositoryName)
			}
			awsRepo = createRes.Repository
			log.Printf("\t\tCreated: %s.", *awsRepo.RepositoryArn)
		} else {
			log.Printf("\t\tFound: %s.", *awsRepo.RepositoryArn)

			log.Println("\t\tChecking old ECR images.")
			delIds, err := EcrPurgeImages(req)
			if err != nil {
				return err
			}

			// If there are image IDs to delete, delete them.
			if len(delIds) > 0 {
				log.Printf("\t\tDeleted %d images that exceeded limit of %d", len(delIds), req.EcrRepositoryMaxImages)
				for _, imgId := range delIds {
					log.Printf("\t\t\t%s", *imgId.ImageTag)
				}
			}
		}

		tag1 := req.Env + "-" + req.ServiceName
		req.BuildTags = append(req.BuildTags, tag1)

		if v := os.Getenv("CI_COMMIT_REF_NAME"); v != "" {
			tag2 := tag1 + "-" + v
			req.BuildTags = append(req.BuildTags, tag2)
			req.ReleaseImage = *awsRepo.RepositoryUri + ":" + tag2
		} else {
			req.ReleaseImage = *awsRepo.RepositoryUri + ":" + tag1
		}

		log.Printf("\t\trelease image: %s", req.ReleaseImage)

		log.Printf("\t\ttags: %s", strings.Join(req.BuildTags, " "))
		log.Printf("\t%s\tRelease image valid.", tests.Success)

		log.Println("ECR - Retrieve authorization token used for docker login.")

		// Get the credentials necessary for logging into the AWS Elastic Container Registry
		// made available with the AWS access key and AWS secret access keys.
		res, err := svc.GetAuthorizationToken(&ecr.GetAuthorizationTokenInput{})
		if err != nil {
			return errors.Wrap(err, "failed to get ecr authorization token")
		}

		authToken, err := base64.StdEncoding.DecodeString(*res.AuthorizationData[0].AuthorizationToken)
		if err != nil {
			return errors.Wrap(err, "failed to base64 decode ecr authorization token")
		}
		pts := strings.Split(string(authToken), ":")
		user := pts[0]
		pass := pts[1]

		docker, err = dockerClient.NewEnvClient()
		if err != nil {
			return errors.WithMessage(err, "failed to init new docker client from env")
		}

		loginRes, err := docker.RegistryLogin(context.Background(), dockerTypes.AuthConfig{

			Username:      user,
			Password:      pass,
			ServerAddress: *res.AuthorizationData[0].ProxyEndpoint,
		})
		if err != nil {
			return errors.WithMessage(err, "failed docker registry login")
		}
		log.Printf("\t\tStatus: %s", loginRes.Status )


		registryAuth = fmt.Sprintf(`{"Username": "%s", "Password": "%s"}`, user, pass)
		registryAuth = base64.StdEncoding.EncodeToString([]byte(registryAuth))

		log.Printf("\t%s\tdocker login ok.", tests.Success)
	}

	// Do the docker build.
	if req.NoBuild == false {
		dockerFile, err := filepath.Rel(req.ProjectRoot, req.DockerFile)
		if err != nil {
			return errors.Wrapf(err, "failed parse relative path for %s from %s", req.DockerFile, req.ProjectRoot)
		}

		buildOpts := dockerTypes.ImageBuildOptions{
			Tags: []string{req.ReleaseImage},
			BuildArgs: map[string]*string{
				"service": &req.ServiceName,
				"env":     &req.Env,
			},
			Dockerfile: dockerFile,
			NoCache:    req.NoCache,
		}

		// Append the build tags.
		var builtImageTags []string
		for _, t := range req.BuildTags {
			if strings.HasSuffix(req.ReleaseImage, ":"+t) {
				// skip duplicate image tags
				continue
			}

			imageTag := req.ReleaseImage + ":" + t
			buildOpts.Tags = append(buildOpts.Tags, imageTag)
			builtImageTags = append(builtImageTags, imageTag)
		}

		log.Println("starting docker build")

		buildCtx, err := archive.TarWithOptions(req.ProjectRoot, &archive.TarOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to create docker build context")
		}

		_, err = docker.ImageBuild(context.Background(), buildCtx, buildOpts)
		if err != nil {
			return errors.Wrap(err, "failed to build docker image")
		}

		// Push the newly built docker container to the registry.
		if req.NoPush == false {
			log.Printf("\t\tpush release image %s", req.ReleaseImage)

			// Common Errors:
			// 1. Image push failed Error parsing HTTP response: unexpected end of JSON input: ""
			// 		If you are trying to push to an ECR repository, you need to make sure that the
			// 		ecr:BatchCheckLayerAvailability permission is also checked. It is not selected by
			// 		default when using the default push permissions that ECR sets.
			// 		https://github.com/moby/moby/issues/19010

			pushOpts := dockerTypes.ImagePushOptions{
				All: true,
				// Push returns EOF if no 'X-Registry-Auth' header is specified
				// https://github.com/moby/moby/issues/10983
				RegistryAuth: registryAuth,
			}

			closer, err := docker.ImagePush(context.Background(), req.ReleaseImage, pushOpts)
			if err != nil {
				return errors.WithMessagef(err, "failed to push image %s", req.ReleaseImage)
			}
			io.Copy(os.Stdout, closer)
			closer.Close()

			// Push all the build tags.
			for _, t := range builtImageTags {
				log.Printf("\t\tpush tag %s", t)
				closer, err := docker.ImagePush(context.Background(), req.ReleaseImage, pushOpts)
				if err != nil {
					return errors.WithMessagef(err, "failed to push image %s", t)
				}
				io.Copy(os.Stdout, closer)
				closer.Close()
			}
		}

		log.Printf("\t%s\tbuild complete.\n", tests.Success)
	}

	// Exit and don't continue if skip deploy.
	if req.NoDeploy == true {
		return nil
	}

	log.Println("Datadog - Get API Key")
	var datadogApiKey string
	{
		// Load Datadog API Key which can be either stored in an env var or in AWS Secrets Manager.
		// 1. Check env vars for [DEV|STAGE|PROD]_DD_API_KEY and DD_API_KEY
		datadogApiKey = getTargetEnv(req.Env, "DD_API_KEY")

		// 2. Check AWS Secrets Manager for datadog entry prefixed with target env.
		if datadogApiKey == "" {
			prefixedSecretId := strings.ToUpper(req.Env) + "/DATADOG"
			var err error
			datadogApiKey, err = GetAwsSecretValue(req.AwsCreds, prefixedSecretId)
			if err != nil {
				if aerr, ok := errors.Cause(err).(awserr.Error); !ok || aerr.Code() != secretsmanager.ErrCodeResourceNotFoundException {
					return err
				}
			}
		}

		// 3. Check AWS Secrets Manager for datadog entry.
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

	log.Println("CloudWatch Logs - Get or Create Log Group")
	{
		svc := cloudwatchlogs.New(req.awsSession())

		// If no log group was found, create one.
		var err error
		_, err = svc.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(req.CloudWatchLogGroupName),
			Tags: map[string]*string{
				awsTagNameProject: aws.String(req.ProjectName),
				awsTagNameEnv:     aws.String(req.Env),
			},
		})
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


	log.Println("S3 - Setup Buckets")
	{
		svc := s3.New(req.awsSession())

		var bucketNames []string

		if req.S3BucketPrivateName != "" {
			bucketNames = append(bucketNames, req.S3BucketPrivateName)
		}
		if req.S3BucketPublicName != "" {
			bucketNames = append(bucketNames, req.S3BucketPublicName)
		}

		log.Println("\tGet or Create S3 Buckets")
		for _, bucketName := range bucketNames {
			_, err := svc.CreateBucket(&s3.CreateBucketInput{
				Bucket: aws.String(bucketName),
			})
			if err != nil {
				if aerr, ok := err.(awserr.Error); !ok || (aerr.Code() != s3.ErrCodeBucketAlreadyExists && aerr.Code() != s3.ErrCodeBucketAlreadyOwnedByYou) {
					return errors.Wrapf(err, "failed to create s3 bucket '%s'", bucketName)
				}

				log.Printf("\t\tFound: %s.", bucketName)
			} else {
				log.Printf("\t\tCreated: %s.", bucketName)
			}
		}

		log.Println("\tWait for S3 Buckets to exist")
		for _, bucketName := range bucketNames {
			log.Printf("\t\t%s", bucketName)

			err := svc.WaitUntilBucketExists(&s3.HeadBucketInput{
				Bucket: aws.String(bucketName),
			})
			if err != nil {
				return errors.Wrapf(err, "failed to wait for s3 bucket '%s' to exist", bucketName)
			}
			log.Printf("\t\t\tExists")
		}

		log.Println("\tConfigure S3 Buckets to exist")
		for _, bucketName := range bucketNames {
			log.Printf("\t\t%s", bucketName)

			// Add a life cycle policy to expire keys for the temp directory.
			_, err := svc.PutBucketLifecycleConfiguration(&s3.PutBucketLifecycleConfigurationInput{
				Bucket: aws.String(bucketName),
				LifecycleConfiguration: &s3.BucketLifecycleConfiguration{
					Rules: []*s3.LifecycleRule{
						{
							ID:     aws.String("Rule for : "+req.S3BucketTempPrefix),
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
						},
					},
				},
			})
			if err != nil {
				return errors.Wrapf(err, "failed to configure lifecycle rule for s3 bucket '%s'", bucketName)
			}
			log.Printf("\t\t\tAdded lifecycle expiration for prefix '%s'", req.S3BucketTempPrefix)

			if bucketName == req.S3BucketPublicName {
				// Enable CORS for public bucket.
				_, err = svc.PutBucketCors(&s3.PutBucketCorsInput{
					Bucket: aws.String(bucketName),
					CORSConfiguration: &s3.CORSConfiguration{
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
					},
				})
				if err != nil {
					return errors.Wrapf(err, "failed to put CORS on s3 bucket '%s'", bucketName)
				}
				log.Printf("\t\t\tUpdated CORS")

			} else {
				// Block public access for all non-public buckets.
				_, err = svc.PutPublicAccessBlock(&s3.PutPublicAccessBlockInput{
					Bucket: aws.String(bucketName),
					PublicAccessBlockConfiguration: &s3.PublicAccessBlockConfiguration{
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
				})
				if err != nil {
					return errors.Wrapf(err, "failed to block public access for s3 bucket '%s'", bucketName)
				}
				log.Printf("\t\t\tBlocked public access")

				// Add a bucket policy to enable exports from Cloudwatch Logs.
				tmpPolicyResource := strings.Trim(filepath.Join(bucketName, req.S3BucketTempPrefix), "/")
				_, err = svc.PutBucketPolicy(&s3.PutBucketPolicyInput{
					Bucket: aws.String(bucketName),
					Policy: aws.String(fmt.Sprintf(`{
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
						}`, bucketName, req.AwsCreds.Region, tmpPolicyResource, req.AwsCreds.Region )),
				})
				if err != nil {
					return errors.Wrapf(err, "failed to put bucket policy for s3 bucket '%s'", bucketName)
				}
				log.Printf("\t\t\tUpdated bucket policy")
			}
		}

		log.Printf("\t%s\tBuckets setup.\n", tests.Success)
	}

	log.Println("EC2 - Find Subnets")
	var subnetsIDs []string
	var vpcId string
	{
		svc := ec2.New(req.awsSession())

		var subnets []*ec2.Subnet
		if true { // len(req.ec2SubnetIds) == 0 {
			log.Println("\t\tFind all subnets are that default for each available AZ.")

			err := svc.DescribeSubnetsPages(&ec2.DescribeSubnetsInput{}, func(res *ec2.DescribeSubnetsOutput, lastPage bool) bool {
				for _, s := range res.Subnets {
					if *s.DefaultForAz {
						subnets = append(subnets, s)
					}
				}
				return !lastPage
			})
			if err != nil {
				return errors.Wrap(err, "failed to find default subnets")
			}
			/*} else {
			log.Println("\t\tFind all subnets for the IDs provided.")

			err := svc.DescribeSubnetsPages(&ec2.DescribeSubnetsInput{
				SubnetIds: aws.StringSlice(flags.Ec2SubnetIds),
			}, func(res *ec2.DescribeSubnetsOutput, lastPage bool) bool {
				for _, s := range res.Subnets {
					subnets = append(subnets, s)
				}
				return !lastPage
			})
			if err != nil {
				return errors.Wrapf(err, "failed to find subnets: %s", strings.Join(flags.Ec2SubnetIds, ", "))
			} else if len(flags.Ec2SubnetIds) != len(subnets)  {
				return errors.Errorf("failed to find all subnets, expected %d, got %d", len(flags.Ec2SubnetIds) != len(subnets))
			}*/
		}

		if len(subnets) == 0 {
			return errors.New("failed to find any subnets, expected at least 1")
		}

		for _, s := range subnets {
			if s.VpcId == nil {
				continue
			}
			if vpcId == "" {
				vpcId = *s.VpcId
			} else if vpcId != *s.VpcId {
				return errors.Errorf("invalid subnet %s, all subnets should belong to the same VPC, expected %s, got %s", *s.SubnetId, vpcId, *s.VpcId)
			}

			subnetsIDs = append(subnetsIDs, *s.SubnetId)
			log.Printf("\t\t\t%s", *s.SubnetId)
		}

		log.Printf("\t\tFound %d subnets.\n", len(subnets))
	}

	log.Println("EC2 - Find Security Group")
	var securityGroupId string
	{
		svc := ec2.New(req.awsSession())

		log.Printf("\t\tFind security group '%s'.\n", req.Ec2SecurityGroupName)

		err := svc.DescribeSecurityGroupsPages(&ec2.DescribeSecurityGroupsInput{
			GroupNames: aws.StringSlice([]string{req.Ec2SecurityGroupName}),
		}, func(res *ec2.DescribeSecurityGroupsOutput, lastPage bool) bool {
			for _, s := range res.SecurityGroups {
				if *s.GroupName == req.Ec2SecurityGroupName {
					securityGroupId = *s.GroupId
					break
				}

			}
			return !lastPage
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != "InvalidGroup.NotFound" {
				return errors.Wrapf(err, "failed to find security group '%s'", req.Ec2SecurityGroupName)
			}
		}

		if securityGroupId == "" {
			// If no security group was found, create one.
			createRes, err := svc.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
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
				// [EC2-VPC] The ID of the VPC. Required for EC2-VPC.
				VpcId: aws.String(vpcId),
			})
			if err != nil {
				return errors.Wrapf(err, "failed to create security group '%s'", req.Ec2SecurityGroupName)
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

		// When we are not using an Elastic Load Balancer, services need to support direct access via HTTPS.
		// HTTPS is terminated via the web server and not on the Load Balancer.
		if !req.EnableEcsElb {
			// Enable services to be publicly available via HTTPS port 443
			ingressInputs = append(ingressInputs, &ec2.AuthorizeSecurityGroupIngressInput{
				IpProtocol: aws.String("tcp"),
				CidrIp:     aws.String("0.0.0.0/0"),
				FromPort:   aws.Int64(443),
				ToPort:     aws.Int64(443),
				GroupId:    aws.String(securityGroupId),
			})
		}

		// Add all the default ingress to the security group.
		for _, ingressInput := range ingressInputs {
			_, err = svc.AuthorizeSecurityGroupIngress(ingressInput)
			if err != nil {
				if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != "InvalidPermission.Duplicate" {
					return errors.Wrapf(err, "failed to add ingress for security group '%s'", req.Ec2SecurityGroupName)
				}
			}
		}

		log.Printf("\t%s\tUsing Security Group '%s'.\n", tests.Success, req.Ec2SecurityGroupName)
	}

	var cacheCluster *elasticache.CacheCluster
	if req.CacheCluster != nil {
		log.Println("Elastic Cache - Get or Create Cache Cluster")

		// Set the security group of the cache cluster
		req.CacheCluster.SecurityGroupIds = aws.StringSlice([]string{securityGroupId})

		svc := elasticache.New(req.awsSession())

		descRes, err := svc.DescribeCacheClusters(&elasticache.DescribeCacheClustersInput{
			CacheClusterId: req.CacheCluster.CacheClusterId,
			ShowCacheNodeInfo: aws.Bool(true),
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != elasticache.ErrCodeCacheClusterNotFoundFault {
				return errors.Wrapf(err, "failed to describe cache cluster '%s'", *req.CacheCluster.CacheClusterId)
			}
		} else if len(descRes.CacheClusters) > 0 {
			cacheCluster = descRes.CacheClusters[0]
		}

		if cacheCluster == nil {
			// If no repository was found, create one.
			createRes, err := svc.CreateCacheCluster(req.CacheCluster)
			if err != nil {
				return errors.Wrapf(err, "failed to create cluster '%s'",  *req.CacheCluster.CacheClusterId)
			}
			cacheCluster = createRes.CacheCluster

			/*
				// TODO: Tag cache cluster, ARN for the cache cluster is not readly available.
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

		// If the cache cluster is not active as was recently created, wait for it to become active.
		if *cacheCluster.CacheClusterStatus != "available" {
			log.Printf("\t\tWhat for cluster to become available.")
			err = svc.WaitUntilCacheClusterAvailable(&elasticache.DescribeCacheClustersInput{
				CacheClusterId: req.CacheCluster.CacheClusterId,
			})
		}







		if *cacheCluster.Engine == "redis" {
			customCacheParameterGroupName := fmt.Sprintf("%s.%s", req.ProjectNameCamel(), *cacheCluster.Engine)

			// If the cache cluster is using the default parameter group, create a new custom group.
			if strings.HasPrefix(*cacheCluster.CacheParameterGroup.CacheParameterGroupName, "default") {
				// Lookup the group family from the current cache parameter group.
				descRes, err := svc.DescribeCacheParameterGroups(&elasticache.DescribeCacheParameterGroupsInput{
					CacheParameterGroupName: cacheCluster.CacheParameterGroup.CacheParameterGroupName,
				})
				if err != nil {
					if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != elasticache.ErrCodeCacheClusterNotFoundFault {
						return errors.Wrapf(err, "failed to describe cache parameter group '%s'", *req.CacheCluster.CacheClusterId)
					}
				}

				log.Printf("\t\tCreated custom Cache Parameter Group : %s", customCacheParameterGroupName)
				_, err = svc.CreateCacheParameterGroup(&elasticache.CreateCacheParameterGroupInput{
					CacheParameterGroupFamily: descRes.CacheParameterGroups[0].CacheParameterGroupFamily,
					CacheParameterGroupName: aws.String(customCacheParameterGroupName),
					Description: aws.String(fmt.Sprintf("Customized default parameter group for redis4.0 with cluster mode on")),
				})
				if err != nil {
					return errors.Wrapf(err, "failed to cache parameter group '%s'", customCacheParameterGroupName)
				}

				log.Printf("\t\tSet Cache Parameter Group : %s", customCacheParameterGroupName)
				_, err = svc.ModifyCacheCluster(&elasticache.ModifyCacheClusterInput{
					CacheClusterId: cacheCluster.CacheClusterId,
					CacheParameterGroupName: aws.String(customCacheParameterGroupName),
				})
				if err != nil {
					return errors.Wrapf(err, "failed modify cache parameter group '%s' for cache cluster '%s'", customCacheParameterGroupName, *cacheCluster.CacheClusterId)
				}
			}

			// Only modify the cache parameter group if the cache cluster is custom one created to allow other groups to
			// be set on the cache cluster but not modified.
			if *cacheCluster.CacheParameterGroup.CacheParameterGroupName == customCacheParameterGroupName {
				log.Printf("\t\tUpdating Cache Parameter Group : %s", *cacheCluster.CacheParameterGroup.CacheParameterGroupName)

				updateParams := []*elasticache.ParameterNameValue{
					// Recommended to be set to allkeys-lru to avoid OOM since redis will be used as an ephemeral store.
					&elasticache.ParameterNameValue{
						ParameterName: aws.String("maxmemory-policy"),
						ParameterValue: aws.String("allkeys-lru"),
					},
				}
				_, err = svc.ModifyCacheParameterGroup(&elasticache.ModifyCacheParameterGroupInput{
					CacheParameterGroupName: cacheCluster.CacheParameterGroup.CacheParameterGroupName,
					ParameterNameValues: updateParams,
				})
				if err != nil {
					return errors.Wrapf(err, "failed to modify cache parameter group '%s'", * cacheCluster.CacheParameterGroup.CacheParameterGroupName)
				}

				for _, p := range updateParams {
					log.Printf("\t\t\tSet '%s' to '%s'", p.ParameterName, p.ParameterValue)
				}
			}
		}

		log.Printf("\t%s\tUsing Cache Cluster '%s'.\n", tests.Success, *cacheCluster.CacheClusterId)
	}

	return nil

	log.Println("ECS - Get or Create Cluster")
	var ecsCluster *ecs.Cluster
	{
		svc := ecs.New(req.awsSession())

		descRes, err := svc.DescribeClusters(&ecs.DescribeClustersInput{
			Clusters: []*string{aws.String(req.EcsClusterName)},
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != ecs.ErrCodeClusterNotFoundException {
				return errors.Wrapf(err, "failed to describe cluster '%s'", req.EcsClusterName)
			}
		} else if len(descRes.Clusters) > 0 {
			ecsCluster = descRes.Clusters[0]
		}

		if ecsCluster == nil {
			// If no repository was found, create one.
			createRes, err := svc.CreateCluster(&ecs.CreateClusterInput{
				ClusterName: aws.String(req.EcsClusterName),
				Tags: []*ecs.Tag{
					&ecs.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
					&ecs.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
				},
			})
			if err != nil {
				return errors.Wrapf(err, "failed to create cluster '%s'", req.EcsClusterName)
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

	log.Println("ECS - Register task definition")
	var taskDef *ecs.TaskDefinition
	{
		// List of placeholders that can be used in task definition and replaced on deployment.
		placeholders := map[string]string{
			"{SERVICE}":        req.ServiceName,
			"{RELEASE_IMAGE}":  req.ReleaseImage,
			"{ECS_CLUSTER}":    req.EcsClusterName,
			"{ECS_SERVICE}":    req.EcsServiceName,
			"{AWS_REGION}":     req.AwsCreds.Region,
			"{AWSLOGS_GROUP}":  req.CloudWatchLogGroupName,
			"{ENV}":            req.Env,
			"{DATADOG_APIKEY}": datadogApiKey,
			"{DATADOG_ESSENTIAL}": "true",
			"{HTTP_HOST}": "0.0.0.0:80",
			"{HTTPS_HOST}": "", // Not enabled by default
			"{APP_BASE_URL}": "", // Not set by default, requires a hostname to be defined.

			"{CACHE_HOST}": "", // Not enabled by default

			"{DB_HOST}": "XXXXXXXXXXXXXXXXXX",
			"{DB_USER}": "XXXXXXXXXXXXXXXXXX",
			"{DB_PASS}": "XXXXXXXXXXXXXXXXXX",
			"{DB_DATABASE}": "XXXXXXXXXXXXXXXXXX",
			"{DB_DRIVER}": "postgres",
			"{DB_DISABLE_TLS}": "false",

			// Directly map GitLab CICD env variables set during deploy.
			"{CI_COMMIT_REF_NAME}": os.Getenv("CI_COMMIT_REF_NAME"),
			"{CI_COMMIT_REF_SLUG}": os.Getenv("CI_COMMIT_REF_SLUG"),
			"{CI_COMMIT_SHA}": os.Getenv("CI_COMMIT_SHA"),
			"{CI_COMMIT_TAG}": os.Getenv("CI_COMMIT_TAG"),
			"{CI_COMMIT_TITLE}": os.Getenv("CI_COMMIT_TITLE"),
			"{CI_COMMIT_DESCRIPTION}": os.Getenv("CI_COMMIT_DESCRIPTION"),
			"{CI_COMMIT_JOB_ID}": os.Getenv("CI_COMMIT_JOB_ID"),
			"{CI_COMMIT_JOB_URL}": os.Getenv("CI_COMMIT_JOB_URL"),
			"{CI_COMMIT_PIPELINE_ID}": os.Getenv("CI_COMMIT_PIPELINE_ID"),
			"{CI_COMMIT_PIPELINE_URL}": os.Getenv("CI_COMMIT_PIPELINE_URL"),
		}

		// When the datadog API key is empty, don't force the container to be essential have have the whole task fail.
		if datadogApiKey == "" {
			placeholders["{DATADOG_ESSENTIAL}"] = "false"
		}

		// When there is no Elastic Load Balancer, we need to terminate HTTPS on the app.
		if req.EnableHTTPS && !req.EnableEcsElb {
			placeholders["{HTTPS_HOST}"] = "0.0.0.0:443"
		}

		// When a domain name if defined for the service, set the App Base URL. Default to HTTPS if enabled.
		if req.ServiceDomainName != "" {
			var appSchema string
			if req.EnableHTTPS {
				appSchema = "https"
			} else {
				appSchema = "http"
			}

			placeholders["{APP_BASE_URL}"] = fmt.Sprintf("%s://%s/", appSchema, req.ServiceDomainName)
		}

		// When cache cache is set, set the host and port.
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

		//if flags.Debug {
		//	log.Println(string(dat))
		//}

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

		//if flags.Debug {
		//	d, _ := json.Marshal(taskDef)
		//	log.Println(string(d))
		//}

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

			/*
				res, err := svc.CreateServiceLinkedRole(&iam.CreateServiceLinkedRoleInput{
					AWSServiceName: aws.String("ecs.amazonaws.com"),
					Description:  aws.String(""),

				})
				if err != nil {
					return errors.Wrapf(err, "failed to register task definition '%s'", *taskDef.Family)
				}
				taskDefInput.ExecutionRoleArn = res.Role.Arn
			*/

			// Find or create role for ExecutionRoleArn.
			{
				log.Printf("\tAppend ExecutionRoleArn to task definition input for role %s.", req.EcsExecutionRoleName)

				res, err := svc.GetRole(&iam.GetRoleInput{
					RoleName: aws.String(req.EcsExecutionRoleName),
				})
				if err != nil {
					if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != iam.ErrCodeNoSuchEntityException {
						return errors.Wrapf(err, "failed to find task role '%s'", req.EcsExecutionRoleName)
					}
				}

				if res.Role != nil {
					taskDefInput.ExecutionRoleArn = res.Role.Arn
					log.Printf("\t\t\tFound role '%s'", *taskDefInput.ExecutionRoleArn)
				} else {
					// If no repository was found, create one.
					res, err := svc.CreateRole(&iam.CreateRoleInput{
						RoleName:                 aws.String(req.EcsExecutionRoleName),
						Description:              aws.String(fmt.Sprintf("Provides access to other AWS service resources that are required to run Amazon ECS tasks for %s. ", req.ProjectName)),
						AssumeRolePolicyDocument: aws.String("{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":[\"ecs-tasks.amazonaws.com\"]},\"Action\":[\"sts:AssumeRole\"]}]}"),
						Tags: []*iam.Tag{
							&iam.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
							&iam.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
						},
					})
					if err != nil {
						return errors.Wrapf(err, "failed to create task role '%s'", req.EcsExecutionRoleName)
					}
					taskDefInput.ExecutionRoleArn = res.Role.Arn
					log.Printf("\t\t\tCreated role '%s'", *taskDefInput.ExecutionRoleArn)
				}

				policyArns := []string{
					"arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy",
				}

				for _, policyArn := range policyArns {
					_, err = svc.AttachRolePolicy(&iam.AttachRolePolicyInput{
						PolicyArn: aws.String(policyArn),
						RoleName:  aws.String(req.EcsExecutionRoleName),
					})
					if err != nil {
						return errors.Wrapf(err, "failed to attach policy '%s' to task role '%s'", policyArn, req.EcsExecutionRoleName)
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
					return errors.Wrap(err, "failed to list IAM policies")
				}

				if policyArn != "" {
					log.Printf("\t\t\tFound policy '%s' versionId '%s'", policyArn, policyVersionId)

					res, err := svc.GetPolicyVersion(&iam.GetPolicyVersionInput{
						PolicyArn: aws.String(policyArn),
						VersionId: aws.String(policyVersionId),
					})
					if err != nil {
						if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != iam.ErrCodeNoSuchEntityException {
							return errors.Wrapf(err, "failed to read policy '%s' version '%s'", req.EcsTaskPolicyName, policyVersionId)
						}
					}

					// The policy document returned in this structure is URL-encoded compliant with
					// RFC 3986 (https://tools.ietf.org/html/rfc3986). You can use a URL decoding
					// method to convert the policy back to plain JSON text.
					curJson, err := url.QueryUnescape(*res.PolicyVersion.Document)
					if err != nil {
						return errors.Wrapf(err, "failed to url unescape policy document - %s", string(*res.PolicyVersion.Document))
					}

					// Compare policy documents and add any missing actions for each statement by matching Sid.
					var curDoc IamPolicyDocument
					err = json.Unmarshal([]byte(curJson), &curDoc)
					if err != nil {
						return errors.Wrapf(err, "failed to json decode policy document - %s", string(curJson))
					}

					var updateDoc bool
					for _, baseStmt := range baseServicePolicyDocument.Statement {
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
							return errors.Wrap(err, "failed to json encode policy document")
						}

						_, err = svc.CreatePolicyVersion(&iam.CreatePolicyVersionInput{
							PolicyArn:      aws.String(policyArn),
							PolicyDocument: aws.String(string(dat)),
							SetAsDefault:   aws.Bool(true),
						})
						if err != nil {
							if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != iam.ErrCodeNoSuchEntityException {
								return errors.Wrapf(err, "failed to read policy '%s' version '%s'", req.EcsTaskPolicyName, policyVersionId)
							}
						}
					}
				} else {
					dat, err := json.Marshal(baseServicePolicyDocument)
					if err != nil {
						return errors.Wrap(err, "failed to json encode policy document")
					}

					// If no repository was found, create one.
					res, err := svc.CreatePolicy(&iam.CreatePolicyInput{
						PolicyName:     aws.String(req.EcsTaskPolicyName),
						Description:    aws.String(fmt.Sprintf("Defines access for %s services. ", req.ProjectName)),
						PolicyDocument: aws.String(string(dat)),
					})
					if err != nil {
						return errors.Wrapf(err, "failed to create task policy '%s'", req.EcsTaskPolicyName)
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
						return errors.Wrapf(err, "failed to find task role '%s'", req.EcsTaskRoleName)
					}
				}

				if res.Role != nil {
					taskDefInput.TaskRoleArn = res.Role.Arn
					log.Printf("\t\t\tFound role '%s'", *taskDefInput.TaskRoleArn)
				} else {
					// If no repository was found, create one.
					res, err := svc.CreateRole(&iam.CreateRoleInput{
						RoleName:                 aws.String(req.EcsTaskRoleName),
						Description:              aws.String(fmt.Sprintf("Allows ECS tasks for %s to call AWS services on your behalf.", req.ProjectName)),
						AssumeRolePolicyDocument: aws.String("{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":[\"ecs-tasks.amazonaws.com\"]},\"Action\":[\"sts:AssumeRole\"]}]}"),
						Tags: []*iam.Tag{
							&iam.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
							&iam.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
						},
					})
					if err != nil {
						return errors.Wrapf(err, "failed to create task role '%s'", req.EcsTaskRoleName)
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
					return errors.Wrapf(err, "failed to attach policy '%s' to task role '%s'", policyArn, req.EcsTaskRoleName)
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
				return errors.Wrapf(err, "failed to register task definition '%s'", *taskDefInput.Family)
			}
			taskDef = res.TaskDefinition

			log.Printf("\t\tRegistered: %s.", *taskDef.TaskDefinitionArn)
			log.Printf("\t\t\tRevision: %d.", *taskDef.Revision)
			log.Printf("\t\t\tStatus: %s.", *taskDef.Status)

			log.Printf("\t%s\tTask definition registered.\n", tests.Success)
		}
	}

	log.Println("ECS - Find Service")
	var ecsService *ecs.Service
	{
		svc := ecs.New(req.awsSession())

		res, err := svc.DescribeServices(&ecs.DescribeServicesInput{
			Cluster:  ecsCluster.ClusterArn,
			Services: []*string{aws.String(req.EcsServiceName)},
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != ecs.ErrCodeServiceNotFoundException {
				return errors.Wrapf(err, "failed to describe service '%s'", req.EcsServiceName)
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
			// Service was created with no ELB and now ELB is enabled.
			recreateService = true
		} else if !req.EnableEcsElb && (ecsService.LoadBalancers != nil && len(ecsService.LoadBalancers) > 0) {
			// Service was created with ELB and now ELB is disabled.
			recreateService = true
		}

		if recreateService {
			log.Println("ECS - Delete Service")

			svc := ecs.New(req.awsSession())

			_, err := svc.DeleteService(&ecs.DeleteServiceInput{
				Cluster: ecsService.ClusterArn,
				Service: ecsService.ServiceArn,

				// If true, allows you to delete a service even if it has not been scaled down
				// to zero tasks. It is only necessary to use this if the service is using the
				// REPLICA scheduling strategy.
				Force: aws.Bool(forceDelete),
			})
			if err != nil {
				return errors.Wrapf(err, "failed to create security group '%s'", req.Ec2SecurityGroupName)
			}

			log.Printf("\t%s\tDelete Service.\n", tests.Success)
		}
	}

	// If the service exists update the service, else create a new service.
	if ecsService != nil && *ecsService.Status != "INACTIVE" {
		log.Println("ECS - Update Service")

		svc := ecs.New(req.awsSession())

		// If the desired count is zero because it was spun down for termination of staging env, update to launch
		// with at least once task running for the service.
		desiredCount := *ecsService.DesiredCount
		if desiredCount == 0 {
			desiredCount = 1
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
			return errors.Wrapf(err, "failed to update service '%s'", *ecsService.ServiceName)
		}
		ecsService = updateRes.Service

		log.Printf("\t%s\tUpdated ECS Service '%s'.\n", tests.Success, *ecsService.ServiceName)
	} else {

		// If an Elastic Load Balancer is enabled, then ensure one exists else create one.
		var ecsELBs []*ecs.LoadBalancer
		if req.EnableEcsElb {

			var certificateArn string
			if req.EnableHTTPS {
				log.Println("ACM - Find Elastic Load Balance")

				svc := acm.New(req.awsSession())

				err := svc.ListCertificatesPages(&acm.ListCertificatesInput{},
					func(res *acm.ListCertificatesOutput, lastPage bool) bool {
						for _, cert := range res.CertificateSummaryList {
							if *cert.DomainName == req.ServiceDomainName {
								certificateArn = *cert.CertificateArn
								return false
							}
						}
						return !lastPage
					})
				if err != nil {
					return errors.Wrapf(err, "failed to list certificates for '%s'", req.ServiceDomainName)
				}

				if certificateArn == "" {
					// Create hash of all the domain names to be used to mark unique requests.
					idempotencyToken := req.ServiceDomainName + "|" + strings.Join(req.ServiceDomainNameAliases, "|")
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
						DomainName: aws.String(req.ServiceDomainName),

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
						SubjectAlternativeNames: aws.StringSlice(req.ServiceDomainNameAliases),

						// The method you want to use if you are requesting a public certificate to
						// validate that you own or control domain. You can validate with DNS (https://docs.aws.amazon.com/acm/latest/userguide/gs-acm-validate-dns.html)
						// or validate with email (https://docs.aws.amazon.com/acm/latest/userguide/gs-acm-validate-email.html).
						// We recommend that you use DNS validation.
						ValidationMethod: aws.String("DNS"),
					})
					if err != nil {
						return errors.Wrapf(err, "failed to create certiciate '%s'", req.ServiceDomainName)
					}
					certificateArn = *createRes.CertificateArn

					log.Printf("\t\tCreated certiciate '%s'", req.ServiceDomainName)
				} else {
					log.Printf("\t\tFound certiciate '%s'", req.ServiceDomainName)
				}

				log.Printf("\t%s\tUsing ACM Certicate '%s'.\n", tests.Success, certificateArn)
			}

			log.Println("EC2 - Find Elastic Load Balance")
			{
				svc := elbv2.New(req.awsSession())

				var elb *elbv2.LoadBalancer
				err := svc.DescribeLoadBalancersPages(&elbv2.DescribeLoadBalancersInput{
					Names: []*string{aws.String(req.ElbLoadBalancerName)},
				}, func(res *elbv2.DescribeLoadBalancersOutput, lastPage bool) bool {
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
						return errors.Wrapf(err, "failed to describe load balancer '%s'", req.ElbLoadBalancerName)
					}
				}

				var curListeners []*elbv2.Listener
				if elb == nil {
					// If no repository was found, create one.
					createRes, err := svc.CreateLoadBalancer(&elbv2.CreateLoadBalancerInput{
						// The name of the load balancer.
						// This name must be unique per region per account, can have a maximum of 32
						// characters, must contain only alphanumeric characters or hyphens, must not
						// begin or end with a hyphen, and must not begin with "internal-".
						// Name is a required field
						Name: aws.String(req.ElbLoadBalancerName),
						// [Application Load Balancers] The type of IP addresses used by the subnets
						// for your load balancer. The possible values are ipv4 (for IPv4 addresses)
						// and dualstack (for IPv4 and IPv6 addresses).
						IpAddressType: aws.String("dualstack"),
						// The nodes of an Internet-facing load balancer have public IP addresses. The
						// DNS name of an Internet-facing load balancer is publicly resolvable to the
						// public IP addresses of the nodes. Therefore, Internet-facing load balancers
						// can route requests from clients over the internet.
						// The nodes of an internal load balancer have only private IP addresses. The
						// DNS name of an internal load balancer is publicly resolvable to the private
						// IP addresses of the nodes. Therefore, internal load balancers can only route
						// requests from clients with access to the VPC for the load balancer.
						Scheme: aws.String("Internet-facing"),
						// [Application Load Balancers] The IDs of the security groups for the load
						// balancer.
						SecurityGroups: aws.StringSlice([]string{req.Ec2SecurityGroupName}),
						// The IDs of the public subnets. You can specify only one subnet per Availability
						// Zone. You must specify either subnets or subnet mappings.
						// [Application Load Balancers] You must specify subnets from at least two Availability
						// Zones.
						Subnets: aws.StringSlice(subnetsIDs),
						// The type of load balancer.
						Type: aws.String("application"),
						// One or more tags to assign to the load balancer.
						Tags: []*elbv2.Tag{
							&elbv2.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
							&elbv2.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
						},
					})
					if err != nil {
						return errors.Wrapf(err, "failed to create load balancer '%s'", req.ElbLoadBalancerName)
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
						return errors.Wrapf(err, "failed to find listeners for load balancer '%s'", req.ElbLoadBalancerName)
					}
					curListeners = res.Listeners
				}

				// The state code. The initial state of the load balancer is provisioning. After
				// the load balancer is fully set up and ready to route traffic, its state is
				// active. If the load balancer could not be set up, its state is failed.
				log.Printf("\t\t\tState: %s.", *elb.State.Code)

				// Default target groups.
				targetGroupInputs := []*elbv2.CreateTargetGroupInput{
					// Default target group for HTTP via port 80.
					&elbv2.CreateTargetGroupInput{
						// The name of the target group.
						// This name must be unique per region per account, can have a maximum of 32
						// characters, must contain only alphanumeric characters or hyphens, and must
						// not begin or end with a hyphen.
						// Name is a required field
						Name: aws.String(fmt.Sprintf("%s-http", *elb.LoadBalancerName)),

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

						// The identifier of the virtual private cloud (VPC). If the target is a Lambda
						// function, this parameter does not apply.
						VpcId: aws.String(vpcId),
					},
				}

				// If HTTPS is enabled, then add the associated target group.
				if req.EnableHTTPS {
					// Default target group for HTTPS via port 443.
					targetGroupInputs = append(targetGroupInputs, &elbv2.CreateTargetGroupInput{
						Name:                       aws.String(fmt.Sprintf("%s-https", *elb.LoadBalancerName)),
						Port:                       aws.Int64(443),
						Protocol:                   aws.String("HTTPS"),
						HealthCheckEnabled:         aws.Bool(true),
						HealthCheckIntervalSeconds: aws.Int64(30),
						HealthCheckPath:            aws.String("/ping"),
						HealthCheckProtocol:        aws.String("HTTPS"),
						HealthCheckTimeoutSeconds:  aws.Int64(5),
						HealthyThresholdCount:      aws.Int64(3),
						UnhealthyThresholdCount:    aws.Int64(3),
						Matcher: &elbv2.Matcher{
							HttpCode: aws.String("200"),
						},
						TargetType: aws.String("ip"),
						VpcId:      aws.String(vpcId),
					})
				}

				for _, targetGroupInput := range targetGroupInputs {
					var targetGroup *elbv2.TargetGroup
					err = svc.DescribeTargetGroupsPages(&elbv2.DescribeTargetGroupsInput{
						LoadBalancerArn: elb.LoadBalancerArn,
						Names:           []*string{aws.String(req.ElbLoadBalancerName)},
					}, func(res *elbv2.DescribeTargetGroupsOutput, lastPage bool) bool {
						for _, tg := range res.TargetGroups {
							if *tg.TargetGroupName == *targetGroupInput.Name {
								targetGroup = tg
								return false
							}
						}
						return !lastPage
					})
					if err != nil {
						if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != elbv2.ErrCodeTargetGroupNotFoundException {
							return errors.Wrapf(err, "failed to describe target group '%s'", *targetGroupInput.Name)
						}
					}

					if targetGroup == nil {
						// If no target group was found, create one.
						createRes, err := svc.CreateTargetGroup(targetGroupInput)
						if err != nil {
							return errors.Wrapf(err, "failed to create target group '%s'", *targetGroupInput.Name)
						}
						targetGroup = createRes.TargetGroups[0]

						log.Printf("\t\tAdded target group: %s.", *targetGroup.TargetGroupArn)
					} else {
						log.Printf("\t\tHas target group: %s.", *targetGroup.TargetGroupArn)
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
							return errors.Wrapf(err, "failed to modify target group '%s' attributes", *targetGroupInput.Name)
						}

						log.Printf("\t\t\tSet sttributes.")
					}

					var foundListener bool
					for _, cl := range curListeners {
						if cl.Port == targetGroupInput.Port {
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
							Port: targetGroup.Port,

							// The protocol for connections from clients to the load balancer. For Application
							// Load Balancers, the supported protocols are HTTP and HTTPS. For Network Load
							// Balancers, the supported protocols are TCP, TLS, UDP, and TCP_UDP.
							//
							// Protocol is a required field
							Protocol: targetGroup.Protocol,
						}

						if *listenerInput.Protocol == "HTTPS" {
							listenerInput.Certificates = append(listenerInput.Certificates, &elbv2.Certificate{
								CertificateArn: aws.String(certificateArn),
								IsDefault:      aws.Bool(true),
							})
						}

						// If no repository was found, create one.
						createRes, err := svc.CreateListener(listenerInput)
						if err != nil {
							return errors.Wrapf(err, "failed to create listener '%s'", req.ElbLoadBalancerName)
						}

						log.Printf("\t\t\tAdded Listener: %s.", *createRes.Listeners[0].ListenerArn)
					}
				}

				log.Printf("\t%s\tUsing ELB '%s'.\n", tests.Success, *elb.LoadBalancerName)
			}
		}

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
						Subnets: aws.StringSlice(subnetsIDs),
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

			createRes, err := svc.CreateService(serviceInput)

			// If tags aren't enabled for the account, try the request again without them.
			// https://aws.amazon.com/blogs/compute/migrating-your-amazon-ecs-deployment-to-the-new-arn-and-resource-id-format-2/
			if err != nil && strings.Contains(err.Error(), "new ARN and resource ID format must be enabled") {
				serviceInput.Tags = nil
				createRes, err = svc.CreateService(serviceInput)
			}

			if err != nil {
				return errors.Wrapf(err, "failed to create service '%s'", req.EcsServiceName)
			}
			ecsService = createRes.Service

			log.Printf("\t%s\tCreated ECS Service '%s'.\n", tests.Success, *ecsService.ServiceName)
		}
	}

	log.Println("\tWaiting for service to enter stable state.")
	{

		// Helper method to get the logs from cloudwatch for a specific task ID.
		getTaskLogs := func(taskId string) ([]string, error) {
			if req.S3BucketPrivateName == "" {
				// No private S3 bucket defined so unable to export logs streams.
				return []string{}, nil
			}

			// Stream name generated by ECS for the awslogs driver.
			logStreamName := fmt.Sprintf("ecs/%s/%s", *ecsService.ServiceName, taskId)

			// Define s3 key prefix used to export the stream logs to.
			s3KeyPrefix := filepath.Join(req.S3BucketTempPrefix, "logs/cloudwatchlogs/exports", req.CloudWatchLogGroupName)

			var downloadPrefix string
			{
				svc := cloudwatchlogs.New(req.awsSession())

				createRes, err := svc.CreateExportTask(&cloudwatchlogs.CreateExportTaskInput{
					LogGroupName: aws.String(req.CloudWatchLogGroupName),
					LogStreamNamePrefix: aws.String(logStreamName),
					//TaskName: aws.String(taskId),
					Destination: aws.String( req.S3BucketPrivateName),
					DestinationPrefix: aws.String(s3KeyPrefix),
					From: aws.Int64(startTime.UTC().AddDate(0, 0, -1).UnixNano() / int64(time.Millisecond)),
					To: aws.Int64(time.Now().UTC().AddDate(0, 0, 1).UnixNano() / int64(time.Millisecond)),
				})
				if err != nil {
					return []string{}, errors.Wrapf(err, "failed to create export task for from log group '%s' with stream name prefix '%s'", req.CloudWatchLogGroupName, logStreamName)
				}
				exportTaskId := *createRes.TaskId

				for {
					descRes, err := svc.DescribeExportTasks(&cloudwatchlogs.DescribeExportTasksInput{
						TaskId: aws.String(exportTaskId),
					})
					if err != nil {
						return []string{}, errors.Wrapf(err, "failed to describe export task '%s' for from log group '%s' with stream name prefix '%s'", exportTaskId, req.CloudWatchLogGroupName, logStreamName)
					}
					taskStatus :=  *descRes.ExportTasks[0].Status.Code

					if  taskStatus == "COMPLETED" {
						downloadPrefix = filepath.Join(s3KeyPrefix, exportTaskId) + "/"
						break
					} else if taskStatus == "CANCELLED" || taskStatus == "FAILED" {
						break
					}
					time.Sleep(time.Second * 5)
				}
			}

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
					return []string{}, errors.Wrapf(err, "failed to list objects from s3 bucket '%s' with prefix '%s'", req.S3BucketPrivateName, downloadPrefix)
				}

				for _, s3Key := range s3Keys {
					res, err := svc.GetObject(&s3.GetObjectInput{
						Bucket: aws.String(req.S3BucketPrivateName),
						Key: aws.String(s3Key),
					})
					if err != nil {
						return []string{}, errors.Wrapf(err, "failed to get object '%s' from s3 bucket", s3Key, req.S3BucketPrivateName)
					}
					r,_ := gzip.NewReader(res.Body)
					dat, err := ioutil.ReadAll(r)
					res.Body.Close()
					if err != nil {
						return []string{}, errors.Wrapf(err, "failed to read object '%s' from s3 bucket", s3Key, req.S3BucketPrivateName)
					}

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
				Cluster:     aws.String(req.EcsClusterName),
				ServiceName: aws.String(req.EcsServiceName),
				DesiredStatus: aws.String("STOPPED"),
			})
			if err != nil {
				return false, errors.Wrapf(err, "failed to list tasks for cluster '%s' service '%s'", req.EcsClusterName, req.EcsServiceName)
			}

			if len(serviceTaskRes.TaskArns) == 0 {
				return false, nil
			}

			taskRes, err := svc.DescribeTasks(&ecs.DescribeTasksInput{
				Cluster: aws.String(req.EcsClusterName),
				Tasks:   serviceTaskRes.TaskArns,
			})
			if err != nil {
				return false, errors.Wrapf(err, "failed to describe %d tasks for cluster '%s'", len(serviceTaskRes.TaskArns), req.EcsClusterName)
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
					} else  {
						log.Printf("\t\t\tContainer %s exited.\n", *tc.Name)
					}
				}

				// Avoid exporting the logs multiple times.
				logLines, ok := taskLogLines[taskId]
				if !ok {
					logLines, err = getTaskLogs(taskId)
					if err != nil {
						return false, errors.Wrapf(err, "failed to get logs for task %s for cluster '%s'", *t.TaskArn, req.EcsClusterName)
					}
					taskLogLines[taskId] = logLines
				}

				if len(logLines) > 0 {
					log.Printf("\t\t\tTask Logs:\n")
					for _, l := range logLines {
						log.Printf("\t\t\t\t%s\n", l)
					}
				}

				log.Printf("\t%s\tTask %s failed with %s - %s.\n", tests.Failed, *t.TaskArn, *t.StopCode, *t.StoppedReason)

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
				case <- ticker.C:
					stop, err := checkTasks()
					if err != nil {
						log.Printf("\t%s\tFailed to check tasks.\n%+v\n", tests.Failed, err)
					}

					if stop {
						checkErr <- errors.New("all tasks for service are stopped")
						return
					}
				}
			}
		}()

		// Use the AWS ECS method to check for the service to be stable.
		go func() {
			svc := ecs.New(req.awsSession())
			err := svc.WaitUntilServicesStable(&ecs.DescribeServicesInput{
				Cluster: ecsCluster.ClusterArn,
				Services: aws.StringSlice([]string{*ecsService.ServiceArn}),
			})
			if err != nil {
				checkErr <- errors.Wrapf(err, "failed to wait for service '%s' to enter stable state", *ecsService.ServiceName)
			} else {
				// All done.
				checkErr <- nil
			}
		}()

		if err := <-checkErr;  err != nil {
			log.Printf("\t%s\tFailed to check tasks.\n%+v\n", tests.Failed, err)
			return nil
		}

		// Wait for one of the methods to finish and then ensure the ticker is stopped.
		ticker.Stop()

		log.Printf("\t%s\tService running.\n", tests.Success)
	}

	// Route 53 zone lookup when hostname is set. Supports both top level domains or sub domains.
	var zoneArecNames = map[string][]string{}
	if req.ServiceDomainName != "" {
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
			return errors.Wrap(err, "failed list route 53 hosted zones")
		}

		// Generate a slice with the primary domain name and include all the alternative domain names.
		lookupDomains := []string{req.ServiceDomainName}
		for _, dn := range req.ServiceDomainNameAliases {
			lookupDomains = append(lookupDomains, dn)
		}

		// Loop through all the defined domain names and find the associated zone even when they are a sub domain.
		for _, dn := range lookupDomains {
			log.Printf("\t\tFind zone for domain '%s'", dn)

			// Get the top level domain from url.
			zoneName := domainutil.Domain(dn)
			log.Printf("\t\t\tTop Level Domain: '%s'", zoneName)

			// Check if url has subdomain.
			var subdomain string
			if domainutil.HasSubdomain(dn) {
				subdomain = domainutil.Subdomain(dn)
				log.Printf("\t\t\tsubdomain: '%s'", subdomain)
			}

			// Start at the top level domain and try to find a hosted zone. Search until a match is found or there are
			// no more domain levels to search for.
			var zoneId string
			for {
				log.Printf("\t\t\tChecking zone '%s' for associated hosted zone.", zoneName)

				// Loop over each one of hosted zones and try to find match.
				for _, z := range zones {
					log.Printf("\t\t\t\tChecking if %s matches %s", *z.Name, zoneName)

					if strings.TrimRight(*z.Name, ".") == zoneName {
						zoneId = *z.Id
						break
					}
				}

				if zoneId != "" || zoneName == dn {
					// Found a matching zone or have search all possibilities!
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
					CallerReference: aws.String("truss-deploy"),
				})
				if err != nil {
					return errors.Wrapf(err, "failed to create route 53 hosted zone '%s' for domain '%s'", zoneName, dn)
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

	if req.EnableEcsElb {
		// TODO: Need to connect ELB to route53
	} else {
		log.Println("\tFind network interface IDs for running tasks.")
		var networkInterfaceIds []string
		{
			svc := ecs.New(req.awsSession())

			log.Println("\t\tFind tasks for service.")
			servceTaskRes, err := svc.ListTasks(&ecs.ListTasksInput{
				Cluster:     aws.String(req.EcsClusterName),
				ServiceName: aws.String(req.EcsServiceName),
			})
			if err != nil {
				return errors.Wrapf(err, "failed to list tasks for cluster '%s' service '%s'", req.EcsClusterName, req.EcsServiceName)
			}

			log.Println("\t\tDescribe tasks for service.")
			taskRes, err := svc.DescribeTasks(&ecs.DescribeTasksInput{
				Cluster: aws.String(req.EcsClusterName),
				Tasks:   servceTaskRes.TaskArns,
			})
			if err != nil {
				return errors.Wrapf(err, "failed to describe %d tasks for cluster '%s'", len(servceTaskRes.TaskArns), req.EcsClusterName)
			}

			var failures []*ecs.Failure
			var taskArns []string
			for _, t := range taskRes.Tasks {
				if t.TaskDefinitionArn != taskDef.TaskDefinitionArn {
					continue
				}

				taskArns = append(taskArns, *t.TaskArn)

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
			} else {
				log.Printf("\t%s\tTasks founds.\n", tests.Success)
			}

			log.Println("\t\tWaiting for tasks to enter running state.")
			{
				var err error
				err = svc.WaitUntilTasksRunning(&ecs.DescribeTasksInput{
					Cluster: ecsCluster.ClusterArn,
					Tasks:   aws.StringSlice(taskArns),
				})
				if err != nil {
					return errors.Wrapf(err, "failed to wait for tasks to enter running state for cluster '%s'", req.EcsClusterName)
				}
				log.Printf("\t%s\tTasks running.\n", tests.Success)
			}

			log.Println("\t\tDescribe tasks for running tasks.")
			{
				taskRes, err := svc.DescribeTasks(&ecs.DescribeTasksInput{
					Cluster: aws.String(req.EcsClusterName),
					Tasks:   aws.StringSlice(taskArns),
				})
				if err != nil {
					return errors.Wrapf(err, "failed to describe %d running tasks for cluster '%s'", len(taskArns), req.EcsClusterName)
				}

				for _, t := range taskRes.Tasks {
					if t.Attachments == nil {
						continue
					}

					for _, a := range t.Attachments {
						if a.Details == nil {
							continue
						}

						for _, ad := range a.Details {
							if ad.Name != nil && *ad.Name == "networkInterfaceId" {
								networkInterfaceIds = append(networkInterfaceIds, *ad.Value)
							}
						}
					}
				}

				log.Printf("\t%s\tFound %d network interface IDs.\n", tests.Success, len(networkInterfaceIds))
			}
		}

		log.Println("\tGet public IPs for network interface IDs.")
		var publicIps []string
		{
			svc := ec2.New(req.awsSession())

			log.Println("\t\tDescribe network interfaces.")
			res, err := svc.DescribeNetworkInterfaces(&ec2.DescribeNetworkInterfacesInput{
				NetworkInterfaceIds: aws.StringSlice(networkInterfaceIds),
			})
			if err != nil {
				return errors.Wrap(err, "failed to describe network interfaces")
			}

			for _, ni := range res.NetworkInterfaces {
				if ni.Association == nil || ni.Association.PublicIp == nil {
					continue
				}
				publicIps = append(publicIps, *ni.Association.PublicIp)
			}

			log.Printf("\t%s\tFound %d public IPs.\n", tests.Success, len(publicIps))
		}

		log.Println("\tUpdate public IPs for hosted zones.")
		{
			svc := route53.New(req.awsSession())

			// Public IPs to be served as round robin.
			log.Printf("\t\tPublic IPs:\n")
			rrs := []*route53.ResourceRecord{}
			for _, ip := range publicIps {
				log.Printf("\t\t\t%s\n", ip)
				rrs = append(rrs, &route53.ResourceRecord{Value: aws.String(ip)})
			}

			for zoneId, aNames := range zoneArecNames {
				log.Printf("\t\tChange zone '%s'.\n", zoneId)

				input := &route53.ChangeResourceRecordSetsInput{
					ChangeBatch: &route53.ChangeBatch{
						Changes: []*route53.Change{},
					},
					HostedZoneId: aws.String(zoneId),
				}

				// Add all the A record names with the same set of public IPs.
				for _, aName := range aNames {
					log.Printf("\t\t\tAdd A record for '%s'.\n", aName)

					input.ChangeBatch.Changes = append(input.ChangeBatch.Changes, &route53.Change{
						Action: aws.String("UPSERT"),
						ResourceRecordSet: &route53.ResourceRecordSet{
							Name:            aws.String(aName),
							ResourceRecords: rrs,
							TTL:             aws.Int64(60),
							Type:            aws.String("A"),
						},
					})
				}

				_, err := svc.ChangeResourceRecordSets(input)
				if err != nil {
					return errors.Wrapf(err, "failed to update A records for zone '%s'", zoneId)
				}
			}

			log.Printf("\t%s\tDNS entries updated.\n", tests.Success)
		}
	}


	// If Elastic cache is enabled, need to add ingress to security group

	return nil
}
