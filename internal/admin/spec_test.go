package admin

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/config"
	"github.com/useopendesk/opendesk/internal/core"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// TestSpecHidesAdmin: the admin routes register beside core without a schema clash, and
// stay out of the public OpenAPI spec.
func TestSpecHidesAdmin(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{Env: "development", APIURL: "http://api.test", AppURL: "http://app.test", AdminURL: "http://admin.test",
		AuthSecret: []byte("test-auth-secret-test-auth-secret-0000")}
	d := &deps.Deps{Cfg: cfg, Log: log, Tokens: auth.NewTokens(cfg.AuthSecret), Limiter: httpx.NewLimiter()}
	r := chi.NewRouter()
	hc := huma.DefaultConfig("test", "0")
	hc.OpenAPIPath = "/v1/openapi"
	api := humachi.New(r, hc)
	core.Register(d, api, r)
	Register(d, api, r)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/openapi.json", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("openapi: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "/v1/admin/") {
		t.Fatal("the public spec lists admin routes")
	}
}
