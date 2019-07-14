package deploy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

const (
	defaultAwsRegistryMaxImages = 1000
	awsTagNameProject           = "project"
	awsTagNameEnv               = "env"
	awsTagNameName              = "Name"
)

func GetAwsCredentials(targetEnv string) (awsCredentials, error) {
	var creds awsCredentials

	if v := getTargetEnv(targetEnv, "AWS_USE_ROLE"); v != "" {
		creds.UseRole, _ = strconv.ParseBool(v)

		sess, err := session.NewSession()
		if err != nil {
			return creds, errors.Wrap(err, "Failed to load AWS credentials from instance")
		}

		if sess.Config != nil && sess.Config.Region != nil {
			creds.Region = *sess.Config.Region
		} else {
			sm := ec2metadata.New(sess)
			creds.Region, err = sm.Region()
			if err != nil {
				return creds, errors.Wrap(err, "Failed to get region from AWS session")
			}
		}

		return creds, nil
	}

	creds.AccessKeyID = strings.TrimSpace(getTargetEnv(targetEnv, "AWS_ACCESS_KEY_ID"))
	creds.SecretAccessKey = strings.TrimSpace(getTargetEnv(targetEnv, "AWS_SECRET_ACCESS_KEY"))
	creds.Region = strings.TrimSpace(getTargetEnv(targetEnv, "AWS_REGION"))

	errs := validator.New().Struct(creds)
	if errs != nil {
		return creds, errs
	}

	//os.Setenv("AWS_DEFAULT_REGION", creds.Region)

	return creds, nil
}

func GetAwsSecretValue(creds awsCredentials, secretId string) (string, error) {
	svc := secretsmanager.New(creds.Session())

	res, err := svc.GetSecretValue(&secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretId),
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to get value for secret id %s", secretId)
	}

	return string(res.SecretBinary), nil
}

// EcrPurgeImages ensures pipeline does not generate images for max of 10000 and prevent manual deletion of images.
func EcrPurgeImages(req *serviceDeployRequest) ([]*ecr.ImageIdentifier, error) {

	svc := ecr.New(req.awsSession())

	// First list all the image IDs for the repository.
	var imgIds []*ecr.ImageIdentifier
	err := svc.ListImagesPages(&ecr.ListImagesInput{
		RepositoryName: aws.String(req.EcrRepositoryName),
	}, func(res *ecr.ListImagesOutput, lastPage bool) bool {
		imgIds = append(imgIds, res.ImageIds...)
		return !lastPage
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list images for repository '%s'", req.EcrRepositoryName)
	}

	var (
		ts       []int
		tsImgIds = map[int][]*ecr.ImageIdentifier{}
	)

	// Describe all the image IDs to determine oldest.
	err = svc.DescribeImagesPages(&ecr.DescribeImagesInput{
		RepositoryName: aws.String(req.EcrRepositoryName),
		ImageIds:       imgIds,
	}, func(res *ecr.DescribeImagesOutput, lastPage bool) bool {
		for _, img := range res.ImageDetails {

			imgTs := int(img.ImagePushedAt.Unix())

			if _, ok := tsImgIds[imgTs]; !ok {
				tsImgIds[imgTs] = []*ecr.ImageIdentifier{}
				ts = append(ts, imgTs)
			}

			if img.ImageTags != nil {
				tsImgIds[imgTs] = append(tsImgIds[imgTs], &ecr.ImageIdentifier{
					ImageTag: img.ImageTags[0],
				})
			} else if img.ImageDigest != nil {
				tsImgIds[imgTs] = append(tsImgIds[imgTs], &ecr.ImageIdentifier{
					ImageDigest: img.ImageDigest,
				})
			}
		}

		return !lastPage
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to describe images for repository '%s'", req.EcrRepositoryName)
	}

	// Sort the image timestamps in reverse order.
	sort.Sort(sort.Reverse(sort.IntSlice(ts)))

	// Loop over all the timestamps, skip the newest images until count exceeds limit.
	var imgCnt int
	var delIds []*ecr.ImageIdentifier
	for _, imgTs := range ts {
		for _, imgId := range tsImgIds[imgTs] {
			imgCnt = imgCnt + 1

			if imgCnt <= req.EcrRepositoryMaxImages {
				continue
			}
			delIds = append(delIds, imgId)
		}
	}

	// If there are image IDs to delete, delete them.
	if len(delIds) > 0 {
		//log.Printf("\t\tECR has %d images for repository '%s' which exceeds limit of %d", imgCnt, creds.EcrRepositoryName, creds.EcrRepositoryMaxImages)
		//for _, imgId := range delIds {
		//	log.Printf("\t\t\tDelete %s", *imgId.ImageTag)
		//}

		_, err = svc.BatchDeleteImage(&ecr.BatchDeleteImageInput{
			ImageIds:       delIds,
			RepositoryName: aws.String(req.EcrRepositoryName),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to delete %d images for repository '%s'", len(delIds), req.EcrRepositoryName)
		}
	}

	return delIds, nil
}

// EcsReadTaskDefinition reads a task definition file and json decodes it.
func EcsReadTaskDefinition(serviceDir, targetEnv string) ([]byte, error) {
	checkPaths := []string{
		filepath.Join(serviceDir, fmt.Sprintf("ecs-task-definition-%s.json", targetEnv)),
		filepath.Join(serviceDir, "ecs-task-definition.json"),
	}

	var defFile string
	for _, tf := range checkPaths {
		ok, _ := exists(tf)
		if ok {
			defFile = tf
			break
		}
	}

	if defFile == "" {
		return nil, errors.Errorf("failed to locate task definition - checked %s", strings.Join(checkPaths, ", "))
	}

	dat, err := ioutil.ReadFile(defFile)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to read file %s", defFile)
	}

	return dat, nil
}

// parseTaskDefinition json decodes it.
func parseTaskDefinitionInput(dat []byte) (*ecs.RegisterTaskDefinitionInput, error) {
	dat = convertKeys(dat)

	var taskDef *ecs.RegisterTaskDefinitionInput
	if err := json.Unmarshal(dat, &taskDef); err != nil {
		return nil, errors.WithMessagef(err, "failed to json decode task definition - %s", string(dat))
	}

	return taskDef, nil
}

// convertKeys fixes json keys to they can be unmarshaled into aws types. No AWS structs have json tags.
func convertKeys(j json.RawMessage) json.RawMessage {
	m := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(j), &m); err != nil {
		// Not a JSON object
		return j
	}

	for k, v := range m {
		fixed := fixKey(k)
		delete(m, k)
		m[fixed] = convertKeys(v)
	}

	b, err := json.Marshal(m)
	if err != nil {
		return j
	}

	return json.RawMessage(b)
}

func fixKey(key string) string {
	return strings.ToTitle(key)
}
