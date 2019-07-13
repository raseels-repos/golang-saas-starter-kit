package auth_test

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"os"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/tests"
	"github.com/pborman/uuid"
)

var test *tests.Test

// TestMain is the entry point for testing.
func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	tests.DisableDb = true

	test = tests.New()
	defer test.TearDown()

	return m.Run()
}

// TestAuthenticatorFile validates File storage.
func TestAuthenticatorFile(t *testing.T) {

	var authTests = []struct {
		name          string
		now           time.Time
		keyExpiration time.Duration
		error         error
	}{
		{"NoKeyExpiration", time.Now(), time.Duration(0), nil},
		{"KeyExpirationOk", time.Now(), time.Duration(time.Second * 3600), nil},
		{"KeyExpirationDisabled", time.Now().Add(time.Second * 3600 * 3), time.Duration(time.Second * 3600), nil},
	}

	// Generate the token.
	signedClaims := auth.Claims{
		Roles: []string{auth.RoleAdmin},
	}

	t.Log("Given the need to validate initiating a new Authenticator using File storage by key expiration.")
	{
		for i, tt := range authTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				a, err := auth.NewAuthenticatorFile("", tt.now, tt.keyExpiration)
				if err != tt.error {
					t.Log("\t\tGot :", err)
					t.Log("\t\tWant:", tt.error)
					t.Fatalf("\t%s\tNewAuthenticatorFile failed.", tests.Failed)
				}

				tknStr, err := a.GenerateToken(signedClaims)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tGenerateToken failed.", tests.Failed)
				}

				parsedClaims, err := a.ParseClaims(tknStr)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tParseClaims failed.", tests.Failed)
				}

				// Assert expected claims.
				if exp, got := len(signedClaims.Roles), len(parsedClaims.Roles); exp != got {
					t.Log("\t\tGot :", got)
					t.Log("\t\tWant:", exp)
					t.Fatalf("\t%s\tShould got the same number of roles.", tests.Failed)
				}
				if exp, got := signedClaims.Roles[0], parsedClaims.Roles[0]; exp != got {
					t.Log("\t\tGot :", got)
					t.Log("\t\tWant:", exp)
					t.Fatalf("\t%s\tShould got the same role name.", tests.Failed)
				}

				t.Logf("\t%s\tNewAuthenticatorFile ok.", tests.Success)
			}
		}
	}
}

// TestAuthenticatorAws validates AWS storage.
func TestAuthenticatorAws(t *testing.T) {

	awsSecretID := "jwt-key" + uuid.NewRandom().String()

	defer func() {
		// cleanup the secret after test is complete
		sm := secretsmanager.New(test.AwsSession)
		_, err := sm.DeleteSecret(&secretsmanager.DeleteSecretInput{
			SecretId: aws.String(awsSecretID),
		})
		if err != nil {
			t.Fatal(err)
		}
	}()

	var authTests = []struct {
		name          string
		awsSecretID   string
		now           time.Time
		keyExpiration time.Duration
		error         error
	}{
		{"NoKeyExpiration", awsSecretID, time.Now(), time.Duration(0), nil},
		{"KeyExpirationOk", awsSecretID, time.Now(), time.Duration(time.Second * 3600), nil},
		{"KeyExpirationDisabled", awsSecretID, time.Now().Add(time.Second * 3600 * 3), time.Duration(time.Second * 3600), nil},
	}

	// Generate the token.
	signedClaims := auth.Claims{
		Roles: []string{auth.RoleAdmin},
	}

	t.Log("Given the need to validate initiating a new Authenticator using AWS storage by key expiration.")
	{
		for i, tt := range authTests {
			t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
			{
				a, err := auth.NewAuthenticatorAws(test.AwsSession, tt.awsSecretID, tt.now, tt.keyExpiration)
				if err != tt.error {
					t.Log("\t\tGot :", err)
					t.Log("\t\tWant:", tt.error)
					t.Fatalf("\t%s\tNewAuthenticatorAws failed.", tests.Failed)
				}

				tknStr, err := a.GenerateToken(signedClaims)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tGenerateToken failed.", tests.Failed)
				}

				parsedClaims, err := a.ParseClaims(tknStr)
				if err != nil {
					t.Log("\t\tGot :", err)
					t.Fatalf("\t%s\tParseClaims failed.", tests.Failed)
				}

				// Assert expected claims.
				if exp, got := len(signedClaims.Roles), len(parsedClaims.Roles); exp != got {
					t.Log("\t\tGot :", got)
					t.Log("\t\tWant:", exp)
					t.Fatalf("\t%s\tShould got the same number of roles.", tests.Failed)
				}
				if exp, got := signedClaims.Roles[0], parsedClaims.Roles[0]; exp != got {
					t.Log("\t\tGot :", got)
					t.Log("\t\tWant:", exp)
					t.Fatalf("\t%s\tShould got the same role name.", tests.Failed)
				}

				t.Logf("\t%s\tNewAuthenticatorAws ok.", tests.Success)
			}
		}
	}
}
