package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"geeks-accelerator/oss/saas-starter-kit/example-project/cmd/web-api/handlers"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/account"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/auth"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/tests"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/signup"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user"
	"geeks-accelerator/oss/saas-starter-kit/example-project/internal/user_account"
	"github.com/google/go-cmp/cmp"
	"github.com/iancoleman/strcase"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
)

var a http.Handler
var test *tests.Test
var authenticator *auth.Authenticator

// Information about the users we have created for testing.
type roleTest struct {
	Role             string
	Token            user.Token
	Claims           auth.Claims
	User             mockUser
	Account          *account.Account
	ForbiddenUser    mockUser
	ForbiddenAccount *account.Account
}

type requestTest struct {
	name       string
	method     string
	url        string
	request    interface{}
	token      user.Token
	claims     auth.Claims
	statusCode int
	error      interface{}
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

	var err error
	authenticator, err = auth.NewAuthenticatorMemory(now)
	if err != nil {
		panic(err)
	}

	shutdown := make(chan os.Signal, 1)

	log := test.Log
	log.SetOutput(ioutil.Discard)
	a = handlers.API(shutdown, log, test.MasterDB, nil, authenticator)

	// Create a new account directly business logic. This creates an
	// initial account and user that we will use for admin validated endpoints.
	signupReq1 := mockSignupRequest()
	signup1, err := signup.Signup(tests.Context(), auth.Claims{}, test.MasterDB, signupReq1, now)
	if err != nil {
		panic(err)
	}

	expires := time.Now().UTC().Sub(signup1.User.CreatedAt) + time.Hour
	adminTkn, err := user.Authenticate(tests.Context(), test.MasterDB, authenticator, signupReq1.User.Email, signupReq1.User.Password, expires, now)
	if err != nil {
		panic(err)
	}

	adminClaims, err := authenticator.ParseClaims(adminTkn.AccessToken)
	if err != nil {
		panic(err)
	}

	// Create a second account that the first account user should not have access to.
	signupReq2 := mockSignupRequest()
	signup2, err := signup.Signup(tests.Context(), auth.Claims{}, test.MasterDB, signupReq2, now)
	if err != nil {
		panic(err)
	}

	// First test will be for role Admin
	roleTests[auth.RoleAdmin] = roleTest{
		Role:             auth.RoleAdmin,
		Token:            adminTkn,
		Claims:           adminClaims,
		User:             mockUser{signup1.User, signupReq1.User.Password},
		Account:          signup1.Account,
		ForbiddenUser:    mockUser{signup2.User, signupReq2.User.Password},
		ForbiddenAccount: signup2.Account,
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
		UserID:    usr.ID,
		AccountID: signup1.Account.ID,
		Roles:     []user_account.UserAccountRole{user_account.UserAccountRole_User},
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

	// Second test will be for role User
	roleTests[auth.RoleUser] = roleTest{
		Role:             auth.RoleUser,
		Token:            userTkn,
		Claims:           userClaims,
		Account:          signup1.Account,
		User:             mockUser{usr, userReq.Password},
		ForbiddenUser:    mockUser{signup2.User, signupReq2.User.Password},
		ForbiddenAccount: signup2.Account,
	}

	return m.Run()
}

// executeRequestTest provides request execution and basic response validation
func executeRequestTest(t *testing.T, tt requestTest, ctx context.Context) (*httptest.ResponseRecorder, bool) {
	var req []byte
	var rr io.Reader
	if tt.request != nil {
		var ok bool
		req, ok = tt.request.([]byte)
		if !ok {
			var err error
			req, err = json.Marshal(tt.request)
			if err != nil {
				t.Logf("\t\tGot err : %+v", err)
				t.Logf("\t\tEncode request failed.")
				return nil, false
			}
		}
		rr = bytes.NewReader(req)
	}

	r := httptest.NewRequest(tt.method, tt.url, rr).WithContext(ctx)

	w := httptest.NewRecorder()

	r.Header.Set("Content-Type", web.MIMEApplicationJSONCharsetUTF8)
	if tt.token.AccessToken != "" {
		r.Header.Set("Authorization", tt.token.AuthorizationHeader())
	}

	a.ServeHTTP(w, r)

	if w.Code != tt.statusCode {
		t.Logf("\t\tRequest : %s\n", string(req))
		t.Logf("\t\tBody : %s\n", w.Body.String())
		t.Logf("\t\tShould receive a status code of %d for the response : %v", tt.statusCode, w.Code)
		return w, false
	}

	if tt.error != nil {
		var actual web.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &actual); err != nil {
			t.Logf("\t\tBody : %s\n", w.Body.String())
			t.Logf("\t\tGot error : %+v", err)
			t.Logf("\t\tShould get the expected error.")
			return w, false
		}

		if diff := cmp.Diff(actual, tt.error); diff != "" {
			t.Logf("\t\tDiff : %s\n", diff)
			t.Logf("\t\tShould get the expected error.")
			return w, false
		}
	}

	return w, true
}

// decodeMapToStruct used to covert map to json struct so don't have a bunch of raw json strings running around test files.
func decodeMapToStruct(expectedMap map[string]interface{}, expected interface{}) error {
	expectedJson, err := json.Marshal(expectedMap)
	if err != nil {
		return errors.WithStack(err)
	}

	if err := json.Unmarshal([]byte(expectedJson), &expected); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// cmpDiff prints out the raw json to help with debugging.
func cmpDiff(t *testing.T, actual, expected interface{}) bool {
	if actual == nil && expected == nil {
		return false
	}
	if diff := cmp.Diff(actual, expected); diff != "" {
		actualJSON, err := json.MarshalIndent(actual, "", "    ")
		if err != nil {
			t.Fatalf("\t%s\tGot error : %+v", tests.Failed, err)
		}
		t.Logf("\t\tGot : %s\n", actualJSON)

		expectedJSON, err := json.MarshalIndent(expected, "", "    ")
		if err != nil {
			t.Fatalf("\t%s\tGot error : %+v", tests.Failed, err)
		}
		t.Logf("\t\tExpected : %s\n", expectedJSON)

		t.Logf("\t\tDiff : %s\n", diff)

		return true
	}
	return false
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
				printResultMapKeys(ctx, smv, depth+1)
			}

			fmt.Printf("%s},\n", strings.Repeat("\t", depth))
		} else if sm, ok := kv.(map[string]interface{}); ok {
			fmt.Printf("%s\"%s\": map[string]interface{}{\n", strings.Repeat("\t", depth), k)
			printResultMapKeys(ctx, sm, depth+1)
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
