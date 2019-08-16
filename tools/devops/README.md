# SaaS Starter Kit

Copyright 2019, Geeks Accelerator 
twins@geeksaccelerator.com


## Description

_Devops_ handles creating AWS resources and deploying your services with minimal additional configuration. You can 
customizing any of the configuration in the code. While AWS is already a core part of the saas-starter-kit, keeping 
the deployment in GoLang limits the scope of additional technologies required to get your project successfully up and 
running. If you understand Golang, then you will be a master at devops with this tool.

The project includes a Postgres database which adds an additional resource dependency when deploying the 
project. It is important to know that the tasks running schema migration for the Postgres database can not run as shared 
GitLab Runners since they will be outside the deployment AWS VPC. There are two options here: 
1. Enable the AWS RDS database to be publicly available (not recommended).
2. Run your own GitLab runners inside the same AWS VPC and grant access for them to communicate with the database.

This project has opted to implement option 2 and thus setting up the deployment pipeline requires a few more additional steps. 

Note that using shared runners hosted by GitLab also requires AWS credentials to be input into GitLab for configuration.  

Hosted your own GitLab runners uses AWS Roles instead of hardcoding the access key ID and secret access key in GitLab and 
in other configuration files. And since this project is open-source, we wanted to avoid sharing our AWS credentials.

If you don't have an AWS account, signup for one now and then proceed with the deployment setup. 

We assume that if you are deploying the SaaS Starter Kit, you are starting from scratch with no existing dependencies. 
This however, excludes any domain names that you would like to use for resolving your services publicly. To use any 
pre-purchased domain names, make sure they are added to Route 53 in the AWS account. Or you can let the deploy script 
create a new zone is Route 53 and update the DNS for the domain name when your ready to make the transition. It is 
required to hosted the DNS on Route 53 so DNS entries can be managed by this deploy tool. It is possible to use a 
[subdomain that uses Route 53 as the DNS service without migrating the parent domain](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/CreatingNewSubdomain.html). 


## Getting Started

You can run the both commands `build` and `deploy` locally after setting up the initial 
AWS permissions. 

1. You will need an existing AWS account or create a new AWS account.

2. Define a new [AWS IAM Policy](https://console.aws.amazon.com/iam/home?region=us-west-2#/policies$new?step=edit) 
called `saas-starter-kit-deploy` with a defined JSON statement instead of using the visual 
editor. The statement is rather large as each permission is granted individually. A copy of 
the statement is stored in the repo at 
[resources/saas-starter-kit-deploy-policy.json](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/blob/master/resources/saas-starter-kit-deploy-policy.json)

3. Create new [AWS User](https://console.aws.amazon.com/iam/home?region=us-west-2#/users$new?step=details) 
called `saas-starter-kit-deploy` with _Programmatic Access_ and _Attach existing policies directly_ with the policy 
created from step 1 `saas-starter-kit-deploy`

4. Try running the deploy
```bash
go run main.go deploy -service=web-api -env=dev
```

Note: This user created is only for development purposes and is not needed for the build 
pipeline using GitLab CI / CD.
 
 
## Setup GitLab CI / CD

Below outlines the basic steps to setup [Autoscaling GitLab Runner on AWS](https://docs.gitlab.com/runner/configuration/runner_autoscale_aws/). 

1. Define an [AWS IAM Role](https://console.aws.amazon.com/iam/home?region=us-west-2#/roles$new?step=type) that will be
attached to the GitLab Runner instances. The role will need permission to scale (EC2), update the cache (via S3) and 
perform the project specific deployment commands.
    ```
    Trusted Entity: AWS Service
    Service that will use this role: EC2 
    Attach permissions policies:  AmazonEC2FullAccess, AmazonS3FullAccess, saas-starter-kit-deploy 
    Role Name: SaasStarterKitEc2RoleForGitLabRunner
    Role Description: Allows GitLab runners hosted on EC2 instances to call AWS services on your behalf.
    ``` 

2. Launch a new [AWS EC2 Instance](https://us-west-2.console.aws.amazon.com/ec2/v2/home?region=us-west-2#LaunchInstanceWizard). 
`GitLab Runner` will be installed on this instance and will serve as the bastion that spawns new instances. This 
instance will be a dedicated host since we need it always up and running, thus it will be the standard costs apply. 

    Note: Since this machine will not run any jobs itself, it does not need to be very powerful. A t2.micro instance will be sufficient.
    ``` 
    Amazon Machine Image (AMI): Amazon Linux AMI 2018.03.0 (HVM), SSD Volume Type - ami-0f2176987ee50226e
    Instance Type: t2.micro 
    ``` 

3. Configure Instance Details. 

    Note: Do not forget to select the IAM Role _SaasStarterKitEc2RoleForGitLabRunner_ 
    ```
    Number of instances: 1
    Network: default VPC
    Subnet: no Preference
    Auto-assign Public IP: Use subnet setting (Enable)
    Placement Group: not checked/disabled
    Capacity Reservation: Open
    IAM Role: SaasStarterKitEc2RoleForGitLabRunner
    Shutdown behavior: Stop
    Enable termination project: checked/enabled
    Monitoring: not checked/disabled
    Tenancy: Shared
    Elastic Interence: not checked/disabled
    T2/T3 Unlimited: not checked/disabled
    Advanced Details: none 
    ```
    
4. Add Storage. Increase the volume size for the root device to 30 GiB.
    ```    
    Volume Type |   Device      | Size (GiB) |  Volume Type 
    Root        |   /dev/xvda   | 30        |  General Purpose SSD (gp2)
    ```

5. Add Tags.
    ```
    Name:  gitlab-runner 
    ``` 
    
6. Configure Security Group. Create a new security group with the following details:
    ``` 
    Name: gitlab-runner
    Description: Gitlab runners for running CICD.
    Rules:                       
        Type        | Protocol  | Port Range    | Source    | Description
        SSH         | TCP       | 22            | My IP     | SSH access for setup.                        
    ```        
    
7. Review and Launch instance. Select an existing key pair or create a new one. This will be used to SSH into the 
    instance for additional configuration. 
     
8. Update the security group to reference itself. The instances need to be able to communicate between each other. 

    Navigate to edit the security group and add the following two rules where `SECURITY_GROUP_ID` is replaced with the 
    name of the security group created in step 6.
    ``` 
    Rules:                       
        Type        | Protocol  | Port Range    | Source            | Description
        Custom TCP  | TCP       | 2376          | SECURITY_GROUP_ID | Gitlab runner for Docker Machine to communicate with Docker daemon.
        SSH         | TCP       | 22            | SECURITY_GROUP_ID | SSH access for setup.                        
    ```     
    
8. SSH into the newly created instance. 

    ```bash
    ssh -i ~/saas-starter-kit-uswest2-gitlabrunner.pem ec2-user@ec2-52-36-105-172.us-west-2.compute.amazonaws.com
    ``` 
     Note: If you get the error `Permissions 0666 are too open`, then you will need to `chmod 400 FILENAME`
       
9. Install GitLab Runner from the [official GitLab repository](https://docs.gitlab.com/runner/install/linux-repository.html)
    ```bash 
    curl -L https://packages.gitlab.com/install/repositories/runner/gitlab-runner/script.rpm.sh | sudo bash
    sudo yum install gitlab-runner
    ``` 
    
10. [Install Docker Community Edition](https://docs.docker.com/install/).
    ```bash 
    sudo yum install docker
    ```
    
11. [Install Docker Machine](https://docs.docker.com/machine/install-machine/).
    ```bash
    base=https://github.com/docker/machine/releases/download/v0.16.0 &&
      curl -L $base/docker-machine-$(uname -s)-$(uname -m) >/tmp/docker-machine &&
      sudo install /tmp/docker-machine /usr/sbin/docker-machine
    ```
    
12. [Register the runner](https://docs.gitlab.com/runner/register/index.html).
    ```bash
    sudo gitlab-runner register
    ```    
    Notes: 
    * When asked for gitlab-ci tags, enter `master,dev,dev-*`
        * This will limit commits to the master or dev branches from triggering the pipeline to run. This includes a 
        wildcard for any branch named with the prefix `dev-`.
    * When asked the executor type, enter `docker+machine`
    * When asked for the default Docker image, enter `geeksaccelerator/docker-library:golang1.12-docker`
        
13. [Configuring the GitLab Runner](https://docs.gitlab.com/runner/configuration/runner_autoscale_aws/#configuring-the-gitlab-runner)   

    ```bash
    sudo vim /etc/gitlab-runner/config.toml
    ``` 
    
    Update the `[runners.docker]` configuration section in `config.toml` to match the example below replacing the 
    obvious placeholder `XXXXX` with the relevant value. 
    ```yaml
      [runners.docker]
        tls_verify = false
        image = "geeksaccelerator/docker-library:golang1.12-docker"
        privileged = true
        disable_entrypoint_overwrite = false
        oom_kill_disable = false
        disable_cache = true
        volumes = ["/cache"]
        shm_size = 0
      [runners.cache]
        Type = "s3"
        Shared = true
        [runners.cache.s3]
          ServerAddress = "s3.us-west-2.amazonaws.com"
          BucketName = "XXXXX"
          BucketLocation = "us-west-2"
      [runners.machine]
        IdleCount = 0
        IdleTime = 1800
        MachineDriver = "amazonec2"
        MachineName = "gitlab-runner-machine-%s"
        MachineOptions = [
          "amazonec2-iam-instance-profile=SaasStarterKitEc2RoleForGitLabRunner",
          "amazonec2-region=us-west-2",
          "amazonec2-vpc-id=XXXXX",
          "amazonec2-subnet-id=XXXXX",
          "amazonec2-zone=d",
          "amazonec2-use-private-address=true",
          "amazonec2-tags=runner-manager-name,gitlab-aws-autoscaler,gitlab,true,gitlab-runner-autoscale,true",
          "amazonec2-security-group=gitlab-runner",
          "amazonec2-instance-type=t2.large"
        ]                         
    ```  
    
    You will need use the same VPC subnet and availability zone as the instance launched in step 2. We are using AWS 
    region `us-west-2`. The _ServerAddress_ for S3 will need to be updated if the region is changed. For `us-east-1` the
    _ServerAddress_ is `s3.amazonaws.com`. Under MachineOptions you can add anything that the [AWS Docker Machine](https://docs.docker.com/machine/drivers/aws/#options) 
    driver supports.
     
    Below are some example values for the placeholders to ensure for format of your values are correct. 
    ```yaml
    BucketName = saas-starter-kit-usw
    amazonec2-vpc-id=vpc-5f43f027
    amazonec2-subnet-id=subnet-693d3110
    amazonec2-zone=a
    ``` 
    
    Once complete, restart the runner.
    ```bash 
    sudo gitlab-runner restart
    ``` 
    
    
## Examples 

```bash
go run main.go deploy -service=web-app -env=dev -enable_https=true -primary_host=example.saasstartupkit.com -host_names=example.saasstartupkit.com,dev.example.saasstartupkit.com -private_bucket=saas-starter-kit-private -public_bucket=saas-starter-kit-public -public_bucket_cloudfront=true -static_files_s3=true -static_files_img_resize=1 -recreate_service=0
```    