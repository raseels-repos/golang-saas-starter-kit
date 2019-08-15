# SaaS Web App 

Copyright 2019, Geeks Accelerator  
accelerator@geeksinthewoods.com


## Description

Responsive web application that renders HTML using the `html/template` package from the standard library to enable 
direct interaction with clients and their users. It allows clients to sign up new accounts and provides user 
authentication with HTTP sessions. To see screen captures of the web app, check out this Google Slides deck:
https://docs.google.com/presentation/d/1WGYqMZ-YUOaNxlZBfU4srpN8i86MU0ppWWSBb3pkejM/edit#slide=id.p

*You are welcome to add comments to the Google Slides.*

[![Google Slides of Screen Captures for SaaS Starter Kit web app](resources/images/saas-webapp-screencapture-01.jpg)](https://docs.google.com/presentation/d/1WGYqMZ-YUOaNxlZBfU4srpN8i86MU0ppWWSBb3pkejM/edit#slide=id.p)


The web app relies on the Golang business logic packages developed to provide an API for internal requests. 

Once the web-app service is running it will be available on port 3000.

http://127.0.0.1:3000/

While the web-api service has 
significant functionality, this web-app service is still in development. Currently this web-app services only resizes 
an image and displays resized versions of it on the index page. See section below on Future Functionality. 

If you would like to help, please email twins@geeksinthewoods.com.


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


## Getting Started 

### Errors 

- **validation error** - Test by appending `test-validation-error=1` to the request URL.
http://127.0.0.1:3000/signup?test-validation-error=1

- **web error** - Test by appending `test-web-error=1` to the request URL.
http://127.0.0.1:3000/signup?test-web-error=1


### Localization 

Test a specific language by appending the locale to the request URL.
127.0.0.1:3000/signup?local=fr


[github.com/go-playground/validator](https://github.com/go-playground/validator) supports the following languages.
- en - English 
- fr - French
- id - Indonesian
- ja - Japanese
- nl - Dutch
- zh - Chinese

### Future Functionality

This example Web App is going to allow users to manage checklists. Users with role of admin will be allowed to 
create new checklists (projects). Each checklist will have tasks (items) associated with it. Tasks can be assigned to 
users with access to the checklist. Users can then update the status of a task. 

We are referring to "checklists" as "projects" and "tasks" as "items" so this example web-app service will be generic 
enough for you to leverage and build upon without lots of renaming.

The initial contributors to this project created a similar service like this: [standard operating procedure software](https://keeni.space/procedures/software)
for Keeni.Space. Its' Golang web app for [standard operating procedures software](https://keeni.space/procedures/software) is available at [app.keeni.space](https://app.keeni.space) They plan on leveraging this experience and boil it down into a simplified set of functionality 
and corresponding web pages that will be a solid examples for building enterprise SaaS web apps with Golang. 

This web-app service eventually will include the following:
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