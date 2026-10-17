package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
)

// Every area registers on one API, as the binary does at boot: schema names must not
// collide across packages, and the spec must build.
func TestRouterRegistersEveryArea(t *testing.T) {
	d := hdtest.Deps(t)
	h := Router(d)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/openapi.json", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("openapi %d", w.Code)
	}
}
