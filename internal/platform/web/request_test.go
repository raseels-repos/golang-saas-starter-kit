package web

import (
	"github.com/google/go-cmp/cmp"
	"testing"
)

func TestExtractWhereArgs(t *testing.T) {

	var queryTests = []struct {
		where    string
		redacted string
		args     []interface{}
	}{
		{
			"name = 'xxxx' or name = :test",
			"name = :redacted1 or name = :test",
			[]interface{}{"xxxx"},
		},
		{
			"name = 'xxxx' or name is null",
			"name = :redacted1 or name is null",
			[]interface{}{"xxxx"},
		},
		{
			"name = 'xxxx' or name in ('yyyy', 'zzzz')",
			"name = :redacted1 or name in ::redacted2",
			[]interface{}{"xxxx", []interface{}{"yyyy", "zzzz"}},
		},
		{
			"id = 3232 or id in (2323, 3239, 483484)",
			"id = :redacted1 or id in ::redacted2",
			[]interface{}{"3232", []interface{}{"2323", "3239", "483484"}},
		},
	}

	t.Log("Given the need to ensure values are correctly extracted from a where query string.")
	{
		for i, tt := range queryTests {
			t.Logf("\tTest: %d\tWhen running test: #%d", i, i)
			{
				res, args, err := ExtractWhereArgs(tt.where)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t\tExtract failed.")
				}

				if res != tt.redacted {
					t.Logf("\t\tGot : %+v", res)
					t.Logf("\t\tWant: %+v", tt.redacted)
					t.Fatalf("\t\tResulting where does not match expected.")
				}

				if diff := cmp.Diff(tt.args, args); diff != "" {
					t.Logf("\t\tGot : %+v", args)
					t.Logf("\t\tWant: %+v", tt.args)
					t.Fatalf("\t\tResulting args does not match expected. Diff:\n%s", diff)
				}

				t.Logf("\t\tOk.")
			}
		}
	}
}
