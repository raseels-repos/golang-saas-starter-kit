package user

import (
	"context"
	"database/sql"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/go-playground/validator.v9"
)

// The database table for User
const usersTableName = "users"

var (
	// ErrNotFound abstracts the mgo not found error.
	ErrNotFound = errors.New("Entity not found")

	// ErrInvalidID occurs when an ID is not in a valid form.
	ErrInvalidID = errors.New("ID is not in its proper form")

	// ErrAuthenticationFailure occurs when a user attempts to authenticate but
	// anything goes wrong.
	ErrAuthenticationFailure = errors.New("Authentication failed")

	// ErrForbidden occurs when a user tries to do something that is forbidden to them according to our access control policies.
	ErrForbidden = errors.New("Attempted action is not allowed")
)

// usersMapColumns is the list of columns needed for mapRowsToUser
var usersMapColumns = "id,name,email,password_salt,password_hash,password_reset,status,timezone,created_at,updated_at,archived_at"

// mapRowsToUser takes the SQL rows and maps it to the UserAccount struct
// with the columns defined by usersMapColumns
func mapRowsToUser(rows *sql.Rows) (*User, error) {
	var (
		u   User
		err error
	)
	err = rows.Scan(&u.ID, &u.Email, &u.PasswordSalt, &u.PasswordHash, &u.PasswordReset, &u.Status, &u.Timezone, &u.CreatedAt, &u.UpdatedAt, &u.ArchivedAt)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &u, nil
}

// CanReadUserId determines if claims has the authority to access the specified user ID.
func CanReadUserId(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, userID string) error {
	// If the request has claims from a specific user, ensure that the user
	// has the correct access to the user.
	if claims.Subject != "" {
		// When the claims Subject - UserId - does not match the requested user, the
		// claims audience - AccountId - should have a record.
		if claims.Subject != userID {

			query := sqlbuilder.NewSelectBuilder().Select("id").From(usersAccountsTableName)
			query.Where(query.And(
				query.Equal("account_id", claims.Audience),
				query.Equal("user_id", userID),
			))
			sql, args := query.Build()
			sql = dbConn.Rebind(sql)

			var userAccountId string
			err := dbConn.QueryRowContext(ctx, sql, args...).Scan(&userAccountId)
			if err != nil {
				err = errors.Wrapf(err, "query - %s", query.String())
				return err
			}

			// When there is now userAccount ID returned, then the current user does not have access
			// to the specified user.
			if userAccountId == "" {
				return errors.WithStack(ErrForbidden)
			}
		}
	}

	return nil
}

// CanModifyUserId determines if claims has the authority to modify the specified user ID.
func CanModifyUserId(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, userID string) error {
	// First check to see if claims can read the user ID
	err := CanReadUserId(ctx, claims, dbConn, userID)
	if err != nil {
		return err
	}

	// If the request has claims from a specific user, ensure that the user
	// has the correct role for updating an existing user.
	if claims.Subject != "" {
		if claims.Subject == userID {
			// All users are allowed to update their own record
		} else if claims.HasRole(auth.RoleAdmin) {
			// Admin users can update users they have access to.
		} else {
			return errors.WithStack(ErrForbidden)
		}
	}

	return nil
}

// claimsSql applies a sub-query to the provided query to enforce ACL based on
// the claims provided.
// 	1. All role types can access their user ID
// 	2. Any user with the same account ID
// 	3. No claims, request is internal, no ACL applied
func applyClaimsUserSelect(ctx context.Context, claims auth.Claims, query *sqlbuilder.SelectBuilder) error {
	// Claims are empty, don't apply any ACL
	if claims.Audience == "" && claims.Subject == "" {
		return nil
	}

	// Build select statement for users_accounts table
	subQuery := sqlbuilder.NewSelectBuilder().Select("user_id").From(usersAccountsTableName)

	var or []string
	if claims.Audience != "" {
		or = append(or, subQuery.Equal("account_id", claims.Audience))
	}
	if claims.Subject != "" {
		or = append(or, subQuery.Equal("user_id", claims.Subject))
	}
	subQuery.Where(or...)

	// Append sub query
	query.Where(query.In("id", subQuery))

	return nil
}

// selectQuery constructs a base select query for User
func selectQuery() *sqlbuilder.SelectBuilder {
	query := sqlbuilder.NewSelectBuilder()
	query.Select(usersMapColumns)
	query.From(usersTableName)
	return query
}

// userFindRequestQuery generates the select query for the given find request.
func userFindRequestQuery(req UserFindRequest) *sqlbuilder.SelectBuilder {
	query := selectQuery()
	if req.Where != nil {
		query.Where(*req.Where)
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

	b := sqlbuilder.Buildf(query.String(), req.Args...)
	query.BuilderAs(b, usersTableName)

	return query
}

// List enables streaming retrieval of Users from the database. The query results
// will be written to the interface{} resultReceiver channel enabling processing the results while
// they're still being fetched. After all pages have been processed the channel is closed
// Possible types sent to the channel are limited to:
// 		- error
//		- User
//
//	rr := make(chan interface{})
//
//	go List(ctx, claims, db, rr)
//
//	for r := range rr {
//		switch v := r.(type) {
//		case User:
//			// v is of type User
//			// process the user here
//		case error:
//			// v is of type error
//			// handle the error here
//		}
//	}
func List(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UserFindRequest, results chan<- interface{}) {
	query := userFindRequestQuery(req)
	list(ctx, claims, dbConn, query, req.IncludedArchived, results)
}

// List enables streaming retrieval of Users from the database for the supplied query.
func list(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, query *sqlbuilder.SelectBuilder, includedArchived bool, results chan<- interface{}) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.List")
	defer span.Finish()

	// Close the channel on complete
	defer close(results)

	query.Select(usersMapColumns)
	query.From(usersTableName)

	if !includedArchived {
		query.Where(query.IsNull("archived_at"))
	}

	// Check to see if a sub query needs to be applied for the claims
	err := applyClaimsUserSelect(ctx, claims, query)
	if err != nil {
		results <- err
		return
	}
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)

	// fetch all places from the db
	rows, err := dbConn.QueryContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		results <- errors.WithMessage(err, "list users failed")
		return
	}

	// iterate over each row
	for rows.Next() {
		u, err := mapRowsToUser(rows)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			results <- err
			return
		}
		results <- u
	}
}

// Find gets all the users from the database based on the request params
func Find(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UserFindRequest) ([]*User, error) {
	query := userFindRequestQuery(req)
	return find(ctx, claims, dbConn, query, req.IncludedArchived)
}

// find gets all the users from the database based on the query
func find(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, query *sqlbuilder.SelectBuilder, includedArchived bool) ([]*User, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Find")
	defer span.Finish()

	query.Select(usersMapColumns)
	query.From(usersTableName)

	if !includedArchived {
		query.Where(query.IsNull("archived_at"))
	}

	// Check to see if a sub query needs to be applied for the claims
	err := applyClaimsUserSelect(ctx, claims, query)
	if err != nil {
		return nil, err
	}
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)

	// fetch all places from the db
	rows, err := dbConn.QueryContext(ctx, sql, args...)
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

// Retrieve gets the specified user from the database.
func FindById(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, id string, includedArchived bool) (*User, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.FindById")
	defer span.Finish()

	// Filter base select query by ID
	query := selectQuery()
	query.Where(query.Equal("id", id))

	res, err := find(ctx, claims, dbConn, query, includedArchived)
	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		err = errors.WithMessagef(ErrNotFound, "user %s not found", id)
		return nil, err
	}
	u := res[0]

	return u, nil
}

// Validation an email address is unique excluding the current user ID.
func uniqueEmail(ctx context.Context, dbConn *sqlx.DB, email, userId string) (bool, error) {

	query := sqlbuilder.NewSelectBuilder().Select("id").From(usersTableName)
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
func Create(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req CreateUserRequest, now time.Time) (*User, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Create")
	defer span.Finish()

	v := validator.New()

	// Validation email address is unique in the database.
	uniq, err := uniqueEmail(ctx, dbConn, req.Email, "")
	if err != nil {
		return nil, err
	}
	f := func(fl validator.FieldLevel) bool {
		if fl.Field().String() == "invalid" {
			return false
		}
		return uniq
	}
	v.RegisterValidation("unique", f)

	// Validate the request.
	err = v.Struct(req)
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
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: passwordHash,
		PasswordSalt: passwordSalt,
		Status:       UserStatus_Active,
		Timezone:     "America/Anchorage",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if req.Status != nil {
		u.Status = *req.Status
	}
	if req.Timezone != nil {
		u.Timezone = *req.Timezone
	}

	// Build the insert SQL statement.
	query := sqlbuilder.NewInsertBuilder()
	query.InsertInto(usersTableName)
	query.Cols("id", "name", "email", "password_hash", "password_salt", "status", "timezone", "created_at", "updated_at")
	query.Values(u.ID, u.Name, u.Email, u.PasswordHash, u.PasswordSalt, u.Status.String(), u.Timezone, u.CreatedAt, u.UpdatedAt)

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

// Update replaces a user in the database.
func Update(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UpdateUserRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Update")
	defer span.Finish()

	v := validator.New()

	// Validation email address is unique in the database.
	if req.Email != nil {
		uniq, err := uniqueEmail(ctx, dbConn, *req.Email, req.ID)
		if err != nil {
			return err
		}
		f := func(fl validator.FieldLevel) bool {
			if fl.Field().String() == "invalid" {
				return false
			}
			return uniq
		}
		v.RegisterValidation("unique", f)
	}

	// Validate the request.
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the user specified in the request.
	err = CanModifyUserId(ctx, claims, dbConn, req.ID)
	if err != nil {
		err = errors.WithMessagef(err, "Update %s failed", usersTableName)
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
	query.Update(usersTableName)

	fields := []string{}
	if req.Name != nil {
		fields = append(fields, query.Assign("name", req.Name))
	}
	if req.Email != nil {
		fields = append(fields, query.Assign("email", req.Email))
	}
	if req.Status != nil {
		fields = append(fields, query.Assign("status", req.Status))
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

// Update replaces a user in the database.
func UpdatePassword(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UpdatePasswordRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Update")
	defer span.Finish()

	// Validate the request.
	err := validator.New().Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the user specified in the request.
	err = CanModifyUserId(ctx, claims, dbConn, req.ID)
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
	query.Update(usersTableName)
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
func Archive(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, userID string, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Archive")
	defer span.Finish()

	// Defines the struct to apply validation
	req := struct {
		ID string `validate:"required,uuid"`
	}{
		ID: userID,
	}

	// Validate the request.
	err := validator.New().Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the user specified in the request.
	err = CanModifyUserId(ctx, claims, dbConn, req.ID)
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
	query.Update(usersTableName)
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
		query.Update(usersAccountsTableName)
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

// Delete removes a user from the database.
func Delete(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, userID string) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user.Delete")
	defer span.Finish()

	// Defines the struct to apply validation
	req := struct {
		ID string `validate:"required,uuid"`
	}{
		ID: userID,
	}

	// Validate the request.
	err := validator.New().Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the user specified in the request.
	err = CanModifyUserId(ctx, claims, dbConn, req.ID)
	if err != nil {
		return err
	}

	// Build the delete SQL statement.
	query := sqlbuilder.NewDeleteBuilder()
	query.DeleteFrom(usersTableName)
	query.Where(query.Equal("id", req.ID))

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = dbConn.Rebind(sql)
	_, err = dbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "delete user %s failed", req.ID)
		return err
	}

	// Delete all the associated user accounts
	{
		// Build the delete SQL statement.
		query := sqlbuilder.NewDeleteBuilder()
		query.DeleteFrom(usersAccountsTableName)
		query.Where(query.And(
			query.Equal("user_id", req.ID),
		))

		// Execute the query with the provided context.
		sql, args := query.Build()
		sql = dbConn.Rebind(sql)
		_, err = dbConn.ExecContext(ctx, sql, args...)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			err = errors.WithMessagef(err, "delete accounts for user %s failed", req.ID)
			return err
		}
	}

	return nil
}
