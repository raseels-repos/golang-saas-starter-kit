package tests

/*
import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/project"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gopkg.in/mgo.v2/bson"
)


// TestProjects is the entry point for the projects
func TestProjects(t *testing.T) {
	defer tests.Recover(t)

	t.Run("getProjects200Empty", getProjects200Empty)
	t.Run("postProject400", postProject400)
	t.Run("postProject401", postProject401)
	t.Run("getProject404", getProject404)
	t.Run("getProject400", getProject400)
	t.Run("deleteProject404", deleteProject404)
	t.Run("putProject404", putProject404)
	t.Run("crudProjects", crudProject)
}

// TODO: need to test Archive

// getProjects200Empty validates an empty projects list can be retrieved with the endpoint.
func getProjects200Empty(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/projects", nil)
	w := httptest.NewRecorder()

	r.Header.Set("Authorization", userAuthorization)

	a.ServeHTTP(w, r)

	t.Log("Given the need to fetch an empty list of projects with the projects endpoint.")
	{
		t.Log("\tTest 0:\tWhen fetching an empty project list.")
		{
			if w.Code != http.StatusOK {
				t.Fatalf("\t%s\tShould receive a status code of 200 for the response : %v", tests.Failed, w.Code)
			}
			t.Logf("\t%s\tShould receive a status code of 200 for the response.", tests.Success)

			recv := w.Body.String()
			resp := `[]`
			if resp != recv {
				t.Log("Got :", recv)
				t.Log("Want:", resp)
				t.Fatalf("\t%s\tShould get the expected result.", tests.Failed)
			}
			t.Logf("\t%s\tShould get the expected result.", tests.Success)
		}
	}
}

// postProject400 validates a project can't be created with the endpoint
// unless a valid project document is submitted.
func postProject400(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/projects", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	r.Header.Set("Authorization", userAuthorization)

	a.ServeHTTP(w, r)

	t.Log("Given the need to validate a new project can't be created with an invalid document.")
	{
		t.Log("\tTest 0:\tWhen using an incomplete project value.")
		{
			if w.Code != http.StatusBadRequest {
				t.Fatalf("\t%s\tShould receive a status code of 400 for the response : %v", tests.Failed, w.Code)
			}
			t.Logf("\t%s\tShould receive a status code of 400 for the response.", tests.Success)

			// Inspect the response.
			var got web.ErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
				t.Fatalf("\t%s\tShould be able to unmarshal the response to an error type : %v", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to unmarshal the response to an error type.", tests.Success)

			// Define what we want to see.
			want := web.ErrorResponse{
				Error: "field validation error",
				Fields: []web.FieldError{
					{Field: "name", Error: "name is a required field"},
					{Field: "cost", Error: "cost is a required field"},
					{Field: "quantity", Error: "quantity is a required field"},
				},
			}

			// We can't rely on the order of the field errors so they have to be
			// sorted. Tell the cmp package how to sort them.
			sorter := cmpopts.SortSlices(func(a, b web.FieldError) bool {
				return a.Field < b.Field
			})

			if diff := cmp.Diff(want, got, sorter); diff != "" {
				t.Fatalf("\t%s\tShould get the expected result. Diff:\n%s", tests.Failed, diff)
			}
			t.Logf("\t%s\tShould get the expected result.", tests.Success)
		}
	}
}

// postProject401 validates a project can't be created with the endpoint
// unless the user is authenticated
func postProject401(t *testing.T) {
	np := project.NewProject{
		Name:     "Comic Books",
		Cost:     25,
		Quantity: 60,
	}

	body, err := json.Marshal(&np)
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest("POST", "/v1/projects", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	// Not setting an authorization header

	a.ServeHTTP(w, r)

	t.Log("Given the need to validate a new project can't be created with an invalid document.")
	{
		t.Log("\tTest 0:\tWhen using an incomplete project value.")
		{
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("\t%s\tShould receive a status code of 401 for the response : %v", tests.Failed, w.Code)
			}
			t.Logf("\t%s\tShould receive a status code of 401 for the response.", tests.Success)
		}
	}
}

// getProject400 validates a project request for a malformed id.
func getProject400(t *testing.T) {
	id := "12345"

	r := httptest.NewRequest("GET", "/v1/projects/"+id, nil)
	w := httptest.NewRecorder()

	r.Header.Set("Authorization", userAuthorization)

	a.ServeHTTP(w, r)

	t.Log("Given the need to validate getting a project with a malformed id.")
	{
		t.Logf("\tTest 0:\tWhen using the new project %s.", id)
		{
			if w.Code != http.StatusBadRequest {
				t.Fatalf("\t%s\tShould receive a status code of 400 for the response : %v", tests.Failed, w.Code)
			}
			t.Logf("\t%s\tShould receive a status code of 400 for the response.", tests.Success)

			recv := w.Body.String()
			resp := `{"error":"ID is not in its proper form"}`
			if resp != recv {
				t.Log("Got :", recv)
				t.Log("Want:", resp)
				t.Fatalf("\t%s\tShould get the expected result.", tests.Failed)
			}
			t.Logf("\t%s\tShould get the expected result.", tests.Success)
		}
	}
}

// getProject404 validates a project request for a project that does not exist with the endpoint.
func getProject404(t *testing.T) {
	id := bson.NewObjectId().Hex()

	r := httptest.NewRequest("GET", "/v1/projects/"+id, nil)
	w := httptest.NewRecorder()

	r.Header.Set("Authorization", userAuthorization)

	a.ServeHTTP(w, r)

	t.Log("Given the need to validate getting a project with an unknown id.")
	{
		t.Logf("\tTest 0:\tWhen using the new project %s.", id)
		{
			if w.Code != http.StatusNotFound {
				t.Fatalf("\t%s\tShould receive a status code of 404 for the response : %v", tests.Failed, w.Code)
			}
			t.Logf("\t%s\tShould receive a status code of 404 for the response.", tests.Success)

			recv := w.Body.String()
			resp := "Entity not found"
			if !strings.Contains(recv, resp) {
				t.Log("Got :", recv)
				t.Log("Want:", resp)
				t.Fatalf("\t%s\tShould get the expected result.", tests.Failed)
			}
			t.Logf("\t%s\tShould get the expected result.", tests.Success)
		}
	}
}

// deleteProject404 validates deleting a project that does not exist.
func deleteProject404(t *testing.T) {
	id := bson.NewObjectId().Hex()

	r := httptest.NewRequest("DELETE", "/v1/projects/"+id, nil)
	w := httptest.NewRecorder()

	r.Header.Set("Authorization", userAuthorization)

	a.ServeHTTP(w, r)

	t.Log("Given the need to validate deleting a project that does not exist.")
	{
		t.Logf("\tTest 0:\tWhen using the new project %s.", id)
		{
			if w.Code != http.StatusNotFound {
				t.Fatalf("\t%s\tShould receive a status code of 404 for the response : %v", tests.Failed, w.Code)
			}
			t.Logf("\t%s\tShould receive a status code of 404 for the response.", tests.Success)

			recv := w.Body.String()
			resp := "Entity not found"
			if !strings.Contains(recv, resp) {
				t.Log("Got :", recv)
				t.Log("Want:", resp)
				t.Fatalf("\t%s\tShould get the expected result.", tests.Failed)
			}
			t.Logf("\t%s\tShould get the expected result.", tests.Success)
		}
	}
}

// putProject404 validates updating a project that does not exist.
func putProject404(t *testing.T) {
	up := project.UpdateProject{
		Name: tests.StringPointer("Nonexistent"),
	}

	id := bson.NewObjectId().Hex()

	body, err := json.Marshal(&up)
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest("PUT", "/v1/projects/"+id, bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	r.Header.Set("Authorization", userAuthorization)

	a.ServeHTTP(w, r)

	t.Log("Given the need to validate updating a project that does not exist.")
	{
		t.Logf("\tTest 0:\tWhen using the new project %s.", id)
		{
			if w.Code != http.StatusNotFound {
				t.Fatalf("\t%s\tShould receive a status code of 404 for the response : %v", tests.Failed, w.Code)
			}
			t.Logf("\t%s\tShould receive a status code of 404 for the response.", tests.Success)

			recv := w.Body.String()
			resp := "Entity not found"
			if !strings.Contains(recv, resp) {
				t.Log("Got :", recv)
				t.Log("Want:", resp)
				t.Fatalf("\t%s\tShould get the expected result.", tests.Failed)
			}
			t.Logf("\t%s\tShould get the expected result.", tests.Success)
		}
	}
}

// crudProject performs a complete test of CRUD against the api.
func crudProject(t *testing.T) {
	p := postProject201(t)
	defer deleteProject204(t, p.ID.Hex())

	getProject200(t, p.ID.Hex())
	putProject204(t, p.ID.Hex())
}

// postProject201 validates a project can be created with the endpoint.
func postProject201(t *testing.T) project.Project {
	np := project.NewProject{
		Name:     "Comic Books",
		Cost:     25,
		Quantity: 60,
	}

	body, err := json.Marshal(&np)
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest("POST", "/v1/projects", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	r.Header.Set("Authorization", userAuthorization)

	a.ServeHTTP(w, r)

	// p is the value we will return.
	var p project.Project

	t.Log("Given the need to create a new project with the projects endpoint.")
	{
		t.Log("\tTest 0:\tWhen using the declared project value.")
		{
			if w.Code != http.StatusCreated {
				t.Fatalf("\t%s\tShould receive a status code of 201 for the response : %v", tests.Failed, w.Code)
			}
			t.Logf("\t%s\tShould receive a status code of 201 for the response.", tests.Success)

			if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
				t.Fatalf("\t%s\tShould be able to unmarshal the response : %v", tests.Failed, err)
			}

			// Define what we wanted to receive. We will just trust the generated
			// fields like ID and Dates so we copy p.
			want := p
			want.Name = "Comic Books"
			want.Cost = 25
			want.Quantity = 60

			if diff := cmp.Diff(want, p); diff != "" {
				t.Fatalf("\t%s\tShould get the expected result. Diff:\n%s", tests.Failed, diff)
			}
			t.Logf("\t%s\tShould get the expected result.", tests.Success)
		}
	}

	return p
}

// deleteProject200 validates deleting a project that does exist.
func deleteProject204(t *testing.T, id string) {
	r := httptest.NewRequest("DELETE", "/v1/projects/"+id, nil)
	w := httptest.NewRecorder()

	r.Header.Set("Authorization", userAuthorization)

	a.ServeHTTP(w, r)

	t.Log("Given the need to validate deleting a project that does exist.")
	{
		t.Logf("\tTest 0:\tWhen using the new project %s.", id)
		{
			if w.Code != http.StatusNoContent {
				t.Fatalf("\t%s\tShould receive a status code of 204 for the response : %v", tests.Failed, w.Code)
			}
			t.Logf("\t%s\tShould receive a status code of 204 for the response.", tests.Success)
		}
	}
}

// getProject200 validates a project request for an existing id.
func getProject200(t *testing.T, id string) {
	r := httptest.NewRequest("GET", "/v1/projects/"+id, nil)
	w := httptest.NewRecorder()

	r.Header.Set("Authorization", userAuthorization)

	a.ServeHTTP(w, r)

	t.Log("Given the need to validate getting a project that exists.")
	{
		t.Logf("\tTest 0:\tWhen using the new project %s.", id)
		{
			if w.Code != http.StatusOK {
				t.Fatalf("\t%s\tShould receive a status code of 200 for the response : %v", tests.Failed, w.Code)
			}
			t.Logf("\t%s\tShould receive a status code of 200 for the response.", tests.Success)

			var p project.Project
			if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
				t.Fatalf("\t%s\tShould be able to unmarshal the response : %v", tests.Failed, err)
			}

			// Define what we wanted to receive. We will just trust the generated
			// fields like Dates so we copy p.
			want := p
			want.ID = bson.ObjectIdHex(id)
			want.Name = "Comic Books"
			want.Cost = 25
			want.Quantity = 60

			if diff := cmp.Diff(want, p); diff != "" {
				t.Fatalf("\t%s\tShould get the expected result. Diff:\n%s", tests.Failed, diff)
			}
			t.Logf("\t%s\tShould get the expected result.", tests.Success)
		}
	}
}

// putProject204 validates updating a project that does exist.
func putProject204(t *testing.T, id string) {
	body := `{"name": "Graphic Novels", "cost": 100}`
	r := httptest.NewRequest("PUT", "/v1/projects/"+id, strings.NewReader(body))
	w := httptest.NewRecorder()

	r.Header.Set("Authorization", userAuthorization)

	a.ServeHTTP(w, r)

	t.Log("Given the need to update a project with the projects endpoint.")
	{
		t.Log("\tTest 0:\tWhen using the modified project value.")
		{
			if w.Code != http.StatusNoContent {
				t.Fatalf("\t%s\tShould receive a status code of 204 for the response : %v", tests.Failed, w.Code)
			}
			t.Logf("\t%s\tShould receive a status code of 204 for the response.", tests.Success)

			r = httptest.NewRequest("GET", "/v1/projects/"+id, nil)
			w = httptest.NewRecorder()

			r.Header.Set("Authorization", userAuthorization)

			a.ServeHTTP(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("\t%s\tShould receive a status code of 200 for the retrieve : %v", tests.Failed, w.Code)
			}
			t.Logf("\t%s\tShould receive a status code of 200 for the retrieve.", tests.Success)

			var ru project.Project
			if err := json.NewDecoder(w.Body).Decode(&ru); err != nil {
				t.Fatalf("\t%s\tShould be able to unmarshal the response : %v", tests.Failed, err)
			}

			if ru.Name != "Graphic Novels" {
				t.Fatalf("\t%s\tShould see an updated Name : got %q want %q", tests.Failed, ru.Name, "Graphic Novels")
			}
			t.Logf("\t%s\tShould see an updated Name.", tests.Success)
		}
	}
}
*/
