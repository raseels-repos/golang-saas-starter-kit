package checklist

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
	// The database table for Checklist
	checklistTableName = "checklists"
)

var (
	// ErrNotFound abstracts the postgres not found error.
	ErrNotFound = errors.New("Entity not found")

	// ErrForbidden occurs when a user tries to do something that is forbidden to them according to our access control policies.
	ErrForbidden = errors.New("Attempted action is not allowed")
)

// CanReadChecklist determines if claims has the authority to access the specified checklist by id.
func (repo *Repository) CanReadChecklist(ctx context.Context, claims auth.Claims, id string) error {

	// If the request has claims from a specific checklist, ensure that the claims
	// has the correct access to the checklist.
	if claims.Audience != "" {
		// select id from checklists where account_id = [accountID]
		query := sqlbuilder.NewSelectBuilder().Select("id").From(checklistTableName)
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
		// to the specified checklist.
		if id == "" {
			return errors.WithStack(ErrForbidden)
		}

	}

	return nil
}

// CanModifyChecklist determines if claims has the authority to modify the specified checklist by id.
func (repo *Repository) CanModifyChecklist(ctx context.Context, claims auth.Claims, id string) error {
	err := repo.CanReadChecklist(ctx, claims, id)
	if err != nil {
		return err
	}

	// Admin users can update checklists they have access to.
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

// checklistMapColumns is the list of columns needed for find.
var checklistMapColumns = "id,account_id,name,status,created_at,updated_at,archived_at"

// selectQuery constructs a base select query for Checklist.
func selectQuery() *sqlbuilder.SelectBuilder {
	query := sqlbuilder.NewSelectBuilder()
	query.Select(checklistMapColumns)
	query.From(checklistTableName)
	return query
}

// findRequestQuery generates the select query for the given find request.
// TODO: Need to figure out why can't parse the args when appending the where
// 			to the query.
func findRequestQuery(req ChecklistFindRequest) (*sqlbuilder.SelectBuilder, []interface{}) {
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

// Find gets all the checklists from the database based on the request params.
func (repo *Repository) Find(ctx context.Context, claims auth.Claims, req ChecklistFindRequest) (Checklists, error) {
	query, args := findRequestQuery(req)
	return find(ctx, claims, repo.DbConn, query, args, req.IncludeArchived)
}

// find internal method for getting all the checklists from the database using a select query.
func find(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, query *sqlbuilder.SelectBuilder, args []interface{}, includedArchived bool) (Checklists, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.checklist.Find")
	defer span.Finish()

	query.Select(checklistMapColumns)
	query.From(checklistTableName)
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
		err = errors.WithMessage(err, "find checklists failed")
		return nil, err
	}

	// Iterate over each row.
	resp := []*Checklist{}
	for rows.Next() {
		var (
			m   Checklist
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

// ReadByID gets the specified checklist by ID from the database.
func (repo *Repository) ReadByID(ctx context.Context, claims auth.Claims, id string) (*Checklist, error) {
	return repo.Read(ctx, claims, ChecklistReadRequest{
		ID:              id,
		IncludeArchived: false,
	})
}

// Read gets the specified checklist from the database.
func (repo *Repository) Read(ctx context.Context, claims auth.Claims, req ChecklistReadRequest) (*Checklist, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.checklist.Read")
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
		err = errors.WithMessagef(ErrNotFound, "checklist %s not found", req.ID)
		return nil, err
	}

	u := res[0]
	return u, nil
}

// Create inserts a new checklist into the database.
func (repo *Repository) Create(ctx context.Context, claims auth.Claims, req ChecklistCreateRequest, now time.Time) (*Checklist, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.checklist.Create")
	defer span.Finish()
	if claims.Audience != "" {
		// Admin users can update checklists they have access to.
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
	m := Checklist{
		ID:        uuid.NewRandom().String(),
		AccountID: req.AccountID,
		Name:      req.Name,
		Status:    ChecklistStatus_Active,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if req.Status != nil {
		m.Status = *req.Status
	}

	// Build the insert SQL statement.
	query := sqlbuilder.NewInsertBuilder()
	query.InsertInto(checklistTableName)
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
		err = errors.WithMessage(err, "create checklist failed")
		return nil, err
	}

	return &m, nil
}

// Update replaces an checklist in the database.
func (repo *Repository) Update(ctx context.Context, claims auth.Claims, req ChecklistUpdateRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.checklist.Update")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the checklist specified in the request.
	err = repo.CanModifyChecklist(ctx, claims, req.ID)
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
	query.Update(checklistTableName)
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
		err = errors.WithMessagef(err, "update checklist %s failed", req.ID)
		return err
	}

	return nil
}

// Archive soft deleted the checklist from the database.
func (repo *Repository) Archive(ctx context.Context, claims auth.Claims, req ChecklistArchiveRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.checklist.Archive")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the checklist specified in the request.
	err = repo.CanModifyChecklist(ctx, claims, req.ID)
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
	query.Update(checklistTableName)
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
		err = errors.WithMessagef(err, "archive checklist %s failed", req.ID)
		return err
	}

	return nil
}

// Delete removes an checklist from the database.
func (repo *Repository) Delete(ctx context.Context, claims auth.Claims, req ChecklistDeleteRequest) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.checklist.Delete")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the checklist specified in the request.
	err = repo.CanModifyChecklist(ctx, claims, req.ID)
	if err != nil {
		return err
	}

	// Build the delete SQL statement.
	query := sqlbuilder.NewDeleteBuilder()
	query.DeleteFrom(checklistTableName)
	query.Where(query.Equal("id", req.ID))
	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = repo.DbConn.Rebind(sql)
	_, err = repo.DbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "delete checklist %s failed", req.ID)
		return err
	}

	return nil
}
