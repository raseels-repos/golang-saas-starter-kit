package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/checklist"
	"geeks-accelerator/oss/saas-starter-kit/internal/mid"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/weberror"
	"github.com/pborman/uuid"
)

func mockChecklistCreateRequest(accountID string) checklist.ChecklistCreateRequest {
	return checklist.ChecklistCreateRequest{
		Name:      fmt.Sprintf("Moon Launch %s", uuid.NewRandom().String()),
		AccountID: accountID,
	}
}

// mockChecklist creates a new checklist for testing and associates it with the supplied account ID.
func newMockChecklist(accountID string) *checklist.Checklist {
	req := mockChecklistCreateRequest(accountID)
	p, err := appCtx.ChecklistRepo.Create(tests.Context(), auth.Claims{}, req, time.Now().UTC().AddDate(-1, -1, -1))
	if err != nil {
		panic(err)
	}

	return p
}

// TestChecklistCRUDAdmin tests all the checklist CRUD endpoints using an user with role admin.
func TestChecklistCRUDAdmin(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the checklist.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test create.
	var created checklist.ChecklistResponse
	{
		expectedStatus := http.StatusCreated

		req := mockChecklistCreateRequest(tr.Account.ID)
		rt := requestTest{
			fmt.Sprintf("Create %d w/role %s", expectedStatus, tr.Role),
			http.MethodPost,
			"/v1/checklists",
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

		var actual checklist.ChecklistResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}
		created = actual

		expectedMap := map[string]interface{}{
			"updated_at": web.NewTimeResponse(ctx, actual.UpdatedAt.Value),
			"id":         actual.ID,
			"account_id": req.AccountID,
			"status":     web.NewEnumResponse(ctx, "active", checklist.ChecklistStatus_ValuesInterface()...),
			"created_at": web.NewTimeResponse(ctx, actual.CreatedAt.Value),
			"name":       req.Name,
		}

		var expected checklist.ChecklistResponse
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
			fmt.Sprintf("/v1/checklists/%s", created.ID),
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

		var actual checklist.ChecklistResponse
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
			fmt.Sprintf("/v1/checklists/%s", randID),
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
			Details:    fmt.Sprintf("checklist %s not found: Entity not found", randID),
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test Read with forbidden ID.
	forbiddenChecklist := newMockChecklist(newMockSignup().account.ID)
	{
		expectedStatus := http.StatusNotFound

		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s using forbidden ID", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/checklists/%s", forbiddenChecklist.ID),
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
			Details:    fmt.Sprintf("checklist %s not found: Entity not found", forbiddenChecklist.ID),
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

		newName := uuid.NewRandom().String()
		rt := requestTest{
			fmt.Sprintf("Update %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/checklists",
			checklist.ChecklistUpdateRequest{
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

	// Test archive.
	{
		expectedStatus := http.StatusNoContent

		rt := requestTest{
			fmt.Sprintf("Archive %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/checklists/archive",
			checklist.ChecklistArchiveRequest{
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
			fmt.Sprintf("/v1/checklists/%s", created.ID),
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
}

// TestChecklistCRUDUser tests all the checklist CRUD endpoints using an user with role checklist.
func TestChecklistCRUDUser(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleUser]

	// Add claims to the context for the checklist.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test create.
	{
		expectedStatus := http.StatusForbidden

		req := mockChecklistCreateRequest(tr.Account.ID)
		rt := requestTest{
			fmt.Sprintf("Create %d w/role %s", expectedStatus, tr.Role),
			http.MethodPost,
			"/v1/checklists",
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

		expected := mid.ErrorForbidden(ctx).(*weberror.Error).Response(ctx, false)
		expected.StackTrace = actual.StackTrace

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Since role doesn't support create, bypass auth to test other endpoints.
	created := newMockChecklist(tr.Account.ID).Response(ctx)

	// Test read.
	{
		expectedStatus := http.StatusOK

		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/checklists/%s", created.ID),
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

		var actual *checklist.ChecklistResponse
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
			fmt.Sprintf("/v1/checklists/%s", randID),
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
			Details:    fmt.Sprintf("checklist %s not found: Entity not found", randID),
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test Read with forbidden ID.
	forbiddenChecklist := newMockChecklist(newMockSignup().account.ID)
	{
		expectedStatus := http.StatusNotFound

		rt := requestTest{
			fmt.Sprintf("Read %d w/role %s using forbidden ID", expectedStatus, tr.Role),
			http.MethodGet,
			fmt.Sprintf("/v1/checklists/%s", forbiddenChecklist.ID),
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
			Details:    fmt.Sprintf("checklist %s not found: Entity not found", forbiddenChecklist.ID),
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

		newName := uuid.NewRandom().String()
		rt := requestTest{
			fmt.Sprintf("Update %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/checklists",
			checklist.ChecklistUpdateRequest{
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

	// Test archive.
	{
		expectedStatus := http.StatusForbidden

		rt := requestTest{
			fmt.Sprintf("Archive %d w/role %s", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/checklists/archive",
			checklist.ChecklistArchiveRequest{
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
			fmt.Sprintf("/v1/checklists/%s", created.ID),
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

		expected := mid.ErrorForbidden(ctx).(*weberror.Error).Response(ctx, false)
		expected.StackTrace = actual.StackTrace

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestChecklistCreate validates create checklist endpoint.
func TestChecklistCreate(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the checklist.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test create with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		req := mockChecklistCreateRequest(tr.Account.ID)
		invalidStatus := checklist.ChecklistStatus("invalid status")
		req.Status = &invalidStatus
		rt := requestTest{
			fmt.Sprintf("Create %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodPost,
			"/v1/checklists",
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
			Details:    actual.Details,
			Error:      "Field validation error",
			Fields: []weberror.FieldError{
				//{Field: "status", Error: "Key: 'ChecklistCreateRequest.status' Error:Field validation for 'status' failed on the 'oneof' tag"},
				{
					Field:   "status",
					Value:   invalidStatus.String(),
					Tag:     "oneof",
					Error:   "status must be one of [active disabled]",
					Display: "status must be one of [active disabled]",
				},
			},
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestChecklistUpdate validates update checklist endpoint.
func TestChecklistUpdate(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the checklist.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	// Test update with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		invalidStatus := checklist.ChecklistStatus("invalid status")
		rt := requestTest{
			fmt.Sprintf("Update %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/checklists",
			checklist.ChecklistUpdateRequest{
				ID:     uuid.NewRandom().String(),
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

		var actual weberror.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tDecode response body failed.", tests.Failed)
		}

		expected := weberror.ErrorResponse{
			StatusCode: expectedStatus,
			Details:    actual.Details,
			Error:      "Field validation error",
			Fields: []weberror.FieldError{
				//{Field: "status", Error: "Key: 'ChecklistUpdateRequest.status' Error:Field validation for 'status' failed on the 'oneof' tag"},
				{
					Field:   "status",
					Value:   invalidStatus.String(),
					Tag:     "oneof",
					Error:   "status must be one of [active disabled]",
					Display: "status must be one of [active disabled]",
				},
			},
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestChecklistArchive validates archive checklist endpoint.
func TestChecklistArchive(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the checklist.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	forbiddenChecklist := newMockChecklist(newMockSignup().account.ID)

	// Test archive with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		invalidId := "a"

		rt := requestTest{
			fmt.Sprintf("Archive %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/checklists/archive",
			checklist.ChecklistArchiveRequest{
				ID: invalidId,
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
			Details:    actual.Details,
			Error:      "Field validation error",
			Fields: []weberror.FieldError{
				//{Field: "id", Error: "Key: 'ChecklistArchiveRequest.id' Error:Field validation for 'id' failed on the 'uuid' tag"},
				{
					Field:   "id",
					Value:   invalidId,
					Tag:     "uuid",
					Error:   "id must be a valid UUID",
					Display: "id must be a valid UUID",
				},
			},
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test archive with forbidden ID.
	{
		expectedStatus := http.StatusForbidden

		rt := requestTest{
			fmt.Sprintf("Archive %d w/role %s using forbidden ID", expectedStatus, tr.Role),
			http.MethodPatch,
			"/v1/checklists/archive",
			checklist.ChecklistArchiveRequest{
				ID: forbiddenChecklist.ID,
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
			Details:    checklist.ErrForbidden.Error(),
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}

// TestChecklistDelete validates delete checklist endpoint.
func TestChecklistDelete(t *testing.T) {
	defer tests.Recover(t)

	tr := roleTests[auth.RoleAdmin]

	// Add claims to the context for the checklist.
	ctx := context.WithValue(tests.Context(), auth.Key, tr.Claims)

	forbiddenChecklist := newMockChecklist(newMockSignup().account.ID)

	// Test delete with invalid data.
	{
		expectedStatus := http.StatusBadRequest

		invalidId := "a"

		rt := requestTest{
			fmt.Sprintf("Delete %d w/role %s using invalid data", expectedStatus, tr.Role),
			http.MethodDelete,
			"/v1/checklists/" + invalidId,
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
			Details:    actual.Details,
			Error:      "Field validation error",
			Fields: []weberror.FieldError{
				//{Field: "id", Error: "Key: 'id' Error:Field validation for 'id' failed on the 'uuid' tag"},
				{
					Field:   "id",
					Value:   invalidId,
					Tag:     "uuid",
					Error:   "id must be a valid UUID",
					Display: "id must be a valid UUID",
				},
			},
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}

	// Test delete with forbidden ID.
	{
		expectedStatus := http.StatusForbidden

		rt := requestTest{
			fmt.Sprintf("Delete %d w/role %s using forbidden ID", expectedStatus, tr.Role),
			http.MethodDelete,
			fmt.Sprintf("/v1/checklists/%s", forbiddenChecklist.ID),
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
			Details:    checklist.ErrForbidden.Error(),
			StackTrace: actual.StackTrace,
		}

		if diff := cmpDiff(t, expected, actual); diff {
			t.Fatalf("\t%s\tReceived expected error.", tests.Failed)
		}
		t.Logf("\t%s\tReceived expected error.", tests.Success)
	}
}
