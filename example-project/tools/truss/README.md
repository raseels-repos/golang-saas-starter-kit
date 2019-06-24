# SaaS Truss 

Copyright 2019, Geeks Accelerator  
accelerator@geeksinthewoods.com.com


## Description

Truss provides code generation to reduce copy/pasting.


## Local Installation

### Build 
```bash
go build .
```

### Configuration 
```bash
./truss -h

Usage of ./truss
--cmd string  <dbtable2crud>
--db_host string  <127.0.0.1:5433>
--db_user string  <postgres>
--db_pass string  <postgres>
--db_database string  <shared>
--db_driver string  <postgres>
--db_timezone string  <utc>
--db_disabletls bool  <false>
``` 

## Commands:

## dbtable2crud  

Used to bootstrap a new business logic package with basic CRUD.

**Usage**
```bash
./truss dbtable2crud  -table=projects -file=../../internal/project/models.go -model=Project [-dbtable=TABLE] [-templateDir=DIR] [-projectPath=DIR] [-saveChanges=false]    
```
      
**Example**              
1. Define a new database table in `internal/schema/migrations.go`


2. Create a new file for the base model at `internal/projects/models.go`. Only the following struct needs to be included. All the other times will be generated.
```go 
// Project represents a workflow.
type Project struct {
	ID         string        `json:"id" validate:"required,uuid"`
	AccountID  string        `json:"account_id" validate:"required,uuid" truss:"api-create"`
	Name       string        `json:"name"  validate:"required"`
	Status     ProjectStatus `json:"status" validate:"omitempty,oneof=active disabled"`
	CreatedAt  time.Time     `json:"created_at" truss:"api-read"`
	UpdatedAt  time.Time     `json:"updated_at" truss:"api-read"`
	ArchivedAt pq.NullTime   `json:"archived_at" truss:"api-hide"`
}
```

3. Run `dbtable2crud` 
```bash
./truss dbtable2crud -table=projects -file=../../internal/project/models.go -model=Project -save=true
``` 


