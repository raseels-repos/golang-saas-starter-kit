package project_test

import (
	"os"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/project"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
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

// TestProject validates the full set of CRUD operations on Project values.
func TestProject(t *testing.T) {
	defer tests.Recover(t)

	t.Log("Given the need to work with Project records.")
	{
		t.Log("\tWhen handling a single Project.")
		{
			ctx := tests.Context()

			dbConn := test.MasterDB.Copy()
			defer dbConn.Close()

			np := project.NewProject{
				Name:     "Comic Books",
				Cost:     25,
				Quantity: 60,
			}

			p, err := project.Create(ctx, dbConn, &np, time.Now().UTC())
			if err != nil {
				t.Fatalf("\t%s\tShould be able to create a project : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to create a project.", tests.Success)

			savedP, err := project.Retrieve(ctx, dbConn, p.ID.Hex())
			if err != nil {
				t.Fatalf("\t%s\tShould be able to retrieve project by ID: %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to retrieve project by ID.", tests.Success)

			if diff := cmp.Diff(p, savedP); diff != "" {
				t.Fatalf("\t%s\tShould get back the same project. Diff:\n%s", tests.Failed, diff)
			}
			t.Logf("\t%s\tShould get back the same project.", tests.Success)

			upd := project.UpdateProject{
				Name:     tests.StringPointer("Comics"),
				Cost:     tests.IntPointer(50),
				Quantity: tests.IntPointer(40),
			}

			if err := project.Update(ctx, dbConn, p.ID.Hex(), upd, time.Now().UTC()); err != nil {
				t.Fatalf("\t%s\tShould be able to update project : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to update project.", tests.Success)

			savedP, err = project.Retrieve(ctx, dbConn, p.ID.Hex())
			if err != nil {
				t.Fatalf("\t%s\tShould be able to retrieve updated project : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to retrieve updated project.", tests.Success)

			// Build a project matching what we expect to see. We just use the
			// modified time from the database.
			want := &project.Project{
				ID:           p.ID,
				Name:         *upd.Name,
				Cost:         *upd.Cost,
				Quantity:     *upd.Quantity,
				DateCreated:  p.DateCreated,
				DateModified: savedP.DateModified,
			}

			if diff := cmp.Diff(want, savedP); diff != "" {
				t.Fatalf("\t%s\tShould get back the same project. Diff:\n%s", tests.Failed, diff)
			}
			t.Logf("\t%s\tShould get back the same project.", tests.Success)

			upd = project.UpdateProject{
				Name: tests.StringPointer("Graphic Novels"),
			}

			if err := project.Update(ctx, dbConn, p.ID.Hex(), upd, time.Now().UTC()); err != nil {
				t.Fatalf("\t%s\tShould be able to update just some fields of project : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to update just some fields of project.", tests.Success)

			savedP, err = project.Retrieve(ctx, dbConn, p.ID.Hex())
			if err != nil {
				t.Fatalf("\t%s\tShould be able to retrieve updated project : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to retrieve updated project.", tests.Success)

			if savedP.Name != *upd.Name {
				t.Fatalf("\t%s\tShould be able to see updated Name field : got %q want %q.", tests.Failed, savedP.Name, *upd.Name)
			} else {
				t.Logf("\t%s\tShould be able to see updated Name field.", tests.Success)
			}

			if err := project.Delete(ctx, dbConn, p.ID.Hex()); err != nil {
				t.Fatalf("\t%s\tShould be able to delete project : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould be able to delete project.", tests.Success)

			savedP, err = project.Retrieve(ctx, dbConn, p.ID.Hex())
			if errors.Cause(err) != project.ErrNotFound {
				t.Fatalf("\t%s\tShould NOT be able to retrieve deleted project : %s.", tests.Failed, err)
			}
			t.Logf("\t%s\tShould NOT be able to retrieve deleted project.", tests.Success)
		}
	}
}
