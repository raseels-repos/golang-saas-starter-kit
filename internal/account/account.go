package account

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
	// The database table for Account
	accountTableName = "accounts"
	// The database table for User Account
	userAccountTableName = "users_accounts"
	// The database table for AccountPreference
	accountPreferenceTableName = "account_preferences"
)

var (
	// ErrNotFound abstracts the mgo not found error.
	ErrNotFound = errors.New("Entity not found")

	// ErrForbidden occurs when a user tries to do something that is forbidden to them according to our access control policies.
	ErrForbidden = errors.New("Attempted action is not allowed")
)

// CanReadAccount determines if claims has the authority to access the specified account ID.
func CanReadAccount(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, accountID string) error {
	// If the request has claims from a specific account, ensure that the claims
	// has the correct access to the account.
	if claims.Audience != "" && claims.Audience != accountID {
		// When the claims Audience/AccountID does not match the requested account, the
		// claims Audience/AccountID - should have a record for the claims user.
		// select id from users_accounts where account_id = [accountID] and user_id = [claims.Subject]
		query := sqlbuilder.NewSelectBuilder().Select("id").From(userAccountTableName)
		query.Where(query.And(
			query.Equal("account_id", accountID),
			query.Equal("user_id", claims.Subject),
		))
		queryStr, args := query.Build()
		queryStr = dbConn.Rebind(queryStr)

		var userAccountId string
		err := dbConn.QueryRowContext(ctx, queryStr, args...).Scan(&userAccountId)
		if err != nil && err != sql.ErrNoRows {
			err = errors.Wrapf(err, "query - %s", query.String())
			return err
		}

		// When there is no userAccount ID returned, then the current claim user does not have access
		// to the specified account.
		if userAccountId == "" {
			return errors.WithStack(ErrForbidden)
		}
	}

	return nil
}

// CanReadAccount determines if claims has the authority to access the specified account ID.
func (repo *Repository) CanReadAccount(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, accountID string) error {
	return repo.CanReadAccount(ctx, claims, repo.DbConn, accountID)
}

// CanModifyAccount determines if claims has the authority to modify the specified account ID.
func CanModifyAccount(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, accountID string) error {
	// If the request has claims from a specific account, ensure that the claims
	// has the correct access to the account.
	if claims.Audience != "" {
		if claims.Audience == accountID {
			// Admin users can update accounts they have access to.
			if !claims.HasRole(auth.RoleAdmin) {
				return errors.WithStack(ErrForbidden)
			}
		} else {
			// When the claims Audience/AccountID does not match the requested account, the
			// claims Audience/AccountID should have a record with an admin role.
			// select id from users_accounts where account_id = [accountID] and user_id = [claims.Subject] and any (roles) = 'admin'
			query := sqlbuilder.NewSelectBuilder().Select("id").From(userAccountTableName)
			query.Where(query.And(
				query.Equal("account_id", accountID),
				query.Equal("user_id", claims.Subject),
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

			// When there is no userAccount ID returned, then the current claim user does not have access
			// to the specified account.
			if userAccountId == "" {
				return errors.WithStack(ErrForbidden)
			}
		}
	}

	return nil
}

// CanModifyAccount determines if claims has the authority to modify the specified account ID.
func (repo *Repository) CanModifyAccount(ctx context.Context, claims auth.Claims, accountID string) error {
	return CanModifyAccount(ctx, claims, repo.DbConn, accountID)
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
	subQuery := sqlbuilder.NewSelectBuilder().Select("account_id").From(userAccountTableName)

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

// accountMapColumns is the list of columns needed for find.
var accountMapColumns = "id,name,address1,address2,city,region,country,zipcode,status,timezone,signup_user_id,billing_user_id,created_at,updated_at,archived_at"

// selectQuery constructs a base select query for Account.
func selectQuery() *sqlbuilder.SelectBuilder {
	query := sqlbuilder.NewSelectBuilder()
	query.Select(accountMapColumns)
	query.From(accountTableName)
	return query
}

// Find gets all the accounts from the database based on the request params.
// TODO: Need to figure out why can't parse the args when appending the where
// 			to the query.
func (repo *Repository) Find(ctx context.Context, claims auth.Claims, req AccountFindRequest) (Accounts, error) {
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

	return find(ctx, claims, repo.DbConn, query, req.Args, req.IncludeArchived)
}

// find internal method for getting all the accounts from the database using a select query.
func find(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, query *sqlbuilder.SelectBuilder, args []interface{}, includedArchived bool) (Accounts, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.account.Find")
	defer span.Finish()

	query.Select(accountMapColumns)
	query.From(accountTableName)

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
		err = errors.WithMessage(err, "find accounts failed")
		return nil, err
	}
	defer rows.Close()

	// iterate over each row
	resp := []*Account{}
	for rows.Next() {
		var (
			a   Account
			err error
		)
		err = rows.Scan(&a.ID, &a.Name, &a.Address1, &a.Address2, &a.City, &a.Region, &a.Country, &a.Zipcode, &a.Status, &a.Timezone, &a.SignupUserID, &a.BillingUserID, &a.CreatedAt, &a.UpdatedAt, &a.ArchivedAt)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
		}
		resp = append(resp, &a)
	}

	err = rows.Err()
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessage(err, "find accounts failed")
		return nil, err
	}

	return resp, nil
}

// Validation an name is unique excluding the current account ID.
func UniqueName(ctx context.Context, dbConn *sqlx.DB, name, accountId string) (bool, error) {
	query := sqlbuilder.NewSelectBuilder().Select("id").From(accountTableName)
	query.Where(query.And(
		query.Equal("name", name),
		query.NotEqual("id", accountId),
	))
	queryStr, args := query.Build()
	queryStr = dbConn.Rebind(queryStr)

	var existingId string
	err := dbConn.QueryRowContext(ctx, queryStr, args...).Scan(&existingId)
	if err != nil && err != sql.ErrNoRows {
		err = errors.Wrapf(err, "query - %s", query.String())
		return false, err
	}

	// When an ID was found in the db, the name is not unique.
	if existingId != "" {
		return false, nil
	}

	return true, nil
}

// Create inserts a new account into the database.
func (repo *Repository) Create(ctx context.Context, claims auth.Claims, req AccountCreateRequest, now time.Time) (*Account, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.account.Create")
	defer span.Finish()

	v := webcontext.Validator()

	// Validation account name is unique in the database.
	uniq, err := UniqueName(ctx, repo.DbConn, req.Name, "")
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, webcontext.KeyTagUnique, uniq)

	// Validate the request.
	err = v.StructCtx(ctx, req)
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

	a := Account{
		ID:        uuid.NewRandom().String(),
		Name:      req.Name,
		Address1:  req.Address1,
		Address2:  req.Address2,
		City:      req.City,
		Region:    req.Region,
		Country:   req.Country,
		Zipcode:   req.Zipcode,
		Status:    AccountStatus_Pending,
		Timezone:  "America/Anchorage",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if req.Status != nil {
		a.Status = *req.Status
	}
	if req.Timezone != nil {
		a.Timezone = *req.Timezone
	}

	if req.SignupUserID != nil {
		a.SignupUserID = &sql.NullString{String: *req.SignupUserID, Valid: true}
	}
	if req.BillingUserID != nil {
		a.BillingUserID = &sql.NullString{String: *req.BillingUserID, Valid: true}
	}

	// Build the insert SQL statement.
	query := sqlbuilder.NewInsertBuilder()
	query.InsertInto(accountTableName)
	query.Cols("id", "name", "address1", "address2", "city", "region", "country", "zipcode", "status", "timezone", "signup_user_id", "billing_user_id", "created_at", "updated_at")
	query.Values(a.ID, a.Name, a.Address1, a.Address2, a.City, a.Region, a.Country, a.Zipcode, a.Status.String(), a.Timezone, a.SignupUserID, a.BillingUserID, a.CreatedAt, a.UpdatedAt)

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = repo.DbConn.Rebind(sql)
	_, err = repo.DbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessage(err, "create account failed")
		return nil, err
	}

	return &a, nil
}

// ReadByID gets the specified user by ID from the database.
func (repo *Repository) ReadByID(ctx context.Context, claims auth.Claims, id string) (*Account, error) {
	return repo.Read(ctx, claims, AccountReadRequest{
		ID:              id,
		IncludeArchived: false,
	})
}

// Read gets the specified account from the database.
func (repo *Repository) Read(ctx context.Context, claims auth.Claims, req AccountReadRequest) (*Account, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.account.Read")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return nil, err
	}

	// Filter base select query by ID
	query := sqlbuilder.NewSelectBuilder()
	query.Where(query.Equal("id", req.ID))

	res, err := find(ctx, claims, repo.DbConn, query, []interface{}{}, req.IncludeArchived)
	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		err = errors.WithMessagef(ErrNotFound, "account %s not found", req.ID)
		return nil, err
	}
	u := res[0]

	return u, nil
}

// Update replaces an account in the database.
func (repo *Repository) Update(ctx context.Context, claims auth.Claims, req AccountUpdateRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.account.Update")
	defer span.Finish()

	v := webcontext.Validator()

	if req.Name != nil {
		// Validation account name is unique in the database.
		uniq, err := UniqueName(ctx, repo.DbConn, *req.Name, req.ID)
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

	// Ensure the claims can modify the account specified in the request.
	err = CanModifyAccount(ctx, claims, repo.DbConn, req.ID)
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
	query.Update(accountTableName)

	var fields []string
	if req.Name != nil {
		fields = append(fields, query.Assign("name", req.Name))
	}
	if req.Address1 != nil {
		fields = append(fields, query.Assign("address1", req.Address1))
	}
	if req.Address2 != nil {
		fields = append(fields, query.Assign("address2", req.Address2))
	}
	if req.City != nil {
		fields = append(fields, query.Assign("city", req.City))
	}
	if req.Region != nil {
		fields = append(fields, query.Assign("region", req.Region))
	}
	if req.Country != nil {
		fields = append(fields, query.Assign("country", req.Country))
	}
	if req.Zipcode != nil {
		fields = append(fields, query.Assign("zipcode", req.Zipcode))
	}
	if req.Status != nil {
		fields = append(fields, query.Assign("status", req.Status))
	}
	if req.Timezone != nil {
		fields = append(fields, query.Assign("timezone", req.Timezone))
	}
	if req.SignupUserID != nil {
		if *req.SignupUserID != "" {
			fields = append(fields, query.Assign("signup_user_id", req.SignupUserID))
		} else {
			fields = append(fields, query.Assign("signup_user_id", nil))
		}

	}
	if req.BillingUserID != nil {
		if *req.BillingUserID != "" {
			fields = append(fields, query.Assign("billing_user_id", req.BillingUserID))
		} else {
			fields = append(fields, query.Assign("billing_user_id", nil))
		}
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
		err = errors.WithMessagef(err, "update account %s failed", req.ID)
		return err
	}

	return nil
}

// Archive soft deleted the account from the database.
func (repo *Repository) Archive(ctx context.Context, claims auth.Claims, req AccountArchiveRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.account.Archive")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the account specified in the request.
	err = CanModifyAccount(ctx, claims, repo.DbConn, req.ID)
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
	query.Update(accountTableName)
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
		err = errors.WithMessagef(err, "archive account %s failed", req.ID)
		return err
	}

	// Archive all the associated user accounts
	{
		// Build the update SQL statement.
		query := sqlbuilder.NewUpdateBuilder()
		query.Update(userAccountTableName)
		query.Set(query.Assign("archived_at", now))
		query.Where(query.And(
			query.Equal("account_id", req.ID),
		))

		// Execute the query with the provided context.
		sql, args := query.Build()
		sql = repo.DbConn.Rebind(sql)
		_, err = repo.DbConn.ExecContext(ctx, sql, args...)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			err = errors.WithMessagef(err, "archive users for account %s failed", req.ID)
			return err
		}
	}

	return nil
}

// Delete removes an account from the database.
func (repo *Repository) Delete(ctx context.Context, claims auth.Claims, req AccountDeleteRequest) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.account.Delete")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the account specified in the request.
	err = CanModifyAccount(ctx, claims, repo.DbConn, req.ID)
	if err != nil {
		return err
	}

	// Start a new transaction to handle rollbacks on error.
	tx, err := repo.DbConn.Begin()
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
			query.Equal("account_id", req.ID),
		))

		// Execute the query with the provided context.
		sql, args := query.Build()
		sql = repo.DbConn.Rebind(sql)
		_, err = tx.ExecContext(ctx, sql, args...)
		if err != nil {
			tx.Rollback()

			err = errors.Wrapf(err, "query - %s", query.String())
			err = errors.WithMessagef(err, "delete users for account %s failed", req.ID)
			return err
		}
	}

	// Delete all the associated account preferences.
	// Required to execute first to avoid foreign key constraints.
	{
		// Build the delete SQL statement.
		query := sqlbuilder.NewDeleteBuilder()
		query.DeleteFrom(accountPreferenceTableName)
		query.Where(query.And(
			query.Equal("account_id", req.ID),
		))

		// Execute the query with the provided context.
		sql, args := query.Build()
		sql = repo.DbConn.Rebind(sql)
		_, err = tx.ExecContext(ctx, sql, args...)
		if err != nil {
			tx.Rollback()

			err = errors.Wrapf(err, "query - %s", query.String())
			err = errors.WithMessagef(err, "delete preferences for account %s failed", req.ID)
			return err
		}
	}

	// Build the delete SQL statement.
	query := sqlbuilder.NewDeleteBuilder()
	query.DeleteFrom(accountTableName)
	query.Where(query.Equal("id", req.ID))

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = repo.DbConn.Rebind(sql)
	_, err = tx.ExecContext(ctx, sql, args...)
	if err != nil {
		tx.Rollback()

		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "delete account %s failed", req.ID)
		return err
	}

	err = tx.Commit()
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// MockAccount returns a fake Account for testing.
func MockAccount(ctx context.Context, dbConn *sqlx.DB, now time.Time) (*Account, error) {
	s := AccountStatus_Active

	repo := &Repository{
		DbConn: dbConn,
	}

	req := AccountCreateRequest{
		Name:     uuid.NewRandom().String(),
		Address1: "103 East Main St",
		Address2: "Unit 546",
		City:     "Valdez",
		Region:   "AK",
		Country:  "USA",
		Zipcode:  "99686",
		Status:   &s,
	}
	return repo.Create(ctx, auth.Claims{}, req, now)
}
