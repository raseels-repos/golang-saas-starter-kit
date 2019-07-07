package devops

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"github.com/pkg/errors"
)

// requiredCmdsBuild proves a list of required executables for completing build.
var requiredCmdsBuild = [][]string{
	[]string{"docker", "version", "-f", "{{.Client.Version}}"},
}

// Run is the main entrypoint for building a service for a given target env.
func ServiceBuild(log *log.Logger, projectRoot, targetService, targetEnv, releaseImage string, noPush, noCache bool) error {

	log.SetPrefix(log.Prefix() + " build : ")

	//
	log.Println("Verify required commands are installed.")
	for _, cmdVals := range requiredCmdsBuild {
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
	if releaseImage == "" {
		if v := os.Getenv("CI_REGISTRY_IMAGE"); v != "" {
			releaseImage = fmt.Sprintf("%s:%s-%s", v, targetService, targetEnv)
		} else {
			releaseImage = fmt.Sprintf("%s/%s:latest", targetService, targetEnv)
			noPush = true
		}
	}
	log.Printf("\t\trelease image: %s", releaseImage)
	log.Printf("\t%s\tRelease image valid.", tests.Success)

	// Load the AWS
	log.Println("Verify AWS credentials.")
	awsCreds, err := GetAwsCredentials(targetEnv)
	if err != nil {
		return err
	}

	log.Printf("\t\tAccessKeyID: %s", awsCreds.AccessKeyID)
	log.Printf("\t\tRegion: %s", awsCreds.Region)

	// Set default AWS Registry Name if needed.
	if awsCreds.RepositoryName == "" {
		awsCreds.RepositoryName = projectName
		log.Printf("\t\tSetting Repository Name to Project Name.")
	}
	log.Printf("\t\tRepository Name: %s", awsCreds.RepositoryName)
	log.Printf("\t%s\tAWS credentials valid.", tests.Success)

	// Pull the current env variables to be passed in for command execution.
	envVars := EnvVars(os.Environ())

	// Do the docker build.
	{
		cmdVals := []string{
			"docker",
			"build",
			"--file=" + dockerFile,
			"--build-arg", "service=" + targetService,
			"--build-arg", "env=" + targetEnv,
			"-t", releaseImage,
		}

		if noCache {
			cmdVals = append(cmdVals, "--no-cache")
		}
		cmdVals = append(cmdVals, ".")

		log.Printf("starting docker build: \n\t%s\n", strings.Join(cmdVals, " "))
		out, err := execCmds(projectRoot, &envVars, cmdVals)
		if err != nil {
			return err
		}
		log.Printf("build complete\n\t%s\n", string(out[0]))
	}

	// Push the newly built docker container to the registry.
	if !noPush {
		log.Println("Push release image.")
		_, err = execCmds(projectRoot, &envVars, []string{"docker", "push", releaseImage})
		if err != nil {
			return err
		}
	}


	if awsCreds.RepositoryName != "" {
		awsRepo, err := EcrGetOrCreateRepository(awsCreds, projectName, targetEnv)
		if err != nil {
			return err
		}

		maxImages := defaultAwsRegistryMaxImages
		if v := getTargetEnv(targetEnv, "AWS_REPOSITORY_MAX_IMAGES"); v != "" {
			maxImages, err = strconv.Atoi(v)
			if err != nil {
				return errors.WithMessagef(err, "Failed to parse max ECR images")
			}
		}

		log.Println("Purging old ECR images.")
		err = EcrPurgeImages(log, awsCreds, maxImages)
		if err != nil {
			return err
		}

		log.Println("Retrieve ECR authorization token used for docker login.")
		dockerLogin, err := GetEcrLogin(awsCreds)
		if err != nil {
			return err
		}

		awsRegistryImage := *awsRepo.RepositoryUri

		// Login to AWS docker registry and pull release image locally.
		log.Println("Push release image to ECR.")
		log.Printf("\t\t%s", awsRegistryImage)
		_, err = execCmds(projectRoot, &envVars, dockerLogin, []string{"docker", "tag", releaseImage, awsRegistryImage}, []string{"docker", "push", awsRegistryImage})
		if err != nil {
			return err
		}

		tag1 := targetEnv+"-"+targetService
		log.Printf("\t\ttagging as %s", tag1)
		_, err = execCmds(projectRoot, &envVars, []string{"docker", "tag", releaseImage, awsRegistryImage+":"+tag1}, []string{"docker", "push", awsRegistryImage+":"+tag1})
		if err != nil {
			return err
		}

		if v := os.Getenv("CI_COMMIT_REF_NAME"); v != "" {
			tag2 := tag1 + "-" + v
			log.Printf("\t\ttagging as %s", tag2)
			_, err = execCmds(projectRoot, &envVars, []string{"docker", "tag", releaseImage, awsRegistryImage+ ":" + tag2}, []string{"docker", "push", awsRegistryImage + ":" + tag2})
			if err != nil {
				return err
			}
		}
	}

	return nil
}
