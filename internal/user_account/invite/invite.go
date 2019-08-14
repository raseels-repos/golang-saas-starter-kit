package invite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"github.com/pkg/errors"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var (
	// ErrInviteExpired occurs when the the reset hash exceeds the expiration.
	ErrInviteExpired = errors.New("Invite expired")

	// ErrNoPendingInvite occurs when the user does not have an entry in user_accounts with status pending.
	ErrNoPendingInvite = errors.New("No pending invite.")

	// ErrUserAccountActive occurs when the user already has an active user_account entry.
	ErrUserAccountActive = errors.New("User already active.")
)

// SendUserInvites sends emails to the users inviting them to join an account.
func (repo *Repository) SendUserInvites(ctx context.Context, claims auth.Claims, req SendUserInvitesRequest, now time.Time) ([]string, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.invite.SendUserInvites")
	defer span.Finish()

	v := webcontext.Validator()

	// Validate the request.
	err := v.StructCtx(ctx, req)
	if err != nil {
		return nil, err
	}

	// Ensure the claims can modify the account specified in the request.
	err = account.CanModifyAccount(ctx, claims, repo.DbConn, req.AccountID)
	if err != nil {
		return nil, err
	}

	// Find all the users by email address.
	emailUserIDs := make(map[string]string)
	{
		// Find all users without passing in claims to search all users.
		users, err := repo.User.Find(ctx, auth.Claims{}, user.UserFindRequest{
			Where: fmt.Sprintf("email in ('%s')",
				strings.Join(req.Emails, "','")),
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

		userAccs, err := repo.UserAccount.Find(ctx, claims, user_account.UserAccountFindRequest{
			Where: fmt.Sprintf("user_id in ('%s') and status = '%s'",
				strings.Join(args, "','"),
				user_account.UserAccountStatus_Active.String()),
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

		u, err := repo.User.CreateInvite(ctx, claims, user.UserCreateInviteRequest{
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
		_, err = repo.UserAccount.Create(ctx, claims, user_account.UserAccountCreateRequest{
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

	fromUser, err := repo.User.ReadByID(ctx, claims, req.UserID)
	if err != nil {
		return nil, err
	}

	account, err := repo.Account.ReadByID(ctx, claims, req.AccountID)
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
		hash, err := NewInviteHash(ctx, repo.secretKey, userID, req.AccountID, requestIp, req.TTL, now)
		if err != nil {
			return nil, err
		}

		data := map[string]interface{}{
			"FromUser": fromUser.Response(ctx),
			"Account":  account.Response(ctx),
			"Url":      repo.ResetUrl(hash),
			"Minutes":  req.TTL.Minutes(),
		}

		subject := fmt.Sprintf("%s %s has invited you to %s", fromUser.FirstName, fromUser.LastName, account.Name)

		err = repo.Notify.Send(ctx, email, subject, "user_invite", data)
		if err != nil {
			err = errors.WithMessagef(err, "Send invite to %s failed.", email)
			return nil, err
		}

		inviteHashes = append(inviteHashes, hash)
	}

	return inviteHashes, nil
}

// AcceptInvite updates the user using the provided invite hash.
func (repo *Repository) AcceptInvite(ctx context.Context, req AcceptInviteRequest, now time.Time) (*user_account.UserAccount, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.invite.AcceptInvite")
	defer span.Finish()

	v := webcontext.Validator()

	// Validate the request.
	err := v.StructCtx(ctx, req)
	if err != nil {
		return nil, err
	}

	hash, err := ParseInviteHash(ctx, req.InviteHash, repo.secretKey, now)
	if err != nil {
		return nil, err
	}

	u, err := repo.User.Read(ctx, auth.Claims{},
		user.UserReadRequest{ID: hash.UserID, IncludeArchived: true})
	if err != nil {
		return nil, err
	}

	if u.ArchivedAt != nil && !u.ArchivedAt.Time.IsZero() {
		err = repo.User.Restore(ctx, auth.Claims{}, user.UserRestoreRequest{ID: hash.UserID}, now)
		if err != nil {
			return nil, err
		}
	}

	usrAcc, err := repo.UserAccount.Read(ctx, auth.Claims{}, user_account.UserAccountReadRequest{
		UserID:    hash.UserID,
		AccountID: hash.AccountID,
	})
	if err != nil {
		return nil, err
	}

	// Ensure the entry has the status of invited.
	if usrAcc.Status != user_account.UserAccountStatus_Invited {
		// If the entry is already active
		if usrAcc.Status == user_account.UserAccountStatus_Active {
			return usrAcc, errors.WithStack(ErrUserAccountActive)
		}
		return usrAcc, errors.WithStack(ErrNoPendingInvite)
	}

	// If the user already has a password set, then just update the user_account entry to status of active.
	// The user will need to login and should not be auto-authenticated.
	if len(u.PasswordHash) > 0 {
		usrAcc.Status = user_account.UserAccountStatus_Active

		err = repo.UserAccount.Update(ctx, auth.Claims{}, user_account.UserAccountUpdateRequest{
			UserID:    usrAcc.UserID,
			AccountID: usrAcc.AccountID,
			Status:    &usrAcc.Status,
		}, now)
		if err != nil {
			return nil, err
		}
	}

	return usrAcc, nil
}

// AcceptInviteUser updates the user using the provided invite hash.
func (repo *Repository) AcceptInviteUser(ctx context.Context, req AcceptInviteUserRequest, now time.Time) (*user_account.UserAccount, error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "internal.user_account.invite.AcceptInviteUser")
	defer span.Finish()

	v := webcontext.Validator()

	// Validate the request.
	err := v.StructCtx(ctx, req)
	if err != nil {
		return nil, err
	}

	hash, err := ParseInviteHash(ctx, req.InviteHash, repo.secretKey, now)
	if err != nil {
		return nil, err
	}

	u, err := repo.User.Read(ctx, auth.Claims{},
		user.UserReadRequest{ID: hash.UserID, IncludeArchived: true})
	if err != nil {
		return nil, err
	}

	if u.ArchivedAt != nil && !u.ArchivedAt.Time.IsZero() {
		err = repo.User.Restore(ctx, auth.Claims{}, user.UserRestoreRequest{ID: hash.UserID}, now)
		if err != nil {
			return nil, err
		}
	}

	usrAcc, err := repo.UserAccount.Read(ctx, auth.Claims{}, user_account.UserAccountReadRequest{
		UserID:    hash.UserID,
		AccountID: hash.AccountID,
	})
	if err != nil {
		return nil, err
	}

	// Ensure the entry has the status of invited.
	if usrAcc.Status != user_account.UserAccountStatus_Invited {
		// If the entry is already active
		if usrAcc.Status == user_account.UserAccountStatus_Active {
			return usrAcc, errors.WithStack(ErrUserAccountActive)
		}
		return nil, errors.WithStack(ErrNoPendingInvite)
	}

	// These three calls, user.Update,  user.UpdatePassword, and user_account.Update
	// should probably be in a transaction!
	err = repo.User.Update(ctx, auth.Claims{}, user.UserUpdateRequest{
		ID:        hash.UserID,
		Email:     &req.Email,
		FirstName: &req.FirstName,
		LastName:  &req.LastName,
		Timezone:  req.Timezone,
	}, now)
	if err != nil {
		return nil, err
	}

	err = repo.User.UpdatePassword(ctx, auth.Claims{}, user.UserUpdatePasswordRequest{
		ID:              hash.UserID,
		Password:        req.Password,
		PasswordConfirm: req.PasswordConfirm,
	}, now)
	if err != nil {
		return nil, err
	}

	usrAcc.Status = user_account.UserAccountStatus_Active
	err = repo.UserAccount.Update(ctx, auth.Claims{}, user_account.UserAccountUpdateRequest{
		UserID:    usrAcc.UserID,
		AccountID: usrAcc.AccountID,
		Status:    &usrAcc.Status,
	}, now)
	if err != nil {
		return nil, err
	}

	return usrAcc, nil
}
