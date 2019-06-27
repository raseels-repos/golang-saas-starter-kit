package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/signup"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user"
	"github.com/google/go-cmp/cmp"
	"github.com/pborman/uuid"
)

func mockSignupRequest() signup.SignupRequest {
	return signup.SignupRequest{
		Account: signup.SignupAccount{
			Name:     uuid.NewRandom().String(),
			Address1: "103 East Main St",
			Address2: "Unit 546",
			City:     "Valdez",
			Region:   "AK",
			Country:  "USA",
			Zipcode:  "99686",
		},
		User: signup.SignupUser{
			Name:            "Lee Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
	}
}

// TestSignup is the entry point for the signup
func TestSignup(t *testing.T) {
	defer tests.Recover(t)

	t.Run("postSigup", postSigup)
}

// postSigup validates the signup endpoint.
func postSigup(t *testing.T) {

	var rtests []requestTest

	// Test 201.
	// Signup does not require auth, so empty token and claims should result in success.
	req1 := mockSignupRequest()
	rtests = append(rtests, requestTest{
		"No Authorization Valid",
		http.MethodPost,
		"/v1/signup",
		req1,
		user.Token{},
		auth.Claims{},
		http.StatusCreated,
		nil,
		func(treq requestTest, body []byte) bool {
			var actual signup.SignupResponse
			if err := json.Unmarshal(body, &actual); err != nil {
				t.Logf("\t\tGot error : %+v", err)
				return false
			}

			ctx := tests.Context()

			req := treq.request.(signup.SignupRequest )

			expectedMap := map[string]interface{}{
				"user": map[string]interface{}{
					"id": actual.User.ID,
					"name": req.User.Name,
					"email": req.User.Email,
					"timezone": actual.User.Timezone,
					"created_at": web.NewTimeResponse(ctx, actual.User.CreatedAt.Value),
					"updated_at": web.NewTimeResponse(ctx, actual.User.UpdatedAt.Value),
				},
				"account":  map[string]interface{}{
					"updated_at": web.NewTimeResponse(ctx, actual.Account.UpdatedAt.Value),
					"id": actual.Account.ID,
					"address2": req.Account.Address2,
					"region": req.Account.Region,
					"zipcode": req.Account.Zipcode,
					"timezone": actual.Account.Timezone,
					"created_at": web.NewTimeResponse(ctx, actual.Account.CreatedAt.Value),
					"country": req.Account.Country,
					"billing_user_id": &actual.Account.BillingUserID,
					"name": req.Account.Name,
					"address1": req.Account.Address1,
					"city": req.Account.City,
					"status": map[string]interface{}{
						"value": "active",
						"title": "Active",
						"options": []map[string]interface{}{{"selected":false,"title":"[Active Pending Disabled]","value":"[active pending disabled]"}},
					},
					"signup_user_id": &actual.Account.SignupUserID,
				},
			}
			expectedJson, err := json.Marshal(expectedMap)
			if err != nil {
				t.Logf("\t\tGot error : %+v", err)
				return false
			}

			var expected signup.SignupResponse
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

	// Test 404 w/empty request.
	rtests = append(rtests, requestTest{
		"Empty request",
		http.MethodPost,
		"/v1/signup",
		nil,
		user.Token{},
		auth.Claims{},
		http.StatusBadRequest,
		web.ErrorResponse{
			Error: "decode request body failed: EOF",
		},
		func(req requestTest, body []byte) bool {
			return true
		},
	})

	// Test 404 w/validation errors.
	invalidReq := mockSignupRequest()
	invalidReq.User.Email = ""
	invalidReq.Account.Name = ""
	rtests = append(rtests, requestTest{
		"Invalid request",
		http.MethodPost,
		"/v1/signup",
		invalidReq,
		user.Token{},
		auth.Claims{},
		http.StatusBadRequest,
		web.ErrorResponse{
			Error: "field validation error",
			Fields: []web.FieldError{
				{Field: "name", Error: "Key: 'SignupRequest.account.name' Error:Field validation for 'name' failed on the 'required' tag"},
				{Field: "email", Error: "Key: 'SignupRequest.user.email' Error:Field validation for 'email' failed on the 'required' tag"},
			},
		},
		func(req requestTest, body []byte) bool {
			return true
		},
	})

	runRequestTests(t, rtests)
}
