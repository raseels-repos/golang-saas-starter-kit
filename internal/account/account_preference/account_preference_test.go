package account_preference

import (
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/internal/user_account"
	"github.com/dgrijalva/jwt-go"
	"github.com/google/go-cmp/cmp"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
)

var (
	test *tests.Test
	repo *Repository
)

// TestMain is the entry point for testing.
func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	test = tests.New()
	defer test.TearDown()

	repo = NewRepository(test.MasterDB)

	return m.Run()
}

// TestSetValidation ensures all the validation tags work on Set.
func TestSetValidation(t *testing.T) {

	invalidName := AccountPreferenceName("xxxxxx")

	var prefTests = []struct {
		name  string
		req   AccountPreferenceSetRequest
		error error
	}{
		{"Required Fields",
			AccountPreferenceSetRequest{},
			errors.New("Key: 'AccountPreferenceSetRequest.{{account_id}}' Error:Field validation for '{{account_id}}' failed on the 'required' tag\n" +
				"Key: 'AccountPreferenceSetRequest.{{name}}' Error:Field validation for '{{name}}' failed on the 'required' tag\n" +
				"Key: 'AccountPreferenceSetRequest.{{value}}' Error:Field validation for '{{value}}' failed on the 'required' tag"),
		},
		{"Valid Name",
			AccountPreferenceSetRequest{
				AccountID: uuid.NewRandom().String(),
				Name:      invalidName,
				Value:     uuid.NewRandom().String(),
			},
			errors.New("Key: 'AccountPreferenceSetRequest.{{name}}' Error:Field validation for '{{name}}' failed on the 'oneof' tag\n" +
				"Key: 'AccountPreferenceSetRequest.{{value}}' Error:Field validation for '{{value}}' failed on the 'preference_value' tag"),
		},
	}

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure all validation tags are working for account preference set.")
	{
		for i, tt := range prefTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				err := repo.Set(ctx, auth.Claims{}, tt.req, now)
				if err != tt.error {
					// TODO: need a better way to handle validation errors as they are
					// 		 of type interface validator.ValidationErrorsTranslations
					var errStr string
					if err != nil {
						errStr = strings.Replace(err.Error(), "{{", "", -1)
						errStr = strings.Replace(errStr, "}}", "", -1)
					}
					var expectStr string
					if tt.error != nil {
						expectStr = strings.Replace(tt.error.Error(), "{{", "", -1)
						expectStr = strings.Replace(expectStr, "}}", "", -1)
					}
					if errStr != expectStr {
						t.Logf("\t\tGot : %+v", errStr)
						t.Logf("\t\tWant: %+v", expectStr)
						t.Fatalf("\t%s\tSet failed.", tests.Failed)
					}
				}

				// If there was an error that was expected, then don't go any further
				if tt.error != nil {
					t.Logf("\t%s\tSet ok.", tests.Success)
					continue
				}

				t.Logf("\t%s\tSet ok.", tests.Success)
			}
		}
	}
}

// TestCrud validates the full set of CRUD operations for account preferences and ensures ACLs are correctly applied
// by claims.
func TestCrud(t *testing.T) {
	defer tests.Recover(t)

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	// Create a test user and account.
	usrAcc, err := user_account.MockUserAccount(tests.Context(), test.MasterDB, now, user_account.UserAccountRole_Admin)
	if err != nil {
		t.Log("Got :", err)
		t.Fatalf("%s\tCreate account failed.", tests.Failed)
	}

	type prefTest struct {
		name     string
		claims   func(string, string) auth.Claims
		set      AccountPreferenceSetRequest
		writeErr error
		findErr  error
	}

	var prefTests []prefTest

	// Internal request, should bypass ACL.
	prefTests = append(prefTests, prefTest{"EmptyClaims",
		func(accountID, userId string) auth.Claims {
			return auth.Claims{}
		},
		AccountPreferenceSetRequest{
			AccountID: usrAcc.AccountID,
			Name:      AccountPreference_Datetime_Format,
			Value:     AccountPreference_Datetime_Format_Default,
		},
		nil,
		nil,
	})

	// Role of account but claim account does not match update account so forbidden.
	prefTests = append(prefTests, prefTest{"RoleAccountPreferenceDiffAccountPreference",
		func(accountID, userId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Audience: uuid.NewRandom().String(),
					Subject:  userId,
				},
			}
		},
		AccountPreferenceSetRequest{
			AccountID: usrAcc.AccountID,
			Name:      AccountPreference_Datetime_Format,
			Value:     AccountPreference_Datetime_Format_Default,
		},
		account.ErrForbidden,
		ErrNotFound,
	})

	// Role of account AND claim account matches update account so OK.
	prefTests = append(prefTests, prefTest{"RoleAccountPreferenceSameAccountPreference",
		func(accountID, userId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Audience: accountID,
					Subject:  userId,
				},
			}
		},
		AccountPreferenceSetRequest{
			AccountID: usrAcc.AccountID,
			Name:      AccountPreference_Date_Format,
			Value:     AccountPreference_Date_Format_Default,
		},
		nil,
		nil,
	})

	// Role of admin but claim account does not match update account so forbidden.
	prefTests = append(prefTests, prefTest{"RoleAdminDiffAccountPreference",
		func(accountID, userID string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Audience: uuid.NewRandom().String(),
					Subject:  uuid.NewRandom().String(),
				},
			}
		},
		AccountPreferenceSetRequest{
			AccountID: usrAcc.AccountID,
			Name:      AccountPreference_Time_Format,
			Value:     AccountPreference_Time_Format_Default,
		},
		account.ErrForbidden,
		ErrNotFound,
	})

	// Role of admin and claim account matches update account so ok.
	prefTests = append(prefTests, prefTest{"RoleAdminSameAccountPreference",
		func(accountID, userId string) auth.Claims {
			return auth.Claims{
				Roles: []string{auth.RoleAdmin},
				StandardClaims: jwt.StandardClaims{
					Audience: uuid.NewRandom().String(),
					Subject:  userId,
				},
			}
		},
		AccountPreferenceSetRequest{
			AccountID: usrAcc.AccountID,
			Name:      AccountPreference_Time_Format,
			Value:     AccountPreference_Time_Format_Default,
		},
		account.ErrForbidden,
		ErrNotFound,
	})

	t.Log("Given the need to ensure claims are applied as ACL for set account preference.")
	{

		for i, tt := range prefTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				err := repo.Set(ctx, tt.claims(usrAcc.AccountID, usrAcc.UserID), tt.set, now)
				if err != nil && errors.Cause(err) != tt.writeErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.writeErr)
					t.Fatalf("\t%s\tFind failed.", tests.Failed)
				}

				// If user doesn't have access to set, create one anyways to test the other endpoints.
				if tt.writeErr != nil {
					err := repo.Set(ctx, auth.Claims{}, tt.set, now)
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tCreate failed.", tests.Failed)
					}
				}

				// Find the account and make sure the set where made.
				readRes, err := repo.Read(ctx, tt.claims(usrAcc.AccountID, usrAcc.UserID), AccountPreferenceReadRequest{
					AccountID: tt.set.AccountID,
					Name:      tt.set.Name,
				})
				if err != nil && errors.Cause(err) != tt.findErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.findErr)
					t.Fatalf("\t%s\tFind failed.", tests.Failed)
				} else if tt.findErr == nil {
					findExpected := &AccountPreference{
						AccountID: tt.set.AccountID,
						Name:      tt.set.Name,
						Value:     tt.set.Value,
						CreatedAt: readRes.CreatedAt,
						UpdatedAt: readRes.UpdatedAt,
					}

					if diff := cmp.Diff(readRes, findExpected); diff != "" {
						t.Fatalf("\t%s\tExpected find result to match update. Diff:\n%s", tests.Failed, diff)
					}
					t.Logf("\t%s\tRead ok.", tests.Success)
				}

				// Archive (soft-delete) the account.
				err = repo.Archive(ctx, tt.claims(usrAcc.AccountID, usrAcc.UserID), AccountPreferenceArchiveRequest{
					AccountID: tt.set.AccountID,
					Name:      tt.set.Name,
				}, now)
				if err != nil && errors.Cause(err) != tt.writeErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.writeErr)
					t.Fatalf("\t%s\tArchive failed.", tests.Failed)
				} else if tt.findErr == nil {
					// Trying to find the archived account with the includeArchived false should result in not found.
					_, err = repo.Read(ctx, tt.claims(usrAcc.AccountID, usrAcc.UserID), AccountPreferenceReadRequest{
						AccountID: tt.set.AccountID,
						Name:      tt.set.Name,
					})
					if err != nil && errors.Cause(err) != ErrNotFound {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", ErrNotFound)
						t.Fatalf("\t%s\tArchive Read failed.", tests.Failed)
					}

					// Trying to find the archived account with the includeArchived true should result no error.
					_, err = repo.Read(ctx, tt.claims(usrAcc.AccountID, usrAcc.UserID), AccountPreferenceReadRequest{
						AccountID:       tt.set.AccountID,
						Name:            tt.set.Name,
						IncludeArchived: true,
					})
					if err != nil {
						t.Log("\t\tGot :", err)
						t.Fatalf("\t%s\tArchive Read failed.", tests.Failed)
					}
				}
				t.Logf("\t%s\tArchive ok.", tests.Success)

				// Delete (hard-delete) the account.
				err = repo.Delete(ctx, tt.claims(usrAcc.AccountID, usrAcc.UserID), AccountPreferenceDeleteRequest{
					AccountID: tt.set.AccountID,
					Name:      tt.set.Name,
				})
				if err != nil && errors.Cause(err) != tt.writeErr {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.writeErr)
					t.Fatalf("\t%s\tDelete failed.", tests.Failed)
				} else if tt.writeErr == nil {
					// Trying to find the deleted account with the includeArchived true should result in not found.
					_, err = repo.Read(ctx, tt.claims(usrAcc.AccountID, usrAcc.UserID), AccountPreferenceReadRequest{
						AccountID:       tt.set.AccountID,
						Name:            tt.set.Name,
						IncludeArchived: true,
					})
					if errors.Cause(err) != ErrNotFound {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", ErrNotFound)
						t.Fatalf("\t%s\tDelete Read failed.", tests.Failed)
					}
				}
				t.Logf("\t%s\tDelete ok.", tests.Success)
			}
		}
	}
}

// TestFind validates all the request params are correctly parsed into a select query.
func TestFind(t *testing.T) {

	now := time.Now().Add(time.Hour * -1).UTC()

	// Create a test user and account.
	usrAcc, err := user_account.MockUserAccount(tests.Context(), test.MasterDB, now, user_account.UserAccountRole_Admin)
	if err != nil {
		t.Log("Got :", err)
		t.Fatalf("%s\tCreate account failed.", tests.Failed)
	}

	startTime := now.Truncate(time.Millisecond)
	var endTime time.Time

	reqs := []AccountPreferenceSetRequest{
		{
			AccountID: usrAcc.AccountID,
			Name:      AccountPreference_Datetime_Format,
			Value:     AccountPreference_Datetime_Format_Default,
		},
		{
			AccountID: usrAcc.AccountID,
			Name:      AccountPreference_Date_Format,
			Value:     AccountPreference_Date_Format_Default,
		},
		{
			AccountID: usrAcc.AccountID,
			Name:      AccountPreference_Time_Format,
			Value:     AccountPreference_Time_Format_Default,
		},
	}

	var prefs []*AccountPreference
	for idx, req := range reqs {
		err = repo.Set(tests.Context(), auth.Claims{}, req, now.Add(time.Second*time.Duration(idx)))
		if err != nil {
			t.Logf("\t\tGot : %+v", err)
			t.Logf("\t\tRequest : %+v", req)
			t.Fatalf("\t%s\tSet failed.", tests.Failed)
		}

		pref, err := repo.Read(tests.Context(), auth.Claims{}, AccountPreferenceReadRequest{
			AccountID: req.AccountID,
			Name:      req.Name,
		})
		if err != nil {
			t.Logf("\t\tGot : %+v", err)
			t.Logf("\t\tRequest : %+v", req)
			t.Fatalf("\t%s\tSet failed.", tests.Failed)
		}

		prefs = append(prefs, pref)
		endTime = pref.CreatedAt
	}

	type accountTest struct {
		name     string
		req      AccountPreferenceFindRequest
		expected []*AccountPreference
		error    error
	}

	var prefTests []accountTest

	createdFilter := "created_at BETWEEN ? AND ?"

	// Test sort accounts.
	prefTests = append(prefTests, accountTest{"Find all order by created_at asc",
		AccountPreferenceFindRequest{
			Where: createdFilter,
			Args:  []interface{}{startTime, endTime},
			Order: []string{"created_at"},
		},
		prefs,
		nil,
	})

	// Test reverse sorted accounts.
	var expected []*AccountPreference
	for i := len(prefs) - 1; i >= 0; i-- {
		expected = append(expected, prefs[i])
	}
	prefTests = append(prefTests, accountTest{"Find all order by created_at desc",
		AccountPreferenceFindRequest{
			Where: createdFilter,
			Args:  []interface{}{startTime, endTime},
			Order: []string{"created_at desc"},
		},
		expected,
		nil,
	})

	// Test limit.
	var limit uint = 2
	prefTests = append(prefTests, accountTest{"Find limit",
		AccountPreferenceFindRequest{
			Where: createdFilter,
			Args:  []interface{}{startTime, endTime},
			Order: []string{"created_at"},
			Limit: &limit,
		},
		prefs[0:2],
		nil,
	})

	// Test offset.
	var offset uint = 1
	prefTests = append(prefTests, accountTest{"Find limit, offset",
		AccountPreferenceFindRequest{
			Where:  createdFilter,
			Args:   []interface{}{startTime, endTime},
			Order:  []string{"created_at"},
			Limit:  &limit,
			Offset: &offset,
		},
		prefs[1:3],
		nil,
	})

	// Test where filter.
	whereParts := []string{}
	whereArgs := []interface{}{startTime, endTime}
	expected = []*AccountPreference{}
	for i := 0; i < len(prefs); i++ {
		if rand.Intn(100) < 50 {
			continue
		}
		u := *prefs[i]

		whereParts = append(whereParts, "name = ?")
		whereArgs = append(whereArgs, u.Name)
		expected = append(expected, &u)
	}

	prefTests = append(prefTests, accountTest{"Find where",
		AccountPreferenceFindRequest{
			Where: createdFilter + " AND (" + strings.Join(whereParts, " OR ") + ")",
			Args:  whereArgs,
			Order: []string{"created_at"},
		},
		expected,
		nil,
	})

	t.Log("Given the need to ensure find account preferences returns the expected results.")
	{
		for i, tt := range prefTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				res, err := repo.Find(ctx, auth.Claims{}, tt.req)
				if errors.Cause(err) != tt.error {
					t.Logf("\t\tGot : %+v", err)
					t.Logf("\t\tWant: %+v", tt.error)
					t.Fatalf("\t%s\tFind failed.", tests.Failed)
				} else if diff := cmp.Diff(res, tt.expected); diff != "" {
					t.Logf("\t\tGot: %d items", len(res))
					t.Logf("\t\tWant: %d items", len(tt.expected))

					for _, u := range res {
						t.Logf("\t\tGot: %s ID", u.Name)
					}
					for _, u := range tt.expected {
						t.Logf("\t\tExpected: %s ID", u.Name)
					}

					t.Fatalf("\t%s\tExpected find result to match expected. Diff:\n%s", tests.Failed, diff)
				}
				t.Logf("\t%s\tFind ok.", tests.Success)
			}
		}
	}
}
