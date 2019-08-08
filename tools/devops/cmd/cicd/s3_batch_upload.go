package cicd

import (
	"bytes"
	"mime"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/pkg/errors"
)

// DirectoryIterator represents an iterator of a specified directory
type DirectoryIterator struct {
	dir       string
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

	var paths []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})

	return &DirectoryIterator{
		dir:       dir,
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


	buffer, contentType, rerr :=  readFile(f)

	nextPath, _ := filepath.Rel(di.dir, di.next.path)

	return s3manager.BatchUploadObject{
		Object: &s3manager.UploadInput{
			Bucket:      aws.String(di.bucket),
			Key:         aws.String(filepath.Join(di.keyPrefix, nextPath)),
			Body:        bytes.NewReader(buffer),
			ContentType: aws.String(contentType),
			ACL:         acl,
		},
		After: func() error {
			if rerr != nil {
				return rerr
			}
			return f.Close()
		},
	}
}

func readFile(f *os.File) ([]byte, string, error) {
	// Get file size and read the file content into a buffer
	fileInfo, err := f.Stat()
	if err != nil {
		panic(err)
		return nil, "", err
	}
	var size int64 = fileInfo.Size()
	buffer := make([]byte, size)
	f.Read(buffer)

	ext := filepath.Ext(f.Name())
	contentType := mime.TypeByExtension(ext)

	//f.Seek(0, io.SeekStart)
	//ctBuf := make([]byte, 512)
	//f.Read(ctBuf)
	//contentType = http.DetectContentType(ctBuf)

	return buffer, contentType, nil
}
