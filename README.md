# SaaS Starter Kit

Copyright 2019, Geeks Accelerator 
twins@geeksaccelerator.com


## Description

The SaaS Starter Kit is a set of libraries for building scalable software-as-a-service (SaaS) applications while preventing both misuse and fraud. The goal of this project is project is to provide a proven starting point for new projects that reduces the repetitive tasks in getting a new project launched to production that can easily be scaled and ready to onboard enterprise clients. It uses minimal dependencies, implements idiomatic code and follows Golang best practices. Collectively, the toolkit lays out everything logically to minimize guess work and enable engineers to quickly load a mental model for the project. This inturn will make current developers happy and expedite on-boarding of new engineers. 

This project should not be considered a web framework. It is a starter toolkit that provides a set of working examples to handle some of the common challenges for developing SaaS using Golang. Coding is a discovery process and with that, it leaves you in control of your project’s architecture and development. 

There are five areas of expertise that an engineer or her engineering team must do for a project to grow and scale. Based on our experience, a few core decisions were made for each of these areas that help you focus initially on writing the business logic.
1. Micro level - Since SaaS requires transactions, project implements Postgres. Implementation facilitates the data semantics that define the data being captured and their relationships. 
2. Macro level - Uses POD architecture and design that provides the project foundation.
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

Refer to the [Example Project Readme](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit) for additional details.


## Join us on Gopher Slack

If you are having problems installing, troubles getting the project running or would like to contribute, join the channel #saas-starter-kit on [Gopher Slack](http://invite.slack.golangbridge.org/) 
