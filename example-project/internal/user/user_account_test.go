package user

import (
	"github.com/lib/pq"
	"math/rand"
	"strings"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"github.com/dgrijalva/jwt-go"
	"github.com/huandu/go-sqlbuilder"
	"github.com/pborman/uuid"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
)

// TestAccountFindRequestQuery validates accountFindRequestQuery
func TestAccountFindRequestQuery(t *testing.T) {
	where := "account_id = ? or user_id = ?"
	var (
		limit  uint = 12
		offset uint = 34
	)

	req := UserAccountFindRequest{
		Where: &where,
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
	expected := "SELECT " + usersAccountsMapColumns + " FROM " + usersAccountsTableName + " WHERE (account_id = ? or user_id = ?) ORDER BY id asc, created_at desc LIMIT 12 OFFSET 34"

	res, args := accountFindRequestQuery(req)

	if diff := cmp.Diff(res.String(), expected); diff != "" {
		t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
	}
	if diff := cmp.Diff(args, req.Args); diff != "" {
		t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
	}
}

// TestApplyClaimsUserAccountSelect validates applyClaimsUserAccountSelect
func TestApplyClaimsUserAccountSelect(t *testing.T) {
	var claimTests = []struct {
		name        string
		claims      auth.Claims
		expectedSql string
		error       error
	}{
		{"EmptyClaims",
			auth.Claims{},
			"SELECT " + usersAccountsMapColumns + " FROM " + usersAccountsTableName,
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
			"SELECT " + usersAccountsMapColumns + " FROM " + usersAccountsTableName + " WHERE user_id IN (SELECT user_id FROM " + usersAccountsTableName + " WHERE (account_id = 'acc1' OR user_id = 'user1'))",
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
			"SELECT " + usersAccountsMapColumns + " FROM " + usersAccountsTableName + " WHERE user_id IN (SELECT user_id FROM " + usersAccountsTableName + " WHERE (account_id = 'acc1' OR user_id = 'user1'))",
			nil,
		},
	}

	t.Log("Given the need to validate ACLs are enforced by claims to a select query.")
	{
		for i, tt := range claimTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				query := accountSelectQuery()

				err := applyClaimsUserAccountSelect(ctx, tt.claims, query)
				if err != tt.error {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.error)
					t.Fatalf("\t%s\tapplyClaimsUserAccountSelect failed.", tests.Failed)
				}

				sql, args := query.Build()

				// Use mysql flavor so placeholders will get replaced for comparison.
				sql, err = sqlbuilder.MySQL.Interpolate(sql, args)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tapplyClaimsUserAccountSelect failed.", tests.Failed)
				}

				if diff := cmp.Diff(sql, tt.expectedSql); diff != "" {
					t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
				}

				t.Logf("\t%s\tapplyClaimsUserAccountSelect ok.", tests.Success)
			}
		}
	}
}

// TestAddAccountValidation ensures all the validation tags work on account add.
func TestAddAccountValidation(t *testing.T) {

	invalidRole := UserAccountRole("moon")
	invalidStatus := UserAccountStatus("moon")


	var accountTests = []struct {
		name     string
		req      AddAccountRequest
		expected func(req AddAccountRequest, res *UserAccount) *UserAccount
		error    error
	}{
		{"Required Fields",
			AddAccountRequest{},
			func(req AddAccountRequest, res *UserAccount) *UserAccount {
				return nil
			},
			errors.New("Key: 'AddAccountRequest.UserID' Error:Field validation for 'UserID' failed on the 'required' tag\n"+
					"Key: 'AddAccountRequest.AccountID' Error:Field validation for 'AccountID' failed on the 'required' tag\n"+
					"Key: 'AddAccountRequest.Roles' Error:Field validation for 'Roles' failed on the 'required' tag"),
		},
		{"Valid Role",
			AddAccountRequest{
				UserID: uuid.NewRandom().String(),
				AccountID: uuid.NewRandom().String(),
				Roles:          []UserAccountRole{invalidRole},
			},
			func(req AddAccountRequest, res *UserAccount) *UserAccount {
				return nil
			},
			errors.New("Key: 'AddAccountRequest.Roles[0]' Error:Field validation for 'Roles[0]' failed on the 'oneof' tag"),
		},
		{"Valid Status",
			AddAccountRequest{
				UserID: uuid.NewRandom().String(),
				AccountID: uuid.NewRandom().String(),
				Roles:          []UserAccountRole{UserAccountRole_User},
				Status:          &invalidStatus,
			},
			func(req AddAccountRequest, res *UserAccount) *UserAccount {
				return nil
			},
			errors.New("Key: 'AddAccountRequest.Status' Error:Field validation for 'Status' failed on the 'oneof' tag"),
		},
		{"Default Status",
			AddAccountRequest{
				UserID: uuid.NewRandom().String(),
				AccountID: uuid.NewRandom().String(),
				Roles:          []UserAccountRole{UserAccountRole_User},
			},
			func(req AddAccountRequest, res *UserAccount) *UserAccount {
				return &UserAccount{
					UserID:     req.UserID,
					AccountID:     req.AccountID,
					Roles:    req.Roles,
					Status:   UserAccountStatus_Active,

					// Copy this fields from the result.
					ID:            res.ID,
					CreatedAt:     res.CreatedAt,
					UpdatedAt:     res.UpdatedAt,
					//ArchivedAt: nil,
				}
			},
			nil,
		},
	}

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure all validation tags are working for add account.")
	{
		for i, tt := range accountTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				res, err := AddAccount(ctx, auth.Claims{}, test.MasterDB, tt.req, now)
				if err != tt.error {
					// TODO: need a better way to handle validation errors as they are
					// 		 of type interface validator.ValidationErrorsTranslations
					var errStr string
					if err != nil {
						errStr = err.Error()
					}
					var expectStr string
					if tt.error != nil {
						expectStr = tt.error.Error()
					}
					if errStr != expectStr {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", tt.error)
						t.Fatalf("\t%s\tAddAccount failed.", tests.Failed)
					}
				}

				// If there was an error that was expected, then don't go any further
				if tt.error != nil {
					t.Logf("\t%s\tAddAccount ok.", tests.Success)
					continue
				}

				expected := tt.expected(tt.req, res)
				if diff := cmp.Diff(res, expected); diff != "" {
					t.Fatalf("\t%s\tAddAccount result should match. Diff:\n%s", tests.Failed, diff)
				}

				t.Logf("\t%s\tAddAccount ok.", tests.Success)
			}
		}
	}
}

// TestAddAccountExistingEntry validates emails must be unique on add account.
func TestAddAccountExistingEntry(t *testing.T) {

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure duplicate entries for the same user ID + account ID are updated and does not throw a duplicate key error.")
	{
		ctx := tests.Context()

		req1 := AddAccountRequest{
			UserID: uuid.NewRandom().String(),
			AccountID: uuid.NewRandom().String(),
			Roles:          []UserAccountRole{UserAccountRole_User},
		}
		ua1, err := AddAccount(ctx, auth.Claims{}, test.MasterDB, req1, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tAddAccount failed.", tests.Failed)
		}

		if diff := cmp.Diff(ua1.Roles, req1.Roles); diff != "" {
			t.Fatalf("\t%s\tAddAccount roles should match request. Diff:\n%s", tests.Failed, diff)
		}

		req2 := AddAccountRequest{
			UserID: req1.UserID,
			AccountID: req1.AccountID,
			Roles:          []UserAccountRole{UserAccountRole_Admin},
		}
		ua2, err := AddAccount(ctx, auth.Claims{}, test.MasterDB, req2, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tAddAccount failed.", tests.Failed)
		}

		if diff := cmp.Diff(ua2.Roles, req2.Roles); diff != "" {
			t.Fatalf("\t%s\tAddAccount roles should match request. Diff:\n%s", tests.Failed, diff)
		}

		t.Logf("\t%s\tAddAccount ok.", tests.Success)
	}
}

// TestUpdateAccountValidation ensures all the validation tags work on account update.
func TestUpdateAccountValidation(t *testing.T) {

	invalidRole := UserAccountRole("moon")
	invalidStatus := UserAccountStatus("xxxxxxxxx")

	var accountTests = []struct {
		name     string
		req      UpdateAccountRequest
		error    error
	}{
		{"Required Fields",
			UpdateAccountRequest{},
			errors.New("Key: 'UpdateAccountRequest.UserID' Error:Field validation for 'UserID' failed on the 'required' tag\n" +
					"Key: 'UpdateAccountRequest.AccountID' Error:Field validation for 'AccountID' failed on the 'required' tag\n" +
					"Key: 'UpdateAccountRequest.Roles' Error:Field validation for 'Roles' failed on the 'required' tag"),
		},
		{"Valid Role",
			UpdateAccountRequest{
				UserID: uuid.NewRandom().String(),
				AccountID: uuid.NewRandom().String(),
				Roles:          &UserAccountRoles{invalidRole},
			},
			errors.New("Key: 'UpdateAccountRequest.Roles[0]' Error:Field validation for 'Roles[0]' failed on the 'oneof' tag"),
		},

		{"Valid Status",
			UpdateAccountRequest{
				UserID: uuid.NewRandom().String(),
				AccountID: uuid.NewRandom().String(),
				Roles:          &UserAccountRoles{UserAccountRole_User},
				Status: &invalidStatus,
			},
		errors.New("Key: 'UpdateAccountRequest.Status' Error:Field validation for 'Status' failed on the 'oneof' tag"),
		},
	}

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure all validation tags are working for update account.")
	{
		for i, tt := range accountTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				err := UpdateAccount(ctx, auth.Claims{}, test.MasterDB, tt.req, now)
				if err != tt.error {
					// TODO: need a better way to handle validation errors as they are
					// 		 of type interface validator.ValidationErrorsTranslations
					var errStr string
					if err != nil {
						errStr = err.Error()
					}
					var expectStr string
					if tt.error != nil {
						expectStr = tt.error.Error()
					}
					if errStr != expectStr {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", tt.error)
						t.Fatalf("\t%s\tUpdateAccount failed.", tests.Failed)
					}
				}

				// If there was an error that was expected, then don't go any further
				if tt.error != nil {
					t.Logf("\t%s\tUpdateAccount ok.", tests.Success)
					continue
				}

				t.Logf("\t%s\tUpdateAccount ok.", tests.Success)
			}
		}
	}
}

// TestAccountCrud validates the full set of CRUD operations for user accounts and
// ensures ACLs are correctly applied by claims.
func TestAccountCrud(t *testing.T) {
	defer tests.Recover(t)

	type accountTest struct {
		name      string
		claims    func(string, string) auth.Claims
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
	})

	// Role of user but claim user does not match update user so forbidden.
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
		ErrNotFound,
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
		nil,
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
	})

	t.Log("Given the need to validate CRUD functionality for user accounts and ensure claims are applied as ACL.")
	{
		now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

		for i, tt := range accountTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				// Always create the new user with empty claims, testing claims for create user
				// will be handled separately.
				user, err := Create(tests.Context(), auth.Claims{}, test.MasterDB, CreateUserRequest{
					Name:            "Lee Brown",
					Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
					Password:        "akTechFr0n!ier",
					PasswordConfirm: "akTechFr0n!ier",
				}, now)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tCreate user failed.", tests.Failed)
				}

				// Create a new random account and associate that with the user.
				accountID := uuid.NewRandom().String()
				createReq := AddAccountRequest{
					UserID: user.ID,
					AccountID: accountID,
					Roles:          []UserAccountRole{UserAccountRole_User},
				}
				ua, err := AddAccount(tests.Context(), tt.claims(user.ID, accountID), test.MasterDB, createReq, now)
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tUpdateAccount failed.", tests.Failed)
				} else if tt.updateErr == nil {
					if diff := cmp.Diff(ua.Roles, createReq.Roles); diff != "" {
						t.Fatalf("\t%s\tExpected find result to match update. Diff:\n%s", tests.Failed, diff)
					}
					t.Logf("\t%s\tAddAccount ok.", tests.Success)
				}

				// Update the account.
				updateReq := UpdateAccountRequest{
					UserID: user.ID,
					AccountID: accountID,
					Roles:         &UserAccountRoles{UserAccountRole_Admin},
				}
				err = UpdateAccount(tests.Context(), tt.claims(user.ID, accountID), test.MasterDB, updateReq, now)
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tUpdateAccount failed.", tests.Failed)
				}
				t.Logf("\t%s\tUpdateAccount ok.", tests.Success)

				// Find the account for the user to verify the updates where made. There should only
				// be one account associated with the user for this test.
				findRes, err := FindAccountsByUserID(tests.Context(), tt.claims(user.ID, accountID), test.MasterDB, user.ID, false)
				if err != nil && errors.Cause(err) != tt.findErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.findErr)
					t.Fatalf("\t%s\tVerify UpdateAccount failed.", tests.Failed)
				} else if tt.findErr == nil {
					expected := []*UserAccount{
						&UserAccount{
							ID: ua.ID,
							UserID: ua.UserID,
							AccountID: ua.AccountID,
							Roles: *updateReq.Roles,
							Status: ua.Status,
							CreatedAt:ua.CreatedAt,
							UpdatedAt: now,
						},
					}
					if diff := cmp.Diff(findRes, expected); diff != "" {
						t.Fatalf("\t%s\tExpected find result to match update. Diff:\n%s", tests.Failed, diff)
					}
					t.Logf("\t%s\tVerify UpdateAccount ok.", tests.Success)
				}

				// Archive (soft-delete) the user account.
				err = RemoveAccount(tests.Context(), tt.claims(user.ID, accountID), test.MasterDB, RemoveAccountRequest{
					UserID: user.ID,
					AccountID: accountID,
				}, now)
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tRemoveAccount failed.", tests.Failed)
				} else if tt.updateErr == nil {
					// Trying to find the archived user with the includeArchived false should result in not found.
					_, err = FindAccountsByUserID(tests.Context(), tt.claims(user.ID, accountID), test.MasterDB, user.ID, false)
					if errors.Cause(err) != ErrNotFound {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", ErrNotFound)
						t.Fatalf("\t%s\tVerify RemoveAccount failed when excluding archived.", tests.Failed)
					}

					// Trying to find the archived user with the includeArchived true should result no error.
					findRes, err = FindAccountsByUserID(tests.Context(), tt.claims(user.ID, accountID), test.MasterDB, user.ID, true)
					if err != nil {
						t.Logf("\t\tGot : %+v", err)
						t.Fatalf("\t%s\tVerify RemoveAccount failed when including archived.", tests.Failed)
					}

					expected := []*UserAccount{
						&UserAccount{
							ID: ua.ID,
							UserID: ua.UserID,
							AccountID: ua.AccountID,
							Roles: *updateReq.Roles,
							Status: ua.Status,
							CreatedAt:ua.CreatedAt,
							UpdatedAt: now,
							ArchivedAt: pq.NullTime{Time: now, Valid:true},
						},
					}
					if diff := cmp.Diff(findRes, expected); diff != "" {
						t.Fatalf("\t%s\tExpected find result to be archived. Diff:\n%s", tests.Failed, diff)
					}
				}
				t.Logf("\t%s\tRemoveAccount ok.", tests.Success)

				// Delete (hard-delete) the user account.
				err = DeleteAccount(tests.Context(), tt.claims(user.ID, accountID), test.MasterDB, DeleteAccountRequest{
					UserID: user.ID,
					AccountID: accountID,
				})
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tDeleteAccount failed.", tests.Failed)
				} else if tt.updateErr == nil {
					// Trying to find the deleted user with the includeArchived true should result in not found.
					_, err = FindAccountsByUserID(tests.Context(), tt.claims(user.ID, accountID), test.MasterDB, user.ID, true)
					if errors.Cause(err) != ErrNotFound {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", ErrNotFound)
						t.Fatalf("\t%s\tVerify DeleteAccount failed when including archived.", tests.Failed)
					}
				}
				t.Logf("\t%s\tDeleteAccount ok.", tests.Success)
			}
		}
	}
}

// TestAccountFind validates all the request params are correctly parsed into a select query.
func TestAccountFind(t *testing.T) {

	// Ensure all the existing user accounts are deleted.
	{
		// Build the delete SQL statement.
		query := sqlbuilder.NewDeleteBuilder()
		query.DeleteFrom(usersAccountsTableName)

		// Execute the query with the provided context.
		sql, args := query.Build()
		sql = test.MasterDB.Rebind(sql)
		_, err := test.MasterDB.ExecContext(tests.Context(), sql, args...)
		if err != nil {
			t.Logf("\t\tGot : %+v", err)
			t.Fatalf("\t%s\tDelete failed.", tests.Failed)
		}
	}

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	var userAccounts []*UserAccount
	for i := 0; i <= 4; i++ {
		user, err := Create(tests.Context(), auth.Claims{}, test.MasterDB, CreateUserRequest{
			Name:            "Lee Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		}, now.Add(time.Second*time.Duration(i)))
		if err != nil {
			t.Logf("\t\tGot : %+v", err)
			t.Fatalf("\t%s\tCreate user failed.", tests.Failed)
		}

		// Create a new random account and associate that with the user.
		accountID := uuid.NewRandom().String()
		ua, err := AddAccount(tests.Context(), auth.Claims{}, test.MasterDB, AddAccountRequest{
			UserID: user.ID,
			AccountID: accountID,
			Roles:          []UserAccountRole{UserAccountRole_User},
		}, now.Add(time.Second*time.Duration(i)))
		if err != nil {
			t.Logf("\t\tGot : %+v", err)
			t.Fatalf("\t%s\tAdd account failed.", tests.Failed)
		}

		userAccounts = append(userAccounts, ua)
	}

	type accountTest struct {
		name     string
		req      UserAccountFindRequest
		expected []*UserAccount
		error    error
	}

	var accountTests []accountTest

	// Test sort users.
	accountTests = append(accountTests, accountTest{"Find all order by created_at asx",
		UserAccountFindRequest{
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
			Order: []string{"created_at desc"},
		},
		expected,
		nil,
	})

	// Test limit.
	var limit uint = 2
	accountTests = append(accountTests, accountTest{"Find limit",
		UserAccountFindRequest{
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
			Order:  []string{"created_at"},
			Limit:  &limit,
			Offset: &offset,
		},
		userAccounts[3:5],
		nil,
	})

	// Test where filter.
	whereParts := []string{}
	whereArgs := []interface{}{}
	expected = []*UserAccount{}
	selected := make(map[string]bool)
	for i := 0; i <= 2; i++ {
		ranIdx := rand.Intn(len(userAccounts))

		id := userAccounts[ranIdx].ID
		if selected[id] {
			continue
		}
		selected[id] = true

		whereParts = append(whereParts, "id = ?")
		whereArgs = append(whereArgs, id)
		expected = append(expected, userAccounts[ranIdx])
	}
	where := strings.Join(whereParts, " OR ")
	accountTests = append(accountTests, accountTest{"Find where",
		UserAccountFindRequest{
			Where: &where,
			Args:  whereArgs,
		},
		expected,
		nil,
	})

	t.Log("Given the need to ensure find users returns the expected results.")
	{
		for i, tt := range accountTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				res, err := FindAccounts(ctx, auth.Claims{}, test.MasterDB, tt.req)
				if err != nil && errors.Cause(err) != tt.error {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.error)
					t.Fatalf("\t%s\tFind failed.", tests.Failed)
				} else if diff := cmp.Diff(res, tt.expected); diff != "" {
					t.Logf("\t\tGot: %d items", len(res))
					t.Logf("\t\tWant: %d items", len(tt.expected))
					t.Fatalf("\t%s\tExpected find result to match expected. Diff:\n%s", tests.Failed, diff)
				}
				t.Logf("\t%s\tFind ok.", tests.Success)
			}
		}
	}
}
