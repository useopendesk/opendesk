package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCORSAdminRoutes: the admin routes allow only the panel's origin, and the panel's
// origin is not allowed anywhere else.
func TestCORSAdminRoutes(t *testing.T) {
	h := CORS([]string{"https://app.example.com"}, "https://admin.example.com")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	allowed := func(path, origin string) string {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Header().Get("Access-Control-Allow-Origin")
	}
	cases := []struct{ path, origin, want string }{
		{"/v1/admin/me", "https://admin.example.com", "https://admin.example.com"},
		{"/v1/admin/me", "https://app.example.com", ""},
		{"/v1/tickets", "https://app.example.com", "https://app.example.com"},
		{"/v1/tickets", "https://admin.example.com", ""},
		{"/v1/widget/config", "https://admin.example.com", "*"},
	}
	for _, c := range cases {
		if got := allowed(c.path, c.origin); got != c.want {
			t.Errorf("%s from %s: %q, want %q", c.path, c.origin, got, c.want)
		}
	}
}
