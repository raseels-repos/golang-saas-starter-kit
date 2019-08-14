package user_account

import (
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"github.com/dgrijalva/jwt-go"
	"github.com/google/go-cmp/cmp"
	"github.com/huandu/go-sqlbuilder"
	"github.com/lib/pq"
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

	repo = NewRepository(test.MasterDB)

	return m.Run()
}

// TestFindRequestQuery validates findRequestQuery
func TestFindRequestQuery(t *testing.T) {

	var (
		limit  uint = 12
		offset uint = 34
	)

	req := UserAccountFindRequest{
		Where: "account_id = ? or user_id = ?",
		Args: []interface{}{
			"xy7",
			"qwert",
		},
		Order: []string{
			"id asc",
			"created_at desc",
		},
		Limit:  &limit,
		Offset: &offset,
	}
	expected := "SELECT " + userAccountMapColumns + " FROM " + userAccountTableName + " WHERE (account_id = ? or user_id = ?) ORDER BY id asc, created_at desc LIMIT 12 OFFSET 34"

	res, args := findRequestQuery(req)

	if diff := cmp.Diff(res.String(), expected); diff != "" {
		t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
	}
	if diff := cmp.Diff(args, req.Args); diff != "" {
		t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
	}
}

// TestApplyClaimsSelectvalidates applyClaimsSelect
func TestApplyClaimsSelectvalidates(t *testing.T) {
	var claimTests = []struct {
		name        string
		claims      auth.Claims
		expectedSql string
		error       error
	}{
		{"EmptyClaims",
			auth.Claims{},
			"SELECT " + userAccountMapColumns + " FROM " + userAccountTableName,
			nil,
		},
		{"RoleUser",
			auth.Claims{
				Roles: []string{auth.RoleUser},
				StandardClaims: jwt.StandardClaims{
					Subject:  "user1",
					Audience: "acc1",
				},
			},
			"SELECT " + userAccountMapColumns + " FROM " + userAccountTableName + " WHERE id IN (SELECT id FROM " + userAccountTableName + " WHERE (account_id = 'acc1' OR user_id = 'user1'))",
			nil,
		},
		{"RoleAdmin",
			auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Subject:  "user1",
					Audience: "acc1",
				},
			},
			"SELECT " + userAccountMapColumns + " FROM " + userAccountTableName + " WHERE id IN (SELECT id FROM " + userAccountTableName + " WHERE (account_id = 'acc1' OR user_id = 'user1'))",
			nil,
		},
	}

	t.Log("Given the need to validate ACLs are enforced by claims to a select query.")
	{
		for i, tt := range claimTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				query := selectQuery()

				err := applyClaimsSelect(ctx, tt.claims, query)
				if err != tt.error {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.error)
					t.Fatalf("\t%s\tapplyClaimsSelect failed.", tests.Failed)
				}

				sql, args := query.Build()

				// Use mysql flavor so placeholders will get replaced for comparison.
				sql, err = sqlbuilder.MySQL.Interpolate(sql, args)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tapplyClaimsSelect failed.", tests.Failed)
				}

				if diff := cmp.Diff(sql, tt.expectedSql); diff != "" {
					t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
				}

				t.Logf("\t%s\tapplyClaimsSelect ok.", tests.Success)
			}
		}
	}
}

// TestCreateValidation ensures all the validation tags work on user account create.
func TestCreateValidation(t *testing.T) {

	invalidRole := UserAccountRole("moon")
	invalidStatus := UserAccountStatus("moon")

	var accountTests = []struct {
		name     string
		req      UserAccountCreateRequest
		expected func(req UserAccountCreateRequest, res *UserAccount) *UserAccount
		error    error
	}{
		{"Required Fields",
			UserAccountCreateRequest{},
			func(req UserAccountCreateRequest, res *UserAccount) *UserAccount {
				return nil
			},
			errors.New("Key: 'UserAccountCreateRequest.user_id' Error:Field validation for 'user_id' failed on the 'required' tag\n" +
				"Key: 'UserAccountCreateRequest.account_id' Error:Field validation for 'account_id' failed on the 'required' tag\n" +
				"Key: 'UserAccountCreateRequest.roles' Error:Field validation for 'roles' failed on the 'required' tag"),
		},
		{"Valid Role",
			UserAccountCreateRequest{
				UserID:    uuid.NewRandom().String(),
				AccountID: uuid.NewRandom().String(),
				Roles:     []UserAccountRole{invalidRole},
			},
			func(req UserAccountCreateRequest, res *UserAccount) *UserAccount {
				return nil
			},
			errors.New("Key: 'UserAccountCreateRequest.roles[0]' Error:Field validation for 'roles[0]' failed on the 'oneof' tag"),
		},
		{"Valid Status",
			UserAccountCreateRequest{
				UserID:    uuid.NewRandom().String(),
				AccountID: uuid.NewRandom().String(),
				Roles:     []UserAccountRole{UserAccountRole_User},
				Status:    &invalidStatus,
			},
			func(req UserAccountCreateRequest, res *UserAccount) *UserAccount {
				return nil
			},
			errors.New("Key: 'UserAccountCreateRequest.status' Error:Field validation for 'status' failed on the 'oneof' tag"),
		},
		{"Default Status",
			UserAccountCreateRequest{
				UserID:    uuid.NewRandom().String(),
				AccountID: uuid.NewRandom().String(),
				Roles:     []UserAccountRole{UserAccountRole_User},
			},
			func(req UserAccountCreateRequest, res *UserAccount) *UserAccount {
				return &UserAccount{
					UserID:    req.UserID,
					AccountID: req.AccountID,
					Roles:     req.Roles,
					Status:    UserAccountStatus_Active,

					// Copy this fields from the result.
					//ID:        res.ID,
					CreatedAt: res.CreatedAt,
					UpdatedAt: res.UpdatedAt,
					//ArchivedAt: nil,
				}
			},
			nil,
		},
	}

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure all validation tags are working for create user account.")
	{
		for i, tt := range accountTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				// Generate a new random user.
				err := mockUser(tt.req.UserID, now)
				if err != nil {
					t.Logf("\t\tGot : %+v", err)
					t.Fatalf("\t%s\tMock user failed.", tests.Failed)
				}

				// Generate a new random account.
				err = mockAccount(tt.req.AccountID, now)
				if err != nil {
					t.Logf("\t\tGot : %+v", err)
					t.Fatalf("\t%s\tMock account failed.", tests.Failed)
				}

				res, err := repo.Create(ctx, auth.Claims{}, tt.req, now)
				if err != tt.error {
					// TODO: need a better way to handle validation errors as they are
					// 		 of type interface validator.ValidationErrorsTranslations
					var errStr string
					if err != nil {
						errStr = strings.Replace(err.Error(), "{{", "", -1)
						errStr = strings.Replace(errStr, "}}", "", -1)
					}
					var expectStr string
					if tt.error != nil {
						expectStr = tt.error.Error()
					}
					if errStr != expectStr {
						t.Logf("\t\tGot : %+v", errStr)
						t.Logf("\t\tWant: %+v", expectStr)
						t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
					}
				}

				// If there was an error that was expected, then don't go any further
				if tt.error != nil {
					t.Logf("\t%s\tCreate user account ok.", tests.Success)
					continue
				}

				expected := tt.expected(tt.req, res)
				if diff := cmp.Diff(res, expected); diff != "" {
					t.Fatalf("\t%s\tCreate user account result should match. Diff:\n%s", tests.Failed, diff)
				}

				t.Logf("\t%s\tCreate user account ok.", tests.Success)
			}
		}
	}
}

// TestCreateExistingEntry ensures that if an archived user account exist,
// the entry is updated rather than erroring on duplicate constraint.
func TestCreateExistingEntry(t *testing.T) {

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure duplicate entries for the same user ID + account ID are updated and does not throw a duplicate key error.")
	{
		ctx := tests.Context()

		// Generate a new random user.
		userID := uuid.NewRandom().String()
		err := mockUser(userID, now)
		if err != nil {
			t.Logf("\t\tGot : %+v", err)
			t.Fatalf("\t%s\tMock user failed.", tests.Failed)
		}

		// Generate a new random account.
		accountID := uuid.NewRandom().String()
		err = mockAccount(accountID, now)
		if err != nil {
			t.Logf("\t\tGot : %+v", err)
			t.Fatalf("\t%s\tMock account failed.", tests.Failed)
		}

		req1 := UserAccountCreateRequest{
			UserID:    userID,
			AccountID: accountID,
			Roles:     []UserAccountRole{UserAccountRole_User},
		}
		ua1, err := repo.Create(ctx, auth.Claims{}, req1, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		} else if diff := cmp.Diff(ua1.Roles, req1.Roles); diff != "" {
			t.Fatalf("\t%s\tCreate user account roles should match request. Diff:\n%s", tests.Failed, diff)
		}

		req2 := UserAccountCreateRequest{
			UserID:    req1.UserID,
			AccountID: req1.AccountID,
			Roles:     []UserAccountRole{UserAccountRole_Admin},
		}
		ua2, err := repo.Create(ctx, auth.Claims{}, req2, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		} else if diff := cmp.Diff(ua2.Roles, req2.Roles); diff != "" {
			t.Fatalf("\t%s\tCreate user account roles should match request. Diff:\n%s", tests.Failed, diff)
		}

		// Now archive the user account to test trying to create a new entry for an archived entry
		err = repo.Archive(tests.Context(), auth.Claims{}, UserAccountArchiveRequest{
			UserID:    req1.UserID,
			AccountID: req1.AccountID,
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tArchive user account failed.", tests.Failed)
		}

		// Find the archived user account
		arcRes, err := repo.Read(tests.Context(), auth.Claims{},
			UserAccountReadRequest{UserID: req1.UserID, AccountID: req1.AccountID, IncludeArchived: true})
		if err != nil || arcRes == nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tFind user account failed.", tests.Failed)
		} else if !arcRes.ArchivedAt.Valid || arcRes.ArchivedAt.Time.IsZero() {
			t.Fatalf("\t%s\tExpected user account to have archived_at set", tests.Failed)
		}

		// Attempt to create the duplicate user account which should set archived_at back to nil
		req3 := UserAccountCreateRequest{
			UserID:    req1.UserID,
			AccountID: req1.AccountID,
			Roles:     []UserAccountRole{UserAccountRole_User},
		}
		ua3, err := repo.Create(ctx, auth.Claims{}, req3, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		} else if diff := cmp.Diff(ua3.Roles, req3.Roles); diff != "" {
			t.Fatalf("\t%s\tCreate user account roles should match request. Diff:\n%s", tests.Failed, diff)
		}

		// Ensure the user account has archived_at empty
		findRes, err := repo.Read(tests.Context(), auth.Claims{},
			UserAccountReadRequest{UserID: req1.UserID, AccountID: req1.AccountID})
		if err != nil || arcRes == nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tFind user account failed.", tests.Failed)
		} else if findRes.ArchivedAt != nil && findRes.ArchivedAt.Valid && !findRes.ArchivedAt.Time.IsZero() {
			t.Fatalf("\t%s\tExpected user account to have archived_at empty", tests.Failed)
		}

		t.Logf("\t%s\tCreate user account ok.", tests.Success)
	}
}

// TestUpdateValidation ensures all the validation tags work on user account update.
func TestUpdateValidation(t *testing.T) {

	invalidRole := UserAccountRole("moon")
	invalidStatus := UserAccountStatus("xxxxxxxxx")

	var accountTests = []struct {
		name  string
		req   UserAccountUpdateRequest
		error error
	}{
		{"Required Fields",
			UserAccountUpdateRequest{},
			errors.New("Key: 'UserAccountUpdateRequest.user_id' Error:Field validation for 'user_id' failed on the 'required' tag\n" +
				"Key: 'UserAccountUpdateRequest.account_id' Error:Field validation for 'account_id' failed on the 'required' tag"),
		},
		{"Valid Role",
			UserAccountUpdateRequest{
				UserID:    uuid.NewRandom().String(),
				AccountID: uuid.NewRandom().String(),
				Roles:     &UserAccountRoles{invalidRole},
			},
			errors.New("Key: 'UserAccountUpdateRequest.roles[0]' Error:Field validation for 'roles[0]' failed on the 'oneof' tag"),
		},

		{"Valid Status",
			UserAccountUpdateRequest{
				UserID:    uuid.NewRandom().String(),
				AccountID: uuid.NewRandom().String(),
				Roles:     &UserAccountRoles{UserAccountRole_User},
				Status:    &invalidStatus,
			},
			errors.New("Key: 'UserAccountUpdateRequest.status' Error:Field validation for 'status' failed on the 'oneof' tag"),
		},
	}

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure all validation tags are working for update user account.")
	{
		for i, tt := range accountTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				err := repo.Update(ctx, auth.Claims{}, tt.req, now)
				if err != tt.error {
					// TODO: need a better way to handle validation errors as they are
					// 		 of type interface validator.ValidationErrorsTranslations
					var errStr string
					if err != nil {
						errStr = strings.Replace(err.Error(), "{{", "", -1)
						errStr = strings.Replace(errStr, "}}", "", -1)
					}
					var expectStr string
					if tt.error != nil {
						expectStr = tt.error.Error()
					}
					if errStr != expectStr {
						t.Logf("\t\tGot : %+v", errStr)
						t.Logf("\t\tWant: %+v", expectStr)
						t.Fatalf("\t%s\tUpdate user account failed.", tests.Failed)
					}
				}

				// If there was an error that was expected, then don't go any further
				if tt.error != nil {
					t.Logf("\t%s\tUpdate user account ok.", tests.Success)
					continue
				}

				t.Logf("\t%s\tUpdate user account ok.", tests.Success)
			}
		}
	}
}

// TestCrud validates the full set of CRUD operations for user accounts and
// ensures ACLs are correctly applied by claims.
func TestCrud(t *testing.T) {
	defer tests.Recover(t)

	type accountTest struct {
		name      string
		claims    func(string, string) auth.Claims
		createErr error
		updateErr error
		findErr   error
	}

	var accountTests []accountTest

	// Internal request, should bypass ACL.
	accountTests = append(accountTests, accountTest{"EmptyClaims",
		func(userID, accountId string) auth.Claims {
			return auth.Claims{}
		},
		nil,
		nil,
		nil,
	})

	// Role of user but claim user does not match update user so forbidden for update.
	accountTests = append(accountTests, accountTest{"RoleUserDiffUser",
		func(userID, accountId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleUser},
				StandardClaims: jwt.StandardClaims{
					Subject:  uuid.NewRandom().String(),
					Audience: accountId,
				},
			}
		},
		ErrForbidden,
		ErrForbidden,
		ErrForbidden,
	})

	// Role of user AND claim user matches update user so OK.
	accountTests = append(accountTests, accountTest{"RoleUserSameUser",
		func(userID, accountId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleUser},
				StandardClaims: jwt.StandardClaims{
					Subject:  userID,
					Audience: accountId,
				},
			}
		},
		ErrForbidden,
		ErrForbidden,
		nil,
	})

	// Role of admin but claim account does not match update user so forbidden.
	accountTests = append(accountTests, accountTest{"RoleAdminDiffUser",
		func(userID, accountId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Subject:  uuid.NewRandom().String(),
					Audience: uuid.NewRandom().String(),
				},
			}
		},
		ErrForbidden,
		ErrForbidden,
		ErrNotFound,
	})

	// Role of admin and claim account matches update user so ok.
	accountTests = append(accountTests, accountTest{"RoleAdminSameAccount",
		func(userID, accountId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Subject:  uuid.NewRandom().String(),
					Audience: accountId,
				},
			}
		},
		nil,
		nil,
		nil,
	})

	t.Log("Given the need to validate CRUD functionality for user accounts and ensure claims are applied as ACL.")
	{
		now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

		for i, tt := range accountTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				// Generate a new random user.
				userID := uuid.NewRandom().String()
				err := mockUser(userID, now)
				if err != nil {
					t.Logf("\t\tGot : %+v", err)
					t.Fatalf("\t%s\tMock user failed.", tests.Failed)
				}

				// Generate a new random account.
				accountID := uuid.NewRandom().String()
				err = mockAccount(accountID, now)
				if err != nil {
					t.Logf("\t\tGot : %+v", err)
					t.Fatalf("\t%s\tMock account failed.", tests.Failed)
				}

				// Associate that with the user.
				createReq := UserAccountCreateRequest{
					UserID:    userID,
					AccountID: accountID,
					Roles:     []UserAccountRole{UserAccountRole_User},
				}
				ua, err := repo.Create(tests.Context(), tt.claims(userID, accountID), createReq, now)
				if err != nil && errors.Cause(err) != tt.createErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.createErr)
					t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
				} else if tt.createErr == nil {
					if diff := cmp.Diff(ua.Roles, createReq.Roles); diff != "" {
						t.Fatalf("\t%s\tExpected user account roles result to match for create. Diff:\n%s", tests.Failed, diff)
					}
					t.Logf("\t%s\tCreate user account ok.", tests.Success)
				}

				if tt.createErr == ErrForbidden {
					ua, err = repo.Create(tests.Context(), auth.Claims{}, createReq, now)
					if err != nil && errors.Cause(err) != tt.createErr {
						t.Logf("\t\tGot : %+v", err)
						t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
					}
				}

				// Update the account.
				updateReq := UserAccountUpdateRequest{
					UserID:    userID,
					AccountID: accountID,
					Roles:     &UserAccountRoles{UserAccountRole_Admin},
				}
				err = repo.Update(tests.Context(), tt.claims(userID, accountID), updateReq, now)
				if err != nil {
					if errors.Cause(err) != tt.updateErr {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", tt.updateErr)
						t.Fatalf("\t%s\tUpdate user account failed.", tests.Failed)
					}
				} else {
					ua.Roles = *updateReq.Roles
				}
				t.Logf("\t%s\tUpdate user account ok.", tests.Success)

				// Find the account for the user to verify the updates where made. There should only
				// be one account associated with the user for this test.
				findRes, err := repo.Find(tests.Context(), tt.claims(userID, accountID), UserAccountFindRequest{
					Where: "user_id = ? or account_id = ?",
					Args:  []interface{}{userID, accountID},
					Order: []string{"created_at"},
				})
				if err != nil && errors.Cause(err) != tt.findErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.findErr)
					t.Fatalf("\t%s\tVerify update user account failed.", tests.Failed)
				} else if tt.findErr == nil {
					var expected UserAccounts = []*UserAccount{
						&UserAccount{
							//ID:        ua.ID,
							UserID:    ua.UserID,
							AccountID: ua.AccountID,
							Roles:     ua.Roles,
							Status:    ua.Status,
							CreatedAt: ua.CreatedAt,
							UpdatedAt: now,
						},
					}
					if diff := cmp.Diff(findRes, expected); diff != "" {
						t.Fatalf("\t%s\tExpected user account find result to match update. Diff:\n%s", tests.Failed, diff)
					}
					t.Logf("\t%s\tVerify update user account ok.", tests.Success)
				}

				// Archive (soft-delete) the user account.
				err = repo.Archive(tests.Context(), tt.claims(userID, accountID), UserAccountArchiveRequest{
					UserID:    userID,
					AccountID: accountID,
				}, now)
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tArchive user account failed.", tests.Failed)
				} else if tt.updateErr == nil {
					// Trying to find the archived user with the includeArchived false should result in not found.
					_, err = repo.FindByUserID(tests.Context(), tt.claims(userID, accountID), userID, false)
					if errors.Cause(err) != ErrNotFound {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", ErrNotFound)
						t.Fatalf("\t%s\tVerify archive user account failed when excluding archived.", tests.Failed)
					}

					// Trying to find the archived user with the includeArchived true should result no error.
					findRes, err = repo.FindByUserID(tests.Context(), tt.claims(userID, accountID), userID, true)
					if err != nil {
						t.Logf("\t\tGot : %+v", err)
						t.Fatalf("\t%s\tVerify archive user account failed when including archived.", tests.Failed)
					}

					var expected UserAccounts = []*UserAccount{
						&UserAccount{
							//ID:         ua.ID,
							UserID:     ua.UserID,
							AccountID:  ua.AccountID,
							Roles:      *updateReq.Roles,
							Status:     ua.Status,
							CreatedAt:  ua.CreatedAt,
							UpdatedAt:  now,
							ArchivedAt: &pq.NullTime{Time: now, Valid: true},
						},
					}
					if diff := cmp.Diff(findRes, expected); diff != "" {
						t.Fatalf("\t%s\tExpected user account find result to be archived. Diff:\n%s", tests.Failed, diff)
					}
				}
				t.Logf("\t%s\tArchive user account ok.", tests.Success)

				// Delete (hard-delete) the user account.
				err = repo.Delete(tests.Context(), tt.claims(userID, accountID), UserAccountDeleteRequest{
					UserID:    userID,
					AccountID: accountID,
				})
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tDelete user account failed.", tests.Failed)
				} else if tt.updateErr == nil {
					// Trying to find the deleted user with the includeArchived true should result in not found.
					_, err = repo.FindByUserID(tests.Context(), tt.claims(userID, accountID), userID, true)
					if errors.Cause(err) != ErrNotFound {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", ErrNotFound)
						t.Fatalf("\t%s\tVerify delete user account failed when including archived.", tests.Failed)
					}
				}
				t.Logf("\t%s\tDelete user account ok.", tests.Success)
			}
		}
	}
}

// TestFind validates all the request params are correctly parsed into a select query.
func TestFind(t *testing.T) {

	now := time.Now().Add(time.Hour * -2).UTC()

	startTime := now.Truncate(time.Millisecond)
	var endTime time.Time

	var userAccounts []*UserAccount
	for i := 0; i <= 4; i++ {
		// Generate a new random user.
		userID := uuid.NewRandom().String()
		err := mockUser(userID, now)
		if err != nil {
			t.Logf("\t\tGot : %+v", err)
			t.Fatalf("\t%s\tCreate user failed.", tests.Failed)
		}

		// Generate a new random account.
		accountID := uuid.NewRandom().String()
		err = mockAccount(accountID, now)
		if err != nil {
			t.Logf("\t\tGot : %+v", err)
			t.Fatalf("\t%s\tCreate account failed.", tests.Failed)
		}

		// Execute Create that will associate the user with the account.
		ua, err := repo.Create(tests.Context(), auth.Claims{}, UserAccountCreateRequest{
			UserID:    userID,
			AccountID: accountID,
			Roles:     []UserAccountRole{UserAccountRole_User},
		}, now.Add(time.Second*time.Duration(i)))
		if err != nil {
			t.Logf("\t\tGot : %+v", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}

		userAccounts = append(userAccounts, ua)
		endTime = ua.CreatedAt
	}

	type accountTest struct {
		name     string
		req      UserAccountFindRequest
		expected UserAccounts
		error    error
	}

	var accountTests []accountTest

	createdFilter := "created_at BETWEEN ? AND ?"

	// Test sort users.
	accountTests = append(accountTests, accountTest{"Find all order by created_at asx",
		UserAccountFindRequest{
			Where: createdFilter,
			Args:  []interface{}{startTime, endTime},
			Order: []string{"created_at"},
		},
		userAccounts,
		nil,
	})

	// Test reverse sorted user accounts.
	var expected []*UserAccount
	for i := len(userAccounts) - 1; i >= 0; i-- {
		expected = append(expected, userAccounts[i])
	}
	accountTests = append(accountTests, accountTest{"Find all order by created_at desc",
		UserAccountFindRequest{
			Where: createdFilter,
			Args:  []interface{}{startTime, endTime},
			Order: []string{"created_at desc"},
		},
		expected,
		nil,
	})

	// Test limit.
	var limit uint = 2
	accountTests = append(accountTests, accountTest{"Find limit",
		UserAccountFindRequest{
			Where: createdFilter,
			Args:  []interface{}{startTime, endTime},
			Order: []string{"created_at"},
			Limit: &limit,
		},
		userAccounts[0:2],
		nil,
	})

	// Test offset.
	var offset uint = 3
	accountTests = append(accountTests, accountTest{"Find limit, offset",
		UserAccountFindRequest{
			Where:  createdFilter,
			Args:   []interface{}{startTime, endTime},
			Order:  []string{"created_at"},
			Limit:  &limit,
			Offset: &offset,
		},
		userAccounts[3:5],
		nil,
	})

	// Test where filter.
	whereParts := []string{}
	whereArgs := []interface{}{startTime, endTime}
	expected = []*UserAccount{}
	for i := 0; i <= len(userAccounts); i++ {
		if rand.Intn(100) < 50 {
			continue
		}
		ua := *userAccounts[i]

		whereParts = append(whereParts, "(user_id = ? and account_id = ?)")
		whereArgs = append(whereArgs, ua.UserID)
		whereArgs = append(whereArgs, ua.AccountID)
		expected = append(expected, &ua)
	}

	accountTests = append(accountTests, accountTest{"Find where",
		UserAccountFindRequest{
			Where: createdFilter + " AND (" + strings.Join(whereParts, " OR ") + ")",
			Args:  whereArgs,
			Order: []string{"created_at"},
		},
		expected,
		nil,
	})

	t.Log("Given the need to ensure find user accounts returns the expected results.")
	{
		for i, tt := range accountTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				res, err := repo.Find(ctx, auth.Claims{}, tt.req)
				if errors.Cause(err) != tt.error {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.error)
					t.Fatalf("\t%s\tFind user account failed.", tests.Failed)
				} else if diff := cmp.Diff(res, tt.expected); diff != "" {
					t.Logf("\t\tGot: %d items", len(res))
					t.Logf("\t\tWant: %d items", len(tt.expected))
					t.Fatalf("\t%s\tExpected user account find result to match expected. Diff:\n%s", tests.Failed, diff)
				}
				t.Logf("\t%s\tFind user account ok.", tests.Success)
			}
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
