package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/mid"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"github.com/pborman/uuid"
)

type mockUser struct {
	*user.User
	password string
}

func mockUserCreateRequest() user.UserCreateRequest {
	return user.UserCreateRequest{
		Name:            "Lee Brown",
		Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
		Password:        "akTechFr0n!ier",
		PasswordConfirm: "akTechFr0n!ier",
	}
}

// mockUser creates a new user for testing and associates it with the supplied account ID.
func newMockUser(accountID string, role user_account.UserAccountRole) mockUser {
	req := mockUserCreateRequest()
	u, err := user.Create(tests.Context(), auth.Claims{}, test.MasterDB, req, time.Now().UTC().AddDate(-1, -1, -1))
	if err != nil {
		panic(err)
	}

	_, err = user_account.Create(tests.Context(), auth.Claims{}, test.MasterDB, user_account.UserAccountCreateRequest{
		UserID:    u.ID,
		AccountID: accountID,
		Roles:     []user_account.UserAccountRole{role},
	}, time.Now().UTC().AddDate(-1, -1, -1))
	if err != nil {
		panic(err)
	}

	return mockUser{
		User:     u,
		password: req.Password,
	}
}

// TestUserCRUDAdmin tests all the user CRUD endpoints using an user with role admin.
func TestUserCRUDAdmin(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the user.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test create.
	var created user.UserResponse
	{
		expectedStatus := http.StatusCreated

		req := mockUserCreateRequest()
		rt := requestTest{
			fmt.Sprintf("Create %d w/role %s", expectedStatus, tr.Role),
			http.MethodPost,
			"/v1/users",
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

		var actual user.UserResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}
		created = actual

		expectedMap := map[string]interface{}{
			"updated_at": web.NewTimeResponse(ctx, actual.UpdatedAt.Value),
			"id":         actual.ID,
			"email":      req.Email,
			"timezone":   actual.Timezone,
			"created_at": web.NewTimeResponse(ctx, actual.CreatedAt.Value),
			"name":       req.Name,
		}

		var expected user.UserResponse
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

		// Only for user creation do we need to do this.
		_, err := user_account.Create(tests.Context(), auth.Claims{}, test.MasterDB, user_account.UserAccountCreateRequest{
			UserID:    actual.ID,
			AccountID: tr.Account.ID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole_User},
		}, time.Now().UTC().AddDate(-1, -1, -1))
		if err != nil {
			t.Fatalf("\t%s\tLink user to account.", tests.Failed)
		}
	}

	// Test read.
	{
		expectedStatus := http.StatusOK

		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/users/%s", created.ID),
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

		var actual user.UserResponse
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
			fmt.Sprintf("/v1/users/%s", randID),
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
			Error: fmt.Sprintf("user %s not found: Entity not found", randID),
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
			fmt.Sprintf("/v1/users/%s", tr.ForbiddenUser.ID),
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
			Error: fmt.Sprintf("user %s not found: Entity not found", tr.ForbiddenUser.ID),
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
			"/v1/users",
			user.UserUpdateRequest{
				ID:   created.ID,
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

	// Test update password.
	{
		expectedStatus := http.StatusNoContent

		newPass := uuid.NewRandom().String()
		rt := requestTest{
			fmt.Sprintf("Update password %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/users/password",
			user.UserUpdatePasswordRequest{
				ID:              created.ID,
				Password:        newPass,
				PasswordConfirm: newPass,
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
			"/v1/users/archive",
			user.UserArchiveRequest{
				ID: created.ID,
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
			fmt.Sprintf("/v1/users/%s", created.ID),
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

		if len(w.Body.String()) != 0 {
			if diff := cmpDiff(t, w.Body.Bytes(), nil); diff {
				t.Fatalf("\t%s\tReceived expected empty.", tests.Failed)
			}
		}
		t.Logf("\t%s\tReceived expected empty.", tests.Success)
	}

	// Test switch account.
	{
		expectedStatus := http.StatusOK

		newAccount := newMockSignup().account
		rt := requestTest{
			fmt.Sprintf("Switch account %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			fmt.Sprintf("/v1/users/switch-account/%s", newAccount.ID),
			nil,
			tr.Token,
			tr.Claims,
			expectedStatus,
			nil,
		}
		t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

		_, err := user_account.Create(tests.Context(), auth.Claims{}, test.MasterDB, user_account.UserAccountCreateRequest{
			UserID:    tr.User.ID,
			AccountID: newAccount.ID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole_User},
		}, time.Now().UTC().AddDate(-1, -1, -1))
		if err != nil {
			t.Fatalf("\t%s\tAdd user to account failed.", tests.Failed)
		}

		w, ok := executeRequestTest(t, rt, ctx)
		if !ok {
			t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
		}
		t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

		var actual map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		// This is just for response format validation, will verify account from claims.
		expected := map[string]interface{}{
			"access_token": actual["access_token"],
			"token_type":   actual["token_type"],
			"expiry":       actual["expiry"],
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected result.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected result.", tests.Success)

		newClaims, err := authenticator.ParseClaims(actual["access_token"].(string))
		if err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tParse claims failed.", tests.Failed)
		} else if newClaims.Audience != newAccount.ID {
			t.Logf("\t\tGot : %+v", newClaims.Audience)
			t.Logf("\t\tExpected : %+v", newAccount.ID)
			t.Fatalf("\t%s\tParse claims expected audience to match new account.", tests.Failed)
		} else if newClaims.Subject != tr.User.ID {
			t.Logf("\t\tGot : %+v", newClaims.Subject)
			t.Logf("\t\tExpected : %+v", tr.User.ID)
			t.Fatalf("\t%s\tParse claims expected Subject to match user.", tests.Failed)
		}
		t.Logf("\t%s\tParse claims valid.", tests.Success)
	}
}

// TestUserCRUDUser tests all the user CRUD endpoints using an user with role user.
func TestUserCRUDUser(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleUser]

	// Add claims to the context for the user.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test create.
	{
		expectedStatus := http.StatusForbidden

		req := mockUserCreateRequest()
		rt := requestTest{
			fmt.Sprintf("Create %d w/role %s", expectedStatus, tr.Role),
			http.MethodPost,
			"/v1/users",
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

	// Since role doesn't support create, bypass auth to test other endpoints.
	created := newMockUser(tr.Account.ID, user_account.UserAccountRole_User).Response(ctx)

	// Test read.
	{
		expectedStatus := http.StatusOK

		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/users/%s", created.ID),
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

		var actual *user.UserResponse
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
			fmt.Sprintf("/v1/users/%s", randID),
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
			Error: fmt.Sprintf("user %s not found: Entity not found", randID),
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
			fmt.Sprintf("/v1/users/%s", tr.ForbiddenUser.ID),
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
			Error: fmt.Sprintf("user %s not found: Entity not found", tr.ForbiddenUser.ID),
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
			"/v1/users",
			user.UserUpdateRequest{
				ID:   created.ID,
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
			Error: user.ErrForbidden.Error(),
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test update password.
	{
		expectedStatus := http.StatusForbidden

		newPass := uuid.NewRandom().String()
		rt := requestTest{
			fmt.Sprintf("Update password %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/users/password",
			user.UserUpdatePasswordRequest{
				ID:              created.ID,
				Password:        newPass,
				PasswordConfirm: newPass,
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
			Error: user.ErrForbidden.Error(),
		}

		if diff := cmpDiff(t, actual, expected); diff {
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
			"/v1/users/archive",
			user.UserArchiveRequest{
				ID: created.ID,
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

	// Test delete.
	{
		expectedStatus := http.StatusForbidden

		rt := requestTest{
			fmt.Sprintf("Delete %d w/role %s", expectedStatus, tr.Role),
			http.MethodDelete,
			fmt.Sprintf("/v1/users/%s", created.ID),
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
			Error: mid.ErrForbidden.Error(),
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test switch account.
	{
		expectedStatus := http.StatusOK

		newAccount := newMockSignup().account
		rt := requestTest{
			fmt.Sprintf("Switch account %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			fmt.Sprintf("/v1/users/switch-account/%s", newAccount.ID),
			nil,
			tr.Token,
			tr.Claims,
			expectedStatus,
			nil,
		}
		t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

		_, err := user_account.Create(tests.Context(), auth.Claims{}, test.MasterDB, user_account.UserAccountCreateRequest{
			UserID:    tr.User.ID,
			AccountID: newAccount.ID,
			Roles:     []user_account.UserAccountRole{user_account.UserAccountRole_User},
		}, time.Now().UTC().AddDate(-1, -1, -1))
		if err != nil {
			t.Fatalf("\t%s\tAdd user to account failed.", tests.Failed)
		}

		w, ok := executeRequestTest(t, rt, ctx)
		if !ok {
			t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
		}
		t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

		var actual map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		// This is just for response format validation, will verify account from claims.
		expected := map[string]interface{}{
			"access_token": actual["access_token"],
			"token_type":   actual["token_type"],
			"expiry":       actual["expiry"],
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected result.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected result.", tests.Success)

		newClaims, err := authenticator.ParseClaims(actual["access_token"].(string))
		if err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tParse claims failed.", tests.Failed)
		} else if newClaims.Audience != newAccount.ID {
			t.Logf("\t\tGot : %+v", newClaims.Audience)
			t.Logf("\t\tExpected : %+v", newAccount.ID)
			t.Fatalf("\t%s\tParse claims expected audience to match new account.", tests.Failed)
		} else if newClaims.Subject != tr.User.ID {
			t.Logf("\t\tGot : %+v", newClaims.Subject)
			t.Logf("\t\tExpected : %+v", tr.User.ID)
			t.Fatalf("\t%s\tParse claims expected Subject to match user.", tests.Failed)
		}
		t.Logf("\t%s\tParse claims valid.", tests.Success)
	}
}

// TestUserCreate validates create user endpoint.
func TestUserCreate(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the user.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test create with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		req := mockUserCreateRequest()
		req.Email = "invalid email address.com"
		rt := requestTest{
			fmt.Sprintf("Create %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodPost,
			"/v1/users",
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

		var actual web.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := web.ErrorResponse{
			Error: "field validation error",
			Fields: []web.FieldError{
				{Field: "email", Error: "Key: 'UserCreateRequest.email' Error:Field validation for 'email' failed on the 'email' tag"},
			},
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestUserUpdate validates update user endpoint.
func TestUserUpdate(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the user.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test update with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		invalidEmail := "invalid email address"
		rt := requestTest{
			fmt.Sprintf("Update %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/users",
			user.UserUpdateRequest{
				ID:    tr.User.ID,
				Email: &invalidEmail,
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
				{Field: "email", Error: "Key: 'UserUpdateRequest.email' Error:Field validation for 'email' failed on the 'email' tag"},
			},
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestUserUpdatePassword validates update user password endpoint.
func TestUserUpdatePassword(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the user.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Since role doesn't support create, bypass auth to test other endpoints.
	created := newMockUser(tr.Account.ID, user_account.UserAccountRole_User).Response(ctx)

	// Test update user password with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		newPass := uuid.NewRandom().String()
		rt := requestTest{
			fmt.Sprintf("Update password %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/users/password",
			user.UserUpdatePasswordRequest{
				ID:              created.ID,
				Password:        newPass,
				PasswordConfirm: "different",
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
				{Field: "password_confirm", Error: "Key: 'UserUpdatePasswordRequest.password_confirm' Error:Field validation for 'password_confirm' failed on the 'eqfield' tag"},
			},
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestUserArchive validates archive user endpoint.
func TestUserArchive(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the user.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test archive user with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		rt := requestTest{
			fmt.Sprintf("Archive %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/users/archive",
			user.UserArchiveRequest{
				ID: "a",
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
				{Field: "id", Error: "Key: 'UserArchiveRequest.id' Error:Field validation for 'id' failed on the 'uuid' tag"},
			},
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test archive user with forbidden ID.
	{
		expectedStatus := http.StatusForbidden

		rt := requestTest{
			fmt.Sprintf("Archive %d w/role %s using forbidden ID", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/users/archive",
			user.UserArchiveRequest{
				ID: tr.ForbiddenUser.ID,
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
			Error: user.ErrForbidden.Error(),
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestUserDelete validates delete user endpoint.
func TestUserDelete(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the user.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test delete user with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		rt := requestTest{
			fmt.Sprintf("Delete %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodDelete,
			"/v1/users/345345",
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
			Error: "field validation error",
			Fields: []web.FieldError{
				{Field: "id", Error: "Key: 'id' Error:Field validation for 'id' failed on the 'uuid' tag"},
			},
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test delete user with forbidden ID.
	{
		expectedStatus := http.StatusForbidden

		rt := requestTest{
			fmt.Sprintf("Delete %d w/role %s using forbidden ID", expectedStatus, tr.Role),
			http.MethodDelete,
			fmt.Sprintf("/v1/users/%s", tr.ForbiddenUser.ID),
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
			Error: user.ErrForbidden.Error(),
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestUserSwitchAccount validates user switch account endpoint.
func TestUserSwitchAccount(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the user.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test user switch account with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		rt := requestTest{
			fmt.Sprintf("Switch account %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/users/switch-account/sf",
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
			Error: "field validation error",
			Fields: []web.FieldError{
				{Field: "account_id", Error: "Key: 'account_id' Error:Field validation for 'account_id' failed on the 'uuid' tag"},
			},
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test user switch account with forbidden ID.
	{
		expectedStatus := http.StatusUnauthorized

		rt := requestTest{
			fmt.Sprintf("Switch account %d w/role %s using forbidden ID", expectedStatus, tr.Role),
			http.MethodPatch,
			fmt.Sprintf("/v1/users/switch-account/%s", tr.ForbiddenAccount.ID),
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
			Error: user.ErrAuthenticationFailure.Error(),
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestUserToken validates user token endpoint.
func TestUserToken(t *testing.T) {
	defer tests.Recover(t)

	// Test user token with empty credentials.
	{
		expectedStatus := http.StatusUnauthorized

		rt := requestTest{
			fmt.Sprintf("Token %d using empty request", expectedStatus),
			http.MethodPost,
			"/v1/oauth/token",
			nil,
			user.Token{},
			auth.Claims{},
			expectedStatus,
			nil,
		}
		t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

		w, ok := executeRequestTest(t, rt, tests.Context())
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
			Error: "must provide email and password in Basic auth",
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test user token with invalid email.
	{
		expectedStatus := http.StatusUnauthorized

		rt := requestTest{
			fmt.Sprintf("Token %d using invalid email", expectedStatus),
			http.MethodPost,
			"/v1/oauth/token",
			nil,
			user.Token{},
			auth.Claims{},
			expectedStatus,
			nil,
		}
		t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

		r := httptest.NewRequest(rt.method, rt.url, nil)
		r.SetBasicAuth("invalid email.com", "some random password")

		w := httptest.NewRecorder()
		r.Header.Set("Content-Type", web.MIMEApplicationJSONCharsetUTF8)

		a.ServeHTTP(w, r)

		if w.Code != expectedStatus {
			t.Logf("\t\tBody : %s\n", w.Body.String())
			t.Logf("\t\tShould receive a status code of %d for the response : %v", rt.statusCode, w.Code)
			t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
		}
		t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

		var actual web.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := web.ErrorResponse{
			Error: user.ErrAuthenticationFailure.Error(),
		}

		if diff := cmpDiff(t, actual, expected); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test user token with invalid password.
	{
		for _, tr := range roleTests {
			expectedStatus := http.StatusUnauthorized

			rt := requestTest{
				fmt.Sprintf("Token %d w/role %s using invalid password", expectedStatus, tr.Role),
				http.MethodPost,
				"/v1/oauth/token",
				nil,
				user.Token{},
				auth.Claims{},
				expectedStatus,
				nil,
			}
			t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

			r := httptest.NewRequest(rt.method, rt.url, nil)
			r.SetBasicAuth(tr.User.Email, "invalid password")

			w := httptest.NewRecorder()
			r.Header.Set("Content-Type", web.MIMEApplicationJSONCharsetUTF8)

			a.ServeHTTP(w, r)

			if w.Code != expectedStatus {
				t.Logf("\t\tBody : %s\n", w.Body.String())
				t.Logf("\t\tShould receive a status code of %d for the response : %v", rt.statusCode, w.Code)
				t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
			}
			t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

			var actual web.ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
				t.Logf("\t\tGot error : %+v", err)
				t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
			}

			expected := web.ErrorResponse{
				Error: user.ErrAuthenticationFailure.Error(),
			}

			if diff := cmpDiff(t, actual, expected); diff {
				t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
			}
			t.Logf("\t%s\tReceived expected error.", tests.Success)
		}
	}

	// Test user token with valid email and password.
	{
		for _, tr := range roleTests {
			expectedStatus := http.StatusOK

			rt := requestTest{
				fmt.Sprintf("Token %d w/role %s using valid credentials", expectedStatus, tr.Role),
				http.MethodPost,
				"/v1/oauth/token",
				nil,
				user.Token{},
				auth.Claims{},
				expectedStatus,
				nil,
			}
			t.Logf("\tTest: %s - %s %s", rt.name, rt.method, rt.url)

			r := httptest.NewRequest(rt.method, rt.url, nil)
			r.SetBasicAuth(tr.User.Email, tr.User.password)

			w := httptest.NewRecorder()
			r.Header.Set("Content-Type", web.MIMEApplicationJSONCharsetUTF8)

			a.ServeHTTP(w, r)

			if w.Code != expectedStatus {
				t.Logf("\t\tBody : %s\n", w.Body.String())
				t.Logf("\t\tShould receive a status code of %d for the response : %v", rt.statusCode, w.Code)
				t.Fatalf("\t%s\tExecute request failed.", tests.Failed)
			}
			t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

			var actual map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
				t.Logf("\t\tGot error : %+v", err)
				t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
			}

			// This is just for response format validation, will verify account from claims.
			expected := map[string]interface{}{
				"access_token": actual["access_token"],
				"token_type":   actual["token_type"],
				"expiry":       actual["expiry"],
			}

			if diff := cmpDiff(t, actual, expected); diff {
				t.Fatalf("\t%s\tReceived expected result.", tests.Failed)
			}
			t.Logf("\t%s\tReceived expected result.", tests.Success)
		}
	}
}
