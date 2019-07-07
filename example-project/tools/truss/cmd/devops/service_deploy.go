package devops

import (
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/pkg/errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

/*
secretsmanager:GetSecretValue
ecr:GetAuthorizationToken
ecr:ListImages
ecr:DescribeRepositories
CreateRepository
 */

// requiredCmdsBuild proves a list of required executables for completing build.
var requiredCmdsDeploy = [][]string{
	[]string{"docker", "version", "-f", "{{.Client.Version}}"},
}


// Run is the main entrypoint for deploying a service for a given target env.
func ServiceDeploy(log *log.Logger, projectRoot, targetService, targetEnv, releaseImage, ecsCluster string, enableVpc bool, noBuild, noDeploy, noCache bool) error {

	log.SetPrefix(log.Prefix() + " deploy : ")

	//
	log.Println("Verify required commands are installed.")
	for _, cmdVals := range requiredCmdsDeploy {
		cmd := exec.Command(cmdVals[0], cmdVals[1:]...)
		cmd.Env = os.Environ()

		out, err := cmd.CombinedOutput()
		if err != nil {
			return errors.WithMessagef(err, "failed to execute %s - %s\n%s",  strings.Join(cmdVals, " "), string(out))
		}

		log.Printf("\t%s\t%s - %s", tests.Success, cmdVals[0], string(out))
	}

	// When project root directory is empty or set to current working path, then search for the project root by locating
	// the go.mod file.
	var goModFile string
	if projectRoot == "" || projectRoot == "." {
		log.Println("Attempting to location project root directory from current working directory.")

		var err error
		goModFile, err = findProjectGoModFile()
		if err != nil {
			return err
		}
		projectRoot = filepath.Dir(goModFile)
	} else {
		log.Println("Using supplied project root directory.")
		goModFile = filepath.Join(projectRoot, "go.mod")
	}

	log.Printf("\t\tproject root: %s", projectRoot)
	log.Printf("\t\tgo.mod: %s", goModFile)
	log.Printf("\t%s\tFound project root directory.", tests.Success)

	log.Println("Extracting go module name from go.mod.")
	modName, err := loadGoModName(goModFile)
	if err != nil {
		return err
	}
	log.Printf("\t\tmodule name: %s", modName)
	log.Printf("\t%s\tgo module name.", tests.Success)

	log.Println("Determining the project name.")
	projectName := getTargetEnv(targetEnv, "PROJECT_NAME")
	if projectName != "" {
		log.Printf("\t\tproject name: %s", projectName)
		log.Printf("\t%s\tFound env variable.", tests.Success)
	} else {
		projectName = filepath.Base(modName)
		log.Printf("\t\tproject name: %s", projectName)
		log.Printf("\t%s\tSet from go module.", tests.Success)
	}

	log.Println("Attempting to locate service directory from project root directory.")
	dockerFile, err := findServiceDockerFile(projectRoot, targetService)
	if err != nil {
		return err
	}
	serviceDir := filepath.Dir(dockerFile)

	log.Printf("\t\tservice directory: %s", serviceDir)
	log.Printf("\t\tdockerfile: %s", dockerFile)
	log.Printf("\t%s\tFound service directory.", tests.Success)

	log.Println("Verify release image.")
	var noPull bool
	if releaseImage == "" {
		if v := os.Getenv("CI_REGISTRY_IMAGE"); v != "" {
			releaseImage = fmt.Sprintf("%s:%s-%s", v, targetService, targetEnv)
		} else {
			releaseImage = fmt.Sprintf("%s/%s:latest", targetService, targetEnv)
			noPull = true
		}
	}
	log.Printf("\t\trelease image: %s", releaseImage)
	log.Printf("\t%s\tRelease image valid.", tests.Success)

	log.Println("Verify AWS credentials.")
	awsCreds, err := GetAwsCredentials(targetEnv)
	if err != nil {
		return err
	}

	if ecsCluster == "" {
		ecsCluster = filepath.Base(goModFile) +  "-" + targetEnv
		log.Printf("AWS ECS cluster not set, assigning default value %s.", ecsCluster)
	}

	// Create default service name used for deployment.
	serviceName := targetService + "-" + targetEnv
	_ = serviceName



	awsRepo, err := EcrGetOrCreateRepository(awsCreds, projectName, targetEnv)
	if err != nil {
		return err
	}




	envVars := EnvVars(os.Environ())





	if !noPull {
		log.Println("Retrieve ECR authorization token used for docker login.")
		dockerLogin, err := GetEcrLogin(awsCreds)
		if err != nil {
			return err
		}

		// Login to AWS docker registry and pull release image locally.
		log.Println("Pull Release image from ECR.")
		log.Printf("\t\t%s", releaseImage)
		_, err = execCmds(projectRoot, &envVars, dockerLogin, []string{"docker", "pull", releaseImage})
		if err != nil {
			return err
		}
	}


	return nil
}

// GetDatadogApiKey returns the Datadog API Key which can be either stored in an env var or in AWS Secrets Manager.
func GetDatadogApiKey(targetEnv string, creds *AwsCredentials) (string, error) {

	// 1. Check env vars for [DEV|STAGE|PROD]_DD_API_KEY and DD_API_KEY
	apiKey := getTargetEnv(targetEnv, "DD_API_KEY")
	if apiKey != "" {
		return apiKey, nil
	}

	// 2. Check AWS Secrets Manager for datadog entry prefixed with target env.
	prefixedSecretId := strings.ToUpper(targetEnv) + "/DATADOG"
	var err error
	apiKey, err = GetAwsSecretValue(creds, prefixedSecretId)
	if err != nil {
		if aerr, ok := errors.Cause(err).(awserr.Error); !ok || aerr.Code() != secretsmanager.ErrCodeResourceNotFoundException {
			return "", err
		}
	} else if apiKey != "" {
		return apiKey, nil
	}

	// 3. Check AWS Secrets Manager for datadog entry.
	secretId := "DATADOG"
	apiKey, err = GetAwsSecretValue(creds, secretId)
	if err != nil {
		if aerr, ok := errors.Cause(err).(awserr.Error); !ok || aerr.Code() != secretsmanager.ErrCodeResourceNotFoundException {
			return "", err
		}
	}

	return apiKey, nil
}