package devops

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/pkg/errors"
)

// DirectoryIterator represents an iterator of a specified directory
type DirectoryIterator struct {
	filePaths []string
	bucket    string
	keyPrefix string
	acl       string
	next      struct {
		path string
		f    *os.File
	}
	err error
}

// NewDirectoryIterator builds a new DirectoryIterator
func NewDirectoryIterator(bucket, keyPrefix, dir, acl string) s3manager.BatchUploadIterator {

	// The key prefix could end with the base directory name,
	// If this is the case, drop the dirname from the key prefix
	if keyPrefix != "" {
		dirName := filepath.Base(dir)
		keyPrefix = strings.TrimRight(keyPrefix, "/")
		keyPrefix = strings.TrimRight(keyPrefix, dirName)
	}

	var paths []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})

	return &DirectoryIterator{
		filePaths: paths,
		bucket:    bucket,
		keyPrefix: keyPrefix,
		acl:       acl,
	}
}

// Next returns whether next file exists or not
func (di *DirectoryIterator) Next() bool {
	if len(di.filePaths) == 0 {
		di.next.f = nil
		return false
	}

	f, err := os.Open(di.filePaths[0])
	di.err = err
	di.next.f = f
	di.next.path = di.filePaths[0]
	di.filePaths = di.filePaths[1:]

	return true && di.Err() == nil
}

// Err returns error of DirectoryIterator
func (di *DirectoryIterator) Err() error {
	return errors.WithStack(di.err)
}

// UploadObject uploads a file
func (di *DirectoryIterator) UploadObject() s3manager.BatchUploadObject {
	f := di.next.f

	var acl *string
	if di.acl != "" {
		acl = aws.String(di.acl)
	}

	// Get file size and read the file content into a buffer
	fileInfo, _ := f.Stat()
	var size int64 = fileInfo.Size()
	buffer := make([]byte, size)
	f.Read(buffer)

	return s3manager.BatchUploadObject{
		Object: &s3manager.UploadInput{
			Bucket:      aws.String(di.bucket),
			Key:         aws.String(filepath.Join(di.keyPrefix, di.next.path)),
			Body:        bytes.NewReader(buffer),
			ContentType: aws.String(http.DetectContentType(buffer)),
			ACL:         acl,
		},
		After: func() error {
			return f.Close()
		},
	}
}
