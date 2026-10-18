package adminui

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	h := Handler(RuntimeConfig{APIURL: "https://desk.example.com", AppURL: "https://desk.example.com"}, "default-src 'self'")
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	if w := get("/admin"); w.Code != 301 || w.Header().Get("Location") != "/admin/" {
		t.Fatalf("/admin: %d %v", w.Code, w.Header())
	}
	w := get("/admin/config.js")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `window.__OPENDESK_ADMIN__={"apiUrl":"https://desk.example.com"`) {
		t.Fatalf("config.js: %d %q", w.Code, w.Body.String())
	}
	if w := get("/admin/assets/Page-gone.js"); w.Code != 404 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing asset: %d %v", w.Code, w.Header())
	}
	page := get("/admin/workspaces/123")
	if page.Code != 200 || !strings.HasPrefix(page.Header().Get("Content-Type"), "text/html") || page.Header().Get("X-Robots-Tag") == "" ||
		page.Header().Get("Content-Security-Policy") != "default-src 'self'" {
		t.Fatalf("client route: %d %v", page.Code, page.Header())
	}
}
