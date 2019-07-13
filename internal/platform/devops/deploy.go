package devops

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// SyncS3StaticFiles copies the local files from the static directory to s3
// with public-read enabled.
func SyncS3StaticFiles(awsSession *session.Session, staticS3Bucket, staticS3Prefix, staticDir string) error {
	uploader := s3manager.NewUploader(awsSession)

	di := NewDirectoryIterator(staticS3Bucket, staticS3Prefix, staticDir, "public-read")
	if err := uploader.UploadWithIterator(aws.BackgroundContext(), di); err != nil {
		return err
	}

	return nil
}
