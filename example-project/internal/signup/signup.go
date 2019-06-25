package signup

import (
	"context"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user_account"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"github.com/jmoiron/sqlx"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/go-playground/validator.v9"
)

// Signup performs the steps needed to create a new account, new user and then associate
// both records with a new user_account entry.
func Signup(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, req SignupRequest, now time.Time) (*SignupResponse, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.signup.Signup")
	defer span.Finish()

	// Default account status to active for signup if now set.
	if req.Account.Status == nil {
		s := account.AccountStatus_Active
		req.Account.Status = &s
	}

	v := validator.New()

	// Validate the user email address is unique in the database.
	uniqEmail, err := user.UniqueEmail(ctx, dbConn, req.User.Email, "")
	if err != nil {
		return nil, err
	}

	// Validate the account name is unique in the database.
	uniqName, err := account.UniqueName(ctx, dbConn, req.Account.Name, "")
	if err != nil {
		return nil, err
	}

	f := func(fl validator.FieldLevel) bool {
		if fl.Field().String() == "invalid" {
			return false
		}

		var uniq bool
		switch (fl.FieldName()) {
		case "Name":
			uniq = uniqName
		case "Email":
			uniq = uniqEmail
		}

		return uniq
	}
	v.RegisterValidation("unique", f)

	// Validate the request.
	err = v.Struct(req)
	if err != nil {
		return nil, err
	}

	var resp SignupResponse

	// Execute user creation.
	resp.User, err = user.Create(ctx, claims, dbConn, req.User, now)
	if err != nil {
		return nil, err
	}

	// Set the signup and billing user IDs for reference.
	req.Account.SignupUserID = &resp.User.ID
	req.Account.BillingUserID = &resp.User.ID

	// Execute account creation.
	resp.Account, err = account.Create(ctx, claims, dbConn, req.Account, now)
	if err != nil {
		return nil, err
	}

	// Associate the created user with the new account. The first user for the account will
	// always have the role of admin.
	ua := user_account.CreateUserAccountRequest{
		UserID: resp.User.ID,
		AccountID: resp.Account.ID,
		Roles: []user_account.UserAccountRole{user_account.UserAccountRole_Admin},
		//Status:  Use default value
	}

	_, err = user_account.Create(ctx, claims, dbConn, ua, now)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}
