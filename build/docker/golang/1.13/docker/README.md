# GOLang 1.13 with Docker

Copyright 2019, Geeks Accelerator 
twins@geeksaccelerator.com


## Description

This is image is for GoLang 1.13 with Docker support for the GitLab CI/CD pipeline.


## Push updates


docker build -t registry.gitlab.com/alyssa-london/websites .


```bash
docker login registry.gitlab.com
docker build -t golang1.13-docker -t registry.gitlab.com/geeks-accelerator/oss/saas-starter-kit:golang1.13-docker .
docker push registry.gitlab.com/geeks-accelerator/oss/saas-starter-kit:golang1.13-docker
```

## Join us on Gopher Slack

If you are having problems installing, troubles getting the project running or would like to contribute, join the channel #saas-starter-kit on [Gopher Slack](http://invite.slack.golangbridge.org/) 
