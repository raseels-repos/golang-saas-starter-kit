package account

import (
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"github.com/dgrijalva/jwt-go"
	"github.com/google/go-cmp/cmp"
	"github.com/huandu/go-sqlbuilder"
	"github.com/lib/pq"
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

// TestFindRequestQuery validates findRequestQuery
func TestFindRequestQuery(t *testing.T) {
	where := "name = ? or address1 = ?"
	var (
		limit  uint = 12
		offset uint = 34
	)

	req := AccountFindRequest{
		Where: &where,
		Args: []interface{}{
			"lee brown",
			"103 East Main St.",
		},
		Order: []string{
			"id asc",
			"created_at desc",
		},
		Limit:  &limit,
		Offset: &offset,
	}
	expected := "SELECT " + accountMapColumns + " FROM " + accountTableName + " WHERE (name = ? or address1 = ?) ORDER BY id asc, created_at desc LIMIT 12 OFFSET 34"

	res, args := findRequestQuery(req)

	if diff := cmp.Diff(res.String(), expected); diff != "" {
		t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
	}
	if diff := cmp.Diff(args, req.Args); diff != "" {
		t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
	}
}

// TestApplyClaimsSelect validates applyClaimsSelect
func TestApplyClaimsSelect(t *testing.T) {
	var claimTests = []struct {
		name        string
		claims      auth.Claims
		expectedSql string
		error       error
	}{
		{"EmptyClaims",
			auth.Claims{},
			"SELECT " + accountMapColumns + " FROM " + accountTableName,
			nil,
		},
		{"RoleAccount",
			auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Subject:  "user1",
					Audience: "acc1",
				},
			},
			"SELECT " + accountMapColumns + " FROM " + accountTableName + " WHERE id IN (SELECT account_id FROM " + userAccountTableName + " WHERE (account_id = 'acc1' OR user_id = 'user1'))",
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
			"SELECT " + accountMapColumns + " FROM " + accountTableName + " WHERE id IN (SELECT account_id FROM " + userAccountTableName + " WHERE (account_id = 'acc1' OR user_id = 'user1'))",
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

// TestCreateValidation ensures all the validation tags work on Create
func TestCreateValidation(t *testing.T) {

	invalidStatus := AccountStatus("xxxxxx")

	var accountTests = []struct {
		name     string
		req      AccountCreateRequest
		expected func(req AccountCreateRequest, res *Account) *Account
		error    error
	}{
		{"Required Fields",
			AccountCreateRequest{},
			func(req AccountCreateRequest, res *Account) *Account {
				return nil
			},
			errors.New("Key: 'AccountCreateRequest.name' Error:Field validation for 'name' failed on the 'required' tag\n" +
				"Key: 'AccountCreateRequest.address1' Error:Field validation for 'address1' failed on the 'required' tag\n" +
				"Key: 'AccountCreateRequest.city' Error:Field validation for 'city' failed on the 'required' tag\n" +
				"Key: 'AccountCreateRequest.region' Error:Field validation for 'region' failed on the 'required' tag\n" +
				"Key: 'AccountCreateRequest.country' Error:Field validation for 'country' failed on the 'required' tag\n" +
				"Key: 'AccountCreateRequest.zipcode' Error:Field validation for 'zipcode' failed on the 'required' tag"),
		},

		{"Default Timezone & Status",
			AccountCreateRequest{
				Name:     uuid.NewRandom().String(),
				Address1: "103 East Main St",
				Address2: "Unit 546",
				City:     "Valdez",
				Region:   "AK",
				Country:  "USA",
				Zipcode:  "99686",
			},
			func(req AccountCreateRequest, res *Account) *Account {
				return &Account{
					Name:     req.Name,
					Address1: req.Address1,
					Address2: req.Address2,
					City:     req.City,
					Region:   req.Region,
					Country:  req.Country,
					Zipcode:  req.Zipcode,
					Timezone: "America/Anchorage",
					Status:   AccountStatus_Pending,

					// Copy this fields from the result.
					ID:        res.ID,
					CreatedAt: res.CreatedAt,
					UpdatedAt: res.UpdatedAt,
					//ArchivedAt: nil,
				}
			},
			nil,
		},
		{"Valid Status",
			AccountCreateRequest{
				Name:     uuid.NewRandom().String(),
				Address1: "103 East Main St",
				Address2: "Unit 546",
				City:     "Valdez",
				Region:   "AK",
				Country:  "USA",
				Zipcode:  "99686",
				Status:   &invalidStatus,
			},
			func(req AccountCreateRequest, res *Account) *Account {
				return nil
			},
			errors.New("Key: 'AccountCreateRequest.status' Error:Field validation for 'status' failed on the 'oneof' tag"),
		},
	}

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure all validation tags are working for account create.")
	{
		for i, tt := range accountTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				res, err := Create(ctx, auth.Claims{}, test.MasterDB, tt.req, now)
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
						t.Fatalf("\t%s\tCreate failed.", tests.Failed)
					}
				}

				// If there was an error that was expected, then don't go any further
				if tt.error != nil {
					t.Logf("\t%s\tCreate ok.", tests.Success)
					continue
				}

				expected := tt.expected(tt.req, res)
				if diff := cmp.Diff(res, expected); diff != "" {
					t.Fatalf("\t%s\tExpected result should match. Diff:\n%s", tests.Failed, diff)
				}

				t.Logf("\t%s\tCreate ok.", tests.Success)
			}
		}
	}
}

// TestCreateValidationNameUnique validates names must be unique on Create.
func TestCreateValidationNameUnique(t *testing.T) {

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure duplicate names are not allowed for account create.")
	{
		ctx := tests.Context()

		req1 := AccountCreateRequest{
			Name:     uuid.NewRandom().String(),
			Address1: "103 East Main St",
			Address2: "Unit 546",
			City:     "Valdez",
			Region:   "AK",
			Country:  "USA",
			Zipcode:  "99686",
		}
		account1, err := Create(ctx, auth.Claims{}, test.MasterDB, req1, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		req2 := AccountCreateRequest{
			Name:     account1.Name,
			Address1: "103 East Main St",
			Address2: "Unit 546",
			City:     "Valdez",
			Region:   "AK",
			Country:  "USA",
			Zipcode:  "99686",
		}
		expectedErr := errors.New("Key: 'AccountCreateRequest.name' Error:Field validation for 'name' failed on the 'unique' tag")
		_, err = Create(ctx, auth.Claims{}, test.MasterDB, req2, now)
		if err == nil {
			t.Logf("\t\tWant: %+v", expectedErr)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		if err.Error() != expectedErr.Error() {
			t.Logf("\t\tGot : %+v", err)
			t.Logf("\t\tWant: %+v", expectedErr)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		t.Logf("\t%s\tCreate ok.", tests.Success)
	}
}

// TestCreateClaims validates ACLs are correctly applied to Create by claims.
func TestCreateClaims(t *testing.T) {
	defer tests.Recover(t)

	var accountTests = []struct {
		name   string
		claims auth.Claims
		req    AccountCreateRequest
		error  error
	}{
		// Internal request, should bypass ACL.
		{"EmptyClaims",
			auth.Claims{},
			AccountCreateRequest{
				Name:     uuid.NewRandom().String(),
				Address1: "103 East Main St",
				Address2: "Unit 546",
				City:     "Valdez",
				Region:   "AK",
				Country:  "USA",
				Zipcode:  "99686",
			},
			nil,
		},
		// Role of account, only admins can create new accounts.
		{"RoleAccount",
			auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Subject:  "account1",
					Audience: "acc1",
				},
			},
			AccountCreateRequest{
				Name:     uuid.NewRandom().String(),
				Address1: "103 East Main St",
				Address2: "Unit 546",
				City:     "Valdez",
				Region:   "AK",
				Country:  "USA",
				Zipcode:  "99686",
			},
			nil,
		},
		// Role of admin, can create accounts.
		{"RoleAdmin",
			auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Subject:  "account1",
					Audience: "acc1",
				},
			},
			AccountCreateRequest{
				Name:     uuid.NewRandom().String(),
				Address1: "103 East Main St",
				Address2: "Unit 546",
				City:     "Valdez",
				Region:   "AK",
				Country:  "USA",
				Zipcode:  "99686",
			},
			nil,
		},
	}

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need to ensure claims are applied as ACL for create account.")
	{
		for i, tt := range accountTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				_, err := Create(ctx, auth.Claims{}, test.MasterDB, tt.req, now)
				if errors.Cause(err) != tt.error {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.error)
					t.Fatalf("\t%s\tCreate failed.", tests.Failed)
				}

				t.Logf("\t%s\tCreate ok.", tests.Success)
			}
		}
	}
}

// TestUpdateValidation ensures all the validation tags work on Update
func TestUpdateValidation(t *testing.T) {
	// TODO: actually create the account so can test the output of findbyId
	type accountTest struct {
		name  string
		req   AccountUpdateRequest
		error error
	}

	var accountTests = []accountTest{
		{"Required Fields",
			AccountUpdateRequest{},
			errors.New("Key: 'AccountUpdateRequest.id' Error:Field validation for 'id' failed on the 'required' tag"),
		},
	}

	invalidStatus := AccountStatus("xxxxxx")
	accountTests = append(accountTests, accountTest{"Valid Status",
		AccountUpdateRequest{
			ID:     uuid.NewRandom().String(),
			Status: &invalidStatus,
		},
		errors.New("Key: 'AccountUpdateRequest.status' Error:Field validation for 'status' failed on the 'oneof' tag"),
	})

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure all validation tags are working for account update.")
	{
		for i, tt := range accountTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				err := Update(ctx, auth.Claims{}, test.MasterDB, tt.req, now)
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
						t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
					}
				}

				t.Logf("\t%s\tUpdate ok.", tests.Success)
			}
		}
	}
}

// TestUpdateValidationNameUnique validates names must be unique on Update.
func TestUpdateValidationNameUnique(t *testing.T) {

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure duplicate names are not allowed for account update.")
	{
		ctx := tests.Context()

		req1 := AccountCreateRequest{
			Name:     uuid.NewRandom().String(),
			Address1: "103 East Main St",
			Address2: "Unit 546",
			City:     "Valdez",
			Region:   "AK",
			Country:  "USA",
			Zipcode:  "99686",
		}
		account1, err := Create(ctx, auth.Claims{}, test.MasterDB, req1, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		req2 := AccountCreateRequest{
			Name:     uuid.NewRandom().String(),
			Address1: "103 East Main St",
			Address2: "Unit 546",
			City:     "Valdez",
			Region:   "AK",
			Country:  "USA",
			Zipcode:  "99686",
		}
		account2, err := Create(ctx, auth.Claims{}, test.MasterDB, req2, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		// Try to set the email for account 1 on account 2
		updateReq := AccountUpdateRequest{
			ID:   account2.ID,
			Name: &account1.Name,
		}
		expectedErr := errors.New("Key: 'AccountUpdateRequest.name' Error:Field validation for 'name' failed on the 'unique' tag")
		err = Update(ctx, auth.Claims{}, test.MasterDB, updateReq, now)
		if err == nil {
			t.Logf("\t\tWant: %+v", expectedErr)
			t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
		}

		if err.Error() != expectedErr.Error() {
			t.Logf("\t\tGot : %+v", err)
			t.Logf("\t\tWant: %+v", expectedErr)
			t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
		}

		t.Logf("\t%s\tUpdate ok.", tests.Success)
	}
}

// TestCrud validates the full set of CRUD operations for accounts and ensures ACLs are correctly applied by claims.
func TestCrud(t *testing.T) {
	defer tests.Recover(t)

	type accountTest struct {
		name      string
		claims    func(*Account, string) auth.Claims
		create    AccountCreateRequest
		update    func(*Account) AccountUpdateRequest
		updateErr error
		expected  func(*Account, AccountUpdateRequest) *Account
		findErr   error
	}

	var accountTests []accountTest

	// Internal request, should bypass ACL.
	accountTests = append(accountTests, accountTest{"EmptyClaims",
		func(account *Account, userId string) auth.Claims {
			return auth.Claims{}
		},
		AccountCreateRequest{
			Name:     uuid.NewRandom().String(),
			Address1: "103 East Main St",
			Address2: "Unit 546",
			City:     "Valdez",
			Region:   "AK",
			Country:  "USA",
			Zipcode:  "99686",
		},
		func(account *Account) AccountUpdateRequest {
			name := uuid.NewRandom().String()
			return AccountUpdateRequest{
				ID:   account.ID,
				Name: &name,
			}
		},
		nil,
		func(account *Account, req AccountUpdateRequest) *Account {
			return &Account{
				Name: *req.Name,
				// Copy this fields from the created account.
				ID:            account.ID,
				Address1:      account.Address1,
				Address2:      account.Address2,
				City:          account.City,
				Region:        account.Region,
				Country:       account.Country,
				Zipcode:       account.Zipcode,
				Status:        account.Status,
				Timezone:      account.Timezone,
				SignupUserID:  account.SignupUserID,
				BillingUserID: account.BillingUserID,
				CreatedAt:     account.CreatedAt,
				UpdatedAt:     account.UpdatedAt,
				//ArchivedAt: nil,
			}
		},
		nil,
	})

	// Role of account but claim account does not match update account so forbidden.
	accountTests = append(accountTests, accountTest{"RoleAccountDiffAccount",
		func(account *Account, userId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Audience: uuid.NewRandom().String(),
					Subject:  userId,
				},
			}
		},
		AccountCreateRequest{
			Name:     uuid.NewRandom().String(),
			Address1: "103 East Main St",
			Address2: "Unit 546",
			City:     "Valdez",
			Region:   "AK",
			Country:  "USA",
			Zipcode:  "99686",
		},
		func(account *Account) AccountUpdateRequest {
			name := uuid.NewRandom().String()
			return AccountUpdateRequest{
				ID:   account.ID,
				Name: &name,
			}
		},
		ErrForbidden,
		func(account *Account, req AccountUpdateRequest) *Account {
			return account
		},
		ErrNotFound,
	})

	// Role of account AND claim account matches update account so OK.
	accountTests = append(accountTests, accountTest{"RoleAccountSameAccount",
		func(account *Account, userId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Audience: account.ID,
					Subject:  userId,
				},
			}
		},
		AccountCreateRequest{
			Name:     uuid.NewRandom().String(),
			Address1: "103 East Main St",
			Address2: "Unit 546",
			City:     "Valdez",
			Region:   "AK",
			Country:  "USA",
			Zipcode:  "99686",
		},
		func(account *Account) AccountUpdateRequest {
			name := uuid.NewRandom().String()
			return AccountUpdateRequest{
				ID:   account.ID,
				Name: &name,
			}
		},
		nil,
		func(account *Account, req AccountUpdateRequest) *Account {
			return &Account{
				Name: *req.Name,
				// Copy this fields from the created account.
				ID:            account.ID,
				Address1:      account.Address1,
				Address2:      account.Address2,
				City:          account.City,
				Region:        account.Region,
				Country:       account.Country,
				Zipcode:       account.Zipcode,
				Status:        account.Status,
				Timezone:      account.Timezone,
				SignupUserID:  account.SignupUserID,
				BillingUserID: account.BillingUserID,
				CreatedAt:     account.CreatedAt,
				UpdatedAt:     account.UpdatedAt,
				//ArchivedAt: nil,
			}
		},
		nil,
	})

	// Role of admin but claim account does not match update account so forbidden.
	accountTests = append(accountTests, accountTest{"RoleAdminDiffAccount",
		func(account *Account, accountId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Audience: uuid.NewRandom().String(),
					Subject:  uuid.NewRandom().String(),
				},
			}
		},
		AccountCreateRequest{
			Name:     uuid.NewRandom().String(),
			Address1: "103 East Main St",
			Address2: "Unit 546",
			City:     "Valdez",
			Region:   "AK",
			Country:  "USA",
			Zipcode:  "99686",
		},
		func(account *Account) AccountUpdateRequest {
			name := uuid.NewRandom().String()
			return AccountUpdateRequest{
				ID:   account.ID,
				Name: &name,
			}
		},
		ErrForbidden,
		func(account *Account, req AccountUpdateRequest) *Account {
			return nil
		},
		ErrNotFound,
	})

	// Role of admin and claim account matches update account so ok.
	accountTests = append(accountTests, accountTest{"RoleAdminSameAccount",
		func(account *Account, userId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Audience: uuid.NewRandom().String(),
					Subject:  userId,
				},
			}
		},
		AccountCreateRequest{
			Name:     uuid.NewRandom().String(),
			Address1: "103 East Main St",
			Address2: "Unit 546",
			City:     "Valdez",
			Region:   "AK",
			Country:  "USA",
			Zipcode:  "99686",
		},
		func(account *Account) AccountUpdateRequest {
			name := uuid.NewRandom().String()
			return AccountUpdateRequest{
				ID:   account.ID,
				Name: &name,
			}
		},
		nil,
		func(account *Account, req AccountUpdateRequest) *Account {
			return &Account{
				Name: *req.Name,
				// Copy this fields from the created account.
				ID:            account.ID,
				Address1:      account.Address1,
				Address2:      account.Address2,
				City:          account.City,
				Region:        account.Region,
				Country:       account.Country,
				Zipcode:       account.Zipcode,
				Status:        account.Status,
				Timezone:      account.Timezone,
				SignupUserID:  account.SignupUserID,
				BillingUserID: account.BillingUserID,
				CreatedAt:     account.CreatedAt,
				UpdatedAt:     account.UpdatedAt,
				//ArchivedAt: nil,
			}
		},
		nil,
	})

	t.Log("Given the need to ensure claims are applied as ACL for update account.")
	{
		now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

		for i, tt := range accountTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				// Always create the new account with empty claims, testing claims for create account
				// will be handled separately.
				account, err := Create(ctx, auth.Claims{}, test.MasterDB, tt.create, now)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tCreate failed.", tests.Failed)
				}

				// Create a new random account and associate that with the account.
				userId := uuid.NewRandom().String()
				err = mockUserAccount(account.ID, userId, account.CreatedAt, auth.RoleAdmin)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tAdd user account failed.", tests.Failed)
				}

				// Update the account.
				updateReq := tt.update(account)
				err = Update(ctx, tt.claims(account, userId), test.MasterDB, updateReq, now)
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
				}
				t.Logf("\t%s\tUpdate ok.", tests.Success)

				// Find the account and make sure the updates where made.
				findRes, err := Read(ctx, tt.claims(account, userId), test.MasterDB, account.ID, false)
				if err != nil && errors.Cause(err) != tt.findErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.findErr)
					t.Fatalf("\t%s\tRead failed.", tests.Failed)
				} else {
					findExpected := tt.expected(findRes, updateReq)
					if diff := cmp.Diff(findRes, findExpected); diff != "" {
						t.Fatalf("\t%s\tExpected find result to match update. Diff:\n%s", tests.Failed, diff)
					}
					t.Logf("\t%s\tRead ok.", tests.Success)
				}

				// Archive (soft-delete) the account.
				err = ArchiveById(ctx, tt.claims(account, userId), test.MasterDB, account.ID, now)
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tArchive failed.", tests.Failed)
				} else if tt.updateErr == nil {
					// Trying to find the archived account with the includeArchived false should result in not found.
					_, err = Read(ctx, tt.claims(account, userId), test.MasterDB, account.ID, false)
					if err != nil && errors.Cause(err) != ErrNotFound {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", ErrNotFound)
						t.Fatalf("\t%s\tArchive Read failed.", tests.Failed)
					}

					// Trying to find the archived account with the includeArchived true should result no error.
					_, err = Read(ctx, tt.claims(account, userId), test.MasterDB, account.ID, true)
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tArchive Read failed.", tests.Failed)
					}
				}
				t.Logf("\t%s\tArchive ok.", tests.Success)

				// Delete (hard-delete) the account.
				err = Delete(ctx, tt.claims(account, userId), test.MasterDB, account.ID)
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
				} else if tt.updateErr == nil {
					// Trying to find the deleted account with the includeArchived true should result in not found.
					_, err = Read(ctx, tt.claims(account, userId), test.MasterDB, account.ID, true)
					if errors.Cause(err) != ErrNotFound {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", ErrNotFound)
						t.Fatalf("\t%s\tDelete Read failed.", tests.Failed)
					}
				}
				t.Logf("\t%s\tDelete ok.", tests.Success)
			}
		}
	}
}

// TestFind validates all the request params are correctly parsed into a select query.
func TestFind(t *testing.T) {

	now := time.Now().Add(time.Hour * -1).UTC()

	startTime := now.Truncate(time.Millisecond)
	var endTime time.Time

	var accounts []*Account
	for i := 0; i <= 4; i++ {
		account, err := Create(tests.Context(), auth.Claims{}, test.MasterDB, AccountCreateRequest{
			Name:     uuid.NewRandom().String(),
			Address1: "103 East Main St",
			Address2: "Unit 546",
			City:     "Valdez",
			Region:   "AK",
			Country:  "USA",
			Zipcode:  "99686",
		}, now.Add(time.Second*time.Duration(i)))
		if err != nil {
			t.Logf("\t\tGot : %+v", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}
		accounts = append(accounts, account)
		endTime = account.CreatedAt
	}

	type accountTest struct {
		name     string
		req      AccountFindRequest
		expected []*Account
		error    error
	}

	var accountTests []accountTest

	createdFilter := "created_at BETWEEN ? AND ?"

	// Test sort accounts.
	accountTests = append(accountTests, accountTest{"Find all order by created_at asc",
		AccountFindRequest{
			Where: &createdFilter,
			Args:  []interface{}{startTime, endTime},
			Order: []string{"created_at"},
		},
		accounts,
		nil,
	})

	// Test reverse sorted accounts.
	var expected []*Account
	for i := len(accounts) - 1; i >= 0; i-- {
		expected = append(expected, accounts[i])
	}
	accountTests = append(accountTests, accountTest{"Find all order by created_at desc",
		AccountFindRequest{
			Where: &createdFilter,
			Args:  []interface{}{startTime, endTime},
			Order: []string{"created_at desc"},
		},
		expected,
		nil,
	})

	// Test limit.
	var limit uint = 2
	accountTests = append(accountTests, accountTest{"Find limit",
		AccountFindRequest{
			Where: &createdFilter,
			Args:  []interface{}{startTime, endTime},
			Order: []string{"created_at"},
			Limit: &limit,
		},
		accounts[0:2],
		nil,
	})

	// Test offset.
	var offset uint = 3
	accountTests = append(accountTests, accountTest{"Find limit, offset",
		AccountFindRequest{
			Where:  &createdFilter,
			Args:   []interface{}{startTime, endTime},
			Order:  []string{"created_at"},
			Limit:  &limit,
			Offset: &offset,
		},
		accounts[3:5],
		nil,
	})

	// Test where filter.
	whereParts := []string{}
	whereArgs := []interface{}{startTime, endTime}
	expected = []*Account{}
	for i := 0; i <= len(accounts); i++ {
		if rand.Intn(100) < 50 {
			continue
		}
		u := *accounts[i]

		whereParts = append(whereParts, "name = ?")
		whereArgs = append(whereArgs, u.Name)
		expected = append(expected, &u)
	}

	where := createdFilter + " AND (" + strings.Join(whereParts, " OR ") + ")"
	accountTests = append(accountTests, accountTest{"Find where",
		AccountFindRequest{
			Where: &where,
			Args:  whereArgs,
			Order: []string{"created_at"},
		},
		expected,
		nil,
	})

	t.Log("Given the need to ensure find accounts returns the expected results.")
	{
		for i, tt := range accountTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				res, err := Find(ctx, auth.Claims{}, test.MasterDB, tt.req)
				if errors.Cause(err) != tt.error {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.error)
					t.Fatalf("\t%s\tFind failed.", tests.Failed)
				} else if diff := cmp.Diff(res, tt.expected); diff != "" {
					t.Logf("\t\tGot: %d items", len(res))
					t.Logf("\t\tWant: %d items", len(tt.expected))

					for _, u := range res {
						t.Logf("\t\tGot: %s ID", u.ID)
					}
					for _, u := range tt.expected {
						t.Logf("\t\tExpected: %s ID", u.ID)
					}

					t.Fatalf("\t%s\tExpected find result to match expected. Diff:\n%s", tests.Failed, diff)
				}
				t.Logf("\t%s\tFind ok.", tests.Success)
			}
		}
	}
}

func mockUserAccount(accountId, userId string, now time.Time, roles ...string) error {
	var roleArr pq.StringArray
	for _, r := range roles {
		roleArr = append(roleArr, r)
	}

	err := mockUser(userId, now)
	if err != nil {
		return err
	}

	// Build the insert SQL statement.
	query := sqlbuilder.NewInsertBuilder()
	query.InsertInto(userAccountTableName)
	query.Cols("id", "user_id", "account_id", "roles", "created_at", "updated_at")
	query.Values(uuid.NewRandom().String(), userId, accountId, roleArr, now, now)

	// Execute the query with the provided context.
	sql, args := query.Build()
	sql = test.MasterDB.Rebind(sql)
	_, err = test.MasterDB.ExecContext(tests.Context(), sql, args...)
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
