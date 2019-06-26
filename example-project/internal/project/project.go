package project

import (
	"context"
	"database/sql"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/go-playground/validator.v9"
	"time"
)

const (
	// The database table for Project
	projectTableName = "projects"
)

var (
	// ErrNotFound abstracts the postgres not found error.
	ErrNotFound = errors.New("Entity not found")
	// ErrForbidden occurs when a user tries to do something that is forbidden to them according to our access control policies.
	ErrForbidden = errors.New("Attempted action is not allowed")
	// ErrInvalidID occurs when an ID is not in a valid form.
	ErrInvalidID = errors.New("ID is not in its proper form")
)

// projectMapColumns is the list of columns needed for mapRowsToProject
var projectMapColumns = "id,account_id,name,status,created_at,updated_at,archived_at"

// mapRowsToProject takes the SQL rows and maps it to the Project struct
// with the columns defined by projectMapColumns
func mapRowsToProject(rows *sql.Rows) (*Project, error) {
	var (
		m   Project
		err error
	)

	err = rows.Scan(&m.ID, &m.AccountID, &m.Name, &m.Status, &m.CreatedAt, &m.UpdatedAt, &m.ArchivedAt)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &m, nil
}

// CanReadProject determines if claims has the authority to access the specified project by id.
func CanReadProject(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, id string) error {

	// If the request has claims from a specific project, ensure that the claims
	// has the correct access to the project.
	if claims.Audience != "" {
		// select id from projects where account_id = [accountID]
		query := sqlbuilder.NewSelectBuilder().Select("id").From(projectTableName)
		query.Where(query.And(
			query.Equal("account_id", claims.Audience),
			query.Equal("ID", id),
		))

		queryStr, args := query.Build()
		queryStr = dbConn.Rebind(queryStr)
		var id string
		err := dbConn.QueryRowContext(ctx, queryStr, args...).Scan(&id)
		if err != nil && err != sql.ErrNoRows {
			err = errors.Wrapf(err, "query - %s", query.String())
			return err
		}

		// When there is no id returned, then the current claim user does not have access
		// to the specified project.
		if id == "" {
			return errors.WithStack(ErrForbidden)
		}

	}

	return nil
}

// CanModifyProject determines if claims has the authority to modify the specified project by id.
func CanModifyProject(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, id string) error {
	err := CanReadProject(ctx, claims, dbConn, id)
	if err != nil {
		return err
	}

	// Admin users can update projects they have access to.
	if !claims.HasRole(auth.RoleAdmin) {
		return errors.WithStack(ErrForbidden)
	}

	return nil
}

// applyClaimsSelect applies a sub-query to the provided query to enforce ACL based on the claims provided.
// 	1. No claims, request is internal, no ACL applied

// 	2. All role types can access their user ID

func applyClaimsSelect(ctx context.Context, claims auth.Claims, query *sqlbuilder.SelectBuilder) error {
	// Claims are empty, don't apply any ACL
	if claims.Audience == "" {
		return nil
	}

	query.Where(query.Equal("account_id", claims.Audience))
	return nil
}

// selectQuery constructs a base select query for Project
func selectQuery() *sqlbuilder.SelectBuilder {
	query := sqlbuilder.NewSelectBuilder()
	query.Select(projectMapColumns)
	query.From(projectTableName)
	return query
}

// findRequestQuery generates the select query for the given find request.
// TODO: Need to figure out why can't parse the args when appending the where
// 			to the query.
func findRequestQuery(req ProjectFindRequest) (*sqlbuilder.SelectBuilder, []interface{}) {
	query := selectQuery()
	if req.Where != nil {
		query.Where(query.And(*req.Where))
	}

	if len(req.Order) > 0 {
		query.OrderBy(req.Order...)
	}

	if req.Limit != nil {
		query.Limit(int(*req.Limit))
	}

	if req.Offset != nil {
		query.Offset(int(*req.Offset))
	}

	return query, req.Args
}

// Find gets all the projects from the database based on the request params.
func Find(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req ProjectFindRequest) ([]*Project, error) {
	query, args := findRequestQuery(req)
	return find(ctx, claims, dbConn, query, args, req.IncludedArchived)
}

// find internal method for getting all the projects from the database using a select query.
func find(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, query *sqlbuilder.SelectBuilder, args []interface{}, includedArchived bool) ([]*Project, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Find")
	defer span.Finish()
	query.Select(projectMapColumns)
	query.From(projectTableName)
	if !includedArchived {
		query.Where(query.IsNull("archived_at"))
	}

	// Check to see if a sub query needs to be applied for the claims.
	err := applyClaimsSelect(ctx, claims, query)
	if err != nil {
		return nil, err
	}

	queryStr, queryArgs := query.Build()
	queryStr = dbConn.Rebind(queryStr)
	args = append(args, queryArgs...)
	// Fetch all entries from the db.
	rows, err := dbConn.QueryContext(ctx, queryStr, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessage(err, "find projects failed")
		return nil, err
	}

	// Iterate over each row.
	resp := []*Project{}
	for rows.Next() {
		u, err := mapRowsToProject(rows)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			return nil, err
		}

		resp = append(resp, u)
	}

	return resp, nil
}

// Read gets the specified project from the database.
func Read(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, id string, includedArchived bool) (*Project, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Read")
	defer span.Finish()
	// Filter base select query by id
	query := selectQuery()
	query.Where(query.Equal("id", id))
	res, err := find(ctx, claims, dbConn, query, []interface{}{}, includedArchived)
	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		err = errors.WithMessagef(ErrNotFound, "project %s not found", id)
		return nil, err
	}

	u := res[0]
	return u, nil
}

// Create inserts a new project into the database.
func Create(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req ProjectCreateRequest, now time.Time) (*Project, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Create")
	defer span.Finish()
	if claims.Audience != "" {
		// Admin users can update projects they have access to.
		if !claims.HasRole(auth.RoleAdmin) {
			return nil, errors.WithStack(ErrForbidden)
		}

		if req.AccountID != "" {
			// Request accountId must match claims.
			if req.AccountID != claims.Audience {
				return nil, errors.WithStack(ErrForbidden)
			}

		} else {
			// Set the accountId from claims.
			req.AccountID = claims.Audience
		}

	}

	v := validator.New()
	// Validate the request.
	err := v.Struct(req)
	if err != nil {
		return nil, err
	}

	// If now empty set it to the current time.
	if now.IsZero() {
		now = time.Now()
	}

	// Always store the time as UTC.
	now = now.UTC()
	// Postgres truncates times to milliseconds when storing. We and do the same
	// here so the value we return is consistent with what we store.
	now = now.Truncate(time.Millisecond)
	m := Project{
		ID:        uuid.NewRandom().String(),
		AccountID: req.AccountID,
		Name:      req.Name,
		Status:    ProjectStatus_Active,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if req.Status != nil {
		m.Status = *req.Status
	}

	// Build the insert SQL statement.
	query := sqlbuilder.NewInsertBuilder()
	query.InsertInto(projectTableName)
	query.Cols(
		"id",
		"account_id",
		"name",
		"status",
		"created_at",
		"updated_at",
		"archived_at",
	)

	query.Values(
		m.ID,
		m.AccountID,
		m.Name,
		m.Status,
		m.CreatedAt,
		m.UpdatedAt,
		m.ArchivedAt,
	)

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)
	_, err = dbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessage(err, "create project failed")
		return nil, err
	}

	return &m, nil
}

// Update replaces an project in the database.
func Update(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req ProjectUpdateRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Update")
	defer span.Finish()
	v := validator.New()
	// Validate the request.
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the project specified in the request.
	err = CanModifyProject(ctx, claims, dbConn, req.ID)
	if err != nil {
		return err
	}

	// If now empty set it to the current time.
	if now.IsZero() {
		now = time.Now()
	}

	// Always store the time as UTC.
	now = now.UTC()
	// Postgres truncates times to milliseconds when storing. We and do the same
	// here so the value we return is consistent with what we store.
	now = now.Truncate(time.Millisecond)
	// Build the update SQL statement.
	query := sqlbuilder.NewUpdateBuilder()
	query.Update(projectTableName)
	var fields []string
	if req.Name != nil {
		fields = append(fields, query.Assign("name", req.Name))
	}

	if req.Status != nil {
		fields = append(fields, query.Assign("status", req.Status))
	}

	// If there's nothing to update we can quit early.
	if len(fields) == 0 {
		return nil
	}

	// Append the updated_at field
	fields = append(fields, query.Assign("updated_at", now))
	query.Set(fields...)
	query.Where(query.Equal("id", req.ID))
	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)
	_, err = dbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "update project %s failed", req.ID)
		return err
	}

	return nil
}

// Archive soft deleted the project by ID from the database.
func ArchiveById(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, id string, now time.Time) error {
	req := ProjectArchiveRequest{
		ID: id,
	}
	return Archive(ctx, claims, dbConn, req, now)
}

// Archive soft deleted the project from the database.
func Archive(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req ProjectArchiveRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Archive")
	defer span.Finish()

	// Validate the request.
	err := validator.New().Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the project specified in the request.
	err = CanModifyProject(ctx, claims, dbConn, req.ID)
	if err != nil {
		return err
	}

	// If now empty set it to the current time.
	if now.IsZero() {
		now = time.Now()
	}

	// Always store the time as UTC.
	now = now.UTC()
	// Postgres truncates times to milliseconds when storing. We and do the same
	// here so the value we return is consistent with what we store.
	now = now.Truncate(time.Millisecond)
	// Build the update SQL statement.
	query := sqlbuilder.NewUpdateBuilder()
	query.Update(projectTableName)
	query.Set(
		query.Assign("archived_at", now),
	)

	query.Where(query.Equal("id", req.ID))
	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)
	_, err = dbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "archive project %s failed", req.ID)
		return err
	}

	return nil
}

// Delete removes an project from the database.
func Delete(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, id string) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Delete")
	defer span.Finish()
	// Defines the struct to apply validation
	req := struct {
		ID string `validate:"required,uuid"`
	}{}
	// Validate the request.
	err := validator.New().Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the project specified in the request.
	err = CanModifyProject(ctx, claims, dbConn, req.ID)
	if err != nil {
		return err
	}

	// Build the delete SQL statement.
	query := sqlbuilder.NewDeleteBuilder()
	query.DeleteFrom(projectTableName)
	query.Where(query.Equal("id", req.ID))
	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)
	_, err = dbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "delete project %s failed", req.ID)
		return err
	}

	return nil
}
