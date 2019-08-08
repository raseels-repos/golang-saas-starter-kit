# SaaS Web API 

Copyright 2019, Geeks Accelerator  
accelerator@geeksinthewoods.com.com


## Description

Web API is a client facing API. Standard response format is JSON. 

**Not all CRUD methods are exposed as endpoints.** Only endpoints that clients may need should be exposed. Internal 
services should communicate directly with the business logic packages or a new API should be created to support it. This 
separation should help decouple client integrations from internal application development. 


## Making Requests to Web API

Once the web-api service is running it will be available on port 3000. 
http://127.0.0.1:3001/

The easiest way to make requests to the web-api service is by using CURL via your CLI.

To make a request to the web-api service you must have an authenticated user. Users can be created with the API but an 
initial admin user must first be created. The initial admin user can easily be created using the Swagger API documentation. 


## API Documentation

Documentation for this API service is automatically generated using [swag](https://github.com/geeks-accelerator/swag). Once this
web-api service is running, it can be accessed at /docs

http://127.0.0.1:3001/docs/




## Local Installation

### Build 
```bash
go build .
``` 

### Docker 

To build using the docker file, need to be in the project root directory. `Dockerfile` references go.mod in root directory.


```bash
docker build -f cmd/web-api/Dockerfile -t saas-web-api .
```


## Getting Started 

###1. Ensure dependant services are running. 

Navigate to the project root where `docker-compose.yaml` exists. There is only 
one `docker-compose.yaml` file that is shared between all services. 

*Start Services.*
```bash
docker-compose up -d 
```

1.1. Set env variables. 

*Copy the sample file to make your own copy.* 
```bash
cp sample.env local.env
```
*Make any changes to your copy of the file if necessary and then add them to your env.
```bash 
source local.env
```

1.2. Start the web-api service.

*Invoke main.go directly or use `go build .`* 
```bash
go run main.go
```


###2. Initialize the MySQL database.
  
When `docker-compose up` if first ran, as discussed in the main readme, the database is created but does not include 
any schema.

Thus, you need to use the project's schema tool to initialize the database. To do this navigate the the tools/schema
folder. Add the source to your env. Then run the `main.go` file for schema.  
```bash
cd tools/schema/
source sample.env 
go run main.go 
```

  
###3. Open the Swagger UI. 
Navigate your browser to [http://127.0.0.1:3001/docs](http://127.0.0.1:3001/docs).


###4. Signup a new account. 

Find the `signup` endpoint in the Swagger UI.

Click `Try it out`. Example data has been pre-populated to generate a valid POST request. Your can adjust the values 
for the account and user objects accordingly. 

```json 
{
  "account": {
    "address1": "221 Tatitlek Ave",
    "address2": "Box #1832",
    "city": "Valdez",
    "country": "USA",
    "name": "Company 895ff280-5ed9-4b09-b7bc-86ab0f0951d4",
    "region": "AK",
    "timezone": "America/Anchorage",
    "zipcode": "99686"
  },
  "user": {
    "email": "90873f61-663e-43d1-8f0c-00415e73f650@example.com",
    "name": "Gabi May",
    "password": "SecretString",
    "password_confirm": "SecretString"
  }
}
```

**Note the user email and password from the above request will be used in the following steps.**

Click `Execute` and a response with status code 201 should have been returned.
```json
{
  "account": {
    "id": "baae6e0d-29ae-456f-9648-44c1e90ca8af",
    "name": "Company 895ff280-5ed9-4b09-b7bc-86ab0f0951d4",
    "address1": "221 Tatitlek Ave",
    "address2": "Box #1832",
    "city": "Valdez",
    "region": "AK",
    "country": "USA",
    "zipcode": "99686",
    "status": "active",
    "timezone": "America/Anchorage",
    "signup_user_id": {
      "String": "bfdc5ca9-872c-4417-8030-e1b4962a107c",
      "Valid": true
    },
    "billing_user_id": {
      "String": "bfdc5ca9-872c-4417-8030-e1b4962a107c",
      "Valid": true
    },
    "created_at": "2019-06-25T11:00:53.284Z",
    "updated_at": "2019-06-25T11:00:53.284Z"
  },
  "user": {
    "id": "bfdc5ca9-872c-4417-8030-e1b4962a107c",
    "name": "Gabi May",
    "email": "90873f61-663e-43d1-8f0c-00415e73f650@example.com",
    "timezone": "America/Anchorage",
    "created_at": "2019-06-25T11:00:53.284Z",
    "updated_at": "2019-06-25T11:00:53.284Z"
  }
}
```

If successful, data should be returned for code 201 for created.
 
Now you will now be able to use the email and password credentials to generate an auth token with the web-api service. 


###5. Generate an Auth Token    
An auth token is required for all other requests. 
    
Near the top of the Swagger UI locate the button `Authorize` and click it. 

Find the section `OAuth2Password (OAuth2, password)`

Enter the user email and password.

Change the type to `basic auth`

Click the button `Authorize` to generate a token that will be used by the Swagger UI for all future requests.
    
###6. Test Auth Token 

Now that the Swagger UI is authorized, try running endpoint using the oauth token.    

Find the endpoint GET `/accounts/{id}` endpoint in the Swagger UI. This endpoint should return the account by ID.
  
Click `Try it out` and enter the account ID from generated from signup (step 5).   
  
Click `Execute`. The response should be of an Account.


###Authenticating Directly with Web API

If you want to authenticate directly with this web-api service and not via the Swagger UI, use the following steps.

####1. Authenticating

Before any authenticated requests can be sent you must acquire an auth token. Make a request using HTTP Basic auth with 
your email and password to get an auth token.

```bash
curl --user "twin@example.com:SecretString" -X POST http://127.0.0.1:3001/v1/oauth/token
```

It is best to put the resulting token in an environment variable like `$TOKEN`.

####2. Adding Token as Environment Variable

```bash
export TOKEN="COPY TOKEN STRING FROM LAST CALL"
```

####3. Authenticated Requests

To make authenticated requests put the token in the `Authorization` header with the `Bearer ` prefix.

```bash
curl -H "Authorization: Bearer ${TOKEN}" http://127.0.0.1:3001/v1/users
```


## Update Swagger API Documentation 

Documentation is generated using [swag](https://github.com/geeks-accelerator/swag)

If you are developing this web-api service and you want your changes reflected in the API documentation, you will need
to download Swag and then run it each time you want the API documentation to be updated.

Download Swag with this command:
```bash
go get -u github.com/geeks-accelerator/swag/cmd/swag
```

Run `swag init` in the service's root folder which contains the main.go file. This will parse your comments and generate the required files (docs folder and docs/docs.go).
```bash
swag init
```

### Additional Swagger Annotations

Below are some additional example annotions that can be added to `main.go`
```go
// @title SaaS Example API
// @description This provides a public API...
// @termsOfService http://example.com/terms

// @contact.name API Support
// @contact.email support@geeksinthewoods.com
// @contact.url http://example.com/support
```

### Troubleshooting Swag

If you run into errors running `swag init` try the following:
 
#### cannot find package 
Try to install the packages to your $GOPATH.

```bash
GO111MODULE=off go get -u github.com/leodido/go-urn
GO111MODULE=off go get -u github.com/lib/pq/oid
GO111MODULE=off go get -u github.com/lib/pq/scram
GO111MODULE=off go get -u github.com/tinylib/msgp/msgp
GO111MODULE=off go get -u gopkg.in/DataDog/dd-trace-go.v1/ddtrace
GO111MODULE=off go get -u golang.org/x/xerrors
```

#### error writing go.mod

Need to update pkg directory permissions.

Full error: 
```bash
error writing go.mod: open /Users/leebrown/go/pkg/mod/github.com/lib/pq@v1.1.1/go.mod691440060.tmp: permission denied

```

Ensure the `pkg` directory used for go module cache has the correct permissions. 
```bash
sudo chown -R $(whoami):staff ${HOME}/go/pkg
sudo chmod -R 755 ${HOME}/go/pkg 
```
 
 
 

