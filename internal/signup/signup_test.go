package signup

import (
	"geeks-accelerator/oss/saas-starter-kit/internal/user_auth"
	"os"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"github.com/google/go-cmp/cmp"
	"github.com/pborman/uuid"
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

// TestSignupValidation ensures all the validation tags work on Signup
func TestSignupValidation(t *testing.T) {

	var userTests = []struct {
		name     string
		req      SignupRequest
		expected func(req SignupRequest, res *SignupResult) *SignupResult
		error    error
	}{
		{"Required Fields",
			SignupRequest{},
			func(req SignupRequest, res *SignupResult) *SignupResult {
				return nil
			},
			errors.New("Key: 'SignupRequest.{{account}}.{{name}}' Error:Field validation for '{{name}}' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.{{account}}.{{address1}}' Error:Field validation for '{{address1}}' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.{{account}}.{{city}}' Error:Field validation for '{{city}}' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.{{account}}.{{region}}' Error:Field validation for '{{region}}' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.{{account}}.{{country}}' Error:Field validation for '{{country}}' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.{{account}}.{{zipcode}}' Error:Field validation for '{{zipcode}}' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.{{user}}.{{first_name}}' Error:Field validation for '{{first_name}}' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.{{user}}.{{last_name}}' Error:Field validation for '{{last_name}}' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.{{user}}.{{email}}' Error:Field validation for '{{email}}' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.{{user}}.{{password}}' Error:Field validation for '{{password}}' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.{{user}}.{{password_confirm}}' Error:Field validation for '{{password_confirm}}' failed on the 'required' tag"),
		},
	}

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	t.Log("Given the need ensure all validation tags are working for signup.")
	{
		for i, tt := range userTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				ctx := tests.Context()

				res, err := Signup(ctx, auth.Claims{}, test.MasterDB, tt.req, now)
				if err != tt.error {
					// TODO: need a better way to handle validation errors as they are
					// 		 of type interface validator.ValidationErrorsTranslations
					var errStr string
					if err != nil {
						errStr = err.Error()
					}
					var expectStr string
					if tt.error != nil {
						expectStr = tt.error.Error()
					}
					if errStr != expectStr {
						t.Logf("\t\tGot : %+v", err)
						t.Logf("\t\tWant: %+v", tt.error)
						t.Fatalf("\t%s\tSignup failed.", tests.Failed)
					}
				}

				// If there was an error that was expected, then don't go any further
				if tt.error != nil {
					t.Logf("\t%s\tSignup ok.", tests.Success)
					continue
				}

				expected := tt.expected(tt.req, res)
				if diff := cmp.Diff(res, expected); diff != "" {
					t.Fatalf("\t%s\tExpected result should match. Diff:\n%s", tests.Failed, diff)
				}

				t.Logf("\t%s\tSignup ok.", tests.Success)
			}
		}
	}
}

// TestSignupFull validates Signup and ensures the created user can login.
func TestSignupFull(t *testing.T) {

	req := SignupRequest{
		Account: SignupAccount{
			Name:     uuid.NewRandom().String(),
			Address1: "103 East Main St",
			Address2: "Unit 546",
			City:     "Valdez",
			Region:   "AK",
			Country:  "USA",
			Zipcode:  "99686",
		},
		User: SignupUser{
			FirstName:            "Lee",
			LastName:            "Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
	}

	ctx := tests.Context()

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	tknGen := &auth.MockTokenGenerator{}

	t.Log("Given the need to ensure signup works.")
	{
		res, err := Signup(ctx, auth.Claims{}, test.MasterDB, req, now)
		if err != nil {
			t.Logf("\t\tGot error : %+v", err)
			t.Fatalf("\t%s\tSignup failed.", tests.Failed)
		}

		if res.User == nil || res.User.ID == "" {
			t.Fatalf("\t%s\tResponse user is empty.", tests.Failed)
		}

		if res.Account == nil || res.Account.ID == "" {
			t.Fatalf("\t%s\tResponse account is empty.", tests.Failed)
		}

		if res.Account.SignupUserID.String == "" {
			t.Fatalf("\t%s\tResponse account signup user ID is empty.", tests.Failed)
		} else if res.Account.SignupUserID.String != res.User.ID {
			t.Logf("\t\tGot : %+v", res.Account.SignupUserID.String)
			t.Logf("\t\tWant: %+v", res.User.ID)
			t.Fatalf("\t%s\tSigup user ID does not match created user ID.", tests.Failed)
		}

		if res.Account.BillingUserID.String == "" {
			t.Fatalf("\t%s\tResponse account billing user ID is empty.", tests.Failed)
		} else if res.Account.BillingUserID.String != res.User.ID {
			t.Logf("\t\tGot : %+v", res.Account.BillingUserID.String)
			t.Logf("\t\tWant: %+v", res.User.ID)
			t.Fatalf("\t%s\tBilling user ID does not match created user ID.", tests.Failed)
		}

		t.Logf("\t%s\tSignup ok.", tests.Success)

		// Verify that the user can be authenticated with the updated password.
		_, err = user_auth.Authenticate(ctx, test.MasterDB, tknGen, user_auth.AuthenticateRequest{
			Email:res.User.Email,
			Password: req.User.Password,
		}, time.Hour, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tAuthenticate failed.", tests.Failed)
		}
		t.Logf("\t%s\tAuthenticate ok.", tests.Success)
	}
}
