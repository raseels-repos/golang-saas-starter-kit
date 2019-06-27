package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/mid"
	"net/http"
	"strconv"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"github.com/google/go-cmp/cmp"
	"github.com/pborman/uuid"
)

func mockUser() *user.User {
	req := user.UserCreateRequest{
		Name:            "Lee Brown",
		Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
		Password:        "akTechFr0n!ier",
		PasswordConfirm: "akTechFr0n!ier",
	}

	a, err := user.Create(tests.Context(), auth.Claims{}, test.MasterDB, req, time.Now().UTC().AddDate(-1, -1, -1))
	if err != nil {
		panic(err)
	}
	return a
}

// TestUser is the entry point for the user endpoints.
func TestUser(t *testing.T) {
	defer tests.Recover(t)

	t.Run("getUser", getUser)
	t.Run("createUser", createUser)
	t.Run("patchUser", patchUser)
	t.Run("patchUserPassword", patchUserPassword)
}

// getUser validates get user by ID endpoint.
func getUser(t *testing.T) {

	var rtests []requestTest

	forbiddenUser := mockUser()

	// Both roles should be able to read the user.
	for rn, tr := range roleTests {
		usr := tr.SignupResult.User

		// Test 200.
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s 200", rn),
			http.MethodGet,
			fmt.Sprintf("/v1/users/%s", usr.ID),
			nil,
			tr.Token,
			tr.Claims,
			http.StatusOK,
			nil,
			func(treq requestTest, body []byte) bool {
				var actual user.UserResponse
				if err := json.Unmarshal(body, &actual); err != nil {
					t.Logf("\t\tGot error : %+v", err)
					return false
				}

				// Add claims to the context so they can be retrieved later.
				ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

				expectedMap := map[string]interface{}{
					"updated_at":      web.NewTimeResponse(ctx, usr.UpdatedAt),
					"id":              usr.ID,
					"email":        usr.Email,
					"timezone":        usr.Timezone,
					"created_at":      web.NewTimeResponse(ctx, usr.CreatedAt),
					"name":            usr.Name,
				}
				expectedJson, err := json.Marshal(expectedMap)
				if err != nil {
					t.Logf("\t\tGot error : %+v", err)
					return false
				}

				var expected user.UserResponse
				if err := json.Unmarshal([]byte(expectedJson), &expected); err != nil {
					t.Logf("\t\tGot error : %+v", err)
					printResultMap(ctx, body)
					return false
				}

				if diff := cmp.Diff(actual, expected); diff != "" {
					actualJSON, err := json.MarshalIndent(actual, "", "    ")
					if err != nil {
						t.Logf("\t\tGot error : %+v", err)
						return false
					}
					t.Logf("\t\tGot : %s\n", actualJSON)

					expectedJSON, err := json.MarshalIndent(expected, "", "    ")
					if err != nil {
						t.Logf("\t\tGot error : %+v", err)
						return false
					}
					t.Logf("\t\tExpected : %s\n", expectedJSON)

					t.Logf("\t\tDiff : %s\n", diff)

					if len(expectedMap) == 0 {
						printResultMap(ctx, body)
					}

					return false
				}

				return true
			},
		})

		// Test 404.
		invalidID := uuid.NewRandom().String()
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s 404 w/invalid ID", rn),
			http.MethodGet,
			fmt.Sprintf("/v1/users/%s", invalidID),
			nil,
			tr.Token,
			tr.Claims,
			http.StatusNotFound,
			web.ErrorResponse{
				Error: fmt.Sprintf("user %s not found: Entity not found", invalidID),
			},
			func(treq requestTest, body []byte) bool {
				return true
			},
		})

		// Test 404 - User exists but not allowed.
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s 404 w/random user ID", rn),
			http.MethodGet,
			fmt.Sprintf("/v1/users/%s", forbiddenUser.ID),
			nil,
			tr.Token,
			tr.Claims,
			http.StatusNotFound,
			web.ErrorResponse{
				Error: fmt.Sprintf("user %s not found: Entity not found", forbiddenUser.ID),
			},
			func(treq requestTest, body []byte) bool {
				return true
			},
		})
	}

	runRequestTests(t, rtests)
}

// createUser validates create user endpoint.
func createUser(t *testing.T) {

	var rtests []requestTest

	// Test create user.
	// 	Admin role: 201
	//  User role 403
	for rn, tr := range roleTests {
		var expectedStatus int
		var expectedErr interface{}

		// Test 201.
		if rn == auth.RoleAdmin {
			expectedStatus = http.StatusCreated
		} else {
			expectedStatus = http.StatusForbidden
			expectedErr = web.ErrorResponse{
				Error: mid.ErrForbidden.Error(),
			}
		}

		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d", rn, expectedStatus),
			http.MethodPost,
			"/v1/users",
			user.UserCreateRequest{
				Name:            "Lee Brown",
				Email:           uuid.NewRandom().String() + rn + strconv.Itoa(len(rtests))+ "@geeksinthewoods.com",
				Password:        "akTechFr0n!ier",
				PasswordConfirm: "akTechFr0n!ier",
			},
			tr.Token,
			tr.Claims,
			expectedStatus,
			expectedErr,
			func(treq requestTest, body []byte) bool {
				if treq.error != nil {
					return true
				}

				var actual user.UserResponse
				if err := json.Unmarshal(body, &actual); err != nil {
					t.Logf("\t\tGot error : %+v", err)
					return false
				}

				// Add claims to the context so they can be retrieved later.
				ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

				req := treq.request.(user.UserCreateRequest)

				expectedMap := map[string]interface{}{
					"updated_at":      web.NewTimeResponse(ctx, actual.UpdatedAt.Value),
					"id":              actual.ID,
					"email":        	req.Email,
					"timezone":        actual.Timezone,
					"created_at":      web.NewTimeResponse(ctx, actual.CreatedAt.Value),
					"name":            req.Name,
				}
				expectedJson, err := json.Marshal(expectedMap)
				if err != nil {
					t.Logf("\t\tGot error : %+v", err)
					return false
				}

				var expected user.UserResponse
				if err := json.Unmarshal([]byte(expectedJson), &expected); err != nil {
					t.Logf("\t\tGot error : %+v", err)
					printResultMap(ctx, body)
					return false
				}

				if diff := cmp.Diff(actual, expected); diff != "" {
					actualJSON, err := json.MarshalIndent(actual, "", "    ")
					if err != nil {
						t.Logf("\t\tGot error : %+v", err)
						return false
					}
					t.Logf("\t\tGot : %s\n", actualJSON)

					expectedJSON, err := json.MarshalIndent(expected, "", "    ")
					if err != nil {
						t.Logf("\t\tGot error : %+v", err)
						return false
					}
					t.Logf("\t\tExpected : %s\n", expectedJSON)

					t.Logf("\t\tDiff : %s\n", diff)

					if len(expectedMap) == 0 {
						printResultMap(ctx, body)
					}

					return false
				}

				return true
			},
		})
	}

	// Test update a user with invalid data.
	// 	Admin role: 400
	//  User role 403
	for rn, tr := range roleTests {
		var expectedStatus int
		var expectedErr interface{}

		// Test 201.
		if rn == auth.RoleAdmin {
			expectedStatus = http.StatusBadRequest
			expectedErr = web.ErrorResponse{
				Error: "field validation error",
				Fields: []web.FieldError{
					{Field: "email", Error: "Key: 'UserCreateRequest.email' Error:Field validation for 'email' failed on the 'email' tag"},
				},
			}
		} else {
			expectedStatus = http.StatusForbidden
			expectedErr = web.ErrorResponse{
				Error: mid.ErrForbidden.Error(),
			}
		}

		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d w/invalid data", rn, expectedStatus),
			http.MethodPost,
			"/v1/users",
			user.UserCreateRequest{
				Name:            "Lee Brown",
				Email:           "invalid email address",
				Password:        "akTechFr0n!ier",
				PasswordConfirm: "akTechFr0n!ier",
			},
			tr.Token,
			tr.Claims,
			expectedStatus,
			expectedErr,
			func(treq requestTest, body []byte) bool {
				return true
			},
		})
	}

	runRequestTests(t, rtests)
}

// patchUser validates update user by ID endpoint.
func patchUser(t *testing.T) {

	var rtests []requestTest

	// Test update a user
	// 	Admin role: 204
	//  User role 204 - user ID matches claims so OK
	for rn, tr := range roleTests {
		expectedStatus := http.StatusNoContent
		newName := rn + uuid.NewRandom().String() + strconv.Itoa(len(rtests))
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d", rn, expectedStatus),
			http.MethodPatch,
			"/v1/users",
			user.UserUpdateRequest{
				ID:   tr.SignupResult.User.ID,
				Name: &newName,
			},
			tr.Token,
			tr.Claims,
			expectedStatus,
			nil,
			func(treq requestTest, body []byte) bool {
				return true
			},
		})
	}

	// Test update a user with invalid data.
	// 	Admin role: 400
	//  User role 400
	for rn, tr := range roleTests {
		expectedStatus := http.StatusBadRequest
		expectedErr := web.ErrorResponse{
			Error: "field validation error",
			Fields: []web.FieldError{
				{Field: "email", Error: "Key: 'UserUpdateRequest.email' Error:Field validation for 'email' failed on the 'email' tag"},
			},
		}

		invalidEmail :=  "invalid email address"
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d w/invalid data", rn, expectedStatus),
			http.MethodPatch,
			"/v1/users",
			user.UserUpdateRequest{
				ID:   tr.SignupResult.User.ID,
				Email:          &invalidEmail,
			},
			tr.Token,
			tr.Claims,
			expectedStatus,
			expectedErr,
			func(treq requestTest, body []byte) bool {
				return true
			},
		})
	}

	// Test update a user for with an invalid ID.
	// 	Admin role: 403
	//  User role 403
	for rn, tr := range roleTests {

		expectedStatus := http.StatusForbidden
		expectedErr := web.ErrorResponse{
			Error: user.ErrForbidden.Error(),
		}

		newName := rn + uuid.NewRandom().String() + strconv.Itoa(len(rtests))
		invalidID := uuid.NewRandom().String()
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d w/invalid ID", rn, expectedStatus),
			http.MethodPatch,
			"/v1/users",
			user.UserUpdateRequest{
				ID:   invalidID,
				Name: &newName,
			},
			tr.Token,
			tr.Claims,
			expectedStatus,
			expectedErr,
			func(treq requestTest, body []byte) bool {
				return true
			},
		})
	}

	// Test update a user for with random user ID.
	// 	Admin role: 403
	//  User role 403
	forbiddenUser := mockUser()
	for rn, tr := range roleTests {

		expectedStatus := http.StatusForbidden
		expectedErr := web.ErrorResponse{
			Error: user.ErrForbidden.Error(),
		}

		newName := rn+uuid.NewRandom().String()+strconv.Itoa(len(rtests))
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d w/random user ID", rn, expectedStatus),
			http.MethodPatch,
			"/v1/users",
			user.UserUpdateRequest{
				ID: forbiddenUser.ID,
				Name: &newName,
			},
			tr.Token,
			tr.Claims,
			expectedStatus,
			expectedErr,
			func(treq requestTest, body []byte) bool {
				return true
			},
		})
	}

	runRequestTests(t, rtests)
}

// patchUserPassword validates update user password by ID endpoint.
func patchUserPassword(t *testing.T) {

	var rtests []requestTest

	// Test update a user
	// 	Admin role: 204
	//  User role 204 - user ID matches claims so OK
	for rn, tr := range roleTests {
		expectedStatus := http.StatusNoContent
		newPass := uuid.NewRandom().String()
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d", rn, expectedStatus),
			http.MethodPatch,
			"/v1/users/password",
			user.UserUpdatePasswordRequest{
				ID:   tr.SignupResult.User.ID,
				Password: newPass,
				PasswordConfirm: newPass,
			},
			tr.Token,
			tr.Claims,
			expectedStatus,
			nil,
			func(treq requestTest, body []byte) bool {
				return true
			},
		})
	}

	// Test update a user password with invalid data.
	// 	Admin role: 400
	//  User role 400
	for rn, tr := range roleTests {
		expectedStatus := http.StatusBadRequest
		expectedErr := web.ErrorResponse{
			Error: "field validation error",
			Fields: []web.FieldError{
				{Field: "password_confirm", Error: "Key: 'UserUpdatePasswordRequest.password_confirm' Error:Field validation for 'password_confirm' failed on the 'eqfield' tag"},
			},
		}

		newPass := uuid.NewRandom().String()
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d w/invalid data", rn, expectedStatus),
			http.MethodPatch,
			"/v1/users/password",
			user.UserUpdatePasswordRequest{
				ID:   tr.SignupResult.User.ID,
				Password: newPass,
				PasswordConfirm: "different",
			},
			tr.Token,
			tr.Claims,
			expectedStatus,
			expectedErr,
			func(treq requestTest, body []byte) bool {
				return true
			},
		})
	}

	// Test update a user password for with an invalid ID.
	// 	Admin role: 403
	//  User role 403
	for rn, tr := range roleTests {

		expectedStatus := http.StatusForbidden
		expectedErr := web.ErrorResponse{
			Error: user.ErrForbidden.Error(),
		}

		newPass := uuid.NewRandom().String()
		invalidID := uuid.NewRandom().String()
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d w/invalid ID", rn, expectedStatus),
			http.MethodPatch,
			"/v1/users/password",
			user.UserUpdatePasswordRequest{
				ID:   invalidID,
				Password: newPass,
				PasswordConfirm: newPass,
			},
			tr.Token,
			tr.Claims,
			expectedStatus,
			expectedErr,
			func(treq requestTest, body []byte) bool {
				return true
			},
		})
	}

	// Test update a user password for with random user ID.
	// 	Admin role: 403
	//  User role 403
	forbiddenUser := mockUser()
	for rn, tr := range roleTests {

		expectedStatus := http.StatusForbidden
		expectedErr := web.ErrorResponse{
			Error: user.ErrForbidden.Error(),
		}

		newPass := uuid.NewRandom().String()
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d w/random user ID", rn, expectedStatus),
			http.MethodPatch,
			"/v1/users/password",
			user.UserUpdatePasswordRequest{
				ID: forbiddenUser.ID,
				Password: newPass,
				PasswordConfirm: newPass,
			},
			tr.Token,
			tr.Claims,
			expectedStatus,
			expectedErr,
			func(treq requestTest, body []byte) bool {
				return true
			},
		})
	}

	runRequestTests(t, rtests)
}

