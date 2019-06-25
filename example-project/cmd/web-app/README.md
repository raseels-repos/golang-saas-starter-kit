# SaaS Web App 

Copyright 2019, Geeks Accelerator  
accelerator@geeksinthewoods.com.com


## Description

Provides an http service.


## Local Installation

### Build 
```bash
go build .
``` 

### Docker 

To build using the docker file, need to be in the project root directory. `Dockerfile` references go.mod in root directory.


```bash
docker build -f cmd/web-app/Dockerfile -t saas-web-app .
```

