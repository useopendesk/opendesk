package core

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/auth"
)

// softKey is a software passkey for rpID http://app.test (the test APP_URL).
type softKey struct {
	t      *testing.T
	priv   *ecdsa.PrivateKey
	credID []byte
	handle []byte
	count  uint32
}

const (
	testRPID   = "app.test"
	testOrigin = "http://app.test"
)

var b64 = base64.RawURLEncoding

func newSoftKey(t *testing.T) *softKey {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	_, _ = rand.Read(id)
	return &softKey{t: t, priv: priv, credID: id}
}

func clientData(typ, challenge string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": testOrigin, "crossOrigin": false})
	return b
}

func (k *softKey) authData(flags byte, attested []byte) []byte {
	h := sha256.Sum256([]byte(testRPID))
	out := append([]byte{}, h[:]...)
	out = append(out, flags)
	out = binary.BigEndian.AppendUint32(out, k.count)
	return append(out, attested...)
}

// register answers creation options with a "none" attestation (RegistrationResponseJSON).
func (k *softKey) register(options map[string]any) map[string]any {
	k.t.Helper()
	user, _ := options["user"].(map[string]any)
	handle, err := b64.DecodeString(user["id"].(string))
	if err != nil {
		k.t.Fatalf("user.id: %v", err)
	}
	k.handle = handle
	enc, _ := cbor.CoreDetEncOptions().EncMode()
	x, y := k.priv.X.FillBytes(make([]byte, 32)), k.priv.Y.FillBytes(make([]byte, 32))
	cose, err := enc.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: x, -3: y})
	if err != nil {
		k.t.Fatal(err)
	}
	attested := make([]byte, 16) // zero AAGUID
	attested = binary.BigEndian.AppendUint16(attested, uint16(len(k.credID)))
	attested = append(attested, k.credID...)
	attested = append(attested, cose...)
	ad := k.authData(0x01|0x04|0x40, attested) // UP, UV, AT
	att, err := enc.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": ad})
	if err != nil {
		k.t.Fatal(err)
	}
	return map[string]any{
		"id": b64.EncodeToString(k.credID), "rawId": b64.EncodeToString(k.credID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(clientData("webauthn.create", options["challenge"].(string))),
			"attestationObject": b64.EncodeToString(att),
			"transports":        []string{"internal", "hybrid"},
		},
		"clientExtensionResults": map[string]any{}, "authenticatorAttachment": "platform",
	}
}

// assert signs request options (AuthenticationResponseJSON) with the given counter.
func (k *softKey) assert(options map[string]any, count uint32, uv bool) map[string]any {
	k.t.Helper()
	k.count = count
	flags := byte(0x01)
	if uv {
		flags |= 0x04
	}
	ad := k.authData(flags, nil)
	cd := clientData("webauthn.get", options["challenge"].(string))
	cdh := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), cdh[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, k.priv, digest[:])
	if err != nil {
		k.t.Fatal(err)
	}
	return map[string]any{
		"id": b64.EncodeToString(k.credID), "rawId": b64.EncodeToString(k.credID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(cd),
			"authenticatorData": b64.EncodeToString(ad),
			"signature":         b64.EncodeToString(sig),
			"userHandle":        b64.EncodeToString(k.handle),
		},
		"clientExtensionResults": map[string]any{}, "authenticatorAttachment": "platform",
	}
}

func optionsOf(t *testing.T, r resp) map[string]any {
	t.Helper()
	if r.Status != 200 || r.str("passkey_token") == "" {
		t.Fatalf("begin: %d %s", r.Status, r.Raw)
	}
	o, _ := r.Body["options"].(map[string]any)
	return o
}

func (e *testEnv) auditCount(wid, action, method string) int {
	var n int
	_ = e.d.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = $2
		AND ($3 = '' OR data->>'method' = $3)`, wid, action, method).Scan(&n)
	return n
}

func TestPasskeyFlow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if r := e.client().do("GET", "/v1/auth/config", nil); r.Body["passkeys"] != true {
		t.Fatalf("auth config: %s", r.Raw)
	}
	owner, email := e.signup("passkey")
	wid := owner.newWorkspace("Passkey Co")
	uid := owner.do("GET", "/v1/me", nil).str("user", "id")

	// Register.
	begin := owner.do("POST", "/v1/me/passkeys/register/begin", map[string]any{})
	opts := optionsOf(t, begin)
	if rp, _ := opts["rp"].(map[string]any); rp["id"] != testRPID {
		t.Fatalf("rp: %v", opts["rp"])
	}
	if sel, _ := opts["authenticatorSelection"].(map[string]any); sel["residentKey"] != "required" || sel["userVerification"] != "preferred" {
		t.Fatalf("authenticatorSelection: %v", opts["authenticatorSelection"])
	}
	if ex, _ := opts["excludeCredentials"].([]any); len(ex) != 0 {
		t.Fatalf("excludeCredentials: %v", ex)
	}
	key := newSoftKey(t)
	cred := key.register(opts)
	other, _ := e.signup("intruder")
	if r := other.do("POST", "/v1/me/passkeys/register/finish", map[string]any{"passkey_token": begin.str("passkey_token"), "credential": cred}); r.Status != 400 || r.str("code") != "invalid_passkey" {
		t.Fatalf("another user's register token: %d %s", r.Status, r.Raw)
	}
	fin := owner.do("POST", "/v1/me/passkeys/register/finish", map[string]any{"passkey_token": begin.str("passkey_token"), "credential": cred})
	if fin.Status != 201 || fin.str("name") != "Passkey" || fin.str("id") == "" {
		t.Fatalf("register finish: %d %s", fin.Status, fin.Raw)
	}
	pkID := fin.str("id")
	if r := owner.do("POST", "/v1/me/passkeys/register/finish", map[string]any{"passkey_token": begin.str("passkey_token"), "credential": cred}); r.Status != 400 {
		t.Fatalf("register token reused: %d %s", r.Status, r.Raw)
	}
	again := optionsOf(t, owner.do("POST", "/v1/me/passkeys/register/begin", nil))
	if ex, _ := again["excludeCredentials"].([]any); len(ex) != 1 {
		t.Fatalf("excludeCredentials after register: %v", again["excludeCredentials"])
	}

	// List is scoped to the user.
	if r := owner.do("GET", "/v1/me/passkeys", nil); r.Status != 200 || len(r.Body["data"].([]any)) != 1 {
		t.Fatalf("list: %d %s", r.Status, r.Raw)
	}
	if r := other.do("GET", "/v1/me/passkeys", nil); r.Status != 200 || len(r.Body["data"].([]any)) != 0 {
		t.Fatalf("other's list: %d %s", r.Status, r.Raw)
	}

	// Sign in with it: discoverable, no email.
	c := e.client()
	lb := c.do("POST", "/v1/auth/passkey/login/begin", map[string]any{})
	lopts := optionsOf(t, lb)
	if ac, _ := lopts["allowCredentials"].([]any); len(ac) != 0 {
		t.Fatalf("allowCredentials: %v", lopts["allowCredentials"])
	}
	assertion := key.assert(lopts, 1, true)
	lf := c.do("POST", "/v1/auth/passkey/login/finish", map[string]any{"passkey_token": lb.str("passkey_token"), "credential": assertion})
	if lf.Status != 200 || lf.str("access_token") == "" || lf.str("user", "email") != email || c.cookie(RefreshCookieName(uuid.MustParse(lf.str("user", "id")))) == "" {
		t.Fatalf("login finish: %d %s", lf.Status, lf.Raw)
	}
	claims, err := e.d.Tokens.Verify(lf.str("access_token"), auth.PurposeAccess)
	if err != nil || !claims.MFA {
		t.Fatalf("access token mfa: %v %+v", err, claims)
	}
	c.token = lf.str("access_token")
	if r := c.do("GET", "/v1/me", nil); r.Status != 200 || r.str("user", "id") != uid {
		t.Fatalf("session: %d %s", r.Status, r.Raw)
	}
	if r := c.do("POST", "/v1/auth/refresh", nil); r.Status != 200 {
		t.Fatalf("refresh: %d %s", r.Status, r.Raw)
	}
	var count int64
	var used *time.Time
	_ = e.d.DB.Pool.QueryRow(ctx, `SELECT sign_count, last_used_at FROM passkeys WHERE id = $1`, pkID).Scan(&count, &used)
	if count != 1 || used == nil {
		t.Fatalf("counter not stored: %d %v", count, used)
	}

	// The same token again is refused, even with a fresh valid signature.
	replay := c.do("POST", "/v1/auth/passkey/login/finish", map[string]any{"passkey_token": lb.str("passkey_token"), "credential": key.assert(lopts, 2, true)})
	if replay.Status != 401 || replay.str("code") != "invalid_credentials" {
		t.Fatalf("replayed passkey token: %d %s", replay.Status, replay.Raw)
	}

	// A token of another purpose is refused the same way.
	mfaTok, _, _ := e.d.Tokens.Sign(auth.Claims{Purpose: auth.PurposeMFA}, uid, auth.MFATTL)
	lb2 := c.do("POST", "/v1/auth/passkey/login/begin", nil)
	wrong := c.do("POST", "/v1/auth/passkey/login/finish", map[string]any{"passkey_token": mfaTok, "credential": key.assert(optionsOf(t, lb2), 2, true)})
	if wrong.Status != 401 || wrong.str("code") != "invalid_credentials" || wrong.str("detail") != replay.str("detail") {
		t.Fatalf("wrong purpose: %d %s", wrong.Status, wrong.Raw)
	}

	// An unknown credential answers the same.
	stranger := newSoftKey(t)
	stranger.handle = key.handle
	lb3 := c.do("POST", "/v1/auth/passkey/login/begin", nil)
	unknown := c.do("POST", "/v1/auth/passkey/login/finish", map[string]any{"passkey_token": lb3.str("passkey_token"), "credential": stranger.assert(optionsOf(t, lb3), 5, true)})
	if unknown.Status != 401 || unknown.str("detail") != replay.str("detail") {
		t.Fatalf("unknown credential: %d %s", unknown.Status, unknown.Raw)
	}

	// A bad signature answers the same.
	lb4 := c.do("POST", "/v1/auth/passkey/login/begin", nil)
	bad := key.assert(optionsOf(t, lb4), 7, true)
	bad["response"].(map[string]any)["signature"] = stranger.assert(optionsOf(t, lb4), 7, true)["response"].(map[string]any)["signature"]
	if r := c.do("POST", "/v1/auth/passkey/login/finish", map[string]any{"passkey_token": lb4.str("passkey_token"), "credential": bad}); r.Status != 401 || r.str("detail") != replay.str("detail") {
		t.Fatalf("bad signature: %d %s", r.Status, r.Raw)
	}

	// A counter that does not advance is refused.
	lb5 := c.do("POST", "/v1/auth/passkey/login/begin", nil)
	if r := c.do("POST", "/v1/auth/passkey/login/finish", map[string]any{"passkey_token": lb5.str("passkey_token"), "credential": key.assert(optionsOf(t, lb5), 1, true)}); r.Status != 401 {
		t.Fatalf("counter regression: %d %s", r.Status, r.Raw)
	}
	lb6 := c.do("POST", "/v1/auth/passkey/login/begin", nil)
	if r := c.do("POST", "/v1/auth/passkey/login/finish", map[string]any{"passkey_token": lb6.str("passkey_token"), "credential": key.assert(optionsOf(t, lb6), 9, true)}); r.Status != 200 {
		t.Fatalf("advanced counter: %d %s", r.Status, r.Raw)
	}

	// Others cannot rename or remove it.
	if r := other.do("PATCH", "/v1/me/passkeys/"+pkID, map[string]any{"name": "Mine now"}); r.Status != 404 {
		t.Fatalf("other renamed: %d %s", r.Status, r.Raw)
	}
	if r := other.do("DELETE", "/v1/me/passkeys/"+pkID, nil); r.Status != 404 {
		t.Fatalf("other deleted: %d %s", r.Status, r.Raw)
	}
	if r := owner.do("PATCH", "/v1/me/passkeys/"+pkID, map[string]any{"name": "Work laptop"}); r.Status != 200 || r.str("name") != "Work laptop" {
		t.Fatalf("rename: %d %s", r.Status, r.Raw)
	}
	if r := owner.do("PATCH", "/v1/me/passkeys/"+pkID, map[string]any{"name": "https://evil.example"}); r.Status != 422 {
		t.Fatalf("link name: %d %s", r.Status, r.Raw)
	}

	// Audit trail.
	if e.auditCount(wid, "passkey.added", "") != 1 || e.auditCount(wid, "login", "passkey") != 2 || e.auditCount(wid, "login.failed", "passkey") < 2 {
		t.Fatalf("audit: added=%d login=%d failed=%d", e.auditCount(wid, "passkey.added", ""), e.auditCount(wid, "login", "passkey"),
			e.auditCount(wid, "login.failed", "passkey"))
	}
	if r := owner.do("DELETE", "/v1/me/passkeys/"+pkID, nil); r.Status != 204 {
		t.Fatalf("delete: %d %s", r.Status, r.Raw)
	}
	if e.auditCount(wid, "passkey.removed", "") != 1 {
		t.Fatal("removal not audited")
	}
	lb7 := c.do("POST", "/v1/auth/passkey/login/begin", nil)
	if r := c.do("POST", "/v1/auth/passkey/login/finish", map[string]any{"passkey_token": lb7.str("passkey_token"), "credential": key.assert(optionsOf(t, lb7), 10, true)}); r.Status != 401 {
		t.Fatalf("removed passkey signed in: %d %s", r.Status, r.Raw)
	}
}

func TestPasskeyAccountDeletion(t *testing.T) {
	e := newEnv(t)
	u, _ := e.signup("pkdelete")
	key := newSoftKey(t)
	b := u.do("POST", "/v1/me/passkeys/register/begin", nil)
	if r := u.do("POST", "/v1/me/passkeys/register/finish", map[string]any{"passkey_token": b.str("passkey_token"), "credential": key.register(optionsOf(t, b)), "name": "YubiKey 5"}); r.Status != 201 || r.str("name") != "YubiKey 5" {
		t.Fatalf("register: %d %s", r.Status, r.Raw)
	}
	uid := uuid.MustParse(u.do("GET", "/v1/me", nil).str("user", "id"))
	if r := u.do("DELETE", "/v1/me", map[string]any{"password": goodPassword}); r.Status != 204 {
		t.Fatalf("delete account: %d %s", r.Status, r.Raw)
	}
	var n int
	_ = e.d.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM passkeys WHERE user_id = $1`, uid).Scan(&n)
	if n != 0 {
		t.Fatalf("passkeys survived account deletion: %d", n)
	}
}

func TestPasskeyWithoutUserVerificationKeepsTOTP(t *testing.T) {
	e := newEnv(t)
	u, _ := e.signup("pkuv")
	key := newSoftKey(t)
	b := u.do("POST", "/v1/me/passkeys/register/begin", nil)
	if r := u.do("POST", "/v1/me/passkeys/register/finish", map[string]any{"passkey_token": b.str("passkey_token"), "credential": key.register(optionsOf(t, b))}); r.Status != 201 {
		t.Fatalf("register: %d %s", r.Status, r.Raw)
	}
	_, _ = e.d.DB.Pool.Exec(context.Background(), `UPDATE users SET totp_enabled = true, totp_secret_enc = '\x00' WHERE id = $1`,
		uuid.MustParse(u.do("GET", "/v1/me", nil).str("user", "id")))
	c := e.client()
	lb := c.do("POST", "/v1/auth/passkey/login/begin", nil)
	r := c.do("POST", "/v1/auth/passkey/login/finish", map[string]any{"passkey_token": lb.str("passkey_token"), "credential": key.assert(optionsOf(t, lb), 1, false)})
	if r.Status != 200 || r.Body["mfa_required"] != true || r.str("mfa_token") == "" || r.str("access_token") != "" {
		t.Fatalf("passkey without user verification: %d %s", r.Status, r.Raw)
	}
}
