package push

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/crypto"
)

func testKeyPEM(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	return k, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestParseKeyForms(t *testing.T) {
	want, text := testKeyPEM(t)
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	for name, v := range map[string]string{"pem": text, "escaped": strings.ReplaceAll(strings.TrimSpace(text), "\n", `\n`), "base64": b64} {
		got, err := ParseKey(v)
		if err != nil || !got.Equal(want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, v := range map[string]string{"empty": " ", "garbage": "not a key!", "pem junk": "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----"} {
		_, err := ParseKey(v)
		if err == nil {
			t.Errorf("%s accepted", name)
		} else if strings.TrimSpace(v) != "" && strings.Contains(err.Error(), strings.TrimSpace(v)) {
			t.Errorf("%s: error echoes the key", name)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestPayload(t *testing.T) {
	nid, tid, wid := uuid.New(), uuid.New(), uuid.New()
	p := Push{Kind: KindAlert, Type: "customer_replied", NotificationID: &nid, TicketID: &tid, TicketNumber: ptr(int64(1234)),
		WorkspaceID: &wid, Badge: 3}
	raw, err := Payload(p, "https://api.example.com", "Refund for order 42")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		APS struct {
			Alert struct {
				TitleLocKey string   `json:"title-loc-key"`
				LocKey      string   `json:"loc-key"`
				LocArgs     []string `json:"loc-args"`
				Subtitle    string   `json:"subtitle"`
			} `json:"alert"`
			Badge    int    `json:"badge"`
			Sound    string `json:"sound"`
			ThreadID string `json:"thread-id"`
			Category string `json:"category"`
		} `json:"aps"`
		OD map[string]any `json:"od"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	a := got.APS
	if a.Alert.TitleLocKey != "push.customer_replied.title" || a.Alert.LocKey != "push.customer_replied.body" ||
		len(a.Alert.LocArgs) != 1 || a.Alert.LocArgs[0] != "1234" || a.Alert.Subtitle != "Refund for order 42" ||
		a.Badge != 3 || a.Sound != "default" || a.ThreadID != tid.String() || a.Category != "TICKET" {
		t.Fatalf("aps = %+v", a)
	}
	od := got.OD
	if od["v"] != float64(1) || od["server"] != "https://api.example.com" || od["workspace_id"] != wid.String() ||
		od["notification_id"] != nid.String() || od["ticket_id"] != tid.String() || od["ticket_number"] != float64(1234) ||
		od["type"] != "customer_replied" {
		t.Fatalf("od = %v", od)
	}
	if p.Priority() != 10 || p.CollapseID() != nid.String() {
		t.Fatalf("priority %d collapse %q", p.Priority(), p.CollapseID())
	}

	// Without a subtitle (the relay) no ticket text is in the payload at all.
	raw, _ = Payload(p, "https://api.example.com", "")
	if strings.Contains(string(raw), "subtitle") {
		t.Fatalf("subtitle without one: %s", raw)
	}

	b := Push{Kind: KindBadge, Badge: 7}
	raw, _ = Payload(b, "https://api.example.com", "ignored")
	if string(raw) != `{"aps":{"badge":7}}` || b.Priority() != 5 {
		t.Fatalf("badge payload %s priority %d", raw, b.Priority())
	}
}

func TestSignVerify(t *testing.T) {
	key, err := InstanceKey([]byte("test-auth-secret-test-auth-secret-0000"))
	if err != nil {
		t.Fatal(err)
	}
	again, _ := InstanceKey([]byte("test-auth-secret-test-auth-secret-0000"))
	other, _ := InstanceKey([]byte("another-auth-secret-another-secret-00"))
	if !key.Equal(again) || key.Equal(other) {
		t.Fatal("instance key is not a function of AUTH_SECRET")
	}
	pub, err := DecodeKey(EncodeKey(key.Public().(ed25519.PublicKey)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"relay_token":"x","push":{"kind":"badge","badge":1}}`)
	sig := Sign(key, ts, body)
	if err := Verify(pub, ts, body, sig, now); err != nil {
		t.Fatalf("valid signature refused: %v", err)
	}
	cases := map[string]error{
		"tampered body": Verify(pub, ts, append([]byte{}, append(body, ' ')...), sig, now),
		"other key":     Verify(other.Public().(ed25519.PublicKey), ts, body, sig, now),
		"stale":         Verify(pub, ts, body, sig, now.Add(6*time.Minute)),
		"future":        Verify(pub, ts, body, sig, now.Add(-6*time.Minute)),
		"other ts":      Verify(pub, strconv.FormatInt(now.Unix()+1, 10), body, sig, now),
		"no signature":  Verify(pub, ts, body, "", now),
		"bad timestamp": Verify(pub, "soon", body, sig, now),
	}
	for name, err := range cases {
		if !errors.Is(err, ErrSignature) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if err := Verify(pub, ts, body, sig, now.Add(4*time.Minute)); err != nil {
		t.Errorf("within skew refused: %v", err)
	}
}

func TestNormalizeInstance(t *testing.T) {
	ok := map[string]string{
		"https://API.example.com/":     "https://api.example.com",
		"https://desk.example.com/api": "https://desk.example.com/api",
		"https://x.example.com:8443":   "https://x.example.com:8443",
	}
	for in, want := range ok {
		if got, err := NormalizeInstance(in, false); err != nil || got != want {
			t.Errorf("%s: %q %v", in, got, err)
		}
	}
	for _, in := range []string{"http://api.example.com", "https://u:p@api.example.com", "https://api.example.com/?a=1", "ftp://x", "", "api.example.com"} {
		if _, err := NormalizeInstance(in, false); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
	if _, err := NormalizeInstance("http://127.0.0.1:8080", false); err == nil {
		t.Error("http loopback accepted in production")
	}
	if _, err := NormalizeInstance("http://127.0.0.1:8080", true); err != nil {
		t.Errorf("http loopback refused in development: %v", err)
	}
}

func TestRelayToken(t *testing.T) {
	box, _ := crypto.NewBox([]byte("development-only-credentials-key"))
	other, _ := crypto.NewBox([]byte("another-credentials-key-32-bytes"))
	rt, err := SealRelayToken(box, RelayToken{Token: strings.Repeat("ab", 32), Environment: EnvSandbox, Instance: "https://a.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := OpenRelayToken(box, rt)
	if err != nil || got.Instance != "https://a.example.com" || got.Environment != EnvSandbox {
		t.Fatalf("open: %+v %v", got, err)
	}
	if strings.Contains(rt, strings.Repeat("ab", 8)) {
		t.Fatal("relay token shows the device token")
	}
	if _, err := OpenRelayToken(other, rt); err == nil {
		t.Error("opened with another key")
	}
	raw, _ := base64.RawURLEncoding.DecodeString(rt)
	raw[len(raw)-1] ^= 1
	if _, err := OpenRelayToken(box, base64.RawURLEncoding.EncodeToString(raw)); err == nil {
		t.Error("tampered relay token opened")
	}
}

// fakeAPNs records requests and answers per device token.
type fakeAPNs struct {
	mu   sync.Mutex
	reqs []*http.Request
	body [][]byte
	// answer maps a device token to a status and reason; default 200.
	answer map[string][2]string
}

func (f *fakeAPNs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.reqs = append(f.reqs, r)
	f.body = append(f.body, b)
	a, ok := f.answer[strings.TrimPrefix(r.URL.Path, "/3/device/")]
	f.mu.Unlock()
	if !ok {
		w.WriteHeader(200)
		return
	}
	code, _ := strconv.Atoi(a[0])
	w.WriteHeader(code)
	_, _ = w.Write([]byte(`{"reason":"` + a[1] + `"}`))
}

func TestAPNsSend(t *testing.T) {
	key, text := testKeyPEM(t)
	fake := &fakeAPNs{answer: map[string][2]string{
		strings.Repeat("0", 64): {"410", "Unregistered"},
		strings.Repeat("1", 64): {"400", "BadDeviceToken"},
		strings.Repeat("2", 64): {"500", "InternalServerError"},
	}}
	srv := httptest.NewUnstartedServer(fake)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	c, err := NewAPNs(text, "KEY1234567", "TEAM123456", "sh.opendesk.app")
	if err != nil {
		t.Fatal(err)
	}
	c.HTTP, c.ProductionURL, c.SandboxURL = srv.Client(), srv.URL, srv.URL+"/sandbox"

	nid := uuid.New()
	p := Push{Kind: KindAlert, Type: "assigned", NotificationID: &nid, Badge: 1}
	payload, _ := Payload(p, "https://api.example.com", "")
	tok := strings.Repeat("AB", 32)
	if err := c.Send(context.Background(), tok, EnvProduction, p, payload); err != nil {
		t.Fatal(err)
	}
	r := fake.reqs[0]
	if r.ProtoMajor != 2 || r.URL.Path != "/3/device/"+strings.ToLower(tok) || r.Header.Get("apns-topic") != "sh.opendesk.app" ||
		r.Header.Get("apns-push-type") != "alert" || r.Header.Get("apns-priority") != "10" || r.Header.Get("apns-collapse-id") != nid.String() {
		t.Fatalf("request: proto %d path %s headers %v", r.ProtoMajor, r.URL.Path, r.Header)
	}
	bearer := strings.TrimPrefix(r.Header.Get("authorization"), "bearer ")
	parsed, err := jwt.Parse(bearer, func(*jwt.Token) (any, error) { return &key.PublicKey, nil }, jwt.WithValidMethods([]string{"ES256"}))
	if err != nil || parsed.Header["kid"] != "KEY1234567" || parsed.Claims.(jwt.MapClaims)["iss"] != "TEAM123456" {
		t.Fatalf("provider token: %v %v", err, parsed)
	}
	// The provider token is reused, not re-signed per push.
	_ = c.Send(context.Background(), tok, EnvSandbox, Push{Kind: KindBadge}, []byte(`{"aps":{"badge":0}}`))
	if fake.reqs[1].Header.Get("authorization") != r.Header.Get("authorization") || !strings.HasPrefix(fake.reqs[1].URL.Path, "/sandbox/") ||
		fake.reqs[1].Header.Get("apns-priority") != "5" {
		t.Fatalf("badge request: %s %v", fake.reqs[1].URL.Path, fake.reqs[1].Header)
	}

	for _, gone := range []string{strings.Repeat("0", 64), strings.Repeat("1", 64), "not-hex"} {
		if err := c.Send(context.Background(), gone, EnvProduction, p, payload); !errors.Is(err, ErrUnregistered) {
			t.Errorf("%s: err = %v", gone, err)
		}
	}
	if err := c.Send(context.Background(), strings.Repeat("2", 64), EnvProduction, p, payload); err == nil || errors.Is(err, ErrUnregistered) {
		t.Errorf("server error: %v", err)
	}
	// A transport error never carries the device token (it ends up in logs).
	c.ProductionURL = "https://127.0.0.1:1"
	if err := c.Send(context.Background(), strings.Repeat("3", 64), EnvProduction, p, payload); err == nil ||
		strings.Contains(err.Error(), strings.Repeat("3", 16)) {
		t.Errorf("transport error: %v", err)
	}
}
