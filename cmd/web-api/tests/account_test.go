package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/mid"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"github.com/pborman/uuid"
)

// TestAccountCRUDAdmin tests all the account CRUD endpoints using an user with role admin.
func TestAccountCRUDAdmin(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	s := newMockSignup()
	tr.Account = s.account
	tr.User = s.user
	tr.Token = s.token
	tr.Claims = s.claims
	ctx := s.context

	// Test create.
	{
		expectedStatus := http.StatusMethodNotAllowed

		req := mockUserCreateRequest()
		rt := requestTest{
			fmt.Sprintf("Create %d w/role %s", expectedStatus, tr.Role),
			http.MethodPost,
			"/v1/accounts",
			req,
			tr.Token,
			tr.Claims,
			expectedStatus,
			nil,
		}
		t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

		w, ok := executeRequestTest(t, rt, ctx)
		if !ok {
			t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
		}
		t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

		if len(w.Body.String()) != 0 {
			if diff := cmpDiff(t, w.Body.Bytes(), nil); diff {
				t.Fatalf("\t%s\tReceived expected empty.", tests.Failed)
			}
		}
		t.Logf("\t%s\tReceived expected empty.", tests.Success)
	}

	// Test read.
	{
		expectedStatus := http.StatusOK

		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/accounts/%s", tr.Account.ID),
			nil,
			tr.Token,
			tr.Claims,
			expectedStatus,
			nil,
		}
		t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

		w, ok := executeRequestTest(t, rt, ctx)
		if !ok {
			t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
		}
		t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

		var actual account.AccountResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expectedMap := map[string]interface{}{
			"updated_at":      web.NewTimeResponse(ctx, tr.Account.UpdatedAt),
			"id":              tr.Account.ID,
			"address2":        tr.Account.Address2,
			"region":          tr.Account.Region,
			"zipcode":         tr.Account.Zipcode,
			"timezone":        tr.Account.Timezone,
			"created_at":      web.NewTimeResponse(ctx, tr.Account.CreatedAt),
			"country":         tr.Account.Country,
			"billing_user_id": &tr.Account.BillingUserID.String,
			"name":            tr.Account.Name,
			"address1":        tr.Account.Address1,
			"city":            tr.Account.City,
			"status": map[string]interface{}{
				"value":   "active",
				"title":   "Active",
				"options": []map[string]interface{}{{"selected": false, "title": "[Active Pending Disabled]", "value": "[active pending disabled]"}},
			},
			"signup_user_id": &tr.Account.SignupUserID.String,
		}

		var expected account.AccountResponse
		if err := decodeMapToStruct(expectedMap, &expected); err != nil {
			t.Logf("\t\tGot error : %+v\nActual results to format expected : \n", err)
			printResultMap(ctx, w.Body.Bytes()) // used to help format expectedMap
			t.Fatalf("\t%s\tDecode expected failed.", tests.Failed)
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected result.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected result.", tests.Success)
	}

	// Test Read with random ID.
	{
		expectedStatus := http.StatusNotFound

		randID := uuid.NewRandom().String()
		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s using random ID", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/accounts/%s", randID),
			nil,
			tr.Token,
			tr.Claims,
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
			Error: fmt.Sprintf("account %s not found: Entity not found", randID),
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test Read with forbidden ID.
	{
		expectedStatus := http.StatusNotFound

		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s using forbidden ID", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/accounts/%s", tr.ForbiddenAccount.ID),
			nil,
			tr.Token,
			tr.Claims,
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
			Error: fmt.Sprintf("account %s not found: Entity not found", tr.ForbiddenAccount.ID),
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test update.
	{
		expectedStatus := http.StatusNoContent

		newName := uuid.NewRandom().String()
		rt := requestTest{
			fmt.Sprintf("Update %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/accounts",
			account.AccountUpdateRequest{
				ID:   tr.Account.ID,
				Name: &newName,
			},
			tr.Token,
			tr.Claims,
			expectedStatus,
			nil,
		}
		t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

		w, ok := executeRequestTest(t, rt, ctx)
		if !ok {
			t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
		}
		t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

		if len(w.Body.String()) != 0 {
			if diff := cmpDiff(t, w.Body.Bytes(), nil); diff {
				t.Fatalf("\t%s\tReceived expected empty.", tests.Failed)
			}
		}
		t.Logf("\t%s\tReceived expected empty.", tests.Success)
	}
}

// TestAccountCRUDUser tests all the account CRUD endpoints using an user with role user.
func TestAccountCRUDUser(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleUser]

	// Add claims to the context for the user.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test create.
	{
		expectedStatus := http.StatusMethodNotAllowed

		req := mockUserCreateRequest()
		rt := requestTest{
			fmt.Sprintf("Create %d w/role %s", expectedStatus, tr.Role),
			http.MethodPost,
			"/v1/accounts",
			req,
			tr.Token,
			tr.Claims,
			expectedStatus,
			nil,
		}
		t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

		w, ok := executeRequestTest(t, rt, ctx)
		if !ok {
			t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
		}
		t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

		if len(w.Body.String()) != 0 {
			if diff := cmpDiff(t, w.Body.Bytes(), nil); diff {
				t.Fatalf("\t%s\tReceived expected empty.", tests.Failed)
			}
		}
		t.Logf("\t%s\tReceived expected empty.", tests.Success)
	}

	// Test read.
	{
		expectedStatus := http.StatusOK

		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/accounts/%s", tr.Account.ID),
			nil,
			tr.Token,
			tr.Claims,
			expectedStatus,
			nil,
		}
		t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

		w, ok := executeRequestTest(t, rt, ctx)
		if !ok {
			t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
		}
		t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

		var actual account.AccountResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expectedMap := map[string]interface{}{
			"updated_at":      web.NewTimeResponse(ctx, tr.Account.UpdatedAt),
			"id":              tr.Account.ID,
			"address2":        tr.Account.Address2,
			"region":          tr.Account.Region,
			"zipcode":         tr.Account.Zipcode,
			"timezone":        tr.Account.Timezone,
			"created_at":      web.NewTimeResponse(ctx, tr.Account.CreatedAt),
			"country":         tr.Account.Country,
			"billing_user_id": &tr.Account.BillingUserID.String,
			"name":            tr.Account.Name,
			"address1":        tr.Account.Address1,
			"city":            tr.Account.City,
			"status": map[string]interface{}{
				"value":   "active",
				"title":   "Active",
				"options": []map[string]interface{}{{"selected": false, "title": "[Active Pending Disabled]", "value": "[active pending disabled]"}},
			},
			"signup_user_id": &tr.Account.SignupUserID.String,
		}

		var expected account.AccountResponse
		if err := decodeMapToStruct(expectedMap, &expected); err != nil {
			t.Logf("\t\tGot error : %+v\nActual results to format expected : \n", err)
			printResultMap(ctx, w.Body.Bytes()) // used to help format expectedMap
			t.Fatalf("\t%s\tDecode expected failed.", tests.Failed)
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected result.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected result.", tests.Success)
	}

	// Test Read with random ID.
	{
		expectedStatus := http.StatusNotFound

		randID := uuid.NewRandom().String()
		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s using random ID", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/accounts/%s", randID),
			nil,
			tr.Token,
			tr.Claims,
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
			Error: fmt.Sprintf("account %s not found: Entity not found", randID),
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test Read with forbidden ID.
	{
		expectedStatus := http.StatusNotFound

		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s using forbidden ID", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/accounts/%s", tr.ForbiddenAccount.ID),
			nil,
			tr.Token,
			tr.Claims,
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
			Error: fmt.Sprintf("account %s not found: Entity not found", tr.ForbiddenAccount.ID),
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test update.
	{
		expectedStatus := http.StatusForbidden

		newName := uuid.NewRandom().String()
		rt := requestTest{
			fmt.Sprintf("Update %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/accounts",
			account.AccountUpdateRequest{
				ID:   tr.Account.ID,
				Name: &newName,
			},
			tr.Token,
			tr.Claims,
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
			Error: mid.ErrForbidden.Error(),
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestAccountUpdate validates update account by ID endpoint.
func TestAccountUpdate(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the user.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test create with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		invalidStatus := account.AccountStatus("invalid status")
		rt := requestTest{
			fmt.Sprintf("Update %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/accounts",
			account.AccountUpdateRequest{
				ID:     tr.Account.ID,
				Status: &invalidStatus,
			},
			tr.Token,
			tr.Claims,
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
				{Field: "status", Error: "Key: 'AccountUpdateRequest.status' Error:Field validation for 'status' failed on the 'oneof' tag"},
			},
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}
