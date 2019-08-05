package user

import (
	"context"
	"database/sql"
	"github.com/sudo-suhas/symcrypto"
	"strconv"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/notify"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	// The database table for User
	userTableName = "users"
	// The database table for Account
	accountTableName = "accounts"
	// The database table for User Account
	userAccountTableName = "users_accounts"
)

var (
	// ErrNotFound abstracts the mgo not found error.
	ErrNotFound = errors.New("Entity not found")

	// ErrForbidden occurs when a user tries to do something that is forbidden to them according to our access control policies.
	ErrForbidden = errors.New("Attempted action is not allowed")

	// ErrResetExpired occurs when the the reset hash exceeds the expiration.
	ErrResetExpired = errors.New("Reset expired")
)

// userMapColumns is the list of columns needed for mapRowsToUser
var userMapColumns = "id,first_name,last_name,email,password_salt,password_hash,password_reset,timezone,created_at,updated_at,archived_at"

// mapRowsToUser takes the SQL rows and maps it to the UserAccount struct
// with the columns defined by userMapColumns
func mapRowsToUser(rows *sql.Rows) (*User, error) {
	var (
		u   User
		err error
	)
	err = rows.Scan(&u.ID, &u.FirstName, &u.LastName, &u.Email, &u.PasswordSalt, &u.PasswordHash, &u.PasswordReset, &u.Timezone, &u.CreatedAt, &u.UpdatedAt, &u.ArchivedAt)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &u, nil
}

// CanReadUser determines if claims has the authority to access the specified user ID.
func CanReadUser(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, userID string) error {
	// If the request has claims from a specific user, ensure that the user
	// has the correct access to the user.
	if claims.Subject != "" && claims.Subject != userID {
		// When the claims Subject/UserId - does not match the requested user, the
		// claims audience - AccountId - should have a record.
		// select id from users_accounts where account_id = [claims.Audience] and user_id = [userID]
		query := sqlbuilder.NewSelectBuilder().Select("id").From(userAccountTableName)
		query.Where(query.And(
			query.Equal("account_id", claims.Audience),
			query.Equal("user_id", userID),
		))
		queryStr, args := query.Build()
		queryStr = dbConn.Rebind(queryStr)

		var userAccountId string
		err := dbConn.QueryRowContext(ctx, queryStr, args...).Scan(&userAccountId)
		if err != nil && err != sql.ErrNoRows {
			err = errors.Wrapf(err, "query - %s", query.String())
			return err
		}

		// When there is no userAccount ID returned, then the current user does not have access
		// to the specified user.
		if userAccountId == "" {
			return errors.WithStack(ErrForbidden)
		}
	}

	return nil
}

// CanModifyUser determines if claims has the authority to modify the specified user ID.
func CanModifyUser(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, userID string) error {
	// If the request has claims from a specific user, ensure that the user
	// has the correct role for creating a new user.
	if claims.Subject != "" && claims.Subject != userID {
		// Users with the role of admin are ony allows to create users.
		if !claims.HasRole(auth.RoleAdmin) {
			err := errors.WithStack(ErrForbidden)
			return err
		}
	}

	if err := CanReadUser(ctx, claims, dbConn, userID); err != nil {
		return err
	}

	// TODO: Review, this doesn't seem correct, replaced with above.
	/*
		// If the request has claims from a specific account, ensure that the user
		// has the correct access to the account.
		if claims.Subject != "" && claims.Subject != userID {
			// When the claims Audience - AccountID - does not match the requested account, the
			// claims Audience - AccountID - should have a record with an admin role.
			// select id from users_accounts where  account_id = [claims.Audience] and user_id = [userID] and any (roles) = 'admin'
			query := sqlbuilder.NewSelectBuilder().Select("id").From(userAccountTableName)
			query.Where(query.And(
				query.Equal("account_id", claims.Audience),
				query.Equal("user_id", userID),
				"'"+auth.RoleAdmin+"' = ANY (roles)",
			))
			queryStr, args := query.Build()
			queryStr = dbConn.Rebind(queryStr)

			var userAccountId string
			err := dbConn.QueryRowContext(ctx, queryStr, args...).Scan(&userAccountId)
			if err != nil && err != sql.ErrNoRows {
				err = errors.Wrapf(err, "query - %s", query.String())
				return err
			}

			// When there is no userAccount ID returned, then the current user does not have access
			// to the specified account.
			if userAccountId == "" {
				return errors.WithStack(ErrForbidden)
			}
		}
	*/

	return nil
}

// applyClaimsSelect applies a sub-query to the provided query to enforce ACL based on
// the claims provided.
// 	1. All role types can access their user ID
// 	2. Any user with the same account ID
// 	3. No claims, request is internal, no ACL applied
func applyClaimsSelect(ctx context.Context, claims auth.Claims, query *sqlbuilder.SelectBuilder) error {
	// Claims are empty, don't apply any ACL
	if claims.Audience == "" && claims.Subject == "" {
		return nil
	}

	// Build select statement for users_accounts table
	subQuery := sqlbuilder.NewSelectBuilder().Select("user_id").From(userAccountTableName)

	var or []string
	if claims.Audience != "" {
		or = append(or, subQuery.Equal("account_id", claims.Audience))
	}
	if claims.Subject != "" {
		or = append(or, subQuery.Equal("user_id", claims.Subject))
	}

	// Append sub query
	if len(or) > 0 {
		subQuery.Where(subQuery.Or(or...))
		query.Where(query.In("id", subQuery))
	}

	return nil
}

// selectQuery constructs a base select query for User
func selectQuery() *sqlbuilder.SelectBuilder {
	query := sqlbuilder.NewSelectBuilder()
	query.Select(userMapColumns)
	query.From(userTableName)
	return query
}

// findRequestQuery generates the select query for the given find request.
// TODO: Need to figure out why can't parse the args when appending the where
// 			to the query.
func findRequestQuery(req UserFindRequest) (*sqlbuilder.SelectBuilder, []interface{}) {
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

// Find gets all the users from the database based on the request params.
func Find(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UserFindRequest) (Users, error) {
	query, args := findRequestQuery(req)
	return find(ctx, claims, dbConn, query, args, req.IncludeArchived)
}

// find internal method for getting all the users from the database using a select query.
func find(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, query *sqlbuilder.SelectBuilder, args []interface{}, includedArchived bool) (Users, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Find")
	defer span.Finish()

	query.Select(userMapColumns)
	query.From(userTableName)

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
		err = errors.WithMessage(err, "find users failed")
		return nil, err
	}

	// iterate over each row
	resp := []*User{}
	for rows.Next() {
		u, err := mapRowsToUser(rows)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			return nil, err
		}
		resp = append(resp, u)
	}

	return resp, nil
}

// Validation an email address is unique excluding the current user ID.
func UniqueEmail(ctx context.Context, dbConn *sqlx.DB, email, userId string) (bool, error) {
	query := sqlbuilder.NewSelectBuilder().Select("id").From(userTableName)
	query.Where(query.And(
		query.Equal("email", email),
		query.NotEqual("id", userId),
	))
	queryStr, args := query.Build()
	queryStr = dbConn.Rebind(queryStr)

	var existingId string
	err := dbConn.QueryRowContext(ctx, queryStr, args...).Scan(&existingId)
	if err != nil && err != sql.ErrNoRows {
		err = errors.Wrapf(err, "query - %s", query.String())
		return false, err
	}

	// When an ID was found in the db, the email is not unique.
	if existingId != "" {
		return false, nil
	}

	return true, nil
}

// Create inserts a new user into the database.
func Create(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UserCreateRequest, now time.Time) (*User, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Create")
	defer span.Finish()

	v := webcontext.Validator()

	// Validation email address is unique in the database.
	uniq, err := UniqueEmail(ctx, dbConn, req.Email, "")
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, webcontext.KeyTagUnique, uniq)

	// Validate the request.
	err = v.StructCtx(ctx, req)
	if err != nil {
		return nil, err
	}

	// If the request has claims from a specific user, ensure that the user
	// has the correct role for creating a new user.
	if claims.Subject != "" {
		// Users with the role of admin are ony allows to create users.
		if !claims.HasRole(auth.RoleAdmin) {
			err = errors.WithStack(ErrForbidden)
			return nil, err
		}
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

	passwordSalt := uuid.NewRandom().String()
	saltedPassword := req.Password + passwordSalt

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(saltedPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.Wrap(err, "generating password hash")
	}

	u := User{
		ID:           uuid.NewRandom().String(),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Email:        req.Email,
		PasswordHash: passwordHash,
		PasswordSalt: passwordSalt,
		Timezone:     "America/Anchorage",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if req.Timezone != nil {
		u.Timezone = *req.Timezone
	}

	// Build the insert SQL statement.
	query := sqlbuilder.NewInsertBuilder()
	query.InsertInto(userTableName)
	query.Cols("id", "first_name", "last_name", "email", "password_hash", "password_salt", "timezone", "created_at", "updated_at")
	query.Values(u.ID, u.FirstName, u.LastName, u.Email, u.PasswordHash, u.PasswordSalt, u.Timezone, u.CreatedAt, u.UpdatedAt)

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)
	_, err = dbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessage(err, "create user failed")
		return nil, err
	}

	return &u, nil
}

// Create invite inserts a new user into the database.
func CreateInvite(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UserCreateInviteRequest, now time.Time) (*User, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.CreateInvite")
	defer span.Finish()

	v := webcontext.Validator()

	// Validation email address is unique in the database.
	uniq, err := UniqueEmail(ctx, dbConn, req.Email, "")
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, webcontext.KeyTagUnique, uniq)

	// Validate the request.
	err = v.StructCtx(ctx, req)
	if err != nil {
		return nil, err
	}

	// If the request has claims from a specific user, ensure that the user
	// has the correct role for creating a new user.
	if claims.Subject != "" {
		// Users with the role of admin are ony allows to create users.
		if !claims.HasRole(auth.RoleAdmin) {
			err = errors.WithStack(ErrForbidden)
			return nil, err
		}
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

	u := User{
		ID:        uuid.NewRandom().String(),
		Email:     req.Email,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Build the insert SQL statement.
	query := sqlbuilder.NewInsertBuilder()
	query.InsertInto(userTableName)
	query.Cols("id", "email", "password_hash", "password_salt", "created_at", "updated_at")
	query.Values(u.ID, u.Email, "", "", u.CreatedAt, u.UpdatedAt)

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)
	_, err = dbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessage(err, "create user failed")
		return nil, err
	}

	return &u, nil
}

// ReadByID gets the specified user by ID from the database.
func ReadByID(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, id string) (*User, error) {
	return Read(ctx, claims, dbConn, UserReadRequest{
		ID:              id,
		IncludeArchived: false,
	})
}

// Read gets the specified user from the database.
func Read(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UserReadRequest) (*User, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Read")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return nil, err
	}

	// Filter base select query by ID
	query := selectQuery()
	query.Where(query.Equal("id", req.ID))

	res, err := find(ctx, claims, dbConn, query, []interface{}{}, req.IncludeArchived)
	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		err = errors.WithMessagef(ErrNotFound, "user %s not found", req.ID)
		return nil, err
	}
	u := res[0]

	return u, nil
}

// ReadByEmail gets the specified user from the database.
func ReadByEmail(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, email string, includedArchived bool) (*User, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.ReadByEmail")
	defer span.Finish()

	// Filter base select query by ID
	query := selectQuery()
	query.Where(query.Equal("email", email))

	res, err := find(ctx, claims, dbConn, query, []interface{}{}, includedArchived)
	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		err = errors.WithMessagef(ErrNotFound, "user %s not found", email)
		return nil, err
	}
	u := res[0]

	return u, nil
}

// Update replaces a user in the database.
func Update(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UserUpdateRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Update")
	defer span.Finish()

	v := webcontext.Validator()

	// Validation email address is unique in the database.
	if req.Email != nil {
		// Validation email address is unique in the database.
		uniq, err := UniqueEmail(ctx, dbConn, *req.Email, req.ID)
		if err != nil {
			return err
		}
		ctx = context.WithValue(ctx, webcontext.KeyTagUnique, uniq)
	} else {
		ctx = context.WithValue(ctx, webcontext.KeyTagUnique, true)
	}

	// Validate the request.
	err := v.StructCtx(ctx, req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the user specified in the request.
	err = CanModifyUser(ctx, claims, dbConn, req.ID)
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
	query.Update(userTableName)

	var fields []string
	if req.FirstName != nil {
		fields = append(fields, query.Assign("first_name", req.FirstName))
	}
	if req.LastName != nil {
		fields = append(fields, query.Assign("last_name", req.LastName))
	}
	if req.Email != nil {
		fields = append(fields, query.Assign("email", req.Email))
	}
	if req.Timezone != nil {
		fields = append(fields, query.Assign("timezone", req.Timezone))
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
		err = errors.WithMessagef(err, "update user %s failed", req.ID)
		return err
	}

	return nil
}

// Update changes the password for a user in the database.
func UpdatePassword(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UserUpdatePasswordRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.UpdatePassword")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the user specified in the request.
	err = CanModifyUser(ctx, claims, dbConn, req.ID)
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

	// Generate new password hash for the provided password.
	passwordSalt := uuid.NewRandom()
	saltedPassword := req.Password + passwordSalt.String()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(saltedPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.Wrap(err, "generating password hash")
	}

	// Build the update SQL statement.
	query := sqlbuilder.NewUpdateBuilder()
	query.Update(userTableName)
	query.Set(
		query.Assign("password_hash", passwordHash),
		query.Assign("password_salt", passwordSalt),
		query.Assign("updated_at", now),
	)
	query.Where(query.Equal("id", req.ID))

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)
	_, err = dbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "update password for user %s failed", req.ID)
		return err
	}

	return nil
}

// Archive soft deleted the user from the database.
func Archive(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UserArchiveRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Archive")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the user specified in the request.
	err = CanModifyUser(ctx, claims, dbConn, req.ID)
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
	query.Update(userTableName)
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
		err = errors.WithMessagef(err, "archive user %s failed", req.ID)
		return err
	}

	// Archive all the associated user accounts
	{
		// Build the update SQL statement.
		query := sqlbuilder.NewUpdateBuilder()
		query.Update(userAccountTableName)
		query.Set(query.Assign("archived_at", now))
		query.Where(query.And(
			query.Equal("user_id", req.ID),
		))

		// Execute the query with the provided context.
		sql, args := query.Build()
		sql = dbConn.Rebind(sql)
		_, err = dbConn.ExecContext(ctx, sql, args...)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			err = errors.WithMessagef(err, "archive accounts for user %s failed", req.ID)
			return err
		}
	}

	return nil
}

// Restore undeletes the user from the database.
func Restore(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UserRestoreRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Restore")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the user specified in the request.
	err = CanModifyUser(ctx, claims, dbConn, req.ID)
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
	query.Update(userTableName)
	query.Set(
		query.Assign("archived_at", nil),
	)
	query.Where(query.Equal("id", req.ID))

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)
	_, err = dbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "unarchive user %s failed", req.ID)
		return err
	}

	return nil
}

// Delete removes a user from the database.
func Delete(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UserDeleteRequest) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Delete")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the user specified in the request.
	err = CanModifyUser(ctx, claims, dbConn, req.ID)
	if err != nil {
		return err
	}

	// Start a new transaction to handle rollbacks on error.
	tx, err := dbConn.Begin()
	if err != nil {
		return errors.WithStack(err)
	}

	// Delete all the associated user accounts.
	// Required to execute first to avoid foreign key constraints.
	{
		// Build the delete SQL statement.
		query := sqlbuilder.NewDeleteBuilder()
		query.DeleteFrom(userAccountTableName)
		query.Where(query.And(
			query.Equal("user_id", req.ID),
		))

		// Execute the query with the provided context.
		sql, args := query.Build()
		sql = dbConn.Rebind(sql)
		_, err = tx.ExecContext(ctx, sql, args...)
		if err != nil {
			tx.Rollback()

			err = errors.Wrapf(err, "query - %s", query.String())
			err = errors.WithMessagef(err, "delete accounts for user %s failed", req.ID)
			return err
		}
	}

	// Build the delete SQL statement.
	query := sqlbuilder.NewDeleteBuilder()
	query.DeleteFrom(userTableName)
	query.Where(query.Equal("id", req.ID))

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)
	_, err = tx.ExecContext(ctx, sql, args...)
	if err != nil {
		tx.Rollback()

		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "delete user %s failed", req.ID)
		return err
	}

	err = tx.Commit()
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// ResetPassword sends en email to the user to allow them to reset their password.
func ResetPassword(ctx context.Context, dbConn *sqlx.DB, resetUrl func(string) string, notify notify.Email, req UserResetPasswordRequest, secretKey string, now time.Time) (string, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.ResetPassword")
	defer span.Finish()

	v := webcontext.Validator()

	// Validate the request.
	err := v.StructCtx(ctx, req)
	if err != nil {
		return "", err
	}

	// Find user by email address.
	var u *User
	{
		query := selectQuery()
		query.Where(query.Equal("email", req.Email))

		res, err := find(ctx, auth.Claims{}, dbConn, query, []interface{}{}, false)
		if err != nil {
			return "", err
		} else if res == nil || len(res) == 0 {
			err = errors.WithMessagef(ErrNotFound, "No user found using '%s'.", req.Email)
			return "", err
		}
		u = res[0]
	}

	// Update the user with a random string used to confirm the password reset.
	resetId := uuid.NewRandom().String()
	{
		// Always store the time as UTC.
		now = now.UTC()

		// Postgres truncates times to milliseconds when storing. We and do the same
		// here so the value we return is consistent with what we store.
		now = now.Truncate(time.Millisecond)

		// Build the update SQL statement.
		query := sqlbuilder.NewUpdateBuilder()
		query.Update(userTableName)
		query.Set(
			query.Assign("password_reset", resetId),
			query.Assign("updated_at", now),
		)
		query.Where(query.Equal("id", u.ID))

		// Execute the query with the provided context.
		sql, args := query.Build()
		sql = dbConn.Rebind(sql)
		_, err = dbConn.ExecContext(ctx, sql, args...)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			err = errors.WithMessagef(err, "Update user %s failed.", u.ID)
			return "", err
		}
	}

	if req.TTL.Seconds() == 0 {
		req.TTL = time.Minute * 90
	}

	// Load the current IP makings the request.
	var requestIp string
	if vals, _ := webcontext.ContextValues(ctx); vals != nil {
		requestIp = vals.RequestIP
	}

	// Generate a string that embeds additional information.
	hashPts := []string{
		resetId,
		strconv.Itoa(int(now.UTC().Unix())),
		strconv.Itoa(int(now.UTC().Add(req.TTL).Unix())),
		requestIp,
	}
	hashStr := strings.Join(hashPts, "|")

	// This returns the nonce appended with the encrypted string for "hello world".
	crypto, err := symcrypto.New(secretKey)
	if err != nil {
		return "", errors.WithStack(err)
	}
	encrypted, err := crypto.Encrypt(hashStr)
	if err != nil {
		return "", errors.WithStack(err)
	}

	data := map[string]interface{}{
		"Name":    u.FirstName,
		"Url":     resetUrl(encrypted),
		"Minutes": req.TTL.Minutes(),
	}

	err = notify.Send(ctx, u.Email, "Reset your Password", "user_reset_password", data)
	if err != nil {
		err = errors.WithMessagef(err, "Send password reset email to %s failed.", u.Email)
		return "", err
	}

	return encrypted, nil
}

// ResetConfirm updates the password for a user using the provided reset password ID.
func ResetConfirm(ctx context.Context, dbConn *sqlx.DB, req UserResetConfirmRequest, secretKey string, now time.Time) (*User, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.ResetConfirm")
	defer span.Finish()

	v := webcontext.Validator()

	// Validate the request.
	err := v.StructCtx(ctx, req)
	if err != nil {
		return nil, err
	}

	crypto, err := symcrypto.New(secretKey)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	hashStr, err := crypto.Decrypt(req.ResetHash)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	hashPts := strings.Split(hashStr, "|")

	var hash ResetHash
	if len(hashPts) == 4 {
		hash.ResetID = hashPts[0]
		hash.CreatedAt, _ = strconv.Atoi(hashPts[1])
		hash.ExpiresAt, _ = strconv.Atoi(hashPts[2])
		hash.RequestIP = hashPts[3]
	}

	// Validate the hash.
	err = v.StructCtx(ctx, hash)
	if err != nil {
		return nil, err
	}

	if int64(hash.ExpiresAt) < now.UTC().Unix() {
		err = errors.WithMessage(ErrResetExpired, "Password reset has expired.")
		return nil, err
	}

	// Find user by password_reset.
	var u *User
	{
		query := selectQuery()
		query.Where(query.Equal("password_reset", hash.ResetID))

		res, err := find(ctx, auth.Claims{}, dbConn, query, []interface{}{}, false)
		if err != nil {
			return nil, err
		} else if res == nil || len(res) == 0 {
			err = errors.WithMessage(ErrNotFound, "Invalid password reset.")
			return nil, err
		}
		u = res[0]
	}

	// Save the new password for the user.
	{
		// Always store the time as UTC.
		now = now.UTC()

		// Postgres truncates times to milliseconds when storing. We and do the same
		// here so the value we return is consistent with what we store.
		now = now.Truncate(time.Millisecond)

		// Generate new password hash for the provided password.
		passwordSalt := uuid.NewRandom()
		saltedPassword := req.Password + passwordSalt.String()
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(saltedPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, errors.Wrap(err, "generating password hash")
		}

		// Build the update SQL statement.
		query := sqlbuilder.NewUpdateBuilder()
		query.Update(userTableName)
		query.Set(
			query.Assign("password_reset", nil),
			query.Assign("password_hash", passwordHash),
			query.Assign("password_salt", passwordSalt),
			query.Assign("updated_at", now),
		)
		query.Where(query.Equal("id", u.ID))

		// Execute the query with the provided context.
		sql, args := query.Build()
		sql = dbConn.Rebind(sql)
		_, err = dbConn.ExecContext(ctx, sql, args...)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			err = errors.WithMessagef(err, "update password for user %s failed", u.ID)
			return nil, err
		}
	}

	return u, nil
}

type MockUserResponse struct {
	*User
	Password string
}

// MockUser returns a fake User for testing.
func MockUser(ctx context.Context, dbConn *sqlx.DB, now time.Time) (*MockUserResponse, error) {
	pass := uuid.NewRandom().String()

	req := UserCreateRequest{
		FirstName:       "Lee",
		LastName:        "Brown",
		Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
		Password:        pass,
		PasswordConfirm: pass,
	}
	u, err := Create(ctx, auth.Claims{}, dbConn, req, now)
	if err != nil {
		return nil, err
	}

	return &MockUserResponse{
		User:     u,
		Password: pass,
	}, nil
}
