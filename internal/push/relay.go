package push

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/netsafe"
)

// Relay request headers.
const (
	HeaderInstance  = "OD-Push-Instance"
	HeaderTimestamp = "OD-Push-Timestamp"
	HeaderSignature = "OD-Push-Signature"
)

// MaxClockSkew bounds how far a signed relay request's timestamp may be from now.
const MaxClockSkew = 5 * time.Minute

const instanceKeyInfo = "opendesk-push-instance/v1"

// ErrSignature means a relay request is unsigned, stale or signed by another key.
var ErrSignature = errors.New("push: relay signature invalid")

// InstanceKey derives this instance's Ed25519 signing key from AUTH_SECRET
// (HKDF-SHA256, info "opendesk-push-instance/v1"). It needs no storage and changes only
// with AUTH_SECRET.
func InstanceKey(secret []byte) (ed25519.PrivateKey, error) {
	seed, err := hkdf.Key(sha256.New, secret, nil, instanceKeyInfo, ed25519.SeedSize)
	if err != nil {
		return nil, err
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// EncodeKey is the base64url form of a public key served at /v1/push/instance-key.
func EncodeKey(pub ed25519.PublicKey) string { return base64.RawURLEncoding.EncodeToString(pub) }

// DecodeKey parses a served public key.
func DecodeKey(s string) (ed25519.PublicKey, error) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("push: malformed instance key")
	}
	return ed25519.PublicKey(b), nil
}

func signedMessage(ts string, body []byte) []byte {
	msg := make([]byte, 0, len(ts)+1+len(body))
	msg = append(msg, ts...)
	msg = append(msg, '\n')
	return append(msg, body...)
}

// Sign signs a relay request body at ts (unix seconds).
func Sign(key ed25519.PrivateKey, ts string, body []byte) string {
	return base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, signedMessage(ts, body)))
}

// Verify checks a relay request's signature and that its timestamp is within
// MaxClockSkew of now.
func Verify(pub ed25519.PublicKey, ts string, body []byte, sig string, now time.Time) error {
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ErrSignature
	}
	if d := now.Sub(time.Unix(sec, 0)); d > MaxClockSkew || d < -MaxClockSkew {
		return ErrSignature
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(sig, "="))
	if err != nil || len(raw) != ed25519.SignatureSize || len(pub) != ed25519.PublicKeySize {
		return ErrSignature
	}
	if !ed25519.Verify(pub, signedMessage(ts, body), raw) {
		return ErrSignature
	}
	return nil
}

// NormalizeInstance turns an instance API URL into the canonical form both the relay
// token and the OD-Push-Instance header are compared in: scheme, lowercase host, an
// optional path, no trailing slash. https only, except loopback on development relays.
func NormalizeInstance(raw string, dev bool) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 500 || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", errors.New("push: instance must be an https URL")
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && dev && netsafe.IsLoopbackHost(u.Hostname()):
	default:
		return "", errors.New("push: instance must be an https URL")
	}
	return u.Scheme + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.EscapedPath(), "/"), nil
}

// RelayToken is what a relay token seals: the APNs token, its environment and the only
// instance allowed to push to it.
type RelayToken struct {
	Token       string `json:"t"`
	Environment string `json:"e"`
	Instance    string `json:"i"`
}

// SealRelayToken seals t with the relay's key; only the relay can open it.
func SealRelayToken(box *crypto.Box, t RelayToken) (string, error) {
	sealed, err := box.SealJSON(t)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// OpenRelayToken opens a relay token sealed by this relay.
func OpenRelayToken(box *crypto.Box, s string) (RelayToken, error) {
	var t RelayToken
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(raw) == 0 {
		return t, errors.New("push: malformed relay token")
	}
	if err := box.OpenJSON(raw, &t); err != nil {
		return t, errors.New("push: relay token does not open")
	}
	if t.Token == "" || t.Instance == "" {
		return t, errors.New("push: relay token is incomplete")
	}
	return t, nil
}

// RelaySend is the body of POST /v1/push/relay/send.
type RelaySend struct {
	RelayToken string `json:"relay_token" maxLength:"2048"`
	Push       Push   `json:"push"`
}

// RelayClient sends pushes through the relay, signed with the instance key.
type RelayClient struct {
	URL      string // relay origin (PUSH_RELAY_URL)
	Instance string // this instance's API_URL
	Key      ed25519.PrivateKey
	HTTP     *http.Client
	Now      func() time.Time
}

// Send asks the relay to deliver p to the phone behind relayToken. ErrUnregistered
// means the relay answered 410: APNs dropped the token.
func (c *RelayClient) Send(ctx context.Context, relayToken string, p Push) error {
	body, err := json.Marshal(RelaySend{RelayToken: relayToken, Push: p})
	if err != nil {
		return err
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	ts := strconv.FormatInt(now().Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/v1/push/relay/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderInstance, c.Instance)
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderSignature, Sign(c.Key, ts, body))
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("push: relay request: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch {
	case resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK:
		return nil
	case resp.StatusCode == http.StatusGone:
		return ErrUnregistered
	}
	return fmt.Errorf("push: relay status %d", resp.StatusCode)
}
