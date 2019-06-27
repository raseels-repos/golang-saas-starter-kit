package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/mid"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"github.com/google/go-cmp/cmp"
	"github.com/pborman/uuid"
)

func mockAccount() *account.Account {
	req := account.AccountCreateRequest{
		Name:     uuid.NewRandom().String(),
		Address1: "103 East Main St",
		Address2: "Unit 546",
		City:     "Valdez",
		Region:   "AK",
		Country:  "USA",
		Zipcode:  "99686",
	}

	a, err := account.Create(tests.Context(), auth.Claims{}, test.MasterDB, req, time.Now().UTC().AddDate(-1, -1, -1))
	if err != nil {
		panic(err)
	}
	return a
}

// TestAccount is the entry point for the account endpoints.
func TestAccount(t *testing.T) {
	defer tests.Recover(t)

	t.Run("getAccount", getAccount)
	t.Run("patchAccount", patchAccount)
}

// getAccount validates get account by ID endpoint.
func getAccount(t *testing.T) {

	var rtests []requestTest

	forbiddenAccount := mockAccount()

	// Both roles should be able to read the account.
	for rn, tr := range roleTests {
		acc := tr.SignupResult.Account

		// Test 200.
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s 200", rn),
			http.MethodGet,
			fmt.Sprintf("/v1/accounts/%s", acc.ID),
			nil,
			tr.Token,
			tr.Claims,
			http.StatusOK,
			nil,
			func(treq requestTest, body []byte) bool {
				var actual account.AccountResponse
				if err := json.Unmarshal(body, &actual); err != nil {
					t.Logf("\t\tGot error : %+v", err)
					return false
				}

				// Add claims to the context so they can be retrieved later.
				ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

				expectedMap := map[string]interface{}{
					"updated_at":      web.NewTimeResponse(ctx, acc.UpdatedAt),
					"id":              acc.ID,
					"address2":        acc.Address2,
					"region":          acc.Region,
					"zipcode":         acc.Zipcode,
					"timezone":        acc.Timezone,
					"created_at":      web.NewTimeResponse(ctx, acc.CreatedAt),
					"country":         acc.Country,
					"billing_user_id": &acc.BillingUserID,
					"name":            acc.Name,
					"address1":        acc.Address1,
					"city":            acc.City,
					"status": map[string]interface{}{
						"value":   "active",
						"title":   "Active",
						"options": []map[string]interface{}{{"selected": false, "title": "[Active Pending Disabled]", "value": "[active pending disabled]"}},
					},
					"signup_user_id": &acc.SignupUserID,
				}
				expectedJson, err := json.Marshal(expectedMap)
				if err != nil {
					t.Logf("\t\tGot error : %+v", err)
					return false
				}

				var expected account.AccountResponse
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
			fmt.Sprintf("/v1/accounts/%s", invalidID),
			nil,
			tr.Token,
			tr.Claims,
			http.StatusNotFound,
			web.ErrorResponse{
				Error: fmt.Sprintf("account %s not found: Entity not found", invalidID),
			},
			func(treq requestTest, body []byte) bool {
				return true
			},
		})

		// Test 404 - Account exists but not allowed.
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s 404 w/random account ID", rn),
			http.MethodGet,
			fmt.Sprintf("/v1/accounts/%s", forbiddenAccount.ID),
			nil,
			tr.Token,
			tr.Claims,
			http.StatusNotFound,
			web.ErrorResponse{
				Error: fmt.Sprintf("account %s not found: Entity not found", forbiddenAccount.ID),
			},
			func(treq requestTest, body []byte) bool {
				return true
			},
		})
	}

	runRequestTests(t, rtests)
}

// patchAccount validates update account by ID endpoint.
func patchAccount(t *testing.T) {

	var rtests []requestTest

	// Test update an account
	// 	Admin role: 204
	//  User role 403
	for rn, tr := range roleTests {
		var expectedStatus int
		var expectedErr interface{}

		// Test 204.
		if rn == auth.RoleAdmin {
			expectedStatus = http.StatusNoContent
		} else {
			expectedStatus = http.StatusForbidden
			expectedErr = web.ErrorResponse{
				Error: mid.ErrForbidden.Error(),
			}
		}

		newName := rn + uuid.NewRandom().String() + strconv.Itoa(len(rtests))
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d", rn, expectedStatus),
			http.MethodPatch,
			"/v1/accounts",
			account.AccountUpdateRequest{
				ID:   tr.SignupResult.Account.ID,
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

	// Test update an account with invalid data.
	// 	Admin role: 400
	//  User role 400
	for rn, tr := range roleTests {
		var expectedStatus int
		var expectedErr interface{}

		if rn == auth.RoleAdmin {
			expectedStatus = http.StatusBadRequest
			expectedErr = web.ErrorResponse{
				Error: "field validation error",
				Fields: []web.FieldError{
					{Field: "status", Error: "Key: 'AccountUpdateRequest.status' Error:Field validation for 'status' failed on the 'oneof' tag"},
				},
			}
		} else {
			expectedStatus = http.StatusForbidden
			expectedErr = web.ErrorResponse{
				Error: mid.ErrForbidden.Error(),
			}
		}

		invalidStatus := account.AccountStatus("invalid status")
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d w/invalid data", rn, expectedStatus),
			http.MethodPatch,
			"/v1/accounts",
			account.AccountUpdateRequest{
				ID:   tr.SignupResult.User.ID,
				Status:          &invalidStatus,
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

	// Test update an account for with an invalid ID.
	// 	Admin role: 403
	//  User role 403
	for rn, tr := range roleTests {
		var expectedStatus int
		var expectedErr interface{}

		// Test 403.
		if rn == auth.RoleAdmin {
			expectedStatus = http.StatusForbidden
			expectedErr = web.ErrorResponse{
				Error: account.ErrForbidden.Error(),
			}
		} else {
			expectedStatus = http.StatusForbidden
			expectedErr = web.ErrorResponse{
				Error: mid.ErrForbidden.Error(),
			}
		}
		newName := rn + uuid.NewRandom().String() + strconv.Itoa(len(rtests))
		invalidID := uuid.NewRandom().String()
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d w/invalid ID", rn, expectedStatus),
			http.MethodPatch,
			"/v1/accounts",
			account.AccountUpdateRequest{
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

	// Test update an account for with random account ID.
	// 	Admin role: 403
	//  User role 403
	forbiddenAccount := mockAccount()
	for rn, tr := range roleTests {
		var expectedStatus int
		var expectedErr interface{}

		// Test 403.
		if rn == auth.RoleAdmin {
			expectedStatus = http.StatusForbidden
			expectedErr = web.ErrorResponse{
				Error: account.ErrForbidden.Error(),
			}
		} else {
			expectedStatus = http.StatusForbidden
			expectedErr = web.ErrorResponse{
				Error: mid.ErrForbidden.Error(),
			}
		}
		newName := rn+uuid.NewRandom().String()+strconv.Itoa(len(rtests))
		rtests = append(rtests, requestTest{
			fmt.Sprintf("Role %s %d w/random account ID", rn, expectedStatus),
			http.MethodPatch,
			"/v1/accounts",
			account.AccountUpdateRequest{
				ID: forbiddenAccount.ID,
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
