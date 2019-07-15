package cicd

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
)

// serviceDeployRequest defines the details needed to execute a service deployment.
type serviceRequest struct {
	ServiceName string `validate:"required"`
	ServiceDir  string `validate:"required"`
	Env         string `validate:"oneof=dev stage prod"`
	ProjectRoot string `validate:"required"`
	ProjectName string `validate:"required"`
	DockerFile  string `validate:"required"`
	GoModFile   string `validate:"required"`
	GoModName   string `validate:"required"`

	AwsCreds    awsCredentials `validate:"required,dive,required"`
	_awsSession *session.Session

	ReleaseImage string
}

// projectNameCamel takes a project name and returns the camel cased version.
func (r *serviceRequest) ProjectNameCamel() string {
	s := strings.Replace(r.ProjectName, "_", " ", -1)
	s = strings.Replace(s, "-", " ", -1)
	s = strcase.ToCamel(s)
	return s
}

// awsSession returns the current AWS session for the serviceDeployRequest.
func (r *serviceRequest) awsSession() *session.Session {
	if r._awsSession == nil {
		r._awsSession = r.AwsCreds.Session()
	}

	return r._awsSession
}

// init sets the basic details needed for both build and deploy for serviceRequest.
func (req *serviceRequest) init(log *log.Logger) error {
	// When project root directory is empty or set to current working path, then search for the project root by locating
	// the go.mod file.
	log.Println("\tDetermining the project root directory.")
	{
		if req.ProjectRoot == "" || req.ProjectRoot == "." {
			log.Println("\tAttempting to location project root directory from current working directory.")

			var err error
			req.GoModFile, err = findProjectGoModFile()
			if err != nil {
				return err
			}
			req.ProjectRoot = filepath.Dir(req.GoModFile)
		} else {
			log.Printf("\t\tUsing supplied project root directory '%s'.\n", req.ProjectRoot)
			req.GoModFile = filepath.Join(req.ProjectRoot, "go.mod")
		}
		log.Printf("\t\t\tproject root: %s", req.ProjectRoot)
		log.Printf("\t\t\tgo.mod: %s", req.GoModFile)
	}

	log.Println("\tExtracting go module name from go.mod.")
	{
		var err error
		req.GoModName, err = loadGoModName(req.GoModFile)
		if err != nil {
			return err
		}
		log.Printf("\t\t\tmodule name: %s", req.GoModName)
	}

	log.Println("\tDetermining the project name.")
	{
		if req.ProjectName != "" {
			log.Printf("\t\tUse provided value.")
		} else {
			req.ProjectName = filepath.Base(req.GoModName)
			log.Printf("\t\tSet from go module.")
		}
		log.Printf("\t\t\tproject name: %s", req.ProjectName)
	}

	log.Println("\tAttempting to locate service directory from project root directory.")
	{
		if req.DockerFile != "" {
			req.DockerFile = req.DockerFile
			log.Printf("\t\tUse provided value.")

		} else {
			log.Printf("\t\tFind from project root looking for Dockerfile.")
			var err error
			req.DockerFile, err = findServiceDockerFile(req.ProjectRoot, req.ServiceName)
			if err != nil {
				return err
			}
		}

		req.ServiceDir = filepath.Dir(req.DockerFile)

		log.Printf("\t\t\tservice directory: %s", req.ServiceDir)
		log.Printf("\t\t\tdockerfile: %s", req.DockerFile)
	}

	// Verifies AWS credentials specified as environment variables.
	log.Println("\tVerify AWS credentials.")
	{
		var err error
		req.AwsCreds, err = GetAwsCredentials(req.Env)
		if err != nil {
			return err
		}
		if req.AwsCreds.UseRole {
			log.Printf("\t\t\tUsing role")
		} else {
			log.Printf("\t\t\tAccessKeyID: '%s'", req.AwsCreds.AccessKeyID)
		}

		log.Printf("\t\t\tRegion: '%s'", req.AwsCreds.Region)
		log.Printf("\t%s\tAWS credentials valid.", tests.Success)
	}

	return nil
}

// ecrRepositoryName returns the name used for the AWS ECR Repository.
func ecrRepositoryName(projectName string) string {
	return projectName
}

// releaseImage returns the name used for tagging a release image will always include one with environment and
// service name. If the env var CI_COMMIT_REF_NAME is set, it will be appended.
func releaseTag(env, serviceName string) string {

	tag1 := env + "-" + serviceName

	// Generate tags for the release image.
	var releaseTag string
	if v := os.Getenv("BUILDINFO_CI_COMMIT_SHA"); v != "" {
		tag2 := tag1 + "-" + v[0:8]
		releaseTag = tag2
	} else if v := os.Getenv("CI_COMMIT_SHA"); v != "" {
		tag2 := tag1 + "-" + v[0:8]
		releaseTag = tag2
	} else if v := os.Getenv("BUILDINFO_CI_COMMIT_REF_NAME"); v != "" {
		tag2 := tag1 + "-" + v
		releaseTag = tag2
	} else if v := os.Getenv("CI_COMMIT_REF_NAME"); v != "" {
		tag2 := tag1 + "-" + v
		releaseTag = tag2
	} else {
		releaseTag = tag1
	}
	return releaseTag
}

// releaseImage returns the name used for tagging a release image will always include one with environment and
// service name. If the env var CI_COMMIT_REF_NAME is set, it will be appended.
func releaseImage(env, serviceName, repositoryUri string) string {
	return repositoryUri + ":" + releaseTag(env, serviceName)
}

// dBInstanceIdentifier returns the database name.
func dBInstanceIdentifier(projectName, env string) string {
	return projectName + "-" + env
}

// secretID returns the secret name with a standard prefix.
func secretID(projectName, env, secretName string) string {
	return filepath.Join(projectName, env, secretName)
}

// findProjectGoModFile finds the project root directory from the current working directory.
func findProjectGoModFile() (string, error) {
	var err error
	projectRoot, err := os.Getwd()
	if err != nil {
		return "", errors.WithMessage(err, "failed to get current working directory")
	}

	// Try to find the project root for looking for the go.mod file in a parent directory.
	var goModFile string
	testDir := projectRoot
	for i := 0; i < 3; i++ {
		if goModFile != "" {
			testDir = filepath.Join(testDir, "../")
		}
		goModFile = filepath.Join(testDir, "go.mod")
		ok, _ := exists(goModFile)
		if ok {
			projectRoot = testDir
			break
		}
	}

	// Verify the go.mod file was found.
	ok, err := exists(goModFile)
	if err != nil {
		return "", errors.WithMessagef(err, "failed to load go.mod for project using project root %s")
	} else if !ok {
		return "", errors.Errorf("failed to locate project go.mod in project root %s", projectRoot)
	}

	return goModFile, nil
}

// findServiceDockerFile finds the service directory.
func findServiceDockerFile(projectRoot, targetService string) (string, error) {
	checkDirs := []string{
		filepath.Join(projectRoot, "cmd", targetService),
		filepath.Join(projectRoot, "tools", targetService),
	}

	var dockerFile string
	for _, cd := range checkDirs {
		// Check to see if directory contains Dockerfile.
		tf := filepath.Join(cd, "Dockerfile")

		ok, _ := exists(tf)
		if ok {
			dockerFile = tf
			break
		}
	}

	if dockerFile == "" {
		return "", errors.Errorf("failed to locate Dockerfile for service %s", targetService)
	}

	return dockerFile, nil
}

// getTargetEnv checks for an env var that is prefixed with the current target env.
func getTargetEnv(targetEnv, envName string) string {
	k := fmt.Sprintf("%s_%s", strings.ToUpper(targetEnv), envName)

	if v := os.Getenv(k); v != "" {
		// Set the non prefixed env var with the prefixed value.
		os.Setenv(envName, v)
		return v
	}

	return os.Getenv(envName)
}

// loadGoModName parses out the module name from go.mod.
func loadGoModName(goModFile string) (string, error) {
	ok, err := exists(goModFile)
	if err != nil {
		return "", errors.WithMessage(err, "Failed to load go.mod for project")
	} else if !ok {
		return "", errors.Errorf("Failed to locate project go.mod at %s", goModFile)
	}

	b, err := ioutil.ReadFile(goModFile)
	if err != nil {
		return "", errors.WithMessagef(err, "Failed to read go.mod at %s", goModFile)
	}

	var name string
	lines := strings.Split(string(b), "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "module ") {
			name = strings.TrimSpace(strings.Split(l, " ")[1])
			break
		}
	}

	return name, nil
}

// exists returns a bool as to whether a file path exists.
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

// execCmds executes a set of commands using the current env variables.
func execCmds(log *log.Logger, workDir string, cmds ...[]string) error {
	for _, cmdVals := range cmds {
		cmd := exec.Command(cmdVals[0], cmdVals[1:]...)
		cmd.Dir = workDir
		cmd.Env = os.Environ()

		cmd.Stderr = log.Writer()
		cmd.Stdout = log.Writer()

		err := cmd.Run()

		if err != nil {
			return errors.WithMessagef(err, "failed to execute %s", strings.Join(cmdVals, " "))
		}
	}

	return nil
}
