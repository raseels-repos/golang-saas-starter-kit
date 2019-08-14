package account_preference

import (
	"context"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jmoiron/sqlx"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/go-playground/validator.v9"
)

const (
	// The database table for AccountPreference
	accountPreferenceTableName = "account_preferences"
	// The database table for User Account
	userAccountTableName = "users_accounts"
)

var (
	// ErrNotFound abstracts the mgo not found error.
	ErrNotFound = errors.New("Entity not found")
)

// The list of columns needed for find
var accountPreferenceMapColumns = "account_id,name,value,created_at,updated_at,archived_at"

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
		query.Where(query.In("account_id", subQuery))
	}

	return nil
}

// Find gets all the account preferences from the database based on the request params.
// TODO: Need to figure out why can't parse the args when appending the where to the query.
func (repo *Repository) Find(ctx context.Context, claims auth.Claims, req AccountPreferenceFindRequest) ([]*AccountPreference, error) {
	query := sqlbuilder.NewSelectBuilder()
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

// FindByAccountID gets the specified account preferences for an account from the database.
func (repo *Repository) FindByAccountID(ctx context.Context, claims auth.Claims, req AccountPreferenceFindByAccountIDRequest) ([]*AccountPreference, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.account_preference.FindByAccountID")
	defer span.Finish()

	// Validate the request.
	err := Validator().StructCtx(ctx, req)
	if err != nil {
		return nil, err
	}

	// Filter base select query by ID
	query := sqlbuilder.NewSelectBuilder()
	query.Where(query.Equal("account_id", req.AccountID))

	if len(req.Order) > 0 {
		query.OrderBy(req.Order...)
	}
	if req.Limit != nil {
		query.Limit(int(*req.Limit))
	}
	if req.Offset != nil {
		query.Offset(int(*req.Offset))
	}

	return find(ctx, claims, repo.DbConn, query, []interface{}{}, req.IncludeArchived)
}

// find internal method for getting all the account preferences from the database using a select query.
func find(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, query *sqlbuilder.SelectBuilder, args []interface{}, includedArchived bool) ([]*AccountPreference, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.account_preference.Find")
	defer span.Finish()

	query.Select(accountPreferenceMapColumns)
	query.From(accountPreferenceTableName)

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
		err = errors.WithMessage(err, "find account preferences failed")
		return nil, err
	}

	// iterate over each row
	resp := []*AccountPreference{}
	for rows.Next() {
		var (
			a   AccountPreference
			err error
		)
		err = rows.Scan(&a.AccountID, &a.Name, &a.Value, &a.CreatedAt, &a.UpdatedAt, &a.ArchivedAt)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			return nil, err
		}
		resp = append(resp, &a)
	}

	return resp, nil
}

// Read gets the specified account preference from the database.
func (repo *Repository) Read(ctx context.Context, claims auth.Claims, req AccountPreferenceReadRequest) (*AccountPreference, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.account_preference.Read")
	defer span.Finish()

	// Validate the request.
	err := Validator().StructCtx(ctx, req)
	if err != nil {
		return nil, err
	}

	// Filter base select query by ID
	query := sqlbuilder.NewSelectBuilder()
	query.Where(query.And(
		query.Equal("account_id", req.AccountID)),
		query.Equal("name", req.Name))

	res, err := find(ctx, claims, repo.DbConn, query, []interface{}{}, req.IncludeArchived)
	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		err = errors.WithMessagef(ErrNotFound, "account preference %s for account %s not found", req.Name, req.AccountID)
		return nil, err
	}
	u := res[0]

	return u, nil
}

type ctxKeyPreferenceName int

const KeyPreferenceName ctxKeyPreferenceName = 1

// Validator registers a custom validation function for tag preference_value.
func Validator() *validator.Validate {
	v := webcontext.Validator()

	fctx := func(ctx context.Context, fl validator.FieldLevel) bool {
		if fl.Field().String() == "invalid" {
			return false
		}

		name, ok := ctx.Value(KeyPreferenceName).(AccountPreferenceName)
		if !ok {
			return false
		}

		val := fl.Field().String()

		switch name {
		case AccountPreference_Datetime_Format:

			loc, _ := time.LoadLocation("MST")
			tv, _ := time.Parse(time.RFC3339, "2006-01-02T15:04:05Z")
			tv = tv.In(loc)

			pv, err := time.Parse(val, tv.Format(val))
			if err != nil {
				return false
			}

			if pv.Format(val) != tv.Format(val) || pv.Format("2006-01-02") != tv.Format("2006-01-02") || pv.IsZero() {
				return false
			}
			return true

		case AccountPreference_Date_Format:

			loc, _ := time.LoadLocation("MST")
			tv, _ := time.Parse(time.RFC3339, "2006-01-02T15:04:05Z")
			tv = tv.In(loc)

			pv, err := time.Parse(val, tv.Format(val))
			if err != nil {
				return false
			}

			if pv.Format(val) != tv.Format(val) || pv.UTC().Format("2006-01-02") != tv.UTC().Format("2006-01-02") || pv.IsZero() {
				return false
			}
			return true

		case AccountPreference_Time_Format:
			//loc, _ := time.LoadLocation("MST")
			tv, _ := time.Parse(time.RFC3339, "2006-01-02T15:04:05Z")
			//tv = tv.In(loc)

			pv, err := time.Parse(val, tv.Format(val))
			if err != nil {
				return false
			}

			if pv.Format(val) != tv.Format(val) || pv.UTC().Format("15:04") != tv.UTC().Format("15:04") || pv.IsZero() {
				return false
			}

			return true
		}

		return false
	}
	v.RegisterValidationCtx("preference_value", fctx)

	return v
}

// Set inserts a new account preference or updates an existing on.
func (repo *Repository) Set(ctx context.Context, claims auth.Claims, req AccountPreferenceSetRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.account_preference.Set")
	defer span.Finish()

	ctx = context.WithValue(ctx, KeyPreferenceName, req.Name)

	// Validate the request.
	err := Validator().StructCtx(ctx, req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the account specified in the request.
	err = account.CanModifyAccount(ctx, claims, repo.DbConn, req.AccountID)
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

	// Build the insert SQL statement.
	query := sqlbuilder.NewInsertBuilder()
	query.InsertInto(accountPreferenceTableName)
	query.Cols("account_id", "name", "value", "created_at", "updated_at")
	query.Values(req.AccountID, req.Name, req.Value, now, now)

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = repo.DbConn.Rebind(sql)

	sql = sql + " ON CONFLICT ON CONSTRAINT account_preferences_pkey DO UPDATE set value = EXCLUDED.value "

	_, err = repo.DbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessage(err, "set account preference failed")
		return err
	}

	return nil
}

// Archive soft deleted the account preference from the database.
func (repo *Repository) Archive(ctx context.Context, claims auth.Claims, req AccountPreferenceArchiveRequest, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.account_preference.Archive")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the account specified in the request.
	err = account.CanModifyAccount(ctx, claims, repo.DbConn, req.AccountID)
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
	query.Update(accountPreferenceTableName)
	query.Set(
		query.Assign("archived_at", now),
	)
	query.Where(query.Equal("account_id", req.AccountID))

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = repo.DbConn.Rebind(sql)
	_, err = repo.DbConn.ExecContext(ctx, sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "archive account preference %s for account %s failed", req.Name, req.AccountID)
		return err
	}

	return nil
}

// Delete removes an account preference from the database.
func (repo *Repository) Delete(ctx context.Context, claims auth.Claims, req AccountPreferenceDeleteRequest) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.account_preference.Delete")
	defer span.Finish()

	// Validate the request.
	v := webcontext.Validator()
	err := v.Struct(req)
	if err != nil {
		return err
	}

	// Ensure the claims can modify the account specified in the request.
	err = account.CanModifyAccount(ctx, claims, repo.DbConn, req.AccountID)
	if err != nil {
		return err
	}

	// Start a new transaction to handle rollbacks on error.
	tx, err := repo.DbConn.Begin()
	if err != nil {
		return errors.WithStack(err)
	}

	// Build the delete SQL statement.
	query := sqlbuilder.NewDeleteBuilder()
	query.DeleteFrom(accountPreferenceTableName)
	query.Where(query.Equal("account_id", req.AccountID))

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = repo.DbConn.Rebind(sql)
	_, err = tx.ExecContext(ctx, sql, args...)
	if err != nil {
		tx.Rollback()

		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessagef(err, "delete account preference %s for account %s failed", req.Name, req.AccountID)
		return err
	}

	err = tx.Commit()
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// MockAccountPreference returns a fake AccountPreference for testing.
func MockAccountPreference(ctx context.Context, dbConn *sqlx.DB, now time.Time) error {

	repo := &Repository{
		DbConn: dbConn,
	}

	req := AccountPreferenceSetRequest{
		AccountID: uuid.NewRandom().String(),
		Name:      AccountPreference_Datetime_Format,
		Value:     AccountPreference_Datetime_Format_Default,
	}
	return repo.Set(ctx, auth.Claims{}, req, now)
}
