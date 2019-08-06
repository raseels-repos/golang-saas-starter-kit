package project

import (
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"github.com/google/go-cmp/cmp"
	"github.com/huandu/go-sqlbuilder"
	"os"
	"testing"
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

// TestFindRequestQuery validates findRequestQuery
func TestFindRequestQuery(t *testing.T) {

	var (
		limit  uint = 12
		offset uint = 34
	)

	req := ProjectFindRequest{
		Where: "field1 = ? or field2 = ?",
		Args: []interface{}{
			"lee brown",
			"103 East Main St.",
		},

		Order: []string{
			"id asc",
			"created_at desc",
		},

		Limit:  &limit,
		Offset: &offset,
	}

	expected := "SELECT " + projectMapColumns + " FROM " + projectTableName + " WHERE (field1 = ? or field2 = ?) ORDER BY id asc, created_at desc LIMIT 12 OFFSET 34"
	res, args := findRequestQuery(req)
	if diff := cmp.Diff(res.String(), expected); diff != "" {
		t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
	}

	if diff := cmp.Diff(args, req.Args); diff != "" {
		t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
	}

}

// TestApplyClaimsSelect applyClaimsSelect
func TestApplyClaimsSelect(t *testing.T) {
	var claimTests = []struct {
		name        string
		claims      auth.Claims
		expectedSql string
		error       error
	}{}
	t.Log("Given the need to validate ACLs are enforced by claims to a select query.")
	{
		for i, tt := range claimTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()
				query := selectQuery()
				err := applyClaimsSelect(ctx, tt.claims, query)
				if err != tt.error {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.error)
					t.Fatalf("\t%s\tapplyClaimsSelect failed.", tests.Failed)
				}

				sql, args := query.Build()
				// Use mysql flavor so placeholders will get replaced for comparison.
				sql, err = sqlbuilder.MySQL.Interpolate(sql, args)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tapplyClaimsSelect failed.", tests.Failed)
				}

				if diff := cmp.Diff(sql, tt.expectedSql); diff != "" {
					t.Fatalf("\t%s\tExpected result query to match. Diff:\n%s", tests.Failed, diff)
				}

				t.Logf("\t%s\tapplyClaimsSelect ok.", tests.Success)
			}

		}

	}

}
