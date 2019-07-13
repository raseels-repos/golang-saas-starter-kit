
1. Create new policy `saas-starter-kit-deploy` with the following permissions. 
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ServiceDeployPermissions",    
      "Effect": "Allow",
      "Action": [
        "acm:ListCertificates",
        "acm:RequestCertificate",
        "acm:DescribeCertificate",
        "ec2:DescribeSubnets",
        "ec2:DescribeSecurityGroups",
        "ec2:CreateSecurityGroup",
        "ec2:AuthorizeSecurityGroupIngress",
        "ec2:DescribeNetworkInterfaces",
        "ec2:DescribeVpcs",
        "ec2:CreateVpc",
        "ec2:CreateSubnet",
        "ec2:DescribeVpcs",
        "ec2:DescribeInternetGateways",
        "ec2:CreateInternetGateway",
        "ec2:CreateTags",
        "ec2:CreateRouteTable",
        "ec2:DescribeRouteTables",
        "ec2:CreateRoute",
        "ec2:AttachInternetGateway",
        "ec2:DescribeAccountAttributes",
        "elasticache:DescribeCacheClusters",
        "elasticache:CreateCacheCluster",
        "elasticache:DescribeCacheParameterGroups",
        "elasticache:CreateCacheParameterGroup",
        "elasticache:ModifyCacheCluster",
        "elasticache:ModifyCacheParameterGroup",
        "elasticloadbalancing:DescribeLoadBalancers",
        "elasticloadbalancing:CreateLoadBalancer",
        "elasticloadbalancing:CreateListener",
        "elasticloadbalancing:DescribeTargetGroups",
        "elasticloadbalancing:CreateTargetGroup",
        "elasticloadbalancing:DescribeListeners",
        "elasticloadbalancing:ModifyTargetGroupAttributes",
        "ecs:CreateCluster",
        "ecs:CreateService",
        "ecs:DeleteService",
        "ecs:DescribeClusters",
        "ecs:DescribeServices",
        "ecs:UpdateService",
        "ecs:RegisterTaskDefinition",
        "ecs:ListTaskDefinitions",
        "ecr:BatchCheckLayerAvailability",
        "ecr:BatchDeleteImage",
        "ecr:GetAuthorizationToken",
        "ecr:DescribeImages",
	    "ecr:DescribeRepositories",
	    "ecs:DescribeTasks",
	    "ecr:CreateRepository",
	    "ecr:ListImages",
	    "ecs:ListTasks",
	    "ecr:PutImage",
	    "ecr:InitiateLayerUpload",
	    "ecr:UploadLayerPart",
	    "ecr:CompleteLayerUpload",
        "logs:DescribeLogGroups",
        "logs:CreateLogGroup",
        "lambda:ListFunctions",
        "lambda:CreateFunction",
        "lambda:UpdateFunctionCode",
        "lambda:UpdateFunctionConfiguration",
        "iam:GetRole",
        "iam:PassRole",
        "iam:CreateRole",
        "iam:CreateServiceLinkedRole",
        "iam:CreatePolicy",
	    "iam:PutRolePolicy",
	    "iam:TagRole",
	    "iam:AttachRolePolicy",
	    "iam:ListPolicies",
	    "iam:GetPolicyVersion",
	    "iam:CreatePolicyVersion",
        "logs:DescribeLogGroups",
        "logs:CreateLogGroup",
	    "logs:DescribeLogStreams",
	    "logs:CreateExportTask",
	    "logs:DescribeExportTasks",
	    "rds:CreateDBCluster",
	    "rds:CreateDBInstance",
	    "rds:DescribeDBClusters",
	    "rds:DescribeDBInstances",
	    "s3:CreateBucket",
	    "s3:DeleteObject",
        "s3:DeleteObjectVersion",
        "s3:GetBucketPublicAccessBlock",
        "s3:GetBucketAcl",
	    "s3:HeadBucket",
	    "s3:ListObjects",
	    "s3:ListBucket",
	    "s3:GetObject",
	    "s3:PutLifecycleConfiguration",
	    "s3:PutBucketCORS",
	    "s3:PutBucketPolicy",
        "s3:PutBucketPublicAccessBlock",
        "route53:CreateHostedZone",
        "route53:ChangeResourceRecordSets",
        "route53:ListHostedZones",
        "secretsmanager:CreateSecret",
        "secretsmanager:ListSecrets",
        "secretsmanager:GetSecretValue",
        "secretsmanager:UpdateSecret",
        "secretsmanager:RestoreSecret",
        "secretsmanager:DeleteSecret",
        "servicediscovery:ListNamespaces",
        "servicediscovery:CreatePrivateDnsNamespace",
        "servicediscovery:GetOperation",
        "servicediscovery:ListServices",
        "servicediscovery:CreateService",
        "servicediscovery:GetService"
      ],
      "Resource": "*"
    },
    {
        "Action": "iam:CreateServiceLinkedRole",
        "Effect": "Allow",
        "Resource": "arn:aws:iam::*:role/aws-service-role/rds.amazonaws.com/AWSServiceRoleForRDS",
        "Condition": {
            "StringLike": {
                "iam:AWSServiceName":"rds.amazonaws.com"
            }
        }
    }
  ]
}
```

2. Create new user `saas-starter-kit-deploy` with _Programmatic Access_ and _Attach existing policies directly_ with the policy created from step 1 `saas-starter-kit-deploy`

3. Try running the deploy
```bash
go run main.go deploy -service=web-api -env=dev
```




 