# SaaS Schema 

Copyright 2019, Geeks Accelerator  
accelerator@geeksinthewoods.com.com


## Description

Service is handles the schema migration for the project.


## Local Installation

### Build 
```bash
go build .
```

### Usage 
```bash
./schema -h

Usage of ./schema
--env string  <dev>
--db_host string  <127.0.0.1:5433>
--db_user string  <postgres>
--db_pass string  <postgres>
--db_database string  <shared>
--db_driver string  <postgres>
--db_timezone string  <utc>
--db_disabletls bool  <false>
```

### Execution
Manually execute binary after build
```bash
./schema 
Schema : 2019/05/25 08:20:08.152557 main.go:64: main : Started : Application Initializing version "develop"
Schema : 2019/05/25 08:20:08.152814 main.go:75: main : Config : {
    "Env": "dev",
    "DB": {
        "Host": "127.0.0.1:5433",
        "User": "postgres",
        "Database": "shared",
        "Driver": "postgres",
        "Timezone": "utc",
        "DisableTLS": true
    }
}
Schema : 2019/05/25 08:20:08.158270 sqlxmigrate.go:478: HasTable migrations - SELECT 1 FROM migrations
Schema : 2019/05/25 08:20:08.164275 sqlxmigrate.go:413: Migration SCHEMA_INIT - SELECT count(0) FROM migrations WHERE id = $1
Schema : 2019/05/25 08:20:08.166391 sqlxmigrate.go:368: Migration 20190522-01a - checking
Schema : 2019/05/25 08:20:08.166405 sqlxmigrate.go:413: Migration 20190522-01a - SELECT count(0) FROM migrations WHERE id = $1
Schema : 2019/05/25 08:20:08.168066 sqlxmigrate.go:375: Migration 20190522-01a - already ran
Schema : 2019/05/25 08:20:08.168078 sqlxmigrate.go:368: Migration 20190522-01b - checking
Schema : 2019/05/25 08:20:08.168084 sqlxmigrate.go:413: Migration 20190522-01b - SELECT count(0) FROM migrations WHERE id = $1
Schema : 2019/05/25 08:20:08.170297 sqlxmigrate.go:375: Migration 20190522-01b - already ran
Schema : 2019/05/25 08:20:08.170319 sqlxmigrate.go:368: Migration 20190522-01c - checking
Schema : 2019/05/25 08:20:08.170327 sqlxmigrate.go:413: Migration 20190522-01c - SELECT count(0) FROM migrations WHERE id = $1
Schema : 2019/05/25 08:20:08.172044 sqlxmigrate.go:375: Migration 20190522-01c - already ran
Schema : 2019/05/25 08:20:08.172831 main.go:130: main : Migrate : Completed
Schema : 2019/05/25 08:20:08.172935 main.go:131: main : Completed
```

Or alternative use the make file
```bash
make run
```
