package user_account

import (
	"context"
	"database/sql"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/account"
	"github.com/lib/pq"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/go-playground/validator.v9"
)

var (
	// ErrNotFound abstracts the mgo not found error.
	ErrNotFound = errors.New("Entity not found")

	// ErrInvalidID occurs when an ID is not in a valid form.
	ErrInvalidID = errors.New("ID is not in its proper form")

	// ErrForbidden occurs when a user tries to do something that is forbidden to them according to our access control policies.
	ErrForbidden = errors.New("Attempted action is not allowed")
)

// The database table for UserAccount
const userAccountTableName = "users_accounts"

// The list of columns needed for mapRowsToUserAccount
var userAccountMapColumns = "id,user_id,account_id,roles,status,created_at,updated_at,archived_at"

// mapRowsToUserAccount takes the SQL rows and maps it to the UserAccount struct
// with the columns defined by userAccountMapColumns
func mapRowsToUserAccount(rows *sql.Rows) (*UserAccount, error) {
	var (
		ua  UserAccount
		err error
	)
	err = rows.Scan(&ua.ID, &ua.UserID, &ua.AccountID, &ua.Roles, &ua.Status, &ua.CreatedAt, &ua.UpdatedAt, &ua.ArchivedAt)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &ua, nil
}

// CanReadAccount determines if claims has the authority to access the specified user account by user ID.
func CanReadAccount(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, accountID string) error {
	err := account.CanReadAccount(ctx, claims, dbConn, accountID)
	return mapAccountError(err)
}

// CanModifyAccount determines if claims has the authority to modify the specified user ID.
func CanModifyAccount(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, accountID string) error {
	err := account.CanModifyAccount(ctx, claims, dbConn, accountID)
	return mapAccountError(err)
}

// mapAccountError maps account errors to local defined errors.
func mapAccountError(err error) error {
	switch errors.Cause(err) {
	case account.ErrNotFound:
		err = ErrNotFound
	case account.ErrInvalidID:
		err = ErrInvalidID
	case account.ErrForbidden:
		err = ErrForbidden
	}
	return err
}

// applyClaimsSelect applies a sub-query to the provided query
// to enforce ACL based on the claims provided.
// 	1. All role types can access their user ID
// 	2. Any user with the same account ID
// 	3. No claims, request is internal, no ACL applied
func applyClaimsSelect(ctx context.Context, claims auth.Claims, query *sqlbuilder.SelectBuilder) error {
	if claims.Audience == "" && claims.Subject == "" {
		return nil
	}

	// Build select statement for users_accounts table
	subQuery := sqlbuilder.NewSelectBuilder().Select("id").From(userAccountTableName)

	var or []string
	if claims.Audience != "" {
		or = append(or, subQuery.Equal("account_id", claims.Audience))
	}
	if claims.Subject != "" {
		or = append(or, subQuery.Equal("user_id", claims.Subject))
	}
	subQuery.Where(subQuery.Or(or...))

	// Append sub query
	query.Where(query.In("id", subQuery))

	return nil
}

// selectQuery constructs a base select query for User Account
func selectQuery() *sqlbuilder.SelectBuilder {
	query := sqlbuilder.NewSelectBuilder()
	query.Select(userAccountMapColumns)
	query.From(userAccountTableName)
	return query
}

// findRequestQuery generates the select query for the given find request.
// TODO: Need to figure out why can't parse the args when appending the where
// 			to the query.
func findRequestQuery(req UserAccountFindRequest) (*sqlbuilder.SelectBuilder, []interface{}) {
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

// Find gets all the user accounts from the database based on the request params.
func Find(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UserAccountFindRequest) ([]*UserAccount, error) {
	query, args := findRequestQuery(req)
	return find(ctx, claims, dbConn, query, args, req.IncludedArchived)
}

// Find gets all the user accounts from the database based on the select query
func find(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, query *sqlbuilder.SelectBuilder, args []interface{}, includedArchived bool) ([]*UserAccount, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.Find")
	defer span.Finish()

	query.Select(userAccountMapColumns)
	query.From(userAccountTableName)

	if !includedArchived {
		query.Where(query.IsNull("archived_at"))
	}

	// Check to see if a sub query needs to be applied for the claims
	err := applyClaimsSelect(ctx, claims, query)
	if err != nil {
		return nil, err
	}
	queryStr, queryArgs := query.Build()
	queryStr = dbConn.Rebind(queryStr)
	args = append(args, queryArgs...)

	// fetch all places from the db
	rows, err := dbConn.QueryContext(ctx, queryStr, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessage(err, "find user accounts failed")
		return nil, err
	}

	// iterate over each row
	resp := []*UserAccount{}
	for rows.Next() {
		ua, err := mapRowsToUserAccount(rows)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			return nil, err
		}
		resp = append(resp, ua)
	}

	return resp, nil
}

// Retrieve gets the specified user from the database.
func FindByUserID(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, userID string, includedArchived bool) ([]*UserAccount, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.FindByUserID")
	defer span.Finish()

	// Filter base select query by ID
	query := sqlbuilder.NewSelectBuilder()
	query.Where(query.Equal("user_id", userID))
	query.OrderBy("created_at")

	// Execute the find accounts method.
	res, err := find(ctx, claims, dbConn, query, []interface{}{}, includedArchived)
	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		err = errors.WithMessagef(ErrNotFound, "no accounts for user %s found", userID)
		return nil, err
	}

	return res, nil
}

// Create a user account for a given user with specified roles.
func Create(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req CreateUserAccountRequest, now time.Time) (*UserAccount, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.Create")
	defer span.Finish()

	// Validate the request.
	err := validator.New().Struct(req)
	if err != nil {
		return nil, err
	}

	// Ensure the claims can modify the account specified in the request.
	err = CanModifyAccount(ctx, claims, dbConn, req.AccountID)
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

	// Check to see if there is an existing user account, including archived.
	existQuery := selectQuery()
	existQuery.Where(existQuery.And(
		existQuery.Equal("account_id", req.AccountID),
		existQuery.Equal("user_id", req.UserID),
	))
	existing, err := find(ctx, claims, dbConn, existQuery, []interface{}{}, true)
	if err != nil {
		return nil, err
	}

	// If there is an existing entry, then update instead of insert.
	var ua UserAccount
	if len(existing) > 0 {
		upReq := UpdateUserAccountRequest{
			UserID:    req.UserID,
			AccountID: req.AccountID,
			Roles:     &req.Roles,
			unArchive: true,
		}
		err = Update(ctx, claims, dbConn, upReq, now)
		if err != nil {
			return nil, err
		}

		ua = *existing[0]
		ua.Roles = req.Roles
		ua.UpdatedAt = now
		ua.ArchivedAt = pq.NullTime{}
	} else {
		ua = UserAccount{
			ID:        uuid.NewRandom().String(),
			UserID:    req.UserID,
			AccountID: req.AccountID,
			Roles:     req.Roles,
			Status:    UserAccountStatus_Active,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if req.Status != nil {
			ua.Status = *req.Status
		}

		// Build the insert SQL statement.
		query := sqlbuilder.NewInsertBuilder()
		query.InsertInto(userAccountTableName)
		query.Cols("id", "user_id", "account_id", "roles", "status", "created_at", "updated_at")
		query.Values(ua.ID, ua.UserID, ua.AccountID, ua.Roles, ua.Status.String(), ua.CreatedAt, ua.UpdatedAt)

		// Execute the query with the provided context.
		sql, args := query.Build()
		sql = dbConn.Rebind(sql)
		_, err = dbConn.ExecContext(ctx, sql, args...)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			err = errors.WithMessagef(err, "add account %s to user %s failed", req.AccountID, req.UserID)
			return nil, err
		}
	}

	return &ua, nil
}

// Read gets the specified user account from the database.
func Read(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, id string, includedArchived bool) (*UserAccount, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.Read")
	defer span.Finish()

	// Filter base select query by ID
	query := selectQuery()
	query.Where(query.Equal("id", id))

	res, err := find(ctx, claims, dbConn, query, []interface{}{}, includedArchived)
	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		err = errors.WithMessagef(ErrNotFound, "user account %s not found", id)
		return nil, err
	}
	u := res[0]

	return u, nil
}

// Update replaces a user account in the database.
func Update(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UpdateUserAccountRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.Update")
	defer span.Finish()

	// Validate the request.
	err := validator.New().Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the user specified in the request.
	err = CanModifyAccount(ctx, claims, dbConn, req.AccountID)
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
	query.Update(userAccountTableName)

	fields := []string{}
	if req.Roles != nil {
		fields = append(fields, query.Assign("roles", req.Roles))
	}
	if req.Status != nil {
		fields = append(fields, query.Assign("status", req.Status))
	}
	if req.unArchive {
		fields = append(fields, query.Assign("archived_at", nil))
	}

	// If there's nothing to update we can quit early.
	if len(fields) == 0 {
		return nil
	}

	// Append the updated_at field
	fields = append(fields, query.Assign("updated_at", now))

	query.Set(fields...)

	query.Where(query.And(
		query.Equal("user_id", req.UserID),
		query.Equal("account_id", req.AccountID),
	))

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)
	_, err = dbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "update account %s for user %s failed", req.AccountID, req.UserID)
		return err
	}

	return nil
}

// Archive soft deleted the user account from the database.
func Archive(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req ArchiveUserAccountRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.Archive")
	defer span.Finish()

	// Validate the request.
	err := validator.New().Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the user specified in the request.
	err = CanModifyAccount(ctx, claims, dbConn, req.AccountID)
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
	query.Update(userAccountTableName)
	query.Set(query.Assign("archived_at", now))
	query.Where(query.And(
		query.Equal("user_id", req.UserID),
		query.Equal("account_id", req.AccountID),
	))

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)
	_, err = dbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "archive account %s from user %s failed", req.AccountID, req.UserID)
		return err
	}

	return nil
}

// Delete removes a user account from the database.
func Delete(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req DeleteUserAccountRequest) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.Delete")
	defer span.Finish()

	// Validate the request.
	err := validator.New().Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the user specified in the request.
	err = CanModifyAccount(ctx, claims, dbConn, req.AccountID)
	if err != nil {
		return err
	}

	// Build the delete SQL statement.
	query := sqlbuilder.NewDeleteBuilder()
	query.DeleteFrom(userAccountTableName)
	query.Where(query.And(
		query.Equal("user_id", req.UserID),
		query.Equal("account_id", req.AccountID),
	))

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)
	_, err = dbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "delete account %s for user %s failed", req.AccountID, req.UserID)
		return err
	}

	return nil
}
