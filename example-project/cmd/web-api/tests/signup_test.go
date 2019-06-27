package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/signup"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user"
	"github.com/pborman/uuid"
)

type mockSignup struct {
	account *account.Account
	user    mockUser
	token   user.Token
	claims  auth.Claims
	context context.Context
}

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

func newMockSignup() mockSignup {
	req := mockSignupRequest()
	now := time.Now().UTC().AddDate(-1, -1, -1)
	s, err := signup.Signup(tests.Context(), auth.Claims{}, test.MasterDB, req, now)
	if err != nil {
		panic(err)
	}

	expires := time.Now().UTC().Sub(s.User.CreatedAt) + time.Hour
	tkn, err := user.Authenticate(tests.Context(), test.MasterDB, authenticator, req.User.Email, req.User.Password, expires, now)
	if err != nil {
		panic(err)
	}

	claims, err := authenticator.ParseClaims(tkn.AccessToken)
	if err != nil {
		panic(err)
	}

	// Add claims to the context for the user.
	ctx := context.WithValue(tests.Context(), auth.Key, claims)

	return mockSignup{
		account: s.Account,
		user:    mockUser{s.User, req.User.Password},
		claims:  claims,
		token:   tkn,
		context: ctx,
	}
}

// TestSignup validates the signup endpoint.
func TestSignup(t *testing.T) {
	defer tests.Recover(t)

	ctx := tests.Context()

	// Test signup.
	{
		expectedStatus := http.StatusCreated

		req := mockSignupRequest()
		rt := requestTest{
			fmt.Sprintf("Signup %d w/no authorization", expectedStatus),
			http.MethodPost,
			"/v1/signup",
			req,
			user.Token{},
			auth.Claims{},
			expectedStatus,
			nil,
		}
		t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

		w, ok := executeRequestTest(t, rt, ctx)
		if !ok {
			t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
		}
		t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

		var actual signup.SignupResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expectedMap := map[string]interface{}{
			"user": map[string]interface{}{
				"id":         actual.User.ID,
				"name":       req.User.Name,
				"email":      req.User.Email,
				"timezone":   actual.User.Timezone,
				"created_at": web.NewTimeResponse(ctx, actual.User.CreatedAt.Value),
				"updated_at": web.NewTimeResponse(ctx, actual.User.UpdatedAt.Value),
			},
			"account": map[string]interface{}{
				"updated_at":      web.NewTimeResponse(ctx, actual.Account.UpdatedAt.Value),
				"id":              actual.Account.ID,
				"address2":        req.Account.Address2,
				"region":          req.Account.Region,
				"zipcode":         req.Account.Zipcode,
				"timezone":        actual.Account.Timezone,
				"created_at":      web.NewTimeResponse(ctx, actual.Account.CreatedAt.Value),
				"country":         req.Account.Country,
				"billing_user_id": &actual.Account.BillingUserID,
				"name":            req.Account.Name,
				"address1":        req.Account.Address1,
				"city":            req.Account.City,
				"status": map[string]interface{}{
					"value":   "active",
					"title":   "Active",
					"options": []map[string]interface{}{{"selected": false, "title": "[Active Pending Disabled]", "value": "[active pending disabled]"}},
				},
				"signup_user_id": &actual.Account.SignupUserID,
			},
		}

		var expected signup.SignupResponse
		if err := decodeMapToStruct(expectedMap, &expected); err != nil {
			t.Logf("\t\tGot error : %+v\nActual results to format expected : \n", err)
			printResultMap(ctx, w.Body.Bytes()) // used to help format expectedMap
			t.Fatalf("\t%s\tDecode expected failed.", tests.Failed)
		}

		if diff := cmpDiff(t, actual, expected); diff {
			if len(expectedMap) == 0 {
				printResultMap(ctx, w.Body.Bytes()) // used to help format expectedMap
			}
			t.Fatalf("\t%s\tReceived expected result.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected result.", tests.Success)
	}

	// Test signup w/empty request.
	{
		expectedStatus := http.StatusBadRequest

		rt := requestTest{
			fmt.Sprintf("Signup %d w/empty request", expectedStatus),
			http.MethodPost,
			"/v1/signup",
			nil,
			user.Token{},
			auth.Claims{},
			expectedStatus,
			nil,
		}
		t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

		w, ok := executeRequestTest(t, rt, ctx)
		if !ok {
			t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
		}
		t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

		var actual web.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := web.ErrorResponse{
			Error: "decode request body failed: EOF",
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test signup w/validation errors.
	{
		expectedStatus := http.StatusBadRequest

		req := mockSignupRequest()
		req.User.Email = ""
		req.Account.Name = ""
		rt := requestTest{
			fmt.Sprintf("Signup %d w/validation errors", expectedStatus),
			http.MethodPost,
			"/v1/signup",
			req,
			user.Token{},
			auth.Claims{},
			expectedStatus,
			nil,
		}
		t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

		w, ok := executeRequestTest(t, rt, ctx)
		if !ok {
			t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
		}
		t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

		var actual web.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := web.ErrorResponse{
			Error: "field validation error",
			Fields: []web.FieldError{
				{Field: "name", Error: "Key: 'SignupRequest.account.name' Error:Field validation for 'name' failed on the 'required' tag"},
				{Field: "email", Error: "Key: 'SignupRequest.user.email' Error:Field validation for 'email' failed on the 'required' tag"},
			},
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}
