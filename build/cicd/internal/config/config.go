package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/schema"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/iancoleman/strcase"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gitlab.com/geeks-accelerator/oss/devops/pkg/devdeploy"
)

const (
	// GitLabProjectBaseUrl is the base url used to create links to a specific CI/CD job or pipeline by ID.
	GitLabProjectBaseUrl = "https://gitlab.com/geeks-accelerator/oss/saas-starter-kit"

	// EnableRdsServerless will use the Aurora database engine that scales the capacity based on database load. This is
	// a good option for intermittent or unpredictable workloads.
	EnableRdsServerless = true

	// EnableCloudFront will create a CloudFront distribution (CDN) that is associated with your public bucket.
	// Static asset files will be served by CloudFront instead of from S3 which will improve performance.
	EnableCloudFront = true
)

var (
	// ProjectNamePrefix will be appending to the name of the project.
	ProjectNamePrefix = ""
)

// Env defines the target deployment environment.
type Env = string

var (
	EnvDev   Env = webcontext.Env_Dev
	EnvStage Env = webcontext.Env_Stage
	EnvProd  Env = webcontext.Env_Prod
)

// List of env names used by main.go for help.
var EnvNames = []Env{
	EnvDev,
	EnvStage,
	EnvProd,
}

// init ensures global variables are set correctly.
func init() {
	ProjectNamePrefix = strings.Replace(ProjectNamePrefix, ".", "-", -1)
}

// NewConfig defines the details to setup the target environment for the project to build services and functions.
func NewConfig(log *log.Logger, targetEnv Env, awsCredentials devdeploy.AwsCredentials) (*devdeploy.Config, error) {
	cfg := &devdeploy.Config{
		Env:            targetEnv,
		AwsCredentials: awsCredentials,
	}

	// If AWS Credentials are not set and use role is not enabled, try to load the credentials from env vars.
	if cfg.AwsCredentials.UseRole == false && cfg.AwsCredentials.AccessKeyID == "" {
		var err error
		cfg.AwsCredentials, err = devdeploy.GetAwsCredentialsFromEnv(cfg.Env)
		if err != nil {
			return nil, err
		}
	} else if cfg.AwsCredentials.Region == "" {
		awsCreds, err := devdeploy.GetAwsCredentialsFromEnv(cfg.Env)
		if err != nil {
			return nil, err
		}
		cfg.AwsCredentials.Region = awsCreds.Region
	}

	// Get the current working directory. This should be somewhere contained within the project.
	workDir, err := os.Getwd()
	if err != nil {
		return cfg, errors.Wrap(err, "Failed to get current working directory.")
	}

	// Set the project root directory and project name. This is current set by finding the go.mod file for the project
	// repo. Project name is the directory name.
	modDetails, err := devdeploy.LoadModuleDetails(workDir)
	if err != nil {
		return cfg, err
	}

	// ProjectRoot should be the root directory for the project.
	cfg.ProjectRoot = modDetails.ProjectRoot

	// ProjectName will be used for prefixing AWS resources. This could be changed as needed or manually defined.
	cfg.ProjectName = ProjectNamePrefix + modDetails.ProjectName

	// In a verbatim fork of the repo, a CI/CD would fail due to a conflict creating AWS resources (such as S3) since
	// their name is calculated with the go.mod path. Since the name-scope of AWS resources is region/global scope,
	// it will fail to create appropriate resources for the account of the forked user.
	if cfg.ProjectName == "saas-starter-kit" {
		remoteUser := gitRemoteUser(modDetails.ProjectRoot)

		// Its a true fork from the origin repo.
		if remoteUser != "oss" && remoteUser != "geeks-accelerator" {
			// Replace the prefix 'saas' with the parent directory name, hopefully the gitlab group/username.
			projectPrefix := filepath.Base(filepath.Dir(cfg.ProjectRoot))
			projectPrefix = strings.Replace(projectPrefix, ".", "", -1)
			if len(projectPrefix) > 10 {
				projectPrefix = projectPrefix[0:10]
			}

			cfg.ProjectName = projectPrefix + "-starter-kit"

			log.Println("switching project name to ", cfg.ProjectName)
		}
	}

	// Set default AWS ECR Repository Name.
	cfg.AwsEcrRepository = &devdeploy.AwsEcrRepository{
		RepositoryName: cfg.ProjectName,
		Tags: []devdeploy.Tag{
			{Key: devdeploy.AwsTagNameProject, Value: cfg.ProjectName},
			{Key: devdeploy.AwsTagNameEnv, Value: cfg.Env},
		},
	}

	// Set the deployment to use the default VPC for the region.
	cfg.AwsEc2Vpc = &devdeploy.AwsEc2Vpc{
		IsDefault: true,
	}

	// Set the security group to use for the deployed services, database and cluster. This will used the VPC ID defined
	// for the deployment.
	cfg.AwsEc2SecurityGroup = &devdeploy.AwsEc2SecurityGroup{
		GroupName:   cfg.ProjectName + "-" + cfg.Env,
		Description: fmt.Sprintf("Security group for %s services running on ECS", cfg.ProjectName),
		Tags: []devdeploy.Tag{
			{Key: devdeploy.AwsTagNameProject, Value: cfg.ProjectName},
			{Key: devdeploy.AwsTagNameEnv, Value: cfg.Env},
		},
	}

	// Set the name of the EC2 Security Group used by the gitlab runner. This is used to ensure the security
	// group defined above has access to the RDS cluster/instance and can thus handle schema migrations.
	cfg.GitlabRunnerEc2SecurityGroupName = "gitlab-runner"

	// Set the s3 buckets used by the deployed services.
	// S3 temp prefix used by services for short term storage. A lifecycle policy will be used for expiration.
	s3BucketTempPrefix := "tmp/"

	// Defines a life cycle policy to expire keys for the temp directory.
	bucketLifecycleTempRule := &s3.LifecycleRule{
		ID:     aws.String("Rule for : " + s3BucketTempPrefix),
		Status: aws.String("Enabled"),
		Filter: &s3.LifecycleRuleFilter{
			Prefix: aws.String(s3BucketTempPrefix),
		},
		Expiration: &s3.LifecycleExpiration{
			// Indicates the lifetime, in days, of the objects that are subject to the rule.
			// The value must be a non-zero positive integer.
			Days: aws.Int64(1),
		},
		// Specifies the days since the initiation of an incomplete multipart upload
		// that Amazon S3 will wait before permanently removing all parts of the upload.
		// For more information, see Aborting Incomplete Multipart Uploads Using a Bucket
		// Lifecycle Policy (https://docs.aws.amazon.com/AmazonS3/latest/dev/mpuoverview.html#mpu-abort-incomplete-mpu-lifecycle-config)
		// in the Amazon Simple Storage Service Developer Guide.
		AbortIncompleteMultipartUpload: &s3.AbortIncompleteMultipartUpload{
			DaysAfterInitiation: aws.Int64(1),
		},
	}

	// Define the public S3 bucket used to serve static files for all the services.
	cfg.AwsS3BucketPublic = &devdeploy.AwsS3Bucket{
		BucketName:         cfg.ProjectName + "-public",
		IsPublic:           true,
		TempPrefix:         s3BucketTempPrefix,
		LocationConstraint: &cfg.AwsCredentials.Region,
		LifecycleRules:     []*s3.LifecycleRule{bucketLifecycleTempRule},
		CORSRules: []*s3.CORSRule{
			&s3.CORSRule{
				// Headers that are specified in the Access-Control-Request-Headers header.
				// These headers are allowed in a preflight OPTIONS request. In response to
				// any preflight OPTIONS request, Amazon S3 returns any requested headers that
				// are allowed.
				// AllowedHeaders: aws.StringSlice([]string{}),

				// An HTTP method that you allow the origin to execute. Valid values are GET,
				// PUT, HEAD, POST, and DELETE.
				//
				// AllowedMethods is a required field
				AllowedMethods: aws.StringSlice([]string{"GET", "POST"}),

				// One or more origins you want customers to be able to access the bucket from.
				//
				// AllowedOrigins is a required field
				AllowedOrigins: aws.StringSlice([]string{"*"}),

				// One or more headers in the response that you want customers to be able to
				// access from their applications (for example, from a JavaScript XMLHttpRequest
				// object).
				// ExposeHeaders: aws.StringSlice([]string{}),

				// The time in seconds that your browser is to cache the preflight response
				// for the specified resource.
				// MaxAgeSeconds: aws.Int64(),
			},
		},
	}

	// The base s3 key prefix used to upload static files.
	cfg.AwsS3BucketPublicKeyPrefix = "/public"

	// For production, enable Cloudfront CDN for all static files to avoid serving them from the slower S3 option.
	if EnableCloudFront && cfg.Env == EnvProd {
		cfg.AwsS3BucketPublic.CloudFront = &devdeploy.AwsS3BucketCloudFront{
			// S3 key prefix to request your content from a directory in your Amazon S3 bucket.
			OriginPath: cfg.AwsS3BucketPublicKeyPrefix,

			// A complex type that controls whether CloudFront caches the response to requests.
			CachedMethods: []string{"HEAD", "GET"},

			// The distribution's configuration information.
			DistributionConfig: &cloudfront.DistributionConfig{
				Comment:       aws.String(""),
				Enabled:       aws.Bool(true),
				HttpVersion:   aws.String("http2"),
				IsIPV6Enabled: aws.Bool(true),
				DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
					Compress:   aws.Bool(true),
					DefaultTTL: aws.Int64(1209600),
					MinTTL:     aws.Int64(604800),
					MaxTTL:     aws.Int64(31536000),
					ForwardedValues: &cloudfront.ForwardedValues{
						QueryString: aws.Bool(true),
						Cookies: &cloudfront.CookiePreference{
							Forward: aws.String("none"),
						},
					},
					TrustedSigners: &cloudfront.TrustedSigners{
						Enabled:  aws.Bool(false),
						Quantity: aws.Int64(0),
					},
					ViewerProtocolPolicy: aws.String("allow-all"),
				},
				ViewerCertificate: &cloudfront.ViewerCertificate{
					CertificateSource:            aws.String("cloudfront"),
					MinimumProtocolVersion:       aws.String("TLSv1"),
					CloudFrontDefaultCertificate: aws.Bool(true),
				},
				PriceClass:      aws.String("PriceClass_All"),
				CallerReference: aws.String("devops-deploy" + cfg.AwsS3BucketPublic.BucketName),
			},
		}
	}

	// Define the private S3 bucket used for long term file storage including but not limited to: log exports,
	// AWS Lambda code, application caching.
	cfg.AwsS3BucketPrivate = &devdeploy.AwsS3Bucket{
		BucketName:         cfg.ProjectName + "-private",
		IsPublic:           false,
		TempPrefix:         s3BucketTempPrefix,
		LocationConstraint: &cfg.AwsCredentials.Region,
		LifecycleRules:     []*s3.LifecycleRule{bucketLifecycleTempRule},
		PublicAccessBlock: &s3.PublicAccessBlockConfiguration{
			// Specifies whether Amazon S3 should block public access control lists (ACLs)
			// for this bucket and objects in this bucket. Setting this element to TRUE
			// causes the following behavior:
			//
			//    * PUT Bucket acl and PUT Object acl calls fail if the specified ACL is
			//    public.
			//
			//    * PUT Object calls fail if the request includes a public ACL.
			//
			// Enabling this setting doesn't affect existing policies or ACLs.
			BlockPublicAcls: aws.Bool(true),

			// Specifies whether Amazon S3 should block public bucket policies for this
			// bucket. Setting this element to TRUE causes Amazon S3 to reject calls to
			// PUT Bucket policy if the specified bucket policy allows public access.
			//
			// Enabling this setting doesn't affect existing bucket policies.
			BlockPublicPolicy: aws.Bool(true),

			// Specifies whether Amazon S3 should restrict public bucket policies for this
			// bucket. Setting this element to TRUE restricts access to this bucket to only
			// AWS services and authorized users within this account if the bucket has a
			// public policy.
			//
			// Enabling this setting doesn't affect previously stored bucket policies, except
			// that public and cross-account access within any public bucket policy, including
			// non-public delegation to specific accounts, is blocked.
			RestrictPublicBuckets: aws.Bool(true),

			// Specifies whether Amazon S3 should ignore public ACLs for this bucket and
			// objects in this bucket. Setting this element to TRUE causes Amazon S3 to
			// ignore all public ACLs on this bucket and objects in this bucket.
			//
			// Enabling this setting doesn't affect the persistence of any existing ACLs
			// and doesn't prevent new public ACLs from being set.
			IgnorePublicAcls: aws.Bool(true),
		},
	}

	// Add a bucket policy to enable exports from Cloudwatch Logs for the private S3 bucket.
	cfg.AwsS3BucketPrivate.Policy = func() string {
		policyResource := strings.Trim(filepath.Join(cfg.AwsS3BucketPrivate.BucketName, cfg.AwsS3BucketPrivate.TempPrefix), "/")
		return fmt.Sprintf(`{
				"Version": "2012-10-17",
				"Statement": [
				  {
					  "Action": "s3:GetBucketAcl",
					  "Effect": "Allow",
					  "Resource": "arn:aws:s3:::%s",
					  "Principal": { "Service": "logs.%s.amazonaws.com" }
				  },
				  {
					  "Action": "s3:PutObject" ,
					  "Effect": "Allow",
					  "Resource": "arn:aws:s3:::%s/*",
					  "Condition": { "StringEquals": { "s3:x-amz-acl": "bucket-owner-full-control" } },
					  "Principal": { "Service": "logs.%s.amazonaws.com" }
				  }
				]
			}`, cfg.AwsS3BucketPrivate.BucketName, cfg.AwsCredentials.Region, policyResource, cfg.AwsCredentials.Region)
	}()

	// Define the Redis Cache cluster used for ephemeral storage.
	cfg.AwsElasticCacheCluster = &devdeploy.AwsElasticCacheCluster{
		CacheClusterId:          cfg.ProjectName + "-" + cfg.Env,
		CacheNodeType:           "cache.t2.micro",
		CacheSubnetGroupName:    "default",
		Engine:                  "redis",
		EngineVersion:           "5.0.4",
		NumCacheNodes:           1,
		Port:                    6379,
		AutoMinorVersionUpgrade: aws.Bool(true),
		SnapshotRetentionLimit:  aws.Int64(7),
		ParameterNameValues: []devdeploy.AwsElasticCacheParameter{
			devdeploy.AwsElasticCacheParameter{
				ParameterName:  "maxmemory-policy",
				ParameterValue: "allkeys-lru",
			},
		},
	}

	// If serverless RDS is enabled, defined the RDS database cluster and link it to the database instance.
	if EnableRdsServerless {
		cfg.AwsRdsDBCluster = &devdeploy.AwsRdsDBCluster{
			DBClusterIdentifier:   cfg.ProjectName + "-" + cfg.Env,
			Engine:                "aurora-postgresql",
			EngineMode:            "serverless",
			DatabaseName:          "shared",
			MasterUsername:        "god",
			Port:                  5432,
			BackupRetentionPeriod: aws.Int64(7),
			CopyTagsToSnapshot:    aws.Bool(true),
			Tags: []devdeploy.Tag{
				{Key: devdeploy.AwsTagNameProject, Value: cfg.ProjectName},
				{Key: devdeploy.AwsTagNameEnv, Value: cfg.Env},
			},
			PreCreate: func(input *rds.CreateDBClusterInput) error {
				input.ScalingConfiguration = &rds.ScalingConfiguration{
					// A value that indicates whether to allow or disallow automatic pause for an
					// Aurora DB cluster in serverless DB engine mode. A DB cluster can be paused
					// only when it's idle (it has no connections).
					//
					// If a DB cluster is paused for more than seven days, the DB cluster might
					// be backed up with a snapshot. In this case, the DB cluster is restored when
					// there is a request to connect to it.
					AutoPause: aws.Bool(true),

					// The maximum capacity for an Aurora DB cluster in serverless DB engine mode.
					// Valid capacity values are 1, 2, 4, 8, 16, 32, 64, 128, and 256.
					// The maximum capacity must be greater than or equal to the minimum capacity.
					MaxCapacity: aws.Int64(2),

					// The minimum capacity for an Aurora DB cluster in serverless DB engine mode.
					// Valid capacity values are 1, 2, 4, 8, 16, 32, 64, 128, and 256.
					// The minimum capacity must be less than or equal to the maximum capacity.
					MinCapacity: aws.Int64(2),

					// The time, in seconds, before an Aurora DB cluster in serverless mode is paused.
					SecondsUntilAutoPause: aws.Int64(3600),

					// The action to take when the timeout is reached, either ForceApplyCapacityChange
					// or RollbackCapacityChange.
					// ForceApplyCapacityChange sets the capacity to the specified value as soon
					// as possible.
					// RollbackCapacityChange, the default, ignores the capacity change if a scaling
					// point is not found in the timeout period.
					// If you specify ForceApplyCapacityChange, connections that prevent Aurora
					// Serverless from finding a scaling point might be dropped.
					// For more information, see Autoscaling for Aurora Serverless (https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/aurora-serverless.how-it-works.html#aurora-serverless.how-it-works.auto-scaling)
					// in the Amazon Aurora User Guide.
					TimeoutAction: aws.String("ForceApplyCapacityChange"),
				}

				return nil
			},
			AfterCreate: func(res *rds.DBCluster, dbInfo *devdeploy.DBConnInfo, masterDb *sqlx.DB) error {
				return schema.Migrate(context.Background(), cfg.Env, masterDb, log, false)
			},
		}
	} else {
		// Define the RDS database instance for transactional data. A random password will be generated for any created instance.
		cfg.AwsRdsDBInstance = &devdeploy.AwsRdsDBInstance{
			DBInstanceIdentifier:    cfg.ProjectName + "-" + cfg.Env,
			DBName:                  "shared",
			Engine:                  "postgres",
			MasterUsername:          "god",
			Port:                    5432,
			DBInstanceClass:         "db.t2.small",
			AllocatedStorage:        20,
			PubliclyAccessible:      false,
			BackupRetentionPeriod:   aws.Int64(7),
			AutoMinorVersionUpgrade: true,
			CopyTagsToSnapshot:      aws.Bool(true),
			Tags: []devdeploy.Tag{
				{Key: devdeploy.AwsTagNameProject, Value: cfg.ProjectName},
				{Key: devdeploy.AwsTagNameEnv, Value: cfg.Env},
			},
			AfterCreate: func(res *rds.DBInstance, dbInfo *devdeploy.DBConnInfo, masterDb *sqlx.DB) error {
				return schema.Migrate(context.Background(), cfg.Env, masterDb, log, false)
			},
		}
	}

	// AwsIamPolicy defines the name and policy that will be attached to the task role. The policy document grants
	// the permissions required for deployed services to access AWS services. If the policy already exists, the
	// statements will be used to add new required actions, but not for removal.
	cfg.AwsIamPolicy = &devdeploy.AwsIamPolicy{
		PolicyName:  fmt.Sprintf("%s%sServices", cfg.ProjectNameCamel(), strcase.ToCamel(cfg.Env)),
		Description: fmt.Sprintf("Defines access for %s services. ", cfg.ProjectName),
		PolicyDocument: devdeploy.AwsIamPolicyDocument{
			Version: "2012-10-17",
			Statement: []devdeploy.AwsIamStatementEntry{
				{
					Sid:    "DefaultServiceAccess",
					Effect: "Allow",
					Action: []string{
						"cloudfront:ListDistributions",
						"ec2:DescribeNetworkInterfaces",
						"ec2:DeleteNetworkInterface",
						"ecs:ListTasks",
						"ecs:DescribeServices",
						"ecs:DescribeTasks",
						"ec2:DescribeNetworkInterfaces",
						"route53:ListHostedZones",
						"route53:ListResourceRecordSets",
						"route53:ChangeResourceRecordSets",
						"ecs:UpdateService",
						"ses:SendEmail",
						"ses:ListIdentities",
						"ses:GetAccountSendingEnabled",
						"secretsmanager:ListSecretVersionIds",
						"secretsmanager:GetSecretValue",
						"secretsmanager:CreateSecret",
						"secretsmanager:UpdateSecret",
						"secretsmanager:RestoreSecret",
						"secretsmanager:DeleteSecret",
					},
					Resource: "*",
				},

				{
					Effect: "Allow",
					Action: []string{
						"s3:ListBucket",
					},
					Resource: []string{
						"arn:aws:s3:::" + cfg.AwsS3BucketPublic.BucketName,
						"arn:aws:s3:::" + cfg.AwsS3BucketPrivate.BucketName,
					},
				},
				{
					Effect: "Allow",
					Action: []string{
						"s3:PutObject",
						"s3:PutObjectAcl",
						"s3:GetObject",
					},
					Resource: []string{
						"arn:aws:::" + cfg.AwsS3BucketPublic.BucketName + "/*",
						"arn:aws:::" + cfg.AwsS3BucketPrivate.BucketName + "/*",
					},
				},
				{
					Sid:    "ServiceInvokeLambda",
					Effect: "Allow",
					Action: []string{
						"iam:GetRole",
						"lambda:InvokeFunction",
						"lambda:ListVersionsByFunction",
						"lambda:GetFunction",
						"lambda:InvokeAsync",
						"lambda:GetFunctionConfiguration",
						"iam:PassRole",
						"lambda:GetAlias",
						"lambda:GetPolicy",
					},
					Resource: []string{
						"arn:aws:iam:::role/*",
						"arn:aws:lambda:::function:*",
					},
				},
				{
					Sid:    "datadoglambda",
					Effect: "Allow",
					Action: []string{
						"cloudwatch:Get*",
						"cloudwatch:List*",
						"ec2:Describe*",
						"support:*",
						"tag:GetResources",
						"tag:GetTagKeys",
						"tag:GetTagValues",
					},
					Resource: "*",
				},
			},
		},
	}
	log.Printf("\t\tSet Task Policy Name to '%s'.", cfg.AwsIamPolicy.PolicyName)

	// Append all the defined services to the config.
	for _, n := range ServiceNames {
		srv, err := NewService(log, n, cfg)
		if err != nil {
			return nil, err
		}
		cfg.ProjectServices = append(cfg.ProjectServices, srv)
	}

	// Append all the defined functions to the config.
	for _, n := range FunctionNames {
		fn, err := NewFunction(log, n, cfg)
		if err != nil {
			return nil, err
		}
		cfg.ProjectFunctions = append(cfg.ProjectFunctions, fn)
	}

	return cfg, nil
}

// getDatadogApiKey tries to find the datadog api key from env variable or AWS Secrets Manager.
func getDatadogApiKey(cfg *devdeploy.Config) (string, error) {
	// Load Datadog API key which can be either stored in an environment variable or in AWS Secrets Manager.
	// 1. Check env vars for [DEV|STAGE|PROD]_DD_API_KEY and DD_API_KEY
	apiKey := devdeploy.GetTargetEnv(cfg.Env, "DD_API_KEY")

	// 2. Check AWS Secrets Manager for datadog entry prefixed with target environment.
	if apiKey == "" {
		prefixedSecretId := cfg.SecretID("datadog")
		var err error
		apiKey, err = devdeploy.SecretManagerGetString(cfg.AwsCredentials.Session(), prefixedSecretId)
		if err != nil {
			if errors.Cause(err) != devdeploy.ErrSecreteNotFound {
				return "", err
			}
		}
	}

	// 3. Check AWS Secrets Manager for Datadog entry.
	if apiKey == "" {
		secretId := "datadog"
		var err error
		apiKey, err = devdeploy.SecretManagerGetString(cfg.AwsCredentials.Session(), secretId)
		if err != nil {
			if errors.Cause(err) != devdeploy.ErrSecreteNotFound {
				return "", err
			}
		}
	}

	return apiKey, nil
}

// getCommitRef returns a string that will be used by go build to replace main.go:build constant.
func getCommitRef() string {
	var commitRef string

	// Set the commit ref based on the GitLab CI/CD environment variables.
	if ev := os.Getenv("CI_COMMIT_TAG"); ev != "" {
		commitRef = "tag-" + ev
	} else if ev := os.Getenv("CI_COMMIT_REF_NAME"); ev != "" {
		commitRef = "branch-" + ev
	}

	if commitRef != "" {
		if ev := os.Getenv("CI_COMMIT_SHORT_SHA"); ev != "" {
			commitRef = commitRef + "@" + ev
		}
	}

	return commitRef
}

// gitRemoteUser returns the git username/organization for the git repo
func gitRemoteUser(projectRoot string) string {

	var remoteUrl string
	if ev := os.Getenv("CI_PROJECT_PATH"); ev != "" {
		if strings.Contains(ev, "/") {
			remoteUrl = strings.Split(ev, "/")[1]
		} else {
			remoteUrl = ev
		}
	} else {
		dat, err := ioutil.ReadFile(filepath.Join(projectRoot, ".git/config"))
		if err != nil {
			return ""
		}

		lines := strings.Split(string(dat), "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "url =") {
				remoteUrl = l
				break
			}
		}

		if remoteUrl == "" {
			return ""
		}
		remoteUrl = strings.TrimSpace(strings.Split(remoteUrl, "=")[1])

		if !strings.Contains(remoteUrl, ":") {
			return ""
		}
		remoteUrl = strings.Split(remoteUrl, ":")[1]

	}

	remoteUser := strings.Split(remoteUrl, "/")[0]

	return remoteUser
}

// DeployInfrastructureForTargetEnv executes the deploy commands for a target function.
func DeployInfrastructureForTargetEnv(log *log.Logger, awsCredentials devdeploy.AwsCredentials, targetEnv Env, dryRun bool) error {

	cfg, err := NewConfig(log, targetEnv, awsCredentials)
	if err != nil {
		return err
	}

	if dryRun {
		cfgJSON, err := json.MarshalIndent(cfg, "", "    ")
		if err != nil {
			log.Fatalf("DeployFunctionForTargetEnv : Marshalling config to JSON : %+v", err)
		}
		log.Printf("DeployFunctionForTargetEnv : config : %v\n", string(cfgJSON))

		return nil
	}

	_, err = devdeploy.SetupInfrastructure(log, cfg, devdeploy.SetupOptionSkipCache)
	if err != nil {
		return err
	}

	return nil
}
