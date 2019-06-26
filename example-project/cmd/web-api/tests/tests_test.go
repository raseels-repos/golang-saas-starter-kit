package tests

import (
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/signup"
	"github.com/pborman/uuid"
	"net/http"
	"os"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/cmd/web-api/handlers"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user"
)

var a http.Handler
var test *tests.Test

// Information about the users we have created for testing.
type roleTest struct {
	Token          user.Token
	Claims         auth.Claims
	SignupRequest  *signup.SignupRequest
	SignupResponse *signup.SignupResponse
	User           *user.User
	Account        *account.Account
}

var roleTests map[string]roleTest

func init() {
	roleTests = make(map[string]roleTest)
}

// TestMain is the entry point for testing.
func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	test = tests.New()
	defer test.TearDown()

	now := time.Date(2018, time.October, 1, 0, 0, 0, 0, time.UTC)

	authenticator, err := auth.NewAuthenticatorMemory(now)
	if err != nil {
		panic(err)
	}

	shutdown := make(chan os.Signal, 1)
	a = handlers.API(shutdown, test.Log, test.MasterDB, nil, authenticator)

	// Create a new account directly business logic. This creates an
	// initial account and user that we will use for admin validated endpoints.
	signupReq := signup.SignupRequest{
		Account: signup.SignupAccount{
			Name:     uuid.NewRandom().String(),
			Address1: "103 East Main St",
			Address2: "Unit 546",
			City:     "Valdez",
			Region:   "AK",
			Country:  "USA",
			Zipcode:  "99686",
		},
		User: signup.SignupUser{
			Name:            "Lee Brown",
			Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
			Password:        "akTechFr0n!ier",
			PasswordConfirm: "akTechFr0n!ier",
		},
	}
	signup, err := signup.Signup(tests.Context(), auth.Claims{}, test.MasterDB, signupReq, now)
	if err != nil {
		panic(err)
	}

	expires := time.Now().UTC().Sub(signup.User.CreatedAt) + time.Hour
	adminTkn, err := user.Authenticate(tests.Context(), test.MasterDB, authenticator, signupReq.User.Email, signupReq.User.Password, expires, now)
	if err != nil {
		panic(err)
	}

	adminClaims, err := authenticator.ParseClaims(adminTkn.AccessToken)
	if err != nil {
		panic(err)
	}

	roleTests[auth.RoleAdmin] = roleTest{
		Token:          adminTkn,
		Claims:         adminClaims,
		SignupRequest:  &signupReq,
		SignupResponse: signup,
		User:           signup.User,
		Account:        signup.Account,
	}

	// Create a regular user to use when calling regular validated endpoints.
	userReq := user.UserCreateRequest{
		Name:            "Lucas Brown",
		Email:           uuid.NewRandom().String() + "@geeksinthewoods.com",
		Password:        "akTechFr0n!ier",
		PasswordConfirm: "akTechFr0n!ier",
	}
	usr, err := user.Create(tests.Context(), adminClaims, test.MasterDB, userReq, now)
	if err != nil {
		panic(err)
	}

	userTkn, err := user.Authenticate(tests.Context(), test.MasterDB, authenticator, usr.Email, userReq.Password, expires, now)
	if err != nil {
		panic(err)
	}

	userClaims, err := authenticator.ParseClaims(userTkn.AccessToken)
	if err != nil {
		panic(err)
	}

	roleTests[auth.RoleUser] = roleTest{
		Token:          userTkn,
		Claims:         userClaims,
		SignupRequest:  &signupReq,
		SignupResponse: signup,
		Account:        signup.Account,
		User:           usr,
	}

	return m.Run()
}
