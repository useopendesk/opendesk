package core

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/appattest"
	"github.com/useopendesk/opendesk/internal/captcha"
	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// App Attest (contracts/native.md): the iPhone app's stand-in for Turnstile. A key is
// attested by Apple once per install; each protected request then carries an assertion
// over a fresh single-use challenge, made for that request's action.
const (
	attestChallengeTTL   = 5 * time.Minute
	attestChallengePerIP = 60 // per hour
	attestKeysPerIP      = 20 // per hour
	// attestKeyBudget bounds the protected requests one install can make per hour, on top
	// of the per-IP and per-account limits.
	attestKeyBudget = 30
)

// appAttestProof is what a native request carries in place of turnstile_token.
type appAttestProof struct {
	KeyID     string `json:"key_id" maxLength:"100" doc:"The App Attest key id, base64"`
	Assertion string `json:"assertion" maxLength:"8192" doc:"The assertion object, base64 CBOR"`
	Challenge string `json:"challenge" maxLength:"100" doc:"From POST /v1/auth/app-attest/challenge"`
}

func errInvalidAttestation() *httpx.Error {
	return httpx.E(400, "invalid_attestation", "This device could not be verified.")
}

func (s *svc) attestEnabled() bool { return len(s.d.Cfg.AppAttestAppIDs) > 0 }

func (s *svc) attestPolicy() appattest.Policy {
	return appattest.Policy{AppIDs: s.d.Cfg.AppAttestAppIDs, AllowDevelopment: s.d.Cfg.AppAttestDevelopment}
}

// humanCheck is the human check on login, registration and password forgot: an App
// Attest proof for a native request that sends one (when App Attest is on), Turnstile
// otherwise. Either failure is the same captcha_failed.
func (s *svc) humanCheck(ctx context.Context, ri reqInfo, action, turnstile string, proof *appAttestProof) error {
	if proof != nil && ri.Native && s.attestEnabled() {
		return s.verifyAppAttest(ctx, action, proof)
	}
	return s.d.Captcha.Verify(ctx, turnstile, ri.IP)
}

// decodeB64 accepts standard or URL-safe base64, padded or not.
func decodeB64(s string) ([]byte, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	if strings.ContainsAny(s, "-_") {
		return base64.RawURLEncoding.DecodeString(s)
	}
	return base64.RawStdEncoding.DecodeString(s)
}

// canonicalKeyID returns the stored form of a key id (standard base64 of 32 bytes).
func canonicalKeyID(s string) ([]byte, string, bool) {
	raw, err := decodeB64(s)
	if err != nil || len(raw) != sha256.Size {
		return nil, "", false
	}
	return raw, base64.StdEncoding.EncodeToString(raw), true
}

// consumeChallenge spends a challenge and reports whether it was live. A challenge is
// gone after its first use, whatever the outcome.
func consumeChallenge(ctx context.Context, q db.Querier, challenge string) (bool, error) {
	if challenge == "" || len(challenge) > 100 {
		return false, nil
	}
	var expires time.Time
	err := q.QueryRow(ctx, `DELETE FROM app_attest_challenges WHERE challenge_hash = $1 RETURNING expires_at`,
		crypto.HashToken(challenge)).Scan(&expires)
	if db.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return time.Now().Before(expires), nil
}

// verifyAppAttest checks a proof made for action. The challenge is consumed first, so a
// refused proof spends it too; the key's counter only moves forward.
func (s *svc) verifyAppAttest(ctx context.Context, action string, proof *appAttestProof) error {
	if proof == nil || proof.KeyID == "" || proof.Assertion == "" {
		return captcha.ErrCaptcha
	}
	ok, err := consumeChallenge(ctx, s.d.DB.Pool, proof.Challenge)
	if err != nil {
		return err
	}
	_, keyID, valid := canonicalKeyID(proof.KeyID)
	if !ok || !valid {
		return captcha.ErrCaptcha
	}
	object, err := decodeB64(proof.Assertion)
	if err != nil {
		return captcha.ErrCaptcha
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		var appID, env string
		var pub []byte
		var counter int64
		err := tx.QueryRow(ctx, `SELECT app_id, public_key, counter, environment FROM app_attest_keys WHERE key_id = $1 FOR UPDATE`,
			keyID).Scan(&appID, &pub, &counter, &env)
		if db.IsNotFound(err) {
			return captcha.ErrCaptcha
		}
		if err != nil {
			return err
		}
		if !s.d.Limiter.Allow("appattest:key:"+keyID, attestKeyBudget, time.Hour) {
			return httpx.RateLimited()
		}
		// A key outlives the policy it was attested under; it answers to the current one.
		policy := s.attestPolicy()
		if !slices.Contains(policy.AppIDs, appID) || (env == string(appattest.Development) && !policy.AllowDevelopment) {
			return captcha.ErrCaptcha
		}
		next, err := appattest.VerifyAssertion(object, appattest.ClientData(action, proof.Challenge), pub, appID,
			uint32(counter), policy.AllowDevelopment)
		if err != nil {
			if errors.Is(err, appattest.ErrInvalid) {
				s.d.Log.Info("app attest assertion refused", "action", action, "reason", err.Error())
				return captcha.ErrCaptcha
			}
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE app_attest_keys SET counter = $2, last_used_at = now() WHERE key_id = $1 AND counter < $2`,
			keyID, int64(next))
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return captcha.ErrCaptcha
		}
		return nil
	})
}

func (s *svc) registerAppAttest(api huma.API) {
	tags := []string{"Auth"}
	off := func() *httpx.Error {
		return httpx.E(404, "app_attest_off", "App Attest is not enabled on this server.")
	}

	register(s, api, huma.Operation{OperationID: "auth-app-attest-challenge", Method: http.MethodPost, Path: "/v1/auth/app-attest/challenge",
		Summary: "Get a single-use App Attest challenge", Tags: tags, Security: []map[string][]string{}},
		func(ctx context.Context, _ *struct {
			Body *struct{}
		}) (*struct {
			CacheControl string `header:"Cache-Control"`
			Body         struct {
				Challenge string `json:"challenge" doc:"32 random bytes, base64url; single use, valid 5 minutes"`
			}
		}, error) {
			if !s.attestEnabled() {
				return nil, off()
			}
			ri := requestInfo(ctx)
			if err := s.limit("appattest:challenge:"+ri.IP, attestChallengePerIP, time.Hour); err != nil {
				return nil, err
			}
			challenge := crypto.RandomToken(32)
			if _, err := s.d.DB.Pool.Exec(ctx, `INSERT INTO app_attest_challenges (challenge_hash, expires_at) VALUES ($1, $2)`,
				crypto.HashToken(challenge), time.Now().Add(attestChallengeTTL)); err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct {
				CacheControl string `header:"Cache-Control"`
				Body         struct {
					Challenge string `json:"challenge" doc:"32 random bytes, base64url; single use, valid 5 minutes"`
				}
			}{CacheControl: "no-store"}
			out.Body.Challenge = challenge
			return out, nil
		})

	register(s, api, huma.Operation{OperationID: "auth-app-attest-key", Method: http.MethodPost, Path: "/v1/auth/app-attest/keys",
		Summary: "Register an attested App Attest key", Tags: tags, Security: []map[string][]string{}, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			Body struct {
				KeyID       string `json:"key_id" maxLength:"100" doc:"base64"`
				Attestation string `json:"attestation" maxLength:"16384" doc:"The attestation object, base64 CBOR"`
				Challenge   string `json:"challenge" maxLength:"100"`
			}
		}) (*struct{}, error) {
			if !s.attestEnabled() {
				return nil, off()
			}
			ri := requestInfo(ctx)
			if err := s.limit("appattest:keys:"+ri.IP, attestKeysPerIP, time.Hour); err != nil {
				return nil, err
			}
			ok, err := consumeChallenge(ctx, s.d.DB.Pool, in.Body.Challenge)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			rawID, keyID, valid := canonicalKeyID(in.Body.KeyID)
			object, derr := decodeB64(in.Body.Attestation)
			if !ok || !valid || derr != nil {
				return nil, errInvalidAttestation()
			}
			cdh := sha256.Sum256([]byte(in.Body.Challenge))
			key, err := appattest.VerifyAttestation(object, rawID, cdh[:], s.attestPolicy(), time.Now())
			if err != nil {
				s.d.Log.Info("app attest attestation refused", "reason", err.Error())
				return nil, errInvalidAttestation()
			}
			tag, err := s.d.DB.Pool.Exec(ctx, `INSERT INTO app_attest_keys (key_id, app_id, public_key, environment)
				VALUES ($1, $2, $3, $4) ON CONFLICT (key_id) DO NOTHING`, keyID, key.AppID, key.PublicKey, string(key.Environment))
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			// A key is attested once; attesting the same key again is a replay.
			if tag.RowsAffected() == 0 {
				return nil, errInvalidAttestation()
			}
			// Registration happens before sign-in, so there is no account or workspace to
			// audit it in; the log line carries no key material.
			s.d.Log.Info("app attest key registered", "app_id", key.AppID, "environment", string(key.Environment), "ip", ri.IP)
			return nil, nil
		})
}
