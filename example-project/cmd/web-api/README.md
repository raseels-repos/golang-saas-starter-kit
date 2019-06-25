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
