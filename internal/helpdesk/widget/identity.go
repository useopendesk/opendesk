package widget

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// A host page proves who its signed-in user is with the secret of the widget the session is for
// (contracts/teams.md "Widget identity"), in one of two ways:
//
//   - user_hash: hex HMAC-SHA256(secret, user_id). It never expires, so a widget can refuse it
//     (require_user_token).
//   - user_token: an HS256 JWT signed with the same secret, carrying user_id and exp. Its exp is
//     at most the widget's user_token_max_hours ahead, so a copied token stops working on its own.

// userTokenSkew allows for the clock of the server that signed a user_token.
const userTokenSkew = 2 * time.Minute

// errInvalidHash answers a user_hash that does not verify.
var errInvalidHash = httpx.E(http.StatusUnauthorized, "invalid_user_hash", "The user_hash does not match user_id.")

var (
	errInvalidToken  = httpx.E(http.StatusUnauthorized, "invalid_user_token", "The user_token is not signed with this widget's secret.")
	errTokenExpired  = httpx.E(http.StatusUnauthorized, "user_token_expired", "The user_token has expired. Sign a new one on your server.")
	errTokenRequired = httpx.E(http.StatusUnauthorized, "user_token_required", "This widget verifies signed-in users with a user_token.")
)

// identity is who a session request proves its visitor to be.
type identity struct {
	userID   string
	verified bool
	// expired: a user_token this widget's secret signed for userID whose exp has passed. It
	// carries on that user's live verified session on this browser, and starts nothing.
	expired bool
}

// identify checks what the request says about the signed-in user. Without user_id, user_hash
// or user_token the visitor is anonymous. user_id names the user and comes with its proof,
// user_hash or user_token; with a user_token, user_hash is not read.
func (h *handler) identify(w widgetRow, userID, userHash, userToken string, now time.Time) (identity, error) {
	userID = strings.TrimSpace(userID)
	if userToken = strings.TrimSpace(userToken); userToken != "" {
		return h.verifyToken(w, userID, userToken, now)
	}
	if userID == "" && userHash == "" {
		return identity{}, nil
	}
	if w.Settings.RequireUserToken {
		return identity{}, errTokenRequired
	}
	if err := h.verifyUser(w, userID, userHash); err != nil {
		return identity{}, err
	}
	return identity{userID: userID, verified: true}, nil
}

// verifyUser checks user_hash with the identity secret of the widget the session is for.
func (h *handler) verifyUser(w widgetRow, userID, userHash string) error {
	if userID == "" || userHash == "" {
		return httpx.Validation("user_id and user_hash must be sent together")
	}
	secret, err := h.widgetSecret(w)
	if err != nil {
		return err
	}
	if secret == nil || !crypto.EqualHex(crypto.HMACHex(secret, userID), strings.ToLower(strings.TrimSpace(userHash))) {
		return errInvalidHash
	}
	return nil
}

// verifyToken checks a user_token with the identity secret of the widget the session is for.
// The user_id sent beside it must be the one it carries.
func (h *handler) verifyToken(w widgetRow, userID, raw string, now time.Time) (identity, error) {
	if userID == "" {
		return identity{}, httpx.Validation("user_id and user_token must be sent together")
	}
	secret, err := h.widgetSecret(w)
	if err != nil {
		return identity{}, err
	}
	if secret == nil {
		return identity{}, errInvalidToken
	}
	maxAge := time.Duration(w.Settings.UserTokenMaxHours) * time.Hour
	id, expired, err := parseUserToken(secret, raw, now, maxAge)
	if err != nil {
		return identity{}, err
	}
	if userID != id {
		return identity{}, httpx.E(http.StatusUnauthorized, "invalid_user_token", "The user_id does not match the user_token.")
	}
	return identity{userID: id, verified: true, expired: expired}, nil
}

// widgetSecret opens the widget's identity secret; nil when it has none, and then nothing
// verifies.
func (h *handler) widgetSecret(w widgetRow) ([]byte, error) {
	if len(w.SecretEnc) == 0 {
		return nil, nil
	}
	return h.d.Box.Open(w.SecretEnc)
}

// userClaims are the claims of a user_token. user_id may be a string or an integer.
type userClaims struct {
	UserID json.RawMessage `json:"user_id"`
	jwt.RegisteredClaims
}

// parseUserToken checks a user_token's signature and claims at now, and answers the user id it
// carries and whether its exp has passed. Its exp must be at most maxAge ahead; an nbf or iat
// after now refuses it.
func parseUserToken(secret []byte, raw string, now time.Time, maxAge time.Duration) (string, bool, error) {
	var c userClaims
	// The claims are checked below, against now, so an expired token is told apart from a
	// forged one: only a token this secret signed is ever reported as expired.
	tok, err := jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithoutClaimsValidation())
	if err != nil || !tok.Valid {
		return "", false, errInvalidToken
	}
	id, ok := claimUserID(c.UserID)
	if !ok {
		return "", false, httpx.E(http.StatusUnauthorized, "invalid_user_token", "The user_token needs a user_id of up to 200 characters.")
	}
	if c.ExpiresAt == nil {
		return "", false, httpx.E(http.StatusUnauthorized, "invalid_user_token", "The user_token needs an exp.")
	}
	early := now.Add(userTokenSkew)
	if (c.NotBefore != nil && c.NotBefore.After(early)) || (c.IssuedAt != nil && c.IssuedAt.After(early)) {
		return "", false, httpx.E(http.StatusUnauthorized, "invalid_user_token", "The user_token is not valid yet.")
	}
	if c.ExpiresAt.After(now.Add(maxAge + userTokenSkew)) {
		return "", false, httpx.E(http.StatusUnauthorized, "invalid_user_token",
			fmt.Sprintf("The user_token's exp must be at most %d hours ahead.", int(maxAge/time.Hour)))
	}
	return id, !now.Before(c.ExpiresAt.Add(userTokenSkew)), nil
}

// claimUserID reads the user_id claim: a string, or an integer as its decimal digits, of 1 to
// 200 characters.
func claimUserID(raw json.RawMessage) (string, bool) {
	var v any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if len(raw) == 0 || d.Decode(&v) != nil {
		return "", false
	}
	var id string
	switch x := v.(type) {
	case string:
		id = strings.TrimSpace(x)
	case json.Number:
		if _, err := x.Int64(); err != nil {
			return "", false
		}
		id = x.String()
	default:
		return "", false
	}
	return id, id != "" && len(id) <= 200
}
