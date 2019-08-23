

schema  
=== 

_schema_ is a command line tool for local development that executes database migrations. 


<!-- toc -->

- [Overview](#overview)
- [Installation](#installation)
- [Usage](#usage)
    * [Commands](#commands)
    * [Examples](#examples)
- [Join us on Gopher Slack](#join-us-on-gopher-slack)

<!-- tocstop -->



## Overview

The command line tool that executes the database migrations defined in 
[internal/schema](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/tree/master/internal/schema). This tool 
should be used to test and deploy schema migrations against your local development database (hosted by docker). 

For additional details regarding this tool, refer to 
[build/cicd](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/tree/master/build/cicd#schema-migrations)



## Installation

Make sure you have a working Go environment.  Go version 1.2+ is supported.  [See
the install instructions for Go](http://golang.org/doc/install.html).



## Usage 

```bash
$ go run main.go [global options] command [command options] [arguments...]
```

### Global Options 


* Show help 

    `--help, -h`  

* Print the version 

   `--version, -v`  

### Commands

* `migrate` - Executes the database migrations defined in 
[internal/schema](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/tree/master/internal/schema) for local 
development. Default values are set for all command options that target the Postgres database running via 
[docker compose](https://gitlab.com/geeks-accelerator/oss/saas-starter-kit/blob/master/docker-compose.yaml#L11). 
Environment variables can be set as an alternative to passing in the command line options. 
   
    ```bash
    $ go run main.go migrate [command options]
    ``` 
    
    Options: 
    ```bash
    --env value       target environment, one of [dev, stage, prod] (default: "dev") [$ENV]
   --host value      host (default: "127.0.0.1:5433") [$SCHEMA_DB_HOST]
   --user value      username (default: "postgres") [$SCHEMA_DB_USER]
   --pass value      password (default: "postgres") [$SCHEMA_DB_PASS]
   --database value  name of the default (default: "shared") [$SCHEMA_DB_DATABASE]
   --driver value    database drive to use for connection (default: "postgres") [$SCHEMA_DB_DRIVER]
   --disable-tls     disable TLS for the database connection [$SCHEMA_DB_DISABLE_TLS]
    ``` 
    
* `help` - Shows a list of commands
       
    ```bash
    $ go run main.go help
    ```
        
    Or for one command:    
    ```bash
    $ go run main.go help migrate
    ```


### Examples

Execute the database migrations against the local Postgres database. 
```bash
$ go run main.go migrate 
```


## Join us on Gopher Slack

If you are having problems installing, troubles getting the project running or would like to contribute, join the 
channel #saas-starter-kit on [Gopher Slack](http://invite.slack.golangbridge.org/) 
