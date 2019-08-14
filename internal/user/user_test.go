package user

import (
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/notify"
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

	// Mock the methods needed to make a password reset.
	resetUrl := func(string) string {
		return ""
	}
	notify := &notify.MockEmail{}
	secretKey := "6368616e676520746869732070617373"

	repo = NewRepository(test.MasterDB, resetUrl, notify, secretKey)

	return m.Run()
}

// TestFindRequestQuery validates findRequestQuery
func TestFindRequestQuery(t *testing.T) {
	var (
		limit  uint = 12
		offset uint = 34
	)

	req := UserFindRequest{
		Where: "first_name = ? or email = ?",
		Args: []interface{}{
			"lee",
			"lee@geeksinthewoods.com",
		},
		Order: []string{
			"id asc",
			"created_at desc",
		},
		Limit:  &limit,
		Offset: &offset,
	}
	expected := "SELECT " + userMapColumns + " FROM " + userTableName + " WHERE (first_name = ? or email = ?) ORDER BY id asc, created_at desc LIMIT 12 OFFSET 34"

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
			"SELECT " + userMapColumns + " FROM " + userTableName,
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
			"SELECT " + userMapColumns + " FROM " + userTableName + " WHERE id IN (SELECT user_id FROM " + userAccountTableName + " WHERE (account_id = 'acc1' OR user_id = 'user1'))",
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
			"SELECT " + userMapColumns + " FROM " + userTableName + " WHERE id IN (SELECT user_id FROM " + userAccountTableName + " WHERE (account_id = 'acc1' OR user_id = 'user1'))",
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

// TestCreateValidation ensures all the validation tags work on Create
func TestCreateValidation(t *testing.T) {

	var userTests = []struct {
		name     string
		req      UserCreateRequest
		expected func(req UserCreateRequest, res *User) *User
		error    error
	}{
		{"Required Fields",
			UserCreateRequest{},
			func(req UserCreateRequest, res *User) *User {
				return nil
			},
			errors.New("Key: 'UserCreateRequest.first_name' Error:Field validation for 'first_name' failed on the 'required' tag\n" +
				"Key: 'UserCreateRequest.last_name' Error:Field validation for 'last_name' failed on the 'required' tag\n" +
				"Key: 'UserCreateRequest.email' Error:Field validation for 'email' failed on the 'required' tag\n" +
				"Key: 'UserCreateRequest.password' Error:Field validation for 'password' failed on the 'required' tag\n" +
				"Key: 'UserCreateRequest.password_confirm' Error:Field validation for 'password_confirm' failed on the 'required' tag"),
		},
		{"Valid Email",
			UserCreateRequest{
				FirstName:       "Lee",
				LastName:        "Brown",
				Email:           "xxxxxxxxxx",
				Password:        "akTechFr0n!ier",
				PasswordConfirm: "akTechFr0n!ier",
			},
			func(req UserCreateRequest, res *User) *User {
				return nil
			},
			errors.New("Key: 'UserCreateRequest.email' Error:Field validation for 'email' failed on the 'email' tag"),
		},
		{"Passwords Match",
			UserCreateRequest{
				FirstName:       "Lee",
				LastName:        "Brown",
				Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
				Password:        "akTechFr0n!ier",
				PasswordConfirm: "W0rkL1fe#",
			},
			func(req UserCreateRequest, res *User) *User {
				return nil
			},
			errors.New("Key: 'UserCreateRequest.password_confirm' Error:Field validation for 'password_confirm' failed on the 'eqfield' tag"),
		},
		{"Default Timezone",
			UserCreateRequest{
				FirstName:       "Lee",
				LastName:        "Brown",
				Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
				Password:        "akTechFr0n!ier",
				PasswordConfirm: "akTechFr0n!ier",
			},
			func(req UserCreateRequest, res *User) *User {
				return &User{
					FirstName: "Lee",
					LastName:  "Brown",
					Email:     req.Email,
					Timezone:  nil,

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

// TestCreateValidationEmailUnique validates emails must be unique on Create.
func TestCreateValidationEmailUnique(t *testing.T) {

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure duplicate emails are not allowed for user create.")
	{
		ctx := tests.Context()

		req1 := UserCreateRequest{
			FirstName:       "Lee",
			LastName:        "Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		}
		user1, err := repo.Create(ctx, auth.Claims{}, req1, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		req2 := UserCreateRequest{
			FirstName:       "Lee",
			LastName:        "Brown",
			Email:           user1.Email,
			Password:        "W0rkL1fe#",
			PasswordConfirm: "W0rkL1fe#",
		}
		expectedErr := errors.New("Key: 'UserCreateRequest.email' Error:Field validation for 'email' failed on the 'unique' tag")
		_, err = repo.Create(ctx, auth.Claims{}, req2, now)
		if err == nil {
			t.Logf("\t\tWant: %+v", expectedErr)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		errStr := strings.Replace(err.Error(), "{{", "", -1)
		errStr = strings.Replace(errStr, "}}", "", -1)

		if errStr != expectedErr.Error() {
			t.Logf("\t\tGot : %+v", errStr)
			t.Logf("\t\tWant: %+v", expectedErr)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		t.Logf("\t%s\tCreate ok.", tests.Success)
	}
}

// TestCreateClaims validates ACLs are correctly applied to Create by claims.
func TestCreateClaims(t *testing.T) {
	defer tests.Recover(t)

	var userTests = []struct {
		name   string
		claims auth.Claims
		req    UserCreateRequest
		error  error
	}{
		// Internal request, should bypass ACL.
		{"EmptyClaims",
			auth.Claims{},
			UserCreateRequest{
				FirstName:       "Lee",
				LastName:        "Brown",
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
			UserCreateRequest{
				FirstName:       "Lee",
				LastName:        "Brown",
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
			UserCreateRequest{
				FirstName:       "Lee",
				LastName:        "Brown",
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

				_, err := repo.Create(ctx, tt.claims, tt.req, now)
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
	// TODO: actually create the user so can test the output of findbyId
	type userTest struct {
		name  string
		req   UserUpdateRequest
		error error
	}

	var userTests = []userTest{
		{"Required Fields",
			UserUpdateRequest{},
			errors.New("Key: 'UserUpdateRequest.id' Error:Field validation for 'id' failed on the 'required' tag"),
		},
	}

	invalidEmail := "xxxxxxxxxx"
	userTests = append(userTests, userTest{"Valid Email",
		UserUpdateRequest{
			ID:    uuid.NewRandom().String(),
			Email: &invalidEmail,
		},
		errors.New("Key: 'UserUpdateRequest.email' Error:Field validation for 'email' failed on the 'email' tag"),
	})

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure all validation tags are working for user update.")
	{
		for i, tt := range userTests {
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
						t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
					}
				}

				t.Logf("\t%s\tUpdate ok.", tests.Success)
			}
		}
	}
}

// TestUpdateValidationEmailUnique validates emails must be unique on Update.
func TestUpdateValidationEmailUnique(t *testing.T) {

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure duplicate emails are not allowed for user update.")
	{
		ctx := tests.Context()

		req1 := UserCreateRequest{
			FirstName:       "Lee",
			LastName:        "Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		}
		user1, err := repo.Create(ctx, auth.Claims{}, req1, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		req2 := UserCreateRequest{
			FirstName:       "Lee",
			LastName:        "Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "W0rkL1fe#",
			PasswordConfirm: "W0rkL1fe#",
		}
		user2, err := repo.Create(ctx, auth.Claims{}, req2, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		// Try to set the email for user 1 on user 2
		updateReq := UserUpdateRequest{
			ID:    user2.ID,
			Email: &user1.Email,
		}
		expectedErr := errors.New("Key: 'UserUpdateRequest.email' Error:Field validation for 'email' failed on the 'unique' tag")
		err = repo.Update(ctx, auth.Claims{}, updateReq, now)
		if err == nil {
			t.Logf("\t\tWant: %+v", expectedErr)
			t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
		}

		errStr := strings.Replace(err.Error(), "{{", "", -1)
		errStr = strings.Replace(errStr, "}}", "", -1)

		if errStr != expectedErr.Error() {
			t.Logf("\t\tGot : %+v", errStr)
			t.Logf("\t\tWant: %+v", expectedErr)
			t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
		}

		t.Logf("\t%s\tUpdate ok.", tests.Success)
	}
}

// TestUpdatePassword validates update user password works.
func TestUpdatePassword(t *testing.T) {

	t.Log("Given the need ensure a user password can be updated.")
	{
		ctx := tests.Context()

		now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

		// Create a new user for testing.
		initPass := uuid.NewRandom().String()
		user, err := repo.Create(ctx, auth.Claims{}, UserCreateRequest{
			FirstName:       "Lee",
			LastName:        "Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        initPass,
			PasswordConfirm: initPass,
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		// Create a new random account.
		accountId := uuid.NewRandom().String()
		err = mockAccount(accountId, user.CreatedAt)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate account failed.", tests.Failed)
		}

		// Associate new random account with user.
		err = mockUserAccount(user.ID, accountId, user.CreatedAt, auth.RoleUser)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}

		// Ensure validation is working by trying UpdatePassword with an empty request.
		expectedErr := errors.New("Key: 'UserUpdatePasswordRequest.id' Error:Field validation for 'id' failed on the 'required' tag\n" +
			"Key: 'UserUpdatePasswordRequest.password' Error:Field validation for 'password' failed on the 'required' tag\n" +
			"Key: 'UserUpdatePasswordRequest.password_confirm' Error:Field validation for 'password_confirm' failed on the 'required' tag")
		err = repo.UpdatePassword(ctx, auth.Claims{}, UserUpdatePasswordRequest{}, now)
		if err == nil {
			t.Logf("\t\tWant: %+v", expectedErr)
			t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
		}

		errStr := strings.Replace(err.Error(), "{{", "", -1)
		errStr = strings.Replace(errStr, "}}", "", -1)

		if errStr != expectedErr.Error() {
			t.Logf("\t\tGot : %+v", errStr)
			t.Logf("\t\tWant: %+v", expectedErr)
			t.Fatalf("\t%s\tValidation failed.", tests.Failed)
		}
		t.Logf("\t%s\tValidation ok.", tests.Success)

		// Update the users password.
		newPass := uuid.NewRandom().String()
		err = repo.UpdatePassword(ctx, auth.Claims{}, UserUpdatePasswordRequest{
			ID:              user.ID,
			Password:        newPass,
			PasswordConfirm: newPass,
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tUpdate password failed.", tests.Failed)
		}
		t.Logf("\t%s\tUpdatePassword ok.", tests.Success)
	}
}

// TestCrud validates the full set of CRUD operations for users and ensures ACLs are correctly applied by claims.
func TestCrud(t *testing.T) {
	defer tests.Recover(t)

	type userTest struct {
		name      string
		claims    func(*User, string) auth.Claims
		create    UserCreateRequest
		update    func(*User) UserUpdateRequest
		updateErr error
		expected  func(*User, UserUpdateRequest) *User
		findErr   error
	}

	var userTests []userTest

	// Internal request, should bypass ACL.
	userTests = append(userTests, userTest{"EmptyClaims",
		func(user *User, accountId string) auth.Claims {
			return auth.Claims{}
		},
		UserCreateRequest{
			FirstName:       "Lee",
			LastName:        "Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
		func(user *User) UserUpdateRequest {
			email := uuid.NewRandom().String() + "@geeksinthewoods.com"
			return UserUpdateRequest{
				ID:    user.ID,
				Email: &email,
			}
		},
		nil,
		func(user *User, req UserUpdateRequest) *User {
			return &User{
				Email: *req.Email,
				// Copy this fields from the created user.
				ID:            user.ID,
				FirstName:     user.FirstName,
				LastName:      user.LastName,
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
		UserCreateRequest{
			FirstName:       "Lee",
			LastName:        "Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
		func(user *User) UserUpdateRequest {
			email := uuid.NewRandom().String() + "@geeksinthewoods.com"
			return UserUpdateRequest{
				ID:    user.ID,
				Email: &email,
			}
		},
		ErrForbidden,
		func(user *User, req UserUpdateRequest) *User {
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
		UserCreateRequest{
			FirstName:       "Lee",
			LastName:        "Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
		func(user *User) UserUpdateRequest {
			email := uuid.NewRandom().String() + "@geeksinthewoods.com"
			return UserUpdateRequest{
				ID:    user.ID,
				Email: &email,
			}
		},
		nil,
		func(user *User, req UserUpdateRequest) *User {
			return &User{
				Email: *req.Email,
				// Copy this fields from the created user.
				ID:            user.ID,
				FirstName:     user.FirstName,
				LastName:      user.LastName,
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
		UserCreateRequest{
			FirstName:       "Lee",
			LastName:        "Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
		func(user *User) UserUpdateRequest {
			email := uuid.NewRandom().String() + "@geeksinthewoods.com"
			return UserUpdateRequest{
				ID:    user.ID,
				Email: &email,
			}
		},
		ErrForbidden,
		func(user *User, req UserUpdateRequest) *User {
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
		UserCreateRequest{
			FirstName:       "Lee",
			LastName:        "Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
		func(user *User) UserUpdateRequest {
			email := uuid.NewRandom().String() + "@geeksinthewoods.com"
			return UserUpdateRequest{
				ID:    user.ID,
				Email: &email,
			}
		},
		nil,
		func(user *User, req UserUpdateRequest) *User {
			return &User{
				Email: *req.Email,
				// Copy this fields from the created user.
				ID:            user.ID,
				FirstName:     user.FirstName,
				LastName:      user.LastName,
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
				user, err := repo.Create(tests.Context(), auth.Claims{}, tt.create, now)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tCreate user failed.", tests.Failed)
				}

				// Create a random account for the new user.
				accountId := uuid.NewRandom().String()
				err = mockAccount(accountId, user.CreatedAt)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tCreate account failed.", tests.Failed)
				}

				// Associate the account with the new test user.
				err = mockUserAccount(user.ID, accountId, user.CreatedAt, auth.RoleAdmin)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
				}

				// Update the user.
				updateReq := tt.update(user)
				err = repo.Update(ctx, tt.claims(user, accountId), updateReq, now)
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
				}
				t.Logf("\t%s\tUpdate ok.", tests.Success)

				// Find the user and make sure the updates where made.
				findRes, err := repo.ReadByID(ctx, tt.claims(user, accountId), user.ID)
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

				// Archive (soft-delete) the user.
				err = repo.Archive(ctx, tt.claims(user, accountId), UserArchiveRequest{ID: user.ID, force: true}, now)
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tArchive failed.", tests.Failed)
				} else if tt.updateErr == nil {
					// Trying to find the archived user with the includeArchived false should result in not found.
					_, err = repo.ReadByID(ctx, tt.claims(user, accountId), user.ID)
					if err != nil && errors.Cause(err) != ErrNotFound {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", ErrNotFound)
						t.Fatalf("\t%s\tArchive Read failed.", tests.Failed)
					}

					// Trying to find the archived user with the includeArchived true should result no error.
					_, err = repo.Read(ctx, tt.claims(user, accountId),
						UserReadRequest{ID: user.ID, IncludeArchived: true})
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tArchive Read failed.", tests.Failed)
					}
				}
				t.Logf("\t%s\tArchive ok.", tests.Success)

				// Restore (un-delete) the user.
				err = repo.Restore(ctx, tt.claims(user, accountId), UserRestoreRequest{ID: user.ID}, now)
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tUnarchive failed.", tests.Failed)
				} else if tt.updateErr == nil {
					// Trying to find the archived user with the includeArchived false should result no error.
					_, err = repo.ReadByID(ctx, tt.claims(user, accountId), user.ID)
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tUnarchive Read failed.", tests.Failed)
					}
				}
				t.Logf("\t%s\tUnarchive ok.", tests.Success)

				// Delete (hard-delete) the user.
				err = repo.Delete(ctx, tt.claims(user, accountId), UserDeleteRequest{ID: user.ID, force: true})
				if err != nil && errors.Cause(err) != tt.updateErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.updateErr)
					t.Fatalf("\t%s\tUpdate failed.", tests.Failed)
				} else if tt.updateErr == nil {
					// Trying to find the deleted user with the includeArchived true should result in not found.
					_, err = repo.ReadByID(ctx, tt.claims(user, accountId), user.ID)
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

	var users []*User
	for i := 0; i <= 4; i++ {
		user, err := repo.Create(tests.Context(), auth.Claims{},  UserCreateRequest{
			FirstName:       "Lee",
			LastName:        "Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		}, now.Add(time.Second*time.Duration(i)))
		if err != nil {
			t.Logf("\t\tGot : %+v", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}
		users = append(users, user)
		endTime = user.CreatedAt
	}

	type userTest struct {
		name     string
		req      UserFindRequest
		expected Users
		error    error
	}

	var userTests []userTest

	createdFilter := "created_at BETWEEN ? AND ?"

	// Test sort users.
	userTests = append(userTests, userTest{"Find all order by created_at asc",
		UserFindRequest{
			Where: createdFilter,
			Args:  []interface{}{startTime, endTime},
			Order: []string{"created_at"},
		},
		users,
		nil,
	})

	// Test reverse sorted users.
	var expected Users
	for i := len(users) - 1; i >= 0; i-- {
		expected = append(expected, users[i])
	}
	userTests = append(userTests, userTest{"Find all order by created_at desc",
		UserFindRequest{
			Where: createdFilter,
			Args:  []interface{}{startTime, endTime},
			Order: []string{"created_at desc"},
		},
		expected,
		nil,
	})

	// Test limit.
	var limit uint = 2
	userTests = append(userTests, userTest{"Find limit",
		UserFindRequest{
			Where: createdFilter,
			Args:  []interface{}{startTime, endTime},
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
			Where:  createdFilter,
			Args:   []interface{}{startTime, endTime},
			Order:  []string{"created_at"},
			Limit:  &limit,
			Offset: &offset,
		},
		users[3:5],
		nil,
	})

	// Test where filter.
	whereParts := []string{}
	whereArgs := []interface{}{startTime, endTime}
	expected = []*User{}
	for i := 0; i <= len(users); i++ {
		if rand.Intn(100) < 50 {
			continue
		}
		u := *users[i]

		whereParts = append(whereParts, "email = ?")
		whereArgs = append(whereArgs, u.Email)
		expected = append(expected, &u)
	}

	where := createdFilter + " AND (" + strings.Join(whereParts, " OR ") + ")"
	userTests = append(userTests, userTest{"Find where",
		UserFindRequest{
			Where: where,
			Args:  whereArgs,
			Order: []string{"created_at"},
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

				res, err := repo.Find(ctx, auth.Claims{},  tt.req)
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

// TestResetPassword validates that reset password for a user works.
func TestResetPassword(t *testing.T) {

	t.Log("Given the need ensure a user can reset their password.")
	{
		ctx := tests.Context()

		now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

		// Create a new user for testing.
		initPass := uuid.NewRandom().String()
		user, err := repo.Create(ctx, auth.Claims{}, UserCreateRequest{
			FirstName:       "Lee",
			LastName:        "Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        initPass,
			PasswordConfirm: initPass,
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate failed.", tests.Failed)
		}

		// Create a new random account.
		accountId := uuid.NewRandom().String()
		err = mockAccount(accountId, user.CreatedAt)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate account failed.", tests.Failed)
		}

		// Associate new random account with user.
		err = mockUserAccount(user.ID, accountId, user.CreatedAt, auth.RoleUser)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tCreate user account failed.", tests.Failed)
		}

		// Ensure validation is working by trying ResetPassword with an empty request.
		{
			expectedErr := errors.New("Key: 'UserResetPasswordRequest.email' Error:Field validation for 'email' failed on the 'required' tag")
			_, err = repo.ResetPassword(ctx, UserResetPasswordRequest{}, now)
			if err == nil {
				t.Logf("\t\tWant: %+v", expectedErr)
				t.Fatalf("\t%s\tResetPassword failed.", tests.Failed)
			}

			errStr := strings.Replace(err.Error(), "{{", "", -1)
			errStr = strings.Replace(errStr, "}}", "", -1)

			if errStr != expectedErr.Error() {
				t.Logf("\t\tGot : %+v", errStr)
				t.Logf("\t\tWant: %+v", expectedErr)
				t.Fatalf("\t%s\tResetPassword Validation failed.", tests.Failed)
			}
			t.Logf("\t%s\tResetPassword Validation ok.", tests.Success)
		}

		ttl := time.Hour

		// Make the reset password request.
		resetHash, err := repo.ResetPassword(ctx, UserResetPasswordRequest{
			Email: user.Email,
			TTL:   ttl,
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tResetPassword failed.", tests.Failed)
		}
		t.Logf("\t%s\tResetPassword ok.", tests.Success)

		// Read the user to ensure the password_reset field was set.
		user, err = repo.ReadByID(ctx, auth.Claims{}, user.ID)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tRead failed.", tests.Failed)
		} else if user.PasswordReset == nil || user.PasswordReset.String == "" {
			t.Fatalf("\t%s\tUser field password_reset is empty.", tests.Failed)
		}

		// Ensure validation is working by trying ResetConfirm with an empty request.
		{
			expectedErr := errors.New("Key: 'UserResetConfirmRequest.reset_hash' Error:Field validation for 'reset_hash' failed on the 'required' tag\n" +
				"Key: 'UserResetConfirmRequest.password' Error:Field validation for 'password' failed on the 'required' tag\n" +
				"Key: 'UserResetConfirmRequest.password_confirm' Error:Field validation for 'password_confirm' failed on the 'required' tag")
			_, err = repo.ResetConfirm(ctx, UserResetConfirmRequest{}, now)
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
			_, err = repo.ResetConfirm(ctx, UserResetConfirmRequest{
				ResetHash:       resetHash,
				Password:        newPass,
				PasswordConfirm: newPass,
			}, now.UTC().Add(ttl*2))
			if errors.Cause(err) != ErrResetExpired {
				t.Logf("\t\tGot : %+v", errors.Cause(err))
				t.Logf("\t\tWant: %+v", ErrResetExpired)
				t.Fatalf("\t%s\tResetConfirm enforce TTL failed.", tests.Failed)
			}
			t.Logf("\t%s\tResetConfirm enforce TTL ok.", tests.Success)
		}

		// Assuming we have received the email and clicked the link, we now can ensure confirm works.
		newPass := uuid.NewRandom().String()
		reset, err := repo.ResetConfirm(ctx, UserResetConfirmRequest{
			ResetHash:       resetHash,
			Password:        newPass,
			PasswordConfirm: newPass,
		}, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tResetConfirm failed.", tests.Failed)
		} else if reset.ID != user.ID {
			t.Logf("\t\tGot : %+v", reset.ID)
			t.Logf("\t\tWant: %+v", user.ID)
			t.Fatalf("\t%s\tResetConfirm failed.", tests.Failed)
		}
		t.Logf("\t%s\tResetConfirm ok.", tests.Success)

		// Ensure the reset hash does not work after its used.
		{
			newPass := uuid.NewRandom().String()
			_, err = repo.ResetConfirm(ctx, UserResetConfirmRequest{
				ResetHash:       resetHash,
				Password:        newPass,
				PasswordConfirm: newPass,
			}, now)
			if errors.Cause(err) != ErrNotFound {
				t.Logf("\t\tGot : %+v", errors.Cause(err))
				t.Logf("\t\tWant: %+v", ErrNotFound)
				t.Fatalf("\t%s\tResetConfirm enforce TTL failed.", tests.Failed)
			}
			t.Logf("\t%s\tResetConfirm reuse disabled ok.", tests.Success)
		}
	}
}

func mockUserAccount(userId, accountId string, now time.Time, roles ...string) error {
	var roleArr pq.StringArray
	for _, r := range roles {
		roleArr = append(roleArr, r)
	}

	// Build the insert SQL statement.
	query := sqlbuilder.NewInsertBuilder()
	query.InsertInto(userAccountTableName)
	query.Cols("id", "user_id", "account_id", "roles", "created_at", "updated_at")
	query.Values(uuid.NewRandom().String(), userId, accountId, roleArr, now, now)

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

func mockAccount(accountId string, now time.Time) error {

	// Build the insert SQL statement.
	query := sqlbuilder.NewInsertBuilder()
	query.InsertInto(accountTableName)
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
