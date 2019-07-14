package cicd

import (
	"encoding/base64"
	"log"
	"path/filepath"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

// ServiceBuildFlags defines the flags used for executing a service build.
type ServiceBuildFlags struct {
	// Required flags.
	ServiceName string `validate:"required" example:"web-api"`
	Env         string `validate:"oneof=dev stage prod" example:"dev"`

	// Optional flags.
	ProjectRoot string `validate:"omitempty" example:"."`
	ProjectName string ` validate:"omitempty" example:"example-project"`
	DockerFile  string `validate:"omitempty" example:"./cmd/web-api/Dockerfile"`
	NoCache     bool   `validate:"omitempty" example:"false"`
	NoPush      bool   `validate:"omitempty" example:"false"`
}

// serviceBuildRequest defines the details needed to execute a service build.
type serviceBuildRequest struct {
	*serviceRequest

	EcrRepositoryName      string `validate:"required"`
	EcrRepository          *ecr.CreateRepositoryInput
	EcrRepositoryMaxImages int `validate:"omitempty"`

	NoCache bool `validate:"omitempty"`
	NoPush  bool `validate:"omitempty"`

	flags ServiceBuildFlags
}

// NewServiceBuildRequest generates a new request for executing build of a single service for a given set of CLI flags.
func NewServiceBuildRequest(log *log.Logger, flags ServiceBuildFlags) (*serviceBuildRequest, error) {

	// Validates specified CLI flags map to struct successfully.
	log.Println("Validate flags.")
	{
		errs := validator.New().Struct(flags)
		if errs != nil {
			return nil, errs
		}
		log.Printf("\t%s\tFlags ok.", tests.Success)
	}

	// Generate a deploy request using CLI flags and AWS credentials.
	log.Println("Generate deploy request.")
	var req serviceBuildRequest
	{
		// Define new service request.
		sr := &serviceRequest{
			ServiceName: flags.ServiceName,
			Env:         flags.Env,
			ProjectRoot: flags.ProjectRoot,
			ProjectName: flags.ProjectName,
			DockerFile:  flags.DockerFile,
		}
		if err := sr.init(log); err != nil {
			return nil, err
		}

		req = serviceBuildRequest{
			serviceRequest: sr,

			NoCache: flags.NoCache,
			NoPush:  flags.NoPush,

			flags: flags,
		}

		// Set default AWS ECR Repository Name.
		req.EcrRepositoryName = ecrRepositoryName(req.ProjectName)
		req.EcrRepository = &ecr.CreateRepositoryInput{
			RepositoryName: aws.String(req.EcrRepositoryName),
			Tags: []*ecr.Tag{
				&ecr.Tag{Key: aws.String(awsTagNameProject), Value: aws.String(req.ProjectName)},
				&ecr.Tag{Key: aws.String(awsTagNameEnv), Value: aws.String(req.Env)},
			},
		}
		log.Printf("\t\t\tSet ECR Repository Name to '%s'.", req.EcrRepositoryName)

		// Set default AWS ECR Regsistry Max Images.
		req.EcrRepositoryMaxImages = defaultAwsRegistryMaxImages
		log.Printf("\t\t\tSet ECR Regsistry Max Images to '%d'.", req.EcrRepositoryMaxImages)

	}

	return &req, nil
}

// Run is the main entrypoint for building a service for a given target environment.
func ServiceBuild(log *log.Logger, req *serviceBuildRequest) error {

	// Load the AWS ECR repository. Try to find by name else create new one.
	var dockerLoginCmd []string
	{
		log.Println("ECR - Get or create repository.")

		svc := ecr.New(req.awsSession())

		// First try to find ECR repository by name.
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
			createRes, err := svc.CreateRepository(req.EcrRepository)
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

			// Since ECR has max number of repository images, need to delete old ones so can stay under limit.
			// If there are image IDs to delete, delete them.
			if len(delIds) > 0 {
				log.Printf("\t\tDeleted %d images that exceeded limit of %d", len(delIds), req.EcrRepositoryMaxImages)
				for _, imgId := range delIds {
					log.Printf("\t\t\t%s", *imgId.ImageTag)
				}
			}
		}

		req.ReleaseImage = releaseImage(req.Env, req.ServiceName, *awsRepo.RepositoryUri)
		if err != nil {
			return err
		}

		log.Printf("\t\trelease image: %s", req.ReleaseImage)
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

		dockerLoginCmd = []string{
			"docker",
			"login",
			"-u", user,
			"-p", pass,
			*res.AuthorizationData[0].ProxyEndpoint,
		}

		log.Printf("\t%s\tdocker login ok.", tests.Success)
	}

	// Once we can access the repository in ECR, do the docker build.
	{
		log.Println("Starting docker build")

		dockerFile, err := filepath.Rel(req.ProjectRoot, req.DockerFile)
		if err != nil {
			return errors.Wrapf(err, "Failed parse relative path for %s from %s", req.DockerFile, req.ProjectRoot)
		}

		// The initial build command slice.
		buildCmd := []string{
			"docker", "build",
			"--file=" + dockerFile,
			"--build-arg", "service=" + req.ServiceName,
			"--build-arg", "env=" + req.Env,
			"-t", req.ReleaseImage,
		}

		// Append additional build flags.
		if req.NoCache {
			buildCmd = append(buildCmd, "--no-cache")
		}

		// Finally append the build context as the current directory since os.Exec will use the project root as
		// the working directory.
		buildCmd = append(buildCmd, ".")

		err = execCmds(log, req.ProjectRoot, buildCmd)
		if err != nil {
			return errors.Wrap(err, "Failed to build docker image")
		}

		// Push the newly built image of the Docker container to the registry.
		if req.NoPush == false {

			log.Printf("\t\tDocker Login")
			err = execCmds(log, req.ProjectRoot, dockerLoginCmd)
			if err != nil {
				return errors.Wrapf(err, "Failed to login to AWS ECR")
			}

			log.Printf("\t\tPush release image %s", req.ReleaseImage)
			err = execCmds(log, req.ProjectRoot, []string{"docker", "push", req.ReleaseImage})
			if err != nil {
				return errors.Wrapf(err, "Failed to push docker image %s", req.ReleaseImage)
			}
		}

		log.Printf("\t%s\tbuild complete.\n", tests.Success)
	}

	return nil
}
