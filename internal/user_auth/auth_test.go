package user_auth

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/account/account_preference"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"github.com/google/go-cmp/cmp"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
)

var (
	test *tests.Test
	repo *Repository
)

// TestMain is the entry point for testing.
func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	test = tests.New()
	defer test.TearDown()

	tknGen := &auth.MockTokenGenerator{}

	userRepo := user.MockRepository(test.MasterDB)
	userAccRepo := user_account.NewRepository(test.MasterDB)
	accPrefRepo := account_preference.NewRepository(test.MasterDB)

	repo = NewRepository(test.MasterDB, tknGen, userRepo, userAccRepo, accPrefRepo)

	return m.Run()
}

// TestAuthenticate validates the behavior around authenticating users.
func TestAuthenticate(t *testing.T) {
	defer tests.Recover(t)

	t.Log("Given the need to authenticate users")
	{
		t.Log("\tWhen handling a single User.")
		{
			ctx := tests.Context()

			// Auth tokens are valid for an our and is verified against current time.
			// Issue the token one hour ago.
			now := time.Now().Add(time.Hour * -1)

			// Try to authenticate an invalid user.
			_, err := repo.Authenticate(ctx,
				AuthenticateRequest{
					Email:    "doesnotexist@gmail.com",
					Password: "xy7",
				}, time.Hour, now)
			if errors.Cause(err) != ErrAuthenticationFailure {
				t.Logf("\t\tGot : %+v", err)
				t.Logf("\t\tWant: %+v", ErrAuthenticationFailure)
				t.Fatalf("\t%s\tAuthenticate non existant user failed.", tests.Failed)
			}
			t.Logf("\t%s\tAuthenticate non existant user ok.", tests.Success)

			// Create a new user for testing.
			usrAcc, err := user_account.MockUserAccount(ctx, test.MasterDB, now, user_account.UserAccountRole_User)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
			}
			t.Logf("\t%s\tCreate user account ok.", tests.Success)

			// Add 30 minutes to now to simulate time passing.
			now = now.Add(time.Minute * 30)

			acc2, err := account.MockAccount(ctx, test.MasterDB, now)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tCreate second account failed.", tests.Failed)
			}
			t.Logf("\t%s\tCreate second account ok.", tests.Success)

			// Associate second new account with user user. Need to ensure that now
			// is always greater than the first user_account entry created so it will
			// be returned consistently back in the same order, last.
			account2Role := auth.RoleUser
			_, err = repo.UserAccount.Create(ctx, auth.Claims{}, user_account.UserAccountCreateRequest{
				UserID:    usrAcc.UserID,
				AccountID: acc2.ID,
				Roles:     []user_account.UserAccountRole{user_account.UserAccountRole(account2Role)},
			}, now)

			// Add 30 minutes to now to simulate time passing.
			now = now.Add(time.Minute * 5)

			// Try to authenticate valid user with invalid password.
			_, err = repo.Authenticate(ctx,
				AuthenticateRequest{
					Email:    usrAcc.User.Email,
					Password: "xy7",
				},
				time.Hour, now)
			if errors.Cause(err) != ErrAuthenticationFailure {
				t.Logf("\t\tGot : %+v", err)
				t.Logf("\t\tWant: %+v", ErrAuthenticationFailure)
				t.Fatalf("\t%s\tAuthenticate user w/invalid password failed.", tests.Failed)
			}
			t.Logf("\t%s\tAuthenticate user w/invalid password ok.", tests.Success)

			// Verify that the user can be authenticated with the created user.
			tkn1, err := repo.Authenticate(ctx,
				AuthenticateRequest{
					Email:    usrAcc.User.Email,
					Password: usrAcc.User.Password,
				}, time.Hour, now)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tAuthenticate user failed.", tests.Failed)
			}
			t.Logf("\t%s\tAuthenticate user ok.", tests.Success)

			// Ensure the token string was correctly generated.
			claims1, err := repo.TknGen.ParseClaims(tkn1.AccessToken)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tParse claims from token failed.", tests.Failed)
			}
			expectClaims := tkn1.claims
			expectClaims.RootUserID = ""
			expectClaims.RootAccountID = ""
			expectClaims.Subject = usrAcc.UserID
			expectClaims.Audience = usrAcc.AccountID

			if diff := cmpClaims(claims1, expectClaims); diff != "" {
				t.Fatalf("\t%s\tExpected parsed claims to match from token. Diff:\n%s", tests.Failed, diff)
			}
			t.Logf("\t%s\tAuthenticate parse claims from token ok.", tests.Success)

			// Try switching to a second account using the first set of claims.
			tkn2, err := repo.SwitchAccount(ctx, claims1,
				SwitchAccountRequest{AccountID: acc2.ID}, time.Hour, now)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tSwitchAccount user failed.", tests.Failed)
			}
			t.Logf("\t%s\tSwitchAccount user ok.", tests.Success)

			// Ensure the token string was correctly generated.
			claims2, err := repo.TknGen.ParseClaims(tkn2.AccessToken)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tParse claims from token failed.", tests.Failed)
			}
			expectClaims = tkn2.claims
			expectClaims.RootUserID = usrAcc.UserID
			expectClaims.RootAccountID = acc2.ID
			expectClaims.Subject = usrAcc.UserID
			expectClaims.Audience = acc2.ID

			if diff := cmpClaims(claims2, expectClaims); diff != "" {
				t.Fatalf("\t%s\tExpected parsed claims to match from token. Diff:\n%s", tests.Failed, diff)
			}
			t.Logf("\t%s\tSwitchAccount parse claims from token ok.", tests.Success)
		}
	}
}

// TestUserUpdatePassword validates update user password works.
func TestUserUpdatePassword(t *testing.T) {

	t.Log("Given the need ensure a user password can be updated.")
	{
		ctx := tests.Context()

		now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

		// Create a new user for testing.
		usrAcc, err := user_account.MockUserAccount(ctx, test.MasterDB, now, user_account.UserAccountRole_User)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}
		t.Logf("\t%s\tCreate user account ok.", tests.Success)

		// Verify that the user can be authenticated with the created user.
		_, err = repo.Authenticate(ctx,
			AuthenticateRequest{
				Email:    usrAcc.User.Email,
				Password: usrAcc.User.Password,
			}, time.Hour, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tAuthenticate failed.", tests.Failed)
		}

		// Update the users password.
		newPass := uuid.NewRandom().String()
		err = repo.User.UpdatePassword(ctx, auth.Claims{}, user.UserUpdatePasswordRequest{
			ID:              usrAcc.UserID,
			Password:        newPass,
			PasswordConfirm: newPass,
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tUpdate password failed.", tests.Failed)
		}
		t.Logf("\t%s\tUpdatePassword ok.", tests.Success)

		// Verify that the user can be authenticated with the updated password.
		_, err = repo.Authenticate(ctx,
			AuthenticateRequest{
				Email:    usrAcc.User.Email,
				Password: newPass,
			}, time.Hour, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tAuthenticate failed.", tests.Failed)
		}
		t.Logf("\t%s\tAuthenticate ok.", tests.Success)
	}
}

// TestUserResetPassword validates that reset password for a user works.
func TestUserResetPassword(t *testing.T) {

	t.Log("Given the need ensure a user can reset their password.")
	{
		ctx := tests.Context()

		now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

		// Create a new user for testing.
		usrAcc, err := user_account.MockUserAccount(ctx, test.MasterDB, now, user_account.UserAccountRole_User)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}
		t.Logf("\t%s\tCreate user account ok.", tests.Success)

		ttl := time.Hour

		// Make the reset password request.
		resetHash, err := repo.User.ResetPassword(ctx, user.UserResetPasswordRequest{
			Email: usrAcc.User.Email,
			TTL:   ttl,
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tResetPassword failed.", tests.Failed)
		}
		t.Logf("\t%s\tResetPassword ok.", tests.Success)

		// Assuming we have received the email and clicked the link, we now can ensure confirm works.
		newPass := uuid.NewRandom().String()
		reset, err := repo.User.ResetConfirm(ctx, user.UserResetConfirmRequest{
			ResetHash:       resetHash,
			Password:        newPass,
			PasswordConfirm: newPass,
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tResetConfirm failed.", tests.Failed)
		} else if reset.ID != usrAcc.User.ID {
			t.Logf("\t\tGot : %+v", reset.ID)
			t.Logf("\t\tWant: %+v", usrAcc.User.ID)
			t.Fatalf("\t%s\tResetConfirm failed.", tests.Failed)
		}
		t.Logf("\t%s\tResetConfirm ok.", tests.Success)

		// Verify that the user can be authenticated with the updated password.
		_, err = repo.Authenticate(ctx,
			AuthenticateRequest{
				Email:    usrAcc.User.Email,
				Password: newPass,
			}, time.Hour, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tAuthenticate failed.", tests.Failed)
		}
		t.Logf("\t%s\tAuthenticate ok.", tests.Success)
	}
}

// TestSwitchAccount validates the behavior around allowing users to switch between their accounts.
func TestSwitchAccount(t *testing.T) {
	defer tests.Recover(t)

	// Auth tokens are valid for an our and is verified against current time.
	// Issue the token one hour ago.
	now := time.Now().Add(time.Hour * -1)

	ctx := tests.Context()

	type authTest struct {
		name          string
		root          *user_account.MockUserAccountResponse
		switch1Req    SwitchAccountRequest
		switch1Roles  []user_account.UserAccountRole
		switch1Scopes []string
		switch1Err    error
		switch2Req    SwitchAccountRequest
		switch2Roles  []user_account.UserAccountRole
		switch2Scopes []string
		switch2Err    error
	}

	var authTests []authTest

	// Test all the combinations there the user has access to all three accounts.
	if true {
		for _, roles := range [][]user_account.UserAccountRole{
			{user_account.UserAccountRole_Admin, user_account.UserAccountRole_Admin, user_account.UserAccountRole_Admin},
			{user_account.UserAccountRole_User, user_account.UserAccountRole_User, user_account.UserAccountRole_User},
			{user_account.UserAccountRole_Admin, user_account.UserAccountRole_User, user_account.UserAccountRole_Admin},
			{user_account.UserAccountRole_User, user_account.UserAccountRole_Admin, user_account.UserAccountRole_User},
		} {
			// Create a new user for testing.
			usrAcc, err := user_account.MockUserAccount(ctx, test.MasterDB, now, roles[0])
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
			}

			// Create the second account.
			now = now.Add(time.Minute)
			acc2, err := account.MockAccount(ctx, test.MasterDB, now)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tCreate second account failed.", tests.Failed)
			}

			// Associate the second account with root user.
			usrAcc2, err := repo.UserAccount.Create(ctx, auth.Claims{}, user_account.UserAccountCreateRequest{
				UserID:    usrAcc.UserID,
				AccountID: acc2.ID,
				Roles:     []user_account.UserAccountRole{user_account.UserAccountRole(roles[1])},
			}, now)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tLinking second account to user failed.", tests.Failed)
			}

			// Create the third account.
			now = now.Add(time.Minute)
			acc3, err := account.MockAccount(ctx, test.MasterDB, now)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tCreate third account failed.", tests.Failed)
			}

			// Associate the third account with root user.
			usrAcc3, err := repo.UserAccount.Create(ctx, auth.Claims{}, user_account.UserAccountCreateRequest{
				UserID:    usrAcc.UserID,
				AccountID: acc3.ID,
				Roles:     []user_account.UserAccountRole{user_account.UserAccountRole(roles[2])},
			}, now)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tLinking third account to user failed.", tests.Failed)
			}

			authTests = append(authTests, authTest{
				name: fmt.Sprintf("Root account role %s -> role %s account 2 -> role %s account 3.",
					roles[0], roles[1], roles[2]),
				root:         usrAcc,
				switch1Req:   SwitchAccountRequest{AccountID: acc2.ID},
				switch1Roles: usrAcc2.Roles,
				switch1Err:   nil,
				switch2Req:   SwitchAccountRequest{AccountID: acc3.ID},
				switch2Err:   nil,
				switch2Roles: usrAcc3.Roles,
			})
		}
	}

	// Root account 1 -> invalid account 2
	if true {
		// Create a new user for testing.
		usrAcc, err := user_account.MockUserAccount(ctx, test.MasterDB, now, user_account.UserAccountRole_Admin)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}

		// Create the second account and don't associate it with the root user.
		now = now.Add(time.Minute)
		acc2, err := account.MockAccount(ctx, test.MasterDB, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate second account failed.", tests.Failed)
		}

		authTests = append(authTests, authTest{
			name:       "Root account 1 -> invalid account 2.",
			root:       usrAcc,
			switch1Req: SwitchAccountRequest{AccountID: acc2.ID},
			switch1Err: ErrAuthenticationFailure,
		})
	}

	// Root account 1 -> valid account 2 with scopes -> valid account 3 with invalid scope.
	if true {
		// Create a new user for testing.
		usrAcc, err := user_account.MockUserAccount(ctx, test.MasterDB, now, user_account.UserAccountRole_Admin)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}

		// Create the second account.
		now = now.Add(time.Minute)
		acc2, err := account.MockAccount(ctx, test.MasterDB, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate second account failed.", tests.Failed)
		}

		// Associate the second account with root user.
		usrAcc2, err := repo.UserAccount.Create(ctx, auth.Claims{}, user_account.UserAccountCreateRequest{
			UserID:    usrAcc.UserID,
			AccountID: acc2.ID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole_Admin},
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tLinking second account to user failed.", tests.Failed)
		}

		// Create the third account.
		now = now.Add(time.Minute)
		acc3, err := account.MockAccount(ctx, test.MasterDB, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate third account failed.", tests.Failed)
		}

		// Associate the third account with root user.
		usrAcc3, err := repo.UserAccount.Create(ctx, auth.Claims{}, user_account.UserAccountCreateRequest{
			UserID:    usrAcc.UserID,
			AccountID: acc3.ID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole_User},
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tLinking third account to user failed.", tests.Failed)
		}

		authTests = append(authTests, authTest{
			name:          "Root account 1 -> valid account 2 with scopes -> valid account 3 with invalid scope.",
			root:          usrAcc,
			switch1Req:    SwitchAccountRequest{AccountID: acc2.ID},
			switch1Roles:  usrAcc2.Roles,
			switch1Scopes: []string{user_account.UserAccountRole_User.String()},
			switch1Err:    nil,
			switch2Req:    SwitchAccountRequest{AccountID: acc3.ID},
			switch2Roles:  usrAcc3.Roles,
			switch2Scopes: []string{user_account.UserAccountRole_Admin.String()},
			switch2Err:    ErrForbidden,
		})
	}

	// Add 30 minutes to now to simulate time passing.
	now = now.Add(time.Minute * 5)

	t.Log("Given the need to switch accounts.")
	{
		for i, authTest := range authTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, authTest.name)
			{
				// Verify that the user can be authenticated with the created user.
				var claims1 auth.Claims
				tkn1, err := repo.Authenticate(ctx,
					AuthenticateRequest{
						Email:    authTest.root.User.Email,
						Password: authTest.root.User.Password,
					}, time.Hour, now)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tAuthenticate user failed.", tests.Failed)
				} else {
					// Ensure the token string was correctly generated.
					claims1, err = repo.TknGen.ParseClaims(tkn1.AccessToken)
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tParse claims from token failed.", tests.Failed)
					}
					expectClaims := tkn1.claims
					expectClaims.RootUserID = ""
					expectClaims.RootAccountID = ""
					expectClaims.Subject = authTest.root.UserID
					expectClaims.Audience = authTest.root.AccountID
					expectClaims.Roles = rolesStringSlice(authTest.root.Roles)

					if diff := cmpClaims(claims1, expectClaims); diff != "" {
						t.Fatalf("\t%s\tExpected parsed claims to match from token. Diff:\n%s", tests.Failed, diff)
					}
				}
				t.Logf("\t%s\tAuthenticate root user with role %v ok.", tests.Success, authTest.root.Roles)

				// Try to switch to account 2.
				var claims2 auth.Claims
				tkn2, err := repo.SwitchAccount(ctx, claims1, authTest.switch1Req, time.Hour, now, authTest.switch1Scopes...)
				if err != authTest.switch1Err {
					if errors.Cause(err) != authTest.switch1Err {
						t.Log("\t\tExpected :", authTest.switch1Err)
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tSwitchAccount account 1 with role %v failed.", tests.Failed, authTest.switch1Roles)
					}
				} else {
					// Ensure the token string was correctly generated.
					claims2, err = repo.TknGen.ParseClaims(tkn2.AccessToken)
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tParse claims from token failed.", tests.Failed)
					}
					expectClaims := tkn2.claims
					expectClaims.RootUserID = authTest.root.UserID
					expectClaims.RootAccountID = authTest.switch1Req.AccountID
					expectClaims.Subject = authTest.root.UserID
					expectClaims.Audience = authTest.switch1Req.AccountID

					if len(authTest.switch1Scopes) > 0 {
						expectClaims.Roles = authTest.switch1Scopes
					} else {
						expectClaims.Roles = rolesStringSlice(authTest.switch1Roles)
					}

					if diff := cmpClaims(claims2, expectClaims); diff != "" {
						t.Fatalf("\t%s\tExpected parsed claims to match from token. Diff:\n%s", tests.Failed, diff)
					}
				}
				t.Logf("\t%s\tSwitchAccount account 1 with role %v ok.", tests.Success, authTest.switch1Roles)

				// If the user can't login, don't need to test any further.
				if authTest.switch1Err != nil || authTest.switch2Req.AccountID == "" {
					continue
				}

				// Try to switch to account 3.
				tkn3, err := repo.SwitchAccount(ctx, claims2, authTest.switch2Req, time.Hour, now, authTest.switch2Scopes...)
				if err != authTest.switch2Err {
					if errors.Cause(err) != authTest.switch2Err {
						t.Log("\t\tExpected :", authTest.switch2Err)
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tSwitchAccount account 2 with role %v failed.", tests.Failed, authTest.switch2Roles)
					}
				} else {
					// Ensure the token string was correctly generated.
					claims3, err := repo.TknGen.ParseClaims(tkn3.AccessToken)
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tParse claims from token failed.", tests.Failed)
					}
					expectClaims := tkn3.claims
					expectClaims.RootUserID = authTest.root.UserID
					expectClaims.RootAccountID = authTest.switch2Req.AccountID
					expectClaims.Subject = authTest.root.UserID
					expectClaims.Audience = authTest.switch2Req.AccountID

					if len(authTest.switch2Scopes) > 0 {
						expectClaims.Roles = authTest.switch2Scopes
					} else {
						expectClaims.Roles = rolesStringSlice(authTest.switch2Roles)
					}

					if diff := cmpClaims(claims3, expectClaims); diff != "" {
						t.Fatalf("\t%s\tExpected parsed claims to match from token. Diff:\n%s", tests.Failed, diff)
					}
				}
				t.Logf("\t%s\tSwitchAccount account 2 with role %v ok.", tests.Success, authTest.switch2Roles)
			}
		}
	}
}

// TestVirtualLogin validates the behavior around allowing users to virtual login users.
func TestVirtualLogin(t *testing.T) {
	defer tests.Recover(t)

	// Auth tokens are valid for an our and is verified against current time.
	// Issue the token one hour ago.
	now := time.Now().Add(time.Hour * -1)

	ctx := tests.Context()

	type authTest struct {
		name         string
		root         *user_account.MockUserAccountResponse
		login1Req    VirtualLoginRequest
		login1Err    error
		login1Role   user_account.UserAccountRole
		login2Req    VirtualLoginRequest
		login2Err    error
		login2Role   user_account.UserAccountRole
		login2Logout bool
	}

	var authTests []authTest

	// Root admin -> role admin -> role admin
	{
		// Create a new user for testing.
		usrAcc, err := user_account.MockUserAccount(ctx, test.MasterDB, now, user_account.UserAccountRole_Admin)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}

		usr2, err := user.MockUser(ctx, test.MasterDB, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate second account failed.", tests.Failed)
		}

		// Associate second user with basic role associated with the same account.
		usrAcc2, err := repo.UserAccount.Create(ctx, auth.Claims{}, user_account.UserAccountCreateRequest{
			UserID:    usr2.ID,
			AccountID: usrAcc.AccountID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole(user_account.UserAccountRole_Admin)},
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tLinking second user to account failed.", tests.Failed)
		}

		usr3, err := user.MockUser(ctx, test.MasterDB, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate second account failed.", tests.Failed)
		}

		// Associate second user with basic role associated with the same account.
		usrAcc3, err := repo.UserAccount.Create(ctx, auth.Claims{}, user_account.UserAccountCreateRequest{
			UserID:    usr3.ID,
			AccountID: usrAcc.AccountID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole(user_account.UserAccountRole_Admin)},
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tLinking third user to account failed.", tests.Failed)
		}

		authTests = append(authTests, authTest{
			name: "Root admin -> role admin -> role admin",
			root: usrAcc,
			login1Req: VirtualLoginRequest{
				UserID:    usr2.ID,
				AccountID: usrAcc.AccountID,
			},
			login1Role: usrAcc2.Roles[0],
			login1Err:  nil,
			login2Req: VirtualLoginRequest{
				UserID:    usr3.ID,
				AccountID: usrAcc.AccountID,
			},
			login2Err:    nil,
			login2Role:   usrAcc3.Roles[0],
			login2Logout: true,
		})
	}

	// Root admin -> role admin -> role user
	if true {
		// Create a new user for testing.
		usrAcc, err := user_account.MockUserAccount(ctx, test.MasterDB, now, user_account.UserAccountRole_Admin)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}

		usr2, err := user.MockUser(ctx, test.MasterDB, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate second account failed.", tests.Failed)
		}

		// Associate second user with basic role associated with the same account.
		usrAcc2, err := repo.UserAccount.Create(ctx, auth.Claims{}, user_account.UserAccountCreateRequest{
			UserID:    usr2.ID,
			AccountID: usrAcc.AccountID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole(user_account.UserAccountRole_Admin)},
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tLinking second user to account failed.", tests.Failed)
		}

		usr3, err := user.MockUser(ctx, test.MasterDB, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate second account failed.", tests.Failed)
		}

		// Associate second user with basic role associated with the same account.
		usrAcc3, err := repo.UserAccount.Create(ctx, auth.Claims{}, user_account.UserAccountCreateRequest{
			UserID:    usr3.ID,
			AccountID: usrAcc.AccountID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole(user_account.UserAccountRole_User)},
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tLinking third user to account failed.", tests.Failed)
		}

		authTests = append(authTests, authTest{
			name: "Root admin -> role admin -> role user",
			root: usrAcc,
			login1Req: VirtualLoginRequest{
				UserID:    usr2.ID,
				AccountID: usrAcc.AccountID,
			},
			login1Err:  nil,
			login1Role: usrAcc2.Roles[0],
			login2Req: VirtualLoginRequest{
				UserID:    usr3.ID,
				AccountID: usrAcc.AccountID,
			},
			login2Err:    nil,
			login2Role:   usrAcc3.Roles[0],
			login2Logout: true,
		})
	}

	// Root admin -> role user -> role admin
	if true {
		// Create a new user for testing.
		usrAcc, err := user_account.MockUserAccount(ctx, test.MasterDB, now, user_account.UserAccountRole_Admin)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}

		usr2, err := user.MockUser(ctx, test.MasterDB, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate second account failed.", tests.Failed)
		}

		// Associate second user with basic role associated with the same account.
		usrAcc2, err := repo.UserAccount.Create(ctx, auth.Claims{}, user_account.UserAccountCreateRequest{
			UserID:    usr2.ID,
			AccountID: usrAcc.AccountID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole(user_account.UserAccountRole_User)},
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tLinking second user to account failed.", tests.Failed)
		}

		usr3, err := user.MockUser(ctx, test.MasterDB, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate second account failed.", tests.Failed)
		}

		// Associate second user with basic role associated with the same account.
		usrAcc3, err := repo.UserAccount.Create(ctx, auth.Claims{}, user_account.UserAccountCreateRequest{
			UserID:    usr3.ID,
			AccountID: usrAcc.AccountID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole(user_account.UserAccountRole_Admin)},
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tLinking third user to account failed.", tests.Failed)
		}

		authTests = append(authTests, authTest{
			name: "Root admin -> role user -> role admin",
			root: usrAcc,
			login1Req: VirtualLoginRequest{
				UserID:    usr2.ID,
				AccountID: usrAcc.AccountID,
			},
			login1Err:  nil,
			login1Role: usrAcc2.Roles[0],
			login2Req: VirtualLoginRequest{
				UserID:    usr3.ID,
				AccountID: usrAcc.AccountID,
			},
			login2Err:    ErrForbidden,
			login2Role:   usrAcc3.Roles[0],
			login2Logout: true,
		})
	}

	// Root user -> role admin
	if true {
		// Create a new user for testing.
		usrAcc, err := user_account.MockUserAccount(ctx, test.MasterDB, now, user_account.UserAccountRole_User)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}

		usr2, err := user.MockUser(ctx, test.MasterDB, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate second account failed.", tests.Failed)
		}

		// Associate second user with basic role associated with the same account.
		usrAcc2, err := repo.UserAccount.Create(ctx, auth.Claims{}, user_account.UserAccountCreateRequest{
			UserID:    usr2.ID,
			AccountID: usrAcc.AccountID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole(user_account.UserAccountRole_Admin)},
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tLinking second user to account failed.", tests.Failed)
		}

		authTests = append(authTests, authTest{
			name: "Root user -> role admin",
			root: usrAcc,
			login1Req: VirtualLoginRequest{
				UserID:    usr2.ID,
				AccountID: usrAcc.AccountID,
			},
			login1Err:    ErrForbidden,
			login1Role:   usrAcc2.Roles[0],
			login2Logout: true,
		})
	}

	// Root user -> role user
	if true {
		// Create a new user for testing.
		usrAcc, err := user_account.MockUserAccount(ctx, test.MasterDB, now, user_account.UserAccountRole_User)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}

		usr2, err := user.MockUser(ctx, test.MasterDB, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate second account failed.", tests.Failed)
		}

		// Associate second user with basic role associated with the same account.
		usrAcc2, err := repo.UserAccount.Create(ctx, auth.Claims{}, user_account.UserAccountCreateRequest{
			UserID:    usr2.ID,
			AccountID: usrAcc.AccountID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole(user_account.UserAccountRole_User)},
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tLinking second user to account failed.", tests.Failed)
		}

		authTests = append(authTests, authTest{
			name: "Root user -> role admin",
			root: usrAcc,
			login1Req: VirtualLoginRequest{
				UserID:    usr2.ID,
				AccountID: usrAcc.AccountID,
			},
			login1Err:    ErrForbidden,
			login1Role:   usrAcc2.Roles[0],
			login2Logout: true,
		})
	}

	// Add 30 minutes to now to simulate time passing.
	now = now.Add(time.Minute * 5)

	t.Log("Given the need to virtual login.")
	{
		for i, authTest := range authTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, authTest.name)
			{
				// Verify that the user can be authenticated with the created user.
				var claims1 auth.Claims
				tkn1, err := repo.Authenticate(ctx,
					AuthenticateRequest{
						Email:    authTest.root.User.Email,
						Password: authTest.root.User.Password,
					}, time.Hour, now)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tAuthenticate user failed.", tests.Failed)
				} else {
					// Ensure the token string was correctly generated.
					claims1, err = repo.TknGen.ParseClaims(tkn1.AccessToken)
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tParse claims from token failed.", tests.Failed)
					}
					expectClaims := tkn1.claims
					expectClaims.RootUserID = ""
					expectClaims.RootAccountID = ""
					expectClaims.Subject = authTest.root.UserID
					expectClaims.Audience = authTest.root.AccountID

					// Hack for Unhandled Exception in go-cmp@v0.3.0/cmp/options.go:229
					if diff := cmpClaims(claims1, expectClaims); diff != "" {
						t.Fatalf("\t%s\tExpected parsed claims to match from token. Diff:\n%s", tests.Failed, diff)
					}
				}
				t.Logf("\t%s\tAuthenticate root user with role %s ok.", tests.Success, authTest.root.Roles[0])

				// Try virtual login to user 2.
				var claims2 auth.Claims
				tkn2, err := repo.VirtualLogin(ctx, claims1, authTest.login1Req, time.Hour, now)
				if err != authTest.login1Err {
					if errors.Cause(err) != authTest.login1Err {
						t.Log("\t\tExpected :", authTest.login1Err)
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tVirtualLogin user 1 with role %s failed.", tests.Failed, authTest.login1Role)
					}
				} else {
					// Ensure the token string was correctly generated.
					claims2, err = repo.TknGen.ParseClaims(tkn2.AccessToken)
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tParse claims from token failed.", tests.Failed)
					}
					expectClaims := tkn2.claims
					expectClaims.RootUserID = authTest.root.UserID
					expectClaims.RootAccountID = authTest.root.AccountID
					expectClaims.Subject = authTest.login1Req.UserID
					expectClaims.Audience = authTest.login1Req.AccountID

					// Hack for Unhandled Exception in go-cmp@v0.3.0/cmp/options.go:229
					if diff := cmpClaims(claims2, expectClaims); diff != "" {
						t.Fatalf("\t%s\tExpected parsed claims to match from token. Diff:\n%s", tests.Failed, diff)
					}
				}
				t.Logf("\t%s\tVirtualLogin user 1 with role %s ok.", tests.Success, authTest.login1Role)

				// If the user can't login, don't need to test any further.
				if authTest.login1Err != nil {
					continue
				}

				// Try virtual login to user 3.
				tkn3, err := repo.VirtualLogin(ctx, claims2, authTest.login2Req, time.Hour, now)
				if err != authTest.login2Err {
					if errors.Cause(err) != authTest.login2Err {
						t.Log("\t\tExpected :", authTest.login2Err)
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tVirtualLogin user 2 with role %s failed.", tests.Failed, authTest.login2Role)
					}
				} else {
					// Ensure the token string was correctly generated.
					claims3, err := repo.TknGen.ParseClaims(tkn3.AccessToken)
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tParse claims from token failed.", tests.Failed)
					}
					expectClaims := tkn3.claims
					expectClaims.RootUserID = authTest.root.UserID
					expectClaims.RootAccountID = authTest.root.AccountID
					expectClaims.Subject = authTest.login2Req.UserID
					expectClaims.Audience = authTest.login2Req.AccountID

					// Hack for Unhandled Exception in go-cmp@v0.3.0/cmp/options.go:229
					if diff := cmpClaims(claims3, expectClaims); diff != "" {
						t.Fatalf("\t%s\tExpected parsed claims to match from token. Diff:\n%s", tests.Failed, diff)
					}
				}
				t.Logf("\t%s\tVirtualLogin user 2 with role %s ok.", tests.Success, authTest.login2Role)

				if authTest.login2Logout {
					tknOut, err := repo.VirtualLogout(ctx, claims2, time.Hour, now)
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tVirtualLogout user 2 failed.", tests.Failed)
					}

					// Ensure the token string was correctly generated.
					claimsOut, err := repo.TknGen.ParseClaims(tknOut.AccessToken)
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tParse claims from token failed.", tests.Failed)
					}
					expectClaims := tknOut.claims
					expectClaims.RootUserID = authTest.root.UserID
					expectClaims.RootAccountID = authTest.root.AccountID
					expectClaims.Subject = authTest.root.UserID
					expectClaims.Audience = authTest.root.AccountID

					if diff := cmpClaims(claimsOut, expectClaims); diff != "" {
						t.Fatalf("\t%s\tExpected parsed claims to match from token. Diff:\n%s", tests.Failed, diff)
					}
					t.Logf("\t%s\tVirtualLogout user 2 with role %s ok.", tests.Success, authTest.login2Role)
				}
			}
		}
	}
}

// rolesStringSlice converts a list of roles to a string slice.
func rolesStringSlice(roles []user_account.UserAccountRole) []string {
	var l []string
	for _, r := range roles {
		l = append(l, string(r))
	}
	return l
}

// cmpClaims is a hack for Unhandled Exception in go-cmp@v0.3.0/cmp/options.go:229
func cmpClaims(actualClaims, expectedclaims auth.Claims) string {
	dat1, _ := json.Marshal(actualClaims)
	dat2, _ := json.Marshal(expectedclaims)
	return cmp.Diff(string(dat1), string(dat2))
}
