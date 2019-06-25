# SaaS Web API 

Copyright 2019, Geeks Accelerator  
accelerator@geeksinthewoods.com.com


## Description

Service exposes a JSON api.


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


## API Documentation 

Documentation is generated using [swag](https://github.com/swaggo/swag)

Download swag by using:
```bash
go get -u github.com/swaggo/swag/cmd/swag
```

Run `swag init` in the service's root folder which contains the main.go file. This will parse your comments and generate the required files (docs folder and docs/docs.go).
```bash
swag init
```


### Trouble shooting

If you run into errors running `swag init` try the following:
 

#### cannot find package 
Try to install the packages to your $GOPATH.

```bash
GO111MODULE=off go get github.com/leodido/go-urn
GO111MODULE=off go get github.com/lib/pq/oid
GO111MODULE=off go get  github.com/lib/pq/scram
GO111MODULE=off go get github.com/tinylib/msgp/msgp
GO111MODULE=off go get gopkg.in/DataDog/dd-trace-go.v1/ddtrace
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
 
 