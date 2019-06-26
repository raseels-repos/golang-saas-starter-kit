package signup

import (
	"os"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user"
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
		expected func(req SignupRequest, res *SignupResponse) *SignupResponse
		error    error
	}{
		{"Required Fields",
			SignupRequest{},
			func(req SignupRequest, res *SignupResponse) *SignupResponse {
				return nil
			},
			errors.New("Key: 'SignupRequest.Account.Name' Error:Field validation for 'Name' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.Account.Address1' Error:Field validation for 'Address1' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.Account.City' Error:Field validation for 'City' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.Account.Region' Error:Field validation for 'Region' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.Account.Country' Error:Field validation for 'Country' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.Account.Zipcode' Error:Field validation for 'Zipcode' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.User.Name' Error:Field validation for 'Name' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.User.Email' Error:Field validation for 'Email' failed on the 'required' tag\n" +
				"Key: 'SignupRequest.User.Password' Error:Field validation for 'Password' failed on the 'required' tag"),
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
			Name:            "Lee Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
	}

	ctx := tests.Context()

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	tknGen := &user.MockTokenGenerator{}

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
		_, err = user.Authenticate(ctx, test.MasterDB, tknGen, res.User.Email, req.User.Password, time.Hour, now)
		if err != nil {
			t.Log("\t\tGot :", err)
			t.Fatalf("\t%s\tAuthenticate failed.", tests.Failed)
		}
		t.Logf("\t%s\tAuthenticate ok.", tests.Success)
	}
}
