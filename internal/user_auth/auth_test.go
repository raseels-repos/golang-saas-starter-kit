package user_auth

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/notify"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"github.com/google/go-cmp/cmp"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
)

var test *tests.Test

// TestMain is the entry point for testing.
func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	test = tests.New()
	defer test.TearDown()
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

			tknGen := &auth.MockTokenGenerator{}

			// Auth tokens are valid for an our and is verified against current time.
			// Issue the token one hour ago.
			now := time.Now().Add(time.Hour * -1)

			// Try to authenticate an invalid user.
			_, err := Authenticate(ctx, test.MasterDB, tknGen, "doesnotexist@gmail.com", "xy7", time.Hour, now)
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
			_, err = user_account.Create(ctx, auth.Claims{}, test.MasterDB, user_account.UserAccountCreateRequest{
				UserID:    usrAcc.UserID,
				AccountID: acc2.ID,
				Roles:     []user_account.UserAccountRole{user_account.UserAccountRole(account2Role)},
			}, now)

			// Add 30 minutes to now to simulate time passing.
			now = now.Add(time.Minute * 30)

			// Try to authenticate valid user with invalid password.
			_, err = Authenticate(ctx, test.MasterDB, tknGen, usrAcc.User.Email, "xy7", time.Hour, now)
			if errors.Cause(err) != ErrAuthenticationFailure {
				t.Logf("\t\tGot : %+v", err)
				t.Logf("\t\tWant: %+v", ErrAuthenticationFailure)
				t.Fatalf("\t%s\tAuthenticate user w/invalid password failed.", tests.Failed)
			}
			t.Logf("\t%s\tAuthenticate user w/invalid password ok.", tests.Success)

			// Verify that the user can be authenticated with the created user.
			tkn1, err := Authenticate(ctx, test.MasterDB, tknGen, usrAcc.User.Email, usrAcc.User.Password, time.Hour, now)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tAuthenticate user failed.", tests.Failed)
			}
			t.Logf("\t%s\tAuthenticate user ok.", tests.Success)

			// Ensure the token string was correctly generated.
			claims1, err := tknGen.ParseClaims(tkn1.AccessToken)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tParse claims from token failed.", tests.Failed)
			}

			// Hack for Unhandled Exception in go-cmp@v0.3.0/cmp/options.go:229
			resClaims, _ := json.Marshal(claims1)
			expectClaims, _ := json.Marshal(tkn1.claims)
			if diff := cmp.Diff(string(resClaims), string(expectClaims)); diff != "" {
				t.Fatalf("\t%s\tExpected parsed claims to match from token. Diff:\n%s", tests.Failed, diff)
			}
			t.Logf("\t%s\tAuthenticate parse claims from token ok.", tests.Success)

			// Try switching to a second account using the first set of claims.
			tkn2, err := SwitchAccount(ctx, test.MasterDB, tknGen, claims1, acc2.ID, time.Hour, now)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tSwitchAccount user failed.", tests.Failed)
			}
			t.Logf("\t%s\tSwitchAccount user ok.", tests.Success)

			// Ensure the token string was correctly generated.
			claims2, err := tknGen.ParseClaims(tkn2.AccessToken)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tParse claims from token failed.", tests.Failed)
			}

			// Hack for Unhandled Exception in go-cmp@v0.3.0/cmp/options.go:229
			resClaims, _ = json.Marshal(claims2)
			expectClaims, _ = json.Marshal(tkn2.claims)
			if diff := cmp.Diff(string(resClaims), string(expectClaims)); diff != "" {
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

		tknGen := &auth.MockTokenGenerator{}

		// Create a new user for testing.
		usrAcc, err := user_account.MockUserAccount(ctx, test.MasterDB, now, user_account.UserAccountRole_User)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}
		t.Logf("\t%s\tCreate user account ok.", tests.Success)

		// Verify that the user can be authenticated with the created user.
		_, err = Authenticate(ctx, test.MasterDB, tknGen, usrAcc.User.Email, usrAcc.User.Password, time.Hour, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tAuthenticate failed.", tests.Failed)
		}

		// Update the users password.
		newPass := uuid.NewRandom().String()
		err = user.UpdatePassword(ctx, auth.Claims{}, test.MasterDB, user.UserUpdatePasswordRequest{
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
		_, err = Authenticate(ctx, test.MasterDB, tknGen, usrAcc.User.Email, newPass, time.Hour, now)
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

		tknGen := &auth.MockTokenGenerator{}

		// Create a new user for testing.
		usrAcc, err := user_account.MockUserAccount(ctx, test.MasterDB, now, user_account.UserAccountRole_User)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}
		t.Logf("\t%s\tCreate user account ok.", tests.Success)

		// Mock the methods needed to make a password reset.
		resetUrl := func(string) string {
			return ""
		}
		notify := &notify.MockEmail{}

		secretKey := "6368616e676520746869732070617373"

		ttl := time.Hour

		// Make the reset password request.
		resetHash, err := user.ResetPassword(ctx, test.MasterDB, resetUrl, notify, user.UserResetPasswordRequest{
			Email: usrAcc.User.Email,
			TTL:   ttl,
		}, secretKey, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tResetPassword failed.", tests.Failed)
		}
		t.Logf("\t%s\tResetPassword ok.", tests.Success)

		// Assuming we have received the email and clicked the link, we now can ensure confirm works.
		newPass := uuid.NewRandom().String()
		reset, err := user.ResetConfirm(ctx, test.MasterDB, user.UserResetConfirmRequest{
			ResetHash:       resetHash,
			Password:        newPass,
			PasswordConfirm: newPass,
		}, secretKey, now)
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
		_, err = Authenticate(ctx, test.MasterDB, tknGen, usrAcc.User.Email, newPass, time.Hour, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tAuthenticate failed.", tests.Failed)
		}
		t.Logf("\t%s\tAuthenticate ok.", tests.Success)
	}
}
