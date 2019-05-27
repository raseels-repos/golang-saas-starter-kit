package user

import (
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"github.com/dgrijalva/jwt-go"
	"github.com/google/go-cmp/cmp"
	"github.com/huandu/go-sqlbuilder"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"os"
	"testing"
	"time"
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
	expected := "SELECT " + usersMapColumns + " FROM " + usersTableName + " WHERE name = ? or email = ? ORDER BY id asc, created_at desc LIMIT 12 OFFSET 34"

	res := userFindRequestQuery(req)

	if diff := cmp.Diff(res.String(), expected); diff != "" {
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
			"SELECT " + usersMapColumns + " FROM " + usersTableName + " WHERE id IN (SELECT user_id FROM " + usersAccountsTableName + " WHERE account_id = 'acc1' AND user_id = 'user1')",
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
			"SELECT " + usersMapColumns + " FROM " + usersTableName + " WHERE id IN (SELECT user_id FROM " + usersAccountsTableName + " WHERE account_id = 'acc1' AND user_id = 'user1')",
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

// TestCreateUser validates CreateUser
func TestCreateUser(t *testing.T) {

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	// Use disabled status since default is active
	us := UserStatus_Disabled
	utz := "America/Santiago"

	dupEmail := uuid.NewRandom().String() + "@geeksinthewoods.com"

	var userTests = []struct {
		name   string
		claims auth.Claims
		req    CreateUserRequest
		error  error
	}{
		{"EmptyClaims",
			auth.Claims{},
			CreateUserRequest{
				Name:            "Lee Brown",
				Email:           dupEmail,
				Password:        "akTechFr0n!ier",
				PasswordConfirm: "akTechFr0n!ier",
				Status:          &us,
				Timezone:        &utz,
			},
			nil,
		},
		{"DuplicateEmailValidation",
			auth.Claims{},
			CreateUserRequest{
				Name:            "Lee Brown",
				Email:           dupEmail,
				Password:        "akTechFr0n!ier",
				PasswordConfirm: "akTechFr0n!ier",
				Status:          &us,
				Timezone:        &utz,
			},
			errors.New("Key: 'CreateUserRequest.Email' Error:Field validation for 'Email' failed on the 'unique' tag"),
		},
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
				Status:          &us,
				Timezone:        &utz,
			},
			ErrForbidden,
		},
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
				Status:          &us,
				Timezone:        &utz,
			},
			nil,
		},
	}

	t.Log("Given the need to validate ACLs are enforced by claims for user create.")
	{
		for i, tt := range userTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				dbConn := test.MasterDB
				defer dbConn.Close()

				res, err := Create(ctx, tt.claims, dbConn, tt.req, now)
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
						t.Fatalf("\t%s\tapplyClaimsUserSelect failed.", tests.Failed)
					}
				}

				// If there was an error that was expected, then don't go any further
				if tt.error != nil {
					continue
				}

				expected := &User{
					Name:     tt.req.Name,
					Email:    tt.req.Email,
					Status:   *tt.req.Status,
					Timezone: *tt.req.Timezone,

					// Copy this fields from the result.
					ID:            res.ID,
					PasswordSalt:  res.PasswordSalt,
					PasswordHash:  res.PasswordHash,
					PasswordReset: res.PasswordReset,
					CreatedAt:     res.CreatedAt,
					UpdatedAt:     res.UpdatedAt,
					//ArchivedAt: nil,
				}

				if diff := cmp.Diff(res, expected); diff != "" {
					t.Fatalf("\t%s\tExpected result should match. Diff:\n%s", tests.Failed, diff)
				}

				t.Logf("\t%s\tapplyClaimsUserSelect ok.", tests.Success)
			}
		}
	}
}

// TestUpdateUser validates Update
func TestUpdateUser(t *testing.T) {

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	// Use disabled status since default is active
	us := UserStatus_Disabled
	utz := "America/Santiago"

	create := CreateUserRequest{
		Name:            "Lee Brown",
		Password:        "akTechFr0n!ier",
		PasswordConfirm: "akTechFr0n!ier",
		Status:          &us,
		Timezone:        &utz,
	}

	dupEmail := uuid.NewRandom().String() + "@geeksinthewoods.com"

	var userTests = []struct {
		name   string
		claims auth.Claims
		req    UpdateUserRequest
		error  error
	}{
		{"EmptyClaims",
			auth.Claims{},
			UpdateUserRequest{
				Name:     "Lee Brown",
				Email:    dupEmail,
				Status:   &us,
				Timezone: &utz,
			},
			nil,
		},
		{"DuplicateEmailValidation",
			auth.Claims{},
			UpdateUserRequest{
				Name:     "Lee Brown",
				Email:    dupEmail,
				Status:   &us,
				Timezone: &utz,
			},
			errors.New("Key: 'CreateUserRequest.Email' Error:Field validation for 'Email' failed on the 'unique' tag"),
		},
		{"RoleUser",
			auth.Claims{
				Roles: []string{auth.RoleUser},
				StandardClaims: jwt.StandardClaims{
					Subject:  "user1",
					Audience: "acc1",
				},
			},
			UpdateUserRequest{
				Name:     "Lee Brown",
				Email:    &uuid.NewRandom().String(),
				Status:   &us,
				Timezone: &utz,
			},
			ErrForbidden,
		},
		{"RoleAdmin",
			auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Subject:  "user1",
					Audience: "acc1",
				},
			},
			UpdateUserRequest{
				Name:     "Lee Brown",
				Email:    uuid.NewRandom().String() + "@geeksinthewoods.com",
				Status:   &us,
				Timezone: &utz,
			},
			nil,
		},
	}

	t.Log("Given the need to validate ACLs are enforced by claims for user update.")
	{
		for i, tt := range userTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				dbConn := test.MasterDB
				defer dbConn.Close()

				err := Update(ctx, tt.claims, dbConn, tt.req, now)
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
						t.Fatalf("\t%s\tapplyClaimsUserSelect failed.", tests.Failed)
					}
				}

				// If there was an error that was expected, then don't go any further
				if tt.error != nil {
					continue
				}

				expected := &User{
					Name:     tt.req.Name,
					Email:    tt.req.Email,
					Status:   *tt.req.Status,
					Timezone: *tt.req.Timezone,

					// Copy this fields from the result.
					ID:            res.ID,
					PasswordSalt:  res.PasswordSalt,
					PasswordHash:  res.PasswordHash,
					PasswordReset: res.PasswordReset,
					CreatedAt:     res.CreatedAt,
					UpdatedAt:     res.UpdatedAt,
					//ArchivedAt: nil,
				}

				if diff := cmp.Diff(res, expected); diff != "" {
					t.Fatalf("\t%s\tExpected result should match. Diff:\n%s", tests.Failed, diff)
				}

				t.Logf("\t%s\tapplyClaimsUserSelect ok.", tests.Success)
			}
		}
	}
}

/*
// TestUser validates the full set of CRUD operations on User values.
func TestUser(t *testing.T) {
	defer tests.Recover(t)

	t.Log("Given the need to work with User records.")
	{
		t.Log("\tWhen handling a single User.")
		{
			ctx := tests.Context()

			dbConn := test.MasterDB
			defer dbConn.Close()



			u, err := Create(ctx, dbConn, &nu, now)
			if err != nil {
				t.Fatalf("\t%s\tShould be able to create user : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to create user.", tests.Success)



			// claims is information about the person making the request.
			claims := auth.NewClaims(bson.NewObjectId().Hex(), []string{auth.RoleAdmin}, now, time.Hour)


			savedU, err := user.Retrieve(ctx, claims, dbConn, u.ID.Hex())
			if err != nil {
				t.Fatalf("\t%s\tShould be able to retrieve user by ID: %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to retrieve user by ID.", tests.Success)

			if diff := cmp.Diff(u, savedU); diff != "" {
				t.Fatalf("\t%s\tShould get back the same user. Diff:\n%s", tests.Failed, diff)
			}
			t.Logf("\t%s\tShould get back the same user.", tests.Success)

			upd := user.UpdateUser{
				Name:  tests.StringPointer("Jacob Walker"),
				Email: tests.StringPointer("jacob@ardanlabs.com"),
			}

			if err := user.Update(ctx, dbConn, u.ID.Hex(), &upd, now); err != nil {
				t.Fatalf("\t%s\tShould be able to update user : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to update user.", tests.Success)

			savedU, err = user.Retrieve(ctx, claims, dbConn, u.ID.Hex())
			if err != nil {
				t.Fatalf("\t%s\tShould be able to retrieve user : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to retrieve user.", tests.Success)

			if savedU.Name != *upd.Name {
				t.Errorf("\t%s\tShould be able to see updates to Name.", tests.Failed)
				t.Log("\t\tGot:", savedU.Name)
				t.Log("\t\tExp:", *upd.Name)
			} else {
				t.Logf("\t%s\tShould be able to see updates to Name.", tests.Success)
			}

			if savedU.Email != *upd.Email {
				t.Errorf("\t%s\tShould be able to see updates to Email.", tests.Failed)
				t.Log("\t\tGot:", savedU.Email)
				t.Log("\t\tExp:", *upd.Email)
			} else {
				t.Logf("\t%s\tShould be able to see updates to Email.", tests.Success)
			}

			if err := user.Delete(ctx, dbConn, u.ID.Hex()); err != nil {
				t.Fatalf("\t%s\tShould be able to delete user : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to delete user.", tests.Success)

			savedU, err = user.Retrieve(ctx, claims, dbConn, u.ID.Hex())
			if errors.Cause(err) != user.ErrNotFound {
				t.Fatalf("\t%s\tShould NOT be able to retrieve user : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould NOT be able to retrieve user.", tests.Success)


		}
	}
}


// mockTokenGenerator is used for testing that Authenticate calls its provided
// token generator in a specific way.
type mockTokenGenerator struct{}

// GenerateToken implements the TokenGenerator interface. It returns a "token"
// that includes some information about the claims it was passed.
func (mockTokenGenerator) GenerateToken(claims auth.Claims) (string, error) {
	return fmt.Sprintf("sub:%q iss:%d", claims.Subject, claims.IssuedAt), nil
}

// TestAuthenticate validates the behavior around authenticating users.
func TestAuthenticate(t *testing.T) {
	defer tests.Recover(t)

	t.Log("Given the need to authenticate users")
	{
		t.Log("\tWhen handling a single User.")
		{
			ctx := tests.Context()

			dbConn := test.MasterDB.Copy()
			defer dbConn.Close()

			nu := user.NewUser{
				Name:            "Anna Walker",
				Email:           "anna@ardanlabs.com",
				Roles:           []string{auth.RoleAdmin},
				Password:        "goroutines",
				PasswordConfirm: "goroutines",
			}

			now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

			u, err := user.Create(ctx, dbConn, &nu, now)
			if err != nil {
				t.Fatalf("\t%s\tShould be able to create user : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to create user.", tests.Success)

			var tknGen mockTokenGenerator
			tkn, err := user.Authenticate(ctx, dbConn, tknGen, now, "anna@ardanlabs.com", "goroutines")
			if err != nil {
				t.Fatalf("\t%s\tShould be able to generate a token : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to generate a token.", tests.Success)

			want := fmt.Sprintf("sub:%q iss:1538352000", u.ID.Hex())
			if tkn.Token != want {
				t.Log("\t\tGot :", tkn.Token)
				t.Log("\t\tWant:", want)
				t.Fatalf("\t%s\tToken should indicate the specified user and time were used.", tests.Failed)
			}
			t.Logf("\t%s\tToken should indicate the specified user and time were used.", tests.Success)

			if err := user.Delete(ctx, dbConn, u.ID.Hex()); err != nil {
				t.Fatalf("\t%s\tShould be able to delete user : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to delete user.", tests.Success)
		}
	}
}
*/
