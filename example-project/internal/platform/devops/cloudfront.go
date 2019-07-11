package devops

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/pkg/errors"
)

func CloudFrontDistribution(awsSession *session.Session, s3Bucket string) (*cloudfront.DistributionSummary, error) {
	// Init new CloudFront using provided AWS session.
	cloudFront := cloudfront.New(awsSession)

	// Loop through all the cloudfront distributions and find the one that matches the
	// S3 Bucket name. AWS doesn't current support multiple distributions per bucket
	// so this should always be a one to one match.
	var distribution *cloudfront.DistributionSummary
	err := cloudFront.ListDistributionsPages(&cloudfront.ListDistributionsInput{},
		func(page *cloudfront.ListDistributionsOutput, lastPage bool) bool {
			if page.DistributionList != nil {
				for _, v := range page.DistributionList.Items {
					if v.DomainName == nil || v.Origins == nil || v.Origins.Items == nil {
						continue
					}

					for _, o := range v.Origins.Items {
						if o.DomainName == nil || !strings.HasPrefix(*o.DomainName, s3Bucket+".") {
							continue
						}

						distribution = v
						break
					}

					if distribution != nil {
						break
					}
				}
			}

			if distribution != nil {
				return false
			}

			return !lastPage
		},
	)
	if err != nil {
		return nil, err
	}

	if distribution == nil {
		return nil, errors.Errorf("aws cloud front deployment does not exist for s3 bucket %s.", s3Bucket)
	}

	return distribution, nil
}

// NewAuthenticator creates an *Authenticator for use.
// key expiration is optional to filter out old keys
// It will error if:
// - The aws session is nil.
// - The aws s3 bucket is blank.
func S3UrlFormatter(awsSession *session.Session, s3Bucket, s3KeyPrefix string, enableCloudFront bool) (func(string) string, error) {
	if awsSession == nil {
		return nil, errors.New("aws session cannot be nil")
	}

	if s3Bucket == "" {
		return nil, errors.New("aws s3 bucket cannot be empty")
	}

	var (
		baseS3Url    string
		baseS3Origin string
	)
	if enableCloudFront {
		dist, err := CloudFrontDistribution(awsSession, s3Bucket)
		if err != nil {
			return nil, err
		}

		// Format the domain as an HTTPS url, "dzuyel7n94hma.cloudfront.net"
		baseS3Url = fmt.Sprintf("https://%s/", *dist.DomainName)

		// The origin used for the cloudfront needs to be striped from the path
		// provided, the URL shouldn't have one, but "/public"
		baseS3Origin = *dist.Origins.Items[0].OriginPath
	} else {
		// The static files are upload to a specific prefix, so need to ensure
		// the path reference includes this prefix
		s3Path := filepath.Join(s3Bucket, s3KeyPrefix)

		if *awsSession.Config.Region == "us-east-1" {
			// US East (N.Virginia) region endpoint, http://s3.amazonaws.com/bucket or
			// http://s3-external-1.amazonaws.com/bucket/
			baseS3Url = fmt.Sprintf("https://s3.amazonaws.com/%s/", s3Path)
		} else {
			// Region-specific endpoint, http://s3-aws-region.amazonaws.com/bucket
			baseS3Url = fmt.Sprintf("https://s3-%s.amazonaws.com/%s/", *awsSession.Config.Region, s3Path)
		}

		baseS3Origin = s3KeyPrefix
	}

	f := func(p string) string {
		return S3Url(baseS3Url, baseS3Origin, p)
	}

	return f, nil
}

// S3Url formats a path to include either the S3 URL or a CloudFront
// URL instead of serving the file from local file system.
func S3Url(baseS3Url, baseS3Origin, p string) string {
	// If its already a URL, then don't format it
	if strings.HasPrefix(p, "http") {
		return p
	}

	// Drop the beginning forward slash
	p = strings.TrimLeft(p, "/")

	// In the case of cloudfront, the base URL may not match S3,
	// removing the origin from the path provided
	// ie. The s3 bucket + path of
	//		gitw-corp-web.s3.amazonaws.com/public
	// 		maps to dzuyel7n94hma.cloudfront.net
	//      where the path prefix of '/public' needs to be dropped.
	org := strings.Trim(baseS3Origin, "/")
	if org != "" {
		p = strings.Replace(p, org+"/", "", 1)
	}

	// Parse out the querystring from the path
	var pathQueryStr string
	if strings.Contains(p, "?") {
		pts := strings.Split(p, "?")
		p = pts[0]
		if len(pts) > 1 {
			pathQueryStr = pts[1]
		}
	}

	u, err := url.Parse(baseS3Url)
	if err != nil {
		return "?"
	}
	ldir := filepath.Base(u.Path)

	if strings.HasPrefix(p, ldir) {
		p = strings.Replace(p, ldir+"/", "", 1)
	}

	u.Path = filepath.Join(u.Path, p)
	u.RawQuery = pathQueryStr

	return u.String()
}
