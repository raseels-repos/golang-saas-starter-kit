package project

import (
	"context"
	"database/sql"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
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
)

// CanReadProject determines if claims has the authority to access the specified project by id.
func (repo *Repository) CanReadProject(ctx context.Context, claims auth.Claims, id string) error {

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
		queryStr = repo.DbConn.Rebind(queryStr)
		var id string
		err := repo.DbConn.QueryRowContext(ctx, queryStr, args...).Scan(&id)
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
func (repo *Repository) CanModifyProject(ctx context.Context, claims auth.Claims, id string) error {
	err := repo.CanReadProject(ctx, claims, id)
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

// projectMapColumns is the list of columns needed for find.
var projectMapColumns = "id,account_id,name,status,created_at,updated_at,archived_at"

// selectQuery constructs a base select query for Project.
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

	if req.Where != "" {
		query.Where(query.And(req.Where))
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
func (repo *Repository) Find(ctx context.Context, claims auth.Claims, req ProjectFindRequest) (Projects, error) {
	query, args := findRequestQuery(req)
	return find(ctx, claims, repo.DbConn, query, args, req.IncludeArchived)
}

// find internal method for getting all the projects from the database using a select query.
func find(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, query *sqlbuilder.SelectBuilder, args []interface{}, includedArchived bool) (Projects, error) {
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
		var (
			m   Project
			err error
		)
		err = rows.Scan(&m.ID, &m.AccountID, &m.Name, &m.Status, &m.CreatedAt, &m.UpdatedAt, &m.ArchivedAt)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			return nil, err
		}

		resp = append(resp, &m)
	}

	return resp, nil
}

// ReadByID gets the specified project by ID from the database.
func (repo *Repository) ReadByID(ctx context.Context, claims auth.Claims, id string) (*Project, error) {
	return repo.Read(ctx, claims, ProjectReadRequest{
		ID:              id,
		IncludeArchived: false,
	})
}

// Read gets the specified project from the database.
func (repo *Repository) Read(ctx context.Context, claims auth.Claims, req ProjectReadRequest) (*Project, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Read")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return nil, err
	}

	// Filter base select query by id
	query := sqlbuilder.NewSelectBuilder()
	query.Where(query.Equal("id", req.ID))

	res, err := find(ctx, claims, repo.DbConn, query, []interface{}{}, req.IncludeArchived)
	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		err = errors.WithMessagef(ErrNotFound, "project %s not found", req.ID)
		return nil, err
	}

	u := res[0]
	return u, nil
}

// Create inserts a new project into the database.
func (repo *Repository) Create(ctx context.Context, claims auth.Claims, req ProjectCreateRequest, now time.Time) (*Project, error) {
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

	// Validate the request.
	v := webcontext.Validator()
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
	sql = repo.DbConn.Rebind(sql)
	_, err = repo.DbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessage(err, "create project failed")
		return nil, err
	}

	return &m, nil
}

// Update replaces an project in the database.
func (repo *Repository) Update(ctx context.Context, claims auth.Claims, req ProjectUpdateRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Update")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the project specified in the request.
	err = repo.CanModifyProject(ctx, claims, req.ID)
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
	sql = repo.DbConn.Rebind(sql)
	_, err = repo.DbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "update project %s failed", req.ID)
		return err
	}

	return nil
}

// Archive soft deleted the project from the database.
func (repo *Repository) Archive(ctx context.Context, claims auth.Claims, req ProjectArchiveRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Archive")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the project specified in the request.
	err = repo.CanModifyProject(ctx, claims, req.ID)
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
	sql = repo.DbConn.Rebind(sql)
	_, err = repo.DbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "archive project %s failed", req.ID)
		return err
	}

	return nil
}

// Delete removes an project from the database.
func (repo *Repository) Delete(ctx context.Context, claims auth.Claims, req ProjectDeleteRequest) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.project.Delete")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the project specified in the request.
	err = repo.CanModifyProject(ctx, claims, req.ID)
	if err != nil {
		return err
	}

	// Build the delete SQL statement.
	query := sqlbuilder.NewDeleteBuilder()
	query.DeleteFrom(projectTableName)
	query.Where(query.Equal("id", req.ID))
	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = repo.DbConn.Rebind(sql)
	_, err = repo.DbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "delete project %s failed", req.ID)
		return err
	}

	return nil
}
