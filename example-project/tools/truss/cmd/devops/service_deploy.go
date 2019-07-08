package devops

import (
	"encoding/json"
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type ServiceDeployFlags struct {
	ServiceName string
	ServiceDir string
	Env string
	GoModFile string
	GoModName string
	ProjectRoot  string
	ProjectName string
	DockerFile string
	EcsExecutionRoleArn string
	EcsTaskRoleArn string
	VPC bool
	ELB bool
	SD bool
	ReleaseImage string
	BuildTags []string
	NoBuild  bool
	NoDeploy bool
	NoCache bool
	NoPush bool
	Debug bool
}

func (f *ServiceDeployFlags) ProjectNameCamel() string {
	s := strings.Replace(f.ProjectName, "_", " ", -1)
	s = strings.Replace(s, "-", " ", -1)
	s =  strcase.ToCamel(s)
	return s
}

/*
secretsmanager:GetSecretValue
ecr:GetAuthorizationToken
ecr:ListImages
ecr:DescribeRepositories
ecr:CreateRepository
ecs:CreateCluster
ecs:DescribeClusters
esc:RegisterTaskDefinition
cloudwatchlogs:DescribeLogGroups
cloudwatchlogs:CreateLogGroup
iam:CreateServiceLinkedRole
iam:PutRolePolicy
 */

// requiredCmdsBuild proves a list of required executables for completing build.
var requiredCmdsDeploy = [][]string{
	[]string{"docker", "version", "-f", "{{.Client.Version}}"},
}


// Run is the main entrypoint for deploying a service for a given target env.
func ServiceDeploy(log *log.Logger, flags *ServiceDeployFlags) error {

	log.SetPrefix(log.Prefix() + " deploy : ")

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


	log.Println("Verify flags.")
	var (
		awsCreds *AwsCredentials
		err error
	)
	{
		// When project root directory is empty or set to current working path, then search for the project root by locating
		// the go.mod file.
		if flags.ProjectRoot == "" || flags.ProjectRoot == "." {
			log.Println("\tAttempting to location project root directory from current working directory.")

			var err error
			flags.GoModFile, err = findProjectGoModFile()
			if err != nil {
				return err
			}
			flags.ProjectRoot = filepath.Dir(flags.GoModFile)
		} else {
			log.Println("\t\tUsing supplied project root directory.")
			flags.GoModFile = filepath.Join(flags.ProjectRoot, "go.mod")
		}

		log.Printf("\t\t\tproject root: %s", flags.ProjectRoot)
		log.Printf("\t\t\tgo.mod: %s", flags.GoModFile )
		log.Printf("\t%s\tFound project root directory.", tests.Success)

		log.Println("\tExtracting go module name from go.mod.")
		flags.GoModName, err = loadGoModName(flags.GoModFile )
		if err != nil {
			return err
		}
		log.Printf("\t\t\tmodule name: %s", flags.GoModName)
		log.Printf("\t%s\tgo module name.", tests.Success)

		log.Println("\tDetermining the project name.")
		flags.ProjectName = getTargetEnv(flags.Env, "PROJECT_NAME")
		if flags.ProjectName != "" {
			log.Printf("\t\t\tproject name: %s", flags.ProjectName)
			log.Printf("\t%s\tFound env variable.", tests.Success)
		} else {
			flags.ProjectName = filepath.Base(flags.GoModName)
			log.Printf("\t\t\tproject name: %s", flags.ProjectName)
			log.Printf("\t%s\tSet from go module.", tests.Success)
		}

		log.Println("\tAttempting to locate service directory from project root directory.")
		flags.DockerFile, err = findServiceDockerFile(flags.ProjectRoot, flags.ServiceName)
		if err != nil {
			return err
		}
		flags.ServiceDir = filepath.Dir(flags.DockerFile)

		log.Printf("\t\t\tservice directory: %s", flags.ServiceDir)
		log.Printf("\t\t\tdockerfile: %s", flags.DockerFile)
		log.Printf("\t%s\tFound service directory.", tests.Success)

		log.Println("\tVerify AWS credentials.")
		{
			awsCreds, err = GetAwsCredentials(flags.Env)
			if err != nil {
				return err
			}
			log.Printf("\t\t\tAccessKeyID: %s", awsCreds.AccessKeyID)
			log.Printf("\t\t\tRegion: %s", awsCreds.Region)
			log.Printf("\t\t\tRepository Name: %s", awsCreds.EcrRepositoryName)
			log.Printf("\t%s\tAWS credentials valid.", tests.Success)
		}

		log.Println("\tSet defaults not defined in env vars.")
		{
			// Set default AWS Registry Name if needed.
			if awsCreds.EcrRepositoryName == "" {
				awsCreds.EcrRepositoryName = flags.ProjectName
				log.Printf("\t\t\tSet ECR Repository Name to '%s'.", awsCreds.EcrRepositoryName )
			}

			// Set default AWS Registry Name if needed.
			if awsCreds.EcsClusterName == "" {
				awsCreds.EcsClusterName = flags.ProjectName + "-" + flags.Env
				log.Printf("\t\t\tSet ECS Cluster Name to '%s'.", awsCreds.EcsClusterName )
			}

			// Set default AWS Registry Name if needed.
			if awsCreds.EcsServiceName == "" {
				awsCreds.EcsServiceName = flags.ServiceName + "-" + flags.Env
				log.Printf("\t\t\tSet ECS Service Name to '%s'.", awsCreds.EcsServiceName )
			}

			// Set default AWS Registry Name if needed.
			if awsCreds.CloudWatchLogGroupName == "" {
				awsCreds.CloudWatchLogGroupName = fmt.Sprintf("logs/env_%s/aws/ecs/cluster_%s/service_%s", flags.Env, awsCreds.EcsClusterName, flags.ServiceName)
				log.Printf("\t\t\tSet CloudWatch Log Group Name to '%s'.", awsCreds.CloudWatchLogGroupName )
			}

			log.Printf("\t%s\tDefaults set.", tests.Success)
		}
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
		} else {
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
			"-t", releaseImage,
		}

		// Append the build tags.
		for _, t := range buildTags {
			cmdVals = append(cmdVals, "-t")
			cmdVals = append(cmdVals, releaseImage+":"+t)
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
			log.Printf("\t\tpush release image %s", releaseImage)
			_, err = execCmds(flags.ProjectRoot, &envVars, []string{"docker", "push", releaseImage})
			if err != nil {
				return err
			}

			// Push all the build tags.
			for _, t := range buildTags {
				log.Printf("\t\tpush tag %s", t)
				_, err = execCmds(flags.ProjectRoot, &envVars, []string{"docker", "push", releaseImage + ":" + t})
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
		logGroupCreated, err := CloudWatchLogsGetOrCreateLogGroup(awsCreds, flags.ProjectName, flags.Env, awsCreds.CloudWatchLogGroupName)
		if err != nil {
			return err
		}

		if logGroupCreated {
			log.Printf("\t\tCreated: %s.", awsCreds.CloudWatchLogGroupName)

		} else {
			log.Printf("\t\tFound: %s.", awsCreds.CloudWatchLogGroupName)
		}

		log.Printf("\t%s\tUsing Log Group '%s'.\n", tests.Success, awsCreds.CloudWatchLogGroupName)
	}

	log.Println("ECS - Get or Create Cluster")
	var ecsCluster *ecs.Cluster
	{
		var clusterCreated bool
		ecsCluster, clusterCreated, err = EscGetOrCreateCluster(awsCreds, flags.ProjectName, flags.Env, awsCreds.EcsClusterName)
		if err != nil {
			return err
		}

		if clusterCreated {
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
			"{ECS_CLUSTER}": awsCreds.EcsClusterName,
			"{ECS_SERVICE}": awsCreds.EcsServiceName,
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

		if taskDefInput.Family == nil || *taskDefInput.Family == "" {
			taskDefInput.Family = &flags.ServiceName
		}


		if len(taskDefInput.ContainerDefinitions) > 0 {


			if taskDefInput.ContainerDefinitions[0].Name == nil || *taskDefInput.ContainerDefinitions[0].Name == "" {
				taskDefInput.ContainerDefinitions[0].Name = &awsCreds.EcsServiceName
			}
			if taskDefInput.ContainerDefinitions[0].Image == nil || *taskDefInput.ContainerDefinitions[0].Image == "" {
				taskDefInput.ContainerDefinitions[0].Image = &awsCreds.EcsServiceName
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

		// The task role is the IAM role used by the task itself to access other AWS Services. To access services
		// like S3, SQS, etc then those permissions would need to be covered by the TaskRole.
		if taskDefInput.TaskRoleArn == nil || *taskDefInput.TaskRoleArn == "" {

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

		log.Println("\tRegister new task definition.")
		taskDef, err = EscRegisterTaskDefinition(awsCreds, taskDefInput)
		if err != nil {
			return err
		}
		log.Printf("\t\tRegistered: %s.", *taskDef.TaskDefinitionArn)
		log.Printf("\t\t\tRevision: %d.", *taskDef.Revision)
		log.Printf("\t\t\tStatus: %s.", *taskDef.Status)

		log.Printf("\t%s\tTask definition registered.\n", tests.Success)
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
	if ecsService != nil {
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
		log.Println("ECS - Create Service")

		svc := ecs.New(awsCreds.Session())

	}




	return nil
}

