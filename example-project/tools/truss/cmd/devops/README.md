
1. Create new policy `saas-starter-kit-deploy` with the following permissions. 
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ServiceDeployPermissions",    
      "Effect": "Allow",
      "Action": [
        "cloudwatchlogs:DescribeLogGroups",
	    "cloudwatchlogs:CreateLogGroup",
        "ec2:DescribeSubnets",
        "ec2:DescribeSubnets",
        "ec2:DescribeSecurityGroups",
        "ec2:CreateSecurityGroup",
        "ec2:AuthorizeSecurityGroupIngress",
        "elasticloadbalancing:DescribeLoadBalancers",
        "elasticloadbalancing:CreateLoadBalancer",
        "elasticloadbalancing:DescribeTargetGroups",
        "elasticloadbalancing:CreateTargetGroup",
        "elasticloadbalancing:DescribeListeners",
        "elasticloadbalancing:ModifyTargetGroupAttributes",
        "ecs:CreateCluster",
        "ecs:CreateService",
        "ecs:DescribeClusters",
        "ecs:DescribeServices",
        "ecs:UpdateService",
        "ecs:RegisterTaskDefinition",
        "ecs:ListTaskDefinitions",
        "ecr:BatchDeleteImage",
        "ecr:GetAuthorizationToken",
        "ecr:DescribeImages",
	    "ecr:DescribeRepositories",
	    "ecr:CreateRepository",
	    "ecr:ListImages",
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
        "secretsmanager:ListSecrets",
        "secretsmanager:GetSecretValue"
      ],
      "Resource": "*"
    }
  ]
}
```

2. Create new user `saas-starter-kit-deploy` with _Programmatic Access_ and _Attach existing policies directly_ with the policy created from step 1 `saas-starter-kit-deploy`

3. Try running the deploy
```bash
go run main.go deploy -service=web-api -env=dev
```




 