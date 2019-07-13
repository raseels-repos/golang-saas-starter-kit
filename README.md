# SaaS Starter Kit

Copyright 2019, Geeks Accelerator 
twins@geeksaccelerator.com


## Description

The SaaS Starter Kit is a set of libraries for building scalable software-as-a-service (SaaS) applications while preventing both misuse and fraud. The goal of this project is project is to provide a proven starting point for new projects that reduces the repetitive tasks in getting a new project launched to production that can easily be scaled and ready to onboard enterprise clients. It uses minimal dependencies, implements idiomatic code and follows Golang best practices. Collectively, the toolkit lays out everything logically to minimize guess work and enable engineers to quickly load a mental model for the project. This inturn will make current developers happy and expedite on-boarding of new engineers. 

This project should not be considered a web framework. It is a starter toolkit that provides a set of working examples to handle some of the common challenges for developing SaaS using Golang. Coding is a discovery process and with that, it leaves you in control of your project’s architecture and development. 

There are five areas of expertise that an engineer or her engineering team must do for a project to grow and scale. Based on our experience, a few core decisions were made for each of these areas that help you focus initially on writing the business logic.
1. Micro level - Since SaaS requires transactions, project implements Postgres. Implementation facilitates the data semantics that define the data being captured and their relationships. 
2. Macro level - The project architecture and design, provides basic project structure and foundation for development.
3. Business logic - Defines an example Golang package that helps illustrate where value generating activities should reside and how the code will be delivered to clients.
4. Deployment and Operations - Integrates with GitLab for CI/CD and AWS for serverless deployments with AWS Fargate.  
5. Observability - Implements Datadog to facilitate exposing metrics, logs and request tracing that ensure stable and responsive service for clients.  

SaaS product offerings typically provide two main components: an API and a web application. Both facilitate delivering a valuable software based product to clients ideally from a single code base on a recurring basis delivered over the internet. 

The example project is a complete starter kit for building SasS with GoLang. It provides three example services:
* Web App - Responsive web application to provide service to clients. Includes user signup and user authentication for direct client interaction. 
* Web API - REST API with JWT authentication that renders results as JSON. This allows clients to develop deep integrations with the project.
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

With SaaS, a client subscribes to an online service you provide them. The example project provides functionality for clients to subscribe and then once subscribed they can interact with your software service. For this example, *projects* will be the single business logic package that will be exposed to users for management based on their role. Additional business logic packages can be added to support your project. It’s important at the beginning to minimize the connection between business logic packages on the same horizontal level. 
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

All business logic should be contained as a package inside the internal directory. This enables both the web app and web API to use the same API (Golang packages) with the only main difference between them is their response, HTML or JSON.



## Local Installation

Docker is required to run this project on your local machine. This project uses multiple third-party services that will be hosted locally via docker. 
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

Clone the repo into your desired location. This project uses Go modules and does not need to be in your GOPATH. You will need to be using Go >= 1.11.

You should now be able to compile the project locally.

```
cd example-project/
GO111MODULE=on go install ./...
```


### Go Modules

This project is using Go Module support for vendoring dependencies. 

We are using the `tidy` command to maintain the dependencies and make sure the project can create reproducible builds. 

```
cd example-project
GO111MODULE=on go mod tidy
```

It is recommended to use at least go 1.12 and enable go modules.

```bash
echo "export  GO111MODULE=on" >> ~/.bash_profile
```


### Installing Docker

Docker is a critical component and required to run this project.

https://docs.docker.com/install/


## Running The Project

There is a `docker-compose` file that knows how to build and run all the services. Each service has its own a `dockerfile`.


### Running the project

Navigate to the root of the project and first set your AWS configs. Copy `sample.env_docker_compose` to `.env_docker_compose` and update the credentials for docker-compose.

```
$ cd $GOPATH/src/geeks-accelerator/oss/saas-starter-kit/example-project
$ cp sample.env_docker_compose .env_docker_compose
```

Use the `docker-compose.yaml` to run all of the services, including the 3rd party services. The first time to run this command, Docker will download the required images for the 3rd party services.

```
$ docker-compose up
```

Default configuration is set which should be valid for most systems. 

Use the `docker-compose.yaml` file to configure the services differently using environment variables when necessary. 


### Stopping the project

You can hit <ctrl>C in the terminal window running `docker-compose up`. 

Once that shutdown sequence is complete, it is important to run the `docker-compose down` command.

```
$ <ctrl>C
$ docker-compose down
```

Running `docker-compose down` will properly stop and terminate the Docker Compose session.






## Web App
[cmd/web-app](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/tree/master/example-project/cmd/web-app)

Responsive web application that renders HTML using the `html/template` package from the standard library to enable direct interaction with clients and their users. It allows clients to sign up new accounts and provides user authentication with HTTP sessions. The web app relies on the Golang business logic packages developed to provide an API for internal requests. 


## Web API
[cmd/web-api](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/tree/master/example-project/cmd/web-api)

REST API available to clients for supporting deeper integrations. This API is also a foundation for third-party integrations. The API implements JWT authentication that renders results as JSON to clients. This API is not directly used by the web app to prevent locking the functionally needed internally for development of the web app to the same functionality exposed to clients. It is believed that in the beginning, having to define an additional API for internal purposes is worth at additional effort as the internal API can handle more flexible updates. The API exposed to clients can then be maintained in a more rigid/structured process to manage client expectations.


### Making Requests

#### Initial User

To make a request to the service you must have an authenticated user. Users can be created with the API but an initial admin user must first be created.  While the Web App service is running, signup to create a new account. The email and password used to create the initial account can be used to make API requests. 

#### Authenticating

Before any authenticated requests can be sent you must acquire an auth token. Make a request using HTTP Basic auth with your email and password to get the token.

```
$ curl --user "admin@example.com:gophers" http://localhost:3000/v1/users/token
```

It’s best to put the resulting token in an environment variable like `$TOKEN`.

#### Authenticated Requests

To make authenticated requests put the token in the `Authorization` header with the `Bearer ` prefix.

```
$ curl -H "Authorization: Bearer ${TOKEN}" http://localhost:3000/v1/users
```

## Schema 
[cmd/schema](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/tree/master/example-project/cmd/schema)

Schema is a minimalistic database migration helper that can manually be invoked via CLI. It provides schema versioning and migration rollback. 

To support POD architecture, the schema for the entire project is defined globally and is located inside internal: [internal/schema](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/tree/master/example-project/internal/schema)

Keeping a global schema helps ensure business logic then can be decoupled across multiple packages. It’s a firm belief that data models should not be part of feature functionality. Globally defined structs are dangerous as they create large code dependencies. Structs for the same database table can be defined by package to help mitigate large code dependencies. 

The example schema package provides two separate methods for handling schema migration.
* [Migrations](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/blob/master/example-project/internal/schema/migrations.go) 
List of direct SQL statements for each migration with defined version ID. A database table is created to persist executed migrations. Upon run of each schema migration run, the migraction logic checks the migration database table to check if it’s already been executed. Thus, schema migrations are only ever executed once. Migrations are defined as a function to enable complex migrations so results from query manipulated before being piped to the next query. 
* [Init Schema](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/blob/master/example-project/internal/schema/init_schema.go) 
If you have a lot of migrations, it can be a pain to run all them, as an example, when you are deploying a new instance of the app, in a clean database. To prevent this, use the initSchema function that will run if no migration was run before (in a new clean database). If you are using this to help seed the database, you will need to create everything needed, all tables, foreign keys, etc.

Another bonus with the globally defined schema allows testing to spin up database containers on demand include all the migrations. The testing package enables unit tests to programmatically execute schema migrations before running any unit tests. 


### Accessing Postgres 

Login to the local postgres container 
```bash
docker exec -it example-project_postgres_1 /bin/bash
bash-4.4# psql -u postgres shared
```

Show tables
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


## Making db calls

Postgres is only supported based on its dependency of sqlxmigrate. MySQL should be easy to add to sqlxmigrate after determining better method for abstracting the create table and other SQL statements from the main testing logic.

### bindvars

When making new packages that use sqlx, bind vars for mysql are `?` where as postgres is `$1`.
To database agnostic, sqlx supports using `?` for all queries and exposes the method `Rebind` to
remap the placeholders to the correct database.

```go
sqlQueryStr = db.Rebind(sqlQueryStr)
```

For additional details refer to https://jmoiron.github.io/sqlx/#bindvars

### datadog

Datadog has a custom init script to support setting multiple expvar urls for monitoring. The docker-compose file then can set a single env variable.
```bash
DD_EXPVAR=service_name=web-app env=dev url=http://web-app:4000/debug/vars|service_name=web-api env=dev url=http://web-api:4001/debug/vars
```


### gitlab 

[GitLab CI/CD Pipeline Configuration Reference](https://docs.gitlab.com/ee/ci/yaml/)



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

The example web app service allows static files to be served from AWS CloudFront for increased performance. Enable for static files to be served from CloudFront instead of from service directly. 
```
cloudFront:ListDistributions
```


## What's Next

We are in the process of writing more documentation about this code. We welcome you to make enhancements to this documentation or just send us your feedback and suggestions ; ) 


## Join us on Gopher Slack

If you are having problems installing, troubles getting the project running or would like to contribute, join the channel #saas-starter-kit on [Gopher Slack](http://invite.slack.golangbridge.org/) 
