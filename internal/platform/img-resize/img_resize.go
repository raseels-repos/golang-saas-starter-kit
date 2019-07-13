package img_resize

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/nfnt/resize"
	"github.com/pkg/errors"
	"github.com/sethgrid/pester"
	redistrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/go-redis/redis"
)

// S3ImgUrl parses the original url from an srcset
func S3ImgUrl(ctx context.Context, redisClient *redistrace.Client, s3UrlFormatter func(string) string, awsSession *session.Session, s3Bucket, S3KeyPrefix, p string, size int) (string, error) {
	src, err := S3ImgSrc(ctx, redisClient, s3UrlFormatter, awsSession, s3Bucket, S3KeyPrefix, p, []int{size}, true)
	if err != nil {
		return "", err
	}

	var imgUrl string
	if strings.Contains(src, "srcset=\"") {
		imgUrl = strings.Split(src, "srcset=\"")[1]
		imgUrl = strings.Trim(strings.Split(imgUrl, ",")[0], "\"")
	} else if strings.Contains(src, "src=\"") {
		imgUrl = strings.Split(src, "src=\"")[1]
		imgUrl = strings.Trim(strings.Split(imgUrl, ",")[0], "\"")
	} else {
		imgUrl = src
	}

	if strings.Contains(imgUrl, " ") {
		imgUrl = strings.Split(imgUrl, " ")[0]
	}

	return imgUrl, nil
}

// S3ImgSrc returns an srcset for a given image url and defined sizes
// Format the local image path to the fully qualified image URL,
// on stage and prod the app will not have access to the local image
// files if App.StaticS3 is enabled.
func S3ImgSrc(ctx context.Context, redisClient *redistrace.Client, s3UrlFormatter func(string) string, awsSession *session.Session, s3Bucket, s3KeyPrefix, imgUrlStr string, sizes []int, includeOrig bool) (string, error) {

	// Default return value on error.
	defaultSrc := fmt.Sprintf(`src="%s"`, imgUrlStr)

	// Only fully qualified image URLS are supported. On dev the app host should
	// still be included as this lacks the concept of the static directory.
	if !strings.HasPrefix(imgUrlStr, "http") {
		return defaultSrc, nil
	}

	// Extract the image path from the URL.
	imgUrl, err := url.Parse(imgUrlStr)
	if err != nil {
		return defaultSrc, errors.WithStack(err)
	}

	// Determine the file extension for the image path.
	pts := strings.Split(imgUrl.Path, ".")
	filExt := strings.ToLower(pts[len(pts)-1])
	if filExt == "jpg" {
		filExt = ".jpg"
	} else if filExt == "jpeg" {
		filExt = ".jpeg"
	} else if filExt == "gif" {
		filExt = ".gif"
	} else if filExt == "png" {
		filExt = ".png"
	} else {
		return defaultSrc, nil
	}

	// Cache Key used by Redis for storing the resulting image src to avoid having to
	// regenerate on each page load.
	data := []byte(fmt.Sprintf("S3ImgSrc:%s:%v:%v", imgUrlStr, sizes, includeOrig))
	ck := fmt.Sprintf("%x", md5.Sum(data))

	// Check redis for the cache key.
	var imgSrc string
	cv, err := redisClient.WithContext(ctx).Get(ck).Result()
	if err != nil {
		// TODO: log the error as a warning
	} else if len(cv) > 0 {
		imgSrc = string(cv)
	}

	if imgSrc == "" {
		// Make the http request to retrieve the image.
		res, err := pester.Get(imgUrl.String())
		if err != nil {
			return imgSrc, errors.WithStack(err)
		}
		defer res.Body.Close()

		// Validate the http status is OK and request did not fail.
		if res.StatusCode != http.StatusOK {
			err = errors.Errorf("Request failed with statusCode %v for %s", res.StatusCode, imgUrlStr)
			return defaultSrc, errors.WithStack(err)
		}

		// Read all the image bytes.
		dat, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return defaultSrc, errors.WithStack(err)
		}

		//if hv, ok := res.Request.Response.Header["Last-Modified"]; ok && len(hv) > 0 {
		//	// Expires: Sun, 03 May 2015 23:02:37 GMT
		//	http.ParseTime(hv[0])
		//}

		// s3Path is the base s3 key to store all the associated resized images.
		// Store the by the image host + path
		s3Path := filepath.Join(s3KeyPrefix, fmt.Sprintf("%x", md5.Sum([]byte(imgUrl.Host+imgUrl.Path))))

		// baseImgName is the base image filename
		// Extract the image filename from the url
		baseImgName := filepath.Base(imgUrl.Path)

		// If the image has a query string, append md5 and append to s3Path
		if len(imgUrl.Query()) > 0 {
			qh := fmt.Sprintf("%x", md5.Sum([]byte(imgUrl.Query().Encode())))
			s3Path = s3Path + "q" + qh

			// Update the base image name to include the query string hash
			pts := strings.Split(baseImgName, ".")
			if len(pts) >= 2 {
				pts[len(pts)-2] = pts[len(pts)-2] + "-" + qh
				baseImgName = strings.Join(pts, ".")
			} else {
				baseImgName = baseImgName + "-" + qh
			}
		}

		// checkSum is used to determine if the contents of the src file changed.
		var checkSum string

		// Try to pull a value from the response headers to be used as a checksum
		if hv, ok := res.Header["ETag"]; ok && len(hv) > 0 {
			// ETag: "5485fac7-ae74"
			checkSum = strings.Trim(hv[0], "\"")
		} else if hv, ok := res.Header["Last-Modified"]; ok && len(hv) > 0 {
			// Last-Modified: Mon, 08 Dec 2014 19:23:51 GMT
			checkSum = fmt.Sprintf("%x", md5.Sum([]byte(hv[0])))
		} else {
			checkSum = fmt.Sprintf("%x", md5.Sum(dat))
		}

		// Append the checkSum to the s3Path
		s3Path = filepath.Join(s3Path, checkSum)

		// Init new CloudFront using provided AWS session.
		s3srv := s3.New(awsSession)

		// List all the current images that exist on s3 for the s3 path.
		// New files will have none until they are generated below and uploaded.
		listRes, err := s3srv.ListObjects(&s3.ListObjectsInput{
			Bucket: aws.String(s3Bucket),
			Prefix: aws.String(s3Path),
		})
		if err != nil {
			return defaultSrc, errors.WithStack(err)
		}

		// Loop through all the S3 objects and store by in map by
		// filename with its current lastModified time
		curFiles := make(map[string]time.Time)
		if listRes != nil && listRes.Contents != nil {
			for _, obj := range listRes.Contents {
				fname := filepath.Base(*obj.Key)
				curFiles[fname] = obj.LastModified.UTC()
			}
		}

		pts := strings.Split(baseImgName, ".")
		var uidx int
		if len(pts) >= 2 {
			uidx = len(pts) - 2
		}

		var maxSize int
		expFiles := make(map[int]string)
		for _, s := range sizes {
			spts := pts
			spts[uidx] = fmt.Sprintf("%s-%dw", spts[uidx], s)

			nname := strings.Join(spts, ".")
			expFiles[s] = nname

			if s > maxSize {
				maxSize = s
			}
		}

		renderFiles := make(map[int]string)
		for s, fname := range expFiles {
			if _, ok := curFiles[fname]; !ok {
				// Image does not exist, render
				renderFiles[s] = fname
			}
		}

		if len(renderFiles) > 0 {
			uploader := s3manager.NewUploaderWithClient(s3srv, func(d *s3manager.Uploader) {
				//d.PartSize = s.UploadPartSize
				//d.Concurrency = s.UploadConcurrency
			})

			for s, fname := range renderFiles {
				// Render new image with specified width, height of
				// of 0 will preserve the current aspect ratio.
				var (
					contentType string
					uploadBytes []byte
				)
				if filExt == ".gif" {
					contentType = "image/gif"
					uploadBytes, err = ResizeGif(dat, uint(s), 0)
				} else if filExt == ".png" {
					contentType = "image/png"
					uploadBytes, err = ResizePng(dat, uint(s), 0)
				} else {
					contentType = "image/jpeg"
					uploadBytes, err = ResizeJpg(dat, uint(s), 0)
				}
				if err != nil {
					return defaultSrc, errors.WithStack(err)
				}

				// The s3 key for the newly resized image file.
				renderedS3Key := filepath.Join(s3Path, fname)

				// Upload the s3 key with the resized image bytes.
				p := &s3manager.UploadInput{
					Bucket: aws.String(s3Bucket),
					Key:    aws.String(renderedS3Key),
					Body:   bytes.NewReader(uploadBytes),
					Metadata: map[string]*string{
						"Content-Type":  aws.String(contentType),
						"Cache-Control": aws.String("max-age=604800"),
					},
				}
				_, err = uploader.Upload(p)
				if err != nil {
					return defaultSrc, errors.WithStack(err)
				}

				// Grant public read access to the uploaded image file.
				_, err = s3srv.PutObjectAcl(&s3.PutObjectAclInput{
					Bucket: aws.String(s3Bucket),
					Key:    aws.String(renderedS3Key),
					ACL:    aws.String("public-read"),
				})
				if err != nil {
					return defaultSrc, errors.WithStack(err)
				}
			}
		}

		// Determine the current width of the image, don't need height since will be using
		// maintain the current aspect ratio.
		lw, _, err := getImageDimension(dat)
		if includeOrig {
			if lw > maxSize && (!strings.HasPrefix(imgUrlStr, "http") || strings.HasPrefix(imgUrlStr, "https:")) {
				maxSize = lw
				sizes = append(sizes, lw)
			}
		} else {
			maxSize = sizes[len(sizes)-1]
		}

		sort.Ints(sizes)

		var srcUrl string
		srcSets := []string{}
		srcSizes := []string{}
		for _, s := range sizes {
			var nu string
			if lw == s {
				nu = imgUrlStr
			} else {
				fname := expFiles[s]
				nk := filepath.Join(s3Path, fname)
				nu = s3UrlFormatter(nk)
			}

			srcSets = append(srcSets, fmt.Sprintf("%s %dw", nu, s))
			if s == maxSize {
				srcSizes = append(srcSizes, fmt.Sprintf("%dpx", s))
				srcUrl = nu
			} else {
				srcSizes = append(srcSizes, fmt.Sprintf("(max-width: %dpx) %dpx", s, s))
			}
		}

		imgSrc = fmt.Sprintf(`srcset="%s" sizes="%s" src="%s"`, strings.Join(srcSets, ","), strings.Join(srcSizes, ","), srcUrl)
	}

	err = redisClient.WithContext(ctx).Set(ck, imgSrc, 0).Err()
	if err != nil {
		return imgSrc, errors.WithStack(err)
	}

	return imgSrc, nil
}

// ResizeJpg resizes a JPG image file to specified width and height using
// lanczos resampling and preserving the aspect ratio.
func ResizeJpg(dat []byte, width, height uint) ([]byte, error) {
	// decode jpeg into image.Image
	img, err := jpeg.Decode(bytes.NewReader(dat))
	if err != nil {
		return []byte{}, errors.WithStack(err)
	}

	// resize to width 1000 using Lanczos resampling
	// and preserve aspect ratio
	m := resize.Resize(width, height, img, resize.NearestNeighbor)

	// write new image to file
	var out = new(bytes.Buffer)
	jpeg.Encode(out, m, nil)

	return out.Bytes(), nil
}

// ResizeGif resizes a GIF image file to specified width and height using
// lanczos resampling and preserving the aspect ratio.
func ResizeGif(dat []byte, width, height uint) ([]byte, error) {
	// decode gif into image.Image
	img, err := gif.Decode(bytes.NewReader(dat))
	if err != nil {
		return []byte{}, errors.WithStack(err)
	}

	// resize to width 1000 using Lanczos resampling
	// and preserve aspect ratio
	m := resize.Resize(width, height, img, resize.NearestNeighbor)

	// write new image to file
	var out = new(bytes.Buffer)
	gif.Encode(out, m, nil)

	return out.Bytes(), nil
}

// ResizePng resizes a PNG image file to specified width and height using
// lanczos resampling and preserving the aspect ratio.
func ResizePng(dat []byte, width, height uint) ([]byte, error) {
	// decode png into image.Image
	img, err := png.Decode(bytes.NewReader(dat))
	if err != nil {
		return []byte{}, errors.WithStack(err)
	}

	// resize to width 1000 using Lanczos resampling
	// and preserve aspect ratio
	m := resize.Resize(width, height, img, resize.NearestNeighbor)

	// write new image to file
	var out = new(bytes.Buffer)
	png.Encode(out, m)

	return out.Bytes(), nil
}

// getImageDimension returns the width and height for a given local file path
func getImageDimension(dat []byte) (int, int, error) {
	image, _, err := image.DecodeConfig(bytes.NewReader(dat))
	if err != nil {
		return 0, 0, errors.WithStack(err)
	}
	return image.Width, image.Height, nil
}
