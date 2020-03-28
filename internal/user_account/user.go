package user_account

import (
	"context"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"github.com/huandu/go-sqlbuilder"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// UserFindByAccount lists all the users for a given account ID.
func (repo *Repository) UserFindByAccount(ctx context.Context, claims auth.Claims, req UserFindByAccountRequest) (Users, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.UserFindByAccount")
	defer span.Finish()

	v := webcontext.Validator()

	// Validate the request.
	err := v.StructCtx(ctx, req)
	if err != nil {
		return nil, err
	}

	/*
		SELECT
			id,
			first_name,
			last_name,
			name,
			email,
			timezone,
			account_id,
			status,
			roles,
			created_at,
			updated_at,
			archived_at FROM (
				SELECT
					u.id,
					u.first_name,
					u.last_name,
					concat(u.first_name, ' ',u.last_name) as name,
					u.email,
					u.timezone,
					ua.account_id,
					ua.status,
					ua.roles,
					CASE WHEN ua.created_at > u.created_at THEN ua.created_at ELSE u.created_at END AS created_at,
					CASE WHEN ua.updated_at > u.updated_at THEN ua.updated_at ELSE u.updated_at END AS updated_at,
					CASE WHEN ua.archived_at > u.archived_at THEN ua.archived_at ELSE u.archived_at END AS archived_at
				FROM users u
				JOIN users_accounts ua
					ON u.id = ua.user_id AND ua.account_id = 'df1a8a65-b00b-4640-9a64-66c1a355b17c'
				WHERE
					(u.archived_at IS NULL AND ua.archived_at IS NULL) AND
					account_id IN (SELECT account_id FROM users_accounts WHERE (account_id = ? OR user_id = ?))
			) res ORDER BY id asc

	*/

	subQuery := sqlbuilder.NewSelectBuilder().
		Select("u.id,u.first_name,u.last_name,concat(u.first_name, ' ',u.last_name) as name,u.email,u.timezone,ua.account_id,ua.status,ua.roles,"+
			"CASE WHEN ua.created_at > u.created_at THEN ua.created_at ELSE u.created_at END AS created_at,"+
			"CASE WHEN ua.updated_at > u.updated_at THEN ua.updated_at ELSE u.updated_at END AS updated_at,"+
			"CASE WHEN ua.archived_at > u.archived_at THEN ua.archived_at ELSE u.archived_at END AS archived_at").
		From(userTableName+" u").
		Join(userAccountTableName+" ua", "u.id = ua.user_id", "ua.account_id = '"+req.AccountID+"'")

	if !req.IncludeArchived {
		subQuery.Where(subQuery.And(
			subQuery.IsNull("u.archived_at"),
			subQuery.IsNull("ua.archived_at")))
	}

	if claims.Audience != "" || claims.Subject != "" {
		// Build select statement for users_accounts table
		authQuery := sqlbuilder.NewSelectBuilder().Select("account_id").From(userAccountTableName)

		var or []string
		if claims.Audience != "" {
			or = append(or, authQuery.Equal("account_id", claims.Audience))
		}
		if claims.Subject != "" {
			or = append(or, authQuery.Equal("user_id", claims.Subject))
		}

		// Append sub query
		if len(or) > 0 {
			authQuery.Where(authQuery.Or(or...))
			subQuery.Where(subQuery.In("account_id", authQuery))
		}
	}

	subQueryStr, queryArgs := subQuery.Build()

	query := sqlbuilder.NewSelectBuilder().
		Select("id,first_name,last_name,name,email,timezone,account_id,status,roles,created_at,updated_at,archived_at").
		From("(" + subQueryStr + ") res")
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

	queryStr, moreQueryArgs := query.Build()
	queryStr = repo.DbConn.Rebind(queryStr)

	queryArgs = append(queryArgs, moreQueryArgs...)

	// fetch all places from the db
	rows, err := repo.DbConn.QueryContext(ctx, queryStr, queryArgs...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessage(err, "find users failed")
		return nil, err
	}
	defer rows.Close()

	// iterate over each row
	resp := []*User{}
	for rows.Next() {

		var (
			u   User
			err error
		)
		err = rows.Scan(&u.ID, &u.FirstName, &u.LastName, &u.Name, &u.Email, &u.Timezone, &u.AccountID, &u.Status,
			&u.Roles, &u.CreatedAt, &u.UpdatedAt, &u.ArchivedAt)
		if err != nil {
			err = errors.Wrapf(err, "query - %s", query.String())
			return nil, err
		}

		resp = append(resp, &u)
	}

	err = rows.Err()
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		err = errors.WithMessage(err, "find users failed")
		return nil, err
	}

	return resp, nil
}
