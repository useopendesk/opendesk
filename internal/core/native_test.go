package core

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/useopendesk/opendesk/internal/appattest"
	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/captcha"
	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/ids"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/push"
)

var iosHeader = []string{"OD-Client", "ios/1.0"}

// TestNativeSessions: a native request (OD-Client ios/..., no Origin) gets the refresh
// token in the body and no cookie; the same request with Origin is a browser's.
func TestNativeSessions(t *testing.T) {
	e := newEnv(t)
	email := uniqueEmail(t, "native")
	phone := e.client()
	r := phone.do("POST", "/v1/auth/register", map[string]any{"email": email, "password": goodPassword, "name": "Nat Ive"}, iosHeader...)
	if r.Status != 200 || r.str("refresh_token") == "" || r.Header.Get("Set-Cookie") != "" {
		t.Fatalf("native register: %d cookie=%q %s", r.Status, r.Header.Get("Set-Cookie"), r.Raw)
	}
	refresh := r.str("refresh_token")
	phone.token = r.str("access_token")

	browser := e.client()
	b := browser.do("POST", "/v1/auth/login", map[string]any{"email": email, "password": goodPassword},
		"OD-Client", "ios/1.0", "Origin", "https://evil.example")
	if _, ck := browser.accountCookie(); b.Status != 200 || b.str("refresh_token") != "" || ck == "" {
		t.Fatalf("login with Origin: %d body token=%q cookie=%q", b.Status, b.str("refresh_token"), ck)
	}
	browser.token = b.str("access_token")

	desk := e.client()
	d := desk.do("POST", "/v1/auth/login", map[string]any{"email": email, "password": goodPassword},
		"User-Agent", "Mozilla/5.0 Chrome/140 opendesk-desktop/1.2.0", "Origin", "https://app.test")
	if d.Status != 200 || d.str("refresh_token") != "" {
		t.Fatalf("desktop login: %d %s", d.Status, d.Raw)
	}

	sessions := phone.do("GET", "/v1/me/sessions", nil)
	clients := map[string]int{}
	for _, x := range sessions.Body["data"].([]any) {
		m := x.(map[string]any)
		clients[m["client"].(string)]++
		if m["current"] == true && m["client"] != "ios" {
			t.Fatalf("current session client = %v", m["client"])
		}
	}
	if clients["ios"] != 1 || clients["web"] != 1 || clients["desktop"] != 1 {
		t.Fatalf("session clients = %v", clients)
	}

	// Refresh with the body token: a new token in the body, still no cookie.
	r = e.client().do("POST", "/v1/auth/refresh", map[string]any{"refresh_token": refresh}, iosHeader...)
	if r.Status != 200 || r.str("refresh_token") == "" || r.str("refresh_token") == refresh || r.Header.Get("Set-Cookie") != "" {
		t.Fatalf("native refresh: %d cookie=%q %s", r.Status, r.Header.Get("Set-Cookie"), r.Raw)
	}
	next := r.str("refresh_token")
	// The rotated token presented again after the grace window ends the session.
	_, _ = e.d.DB.Pool.Exec(context.Background(), `UPDATE sessions SET rotated_at = now() - interval '1 minute' WHERE prev_refresh_hash = $1`,
		crypto.HashToken(refresh))
	if r := e.client().do("POST", "/v1/auth/refresh", map[string]any{"refresh_token": refresh}, iosHeader...); r.Status != 401 ||
		r.Header.Get("Set-Cookie") != "" {
		t.Fatalf("reused native refresh: %d %q", r.Status, r.Header.Get("Set-Cookie"))
	}
	if r := e.client().do("POST", "/v1/auth/refresh", map[string]any{"refresh_token": next}, iosHeader...); r.Status != 401 {
		t.Fatalf("session survived reuse: %d", r.Status)
	}

	// Logout with the body token.
	r = e.client().do("POST", "/v1/auth/login", map[string]any{"email": email, "password": goodPassword}, iosHeader...)
	tok := r.str("refresh_token")
	if r := e.client().do("POST", "/v1/auth/logout", map[string]any{"refresh_token": tok}, iosHeader...); r.Status != 204 ||
		r.Header.Get("Set-Cookie") != "" {
		t.Fatalf("native logout: %d %q", r.Status, r.Header.Get("Set-Cookie"))
	}
	if r := e.client().do("POST", "/v1/auth/refresh", map[string]any{"refresh_token": tok}, iosHeader...); r.Status != 401 {
		t.Fatalf("refresh after logout: %d", r.Status)
	}
	// Browsers keep the cookie flow, with or without a body.
	if r := browser.do("POST", "/v1/auth/refresh", nil); r.Status != 200 || r.str("refresh_token") != "" {
		t.Fatalf("browser refresh: %d %s", r.Status, r.Raw)
	}
}

// TestNativeOAuthPKCE: native codes need the PKCE verifier, which the callback flow binds.
func TestNativeOAuthPKCE(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, email := e.signup("pkce")
	var uid uuid.UUID
	_ = e.d.DB.Pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&uid)
	if _, err := e.d.DB.Pool.Exec(ctx, `INSERT INTO identities (id, user_id, provider, issuer, subject, email) VALUES ($1, $2, 'github', $3, $4, $5)`,
		ids.New(), uid, githubIssuer, "gh-"+uid.String(), email); err != nil {
		t.Fatal(err)
	}
	id := &fedIdentity{Provider: "github", Issuer: githubIssuer, Subject: "gh-" + uid.String(), Email: email, EmailVerified: true}
	verifier := crypto.RandomToken(48)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	code, errCode := e.s.resolveIdentity(ctx, id, "/inbox", challenge, reqInfo{})
	if errCode != "" {
		t.Fatal(errCode)
	}
	if r := e.client().do("POST", "/v1/auth/oauth/exchange", map[string]any{"code": code}, iosHeader...); r.Status != 400 || r.str("code") != "invalid_code" {
		t.Fatalf("exchange without verifier: %d %s", r.Status, r.Raw)
	}
	if r := e.client().do("POST", "/v1/auth/oauth/exchange", map[string]any{"code": code, "code_verifier": verifier}, iosHeader...); r.Status != 400 {
		t.Fatalf("spent code accepted: %d %s", r.Status, r.Raw)
	}
	code, _ = e.s.resolveIdentity(ctx, id, "/inbox", challenge, reqInfo{})
	if r := e.client().do("POST", "/v1/auth/oauth/exchange", map[string]any{"code": code, "code_verifier": crypto.RandomToken(48)}, iosHeader...); r.Status != 400 ||
		r.str("code") != "invalid_code" {
		t.Fatalf("wrong verifier: %d %s", r.Status, r.Raw)
	}
	code, _ = e.s.resolveIdentity(ctx, id, "/inbox", challenge, reqInfo{})
	r := e.client().do("POST", "/v1/auth/oauth/exchange", map[string]any{"code": code, "code_verifier": verifier}, iosHeader...)
	if r.Status != 200 || r.str("access_token") == "" || r.str("refresh_token") == "" || r.str("redirect_to") != "/inbox" {
		t.Fatalf("native exchange: %d %s", r.Status, r.Raw)
	}
	// A web code needs no verifier.
	code, _ = e.s.resolveIdentity(ctx, id, "", "", reqInfo{})
	if r := e.client().do("POST", "/v1/auth/oauth/exchange", map[string]any{"code": code}); r.Status != 200 || r.str("refresh_token") != "" {
		t.Fatalf("web exchange: %d %s", r.Status, r.Raw)
	}

	// start: native flows carry the challenge in the state; a bad challenge goes back to the app.
	e.s.oauth = newOAuthProviders(e.d.Cfg)
	gh := &oauthProvider{id: "github", name: "GitHub", kind: "github"}
	gh.cfg.ClientID, gh.cfg.Endpoint.AuthURL = "cid", "https://github.com/login/oauth/authorize"
	e.s.oauth.byID["github"] = gh
	start := func(q string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/v1/auth/oauth/github/start?"+q, nil)
		rc := chiCtx(req, "provider", "github")
		w := httptest.NewRecorder()
		e.s.oauthStart(w, rc)
		return w
	}
	w := start("client=native&code_challenge=short")
	if loc := w.Header().Get("Location"); loc != "opendesk://auth/callback?error=invalid_request" {
		t.Fatalf("bad challenge: %d %s", w.Code, loc)
	}
	w = start("client=native&code_challenge=" + challenge)
	loc, _ := url.Parse(w.Header().Get("Location"))
	st, err := e.d.Tokens.Verify(loc.Query().Get("state"), auth.PurposeOAuthState)
	if err != nil || st.Data["client"] != "native" || st.Data["cc"] != challenge {
		t.Fatalf("state: %v %v", err, st)
	}
	// A provider error on a native flow returns to the app; a forged state never does.
	cb := func(q string) string {
		req := httptest.NewRequest("GET", "/v1/auth/oauth/github/callback?"+q, nil)
		w := httptest.NewRecorder()
		e.s.oauthCallback(w, chiCtx(req, "provider", "github"))
		return w.Header().Get("Location")
	}
	if got := cb("error=access_denied&state=" + url.QueryEscape(loc.Query().Get("state"))); got != "opendesk://auth/callback?error=access_denied" {
		t.Fatalf("native denied: %s", got)
	}
	if got := cb("error=access_denied&state=forged"); !strings.HasPrefix(got, "http://app.test/auth/callback?error=") {
		t.Fatalf("forged state: %s", got)
	}

	// A popup sign-in returns to /auth/popup with its id; a malformed id falls back to the
	// page flow, and a native flow ignores it.
	popupID := crypto.RandomToken(16)
	w = start("popup=" + popupID)
	loc, _ = url.Parse(w.Header().Get("Location"))
	if st, err := e.d.Tokens.Verify(loc.Query().Get("state"), auth.PurposeOAuthState); err != nil || st.Data["popup"] != popupID {
		t.Fatalf("popup state: %v %v", err, st)
	}
	if got := cb("error=access_denied&state=" + url.QueryEscape(loc.Query().Get("state"))); got != "http://app.test/auth/popup?error=access_denied&popup="+popupID {
		t.Fatalf("popup denied: %s", got)
	}
	for _, q := range []string{"popup=short", "popup=" + url.QueryEscape("bad id with spaces!"), "client=native&code_challenge=" + challenge + "&popup=" + popupID} {
		w = start(q)
		loc, _ = url.Parse(w.Header().Get("Location"))
		if st, err := e.d.Tokens.Verify(loc.Query().Get("state"), auth.PurposeOAuthState); err != nil || st.Data["popup"] != "" {
			t.Fatalf("%s: popup kept: %v %v", q, err, st)
		}
	}
}

// attestDevice is an App Attest key the test controls, stored as if attested.
type attestDevice struct {
	key     *ecdsa.PrivateKey
	keyID   string
	appID   string
	counter uint32
}

func newAttestDevice(t *testing.T, e *testEnv, appID string) *attestDevice {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ek, _ := k.PublicKey.ECDH()
	sum := sha256.Sum256(ek.Bytes())
	d := &attestDevice{key: k, keyID: base64.StdEncoding.EncodeToString(sum[:]), appID: appID}
	if _, err := e.d.DB.Pool.Exec(context.Background(), `INSERT INTO app_attest_keys (key_id, app_id, public_key, environment)
		VALUES ($1, $2, $3, 'production')`, d.keyID, appID, ek.Bytes()); err != nil {
		t.Fatal(err)
	}
	return d
}

func (d *attestDevice) proof(t *testing.T, e *testEnv, action string, counter uint32) map[string]any {
	t.Helper()
	ch := e.client().do("POST", "/v1/auth/app-attest/challenge", map[string]any{}, iosHeader...)
	if ch.Status != 200 {
		t.Fatalf("challenge: %d %s", ch.Status, ch.Raw)
	}
	challenge := ch.str("challenge")
	rp := sha256.Sum256([]byte(d.appID))
	ad := append(append([]byte{}, rp[:]...), 0)
	ad = binary.BigEndian.AppendUint32(ad, counter)
	cdh := sha256.Sum256(appattest.ClientData(action, challenge))
	nonce := sha256.Sum256(append(append([]byte{}, ad...), cdh[:]...))
	digest := sha256.Sum256(nonce[:])
	sig, _ := ecdsa.SignASN1(rand.Reader, d.key, digest[:])
	obj, _ := cbor.Marshal(map[string][]byte{"signature": sig, "authenticatorData": ad})
	return map[string]any{"key_id": d.keyID, "assertion": base64.StdEncoding.EncodeToString(obj), "challenge": challenge}
}

// TestAppAttest: a native proof stands in for Turnstile; challenges are single use and
// counters only move forward.
func TestAppAttest(t *testing.T) {
	e := newEnv(t)
	const app = "TEAM123456.sh.opendesk.app"
	if r := e.client().do("POST", "/v1/auth/app-attest/challenge", map[string]any{}, iosHeader...); r.Status != 404 {
		t.Fatalf("challenge with App Attest off: %d", r.Status)
	}
	e.d.Cfg.AppAttestAppIDs = []string{app}
	// Turnstile on, and every token refused.
	turn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"success":false}`)) }))
	defer turn.Close()
	e.d.Captcha = captcha.NewTurnstileFromConfig(captcha.TurnstileConfig{Secret: "s", SiteVerifyURL: turn.URL})
	if c := e.client().do("GET", "/v1/auth/config", nil); c.Body["native"].(map[string]any)["app_attest"] != true ||
		c.Body["native"].(map[string]any)["push"] != "off" {
		t.Fatalf("auth config: %s", c.Raw)
	}

	dev := newAttestDevice(t, e, app)
	email := uniqueEmail(t, "attest")
	body := func(proof map[string]any) map[string]any {
		return map[string]any{"email": email, "password": goodPassword, "app_attest": proof}
	}
	regBody := func(proof map[string]any) map[string]any {
		b := body(proof)
		b["name"] = "App Attest"
		return b
	}
	if r := e.client().do("POST", "/v1/auth/register", regBody(nil), iosHeader...); r.Status != 400 || r.str("code") != "captcha_failed" {
		t.Fatalf("register without proof: %d %s", r.Status, r.Raw)
	}
	p := dev.proof(t, e, appattest.ActionRegister, 1)
	if r := e.client().do("POST", "/v1/auth/register", regBody(p), iosHeader...); r.Status != 200 {
		t.Fatalf("register with proof: %d %s", r.Status, r.Raw)
	}
	// The same proof again: its challenge is spent.
	if r := e.client().do("POST", "/v1/auth/login", body(p), iosHeader...); r.Status != 400 || r.str("code") != "captcha_failed" {
		t.Fatalf("spent challenge: %d %s", r.Status, r.Raw)
	}
	// A proof for another action, an old counter, or from a browser is refused.
	if r := e.client().do("POST", "/v1/auth/login", body(dev.proof(t, e, appattest.ActionRegister, 2)), iosHeader...); r.Status != 400 {
		t.Fatalf("proof for another action: %d %s", r.Status, r.Raw)
	}
	if r := e.client().do("POST", "/v1/auth/login", body(dev.proof(t, e, appattest.ActionLogin, 1)), iosHeader...); r.Status != 400 {
		t.Fatalf("counter replay: %d %s", r.Status, r.Raw)
	}
	if r := e.client().do("POST", "/v1/auth/login", body(dev.proof(t, e, appattest.ActionLogin, 5)), "OD-Client", "ios/1.0", "Origin", "https://evil.example"); r.Status != 400 {
		t.Fatalf("proof from a browser: %d %s", r.Status, r.Raw)
	}
	r := e.client().do("POST", "/v1/auth/login", body(dev.proof(t, e, appattest.ActionLogin, 6)), iosHeader...)
	if r.Status != 200 || r.str("refresh_token") == "" {
		t.Fatalf("login with proof: %d %s", r.Status, r.Raw)
	}
	var counter int64
	_ = e.d.DB.Pool.QueryRow(context.Background(), `SELECT counter FROM app_attest_keys WHERE key_id = $1`, dev.keyID).Scan(&counter)
	if counter != 6 {
		t.Fatalf("stored counter = %d", counter)
	}
	if r := e.client().do("POST", "/v1/auth/password/forgot", map[string]any{"email": email,
		"app_attest": dev.proof(t, e, appattest.ActionPasswordForgot, 7)}, iosHeader...); r.Status != 204 {
		t.Fatalf("forgot with proof: %d %s", r.Status, r.Raw)
	}
	other := newAttestDevice(t, e, "OTHERTEAM.app")
	if r := e.client().do("POST", "/v1/auth/login", body(other.proof(t, e, appattest.ActionLogin, 1)), iosHeader...); r.Status != 400 {
		t.Fatalf("key of an app no longer accepted: %d %s", r.Status, r.Raw)
	}

	// Key registration refuses anything Apple did not attest, and spends the challenge.
	ch := e.client().do("POST", "/v1/auth/app-attest/challenge", map[string]any{}, iosHeader...).str("challenge")
	att := map[string]any{"key_id": dev.keyID, "attestation": base64.StdEncoding.EncodeToString([]byte("not cbor")), "challenge": ch}
	if r := e.client().do("POST", "/v1/auth/app-attest/keys", att, iosHeader...); r.Status != 400 || r.str("code") != "invalid_attestation" {
		t.Fatalf("bogus attestation: %d %s", r.Status, r.Raw)
	}
	var left int
	_ = e.d.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM app_attest_challenges WHERE challenge_hash = $1`, crypto.HashToken(ch)).Scan(&left)
	if left != 0 {
		t.Fatal("challenge survived a failed attestation")
	}
}

// fakeAPNs is an HTTP/2 APNs stand-in that records pushes.
type fakeAPNs struct {
	mu     sync.Mutex
	tokens []string
	bodies []map[string]any
	gone   map[string]bool
}

func (f *fakeAPNs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	tok := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens = append(f.tokens, tok)
	f.bodies = append(f.bodies, body)
	if f.gone[tok] {
		w.WriteHeader(410)
		_, _ = w.Write([]byte(`{"reason":"Unregistered"}`))
	}
}

func (f *fakeAPNs) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.bodies) }

func (f *fakeAPNs) last() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[len(f.bodies)-1]
}

func withAPNs(t *testing.T, e *testEnv) *fakeAPNs {
	t.Helper()
	fake := &fakeAPNs{gone: map[string]bool{}}
	srv := httptest.NewUnstartedServer(fake)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	c, err := push.NewAPNs(keyPEM, "KEY1234567", "TEAM123456", "sh.opendesk.app")
	if err != nil {
		t.Fatal(err)
	}
	c.HTTP, c.ProductionURL, c.SandboxURL = srv.Client(), srv.URL, srv.URL
	e.d.APNs = c
	e.d.Cfg.APNsKey, e.d.Cfg.APNsKeyID, e.d.Cfg.APNsTeamID = keyPEM, "KEY1234567", "TEAM123456"
	return fake
}

// nativeUser signs up from the iPhone app and returns the client and its refresh token.
func (e *testEnv) nativeUser(tag string) (*client, string, string) {
	e.t.Helper()
	c := e.client()
	email := uniqueEmail(e.t, tag)
	r := c.do("POST", "/v1/auth/register", map[string]any{"email": email, "password": goodPassword, "name": "Phone " + tag}, iosHeader...)
	if r.Status != 200 {
		e.t.Fatalf("register: %d %s", r.Status, r.Raw)
	}
	c.token = r.str("access_token")
	return c, r.str("refresh_token"), email
}

func hexToken(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	const hexd = "0123456789abcdef"
	out := make([]byte, 64)
	for i, x := range b {
		out[2*i], out[2*i+1] = hexd[x>>4], hexd[x&15]
	}
	return string(out)
}

func (e *testEnv) deviceCount(where string, arg any) int {
	var n int
	_ = e.d.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM push_devices WHERE `+where, arg).Scan(&n)
	return n
}

// TestDevicesEndWithSessions: every way a session ends deletes its devices in the same
// transaction.
func TestDevicesEndWithSessions(t *testing.T) {
	e := newEnv(t)
	withAPNs(t, e)
	phone, refresh, email := e.nativeUser("devices")
	if r := phone.do("POST", "/v1/me/devices", map[string]any{"platform": "ios", "relay_token": "x", "environment": "production",
		"app_version": "1.0", "name": "iPhone"}); r.Status != 422 {
		t.Fatalf("relay token on a direct instance: %d %s", r.Status, r.Raw)
	}
	reg := func(c *client) string {
		t.Helper()
		r := c.do("POST", "/v1/me/devices", map[string]any{"platform": "ios", "token": hexToken(t), "environment": "sandbox",
			"app_version": "1.0 (7)", "name": "Anna's iPhone"})
		if r.Status != 200 || r.Body["current"] != true || r.str("environment") != "sandbox" {
			t.Fatalf("register device: %d %s", r.Status, r.Raw)
		}
		return r.str("id")
	}
	id := reg(phone)
	if r := phone.do("PATCH", "/v1/me/devices/"+id, map[string]any{"muted_types": []string{"sla_breached", "assigned", "assigned"}}); r.Status != 200 ||
		len(r.Body["muted_types"].([]any)) != 2 {
		t.Fatalf("mute: %d %s", r.Status, r.Raw)
	}
	if r := phone.do("PATCH", "/v1/me/devices/"+id, map[string]any{"muted_types": []string{"Not A Type"}}); r.Status != 422 {
		t.Fatalf("bad muted type: %d", r.Status)
	}
	// One phone per session: registering a new token replaces the old one.
	id2 := reg(phone)
	if e.deviceCount("id = $1", id) != 0 || e.deviceCount("id = $1", id2) != 1 {
		t.Fatal("replaced token kept")
	}
	other, _ := e.signup("stranger")
	if r := other.do("DELETE", "/v1/me/devices/"+id2, nil); r.Status != 404 {
		t.Fatalf("another user's device: %d", r.Status)
	}

	// Session revoked from another session.
	web := e.client()
	web.token = web.do("POST", "/v1/auth/login", map[string]any{"email": email, "password": goodPassword}).str("access_token")
	var sid string
	_ = e.d.DB.Pool.QueryRow(context.Background(), `SELECT session_id FROM push_devices WHERE id = $1`, id2).Scan(&sid)
	if r := web.do("DELETE", "/v1/me/sessions/"+sid, nil); r.Status != 204 || e.deviceCount("id = $1", id2) != 0 {
		t.Fatalf("revoke session: %d devices=%d", r.Status, e.deviceCount("id = $1", id2))
	}

	// Logout (body token).
	phone, refresh, email = e.nativeUser("logout")
	id = reg(phone)
	if r := e.client().do("POST", "/v1/auth/logout", map[string]any{"refresh_token": refresh}, iosHeader...); r.Status != 204 || e.deviceCount("id = $1", id) != 0 {
		t.Fatalf("logout kept the device: %d", r.Status)
	}

	// Password change from another session.
	phone, _, email = e.nativeUser("pwchange")
	id = reg(phone)
	web = e.client()
	web.token = web.do("POST", "/v1/auth/login", map[string]any{"email": email, "password": goodPassword}).str("access_token")
	if r := web.do("POST", "/v1/me/password", map[string]any{"current_password": goodPassword, "new_password": goodPassword + "!"}); r.Status != 204 ||
		e.deviceCount("id = $1", id) != 0 {
		t.Fatalf("password change kept the device: %d", r.Status)
	}

	// Refresh token reuse.
	phone, refresh, _ = e.nativeUser("reuse")
	id = reg(phone)
	_ = e.client().do("POST", "/v1/auth/refresh", map[string]any{"refresh_token": refresh}, iosHeader...)
	_, _ = e.d.DB.Pool.Exec(context.Background(), `UPDATE sessions SET rotated_at = now() - interval '1 minute' WHERE prev_refresh_hash = $1`,
		crypto.HashToken(refresh))
	if r := e.client().do("POST", "/v1/auth/refresh", map[string]any{"refresh_token": refresh}, iosHeader...); r.Status != 401 ||
		e.deviceCount("id = $1", id) != 0 {
		t.Fatalf("reuse kept the device: %d", r.Status)
	}

	// Account deletion.
	phone, _, _ = e.nativeUser("delete")
	id = reg(phone)
	if r := phone.do("DELETE", "/v1/me", map[string]any{"password": goodPassword}); r.Status != 204 || e.deviceCount("id = $1", id) != 0 {
		t.Fatalf("account deletion kept the device: %d", r.Status)
	}

	// Explicit removal.
	phone, _, _ = e.nativeUser("remove")
	id = reg(phone)
	if r := phone.do("GET", "/v1/me/devices", nil); r.Status != 200 || !strings.Contains(string(r.Raw), id) {
		t.Fatalf("list: %d %s", r.Status, r.Raw)
	}
	if r := phone.do("DELETE", "/v1/me/devices/"+id, nil); r.Status != 204 || e.deviceCount("id = $1", id) != 0 {
		t.Fatalf("delete: %d", r.Status)
	}
}

// notify stores a notification for uid on a ticket, as hd.Notify does.
func (e *testEnv) notify(t *testing.T, wid, uid string, typ string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	tid, nid := ids.New(), ids.New()
	if _, err := e.d.DB.Pool.Exec(ctx, `INSERT INTO tickets (id, workspace_id, number, subject, channel, brand_id) VALUES ($1, $2,
		(SELECT COALESCE(max(number), 0) + 1 FROM tickets WHERE workspace_id = $2), 'Refund for order 42', 'api',
		(SELECT id FROM brands WHERE workspace_id = $2 AND is_default))`, tid, wid); err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.DB.Pool.Exec(ctx, `INSERT INTO notifications (id, workspace_id, user_id, type, ticket_id) VALUES ($1, $2, $3, $4, $5)`,
		nid, wid, uid, typ, tid); err != nil {
		t.Fatal(err)
	}
	return nid, tid
}

func runPush(t *testing.T, e *testEnv, a jobs.PushNotificationArgs) error {
	t.Helper()
	w := &pushNotificationWorker{s: e.s}
	return w.Work(context.Background(), &river.Job[jobs.PushNotificationArgs]{JobRow: &rivertype.JobRow{}, Args: a})
}

// TestPushDirect: the worker sends the contract payload, with the subject as subtitle,
// skips muted types and deletes devices APNs dropped.
func TestPushDirect(t *testing.T) {
	e := newEnv(t)
	fake := withAPNs(t, e)
	phone, _, _ := e.nativeUser("pushdirect")
	wid := phone.newWorkspace("Push Direct")
	me := phone.do("GET", "/v1/me", nil)
	uid := me.str("user", "id")
	tok := hexToken(t)
	dev := phone.do("POST", "/v1/me/devices", map[string]any{"platform": "ios", "token": tok, "environment": "production", "app_version": "1", "name": "iPhone"})
	if dev.Status != 200 {
		t.Fatalf("device: %d %s", dev.Status, dev.Raw)
	}
	nid, tid := e.notify(t, wid, uid, "customer_replied")
	if err := runPush(t, e, jobs.PushNotificationArgs{WorkspaceID: uuid.MustParse(wid), UserID: uuid.MustParse(uid), NotificationID: nid}); err != nil {
		t.Fatal(err)
	}
	if fake.count() != 1 {
		t.Fatalf("pushes = %d", fake.count())
	}
	body := fake.last()
	aps := body["aps"].(map[string]any)
	alert := aps["alert"].(map[string]any)
	od := body["od"].(map[string]any)
	if alert["loc-key"] != "push.customer_replied.body" || alert["subtitle"] != "Refund for order 42" || aps["badge"] != float64(1) ||
		aps["thread-id"] != tid.String() || od["server"] != e.d.Cfg.APIURL || od["notification_id"] != nid.String() || od["workspace_id"] != wid {
		t.Fatalf("payload = %v", body)
	}

	// hd.Notify enqueues the push in the notification's transaction, for users with a phone.
	hd.ConfigurePush(e.d.Jobs)
	defer hd.ConfigurePush(nil)
	err := pgx.BeginFunc(context.Background(), e.d.DB.Pool, func(tx pgx.Tx) error {
		return hd.Notify(context.Background(), tx, uuid.MustParse(wid), uuid.MustParse(uid), hd.NotifyAssigned, &tid, nil)
	})
	var pushJobs int
	_ = e.d.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM river_job WHERE kind = 'push.notification' AND args->>'user_id' = $1`,
		uid).Scan(&pushJobs)
	if err != nil || pushJobs != 1 {
		t.Fatalf("notify push job: %v jobs=%d", err, pushJobs)
	}

	// Muted type: nothing sent.
	phone.do("PATCH", "/v1/me/devices/"+dev.str("id"), map[string]any{"muted_types": []string{"customer_replied"}})
	nid2, _ := e.notify(t, wid, uid, "customer_replied")
	_ = runPush(t, e, jobs.PushNotificationArgs{WorkspaceID: uuid.MustParse(wid), UserID: uuid.MustParse(uid), NotificationID: nid2})
	if fake.count() != 1 {
		t.Fatal("muted type pushed")
	}

	// Reading enqueues a badge update; a dropped token deletes the device.
	if r := phone.do("POST", "/v1/notifications/read", map[string]any{"all": true}); r.Status != 204 {
		t.Fatalf("read: %d", r.Status)
	}
	var badgeJobs int
	_ = e.d.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM river_job WHERE kind = 'push.badge' AND args->>'user_id' = $1`, uid).Scan(&badgeJobs)
	if badgeJobs != 1 {
		t.Fatalf("badge jobs = %d", badgeJobs)
	}
	fake.gone[tok] = true
	bw := &pushBadgeWorker{s: e.s}
	if err := bw.Work(context.Background(), &river.Job[jobs.PushBadgeArgs]{JobRow: &rivertype.JobRow{}, Args: jobs.PushBadgeArgs{UserID: uuid.MustParse(uid)}}); err != nil {
		t.Fatal(err)
	}
	if b := fake.last(); len(b) != 1 || b["aps"].(map[string]any)["badge"] != float64(0) {
		t.Fatalf("badge payload = %v", b)
	}
	if e.deviceCount("id = $1", dev.str("id")) != 0 {
		t.Fatal("device APNs dropped was kept")
	}
}

// TestPushRelay: the relay checks the instance's signature and the token's instance,
// and builds the payload itself without any ticket text.
func TestPushRelay(t *testing.T) {
	e := newEnv(t)
	fake := withAPNs(t, e)
	// This test server is both the relay and the instance.
	e.d.Cfg.APIURL = e.srv.URL
	e.d.Cfg.PushRelayServe = true
	key := e.s.instanceKey()

	if r := e.client().do("GET", "/v1/push/instance-key", nil); r.str("public_key") != push.EncodeKey(key.Public().(ed25519.PublicKey)) {
		t.Fatalf("instance key: %s", r.Raw)
	}
	tok := hexToken(t)
	reg := e.client().do("POST", "/v1/push/relay/register", map[string]any{"token": tok, "environment": "production", "instance": e.srv.URL + "/"})
	if reg.Status != 200 || reg.str("relay_token") == "" || strings.Contains(reg.str("relay_token"), tok[:16]) {
		t.Fatalf("relay register: %d %s", reg.Status, reg.Raw)
	}
	relayToken := reg.str("relay_token")

	nid, tid, wid := uuid.New(), uuid.New(), uuid.New()
	num := int64(77)
	p := push.Push{Kind: push.KindAlert, Type: "assigned", NotificationID: &nid, TicketID: &tid, TicketNumber: &num, WorkspaceID: &wid, Badge: 2}
	send := func(instance string, body []byte, ts string, sig string) resp {
		t.Helper()
		return e.client().doRaw("POST", "/v1/push/relay/send", body, push.HeaderInstance, instance, push.HeaderTimestamp, ts, push.HeaderSignature, sig)
	}
	body, _ := json.Marshal(push.RelaySend{RelayToken: relayToken, Push: p})
	now := strconv.FormatInt(time.Now().Unix(), 10)
	if r := send(e.srv.URL, body, now, push.Sign(key, now, body)); r.Status != 204 {
		t.Fatalf("relay send: %d %s", r.Status, r.Raw)
	}
	got := fake.last()
	if _, ok := got["aps"].(map[string]any)["alert"].(map[string]any)["subtitle"]; ok || got["od"].(map[string]any)["server"] != e.srv.URL ||
		got["od"].(map[string]any)["ticket_number"] != float64(77) {
		t.Fatalf("relayed payload = %v", got)
	}

	other, _ := push.InstanceKey([]byte("another-auth-secret-another-secret-00"))
	stale := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	if r := send(e.srv.URL, body, now, push.Sign(other, now, body)); r.Status != 401 {
		t.Fatalf("other key's signature: %d", r.Status)
	}
	if r := send(e.srv.URL, body, stale, push.Sign(key, stale, body)); r.Status != 401 {
		t.Fatalf("stale timestamp: %d", r.Status)
	}
	// A relay token registered for another instance is useless to this one.
	foreign := e.client().do("POST", "/v1/push/relay/register", map[string]any{"token": tok, "environment": "production",
		"instance": "https://desk.example.com"}).str("relay_token")
	fb, _ := json.Marshal(push.RelaySend{RelayToken: foreign, Push: p})
	if r := send(e.srv.URL, fb, now, push.Sign(key, now, fb)); r.Status != 401 {
		t.Fatalf("instance mismatch: %d %s", r.Status, r.Raw)
	}
	// Ticket text cannot even be sent: the body refuses unknown members.
	withText := []byte(strings.Replace(string(body), `"kind":"alert"`, `"kind":"alert","subject":"Refund for order 42"`, 1))
	if r := send(e.srv.URL, withText, now, push.Sign(key, now, withText)); r.Status != 422 {
		t.Fatalf("ticket text accepted: %d %s", r.Status, r.Raw)
	}
	n := fake.count()

	// The instance side: a relay device delivered through the relay client, 410 deletes it.
	e.s.push.relay = &push.RelayClient{URL: e.srv.URL, Instance: e.srv.URL, Key: key, HTTP: e.srv.Client()}
	phone, _, _ := e.nativeUser("relayphone")
	e.d.Cfg.APNsKey, e.d.Cfg.PushRelayURL = "", e.srv.URL // this instance: push = relay
	ws := phone.newWorkspace("Relay Desk")
	uid := phone.do("GET", "/v1/me", nil).str("user", "id")
	dev := phone.do("POST", "/v1/me/devices", map[string]any{"platform": "ios", "relay_token": relayToken, "environment": "production",
		"app_version": "1", "name": "iPhone"})
	if dev.Status != 200 {
		t.Fatalf("relay device: %d %s", dev.Status, dev.Raw)
	}
	nid2, _ := e.notify(t, ws, uid, "mentioned")
	if err := runPush(t, e, jobs.PushNotificationArgs{WorkspaceID: uuid.MustParse(ws), UserID: uuid.MustParse(uid), NotificationID: nid2}); err != nil {
		t.Fatal(err)
	}
	if fake.count() != n+1 {
		t.Fatal("relay push not delivered")
	}
	if _, ok := fake.last()["aps"].(map[string]any)["alert"].(map[string]any)["subtitle"]; ok {
		t.Fatal("subtitle went through the relay")
	}
	fake.gone[tok] = true
	nid3, _ := e.notify(t, ws, uid, "mentioned")
	_ = runPush(t, e, jobs.PushNotificationArgs{WorkspaceID: uuid.MustParse(ws), UserID: uuid.MustParse(uid), NotificationID: nid3})
	if e.deviceCount("id = $1", dev.str("id")) != 0 {
		t.Fatal("device kept after the relay answered 410")
	}
}

// doRaw sends exact body bytes (signed relay requests).
func (c *client) doRaw(method, path string, body []byte, hdr ...string) resp {
	c.e.t.Helper()
	req, _ := http.NewRequest(method, c.e.srv.URL+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: raw, Header: res.Header}
	_ = json.Unmarshal(raw, &out.Body)
	return out
}

func chiCtx(r *http.Request, k, v string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add(k, v)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}
