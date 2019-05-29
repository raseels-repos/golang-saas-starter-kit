package user

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

// TestUserFindRequestQuery validates userFindRequestQuery
func TestUserFindRequestQuery(t *testing.T) {
	where := "name = ? or email = ?"
	var (
		limit  uint = 12
		offset uint = 34
	)

	req := UserFindRequest{
		Where: &where,
		Args: []interface{}{
			"lee brown",
			"lee@geeksinthewoods.com",
		},
		Order: []string{
			"id asc",
			"created_at desc",
		},
		Limit:  &limit,
		Offset: &offset,
	}
	expected := "SELECT " + usersMapColumns + " FROM " + usersTableName + " WHERE (name = ? or email = ?) ORDER BY id asc, created_at desc LIMIT 12 OFFSET 34"

	res, args := userFindRequestQuery(req)

	if diff := cmp.Diff(res.String(), expected); diff != "" {
		t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
	}
	if diff := cmp.Diff(args, req.Args); diff != "" {
		t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
	}
}

// TestApplyClaimsUserSelect validates applyClaimsUserSelect
func TestApplyClaimsUserSelect(t *testing.T) {
	var claimTests = []struct {
		name        string
		claims      auth.Claims
		expectedSql string
		error       error
	}{
		{"EmptyClaims",
			auth.Claims{},
			"SELECT " + usersMapColumns + " FROM " + usersTableName,
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
			"SELECT " + usersMapColumns + " FROM " + usersTableName + " WHERE id IN (SELECT user_id FROM " + usersAccountsTableName + " WHERE (account_id = 'acc1' OR user_id = 'user1'))",
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
			"SELECT " + usersMapColumns + " FROM " + usersTableName + " WHERE id IN (SELECT user_id FROM " + usersAccountsTableName + " WHERE (account_id = 'acc1' OR user_id = 'user1'))",
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

				err := applyClaimsUserSelect(ctx, tt.claims, query)
				if err != tt.error {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.error)
					t.Fatalf("\t%s\tapplyClaimsUserSelect failed.", tests.Failed)
				}

				sql, args := query.Build()

				// Use mysql flavor so placeholders will get replaced for comparison.
				sql, err = sqlbuilder.MySQL.Interpolate(sql, args)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tapplyClaimsUserSelect failed.", tests.Failed)
				}

				if diff := cmp.Diff(sql, tt.expectedSql); diff != "" {
					t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
				}

				t.Logf("\t%s\tapplyClaimsUserSelect ok.", tests.Success)
			}
		}
	}
}

// TestCreateUser ensures all the validation tags work on Create
func TestCreateUserValidation(t *testing.T) {


	var userTests = []struct {
		name     string
		req      CreateUserRequest
		expected func(req CreateUserRequest, res *User) *User
		error    error
	}{
		{"Required Fields",
			CreateUserRequest{},
			func(req CreateUserRequest, res *User) *User {
				return nil
			},
			errors.New("Key: 'CreateUserRequest.Name' Error:Field validation for 'Name' failed on the 'required' tag\n" +
				"Key: 'CreateUserRequest.Email' Error:Field validation for 'Email' failed on the 'required' tag\n" +
				"Key: 'CreateUserRequest.Password' Error:Field validation for 'Password' failed on the 'required' tag"),
		},
		{"Valid Email",
			CreateUserRequest{
				Name:            "Lee Brown",
				Email:           "xxxxxxxxxx",
				Password:        "akTechFr0n!ier",
				PasswordConfirm: "akTechFr0n!ier",
			},
			func(req CreateUserRequest, res *User) *User {
				return nil
			},
			errors.New("Key: 'CreateUserRequest.Email' Error:Field validation for 'Email' failed on the 'email' tag"),
		},
		{"Passwords Match",
			CreateUserRequest{
				Name:            "Lee Brown",
				Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
				Password:        "akTechFr0n!ier",
				PasswordConfirm: "W0rkL1fe#",
			},
			func(req CreateUserRequest, res *User) *User {
				return nil
			},
			errors.New("Key: 'CreateUserRequest.PasswordConfirm' Error:Field validation for 'PasswordConfirm' failed on the 'eqfield' tag"),
		},
		{"Default Timezone",
			CreateUserRequest{
				Name:            "Lee Brown",
				Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
				Password:        "akTechFr0n!ier",
				PasswordConfirm: "akTechFr0n!ier",
			},
			func(req CreateUserRequest, res *User) *User {
				return &User{
					Name:     req.Name,
					Email:    req.Email,
					Timezone: "America/Anchorage",

					// Copy this fields from the result.
					ID:            res.ID,
					PasswordSalt:  res.PasswordSalt,
					PasswordHash:  res.PasswordHash,
					PasswordReset: res.PasswordReset,
					CreatedAt:     res.CreatedAt,
					UpdatedAt:     res.UpdatedAt,
					//ArchivedAt: nil,
				}
			},
			nil,
		},
	}

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure all validation tags are working for user create.")
	{
		for i, tt := range userTests {
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

// TestCreateUserValidationEmailUnique validates emails must be unique on Create.
func TestCreateUserValidationEmailUnique(t *testing.T) {

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure duplicate emails are not allowed for user create.")
	{
		ctx := tests.Context()

		req1 := CreateUserRequest{
			Name:            "Lee Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		}
		user1, err := Create(ctx, auth.Claims{}, test.MasterDB, req1, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		req2 := CreateUserRequest{
			Name:            "Lucas Brown",
			Email:           user1.Email,
			Password:        "W0rkL1fe#",
			PasswordConfirm: "W0rkL1fe#",
		}
		expectedErr := errors.New("Key: 'CreateUserRequest.Email' Error:Field validation for 'Email' failed on the 'unique' tag")
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

// TestCreateUserClaims validates ACLs are correctly applied to Create by claims.
func TestCreateUserClaims(t *testing.T) {
	defer tests.Recover(t)

	var userTests = []struct {
		name   string
		claims auth.Claims
		req    CreateUserRequest
		error  error
	}{
		// Internal request, should bypass ACL.
		{"EmptyClaims",
			auth.Claims{},
			CreateUserRequest{
				Name:            "Lee Brown",
				Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
				Password:        "akTechFr0n!ier",
				PasswordConfirm: "akTechFr0n!ier",
			},
			nil,
		},
		// Role of user, only admins can create new users.
		{"RoleUser",
			auth.Claims{
				Roles: []string{auth.RoleUser},
				StandardClaims: jwt.StandardClaims{
					Subject:  "user1",
					Audience: "acc1",
				},
			},
			CreateUserRequest{
				Name:            "Lee Brown",
				Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
				Password:        "akTechFr0n!ier",
				PasswordConfirm: "akTechFr0n!ier",
			},
			ErrForbidden,
		},
		// Role of admin, can create users.
		{"RoleAdmin",
			auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Subject:  "user1",
					Audience: "acc1",
				},
			},
			CreateUserRequest{
				Name:            "Lee Brown",
				Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
				Password:        "akTechFr0n!ier",
				PasswordConfirm: "akTechFr0n!ier",
			},
			nil,
		},
	}

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need to ensure claims are applied as ACL for create user.")
	{
		for i, tt := range userTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				_, err := Create(ctx, auth.Claims{}, test.MasterDB, tt.req, now)
				if err != nil && errors.Cause(err) != tt.error {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.error)
					t.Fatalf("\t%s\tCreate failed.", tests.Failed)
				}

				t.Logf("\t%s\tCreate ok.", tests.Success)
			}
		}
	}
}

// TestUpdateUser ensures all the validation tags work on Update
func TestUpdateUserValidation(t *testing.T) {
	// TODO: actually create the user so can test the output of findbyId
	type userTest struct {
		name  string
		req   UpdateUserRequest
		error error
	}

	var userTests = []userTest{
		{"Required Fields",
			UpdateUserRequest{},
			errors.New("Key: 'UpdateUserRequest.ID' Error:Field validation for 'ID' failed on the 'required' tag"),
		},
	}

	invalidEmail := "xxxxxxxxxx"
	userTests = append(userTests, userTest{"Valid Email",
		UpdateUserRequest{
			ID:    uuid.NewRandom().String(),
			Email: &invalidEmail,
		},
		errors.New("Key: 'UpdateUserRequest.Email' Error:Field validation for 'Email' failed on the 'email' tag"),
	})

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure all validation tags are working for user update.")
	{
		for i, tt := range userTests {
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

// TestUpdateUserValidationEmailUnique validates emails must be unique on Update.
func TestUpdateUserValidationEmailUnique(t *testing.T) {

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure duplicate emails are not allowed for user update.")
	{
		ctx := tests.Context()

		req1 := CreateUserRequest{
			Name:            "Lee Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		}
		user1, err := Create(ctx, auth.Claims{}, test.MasterDB, req1, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		req2 := CreateUserRequest{
			Name:            "Lucas Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "W0rkL1fe#",
			PasswordConfirm: "W0rkL1fe#",
		}
		user2, err := Create(ctx, auth.Claims{}, test.MasterDB, req2, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		// Try to set the email for user 1 on user 2
		updateReq := UpdateUserRequest{
			ID:    user2.ID,
			Email: &user1.Email,
		}
		expectedErr := errors.New("Key: 'UpdateUserRequest.Email' Error:Field validation for 'Email' failed on the 'unique' tag")
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

// TestUpdateUserPassword validates update user password works.
func TestUpdateUserPassword(t *testing.T) {

	t.Log("Given the need ensure a user password can be updated.")
	{
		ctx := tests.Context()

		now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

		var tknGen mockTokenGenerator

		// Create a new user for testing.
		initPass := uuid.NewRandom().String()
		user, err := Create(ctx, auth.Claims{}, test.MasterDB, CreateUserRequest{
			Name:            "Lee Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        initPass,
			PasswordConfirm: initPass,
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		// Create a new random account and associate that with the user.
		accountId := uuid.NewRandom().String()
		_, err = AddAccount(tests.Context(), auth.Claims{}, test.MasterDB, AddAccountRequest{
			UserID:    user.ID,
			AccountID: accountId,
			Roles:     []UserAccountRole{UserAccountRole_User},
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tAddAccount failed.", tests.Failed)
		}

		// Verify that the user can be authenticated with the created user.
		_, err = Authenticate(ctx, test.MasterDB, tknGen, now, user.Email, initPass)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tAuthenticate failed.", tests.Failed)
		}

		// Ensure validation is working by trying UpdatePassword with an empty request.
		expectedErr := errors.New("Key: 'UpdatePasswordRequest.ID' Error:Field validation for 'ID' failed on the 'required' tag\n" +
			"Key: 'UpdatePasswordRequest.Password' Error:Field validation for 'Password' failed on the 'required' tag")
		err = UpdatePassword(ctx, auth.Claims{}, test.MasterDB, UpdatePasswordRequest{}, now)
		if err == nil {
			t.Logf("\t\tWant: %+v", expectedErr)
			t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
		} else if err.Error() != expectedErr.Error() {
			t.Logf("\t\tGot : %+v", err)
			t.Logf("\t\tWant: %+v", expectedErr)
			t.Fatalf("\t%s\tValidation failed.", tests.Failed)
		}
		t.Logf("\t%s\tValidation ok.", tests.Success)

		// Update the users password.
		newPass := uuid.NewRandom().String()
		err = UpdatePassword(ctx, auth.Claims{}, test.MasterDB, UpdatePasswordRequest{
			ID:              user.ID,
			Password:        newPass,
			PasswordConfirm: newPass,
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}
		t.Logf("\t%s\tUpdatePassword ok.", tests.Success)

		// Verify that the user can be authenticated with the updated password.
		_, err = Authenticate(ctx, test.MasterDB, tknGen, now, user.Email, newPass)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tAuthenticate failed.", tests.Failed)
		}
		t.Logf("\t%s\tAuthenticate ok.", tests.Success)
	}
}

// TestUserCrud validates the full set of CRUD operations for users and ensures ACLs are correctly applied by claims.
func TestUserCrud(t *testing.T) {
	defer tests.Recover(t)

	type userTest struct {
		name      string
		claims    func(*User, string) auth.Claims
		create    CreateUserRequest
		update    func(*User) UpdateUserRequest
		updateErr error
		expected  func(*User, UpdateUserRequest) *User
		findErr   error
	}

	var userTests []userTest

	// Internal request, should bypass ACL.
	userTests = append(userTests, userTest{"EmptyClaims",
		func(user *User, accountId string) auth.Claims {
			return auth.Claims{}
		},
		CreateUserRequest{
			Name:            "Lee Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
		func(user *User) UpdateUserRequest {
			email := uuid.NewRandom().String() + "@geeksinthewoods.com"
			return UpdateUserRequest{
				ID:    user.ID,
				Email: &email,
			}
		},
		nil,
		func(user *User, req UpdateUserRequest) *User {
			return &User{
				Email: *req.Email,
				// Copy this fields from the created user.
				ID:            user.ID,
				Name:          user.Name,
				PasswordSalt:  user.PasswordSalt,
				PasswordHash:  user.PasswordHash,
				PasswordReset: user.PasswordReset,
				Timezone:      user.Timezone,
				CreatedAt:     user.CreatedAt,
				UpdatedAt:     user.UpdatedAt,
				//ArchivedAt: nil,
			}
		},
		nil,
	})

	// Role of user but claim user does not match update user so forbidden.
	userTests = append(userTests, userTest{"RoleUserDiffUser",
		func(user *User, accountId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleUser},
				StandardClaims: jwt.StandardClaims{
					Subject:  uuid.NewRandom().String(),
					Audience: accountId,
				},
			}
		},
		CreateUserRequest{
			Name:            "Lee Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
		func(user *User) UpdateUserRequest {
			email := uuid.NewRandom().String() + "@geeksinthewoods.com"
			return UpdateUserRequest{
				ID:    user.ID,
				Email: &email,
			}
		},
		ErrForbidden,
		func(user *User, req UpdateUserRequest) *User {
			return user
		},
		ErrNotFound,
	})

	// Role of user AND claim user matches update user so OK.
	userTests = append(userTests, userTest{"RoleUserSameUser",
		func(user *User, accountId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleUser},
				StandardClaims: jwt.StandardClaims{
					Subject:  user.ID,
					Audience: accountId,
				},
			}
		},
		CreateUserRequest{
			Name:            "Lee Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
		func(user *User) UpdateUserRequest {
			email := uuid.NewRandom().String() + "@geeksinthewoods.com"
			return UpdateUserRequest{
				ID:    user.ID,
				Email: &email,
			}
		},
		nil,
		func(user *User, req UpdateUserRequest) *User {
			return &User{
				Email: *req.Email,
				// Copy this fields from the created user.
				ID:            user.ID,
				Name:          user.Name,
				PasswordSalt:  user.PasswordSalt,
				PasswordHash:  user.PasswordHash,
				PasswordReset: user.PasswordReset,
				Timezone:      user.Timezone,
				CreatedAt:     user.CreatedAt,
				UpdatedAt:     user.UpdatedAt,
				//ArchivedAt: nil,
			}
		},
		nil,
	})

	// Role of admin but claim account does not match update user so forbidden.
	userTests = append(userTests, userTest{"RoleAdminDiffUser",
		func(user *User, accountId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Subject:  uuid.NewRandom().String(),
					Audience: uuid.NewRandom().String(),
				},
			}
		},
		CreateUserRequest{
			Name:            "Lee Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
		func(user *User) UpdateUserRequest {
			email := uuid.NewRandom().String() + "@geeksinthewoods.com"
			return UpdateUserRequest{
				ID:    user.ID,
				Email: &email,
			}
		},
		ErrForbidden,
		func(user *User, req UpdateUserRequest) *User {
			return nil
		},
		ErrNotFound,
	})

	// Role of admin and claim account matches update user so ok.
	userTests = append(userTests, userTest{"RoleAdminSameAccount",
		func(user *User, accountId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Subject:  uuid.NewRandom().String(),
					Audience: accountId,
				},
			}
		},
		CreateUserRequest{
			Name:            "Lee Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
		func(user *User) UpdateUserRequest {
			email := uuid.NewRandom().String() + "@geeksinthewoods.com"
			return UpdateUserRequest{
				ID:    user.ID,
				Email: &email,
			}
		},
		nil,
		func(user *User, req UpdateUserRequest) *User {
			return &User{
				Email: *req.Email,
				// Copy this fields from the created user.
				ID:            user.ID,
				Name:          user.Name,
				PasswordSalt:  user.PasswordSalt,
				PasswordHash:  user.PasswordHash,
				PasswordReset: user.PasswordReset,
				Timezone:      user.Timezone,
				CreatedAt:     user.CreatedAt,
				UpdatedAt:     user.UpdatedAt,
				//ArchivedAt: nil,
			}
		},
		nil,
	})

	t.Log("Given the need to ensure claims are applied as ACL for update user.")
	{
		now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

		for i, tt := range userTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				// Always create the new user with empty claims, testing claims for create user
				// will be handled separately.
				user, err := Create(tests.Context(), auth.Claims{}, test.MasterDB, tt.create, now)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tCreate failed.", tests.Failed)
				}

				// Create a new random account and associate that with the user.
				accountId := uuid.NewRandom().String()
				_, err = AddAccount(tests.Context(), auth.Claims{}, test.MasterDB, AddAccountRequest{
					UserID:    user.ID,
					AccountID: accountId,
					Roles:     []UserAccountRole{UserAccountRole_User},
				}, now)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tAddAccount failed.", tests.Failed)
				}

				// Update the user.
				updateReq := tt.update(user)
				err = Update(ctx, tt.claims(user, accountId), test.MasterDB, updateReq, now)
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
				}
				t.Logf("\t%s\tUpdate ok.", tests.Success)

				// Find the user and make sure the updates where made.
				findRes, err := FindById(ctx, tt.claims(user, accountId), test.MasterDB, user.ID, false)
				if err != nil && errors.Cause(err) != tt.findErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.findErr)
					t.Fatalf("\t%s\tFindById failed.", tests.Failed)
				} else {
					findExpected := tt.expected(findRes, updateReq)
					if diff := cmp.Diff(findRes, findExpected); diff != "" {
						t.Fatalf("\t%s\tExpected find result to match update. Diff:\n%s", tests.Failed, diff)
					}
					t.Logf("\t%s\tFindById ok.", tests.Success)
				}

				// Archive (soft-delete) the user.
				err = Archive(ctx, tt.claims(user, accountId), test.MasterDB, user.ID, now)
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tArchive failed.", tests.Failed)
				} else if tt.updateErr == nil {
					// Trying to find the archived user with the includeArchived false should result in not found.
					_, err = FindById(ctx, tt.claims(user, accountId), test.MasterDB, user.ID, false)
					if err != nil && errors.Cause(err) != ErrNotFound {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", ErrNotFound)
						t.Fatalf("\t%s\tArchive FindById failed.", tests.Failed)
					}

					// Trying to find the archived user with the includeArchived true should result no error.
					_, err = FindById(ctx, tt.claims(user, accountId), test.MasterDB, user.ID, true)
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tArchive FindById failed.", tests.Failed)
					}
				}
				t.Logf("\t%s\tArchive ok.", tests.Success)

				// Delete (hard-delete) the user.
				err = Delete(ctx, tt.claims(user, accountId), test.MasterDB, user.ID)
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
				} else if tt.updateErr == nil {
					// Trying to find the deleted user with the includeArchived true should result in not found.
					_, err = FindById(ctx, tt.claims(user, accountId), test.MasterDB, user.ID, true)
					if errors.Cause(err) != ErrNotFound {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", ErrNotFound)
						t.Fatalf("\t%s\tDelete FindById failed.", tests.Failed)
					}
				}
				t.Logf("\t%s\tDelete ok.", tests.Success)
			}
		}
	}
}

// TestUserFind validates all the request params are correctly parsed into a select query.
func TestUserFind(t *testing.T) {

	// Ensure all the existing users are deleted.
	{
		// Build the delete SQL statement.
		query := sqlbuilder.NewDeleteBuilder()
		query.DeleteFrom(usersTableName)

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

	var users []*User
	for i := 0; i <= 4; i++ {
		user, err := Create(tests.Context(), auth.Claims{}, test.MasterDB, CreateUserRequest{
			Name:            "Lee Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		}, now.Add(time.Second*time.Duration(i)))
		if err != nil {
			t.Logf("\t\tGot : %+v", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}
		users = append(users, user)
	}

	type userTest struct {
		name     string
		req      UserFindRequest
		expected []*User
		error    error
	}

	var userTests []userTest

	// Test sort users.
	userTests = append(userTests, userTest{"Find all order by created_at asx",
		UserFindRequest{
			Order: []string{"created_at"},
		},
		users,
		nil,
	})

	// Test reverse sorted users.
	var expected []*User
	for i := len(users) - 1; i >= 0; i-- {
		expected = append(expected, users[i])
	}
	userTests = append(userTests, userTest{"Find all order by created_at desc",
		UserFindRequest{
			Order: []string{"created_at desc"},
		},
		expected,
		nil,
	})

	// Test limit.
	var limit uint = 2
	userTests = append(userTests, userTest{"Find limit",
		UserFindRequest{
			Order: []string{"created_at"},
			Limit: &limit,
		},
		users[0:2],
		nil,
	})

	// Test offset.
	var offset uint = 3
	userTests = append(userTests, userTest{"Find limit, offset",
		UserFindRequest{
			Order:  []string{"created_at"},
			Limit:  &limit,
			Offset: &offset,
		},
		users[3:5],
		nil,
	})

	// Test where filter.
	whereParts := []string{}
	whereArgs := []interface{}{}
	expected = []*User{}
	selected := make(map[string]bool)
	for i := 0; i <= 2; i++ {
		ranIdx := rand.Intn(len(users))

		email := users[ranIdx].Email
		if selected[email] {
			continue
		}
		selected[email] = true

		whereParts = append(whereParts, "email = ?")
		whereArgs = append(whereArgs, email)
		expected = append(expected, users[ranIdx])
	}
	where := strings.Join(whereParts, " OR ")
	userTests = append(userTests, userTest{"Find where",
		UserFindRequest{
			Where: &where,
			Args:  whereArgs,
		},
		expected,
		nil,
	})

	t.Log("Given the need to ensure find users returns the expected results.")
	{
		for i, tt := range userTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				res, err := Find(ctx, auth.Claims{}, test.MasterDB, tt.req)
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
