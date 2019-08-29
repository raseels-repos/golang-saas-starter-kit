package config

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	"github.com/pkg/errors"
	"gitlab.com/geeks-accelerator/oss/devops/pkg/devdeploy"
)

// Image define the name of an image.
type Image = string

var (
	ImageYourBaseImage Image = "your-base-image"
)

// List of images names used by main.go for help and append the functions to config.
var ImageNames = []Image{
	ImageYourBaseImage,
}

// NewImage returns the *devdeploy.ProjectImage.
func NewImage(imageName string, cfg *devdeploy.Config) (*devdeploy.ProjectImage, error) {

	ctx := &devdeploy.ProjectImage{
		Name:               fmt.Sprintf("%s-%s-%s", cfg.Env, cfg.ProjectName, imageName),
		CodeDir:            filepath.Join(cfg.ProjectRoot, "build/docker", imageName),
		DockerBuildDir:     cfg.ProjectRoot,
		DockerBuildContext: ".",

		// Set the release tag for the image to use include env + function name + commit hash/tag.
		ReleaseTag: devdeploy.GitLabCiReleaseTag(cfg.Env, imageName),
	}

	switch imageName {
	case ImageYourBaseImage:
		// No specific settings.

	default:
		return nil, errors.Wrapf(devdeploy.ErrInvalidFunction,
			"No context defined for image '%s'",
			imageName)
	}

	// Set the docker file if no custom one has been defined for the service.
	if ctx.Dockerfile == "" {
		ctx.Dockerfile = filepath.Join(ctx.CodeDir, "Dockerfile")
	}

	return ctx, nil
}

// BuildImageForTargetEnv executes the build commands for a target image.
func BuildImageForTargetEnv(log *log.Logger, awsCredentials devdeploy.AwsCredentials, targetEnv Env, imageName, releaseTag string, dryRun, noCache, noPush bool) error {

	cfg, err := NewConfig(log, targetEnv, awsCredentials)
	if err != nil {
		return err
	}

	targetImage, err := NewImage(imageName, cfg)
	if err != nil {
		return err
	}

	// Override the release tag if set.
	if releaseTag != "" {
		targetImage.ReleaseTag = releaseTag
	}

	// Append build args to be used for all functions.
	if targetImage.DockerBuildArgs == nil {
		targetImage.DockerBuildArgs = make(map[string]string)
	}

	// funcPath is used to copy the service specific code in the Dockerfile.
	codePath, err := filepath.Rel(cfg.ProjectRoot, targetImage.CodeDir)
	if err != nil {
		return err
	}
	targetImage.DockerBuildArgs["code_path"] = codePath

	if dryRun {
		cfgJSON, err := json.MarshalIndent(cfg, "", "    ")
		if err != nil {
			log.Fatalf("BuildFunctionForTargetEnv : Marshalling config to JSON : %+v", err)
		}
		log.Printf("BuildFunctionForTargetEnv : config : %v\n", string(cfgJSON))

		detailsJSON, err := json.MarshalIndent(targetImage, "", "    ")
		if err != nil {
			log.Fatalf("BuildFunctionForTargetEnv : Marshalling details to JSON : %+v", err)
		}
		log.Printf("BuildFunctionForTargetEnv : details : %v\n", string(detailsJSON))

		return nil
	}

	return devdeploy.BuildImageForTargetEnv(log, cfg, targetImage, noCache, noPush)
}
