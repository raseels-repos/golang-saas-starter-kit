package user

import (
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"github.com/google/go-cmp/cmp"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
)

// TestAuthenticate validates the behavior around authenticating users.
func TestAuthenticate(t *testing.T) {
	defer tests.Recover(t)

	t.Log("Given the need to authenticate users")
	{
		t.Log("\tWhen handling a single User.")
		{
			ctx := tests.Context()

			tknGen := &MockTokenGenerator{}

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
			initPass := uuid.NewRandom().String()
			user, err := Create(ctx, auth.Claims{}, test.MasterDB, UserCreateRequest{
				Name:            "Lee Brown",
				Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
				Password:        initPass,
				PasswordConfirm: initPass,
			}, now)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tCreate user failed.", tests.Failed)
			}
			t.Logf("\t%s\tCreate user ok.", tests.Success)

			// Create a new random account.
			account1Id := uuid.NewRandom().String()
			err = mockAccount(account1Id, user.CreatedAt)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tCreate account failed.", tests.Failed)
			}

			// Associate new account with user user. This defined role should be the claims.
			account1Role := auth.RoleAdmin
			err = mockUserAccount(user.ID, account1Id, user.CreatedAt, account1Role)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
			}

			// Create a second new random account. Need to ensure
			account2Id := uuid.NewRandom().String()
			err = mockAccount(account2Id, user.CreatedAt)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tCreate account failed.", tests.Failed)
			}

			// Associate second new account with user user. Need to ensure that now
			// is always greater than the first user_account entry created so it will
			// be returned consistently back in the same order, last.
			account2Role := auth.RoleUser
			err = mockUserAccount(user.ID, account2Id, user.CreatedAt.Add(time.Second), account2Role)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
			}

			// Add 30 minutes to now to simulate time passing.
			now = now.Add(time.Minute * 30)

			// Try to authenticate valid user with invalid password.
			_, err = Authenticate(ctx, test.MasterDB, tknGen, user.Email, "xy7", time.Hour, now)
			if errors.Cause(err) != ErrAuthenticationFailure {
				t.Logf("\t\tGot : %+v", err)
				t.Logf("\t\tWant: %+v", ErrAuthenticationFailure)
				t.Fatalf("\t%s\tAuthenticate user w/invalid password failed.", tests.Failed)
			}
			t.Logf("\t%s\tAuthenticate user w/invalid password ok.", tests.Success)

			// Verify that the user can be authenticated with the created user.
			tkn1, err := Authenticate(ctx, test.MasterDB, tknGen, user.Email, initPass, time.Hour, now)
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
			} else if diff := cmp.Diff(claims1, tkn1.claims); diff != "" {
				t.Fatalf("\t%s\tExpected parsed claims to match from token. Diff:\n%s", tests.Failed, diff)
			} else if diff := cmp.Diff(claims1.Roles, []string{account1Role}); diff != "" {
				t.Fatalf("\t%s\tExpected parsed claims roles to match user account. Diff:\n%s", tests.Failed, diff)
			} else if diff := cmp.Diff(claims1.AccountIds, []string{account1Id, account2Id}); diff != "" {
				t.Fatalf("\t%s\tExpected parsed claims account IDs to match the single user account. Diff:\n%s", tests.Failed, diff)
			}
			t.Logf("\t%s\tAuthenticate parse claims from token ok.", tests.Success)

			// Try switching to a second account using the first set of claims.
			tkn2, err := SwitchAccount(ctx, test.MasterDB, tknGen, claims1, account2Id, time.Hour, now)
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
			} else if diff := cmp.Diff(claims2, tkn2.claims); diff != "" {
				t.Fatalf("\t%s\tExpected parsed claims to match from token. Diff:\n%s", tests.Failed, diff)
			} else if diff := cmp.Diff(claims2.Roles, []string{account2Role}); diff != "" {
				t.Fatalf("\t%s\tExpected parsed claims roles to match user account. Diff:\n%s", tests.Failed, diff)
			} else if diff := cmp.Diff(claims2.AccountIds, []string{account1Id, account2Id}); diff != "" {
				t.Fatalf("\t%s\tExpected parsed claims account IDs to match the single user account. Diff:\n%s", tests.Failed, diff)
			}
			t.Logf("\t%s\tSwitchAccount parse claims from token ok.", tests.Success)
		}
	}
}
