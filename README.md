# SaaS Starter Kit

Copyright 2019, Geeks Accelerator 
twins@geeksaccelerator.com


## Description

The SaaS Starter Kit is a set of libraries for building scalable software-as-a-service (SaaS) applications while 
preventing both misuse and fraud. The goal of this project is project is to provide a proven starting point for new 
projects that reduces the repetitive tasks in getting a new project launched to production that can easily be scaled 
and ready to onboard enterprise clients. It uses minimal dependencies, implements idiomatic code and follows Golang 
best practices. Collectively, the toolkit lays out everything logically to minimize guess work and enable engineers to 
quickly load a mental model for the project. This inturn will make current developers happy and expedite on-boarding of 
new engineers. 

This project should not be considered a web framework. It is a starter toolkit that provides a set of working examples 
to handle some of the common challenges for developing SaaS using Golang. Coding is a discovery process and with that, 
it leaves you in control of your project’s architecture and development. 

There are five areas of expertise that an engineer or her engineering team must do for a project to grow and scale. 
Based on our experience, a few core decisions were made for each of these areas that help you focus initially on writing 
the business logic.
1. Micro level - Since SaaS requires transactions, project implements Postgres. Implementation facilitates the data 
semantics that define the data being captured and their relationships. 
2. Macro level - The project architecture and design, provides basic project structure and foundation for development.
3. Business logic - Defines an example Golang package that helps illustrate where value generating activities should 
reside and how the code will be delivered to clients.
4. Deployment and Operations - Integrates with GitLab for CI/CD and AWS for serverless deployments with AWS Fargate.  
5. Observability - Implements Datadog to facilitate exposing metrics, logs and request tracing that ensure stable and 
responsive service for clients.  

SaaS product offerings typically provide two main components: an API and a web application. Both facilitate delivering a 
valuable software based product to clients ideally from a single code base on a recurring basis delivered over the 
internet. 

The example project is a complete starter kit for building SasS with GoLang. It provides three example services:
* Web App - Responsive web application to provide service to clients. Includes user signup and user authentication for 
direct client interaction. 
* Web API - REST API with JWT authentication that renders results as JSON. This allows clients to develop deep 
integrations with the project.
* Schema - Tool for initializing of Postgres database and handles schema migration. 

It contains the following features:
* Minimal web application using standard html/template package.
* Auto-documented REST API.
* Middleware integration.
* Database support using Postgres.
* Key value store using Redis
* CRUD based pattern.
* Role-based access control (RBAC).
* Account signup and user management.  
* Distributed logging and tracing.
* Integration with Datadog for enterprise-level observability. 
* Testing patterns.
* Use of Docker, Docker Compose, and Makefiles.
* Vendoring dependencies with Modules, requires Go 1.12 or higher.
* Continuous deployment pipeline. 
* Serverless deployments.
* CLI with boilerplate templates to reduce repetitive copy/pasting.
* Integration with GitLab for enterprise-level CI/CD.

### Example project 

With SaaS, a client subscribes to an online service you provide them. The example project provides functionality for 
clients to subscribe and then once subscribed they can interact with your software service. For this example, *projects* 
will be the single business logic package that will be exposed to users for management based on their role. Additional 
business logic packages can be added to support your project. It’s important at the beginning to minimize the connection 
between business logic packages on the same horizontal level. 
This project provides the following functionality to users:

New clients can sign up which creates an account and a user with role of admin.
* Users with the role of admin can manage users for their account. 
* Authenticated users can manage their projects based on RBAC.

The project implements RBAC with two basic roles for users: admin and user. 
* The role of admin provides the ability to perform all CRUD actions on projects and users. 
* The role of user limits users to only view projects and users. 

Of course, this example implementation of RBAC can be modified and enhanced to meet your requirements. 

The project groups code in three distinct directories:
* Cmd - all application stuff (routes and http transport)
* Internal - all business logic (compiler protections) 
* Platform - all foundation stuff (kit)

All business logic should be contained as a package inside the internal directory. This enables both the web app and web 
API to use the same API (Golang packages) with the only main difference between them is their response, HTML or JSON.



## Local Installation

Docker is required to run this project on your local machine. This project uses multiple third-party services that will 
be hosted locally via Docker. 
* Postgres - Transactional database to handle persistence of all data.
* Redis - Key / value storage for sessions and other data. Used only as ephemeral storage.
* Datadog - Provides metrics, logging, and tracing.

An AWS account is required for deployment for the following AWS dependencies:
* Secret Manager - Provides store for private key used for JWT.
* S3 - Host static files on S3 with additional CDN support with CloudFront.
* ECS Fargate - Serverless deployments of application. 
* RDS - Cloud hosted version of Postgres. 
* Route 53 - Management of DNS entries. 


### Getting the project

Clone the repo into your desired location. This project uses Go modules and does not need to be in your GOPATH. You will 
need to be using Go >= 1.11.

You should now be able to clone the project. 

```bash
git clone git@gitlab.com:geeks-accelerator/oss/saas-starter-kit.git
cd saas-starter-kit/
```

If you have Go Modules enabled, you should be able compile the project locally. If you have Go Modulels disabled, see 
the next section.


### Go Modules

This project is using Go Module support for vendoring dependencies. 

We are using the `tidy` command to maintain the dependencies and make sure the project can create reproducible builds. 

```bash
GO111MODULE=on go mod tidy
```

It is recommended to use at least Go 1.12 and enable go modules.

```bash
echo "export  GO111MODULE=on" >> ~/.bash_profile
```


### Installing Docker

Docker is a critical component and required to run this project.

https://docs.docker.com/install/


## Running The Project

There is a `docker-compose` file that knows how to build and run all the services. Each service has its own a 
`dockerfile`.

When you run `docker-compose up` it will run all the services including the main.go file for each Go service. The 
services the project will run are:
- web-api
- web-app 
- postgres
- mysql


### Running the project

Use the `docker-compose.yaml` to run all of the services, including the 3rd party services. The first time to run this 
command, Docker will download the required images for the 3rd party services.

```
$ docker-compose up
```

Default configuration is set which should be valid for most systems. 

Use the `docker-compose.yaml` file to configure the services differently using environment variables when necessary. 


### Stopping the project

You can hit <ctrl>C in the terminal window running `docker-compose up`. 

Once that shutdown sequence is complete, it is important to run the `docker-compose down` command.

```bash
$ <ctrl>C
$ docker-compose down
```

Running `docker-compose down` will properly stop and terminate the Docker Compose session.


### Re-starting a specific Go service for development

When writing code in an iterative fashion, it is nice to be able to restart a specific service so it will run updated 
Go code. This decreases the overhead of stopping all services with `docker-compose down` and then re-starting all the 
services again with 'docker-compose up'.

To restart a specific service, first use `docker ps` to see the list of services running.

```bash
docker ps
CONTAINER ID        IMAGE                            COMMAND                  NAMES
35043164fd0d        example-project/web-api:latest   "/gosrv"                 saas-starter-kit_web-api_1
d34c8fc27f3b        example-project/web-app:latest   "/gosrv"                 saas-starter-kit_web-app_1
fd844456243e        postgres:11-alpine               "docker-entrypoint.s…"   saas-starter-kit_postgres_1
dda16bfbb8b5        redis:latest                     "redis-server --appe…"   saas-starter-kit_redis_1
```

Then use `docker-compose down` for a specific service. In the command include the name of the container for the service 
to shut down. In the example command, we will shut down down the web-api service so we can start it again.

```bash
docker-compose down saas-starter-kit_web-api_1
```

If you are not in the directory for the service you want to restart navigate to it. We will go to the directory for the 
web-api.

```bash
cd cmd/web-api/
```

Then you can start the service again by running main.go
```bash
go run main.go
```


### Optional. Set AWS and Datadog Configs

By default the project will compile and run without AWS configs or other third-party dependencies. 

As you use start utilizing AWS services in this project and/or ready for deployment, you will need to start specifying 
AWS configs in a docker-compose file. You can also set credentials for other dependencies in the new docker-compose file 
too.

The sample docker-compose file is not loaded since it is named sample, which allows the project to run without these 
configs.

To set AWS configs and credentials for other third-party dependencies, you need to create a copy of the sample 
docker-compose file without "sample" prepending the file name. 

Navigate to the root of the project. Copy `sample.env_docker_compose` to `.env_docker_compose`. 

```bash
$ cd $GOPATH/src/geeks-accelerator/oss/saas-starter-kit
$ cp sample.env_docker_compose .env_docker_compose
```

The example the docker-compose file specifies these environmental variables. The $ means that the variable is commented 
out.
```
$ AWS_ACCESS_KEY_ID=
$ AWS_SECRET_ACCESS_KEY=
AWS_REGION=us-east-1
$ AWS_USE_ROLE=false
$ DD_API_KEY=
```

In your new copy of the example docker-compose file ".env_docker_compose", set the AWS configs by updating the following 
environmental variables: AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, and AWS_REGION. Remember to remove the $ before the 
variable name. 

As noted in the Local Installation section, the project is integrated with Datadog for observability. You can specify 
the API key for you Datadog account by setting the environmental variable: DD_API_KEY.


## Web API
[cmd/web-api](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/tree/master/cmd/web-api)

REST API available to clients for supporting deeper integrations. This API is also a foundation for third-party 
integrations. The API implements JWT authentication that renders results as JSON to clients. 

This web-api service is not directly used by the web-app service to prevent locking the functionally required for 
internally development of the web-app service to the same functionality exposed to clients via this web-api service. 
This separate web-api service can be exposed to clients and be maintained in a more rigid/structured process to manage 
client expectations.

The web-app will have its own internal API, similar to this external web-api service, but not exposed for third-party 
integrations. It is believed that in the beginning, having to define an additional API for internal purposes is worth 
the additional effort as the internal API can handle more flexible updates. 


### Making Requests to Web API

Once the web-api service is running it will be available on port 3000. 
http://localhost:3000

The easiest way to make requests to the web-api service is by using CURL via your CLI.

To make a request to the web-api service you must have an authenticated user. Users can be created with the API but an 
initial admin user must first be created. The initial admin user can be created with the web-app service. 


#### Creating Initial User

Create an initial user using the web-app service first. This pattern is how a user would signup to a SaaS product - a 
user would create an account on behalf of their organization. The subscription would then be associated with the 
organization and managed accordingly including billing.

While the web-app service is running, use the signup functionality to create a new user and corresponding account: 
http://localhost/signup

The email and password used to create the initial user and corresponding account can be used to make authenticated 
requests to the web-api service. 

#### Authenticating

Before any authenticated requests can be sent you must acquire an auth token. Make a request using HTTP Basic auth with 
your email and password to get the token.

```bash
$ curl --user "admin@example.com:gophers" http://localhost:3000/v1/users/token
```

It is best to put the resulting token in an environment variable like `$TOKEN`.

#### Adding Token as Environment Variable

dfadsf


#### Authenticated Requests

To make authenticated requests put the token in the `Authorization` header with the `Bearer ` prefix.

```bash
$ curl -H "Authorization: Bearer ${TOKEN}" http://localhost:3000/v1/users
```


## Web App
[cmd/web-app](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/tree/master/cmd/web-app)

Responsive web application that renders HTML using the `html/template` package from the standard library to enable 
direct interaction with clients and their users. It allows clients to sign up new accounts and provides user 
authentication with HTTP sessions. The web app relies on the Golang business logic packages developed to provide an API 
for internal requests. 

Once the web-app service is running it will be available on port 80. 
http://localhost/ or http://localhost:80/

While the web-api service is rocking, this web-app service is still in development. Only the signup functionality works 
in order for a user to create the initial user with role of admin and a corresponding account for their organization. 
If you would like to help, please email twins@geeksinthewoods.com.


### Functionality of Web App

The example web-app service is going to allow users to manage checklists. Users with role of admin will be allowed to 
create new checklists (projects). Each checklist will have tasks (items) associated with it. Tasks can be assigned to 
users with access to the checklist. Users can then update the status of a task. 

We are referring to "checklists" as "projects" and "tasks" as "items" so this example web-app service will be generic 
enough for you to leverage and build upon without lots of renaming.

This web-app service eventually will include the following functionality and corresponding web pages:
- authentication
    - signup (creates user and account records)
    - login
        - with role-based access
    - logout
    - forgot password
    - user management
        - update user and password
    - account management
        - update account
        - manage user
            - view user
            - create and invite user
            - update user
- projects (checklists)
    - index of projects
        - browse, filter, search
    - manage projects
        - view project
            - with project items
        - create project
        - update project
            - user access
    - project items (tasks)
        - view item
        - create item (adds task to checklist)
        - update item




## Schema 
[cmd/schema](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/tree/master/cmd/schema)

Schema is a minimalistic database migration helper that can manually be invoked via CLI. It provides schema versioning 
and migration rollback. 

To support POD architecture, the schema for the entire project is defined globally and is located inside internal: 
[internal/schema](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/tree/master/internal/schema)

Keeping a global schema helps ensure business logic can be decoupled across multiple packages. It is a firm belief that 
data models should not be part of feature functionality. Globally defined structs are dangerous as they create large 
code dependencies. Structs for the same database table can be defined by package to help mitigate large code 
dependencies. 

The example schema package provides two separate methods for handling schema migration:
* [Migrations](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/blob/master/internal/schema/migrations.go) -
List of direct SQL statements for each migration with defined version ID. A database table is created to persist 
executed migrations. Upon run of each schema migration run, the migraction logic checks the migration database table to 
check if it’s already been executed. Thus, schema migrations are only ever executed once. Migrations are defined as a function to enable complex migrations so results from query manipulated before being piped to the next query. 
* [Init Schema](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/blob/master/internal/schema/init_schema.go) - 
If you have a lot of migrations, it can be a pain to run all them. For example, when you are deploying a new instance of 
the app into a clean database. To prevent this, use the initSchema function that will run as-if no migration was run before (in a new clean database). 

Another bonus with the globally defined schema is that it enables the testing package to spin up database containers 
on-demand and automatically include all the migrations. This allows the testing package to programmatically execute 
schema migrations before running any unit tests. 


### Accessing Postgres 

To login to the local Postgres container, use the following command:
```bash
docker exec -it saas-starter-kit_postgres_1 /bin/bash
bash-4.4# psql -u postgres shared
```

The example project currently only includes a few tables. As more functionality is built into both the web-app and 
web-api services, the number of tables will expand. You can use the `show tables` command to list them. 
```commandline
shared=# \dt
             List of relations
 Schema |      Name      | Type  |  Owner   
--------+----------------+-------+----------
 public | accounts       | table | postgres
 public | migrations     | table | postgres
 public | projects       | table | postgres
 public | users          | table | postgres
 public | users_accounts | table | postgres
(5 rows)
```


## Development Notes


### AWS Permissions

Base required permissions.
```
secretsmanager:CreateSecret
secretsmanager:GetSecretValue
secretsmanager:ListSecretVersionIds
secretsmanager:PutSecretValue
secretsmanager:UpdateSecret
```

Additional permissions required for unit tests.
```
secretsmanager:DeleteSecret
```

The example web app service allows static files to be served from AWS CloudFront for increased performance. Enable for 
static files to be served from CloudFront instead of from service directly. 
```
cloudFront:ListDistributions
```


### Datadog

Datadog has a custom init script to support setting multiple expvar urls for monitoring. The docker-compose file then 
can set a single env variable.
```bash
DD_EXPVAR=service_name=web-app env=dev url=http://web-app:4000/debug/vars|service_name=web-api env=dev url=http://web-api:4001/debug/vars
```


### Gitlab 

[GitLab CI/CD Pipeline Configuration Reference](https://docs.gitlab.com/ee/ci/yaml/)


### Postgres and future MySQL support

Postgres is only supported based on its dependency of sqlxmigrate. MySQL should be easy to add to sqlxmigrate after 
determining a better method for abstracting the create table and other SQL statements from the main testing logic.

### SQLx bindvars

When making new packages that use sqlx, bind vars for mysql are `?` where as postgres is `$1`.

To database agnostic, sqlx supports using `?` for all queries and exposes the method `Rebind` to
remap the placeholders to the correct database.

```go
sqlQueryStr = db.Rebind(sqlQueryStr)
```

For additional details refer to https://jmoiron.github.io/sqlx/#bindvars



## Contribute 


### Development Notes regarding this copy of the project. 

#### GitLab CI / CD 

_Shared Runners_ have been disabled for this project. Since the project is open source and we wanted to avoid putting 
our AWS credentials as pipeline variables. Instead we have deployed our own set of autoscaling runnings on AWS EC2 that 
utilize AWS IAM Roles. All other configure is defined for CI/CD is defined in `.gitlab-ci.yaml`.  

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

    Note: This doesn't have to be a powerful machine since it will not run any jobs itself, a t2.micro instance will do.
    ``` 
    Amazon Machine Image (AMI): Amazon Linux AMI 2018.03.0 (HVM), SSD Volume Type - ami-0f2176987ee50226e
    Instance Type: t2.micro 
    ``` 

3. Configure Instance Details. 

    Note: Don't forget to select the IAM Role _SaasStarterKitEc2RoleForGitLabRunner_ 
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
    
4. Add Storage. Increase the volume size for the root device to 100 GiB
    ```    
    Volume Type |   Device      | Size (GiB) |  Volume Type 
    Root        |   /dev/xvda   | 100        |  General Purpose SSD (gp2)
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

## What's Next

We are in the process of writing more documentation about this code. We welcome you to make enhancements to this 
documentation or just send us your feedback and suggestions ; ) 


## Join us on Gopher Slack

If you are having problems installing, troubles getting the project running or would like to contribute, join the 
channel #saas-starter-kit on [Gopher Slack](http://invite.slack.golangbridge.org/) 
