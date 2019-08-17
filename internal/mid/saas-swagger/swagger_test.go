package saasSwagger

import (
	"io/ioutil"
	"log"
	"net/http/httptest"
	"os"
	"testing"

	_ "geeks-accelerator/oss/saas-starter-kit/internal/mid/saas-swagger/example/docs"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web/webcontext"
	"github.com/stretchr/testify/assert"
)

func TestWrapHandler(t *testing.T) {

	log := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)
	log.SetOutput(ioutil.Discard)

	app := web.NewApp(nil, log, webcontext.Env_Dev)
	app.Handle("GET", "/swagger/*", WrapHandler)

	w1 := performRequest("GET", "/swagger/index.html", app)
	assert.Equal(t, 200, w1.Code)

	w2 := performRequest("GET", "/swagger/doc.json", app)
	assert.Equal(t, 200, w2.Code)

	w3 := performRequest("GET", "/swagger/favicon-16x16.png", app)
	assert.Equal(t, 200, w3.Code)

	w4 := performRequest("GET", "/swagger/notfound", app)
	assert.Equal(t, 404, w4.Code)
}

func performRequest(method, target string, app *web.App) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, r)

	return w
}
