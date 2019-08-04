package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/mid"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"github.com/pborman/uuid"
)

// newMockUserAccount creates a new user user for testing and associates it with the supplied account ID.
func newMockUserAccount(accountID string, role user_account.UserAccountRole) *user_account.UserAccount {
	req := mockUserCreateRequest()
	u, err := user.Create(tests.Context(), auth.Claims{}, test.MasterDB, req, time.Now().UTC().AddDate(-1, -1, -1))
	if err != nil {
		panic(err)
	}

	ua, err := user_account.Create(tests.Context(), auth.Claims{}, test.MasterDB, user_account.UserAccountCreateRequest{
		UserID:    u.ID,
		AccountID: accountID,
		Roles:     []user_account.UserAccountRole{role},
	}, time.Now().UTC().AddDate(-1, -1, -1))
	if err != nil {
		panic(err)
	}

	return ua
}

// TestUserAccountCRUDAdmin tests all the user account CRUD endpoints using an user with role admin.
func TestUserAccountCRUDAdmin(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the user_account.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test create.
	var created user_account.UserAccountResponse
	{
		expectedStatus := http.StatusCreated

		rt := requestTest{
			fmt.Sprintf("Create %d w/role %s", expectedStatus, tr.Role),
			http.MethodPost,
			"/v1/user_accounts",
			nil,
			tr.Token,
			tr.Claims,
			expectedStatus,
			nil,
		}
		t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

		newUser, err := user.Create(tests.Context(), auth.Claims{}, test.MasterDB, mockUserCreateRequest(), time.Now().UTC().AddDate(-1, -1, -1))
		if err != nil {
			t.Fatalf("\t%s\tCreate new user failed.", tests.Failed)
		}
		req := user_account.UserAccountCreateRequest{
			UserID:    newUser.ID,
			AccountID: tr.Account.ID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole_User},
		}
		rt.request = req

		w, ok := executeRequestTest(t, rt, ctx)
		if !ok {
			t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
		}
		t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

		var actual user_account.UserAccountResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}
		created = actual

		expectedMap := map[string]interface{}{
			"updated_at": web.NewTimeResponse(ctx, actual.UpdatedAt.Value),
			//"id":         actual.ID,
			"account_id": req.AccountID,
			"user_id":    req.UserID,
			"status":     web.NewEnumResponse(ctx, "active", user_account.UserAccountStatus_Values),
			"roles":      req.Roles,
			"created_at": web.NewTimeResponse(ctx, actual.CreatedAt.Value),
		}

		var expected user_account.UserAccountResponse
		if err := decodeMapToStruct(expectedMap, &expected); err != nil {
			t.Logf("\t\tGot error : %+v\nActual results to format expected : \n", err)
			printResultMap(ctx, w.Body.Bytes()) // used to help format expectedMap
			t.Fatalf("\t%s\tDecode expected failed.", tests.Failed)
		}

		if diff := cmpDiff(t, expected, actual); diff {
			if len(expectedMap) == 0 {
				printResultMap(ctx, w.Body.Bytes()) // used to help format expectedMap
			}
			t.Fatalf("\t%s\tReceived expected result.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected result.", tests.Success)
	}

	// Test read.
	{
		expectedStatus := http.StatusOK

		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/user_accounts/%s/%s", created.UserID, created.AccountID),
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

		var actual user_account.UserAccountResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		if diff := cmpDiff(t, actual, created); diff {
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
			fmt.Sprintf("/v1/user_accounts/%s/%s", randID, randID),
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := weberror.ErrorResponse{
			StatusCode: expectedStatus,
			Error:      http.StatusText(expectedStatus),
			Details:    fmt.Sprintf("entry for user %s account %s not found: Entity not found", randID, randID),
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test Read with forbidden ID.
	forbiddenUserAccount := newMockUserAccount(newMockSignup().account.ID, user_account.UserAccountRole_Admin)
	{
		expectedStatus := http.StatusNotFound

		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s using forbidden ID", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/user_accounts/%s/%s", forbiddenUserAccount.UserID, forbiddenUserAccount.AccountID),
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := weberror.ErrorResponse{
			StatusCode: expectedStatus,
			Error:      http.StatusText(expectedStatus),
			Details:    fmt.Sprintf("entry for user %s account %s not found: Entity not found", forbiddenUserAccount.UserID, forbiddenUserAccount.AccountID),
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test update.
	{
		expectedStatus := http.StatusNoContent

		newStatus := user_account.UserAccountStatus_Invited
		rt := requestTest{
			fmt.Sprintf("Update %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/user_accounts",
			user_account.UserAccountUpdateRequest{
				UserID:    created.UserID,
				AccountID: created.AccountID,
				Status:    &newStatus,
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

	// Test archive.
	{
		expectedStatus := http.StatusNoContent

		rt := requestTest{
			fmt.Sprintf("Archive %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/user_accounts/archive",
			user_account.UserAccountArchiveRequest{
				UserID:    created.UserID,
				AccountID: created.AccountID,
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

	// Test delete.
	{
		expectedStatus := http.StatusNoContent

		rt := requestTest{
			fmt.Sprintf("Delete %d w/role %s", expectedStatus, tr.Role),
			http.MethodDelete,
			"/v1/user_accounts",
			user_account.UserAccountDeleteRequest{
				UserID:    created.UserID,
				AccountID: created.AccountID,
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

// TestUserAccountCRUDUser tests all the user account CRUD endpoints using an user with role user_account.
func TestUserAccountCRUDUser(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleUser]

	// Add claims to the context for the user_account.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test create.
	{
		expectedStatus := http.StatusForbidden

		rt := requestTest{
			fmt.Sprintf("Create %d w/role %s", expectedStatus, tr.Role),
			http.MethodPost,
			"/v1/user_accounts",
			user_account.UserAccountCreateRequest{
				UserID:    uuid.NewRandom().String(),
				AccountID: tr.Account.ID,
				Roles:     []user_account.UserAccountRole{user_account.UserAccountRole_User},
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := mid.ErrorForbidden(ctx).(*weberror.Error).Response(ctx, false)
		expected.StackTrace = actual.StackTrace

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Since role doesn't support create, bypass auth to test other endpoints.
	created := newMockUserAccount(tr.Account.ID, user_account.UserAccountRole_User).Response(ctx)

	// Test read.
	{
		expectedStatus := http.StatusOK

		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/user_accounts/%s/%s", created.UserID, created.AccountID),
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

		var actual *user_account.UserAccountResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		if diff := cmpDiff(t, actual, created); diff {
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
			fmt.Sprintf("/v1/user_accounts/%s/%s", randID, randID),
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := weberror.ErrorResponse{
			StatusCode: expectedStatus,
			Error:      http.StatusText(expectedStatus),
			Details:    fmt.Sprintf("entry for user %s account %s not found: Entity not found", randID, randID),
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test Read with forbidden ID.
	forbiddenUserAccount := newMockUserAccount(newMockSignup().account.ID, user_account.UserAccountRole_Admin)
	{
		expectedStatus := http.StatusNotFound

		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s using forbidden ID", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/user_accounts/%s/%s", forbiddenUserAccount.UserID, forbiddenUserAccount.AccountID),
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := weberror.ErrorResponse{
			StatusCode: expectedStatus,
			Error:      http.StatusText(expectedStatus),
			Details:    fmt.Sprintf("entry for user %s account %s not found: Entity not found", forbiddenUserAccount.UserID, forbiddenUserAccount.AccountID),
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test update.
	{
		expectedStatus := http.StatusForbidden

		newStatus := user_account.UserAccountStatus_Invited
		rt := requestTest{
			fmt.Sprintf("Update %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/user_accounts",
			user_account.UserAccountUpdateRequest{
				UserID:    created.UserID,
				AccountID: created.AccountID,
				Status:    &newStatus,
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := weberror.ErrorResponse{
			StatusCode: expectedStatus,
			Error:      http.StatusText(expectedStatus),
			Details:    account.ErrForbidden.Error(),
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test archive.
	{
		expectedStatus := http.StatusForbidden

		rt := requestTest{
			fmt.Sprintf("Archive %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/user_accounts/archive",
			user_account.UserAccountArchiveRequest{
				UserID:    created.UserID,
				AccountID: created.AccountID,
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := mid.ErrorForbidden(ctx).(*weberror.Error).Response(ctx, false)
		expected.StackTrace = actual.StackTrace

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test delete.
	{
		expectedStatus := http.StatusForbidden

		rt := requestTest{
			fmt.Sprintf("Delete %d w/role %s", expectedStatus, tr.Role),
			http.MethodDelete,
			"/v1/user_accounts",
			user_account.UserAccountArchiveRequest{
				UserID:    created.UserID,
				AccountID: created.AccountID,
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := mid.ErrorForbidden(ctx).(*weberror.Error).Response(ctx, false)
		expected.StackTrace = actual.StackTrace

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestUserAccountCreate validates create user account endpoint.
func TestUserAccountCreate(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the user_account.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test create with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		invalidStatus := user_account.UserAccountStatus("invalid status")
		rt := requestTest{
			fmt.Sprintf("Create %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodPost,
			"/v1/user_accounts",
			user_account.UserAccountCreateRequest{
				UserID:    uuid.NewRandom().String(),
				AccountID: tr.Account.ID,
				Roles:     []user_account.UserAccountRole{user_account.UserAccountRole_User},
				Status:    &invalidStatus,
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := weberror.ErrorResponse{
			StatusCode: expectedStatus,
			Error:      "Field validation error",
			Fields: []weberror.FieldError{
				//{Field: "status", Error: "Key: 'UserAccountCreateRequest.status' Error:Field validation for 'status' failed on the 'oneof' tag"},
				{
					Field:   "status",
					Value:   invalidStatus.String(),
					Tag:     "oneof",
					Error:   "status must be one of [active invited disabled]",
					Display: "status must be one of [active invited disabled]",
				},
			},
			Details:    actual.Details,
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestUserAccountUpdate validates update user account endpoint.
func TestUserAccountUpdate(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the user_account.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test update with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		invalidStatus := user_account.UserAccountStatus("invalid status")
		rt := requestTest{
			fmt.Sprintf("Update %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/user_accounts",
			user_account.UserAccountUpdateRequest{
				UserID:    uuid.NewRandom().String(),
				AccountID: uuid.NewRandom().String(),
				Status:    &invalidStatus,
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := weberror.ErrorResponse{
			StatusCode: expectedStatus,
			Error:      "Field validation error",
			Fields: []weberror.FieldError{
				//{Field: "status", Error: "Key: 'UserAccountUpdateRequest.status' Error:Field validation for 'status' failed on the 'oneof' tag"},
				{
					Field:   "status",
					Value:   invalidStatus.String(),
					Tag:     "oneof",
					Error:   "status must be one of [active invited disabled]",
					Display: "status must be one of [active invited disabled]",
				},
			},
			Details:    actual.Details,
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestUserAccountArchive validates archive user account endpoint.
func TestUserAccountArchive(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the user_account.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test archive with invalid data.
	{
		expectedStatus := http.StatusBadRequest
		req := user_account.UserAccountArchiveRequest{
			UserID:    "foo",
			AccountID: "bar",
		}
		rt := requestTest{
			fmt.Sprintf("Archive %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/user_accounts/archive",
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := weberror.ErrorResponse{
			StatusCode: expectedStatus,
			Error:      "Field validation error",
			Fields: []weberror.FieldError{
				//{Field: "user_id", Error: "Key: 'UserAccountArchiveRequest.user_id' Error:Field validation for 'user_id' failed on the 'uuid' tag"},
				//{Field: "account_id", Error: "Key: 'UserAccountArchiveRequest.account_id' Error:Field validation for 'account_id' failed on the 'uuid' tag"},
				{
					Field:   "user_id",
					Value:   req.UserID,
					Tag:     "uuid",
					Error:   "user_id must be a valid UUID",
					Display: "user_id must be a valid UUID",
				},
				{
					Field:   "account_id",
					Value:   req.AccountID,
					Tag:     "uuid",
					Error:   "account_id must be a valid UUID",
					Display: "account_id must be a valid UUID",
				},
			},
			Details:    actual.Details,
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test archive with forbidden ID.
	forbiddenUserAccount := newMockUserAccount(newMockSignup().account.ID, user_account.UserAccountRole_Admin)
	{
		expectedStatus := http.StatusForbidden

		rt := requestTest{
			fmt.Sprintf("Archive %d w/role %s using forbidden IDs", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/user_accounts/archive",
			user_account.UserAccountArchiveRequest{
				UserID:    forbiddenUserAccount.UserID,
				AccountID: forbiddenUserAccount.AccountID,
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := weberror.ErrorResponse{
			StatusCode: expectedStatus,
			Error:      http.StatusText(expectedStatus),
			Details:    user_account.ErrForbidden.Error(),
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestUserAccountDelete validates delete user account endpoint.
func TestUserAccountDelete(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the user_account.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test delete with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		req := user_account.UserAccountDeleteRequest{
			UserID:    "foo",
			AccountID: "bar",
		}

		rt := requestTest{
			fmt.Sprintf("Delete %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodDelete,
			"/v1/user_accounts",
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := weberror.ErrorResponse{
			StatusCode: expectedStatus,
			Error:      "Field validation error",
			Fields: []weberror.FieldError{
				//{Field: "user_id", Error: "Key: 'UserAccountDeleteRequest.user_id' Error:Field validation for 'user_id' failed on the 'uuid' tag"},
				//{Field: "account_id", Error: "Key: 'UserAccountDeleteRequest.account_id' Error:Field validation for 'account_id' failed on the 'uuid' tag"},
				{
					Field:   "user_id",
					Value:   req.UserID,
					Tag:     "uuid",
					Error:   "user_id must be a valid UUID",
					Display: "user_id must be a valid UUID",
				},
				{
					Field:   "account_id",
					Value:   req.AccountID,
					Tag:     "uuid",
					Error:   "account_id must be a valid UUID",
					Display: "account_id must be a valid UUID",
				},
			},
			Details:    actual.Details,
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test delete with forbidden ID.
	forbiddenUserAccount := newMockUserAccount(newMockSignup().account.ID, user_account.UserAccountRole_Admin)
	{
		expectedStatus := http.StatusForbidden

		rt := requestTest{
			fmt.Sprintf("Delete %d w/role %s using forbidden IDs", expectedStatus, tr.Role),
			http.MethodDelete,
			fmt.Sprintf("/v1/user_accounts"),
			user_account.UserAccountDeleteRequest{
				UserID:    forbiddenUserAccount.UserID,
				AccountID: forbiddenUserAccount.AccountID,
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := weberror.ErrorResponse{
			StatusCode: expectedStatus,
			Error:      http.StatusText(expectedStatus),
			Details:    user_account.ErrForbidden.Error(),
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}
