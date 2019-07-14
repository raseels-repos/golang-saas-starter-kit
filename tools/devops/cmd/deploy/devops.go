package deploy

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

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
func execCmds(log *log.Logger, workDir string, cmds ...[]string) (error) {
	for _, cmdVals := range cmds {
		cmd := exec.Command(cmdVals[0], cmdVals[1:]...)
		cmd.Dir = workDir
		cmd.Env = os.Environ()

		cmd.Stderr = log.Writer()
		cmd.Stdout = log.Writer()

		err := cmd.Run()

		if err != nil {
			return  errors.WithMessagef(err, "failed to execute %s",  strings.Join(cmdVals, " "))
		}
	}

	return  nil
}
