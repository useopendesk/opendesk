package email

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/useopendesk/opendesk/internal/audit"
	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/displayname"
	"github.com/useopendesk/opendesk/internal/email/mailhost"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/ids"
	"github.com/useopendesk/opendesk/internal/plan"
)

// oauthCookie binds a mailbox OAuth flow to the browser that started it, so a
// provider URL handed to someone else cannot connect their mailbox to this workspace.
const (
	oauthCookie     = "od_mailbox_oauth"
	oauthCookiePath = "/v1/mailboxes/oauth"
	oauthFlow       = "mailbox"
)

type oauthStartInput struct {
	Provider string `path:"provider" enum:"google,microsoft"`
	Body     struct {
		MailboxID string `json:"mailbox_id,omitempty" doc:"Reconnect this mailbox instead of adding one"`
		Name      string `json:"name,omitempty" maxLength:"64"`
		Email     string `json:"email,omitempty" maxLength:"254" doc:"Login hint for the provider"`
	}
}

type oauthStartOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      struct {
		URL string `json:"url"`
	}
}

func (s *service) registerOAuthRoutes(api huma.API, r chi.Router) {
	huma.Register(api, huma.Operation{OperationID: "start-mailbox-oauth", Method: http.MethodPost,
		Path: "/v1/mailboxes/oauth/{provider}/start", Summary: "Start connecting a Google or Microsoft mailbox",
		Description: "Returns the provider URL to open in the same browser. The call sets a short-lived cookie, so send it with credentials.",
		Tags:        []string{"Mailboxes"}},
		func(ctx context.Context, in *oauthStartInput) (*oauthStartOutput, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, "")
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if p.Kind != auth.KindUser {
				return nil, httpx.Forbidden()
			}
			prov := s.providers[in.Provider]
			if !prov.configured() {
				return nil, httpx.E(422, "provider_not_configured", providerLabel(in.Provider)+" sign-in is not configured on this instance.")
			}
			b := in.Body
			data := map[string]string{"flow": oauthFlow, "provider": in.Provider, "wid": p.WorkspaceID.String()}
			if b.MailboxID != "" {
				id, ok := ids.Parse(b.MailboxID)
				if !ok {
					return nil, httpx.NotFound("Mailbox")
				}
				m, err := loadMailbox(ctx, s.d.DB.Pool, p.WorkspaceID, id)
				if err != nil {
					return nil, wrapMailboxErr(err)
				}
				// Reconnecting a mailbox that already uses this sign-in is always allowed;
				// moving one onto it is a new connection.
				if m.InboundKind != in.Provider && !s.oauthConnect(in.Provider) {
					return nil, oauthDisabled(in.Provider)
				}
				data["mailbox_id"] = id.String()
				if b.Email == "" {
					b.Email = m.Email
				}
			} else {
				if !s.oauthConnect(in.Provider) {
					return nil, oauthDisabled(in.Provider)
				}
				if err := s.d.Plan.CheckMailbox(ctx, s.d.DB.Pool, p.WorkspaceID); err != nil {
					return nil, httpx.Wrap(err)
				}
				name := b.Name
				if strings.TrimSpace(name) == "" {
					name = providerLabel(in.Provider) + " mailbox"
				}
				if name, err = displayname.Validate("Name", name, displayname.Workspace, false); err != nil {
					return nil, httpx.Wrap(err)
				}
				data["name"] = name
			}
			verifier := oauth2.GenerateVerifier()
			if data["verifier"], err = s.sealVerifier(verifier); err != nil {
				return nil, httpx.Wrap(err)
			}
			nonce := crypto.RandomToken(32)
			data["nonce"] = hex.EncodeToString(crypto.HashToken(nonce))
			state, _, err := s.d.Tokens.Sign(auth.Claims{Purpose: auth.PurposeOAuthState, Data: data}, p.UserID.String(), auth.OAuthStateTTL)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			hint := strings.TrimSpace(b.Email)
			if !validAddress(strings.ToLower(hint)) {
				hint = ""
			}
			out := &oauthStartOutput{SetCookie: s.stateCookie(nonce, int(auth.OAuthStateTTL.Seconds()))}
			out.Body.URL = s.authCodeURL(prov, state, verifier, hint)
			return out, nil
		})

	r.Get("/v1/mailboxes/oauth/{provider}/callback", s.oauthCallback)
}

// oauthDisabled answers a new connection with a sign-in this instance does not offer
// for mailboxes (GOOGLE_MAILBOX_OAUTH, MICROSOFT_MAILBOX_OAUTH).
func oauthDisabled(provider string) error {
	return httpx.E(http.StatusForbidden, "mailbox_oauth_disabled", "Connecting a new mailbox with "+providerLabel(provider)+
		" sign-in is not available on this instance. Connect it with an app password over IMAP and SMTP, or forward its mail into opendesk.")
}

func (s *service) stateCookie(value string, maxAge int) http.Cookie {
	return http.Cookie{Name: oauthCookie, Value: value, Path: oauthCookiePath, Domain: s.d.Cfg.CookieDomain,
		MaxAge: maxAge, HttpOnly: true, Secure: s.d.Cfg.CookieSecure, SameSite: http.SameSiteLaxMode}
}

func (s *service) settingsURL(q url.Values) string {
	return strings.TrimRight(s.d.Cfg.AppURL, "/") + "/settings/mailboxes?" + q.Encode()
}

// oauthCallback finishes the provider flow and redirects back to the app.
func (s *service) oauthCallback(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	fail := func(code string) {
		http.Redirect(w, r, s.settingsURL(url.Values{"error": {code}}), http.StatusFound)
	}
	clear := s.stateCookie("", -1)
	http.SetCookie(w, &clear)

	prov := s.providers[provider]
	if !prov.configured() {
		fail("provider_not_configured")
		return
	}
	q := r.URL.Query()
	claims, err := s.d.Tokens.Verify(q.Get("state"), auth.PurposeOAuthState)
	if err != nil || claims.Data["flow"] != oauthFlow || claims.Data["provider"] != provider {
		fail("oauth_state")
		return
	}
	cookie, err := r.Cookie(oauthCookie)
	if err != nil || cookie.Value == "" ||
		subtle.ConstantTimeCompare([]byte(hex.EncodeToString(crypto.HashToken(cookie.Value))), []byte(claims.Data["nonce"])) != 1 {
		fail("oauth_state")
		return
	}
	if e := q.Get("error"); e != "" {
		fail("access_denied")
		return
	}
	uid, err1 := uuid.Parse(claims.Subject)
	wid, err2 := uuid.Parse(claims.Data["wid"])
	if err1 != nil || err2 != nil {
		fail("oauth_state")
		return
	}
	ctx := r.Context()
	var role string
	if err := s.d.DB.Pool.QueryRow(ctx, `SELECT role FROM members WHERE workspace_id = $1 AND user_id = $2`, wid, uid).Scan(&role); err != nil ||
		!auth.Role(role).AtLeast(auth.RoleAdmin) {
		fail("forbidden")
		return
	}
	verifier, err := s.openVerifier(claims.Data["verifier"])
	if err != nil {
		fail("oauth_state")
		return
	}
	code := q.Get("code")
	if code == "" || len(code) > 4096 {
		fail("provider_error")
		return
	}
	creds, email, username, err := s.completeOAuth(ctx, prov, code, verifier)
	if err != nil {
		s.log.Warn("mailbox oauth exchange failed", "provider", provider, "workspace_id", wid, "err", safeError(err))
		fail("provider_error")
		return
	}
	p := &auth.Principal{Kind: auth.KindUser, UserID: uid, WorkspaceID: wid, Role: auth.Role(role),
		IP: httpx.ClientIP(s.d.Cfg.TrustProxy)(r), UserAgent: truncate(r.UserAgent(), 400)}
	var mailboxID *uuid.UUID
	if v := claims.Data["mailbox_id"]; v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			fail("oauth_state")
			return
		}
		mailboxID = &id
	}
	id, err := s.saveOAuthMailbox(ctx, p, provider, mailboxID, claims.Data["name"], email, username, creds)
	if err != nil {
		switch {
		case db.IsUniqueViolation(err, "mailboxes_workspace_id_email_key"):
			fail("mailbox_conflict")
		case db.IsNotFound(err):
			fail("mailbox_not_found")
		case errors.Is(err, plan.ErrLimit):
			fail("plan_limit")
		default:
			s.log.Error("save oauth mailbox", "workspace_id", wid, "err", err)
			fail("internal")
		}
		return
	}
	http.Redirect(w, r, s.settingsURL(url.Values{"connected": {id.String()}}), http.StatusFound)
}

// completeOAuth redeems the code and reads the account identity. For Microsoft the
// code yields the IMAP (outlook.office.com) token; the refresh token is then exchanged
// for a Graph token, which also proves the send consent was granted.
func (s *service) completeOAuth(ctx context.Context, p *oauthProvider, code, verifier string) (credentials, string, string, error) {
	tr, err := s.exchangeCode(ctx, p, code, verifier)
	if err != nil {
		return credentials{}, "", "", err
	}
	if tr.RefreshToken == "" {
		return credentials{}, "", "", errors.New("the provider did not grant offline access")
	}
	now := timeNow()
	c := credentials{RefreshToken: tr.RefreshToken, AccessToken: tr.AccessToken, Expiry: expiryOf(tr, now)}
	identityToken := tr.AccessToken
	if p.name == KindMicrosoft {
		g, err := s.refresh(ctx, p, c.RefreshToken, p.graphScope)
		if err != nil {
			return credentials{}, "", "", err
		}
		if g.RefreshToken != "" {
			c.RefreshToken = g.RefreshToken
		}
		c.GraphAccessToken, c.GraphExpiry = g.AccessToken, expiryOf(g, now)
		identityToken = g.AccessToken
	}
	email, username, err := s.oauthIdentity(ctx, p, identityToken)
	if err != nil {
		return credentials{}, "", "", err
	}
	return c, email, username, nil
}

// saveOAuthMailbox creates the mailbox, or updates the one being reconnected (or the
// one with the same address), and returns its id.
func (s *service) saveOAuthMailbox(ctx context.Context, p *auth.Principal, provider string, mailboxID *uuid.UUID,
	name, email, username string, creds credentials) (uuid.UUID, error) {
	prov := s.providers[provider]
	cfg := MailboxConfig{IMAPHost: prov.imapHost, IMAPPort: 993, IMAPTLS: TLSImplicit, IMAPUsername: username, OAuthEmail: email,
		MailHost: string(mailhost.ForMailbox("", provider, "", email))}
	sealed, err := s.sealCreds(creds)
	if err != nil {
		return uuid.Nil, err
	}
	wid := p.WorkspaceID
	var id uuid.UUID
	err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		var existing *mailboxRow
		var err error
		if mailboxID != nil {
			existing, err = scanMailbox(tx.QueryRow(ctx, `SELECT `+mailboxColumns+` FROM mailboxes WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, wid, *mailboxID))
			if err != nil {
				return err
			}
		} else {
			existing, err = scanMailbox(tx.QueryRow(ctx, `SELECT `+mailboxColumns+` FROM mailboxes WHERE workspace_id = $1 AND email = $2 FOR UPDATE`, wid, email))
			if err != nil && !db.IsNotFound(err) {
				return err
			}
		}
		typ := "mailbox.updated"
		if existing != nil {
			id = existing.ID
			// A reconnect keeps the mailbox's own address unless it was the old account's.
			addr := existing.Email
			if existing.Config.OAuthEmail == "" || strings.EqualFold(existing.Config.OAuthEmail, existing.Email) {
				addr = email
			}
			resetCursor := !strings.EqualFold(existing.Config.IMAPUsername, username) || existing.InboundKind != provider
			_, err = tx.Exec(ctx, `UPDATE mailboxes SET email = $3, inbound_kind = $4, outbound_kind = $4, config = $5,
				credentials_enc = $6, status = 'active', last_error = '', sync_failures = 0, next_sync_at = NULL,
				imap_uid_validity = CASE WHEN $7 THEN 0 ELSE imap_uid_validity END,
				imap_last_uid = CASE WHEN $7 THEN 0 ELSE imap_last_uid END, updated_at = now()
				WHERE workspace_id = $1 AND id = $2`, wid, id, addr, provider, cfg, sealed, resetCursor)
			if err != nil {
				return err
			}
		} else {
			typ = "mailbox.created"
			if err := s.d.Plan.CheckMailbox(ctx, tx, wid); err != nil {
				return err
			}
			var slug string
			if err := tx.QueryRow(ctx, `SELECT slug FROM workspaces WHERE id = $1`, wid).Scan(&slug); err != nil {
				return err
			}
			m := &mailboxRow{WorkspaceID: wid}
			m.ID = ids.New()
			m.Name, m.Email, m.InboundKind, m.OutboundKind, m.Config, m.Status = name, email, provider, provider, cfg, "active"
			if m.Name == "" {
				m.Name = providerLabel(provider) + " mailbox"
			}
			// A connected mailbox starts in the default brand.
			if err := checkAddressing(ctx, tx, m); err != nil {
				return err
			}
			if err := s.insertMailbox(ctx, tx, m, slug, sealed); err != nil {
				return err
			}
			id = m.ID
		}
		if err := s.publishMailbox(ctx, tx, wid, id, typ); err != nil {
			return err
		}
		if err := audit.Log(ctx, tx, p, wid, "mailbox.connected", "mailbox", &id, map[string]any{
			"provider": provider, "email": email, "reconnect": existing != nil}); err != nil {
			return err
		}
		m, err := loadMailbox(ctx, tx, wid, id)
		if err != nil {
			return err
		}
		return s.enqueueSync(ctx, tx, m)
	})
	return id, err
}
