package invite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/notify"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var (
	// ErrInviteExpired occurs when the the reset hash exceeds the expiration.
	ErrInviteExpired = errors.New("Invite expired")

	// ErrInviteUserPasswordSet occurs when the the reset hash exceeds the expiration.
	ErrInviteUserPasswordSet = errors.New("User password set")
)

// SendUserInvites sends emails to the users inviting them to join an account.
func SendUserInvites(ctx context.Context, claims auth.Claims, dbConn *sqlx.DB, resetUrl func(string) string, notify notify.Email, req SendUserInvitesRequest, secretKey string, now time.Time) ([]string, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.invite.SendUserInvites")
	defer span.Finish()

	v := webcontext.Validator()

	// Validate the request.
	err := v.StructCtx(ctx, req)
	if err != nil {
		return nil, err
	}

	// Ensure the claims can modify the account specified in the request.
	err = user_account.CanModifyAccount(ctx, claims, dbConn, req.AccountID)
	if err != nil {
		return nil, err
	}

	// Find all the users by email address.
	emailUserIDs := make(map[string]string)
	{
		// Find all users without passing in claims to search all users.
		where := fmt.Sprintf("email in ('%s')", strings.Join(req.Emails, "','"))
		users, err := user.Find(ctx, auth.Claims{}, dbConn, user.UserFindRequest{
			Where: &where,
		})
		if err != nil {
			return nil, err
		}

		for _, u := range users {
			emailUserIDs[u.Email] = u.ID
		}
	}

	// Find users that are already active for this account.
	activelUserIDs := make(map[string]bool)
	{
		var args []string
		for _, userID := range emailUserIDs {
			args = append(args, userID)
		}

		where := fmt.Sprintf("user_id in ('%s') and status = '%s'", strings.Join(args, "','"), user_account.UserAccountStatus_Active.String())
		userAccs, err := user_account.Find(ctx, claims, dbConn, user_account.UserAccountFindRequest{
			Where: &where,
		})
		if err != nil {
			return nil, err
		}

		for _, userAcc := range userAccs {
			activelUserIDs[userAcc.UserID] = true
		}
	}

	// Always store the time as UTC.
	now = now.UTC()

	// Postgres truncates times to milliseconds when storing. We and do the same
	// here so the value we return is consistent with what we store.
	now = now.Truncate(time.Millisecond)

	// Create any users that don't already exist.
	for _, email := range req.Emails {
		if uId, ok := emailUserIDs[email]; ok && uId != "" {
			continue
		}

		u, err := user.CreateInvite(ctx, claims, dbConn, user.UserCreateInviteRequest{
			Email: email,
		}, now)
		if err != nil {
			return nil, err
		}

		emailUserIDs[email] = u.ID
	}

	// Loop through all the existing users who either do not have an user_account record or
	// have an existing record, but the status is disabled.
	for _, userID := range emailUserIDs {
		// User already is active, skip.
		if activelUserIDs[userID] {
			continue
		}

		status := user_account.UserAccountStatus_Invited
		_, err = user_account.Create(ctx, claims, dbConn, user_account.UserAccountCreateRequest{
			UserID:    userID,
			AccountID: req.AccountID,
			Roles:     req.Roles,
			Status:    &status,
		}, now)
		if err != nil {
			return nil, err
		}
	}

	if req.TTL.Seconds() == 0 {
		req.TTL = time.Minute * 90
	}

	fromUser, err := user.ReadByID(ctx, claims, dbConn, req.UserID)
	if err != nil {
		return nil, err
	}

	account, err := account.ReadByID(ctx, claims, dbConn, req.AccountID)
	if err != nil {
		return nil, err
	}

	// Load the current IP makings the request.
	var requestIp string
	if vals, _ := webcontext.ContextValues(ctx); vals != nil {
		requestIp = vals.RequestIP
	}

	var inviteHashes []string
	for email, userID := range emailUserIDs {
		hash, err := NewInviteHash(ctx, secretKey, userID, requestIp, req.TTL, now)
		if err != nil {
			return nil, err
		}

		data := map[string]interface{}{
			"FromUser": fromUser.Response(ctx),
			"Account":  account.Response(ctx),
			"Url":      resetUrl(hash),
			"Minutes":  req.TTL.Minutes(),
		}

		subject := fmt.Sprintf("%s %s has invited you to %s", fromUser.FirstName, fromUser.LastName, account.Name)

		err = notify.Send(ctx, email, subject, "user_invite", data)
		if err != nil {
			err = errors.WithMessagef(err, "Send invite to %s failed.", email)
			return nil, err
		}

		inviteHashes = append(inviteHashes, hash)
	}

	return inviteHashes, nil
}

// AcceptInvite updates the user using the provided invite hash.
func AcceptInvite(ctx context.Context, dbConn *sqlx.DB, req AcceptInviteRequest, secretKey string, now time.Time) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.invite.AcceptInvite")
	defer span.Finish()

	v := webcontext.Validator()

	// Validate the request.
	err := v.StructCtx(ctx, req)
	if err != nil {
		return err
	}

	hash, err := ParseInviteHash(ctx, secretKey, req.InviteHash, now)
	if err != nil {
		return err
	}

	u, err := user.Read(ctx, auth.Claims{}, dbConn,
		user.UserReadRequest{ID: hash.UserID, IncludeArchived: true})
	if err != nil {
		return err
	}

	if u.ArchivedAt != nil && !u.ArchivedAt.Time.IsZero() {
		err = user.Restore(ctx, auth.Claims{}, dbConn, user.UserRestoreRequest{ID: hash.UserID}, now)
		if err != nil {
			return err
		}
	} else if len(u.PasswordHash) > 0 {
		// Do not update the password for a user that already has a password set.
		err = errors.WithMessage(ErrInviteUserPasswordSet, "Invite user already has a password set.")
		return err
	}

	err = user.Update(ctx, auth.Claims{}, dbConn, user.UserUpdateRequest{
		ID:        hash.UserID,
		Email:    &req.Email,
		FirstName: &req.FirstName,
		LastName:  &req.LastName,
		Timezone:  req.Timezone,
	}, now)
	if err != nil {
		return err
	}

	err = user.UpdatePassword(ctx, auth.Claims{}, dbConn, user.UserUpdatePasswordRequest{
		ID:              hash.UserID,
		Password:        req.Password,
		PasswordConfirm: req.PasswordConfirm,
	}, now)
	if err != nil {
		return err
	}

	return nil
}
