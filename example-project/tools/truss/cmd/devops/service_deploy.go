package devops

import (
	"encoding/json"
	"fmt"
	"gopkg.in/go-playground/validator.v9"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
)


var requiredDeployIamPermissions = []string{
	"secretsmanager:GetSecretValue",
	"ecr:GetAuthorizationToken",
	"ecr:ListImages",
	"ecr:DescribeRepositories",
	"ecr:CreateRepository",
	"ecs:CreateCluster",
	"ecs:DescribeClusters",
	"esc:RegisterTaskDefinition",
	"cloudwatchlogs:DescribeLogGroups",
	"cloudwatchlogs:CreateLogGroup",
	"iam:CreateServiceLinkedRole",
	"iam:PutRolePolicy",
}

// requiredCmdsBuild proves a list of required executables for completing build.
var requiredCmdsDeploy = [][]string{
	[]string{"docker", "version", "-f", "{{.Client.Version}}"},
}

// NewServiceDeployRequest generated a new request for executing deploy for a given set of flags.
func NewServiceDeployRequest(log *log.Logger, flags *ServiceDeployFlags) (*serviceDeployRequest, error) {

	log.Println("Validate flags.")
	{
		errs := validator.New().Struct(flags)
		if errs != nil {
			return nil, errs
		}
		log.Printf("\t%s\tFlags ok.", tests.Success)
	}

	log.Println("\tVerify AWS credentials.")
	var awsCreds *AwsCredentials
	{
		var err error
		awsCreds, err = GetAwsCredentials(flags.Env)
		if err != nil {
			return nil, err
		}
		log.Printf("\t\t\tAccessKeyID: %s", awsCreds.AccessKeyID)
		log.Printf("\t\t\tRegion: %s", awsCreds.Region)
		log.Printf("\t%s\tAWS credentials valid.", tests.Success)
	}

	log.Println("Generate deploy request.")
	var req *serviceDeployRequest
	{
		req = &serviceDeployRequest{
			// Required flags.
			serviceName: flags.ServiceName,
			env: flags.Env,
			awsCreds: awsCreds,

			// Optional flags.
			projectRoot: flags.ProjectRoot,
			projectName : flags.ProjectName,
			dockerFile: flags.DockerFile,
			enableLambdaVPC: flags.EnableLambdaVPC,
			enableEcsElb : flags.EnableEcsElb,
			noBuild: flags.NoBuild,
			noDeploy: flags.NoDeploy,
			noCache: flags.NoCache,
			noPush: flags.NoPush,
		}

		// When project root directory is empty or set to current working path, then search for the project root by locating
		// the go.mod file.
		log.Println("\tDetermining the project root directory.")
		{
			if flags.ProjectRoot == "" || flags.ProjectRoot == "." {
				log.Println("\tAttempting to location project root directory from current working directory.")

				var err error
				req.goModFile, err = findProjectGoModFile()
				if err != nil {
					return nil, err
				}
				req.projectRoot = filepath.Dir(req.goModFile)
			} else {
				log.Println("\t\tUsing supplied project root directory.")
				req.goModFile = filepath.Join(flags.ProjectRoot, "go.mod")
			}
			log.Printf("\t\t\tproject root: %s", req.projectRoot)
			log.Printf("\t\t\tgo.mod: %s", req.goModFile )
		}

		log.Println("\tExtracting go module name from go.mod.")
		{
			var err error
			req.goModName, err = loadGoModName(req.goModFile)
			if err != nil {
				return nil, err
			}
			log.Printf("\t\t\tmodule name: %s", req.goModName)
		}

		log.Println("\tDetermining the project name.")
		{
			if flags.ProjectName != "" {
				req.projectName = flags.ProjectName
				log.Printf("\t\tUse provided value.")
			} else {
				req.projectName = filepath.Base(req.goModName)
				log.Printf("\t\tSet from go module.")
			}
			log.Printf("\t\t\tproject name: %s", req.projectName)
		}

		log.Println("\tAttempting to locate service directory from project root directory.")
		{
			if flags.DockerFile != "" {
				req.dockerFile = flags.DockerFile
				log.Printf("\t\tUse provided value.")

			} else {
				log.Printf("\t\tFind from project root looking for Dockerfile.")
				var err error
				req.dockerFile, err = findServiceDockerFile(req.projectRoot, req.serviceName)
				if err != nil {
					return nil, err
				}
			}

			req.serviceDir = filepath.Dir(flags.DockerFile)

			log.Printf("\t\t\tservice directory: %s", req.serviceDir)
			log.Printf("\t\t\tdockerfile: %s", req.dockerFile)
		}


		log.Println("\tSet defaults not defined in env vars.")
		{
			// Set default AWS ECR Repository Name.
			req.ecrRepositoryName  = req.projectName
			log.Printf("\t\t\tSet ECR Repository Name to '%s'.", req.ecrRepositoryName)

			// Set default AWS ECR Regsistry Max Images.
			req.ecrRepositoryMaxImages = defaultAwsRegistryMaxImages
			log.Printf("\t\t\tSet ECR Regsistry Max Images to '%d'.", req.ecrRepositoryMaxImages)


			// Set default AWS ECS Cluster Name.
			req.ecsClusterName = req.projectName + "-" + req.env
			log.Printf("\t\t\tSet ECS Cluster Name to '%s'.", req.ecsClusterName)


			// Set default AWS ECS Service Name.
			req.ecsServiceName = req.serviceName + "-" + req.env
			log.Printf("\t\t\tSet ECS Service Name to '%s'.", req.ecsServiceName)


			// Set default Cloudwatch Log Group Name.
			req.cloudWatchLogGroupName = fmt.Sprintf("logs/env_%s/aws/ecs/cluster_%s/service_%s", req.env, req.ecsClusterName, req.serviceName)
			log.Printf("\t\t\tSet CloudWatch Log Group Name to '%s'.", req.cloudWatchLogGroupName)

			// Set default EC2 Security Group Name.
			req.ec2SecurityGroupName = req.ecsClusterName
			log.Printf("\t\t\tSet ECS Security Group Name to '%s'.", req.ec2SecurityGroupName)

			// Set ECS configs based on specified env.
			if flags.Env == "prod" {
				req.ecsServiceMinimumHealthyPercent  = aws.Int64(100)
				req.ecsServiceMaximumPercent = aws.Int64(200)

				req.elbDeregistrationDelay =aws.Int( 300)
			} else {
				req.ecsServiceMinimumHealthyPercent  = aws.Int64(100)
				req.ecsServiceMaximumPercent = aws.Int64(200)

				// force staging to deploy immediately without waiting for connections to drain
				req.elbDeregistrationDelay = aws.Int(0)
			}
			req.ecsServiceDesiredCount = 1
			req.escServiceHealthCheckGracePeriodSeconds = aws.Int64(60)

			log.Printf("\t%s\tDefaults set.", tests.Success)
		}

		log.Println("\tValidate request.")
		errs := validator.New().Struct(req)
		if errs != nil {
			return nil, errs
		}

		log.Printf("\t%s\tNew request generated.", tests.Success)
	}

	return req, nil
}

// Run is the main entrypoint for deploying a service for a given target env.
func ServiceDeploy(log *log.Logger, req *serviceDeployRequest) error {

	//
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

	// Load the ECR repository.
	log.Println("ECR - Get or create repository.")
	var awsRepo *ecr.Repository
	{
		svc := ecr.New(awsCreds.Session())

		descRes, err := svc.DescribeRepositories(&ecr.DescribeRepositoriesInput{
			RepositoryNames: []*string{aws.String(awsCreds.EcrRepositoryName)},
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != ecr.ErrCodeRepositoryNotFoundException {
				return errors.Wrapf(err, "failed to describe repository '%s'", awsCreds.EcrRepositoryName)
			}
		} else if len(descRes.Repositories) > 0 {
			awsRepo = descRes.Repositories[0]
		}

		if awsRepo == nil {
			// If no repository was found, create one.
			createRes, err := svc.CreateRepository(&ecr.CreateRepositoryInput{
				RepositoryName: aws.String(awsCreds.EcrRepositoryName),
				Tags: []*ecr.Tag{
					&ecr.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(flags.ProjectName)},
					&ecr.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(flags.Env)},
				},
			})
			if err != nil {
				return errors.Wrapf(err, "failed to create repository '%s'", awsCreds.EcrRepositoryName)
			}
			awsRepo = createRes.Repository
			log.Printf("\t\tCreated: %s.", *awsRepo.RepositoryArn)
		} else {
			log.Printf("\t\tFound: %s.", *awsRepo.RepositoryArn)

			log.Println("\t\tChecking old ECR images.")
			delIds, err := EcrPurgeImages(awsCreds)
			if err != nil {
				return err
			}

			// If there are image IDs to delete, delete them.
			if len(delIds) > 0 {
				log.Printf("\t\tDeleted %d images that exceeded limit of %d", len(delIds), awsCreds.EcrRepositoryMaxImages)
				for _, imgId := range delIds {
					log.Printf("\t\t\t%s", *imgId.ImageTag)
				}
			}
		}

		if len(flags.BuildTags) > 0 {
			if flags.ReleaseImage == "" {
				flags.ReleaseImage = *awsRepo.RepositoryUri + ":" + flags.BuildTags[0]
			}
		} else if flags.ReleaseImage == "" {
			tag1 := flags.Env + "-" + flags.ServiceName
			flags.BuildTags = append(flags.BuildTags, tag1)

			if v := os.Getenv("CI_COMMIT_REF_NAME"); v != "" {
				tag2 := tag1 + "-" + v
				flags.BuildTags = append(flags.BuildTags, tag2)
				flags.ReleaseImage = *awsRepo.RepositoryUri+":"+tag2
			} else {
				flags.ReleaseImage = *awsRepo.RepositoryUri+":"+tag1
			}
		}
		log.Printf("\t\trelease image: %s", flags.ReleaseImage)

		log.Printf("\t\ttags: %s", strings.Join(flags.BuildTags, " "))
		log.Printf("\t%s\tRelease image valid.", tests.Success)

		log.Println("ECR - Retrieve authorization token used for docker login.")
		dockerLogin, err := GetEcrLogin(awsCreds)
		if err != nil {
			return err
		}

		log.Println("\t\texecute docker login")
		_, err = execCmds(flags.ProjectRoot, &envVars, dockerLogin)
		if err != nil {
			return err
		}
		log.Printf("\t%s\tDocker login complete.", tests.Success)
	}

	// Do the docker build.
	if flags.NoBuild == false {
		cmdVals := []string{
			"docker",
			"build",
			"--file=" + flags.DockerFile,
			"--build-arg", "service=" + flags.ServiceName,
			"--build-arg", "env=" + flags.Env,
			"-t", flags.ReleaseImage,
		}

		// Append the build tags.
		var builtImageTags []string
		for _, t := range flags.BuildTags {
			imageTag := flags.ReleaseImage+":"+t
			if imageTag == flags.ReleaseImage {
				// skip duplicate image tags
				continue
			}

			cmdVals = append(cmdVals, "-t")
			cmdVals = append(cmdVals, imageTag)
			builtImageTags = append(builtImageTags, imageTag)
		}

		if flags.NoCache == true {
			cmdVals = append(cmdVals, "--no-cache")
		}
		cmdVals = append(cmdVals, ".")

		log.Printf("starting docker build: \n\t\t%s", strings.Join(cmdVals, " "))
		out, err := execCmds(flags.ProjectRoot, &envVars, cmdVals)
		if err != nil {
			return err
		}

		// Push the newly built docker container to the registry.
		if flags.NoPush == false {
			log.Printf("\t\tpush release image %s", flags.ReleaseImage)
			_, err = execCmds(flags.ProjectRoot, &envVars, []string{"docker", "push", flags.ReleaseImage})
			if err != nil {
				return err
			}

			// Push all the build tags.
			for _, t := range builtImageTags {
				log.Printf("\t\tpush tag %s", t)
				_, err = execCmds(flags.ProjectRoot, &envVars, []string{"docker", "push", t})
				if err != nil {
					return err
				}
			}
		}

		log.Printf("\t%s\tbuild complete.\n", tests.Success)
		if flags.Debug {
			log.Println(string(out[0]))
		}
	}

	// Exit and don't continue if skip deploy.
	if flags.NoDeploy == true {
		return nil
	}

	log.Println("Datadog - Get API Key")
	var datadogApiKey string
	{
		// Load Datadog API Key which can be either stored in an env var or in AWS Secrets Manager.
		// 1. Check env vars for [DEV|STAGE|PROD]_DD_API_KEY and DD_API_KEY
		datadogApiKey = getTargetEnv(flags.Env, "DD_API_KEY")

		// 2. Check AWS Secrets Manager for datadog entry prefixed with target env.
		if datadogApiKey == "" {
			prefixedSecretId := strings.ToUpper(flags.Env) + "/DATADOG"
			var err error
			datadogApiKey, err = GetAwsSecretValue(awsCreds, prefixedSecretId)
			if err != nil {
				if aerr, ok := errors.Cause(err).(awserr.Error); !ok || aerr.Code() != secretsmanager.ErrCodeResourceNotFoundException {
					return  err
				}
			}
		}

		// 3. Check AWS Secrets Manager for datadog entry.
		if datadogApiKey == "" {
			secretId := "DATADOG"
			datadogApiKey, err = GetAwsSecretValue(awsCreds, secretId)
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
		svc := cloudwatchlogs.New(awsCreds.Session())

		/*var logGroup *cloudwatchlogs.LogGroup
		err := svc.DescribeLogGroupsPages(&cloudwatchlogs.DescribeLogGroupsInput{
			LogGroupNamePrefix: aws.String(awsCreds.CloudWatchLogGroupName),
		}, func(res *cloudwatchlogs.DescribeLogGroupsOutput, lastPage bool) bool{
			for _, lg := range res.LogGroups {
				if *lg.LogGroupName == awsCreds.CloudWatchLogGroupName {
					logGroup = lg
					return false
				}

			}

			return !lastPage
		})
		if err != nil {
			return errors.Wrapf(err, "failed to describe log groups for prefix '%s'", awsCreds.CloudWatchLogGroupName)
		}*/

		// If no log group was found, create one.
		_, err = svc.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(awsCreds.CloudWatchLogGroupName),
			Tags: map[string]*string{
				awsTagNameProject: aws.String(flags.ProjectName),
				awsTagNameEnv: aws.String(flags.Env),
			},
		})
		if err != nil {

			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != cloudwatchlogs.ErrCodeResourceAlreadyExistsException {
				return  errors.Wrapf(err, "failed to create log group '%s'", awsCreds.CloudWatchLogGroupName)
			}

			log.Printf("\t\tFound: %s.", awsCreds.CloudWatchLogGroupName)
		} else {
			log.Printf("\t\tCreated: %s.", awsCreds.CloudWatchLogGroupName)
		}

		log.Printf("\t%s\tUsing Log Group '%s'.\n", tests.Success, awsCreds.CloudWatchLogGroupName)
	}

	log.Println("ECS - Get or Create Cluster")
	var ecsCluster *ecs.Cluster
	{
		svc := ecs.New(awsCreds.Session())

		descRes, err := svc.DescribeClusters(&ecs.DescribeClustersInput{
			Clusters: []*string{aws.String(awsCreds.EcsClusterName)},
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != ecs.ErrCodeClusterNotFoundException {
				return errors.Wrapf(err, "failed to describe cluster '%s'", awsCreds.EcsClusterName)
			}
		} else if len( descRes.Clusters) > 0 {
			ecsCluster = descRes.Clusters[0]
		}

		if ecsCluster == nil  {
			// If no repository was found, create one.
			createRes, err := svc.CreateCluster(&ecs.CreateClusterInput{
				ClusterName: aws.String(awsCreds.EcsClusterName),
				Tags: []*ecs.Tag{
					&ecs.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(flags.ProjectName)},
					&ecs.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(flags.Env)},
				},
			})
			if err != nil {
				return errors.Wrapf(err, "failed to create cluster '%s'", awsCreds.EcsClusterName)
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
			"{SERVICE}": flags.ServiceName,
			"{RELEASE_IMAGE}": flags.ReleaseImage,
			"{ECS_CLUSTER}": awsCreds.EcsClusterName,
			"{ECS_SERVICE}": awsCreds.EcsServiceName,
			"{AWS_REGION}": awsCreds.Region,
			"{AWSLOGS_GROUP}": awsCreds.CloudWatchLogGroupName,
			"{ENV}": flags.Env,
			"{DATADOG_APIKEY}": datadogApiKey,
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
		dat, err := EcsReadTaskDefinition(flags.ServiceDir, flags.Env)
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
			taskDefInput.Family = &flags.ServiceName
		}
		if len(taskDefInput.ContainerDefinitions) > 0 {
			if taskDefInput.ContainerDefinitions[0].Name == nil || *taskDefInput.ContainerDefinitions[0].Name == "" {
				taskDefInput.ContainerDefinitions[0].Name = &awsCreds.EcsServiceName
			}
			if taskDefInput.ContainerDefinitions[0].Image == nil || *taskDefInput.ContainerDefinitions[0].Image == "" {
				taskDefInput.ContainerDefinitions[0].Image = &flags.ReleaseImage
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
				totalCpu int64
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
				selectedCpu int64
			)
			if totalMemory < 8192 {
				if totalMemory > 7168 {
					selectedMemory = 8192

					if totalCpu >= 2048 {
						selectedCpu=4096
					} else if totalCpu >= 1024 {
						selectedCpu = 2048
					} else {
						selectedCpu = 1024
					}
				} else if totalMemory > 6144 {
					selectedMemory=7168

					if totalCpu >= 2048 {
						selectedCpu=4096
					} else if totalCpu >=  1024 {
						selectedCpu = 2048
					} else {
						selectedCpu = 1024
					}
				} else if totalMemory > 5120 || totalCpu >=  1024 {
					selectedMemory=6144

					if totalCpu >= 2048 {
						selectedCpu=4096
					} else if totalCpu >=  1024 {
						selectedCpu = 2048
					} else {
						selectedCpu = 1024
					}
				} else if totalMemory > 4096 {
					selectedMemory=5120

					if totalCpu >= 512 {
						selectedCpu = 1024
					} else {
						selectedCpu = 512
					}
				} else if totalMemory > 3072 {
					selectedMemory=4096

					if totalCpu >= 512 {
						selectedCpu = 1024
					} else {
						selectedCpu = 512
					}
				} else if totalMemory >  2048 || totalCpu >= 512 {
					selectedMemory=3072

					if totalCpu >= 512 {
						selectedCpu = 1024
					} else {
						selectedCpu = 512
					}
				} else if totalMemory >  1024 || totalCpu >= 256 {
					selectedMemory=2048

					if totalCpu >= 256 {
						if totalCpu >= 512 {
							selectedCpu = 1024
						} else {
							selectedCpu = 512
						}
					} else {
						selectedCpu = 256
					}
				} else if totalMemory >  512 {
					selectedMemory=1024

					if totalCpu >= 256 {
						selectedCpu = 512
					} else {
						selectedCpu = 256
					}
				} else {
					selectedMemory=512
					selectedCpu=256
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
			if flags.EcsExecutionRoleArn != "" {
				taskDefInput.ExecutionRoleArn = &flags.EcsExecutionRoleArn
				log.Printf("\t%s\tExecutionRoleArn updated.\n", tests.Success)
			} else {
				svc := iam.New(awsCreds.Session())

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
					roleName := fmt.Sprintf("ecsExecutionRole%s%s", flags.ProjectNameCamel(), strcase.ToCamel(flags.Env))
					log.Printf("\tAppend ExecutionRoleArn to task definition input for role %s.", roleName)

					res, err := svc.GetRole(&iam.GetRoleInput{
						RoleName: aws.String(roleName),
					})
					if err != nil {
						if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != iam.ErrCodeNoSuchEntityException {
							return errors.Wrapf(err, "failed to find task role '%s'", roleName)
						}
					}

					if res.Role != nil {
						taskDefInput.ExecutionRoleArn = res.Role.Arn
						log.Printf("\t\t\tFound role '%s'", *taskDefInput.ExecutionRoleArn)
					} else {
						// If no repository was found, create one.
						res, err := svc.CreateRole(&iam.CreateRoleInput{
							RoleName:                 aws.String(roleName),
							Description:              aws.String(fmt.Sprintf("Provides access to other AWS service resources that are required to run Amazon ECS tasks for %s. ", flags.ProjectName)),
							AssumeRolePolicyDocument: aws.String("{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":[\"ecs.amazonaws.com\"]},\"Action\":[\"sts:AssumeRole\"]}]}"),
							Tags: []*iam.Tag{
								&iam.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(flags.ProjectName)},
								&iam.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(flags.Env)},
							},

						})
						if err != nil {
							return errors.Wrapf(err, "failed to create task role '%s'", roleName)
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
							RoleName:  aws.String(roleName),
						})
						if err != nil {
							return errors.Wrapf(err, "failed to attach policy '%s' to task role '%s'", policyArn, roleName)
						}
						log.Printf("\t\t\t\tAttached Policy '%s'", policyArn)
					}

					log.Printf("\t%s\tExecutionRoleArn updated.\n", tests.Success)
				}
			}
		}

		// The task role is the IAM role used by the task itself to access other AWS Services. To access services
		// like S3, SQS, etc then those permissions would need to be covered by the TaskRole.
		if taskDefInput.TaskRoleArn == nil || *taskDefInput.TaskRoleArn == "" {

			if flags.EcsTaskRoleArn != "" {
				taskDefInput.TaskRoleArn = &flags.EcsTaskRoleArn
				log.Printf("\t%s\tTaskRoleArn updated.\n", tests.Success)
			} else {
				svc := iam.New(awsCreds.Session())

				// Find or create the default service policy.
				var policyArn string
				{
					policyName := fmt.Sprintf("%s%sServices", flags.ProjectNameCamel(), strcase.ToCamel(flags.Env))
					log.Printf("\tFind default service policy %s.", policyName)

					var policyVersionId string
					err = svc.ListPoliciesPages(&iam.ListPoliciesInput{}, func(res *iam.ListPoliciesOutput, lastPage bool) bool{
						for _, p := range res.Policies {
							if *p.PolicyName == policyName {
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
								return errors.Wrapf(err, "failed to read policy '%s' version '%s'", policyName, policyVersionId)
							}
						}

						// Compare policy documents and add any missing actions for each statement by matching Sid.
						var curDoc IamPolicyDocument
						err = json.Unmarshal([]byte(*res.PolicyVersion.Document), &curDoc)
						if err != nil {
							return errors.Wrap(err, "failed to json decode policy document")
						}

						var updateDoc bool
						for _, baseStmt := range baseServicePolicyDocument.Statement {
							var found bool
							for curIdx, curStmt := range curDoc.Statement {
								if baseStmt.Sid != curStmt.Sid {
									continue
								}

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
								PolicyArn: aws.String(policyArn),
								PolicyDocument: aws.String(string(dat)),
								SetAsDefault: aws.Bool(true),
							})
							if err != nil {
								if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != iam.ErrCodeNoSuchEntityException {
									return errors.Wrapf(err, "failed to read policy '%s' version '%s'", policyName, policyVersionId)
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
							PolicyName:                 aws.String(policyName),
							Description:              aws.String(fmt.Sprintf("Defines access for %s services. ", flags.ProjectName)),
							PolicyDocument: aws.String(string(dat)),

						})
						if err != nil {
							return errors.Wrapf(err, "failed to create task policy '%s'", policyName)
						}

						policyArn = *res.Policy.Arn

						log.Printf("\t\t\tCreated policy '%s'", policyArn)
					}

					log.Printf("\t%s\tConfigured default service policy.\n", tests.Success)
				}

				// Find or create role for TaskRoleArn.
				{
					roleName := fmt.Sprintf("ecsTaskRole%s%s", flags.ProjectNameCamel(), strcase.ToCamel(flags.Env))
					log.Printf("\tAppend TaskRoleArn to task definition input for role %s.", roleName)

					res, err := svc.GetRole(&iam.GetRoleInput{
						RoleName: aws.String(roleName),
					})
					if err != nil {
						if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != iam.ErrCodeNoSuchEntityException {
							return errors.Wrapf(err, "failed to find task role '%s'", roleName)
						}
					}

					if res.Role != nil {
						taskDefInput.TaskRoleArn = res.Role.Arn
						log.Printf("\t\t\tFound role '%s'", *taskDefInput.TaskRoleArn)
					} else {
						// If no repository was found, create one.
						res, err := svc.CreateRole(&iam.CreateRoleInput{
							RoleName:                 aws.String(roleName),
							Description:              aws.String(fmt.Sprintf("Allows ECS tasks for %s to call AWS services on your behalf. ", flags.ProjectName)),
							AssumeRolePolicyDocument: aws.String("{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Principal\":{\"Service\":[\"ecs-tasks.amazonaws.com\"]},\"Action\":[\"sts:AssumeRole\"]}]}"),
							Tags: []*iam.Tag{
								&iam.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(flags.ProjectName)},
								&iam.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(flags.Env)},
							},

						})
						if err != nil {
							return errors.Wrapf(err, "failed to create task role '%s'", roleName)
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

					_, err = svc.AttachRolePolicy( &iam.AttachRolePolicyInput{
						PolicyArn: aws.String(policyArn),
						RoleName:  aws.String(roleName),
					})
					if err != nil {
						return errors.Wrapf(err, "failed to attach policy '%s' to task role '%s'", policyArn, roleName)
					}

					log.Printf("\t%s\tTaskRoleArn updated.\n", tests.Success)
				}
			}
		}

		log.Println("\tRegister new task definition.")
		{
			svc := ecs.New(awsCreds.Session())

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
		svc := ecs.New(awsCreds.Session())

		res, err := svc.DescribeServices(&ecs.DescribeServicesInput{
			Services: []*string{aws.String(awsCreds.EcsServiceName)},
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != ecs.ErrCodeServiceNotFoundException {
				return errors.Wrapf(err, "failed to describe service '%s'", awsCreds.EcsServiceName)
			}
		} else if len( res.Services) > 0 {
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

	// If the service exists update the service, else create a new service.
	if ecsService != nil && *ecsService.Status != "INACTIVE" {
		log.Println("ECS - Update Service")

		svc := ecs.New(awsCreds.Session())

		// If the desired count is zero because it was spun down for termination of staging env, update to launch
		// with at least once task running for the service.
		desiredCount := *ecsService.DesiredCount
		if desiredCount == 0 {
			desiredCount = 1
		}

		_, err = svc.UpdateService(&ecs.UpdateServiceInput{
			// The short name or full Amazon Resource Name (ARN) of the cluster that your
			// service is running on. If you do not specify a cluster, the default cluster
			// is assumed.
			Cluster: ecsCluster.ClusterName,

			// The name of the service to update.
			Service: ecsService.ServiceName,

			// The number of instantiations of the task to place and keep running in your
			// service.
			DesiredCount: aws.Int64(desiredCount),

			// Whether to force a new deployment of the service. Deployments are not forced
			// by default. You can use this option to trigger a new deployment with no service
			// definition changes. For example, you can update a service's tasks to use
			// a newer Docker image with the same image/tag combination (my_image:latest)
			// or to roll Fargate tasks onto a newer platform version.
			ForceNewDeployment: aws.Bool(false),

			// The period of time, in seconds, that the Amazon ECS service scheduler should
			// ignore unhealthy Elastic Load Balancing target health checks after a task
			// has first started. This is only valid if your service is configured to use
			// a load balancer. If your service's tasks take a while to start and respond
			// to Elastic Load Balancing health checks, you can specify a health check grace
			// period of up to 1,800 seconds. During that time, the ECS service scheduler
			// ignores the Elastic Load Balancing health check status. This grace period
			// can prevent the ECS service scheduler from marking tasks as unhealthy and
			// stopping them before they have time to come up.
			HealthCheckGracePeriodSeconds:  ecsService.HealthCheckGracePeriodSeconds,

			// The family and revision (family:revision) or full ARN of the task definition
			// to run in your service. If a revision is not specified, the latest ACTIVE
			// revision is used. If you modify the task definition with UpdateService, Amazon
			// ECS spawns a task with the new version of the task definition and then stops
			// an old task after the new version is running.
			TaskDefinition: taskDef.TaskDefinitionArn,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update service '%s'",  *ecsService.ServiceName)
		}

		log.Printf("\t%s\tUpdated ECS Service '%s'.\n", tests.Success,  *ecsService.ServiceName)
	} else {

		log.Println("EC2 - Find Subnets")
		var subnetsIDs []string
		var vpcId string
		{
			svc := ec2.New(awsCreds.Session())

			var subnets []*ec2.Subnet
			if len(flags.Ec2SubnetIds) == 0 {
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
			} else {
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
				}
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

			log.Printf("\t%s\tFound %d subnets.\n", len(subnets))
		}

		log.Println("EC2 - Find Security Group")
		{
			svc := ec2.New(awsCreds.Session())

			log.Printf("\t\tFind security group '%s'.\n", flags.Ec2SecurityGroupName)

			var securityGroupId string
			err := svc.DescribeSecurityGroupsPages(&ec2.DescribeSecurityGroupsInput{
				GroupNames: aws.StringSlice([]string{flags.Ec2SecurityGroupName}),
			}, func(res *ec2.DescribeSecurityGroupsOutput, lastPage bool) bool {
				for _, s := range res.SecurityGroups {
					if *s.GroupName == flags.Ec2SecurityGroupName {
						securityGroupId = *s.GroupId
						break
					}

				}
				return !lastPage
			})
			if err != nil {
				return errors.Wrapf(err, "failed to find security group '%s'", flags.Ec2SecurityGroupName)
			}

			if securityGroupId == ""  {
				// If no security group was found, create one.
				_, err = svc.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
					// The name of the security group.
					// Constraints: Up to 255 characters in length. Cannot start with sg-.
					// Constraints for EC2-Classic: ASCII characters
					// Constraints for EC2-VPC: a-z, A-Z, 0-9, spaces, and ._-:/()#,@[]+=&;{}!$*
					// GroupName is a required field
					GroupName : aws.String(flags.Ec2SecurityGroupName),
					// A description for the security group. This is informational only.
					// Constraints: Up to 255 characters in length
					// Constraints for EC2-Classic: ASCII characters
					// Constraints for EC2-VPC: a-z, A-Z, 0-9, spaces, and ._-:/()#,@[]+=&;{}!$*
					// Description is a required field
					Description:              aws.String(fmt.Sprintf("Security group for %s running on ECS cluster %s", flags.ProjectName, awsCreds.EcsClusterName)),
					// [EC2-VPC] The ID of the VPC. Required for EC2-VPC.
					VpcId : aws.String(vpcId),
				})
				if err != nil {
					return errors.Wrapf(err, "failed to create cluster '%s'", awsCreds.EcsClusterName)
				}

				log.Printf("\t\tCreated: %s.", flags.Ec2SecurityGroupName)
			} else {
				log.Printf("\t\tFound: %s.", flags.Ec2SecurityGroupName)
			}

			ingressInputs := []*ec2.AuthorizeSecurityGroupIngressInput{
				// Enable services to be publicly available via HTTP port 80
				&ec2.AuthorizeSecurityGroupIngressInput{
					IpProtocol: aws.String("tcp"),
					CidrIp: aws.String("0.0.0.0/0"),
					FromPort: aws.Int64(80),
					ToPort: aws.Int64(80),
				},
				// Allow all services in the security group to access other services via HTTP port 80.
				&ec2.AuthorizeSecurityGroupIngressInput{
					IpProtocol: aws.String("tcp"),
					SourceSecurityGroupName: aws.String( flags.Ec2SecurityGroupName),
					FromPort: aws.Int64(80),
					ToPort: aws.Int64(80),
				},
			}

			// When we are not using an Elastic Load Balancer, services need to support direct access via HTTPS.
			// HTTPS is terminated via the web server and not on the Load Balancer.
			if !flags.EnableElb {
				// Enable services to be publicly available via HTTPS port 443
				ingressInputs = append(ingressInputs, &ec2.AuthorizeSecurityGroupIngressInput{
					IpProtocol: aws.String("tcp"),
					CidrIp: aws.String("0.0.0.0/0"),
					FromPort: aws.Int64(443),
					ToPort: aws.Int64(443),
				})
				// Allow all services in the security group to access other services via HTTPS port 443.
				ingressInputs = append(ingressInputs, &ec2.AuthorizeSecurityGroupIngressInput{
					IpProtocol: aws.String("tcp"),
					SourceSecurityGroupName: aws.String( flags.Ec2SecurityGroupName),
					FromPort: aws.Int64(443),
					ToPort: aws.Int64(443),
				})
			}

			// Add all the default ingress to the security group.
			for _, ingressInput := range ingressInputs {
				_, err = svc.AuthorizeSecurityGroupIngress(ingressInput)
				if err != nil {
					return errors.Wrapf(err, "failed to add ingress for securuty group '%s'", flags.Ec2SecurityGroupName)
				}
			}


			log.Printf("\t%s\tUsing Security Group '%s'.\n", tests.Success, flags.Ec2SecurityGroupName)
		}

		// If an Elastic Load Balancer is enabled, then ensure one exists else create one.
		var ecsELBs []*ecs.LoadBalancer
		if flags.EnableElb {
			log.Println("EC2 - Find Elastic Load Balance")

			svc := elbv2.New(awsCreds.Session())

			// Set default EBL if needed.
			var maintainELB bool
			if flags.EnableElb && flags.ElbName == "" {
				if !strings.Contains(awsCreds.EcsClusterName, flags.Env) && !strings.Contains(flags.ServiceName, flags.Env) {
					// When a custom cluster name is provided and/or service name, ensure the ELB contains the current env.
					flags.ElbName = fmt.Sprintf("%s-%s-%s", awsCreds.EcsClusterName, flags.ServiceName, flags.Env)
				} else {
					// Default value when when custom cluster/service name is supplied.
					flags.ElbName = fmt.Sprintf("%s-%s", awsCreds.EcsClusterName, flags.ServiceName)
				}

				log.Printf("\t\t\tSet ELB Name to '%s'.", flags.ElbName)

				// When not ELB name is provided and is assigned by us, we should manage the associated target groups
				// and other properties.
				maintainELB = true
			}

			var elb *elbv2.LoadBalancer
			err := svc.DescribeLoadBalancersPages(&elbv2.DescribeLoadBalancersInput{
				Names: []*string{aws.String(flags.ElbName)},
			}, func(res *elbv2.DescribeLoadBalancersOutput, lastPage bool) bool{
				for _, lb := range res.LoadBalancers {
					if *lb.LoadBalancerName == flags.ElbName {
						elb = lb
						return false
					}
				}
				return !lastPage
			})
			if err != nil {
				if aerr, ok := err.(awserr.Error); !ok || aerr.Code() != elbv2.ErrCodeLoadBalancerNotFoundException {
					return errors.Wrapf(err, "failed to describe load balance '%s'", flags.ElbName)
				}
			}

			if elb == nil  {
				// If no repository was found, create one.
				createRes, err := svc.CreateLoadBalancer(&elbv2.CreateLoadBalancerInput{
					// The name of the load balancer.
					// This name must be unique per region per account, can have a maximum of 32
					// characters, must contain only alphanumeric characters or hyphens, must not
					// begin or end with a hyphen, and must not begin with "internal-".
					// Name is a required field
					Name: aws.String(flags.ElbName),
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
					SecurityGroups: aws.StringSlice([]string{flags.Ec2SecurityGroupName}),
					// The IDs of the public subnets. You can specify only one subnet per Availability
					// Zone. You must specify either subnets or subnet mappings.
					// [Application Load Balancers] You must specify subnets from at least two Availability
					// Zones.
					Subnets: aws.StringSlice(subnetsIDs),
					// The type of load balancer.
					Type: aws.String("application"),
					// One or more tags to assign to the load balancer.
					Tags: []*elbv2.Tag{
						&elbv2.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(flags.ProjectName)},
						&elbv2.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(flags.Env)},
					},
				})
				if err != nil {
					return errors.Wrapf(err, "failed to create cluster '%s'", awsCreds.EcsClusterName)
				}
				elb = createRes.LoadBalancers[0]

				log.Printf("\t\tCreated: %s.", *elb.LoadBalancerArn)
			} else {
				log.Printf("\t\tFound: %s.", *elb.LoadBalancerArn)
			}

			// The state code. The initial state of the load balancer is provisioning. After
			// the load balancer is fully set up and ready to route traffic, its state is
			// active. If the load balancer could not be set up, its state is failed.
			log.Printf("\t\t\tState: %s.", *elb.State.Code)

			if maintainELB {
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
						HealthCheckPath: aws.String( "/ping"),

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
					// Default target group for HTTPS via port 443.
					&elbv2.CreateTargetGroupInput{
						Name: aws.String(fmt.Sprintf("%s-https", *elb.LoadBalancerName)),
						Port: aws.Int64(443),
						Protocol: aws.String("HTTPS"),
						HealthCheckEnabled: aws.Bool(true),
						HealthCheckIntervalSeconds: aws.Int64(30),
						HealthCheckPath: aws.String( "/ping"),
						HealthCheckProtocol: aws.String("HTTPS"),
						HealthCheckTimeoutSeconds: aws.Int64(5),
						HealthyThresholdCount: aws.Int64(3),
						UnhealthyThresholdCount: aws.Int64(3),
						Matcher: &elbv2.Matcher{
							HttpCode: aws.String("200"),
						},
						TargetType: aws.String("ip"),
						VpcId: aws.String(vpcId),
					},
				}

				for _, targetGroupInput := range targetGroupInputs {
					var targetGroup *elbv2.TargetGroup
					err = svc.DescribeTargetGroupsPages(&elbv2.DescribeTargetGroupsInput{
						LoadBalancerArn: elb.LoadBalancerArn,
						Names: []*string{aws.String(flags.ElbName)},
					}, func(res *elbv2.DescribeTargetGroupsOutput, lastPage bool) bool{
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

					if targetGroup == nil  {
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
						ContainerName: aws.String(awsCreds.EcsServiceName),
						// The port on the container to associate with the load balancer. This port
						// must correspond to a containerPort in the service's task definition. Your
						// container instances must allow ingress traffic on the hostPort of the port
						// mapping.
						ContainerPort: targetGroup.Port,
						// The full Amazon Resource Name (ARN) of the Elastic Load Balancing target
						// group or groups associated with a service or task set.
						TargetGroupArn: targetGroup.TargetGroupArn,
					})


					if flags.elbDeregistrationDelay != -1 {
						// If no target group was found, create one.
						_, err = svc.ModifyTargetGroupAttributes(&elbv2.ModifyTargetGroupAttributesInput{
							TargetGroupArn: targetGroup.TargetGroupArn,
							Attributes: []*elbv2.TargetGroupAttribute{
								&elbv2.TargetGroupAttribute{
									// The name of the attribute.
									Key: aws.String("deregistration_delay.timeout_seconds"),

									// The value of the attribute.
									Value: aws.String(strconv.Itoa(flags.elbDeregistrationDelay)),
								},
							},
						})
						if err != nil {
							return errors.Wrapf(err, "failed to modify target group '%s' attributes", *targetGroupInput.Name)
						}

						log.Printf("\t\t\tSet sttributes.")
					}
				}
			}

			log.Printf("\t%s\tUsing ELB '%s'.\n", tests.Success, *elb.LoadBalancerName)
		}

		log.Println("ECS - Create Service")
		{

			svc := ecs.New(awsCreds.Session())

			var assignPublicIp *string
			if len(ecsELBs) == 0 {
				assignPublicIp = aws.String("ENABLED")
			} else {
				assignPublicIp = aws.String("DISABLED")
			}


			createRes, err := svc.CreateService(&ecs.CreateServiceInput{
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
				ServiceName: aws.String(awsCreds.EcsServiceName),

				// Optional deployment parameters that control how many tasks run during the
				// deployment and the ordering of stopping and starting tasks.
				DeploymentConfiguration:  &ecs.DeploymentConfiguration{
					// Refer to documentation for flags.ecsServiceMaximumPercent
					MaximumPercent: aws.Int64(int64(flags.ecsServiceMaximumPercent)),
					// Refer to documentation for flags.ecsServiceMinimumHealthyPercent
					MinimumHealthyPercent: aws.Int64(int64(flags.ecsServiceMinimumHealthyPercent)),
				},

				// Refer to documentation for flags.ecsServiceDesiredCount.
				DesiredCount: aws.Int64(int64(flags.ecsServiceDesiredCount)),

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
				HealthCheckGracePeriodSeconds: aws.Int64(int64(flags.escServiceHealthCheckGracePeriodSeconds)),

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
						SecurityGroups: aws.StringSlice([]string{flags.Ec2SecurityGroupName}),

						// The subnets associated with the task or service. There is a limit of 16 subnets
						// that can be specified per AwsVpcConfiguration.
						// All specified subnets must be from the same VPC.
						// Subnets is a required field
						Subnets : aws.StringSlice(subnetsIDs),
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
					&ecs.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(flags.ProjectName)},
					&ecs.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(flags.Env)},
				},
			})
			if err != nil {
				return errors.Wrapf(err, "failed to create service '%s'", awsCreds.EcsServiceName)
			}
			ecsService = createRes.Service

			log.Printf("\t%s\tCreated ECS Service '%s'.\n", tests.Success,  *ecsService.ServiceName)
		}

	}



	// If Elastic cache is enabled, need to add ingress to security group




	return nil
}

