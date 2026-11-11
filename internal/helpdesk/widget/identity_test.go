package widget_test

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/helpdesk/widget"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// signUserToken signs claims as a host page's server would, with the widget secret.
func signUserToken(t *testing.T, secret string, method jwt.SigningMethod, claims jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(method, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func errCode(err error) string {
	var he *httpx.Error
	if errors.As(err, &he) {
		return he.Code
	}
	return ""
}

func TestParseUserToken(t *testing.T) {
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	at := func(d time.Duration) int64 { return now.Add(d).Unix() }
	sign := func(claims jwt.MapClaims) string { return signUserToken(t, "s3cret", jwt.SigningMethodHS256, claims) }

	for _, c := range []struct {
		name    string
		token   string
		id      string
		expired bool
		code    string
	}{
		{name: "valid", token: sign(jwt.MapClaims{"user_id": "user-42", "exp": at(time.Hour)}), id: "user-42"},
		{name: "integer id", token: sign(jwt.MapClaims{"user_id": 42, "exp": at(time.Hour), "iat": at(0)}), id: "42"},
		{name: "a full day", token: sign(jwt.MapClaims{"user_id": "u", "exp": at(day)}), id: "u"},
		{name: "a server clock a minute ahead", token: sign(jwt.MapClaims{"user_id": "u", "exp": at(day + time.Minute), "iat": at(time.Minute)}), id: "u"},
		{name: "expired", token: sign(jwt.MapClaims{"user_id": "u", "exp": at(-10 * time.Minute)}), id: "u", expired: true},
		{name: "within the clock allowance", token: sign(jwt.MapClaims{"user_id": "u", "exp": at(-time.Minute)}), id: "u"},
		{name: "too long", token: sign(jwt.MapClaims{"user_id": "u", "exp": at(day + 10*time.Minute)}), code: "invalid_user_token"},
		{name: "no exp", token: sign(jwt.MapClaims{"user_id": "u"}), code: "invalid_user_token"},
		{name: "no user_id", token: sign(jwt.MapClaims{"exp": at(time.Hour)}), code: "invalid_user_token"},
		{name: "empty user_id", token: sign(jwt.MapClaims{"user_id": " ", "exp": at(time.Hour)}), code: "invalid_user_token"},
		{name: "fractional user_id", token: sign(jwt.MapClaims{"user_id": 4.2, "exp": at(time.Hour)}), code: "invalid_user_token"},
		{name: "not yet valid", token: sign(jwt.MapClaims{"user_id": "u", "exp": at(time.Hour), "nbf": at(10 * time.Minute)}), code: "invalid_user_token"},
		{name: "issued in the future", token: sign(jwt.MapClaims{"user_id": "u", "exp": at(time.Hour), "iat": at(10 * time.Minute)}), code: "invalid_user_token"},
		{name: "another secret", token: signUserToken(t, "other", jwt.SigningMethodHS256, jwt.MapClaims{"user_id": "u", "exp": at(time.Hour)}), code: "invalid_user_token"},
		{name: "expired with another secret", token: signUserToken(t, "other", jwt.SigningMethodHS256, jwt.MapClaims{"user_id": "u", "exp": at(-time.Hour)}), code: "invalid_user_token"},
		{name: "HS512", token: signUserToken(t, "s3cret", jwt.SigningMethodHS512, jwt.MapClaims{"user_id": "u", "exp": at(time.Hour)}), code: "invalid_user_token"},
		{name: "alg none", token: func() string {
			s, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"user_id": "u", "exp": at(time.Hour)}).SignedString(jwt.UnsafeAllowNoneSignatureType)
			if err != nil {
				t.Fatal(err)
			}
			return s
		}(), code: "invalid_user_token"},
		{name: "a user_hash", token: crypto.HMACHex([]byte("s3cret"), "u"), code: "invalid_user_token"},
	} {
		t.Run(c.name, func(t *testing.T) {
			id, expired, err := widget.ParseUserToken([]byte("s3cret"), c.token, now, day)
			if c.code != "" {
				if errCode(err) != c.code {
					t.Fatalf("err %v, want %s", err, c.code)
				}
				return
			}
			if err != nil || id != c.id || expired != c.expired {
				t.Fatalf("got %q expired=%v err=%v", id, expired, err)
			}
		})
	}
}

func TestSessionUserToken(t *testing.T) {
	e, ws := setup(t)
	exp := func(d time.Duration) int64 { return time.Now().Add(d).Unix() }
	tok := func(claims jwt.MapClaims) string { return signUserToken(t, "s3cret", jwt.SigningMethodHS256, claims) }
	refused := func(body map[string]any, code string) {
		t.Helper()
		body["key"] = ws.WidgetKey
		if m := e.Do("POST", "/v1/widget/session", hdtest.Auth{}, body).Want(t, 401).Map(t); m["code"] != code {
			t.Fatalf("code %v, want %s", m["code"], code)
		}
	}

	good := tok(jwt.MapClaims{"user_id": "user-42", "exp": exp(time.Hour)})
	s := start(t, e, map[string]any{"key": ws.WidgetKey, "user_id": "user-42", "user_token": good, "email": "u42@example.com", "name": "Uma"})
	var verified bool
	var ext string
	e.Scalar(`SELECT verified FROM widget_sessions WHERE contact_id = $1`, &verified, s.Contact.ID)
	e.Scalar(`SELECT external_id FROM contacts WHERE id = $1`, &ext, s.Contact.ID)
	if !verified || ext != "user-42" || s.Contact.Name != "Uma" {
		t.Fatal(verified, ext, s.Contact)
	}
	// The token and a user_hash verify the same user: one contact.
	if h := start(t, e, map[string]any{"key": ws.WidgetKey, "user_id": "user-42", "user_hash": crypto.HMACHex([]byte("s3cret"), "user-42")}); h.Contact.ID != s.Contact.ID {
		t.Fatal("user_hash and user_token gave two contacts")
	}
	// user_id comes with the token, as with a user_hash, and must be the one it carries; a
	// user_hash beside the token is not read.
	if r := start(t, e, map[string]any{"key": ws.WidgetKey, "user_token": good, "user_id": "user-42", "user_hash": "00ff"}); r.Contact.ID != s.Contact.ID {
		t.Fatal(r)
	}
	e.Do("POST", "/v1/widget/session", hdtest.Auth{}, map[string]any{"key": ws.WidgetKey, "user_token": good}).Want(t, 422)
	refused(map[string]any{"user_token": good, "user_id": "user-7"}, "invalid_user_token")
	refused(map[string]any{"user_id": "user-42", "user_token": signUserToken(t, "other", jwt.SigningMethodHS256, jwt.MapClaims{"user_id": "user-42", "exp": exp(time.Hour)})}, "invalid_user_token")
	// No longer than the widget allows: 24 hours unless it says otherwise.
	long := tok(jwt.MapClaims{"user_id": "user-42", "exp": exp(48 * time.Hour)})
	refused(map[string]any{"user_id": "user-42", "user_token": long}, "invalid_user_token")
	e.SetWidgetSettings(ws, uuid.Nil, map[string]any{"user_token_max_hours": 72})
	start(t, e, map[string]any{"key": ws.WidgetKey, "user_id": "user-42", "user_token": long})
	// No secret configured: nothing verifies.
	ws2 := e.NewWorkspace()
	e.Do("POST", "/v1/widget/session", hdtest.Auth{}, map[string]any{"key": ws2.WidgetKey, "user_id": "user-42", "user_token": good}).Want(t, 401)
}

func TestSessionExpiredUserToken(t *testing.T) {
	e, ws := setup(t)
	tok := func(id string, d time.Duration) string {
		return signUserToken(t, "s3cret", jwt.SigningMethodHS256, jwt.MapClaims{"user_id": id, "exp": time.Now().Add(d).Unix()})
	}
	expired := func(body map[string]any) {
		t.Helper()
		body["key"] = ws.WidgetKey
		if m := e.Do("POST", "/v1/widget/session", hdtest.Auth{}, body).Want(t, 401).Map(t); m["code"] != "user_token_expired" {
			t.Fatalf("code %v", m["code"])
		}
	}

	s := start(t, e, map[string]any{"key": ws.WidgetKey, "user_id": "user-42", "user_token": tok("user-42", time.Hour)})
	old := tok("user-42", -time.Hour)
	// A copied token that expired starts nothing: not a session of its own, and not one on
	// another browser's anonymous session.
	expired(map[string]any{"user_id": "user-42", "user_token": old})
	anon := start(t, e, map[string]any{"key": ws.WidgetKey})
	expired(map[string]any{"user_id": "user-42", "user_token": old, "visitor_token": anon.VisitorToken})
	// Nor another user's session.
	other := start(t, e, map[string]any{"key": ws.WidgetKey, "user_id": "user-7", "user_token": tok("user-7", time.Hour)})
	expired(map[string]any{"user_id": "user-42", "user_token": old, "visitor_token": other.VisitorToken})
	// The browser whose session it verified carries on with it, until that session ends.
	r := start(t, e, map[string]any{"key": ws.WidgetKey, "user_id": "user-42", "user_token": old, "visitor_token": s.VisitorToken})
	if r.VisitorToken != s.VisitorToken || r.Contact.ID != s.Contact.ID {
		t.Fatal(r)
	}
	e.Exec(`UPDATE widget_sessions SET created_at = now() - interval '31 days' WHERE token_hash = $1`, crypto.HashToken(s.VisitorToken))
	expired(map[string]any{"user_id": "user-42", "user_token": old, "visitor_token": s.VisitorToken})
	// A fresh token starts the next one, for the same contact.
	if n := start(t, e, map[string]any{"key": ws.WidgetKey, "user_id": "user-42", "user_token": tok("user-42", time.Hour), "visitor_token": s.VisitorToken}); n.Contact.ID != s.Contact.ID || n.VisitorToken == s.VisitorToken {
		t.Fatal(n)
	}
}

func TestRequireUserToken(t *testing.T) {
	e, ws := setup(t)
	hash := crypto.HMACHex([]byte("s3cret"), "user-42")
	token := signUserToken(t, "s3cret", jwt.SigningMethodHS256, jwt.MapClaims{"user_id": "user-42", "exp": time.Now().Add(time.Hour).Unix()})
	s := start(t, e, map[string]any{"key": ws.WidgetKey, "user_id": "user-42", "user_hash": hash})

	for _, bad := range []map[string]any{{"user_token_max_hours": 0}, {"user_token_max_hours": 721}} {
		e.Do("PATCH", "/v1/widgets/"+ws.WidgetID.String(), ws.Admin.As(ws), map[string]any{"settings": bad}).Want(t, 422)
	}
	m := e.Do("PATCH", "/v1/widgets/"+ws.WidgetID.String(), ws.Admin.As(ws),
		map[string]any{"settings": map[string]any{"require_user_token": true, "user_token_max_hours": 12}}).Want(t, 200).Map(t)
	if set, _ := m["settings"].(map[string]any); set["require_user_token"] != true || set["user_token_max_hours"] != float64(12) {
		t.Fatal(m)
	}

	// user_hash is refused, on a new session and on the one it verified before.
	for _, body := range []map[string]any{
		{"key": ws.WidgetKey, "user_id": "user-42", "user_hash": hash},
		{"key": ws.WidgetKey, "user_id": "user-42", "user_hash": hash, "visitor_token": s.VisitorToken},
		{"key": ws.WidgetKey, "user_id": "user-42"},
	} {
		if m := e.Do("POST", "/v1/widget/session", hdtest.Auth{}, body).Want(t, 401).Map(t); m["code"] != "user_token_required" {
			t.Fatal(m)
		}
	}
	// A user_token verifies, and anonymous visitors are not affected.
	if r := start(t, e, map[string]any{"key": ws.WidgetKey, "user_id": "user-42", "user_token": token, "visitor_token": s.VisitorToken}); r.VisitorToken != s.VisitorToken {
		t.Fatal("token resume", r)
	}
	start(t, e, map[string]any{"key": ws.WidgetKey})
}
