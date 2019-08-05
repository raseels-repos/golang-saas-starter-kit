package user_account

import (
	"context"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"github.com/jmoiron/sqlx"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// UserFindByAccount lists all the users for a given account ID.
func UserFindByAccount(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req UserFindByAccountRequest) (Users, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.UserFind")
	defer span.Finish()

	v := webcontext.Validator()

	// Validate the request.
	err := v.StructCtx(ctx, req)
	if err != nil {
		return nil, err
	}


	return nil , nil
}
