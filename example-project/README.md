# SaaS Starter Kit Example Project

Copyright 2019, Geeks Accelerator 
twins@geeksaccelerator.com


## Description

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

