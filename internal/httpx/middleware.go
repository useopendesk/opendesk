package httpx

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/useopendesk/opendesk/internal/crypto"
)

type ridKey struct{}

// RequestID assigns every request an id, echoed in X-Request-Id and error bodies.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := crypto.RandomString(16)
		w.Header().Set("X-Request-Id", id)
		r.Header.Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ridKey{}, id)))
	})
}

func RequestIDFrom(ctx context.Context) string { s, _ := ctx.Value(ridKey{}).(string); return s }

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int)        { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Logger logs method, path, status and duration. Never query strings or bodies:
// they can carry tokens.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: 200}
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic", "err", rec, "stack", string(debug.Stack()), "request_id", RequestIDFrom(r.Context()))
					if sw.status == 200 {
						http.Error(sw, `{"status":500,"title":"Internal Server Error","code":"internal","detail":"`+internalMessage+`"}`, 500)
					}
				}
				if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
					return
				}
				log.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status,
					"ms", time.Since(start).Milliseconds(), "request_id", RequestIDFrom(r.Context()))
			}()
			next.ServeHTTP(sw, r)
		})
	}
}

// SecurityHeaders sets HSTS, nosniff, frame denial, referrer policy and a default-deny CSP
// on every API response. Scope an exception to a route rather than removing a header.
func SecurityHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			if h.Get("Content-Security-Policy") == "" {
				h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS allows the configured app origins with credentials. Widget and public routes,
// and the MCP server's OAuth metadata, registration, token and revocation endpoints
// (browser-based MCP clients call them), allow any origin without credentials.
func CORS(origins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			h := w.Header()
			public := strings.HasPrefix(r.URL.Path, "/v1/widget/") || strings.HasPrefix(r.URL.Path, "/v1/public/") || oauthPublic(r.URL.Path)
			if origin != "" {
				h.Add("Vary", "Origin")
				switch {
				case slices.Contains(origins, origin):
					h.Set("Access-Control-Allow-Origin", origin)
					h.Set("Access-Control-Allow-Credentials", "true")
				case public:
					h.Set("Access-Control-Allow-Origin", "*")
				}
				h.Set("Access-Control-Expose-Headers", "X-Request-Id, RateLimit-Limit, RateLimit-Remaining, RateLimit-Reset, Idempotent-Replayed, Mcp-Session-Id, WWW-Authenticate, OD-Ticket-Hidden")
			}
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, OD-Workspace, OD-Client, Idempotency-Key, Mcp-Protocol-Version, Mcp-Session-Id, Last-Event-ID")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// oauthPublic reports the MCP OAuth endpoints a browser client may call from any origin.
func oauthPublic(path string) bool {
	switch path {
	case "/oauth/token", "/oauth/revoke":
		return true
	}
	return strings.HasPrefix(path, "/.well-known/oauth-")
}

// ClientIP returns the caller's address. Proxy headers are trusted only when trustProxy is set.
func ClientIP(trustProxy bool) func(*http.Request) string {
	return func(r *http.Request) string {
		if trustProxy {
			if ip := r.Header.Get("CF-Connecting-IP"); net.ParseIP(ip) != nil {
				return ip
			}
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				first, _, _ := strings.Cut(xff, ",")
				if ip := strings.TrimSpace(first); net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return r.RemoteAddr
		}
		return host
	}
}

// Limiter is an in-memory fixed-window rate limiter keyed by string. Per process;
// good enough for auth throttling on each API instance.
type Limiter struct {
	mu      sync.Mutex
	windows map[string]*window
}

type window struct {
	start time.Time
	count int
}

func NewLimiter() *Limiter {
	l := &Limiter{windows: map[string]*window{}}
	go func() {
		for range time.Tick(time.Minute) {
			l.mu.Lock()
			for k, w := range l.windows {
				if time.Since(w.start) > time.Hour {
					delete(l.windows, k)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

// Allow records a hit and reports whether key is within limit per period.
func (l *Limiter) Allow(key string, limit int, period time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.windows[key]
	if !ok || time.Since(w.start) > period {
		w = &window{start: time.Now()}
		l.windows[key] = w
	}
	w.count++
	return w.count <= limit
}

// RateLimited is the 429 every throttled route returns.
func RateLimited() *Error {
	return E(429, "rate_limited", "Too many requests. Wait a moment and try again.")
}
