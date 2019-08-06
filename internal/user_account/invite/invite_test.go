package invite

import (
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
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

// TestSendUserInvites validates that invite users works.
func TestSendUserInvites(t *testing.T) {

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
			AccountIDs: []string{a.ID},
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
			expectedErr := errors.New("Key: 'SendUserInvitesRequest.account_id' Error:Field validation for 'account_id' failed on the 'required' tag\n" +
				"Key: 'SendUserInvitesRequest.user_id' Error:Field validation for 'user_id' failed on the 'required' tag\n" +
				"Key: 'SendUserInvitesRequest.emails' Error:Field validation for 'emails' failed on the 'required' tag\n" +
				"Key: 'SendUserInvitesRequest.roles' Error:Field validation for 'roles' failed on the 'required' tag")
			_, err = SendUserInvites(ctx, claims, test.MasterDB, resetUrl, notify, SendUserInvitesRequest{}, secretKey, now)
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
		inviteHashes, err := SendUserInvites(ctx, claims, test.MasterDB, resetUrl, notify, SendUserInvitesRequest{
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
			expectedErr := errors.New("Key: 'AcceptInviteUserRequest.invite_hash' Error:Field validation for 'invite_hash' failed on the 'required' tag\n" +
				"Key: 'AcceptInviteUserRequest.email' Error:Field validation for 'email' failed on the 'required' tag\n" +
				"Key: 'AcceptInviteUserRequest.first_name' Error:Field validation for 'first_name' failed on the 'required' tag\n" +
				"Key: 'AcceptInviteUserRequest.last_name' Error:Field validation for 'last_name' failed on the 'required' tag\n" +
				"Key: 'AcceptInviteUserRequest.password' Error:Field validation for 'password' failed on the 'required' tag\n" +
				"Key: 'AcceptInviteUserRequest.password_confirm' Error:Field validation for 'password_confirm' failed on the 'required' tag")
			_, err = AcceptInviteUser(ctx, test.MasterDB, AcceptInviteUserRequest{}, secretKey, now)
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
			_, err = AcceptInviteUser(ctx, test.MasterDB, AcceptInviteUserRequest{
				InviteHash:      inviteHashes[0],
				Email:           inviteEmails[0],
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
		for idx, inviteHash := range inviteHashes {

			newPass := uuid.NewRandom().String()
			hash, err := AcceptInviteUser(ctx, test.MasterDB, AcceptInviteUserRequest{
				InviteHash:      inviteHash,
				Email:           inviteEmails[idx],
				FirstName:       "Foo",
				LastName:        "Bar",
				Password:        newPass,
				PasswordConfirm: newPass,
			}, secretKey, now)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tInviteAccept failed.", tests.Failed)
			}

			// Validate the result.
			var res = struct {
				UserID    string `validate:"required,uuid"`
				AccountID string `validate:"required,uuid"`
			}{
				UserID:    hash.UserID,
				AccountID: hash.AccountID,
			}
			err = webcontext.Validator().StructCtx(ctx, res)
			if err != nil {
				t.Log("\t\tGot :", err)
				t.Fatalf("\t%s\tInviteAccept failed.", tests.Failed)
			}

			t.Logf("\t%s\tInviteAccept ok.", tests.Success)
		}

		// Ensure the reset hash does not work after its used.
		{
			newPass := uuid.NewRandom().String()
			_, err = AcceptInviteUser(ctx, test.MasterDB, AcceptInviteUserRequest{
				InviteHash:      inviteHashes[0],
				Email:           inviteEmails[0],
				FirstName:       "Foo",
				LastName:        "Bar",
				Password:        newPass,
				PasswordConfirm: newPass,
			}, secretKey, now)
			if errors.Cause(err) != ErrUserAccountActive {
				t.Logf("\t\tGot : %+v", errors.Cause(err))
				t.Logf("\t\tWant: %+v", ErrUserAccountActive)
				t.Fatalf("\t%s\tInviteAccept verify reuse failed.", tests.Failed)
			}
			t.Logf("\t%s\tInviteAccept verify reuse disabled ok.", tests.Success)
		}
	}
}
