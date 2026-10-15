package core

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/crypto"
)

// register signs a new account in to c's browser and returns its id and email.
func (c *client) registerAccount(tag string) (uuid.UUID, string) {
	c.e.t.Helper()
	email := uniqueEmail(c.e.t, tag)
	r := c.do("POST", "/v1/auth/register", map[string]any{"email": email, "password": goodPassword, "name": "Test " + strings.ToUpper(tag[:1]) + tag[1:]})
	if r.Status != 200 {
		c.e.t.Fatalf("register %s: %d %s", tag, r.Status, r.Raw)
	}
	c.token = r.str("access_token")
	return uuid.MustParse(r.str("user", "id")), email
}

func setCookies(r resp) []string { return r.Header.Values("Set-Cookie") }

func hasSetCookie(r resp, name string, cleared bool) bool {
	for _, v := range setCookies(r) {
		if !strings.HasPrefix(v, name+"=") {
			continue
		}
		if isClear := strings.Contains(v, "Max-Age=0") || strings.HasPrefix(v, name+"=;"); isClear == cleared {
			return true
		}
	}
	return false
}

func accountIDs(t *testing.T, r resp) []string {
	t.Helper()
	if r.Status != 200 {
		t.Fatalf("accounts: %d %s", r.Status, r.Raw)
	}
	var out []string
	list, _ := r.Body["accounts"].([]any)
	for _, x := range list {
		u := x.(map[string]any)["user"].(map[string]any)
		out = append(out, u["id"].(string))
	}
	return out
}

// TestMultiAccount: one browser holds two accounts; each refreshes and signs out alone.
func TestMultiAccount(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	a, emailA := c.registerAccount("multia")
	tokA := c.token
	c.newWorkspace("Alpha Co")
	b, _ := c.registerAccount("multib")
	tokB := c.token
	nameA, nameB := RefreshCookieName(a), RefreshCookieName(b)
	if c.cookie(nameA) == "" || c.cookie(nameB) == "" {
		t.Fatalf("expected both account cookies, have %v", c.cookies())
	}

	// The list: both accounts, in the order they were added, with memberships; nothing rotates.
	beforeA, beforeB := c.cookie(nameA), c.cookie(nameB)
	r := c.do("POST", "/v1/auth/accounts", nil)
	if ids := accountIDs(t, r); len(ids) != 2 || ids[0] != a.String() || ids[1] != b.String() {
		t.Fatalf("accounts = %v", ids)
	}
	first := r.Body["accounts"].([]any)[0].(map[string]any)
	if ms := first["memberships"].([]any); len(ms) != 1 || ms[0].(map[string]any)["workspace"].(map[string]any)["name"] != "Alpha Co" {
		t.Fatalf("memberships = %v", first["memberships"])
	}
	if c.cookie(nameA) != beforeA || c.cookie(nameB) != beforeB || len(setCookies(r)) != 0 {
		t.Fatalf("listing changed cookies: %v", setCookies(r))
	}

	// Refreshing A rotates only A's cookie.
	r = c.do("POST", "/v1/auth/refresh", map[string]any{"user_id": a.String()})
	if r.Status != 200 || r.str("user", "id") != a.String() {
		t.Fatalf("refresh a: %d %s", r.Status, r.Raw)
	}
	if !hasSetCookie(r, nameA, false) || len(setCookies(r)) != 1 {
		t.Fatalf("refresh a set cookies: %v", setCookies(r))
	}
	if c.cookie(nameA) == beforeA || c.cookie(nameB) != beforeB {
		t.Fatal("refresh a touched the wrong cookie")
	}
	tokA = r.str("access_token")
	if r := c.do("POST", "/v1/auth/refresh", map[string]any{"user_id": b.String()}); r.Status != 200 || r.str("user", "id") != b.String() {
		t.Fatalf("refresh b: %d %s", r.Status, r.Raw)
	}
	// Without user_id: the account used most recently.
	if r := c.do("POST", "/v1/auth/refresh", nil); r.Status != 200 || r.str("user", "id") != b.String() {
		t.Fatalf("refresh default: %d %s", r.Status, r.Raw)
	}
	// An account the browser isn't signed in to.
	if r := c.do("POST", "/v1/auth/refresh", map[string]any{"user_id": uuid.NewString()}); r.Status != 401 {
		t.Fatalf("refresh of an unknown account: %d", r.Status)
	}

	// A cookie named for B that carries A's token is refused and rotates nothing.
	odd := e.client()
	odd.jar.SetCookies(mustURL(e.srv.URL+"/v1/auth"), []*http.Cookie{{Name: nameB, Value: c.cookie(nameA), Path: "/v1/auth"}})
	if r := odd.do("POST", "/v1/auth/refresh", map[string]any{"user_id": b.String()}); r.Status != 401 {
		t.Fatalf("mismatched cookie accepted: %d", r.Status)
	}
	if r := c.do("POST", "/v1/auth/refresh", map[string]any{"user_id": a.String()}); r.Status != 200 {
		t.Fatalf("a's session hurt by a mismatched cookie: %d %s", r.Status, r.Raw)
	}

	// Log out of A only.
	c.token = tokA
	r = c.do("POST", "/v1/auth/logout", map[string]any{"user_id": a.String()})
	if r.Status != 204 || !hasSetCookie(r, nameA, true) || hasSetCookie(r, nameB, true) {
		t.Fatalf("logout a: %d %v", r.Status, setCookies(r))
	}
	if c.cookie(nameA) != "" || c.cookie(nameB) == "" {
		t.Fatalf("after logout a: %v", c.cookies())
	}
	c.token = ""
	if r := c.do("POST", "/v1/auth/refresh", map[string]any{"user_id": a.String()}); r.Status != 401 {
		t.Fatalf("a refreshed after logout: %d", r.Status)
	}
	if r := c.do("POST", "/v1/auth/refresh", map[string]any{"user_id": b.String()}); r.Status != 200 {
		t.Fatalf("b signed out with a: %d %s", r.Status, r.Raw)
	}
	if ids := accountIDs(t, c.do("POST", "/v1/auth/accounts", nil)); len(ids) != 1 || ids[0] != b.String() {
		t.Fatalf("accounts after logout a = %v", ids)
	}
	var logouts int
	_ = e.d.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action = 'logout' AND entity_id = $1`, a).Scan(&logouts)
	if logouts != 1 {
		t.Fatalf("logout audit rows for a = %d", logouts)
	}

	// Sign A in again, then log out of every account.
	r = c.do("POST", "/v1/auth/login", map[string]any{"email": emailA, "password": goodPassword})
	if r.Status != 200 {
		t.Fatalf("login a: %d %s", r.Status, r.Raw)
	}
	heldA, heldB := c.cookie(nameA), c.cookie(nameB)
	c.token = tokB
	r = c.do("POST", "/v1/auth/logout", map[string]any{"all": true})
	if r.Status != 204 || !hasSetCookie(r, nameA, true) || !hasSetCookie(r, nameB, true) {
		t.Fatalf("logout all: %d %v", r.Status, setCookies(r))
	}
	if len(c.cookies()) != 0 {
		t.Fatalf("cookies left after logout all: %v", c.cookies())
	}
	for name, v := range map[string]string{nameA: heldA, nameB: heldB} {
		x := e.client()
		x.jar.SetCookies(mustURL(e.srv.URL+"/v1/auth"), []*http.Cookie{{Name: name, Value: v, Path: "/v1/auth"}})
		if r := x.do("POST", "/v1/auth/refresh", nil); r.Status != 401 {
			t.Fatalf("session %s survived logout all: %d", name, r.Status)
		}
	}
}

// TestLegacyRefreshCookie: the single od_refresh cookie of earlier versions keeps working
// and moves to the account's own cookie on first use.
func TestLegacyRefreshCookie(t *testing.T) {
	e := newEnv(t)
	src := e.client()
	a, _ := src.registerAccount("legacy")
	name, value := src.accountCookie()

	legacy := func(v string) *client {
		c := e.client()
		c.jar.SetCookies(mustURL(e.srv.URL+"/v1/auth"), []*http.Cookie{{Name: LegacyRefreshCookie, Value: v, Path: "/v1/auth"}})
		return c
	}

	// Listing moves it without rotating it.
	c := legacy(value)
	r := c.do("POST", "/v1/auth/accounts", nil)
	if ids := accountIDs(t, r); len(ids) != 1 || ids[0] != a.String() {
		t.Fatalf("accounts with legacy cookie = %v", ids)
	}
	if c.cookie(LegacyRefreshCookie) != "" || c.cookie(name) != value {
		t.Fatalf("legacy cookie not moved: %v", c.cookies())
	}

	// Refresh moves it and rotates.
	c = legacy(value)
	r = c.do("POST", "/v1/auth/refresh", nil)
	if r.Status != 200 || r.str("user", "id") != a.String() {
		t.Fatalf("legacy refresh: %d %s", r.Status, r.Raw)
	}
	if !hasSetCookie(r, LegacyRefreshCookie, true) || c.cookie(LegacyRefreshCookie) != "" || c.cookie(name) == "" || c.cookie(name) == value {
		t.Fatalf("legacy refresh cookies: %v", setCookies(r))
	}
	value = c.cookie(name)

	// Naming the account works with the legacy cookie too.
	c = legacy(value)
	if r := c.do("POST", "/v1/auth/refresh", map[string]any{"user_id": a.String()}); r.Status != 200 || c.cookie(name) == "" {
		t.Fatalf("legacy refresh by user_id: %d %s", r.Status, r.Raw)
	}
	value = c.cookie(name)

	// Signing a second account in beside a legacy cookie keeps the first account.
	c = legacy(value)
	b, _ := c.registerAccount("legacyb")
	if c.cookie(LegacyRefreshCookie) != "" || c.cookie(name) != value || c.cookie(RefreshCookieName(b)) == "" {
		t.Fatalf("legacy beside a new account: %v", c.cookies())
	}
	if ids := accountIDs(t, c.do("POST", "/v1/auth/accounts", nil)); len(ids) != 2 {
		t.Fatalf("accounts = %v", ids)
	}

	// An invalid legacy cookie is cleared, and only it.
	c = legacy("not-a-token")
	if r := c.do("POST", "/v1/auth/refresh", nil); r.Status != 401 || !hasSetCookie(r, LegacyRefreshCookie, true) {
		t.Fatalf("bad legacy refresh: %d %v", r.Status, setCookies(r))
	}
}

// TestAccountsDropsEndedSessions: a revoked account leaves the list and loses its cookie;
// the others stay.
func TestAccountsDropsEndedSessions(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	a, _ := c.registerAccount("revokea")
	b, _ := c.registerAccount("revokeb")
	nameA := RefreshCookieName(a)
	if _, err := e.d.DB.Pool.Exec(context.Background(), `UPDATE sessions SET revoked_at = now() WHERE refresh_hash = $1`,
		crypto.HashToken(c.cookie(nameA))); err != nil {
		t.Fatal(err)
	}
	r := c.do("POST", "/v1/auth/accounts", nil)
	if ids := accountIDs(t, r); len(ids) != 1 || ids[0] != b.String() {
		t.Fatalf("accounts = %v", ids)
	}
	if !hasSetCookie(r, nameA, true) || c.cookie(nameA) != "" || c.cookie(RefreshCookieName(b)) == "" {
		t.Fatalf("revoked cookie not cleared: %v", setCookies(r))
	}
	// A refused refresh clears only its own account's cookie.
	c2 := e.client()
	x, _ := c2.registerAccount("refusea")
	y, _ := c2.registerAccount("refuseb")
	_, _ = e.d.DB.Pool.Exec(context.Background(), `UPDATE sessions SET revoked_at = now() WHERE user_id = $1`, x)
	r = c2.do("POST", "/v1/auth/refresh", map[string]any{"user_id": x.String()})
	if r.Status != 401 || !hasSetCookie(r, RefreshCookieName(x), true) || len(setCookies(r)) != 1 || c2.cookie(RefreshCookieName(y)) == "" {
		t.Fatalf("refused refresh: %d %v", r.Status, setCookies(r))
	}
}

// TestAccountCap: a browser holds at most MaxAccounts; signing in again to one it holds
// replaces that session.
func TestAccountCap(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	var first uuid.UUID
	var firstEmail string
	for i := 0; i < MaxAccounts; i++ {
		id, email := c.registerAccount("cap")
		if i == 0 {
			first, firstEmail = id, email
		}
	}
	email := uniqueEmail(t, "capx")
	r := c.do("POST", "/v1/auth/register", map[string]any{"email": email, "password": goodPassword, "name": "Too Many"})
	if r.Status != 409 || r.str("code") != "too_many_accounts" {
		t.Fatalf("sixth account: %d %s", r.Status, r.Raw)
	}
	var n int
	_ = e.d.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM users WHERE email = $1`, email).Scan(&n)
	if n != 0 {
		t.Fatal("refused registration still created the user")
	}
	// Signing in again to a held account replaces its session, under the cap.
	old := c.cookie(RefreshCookieName(first))
	r = c.do("POST", "/v1/auth/login", map[string]any{"email": firstEmail, "password": goodPassword})
	if r.Status != 200 {
		t.Fatalf("re-login: %d %s", r.Status, r.Raw)
	}
	if now := c.cookie(RefreshCookieName(first)); now == "" || now == old {
		t.Fatal("re-login did not replace the cookie")
	}
	var revoked bool
	_ = e.d.DB.Pool.QueryRow(context.Background(), `SELECT revoked_at IS NOT NULL FROM sessions WHERE refresh_hash = $1`, crypto.HashToken(old)).Scan(&revoked)
	if !revoked {
		t.Fatal("replaced session still live")
	}
	if ids := accountIDs(t, c.do("POST", "/v1/auth/accounts", nil)); len(ids) != MaxAccounts {
		t.Fatalf("accounts = %d", len(ids))
	}
	// Native requests are not affected: no cookies, no cap.
	if r := c.do("POST", "/v1/auth/login", map[string]any{"email": firstEmail, "password": goodPassword}, iosHeader...); r.Status != 200 ||
		r.str("refresh_token") == "" || len(setCookies(r)) != 0 {
		t.Fatalf("native login: %d %v", r.Status, setCookies(r))
	}
}
