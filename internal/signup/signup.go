package signup

import (
	"context"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// Signup performs the steps needed to create a new account, new user and then associate
// both records with a new user_account entry.
func (repo *Repository) Signup(ctx context.Context, claims auth.Claims, req SignupRequest, now time.Time) (*SignupResult, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.signup.Signup")
	defer span.Finish()

	// Validate the user email address is unique in the database.
	uniqEmail, err := user.UniqueEmail(ctx, repo.DbConn, req.User.Email, "")
	if err != nil {
		return nil, err
	}
	ctx = webcontext.ContextAddUniqueValue(ctx, req.User, "Email", uniqEmail)

	// Validate the account name is unique in the database.
	uniqName, err := account.UniqueName(ctx, repo.DbConn, req.Account.Name, "")
	if err != nil {
		return nil, err
	}
	ctx = webcontext.ContextAddUniqueValue(ctx, req.Account, "Name", uniqName)

	// Validate the request.
	err = webcontext.Validator().StructCtx(ctx, req)
	if err != nil {
		return nil, err
	}

	var resp SignupResult

	// UserCreateRequest contains information needed to create a new User.
	userReq := user.UserCreateRequest{
		FirstName:       req.User.FirstName,
		LastName:        req.User.LastName,
		Email:           req.User.Email,
		Password:        req.User.Password,
		PasswordConfirm: req.User.PasswordConfirm,
		Timezone:        req.Account.Timezone,
	}

	// Execute user creation.
	resp.User, err = repo.User.Create(ctx, claims, userReq, now)
	if err != nil {
		return nil, err
	}

	accountStatus := account.AccountStatus_Active
	accountReq := account.AccountCreateRequest{
		Name:          req.Account.Name,
		Address1:      req.Account.Address1,
		Address2:      req.Account.Address2,
		City:          req.Account.City,
		Region:        req.Account.Region,
		Country:       req.Account.Country,
		Zipcode:       req.Account.Zipcode,
		Status:        &accountStatus,
		Timezone:      req.Account.Timezone,
		SignupUserID:  &resp.User.ID,
		BillingUserID: &resp.User.ID,
	}

	// Execute account creation.
	resp.Account, err = repo.Account.Create(ctx, claims, accountReq, now)
	if err != nil {
		return nil, err
	}

	// Associate the created user with the new account. The first user for the account will
	// always have the role of admin.
	ua := user_account.UserAccountCreateRequest{
		UserID:    resp.User.ID,
		AccountID: resp.Account.ID,
		Roles:     []user_account.UserAccountRole{user_account.UserAccountRole_Admin},
		//Status:  Use default value
	}

	_, err = repo.UserAccount.Create(ctx, claims, ua, now)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}
