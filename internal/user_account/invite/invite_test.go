package invite

import (
	"os"
	"strings"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/notify"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"github.com/dgrijalva/jwt-go"
	"github.com/huandu/go-sqlbuilder"
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

// TestInviteUsers validates that invite users works.
func TestInviteUsers(t *testing.T) {

	t.Log("Given the need ensure a user an invite users to their account.")
	{
		ctx := tests.Context()

		now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

		// Create a new user for testing.
		initPass := uuid.NewRandom().String()
		u, err := user.Create(ctx, auth.Claims{}, test.MasterDB, user.UserCreateRequest{
			FirstName:       "Lee",
			LastName:        "Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        initPass,
			PasswordConfirm: initPass,
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user failed.", tests.Failed)
		}

		a, err := account.Create(ctx, auth.Claims{}, test.MasterDB, account.AccountCreateRequest{
			Name:     uuid.NewRandom().String(),
			Address1: "101 E Main",
			City:     "Valdez",
			Region:   "AK",
			Country:  "US",
			Zipcode:  "99686",
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate account failed.", tests.Failed)
		}

		uRoles := []user_account.UserAccountRole{user_account.UserAccountRole_Admin}
		_, err = user_account.Create(ctx, auth.Claims{}, test.MasterDB, user_account.UserAccountCreateRequest{
			UserID:    u.ID,
			AccountID: a.ID,
			Roles:     uRoles,
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate account failed.", tests.Failed)
		}

		claims := auth.Claims{
			AccountIds: []string{a.ID},
			StandardClaims: jwt.StandardClaims{
				Subject:   u.ID,
				Audience:  a.ID,
				IssuedAt:  now.Unix(),
				ExpiresAt: now.Add(time.Hour).Unix(),
			},
		}
		for _, r := range uRoles {
			claims.Roles = append(claims.Roles, r.String())
		}

		// Mock the methods needed to make a password reset.
		resetUrl := func(string) string {
			return ""
		}
		notify := &notify.MockEmail{}

		secretKey := "6368616e676520746869732070617373"

		// Ensure validation is working by trying ResetPassword with an empty request.
		{
			expectedErr := errors.New("Key: 'InviteUsersRequest.account_id' Error:Field validation for 'account_id' failed on the 'required' tag\n" +
				"Key: 'InviteUsersRequest.user_id' Error:Field validation for 'user_id' failed on the 'required' tag\n" +
				"Key: 'InviteUsersRequest.emails' Error:Field validation for 'emails' failed on the 'required' tag\n" +
				"Key: 'InviteUsersRequest.roles' Error:Field validation for 'roles' failed on the 'required' tag")
			_, err = InviteUsers(ctx, claims, test.MasterDB, resetUrl, notify, InviteUsersRequest{}, secretKey, now)
			if err == nil {
				t.Logf("\t\tWant: %+v", expectedErr)
				t.Fatalf("\t%s\tInviteUsers failed.", tests.Failed)
			}

			errStr := strings.Replace(err.Error(), "{{", "", -1)
			errStr = strings.Replace(errStr, "}}", "", -1)

			if errStr != expectedErr.Error() {
				t.Logf("\t\tGot : %+v", errStr)
				t.Logf("\t\tWant: %+v", expectedErr)
				t.Fatalf("\t%s\tInviteUsers Validation failed.", tests.Failed)
			}
			t.Logf("\t%s\tInviteUsers Validation ok.", tests.Success)
		}

		ttl := time.Hour

		inviteEmails := []string{
			uuid.NewRandom().String() + "@geeksinthewoods.com",
		}

		// Make the reset password request.
		inviteHashes, err := InviteUsers(ctx, claims, test.MasterDB, resetUrl, notify, InviteUsersRequest{
			UserID:    u.ID,
			AccountID: a.ID,
			Emails:    inviteEmails,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole_User},
			TTL:       ttl,
		}, secretKey, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tInviteUsers failed.", tests.Failed)
		} else if len(inviteHashes) != len(inviteEmails) {
			t.Logf("\t\tGot : %+v", len(inviteHashes))
			t.Logf("\t\tWant: %+v", len(inviteEmails))
			t.Fatalf("\t%s\tInviteUsers failed.", tests.Failed)
		}
		t.Logf("\t%s\tInviteUsers ok.", tests.Success)

		// Ensure validation is working by trying ResetConfirm with an empty request.
		{
			expectedErr := errors.New("Key: 'InviteAcceptRequest.invite_hash' Error:Field validation for 'invite_hash' failed on the 'required' tag\n" +
				"Key: 'InviteAcceptRequest.first_name' Error:Field validation for 'first_name' failed on the 'required' tag\n" +
				"Key: 'InviteAcceptRequest.last_name' Error:Field validation for 'last_name' failed on the 'required' tag\n" +
				"Key: 'InviteAcceptRequest.password' Error:Field validation for 'password' failed on the 'required' tag\n" +
				"Key: 'InviteAcceptRequest.password_confirm' Error:Field validation for 'password_confirm' failed on the 'required' tag")
			err = InviteAccept(ctx, test.MasterDB, InviteAcceptRequest{}, secretKey, now)
			if err == nil {
				t.Logf("\t\tWant: %+v", expectedErr)
				t.Fatalf("\t%s\tResetConfirm failed.", tests.Failed)
			}

			errStr := strings.Replace(err.Error(), "{{", "", -1)
			errStr = strings.Replace(errStr, "}}", "", -1)

			if errStr != expectedErr.Error() {
				t.Logf("\t\tGot : %+v", errStr)
				t.Logf("\t\tWant: %+v", expectedErr)
				t.Fatalf("\t%s\tResetConfirm Validation failed.", tests.Failed)
			}
			t.Logf("\t%s\tResetConfirm Validation ok.", tests.Success)
		}

		// Ensure the TTL is enforced.
		{
			newPass := uuid.NewRandom().String()
			err = InviteAccept(ctx, test.MasterDB, InviteAcceptRequest{
				InviteHash:      inviteHashes[0],
				FirstName:       "Foo",
				LastName:        "Bar",
				Password:        newPass,
				PasswordConfirm: newPass,
			}, secretKey, now.UTC().Add(ttl*2))
			if errors.Cause(err) != ErrInviteExpired {
				t.Logf("\t\tGot : %+v", errors.Cause(err))
				t.Logf("\t\tWant: %+v", ErrInviteExpired)
				t.Fatalf("\t%s\tInviteAccept enforce TTL failed.", tests.Failed)
			}
			t.Logf("\t%s\tInviteAccept enforce TTL ok.", tests.Success)
		}

		// Assuming we have received the email and clicked the link, we now can ensure accept works.
		for _, inviteHash := range inviteHashes {
			newPass := uuid.NewRandom().String()
			err = InviteAccept(ctx, test.MasterDB, InviteAcceptRequest{
				InviteHash:      inviteHash,
				FirstName:       "Foo",
				LastName:        "Bar",
				Password:        newPass,
				PasswordConfirm: newPass,
			}, secretKey, now)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tInviteAccept failed.", tests.Failed)
			}
			t.Logf("\t%s\tInviteAccept ok.", tests.Success)
		}

		// Ensure the reset hash does not work after its used.
		{
			newPass := uuid.NewRandom().String()
			err = InviteAccept(ctx, test.MasterDB, InviteAcceptRequest{
				InviteHash:      inviteHashes[0],
				FirstName:       "Foo",
				LastName:        "Bar",
				Password:        newPass,
				PasswordConfirm: newPass,
			}, secretKey, now)
			if errors.Cause(err) != ErrInviteUserPasswordSet {
				t.Logf("\t\tGot : %+v", errors.Cause(err))
				t.Logf("\t\tWant: %+v", ErrInviteUserPasswordSet)
				t.Fatalf("\t%s\tInviteAccept verify reuse failed.", tests.Failed)
			}
			t.Logf("\t%s\tInviteAccept verify reuse disabled ok.", tests.Success)
		}
	}
}

func mockAccount(accountId string, now time.Time) error {

	// Build the insert SQL statement.
	query := sqlbuilder.NewInsertBuilder()
	query.InsertInto("accounts")
	query.Cols("id", "name", "created_at", "updated_at")
	query.Values(accountId, uuid.NewRandom().String(), now, now)

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = test.MasterDB.Rebind(sql)
	_, err := test.MasterDB.ExecContext(tests.Context(), sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		return err
	}

	return nil
}

func mockUser(userId string, now time.Time) error {

	// Build the insert SQL statement.
	query := sqlbuilder.NewInsertBuilder()
	query.InsertInto("users")
	query.Cols("id", "email", "password_hash", "password_salt", "created_at", "updated_at")
	query.Values(userId, uuid.NewRandom().String(), "-", "-", now, now)

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = test.MasterDB.Rebind(sql)
	_, err := test.MasterDB.ExecContext(tests.Context(), sql, args...)
	if err != nil {
		err = errors.Wrapf(err, "query - %s", query.String())
		return err
	}

	return nil
}
