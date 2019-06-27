package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"github.com/google/go-cmp/cmp"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/signup"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user_account"
	"github.com/iancoleman/strcase"
	"github.com/pborman/uuid"
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
	SignupResult *signup.SignupResult
	User           *user.User
	Account        *account.Account
}

type requestTest struct {
	name string
	method string
	url string
	request           interface{}
	token          user.Token
	claims         auth.Claims
	statusCode     int
	error          interface{}
	expected       func(req requestTest, result []byte) bool
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
		SignupResult: signup,
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

	_, err = user_account.Create(tests.Context(), adminClaims, test.MasterDB, user_account.UserAccountCreateRequest{
		UserID:            usr.ID,
		AccountID:         signup.Account.ID,
		Roles:       []user_account.UserAccountRole{user_account.UserAccountRole_User},
		// Status: use default value
	}, now)
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
		SignupResult: signup,
		Account:        signup.Account,
		User:           usr,
	}

	return m.Run()
}


// runRequestTests helper function for testing endpoints.
func runRequestTests(t *testing.T, rtests []requestTest ) {

	for i, tt := range rtests {
		t.Logf("\tTest: %d\tWhen running test: %s", i, tt.name)
		{
			var req []byte
			var rr io.Reader
			if  tt.request != nil {
				var ok bool
				req, ok = tt.request.([]byte)
				if !ok {
					var err error
					req, err = json.Marshal(tt.request)
					if err != nil {
						t.Logf("\t\tGot err : %+v", err)
						t.Fatalf("\t%s\tEncode request failed.", tests.Failed)
					}
				}
				rr = bytes.NewReader(req)
			}

			r := httptest.NewRequest(tt.method, tt.url , rr)
			w := httptest.NewRecorder()

			r.Header.Set("Content-Type", web.MIMEApplicationJSONCharsetUTF8)
			if tt.token.AccessToken != "" {
				r.Header.Set("Authorization", tt.token.AuthorizationHeader())
			}

			a.ServeHTTP(w, r)

			if w.Code != tt.statusCode {
				t.Logf("\t\tRequest : %s\n", string(req))
				t.Logf("\t\tBody : %s\n", w.Body.String())
				t.Fatalf("\t%s\tShould receive a status code of %d for the response : %v", tests.Failed, tt.statusCode, w.Code)
			}
			t.Logf("\t%s\tReceived valid status code of %d.", tests.Success, w.Code)

			if tt.error != nil {



				var actual web.ErrorResponse
				if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
					t.Logf("\t\tBody : %s\n", w.Body.String())
					t.Logf("\t\tGot error : %+v", err)
					t.Fatalf("\t%s\tShould get the expected error.", tests.Failed)
				}

				if diff := cmp.Diff(actual, tt.error); diff != "" {
					t.Logf("\t\tDiff : %s\n", diff)
					t.Fatalf("\t%s\tShould get the expected error.", tests.Failed)
				}
			}

			if ok := tt.expected(tt, w.Body.Bytes()); !ok {
				t.Fatalf("\t%s\tShould get the expected result.", tests.Failed)
			}
			t.Logf("\t%s\tReceived expected result.", tests.Success)
		}
	}
}


func printResultMap(ctx context.Context, result []byte) {
	var m map[string]interface{}
	if err := json.Unmarshal(result, &m); err != nil {
		panic(err)
	}

	fmt.Println(`map[string]interface{}{`)
	printResultMapKeys(ctx, m, 1)
	fmt.Println(`}`)
}

func printResultMapKeys(ctx context.Context, m map[string]interface{}, depth int) {
	var isEnum bool
	if m["value"] != nil && m["title"] != nil && m["options"] != nil {
		isEnum = true
	}

	for k, kv := range m {
		fn := strcase.ToCamel(k)

		switch k {
		case "created_at", "updated_at", "archived_at":
			pv := fmt.Sprintf("web.NewTimeResponse(ctx, actual.%s)", fn)
			fmt.Printf("%s\"%s\": %s,\n", strings.Repeat("\t", depth), k, pv)
			continue
		}

		if sm, ok := kv.([]map[string]interface{}); ok {
			fmt.Printf("%s\"%s\": []map[string]interface{}{\n", strings.Repeat("\t", depth), k)

			for _, smv := range sm {
				printResultMapKeys(ctx, smv, depth +1)
			}

			fmt.Printf("%s},\n", strings.Repeat("\t", depth))
		} else if sm, ok := kv.(map[string]interface{}); ok {
			fmt.Printf("%s\"%s\": map[string]interface{}{\n", strings.Repeat("\t", depth), k)
			printResultMapKeys(ctx, sm, depth +1)
			fmt.Printf("%s},\n", strings.Repeat("\t", depth))
		} else {
			var pv string
			if isEnum {
				jv, _ := json.Marshal(kv)
				pv = string(jv)
			} else {
				pv = fmt.Sprintf("req.%s", fn)
			}

			fmt.Printf("%s\"%s\": %s,\n", strings.Repeat("\t", depth), k, pv)
		}
	}
}

