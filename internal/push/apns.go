// Package push sends notifications to the iPhone app (contracts/native.md "Push"):
// the APNs provider (token auth over HTTP/2), the payload every push carries, and the
// signed relay protocol self-hosted instances use to reach phones through the relay.
package push

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrUnregistered means APNs no longer accepts the device token (app removed, token
// rotated, token for another app). The device is deleted.
var ErrUnregistered = errors.New("push: device token unregistered")

const (
	hostProduction = "https://api.push.apple.com"
	hostSandbox    = "https://api.sandbox.push.apple.com"
	// providerTokenTTL renews the provider JWT well inside Apple's 20..60 minute window.
	providerTokenTTL = 50 * time.Minute
)

// Environments a device token belongs to.
const (
	EnvProduction = "production"
	EnvSandbox    = "sandbox"
)

var deviceTokenRe = regexp.MustCompile(`^[0-9a-f]{64,200}$`)

// NormalizeDeviceToken lowercases a hex APNs token and reports whether it is well formed.
func NormalizeDeviceToken(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	return s, deviceTokenRe.MatchString(s)
}

// APNs is a token-auth APNs provider. The provider JWT is cached and renewed every 50
// minutes; requests go over HTTP/2 to the production or sandbox host by the device's
// environment.
type APNs struct {
	key    *ecdsa.PrivateKey
	keyID  string
	teamID string
	topic  string

	// HTTP, ProductionURL and SandboxURL are replaceable for tests.
	HTTP          *http.Client
	ProductionURL string
	SandboxURL    string

	mu       sync.Mutex
	bearer   string
	bearerAt time.Time
}

// NewAPNs parses the .p8 key (PEM, PEM with escaped newlines, or base64 of the PEM).
// Errors never echo the key.
func NewAPNs(key, keyID, teamID, topic string) (*APNs, error) {
	k, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if keyID == "" || teamID == "" || topic == "" {
		return nil, errors.New("push: APNS_KEY_ID, APNS_TEAM_ID and APNS_TOPIC are required")
	}
	return &APNs{key: k, keyID: keyID, teamID: teamID, topic: topic,
		HTTP: &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{ForceAttemptHTTP2: true,
			TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 5 * time.Minute, MaxIdleConnsPerHost: 4}},
		ProductionURL: hostProduction, SandboxURL: hostSandbox}, nil
}

// ParseKey reads an Apple .p8 signing key in whichever form it was pasted.
func ParseKey(value string) (*ecdsa.PrivateKey, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("push: APNS_KEY is empty")
	}
	var pemBytes []byte
	if strings.Contains(value, "-----BEGIN") {
		pemBytes = []byte(strings.ReplaceAll(value, `\n`, "\n"))
	} else {
		decoded, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(value), ""))
		if err != nil {
			return nil, errors.New("push: APNS_KEY is neither PEM nor base64-encoded PEM")
		}
		pemBytes = decoded
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("push: APNS_KEY is not PEM encoded")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("push: APNS_KEY is not a PKCS#8 private key")
	}
	k, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || k.Curve != elliptic.P256() {
		return nil, errors.New("push: APNS_KEY is not a P-256 key (an Apple .p8 key)")
	}
	return k, nil
}

func (c *APNs) providerToken(renew bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !renew && c.bearer != "" && time.Since(c.bearerAt) < providerTokenTTL {
		return c.bearer, nil
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{"iss": c.teamID, "iat": now.Unix()})
	tok.Header["kid"] = c.keyID
	signed, err := tok.SignedString(c.key)
	if err != nil {
		return "", fmt.Errorf("push: sign provider token: %w", err)
	}
	c.bearer, c.bearerAt = signed, now
	return signed, nil
}

// Send delivers one push to a device token. ErrUnregistered means the token is gone.
func (c *APNs) Send(ctx context.Context, deviceToken, environment string, p Push, payload []byte) error {
	tok, ok := NormalizeDeviceToken(deviceToken)
	if !ok {
		return ErrUnregistered
	}
	host := c.ProductionURL
	if environment == EnvSandbox {
		host = c.SandboxURL
	}
	for attempt := 0; ; attempt++ {
		bearer, err := c.providerToken(attempt > 0)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/3/device/"+tok, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("authorization", "bearer "+bearer)
		req.Header.Set("apns-topic", c.topic)
		req.Header.Set("apns-push-type", "alert")
		req.Header.Set("apns-priority", strconv.Itoa(p.Priority()))
		if id := p.CollapseID(); id != "" {
			req.Header.Set("apns-collapse-id", id)
		}
		req.Header.Set("content-type", "application/json")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			// The request URL names the device token; the error keeps only the cause.
			var ue *url.Error
			if errors.As(err, &ue) {
				err = ue.Err
			}
			return fmt.Errorf("push: apns request: %w", err)
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
		var apiErr struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(raw, &apiErr)
		switch {
		case resp.StatusCode == http.StatusGone, apiErr.Reason == "Unregistered", apiErr.Reason == "BadDeviceToken",
			apiErr.Reason == "DeviceTokenNotForTopic":
			return ErrUnregistered
		case resp.StatusCode == http.StatusForbidden && apiErr.Reason == "ExpiredProviderToken" && attempt == 0:
			continue
		}
		return fmt.Errorf("push: apns status %d %s", resp.StatusCode, apiErr.Reason)
	}
}
