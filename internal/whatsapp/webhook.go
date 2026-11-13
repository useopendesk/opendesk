package whatsapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/whatsapp/provider"
)

const (
	maxWebhookBody = 1 << 20
	// webhookPerMinute bounds what one connection's URL takes in once verified.
	webhookPerMinute = 600
	// badHooksPerMinute bounds the requests one address sends that name no connection or
	// fail verification, which a provider never does.
	badHooksPerMinute = 30
	// eventRetention is how long an applied event's id is kept to recognize the provider's
	// retries (the prune job).
	eventRetention = 7 * 24 * time.Hour
)

// webhook receives a provider's customer messages and delivery statuses. The connection is
// found by its opaque id and the request is verified with the connection's credentials
// before anything is read from it. Nothing in the payload decides which workspace or ticket
// it reaches. A verified event is stored (normalized) and queued in one transaction, then the
// provider gets its 200.
func (s *Service) webhook(w http.ResponseWriter, r *http.Request) {
	bad := "whatsapp-hook-bad:" + httpx.ClientIP(s.d.Cfg.TrustProxy, s.d.Cfg.ProxyHops)(r)
	if s.d.Limiter.Over(bad, badHooksPerMinute, time.Minute) {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	refuse := func(status int) {
		if !s.d.Limiter.Allow(bad, badHooksPerMinute, time.Minute) {
			status = http.StatusTooManyRequests
		}
		w.WriteHeader(status)
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		refuse(http.StatusNotFound)
		return
	}
	c, err := scanConnection(s.d.DB.Pool.QueryRow(r.Context(), connectionSelect+` WHERE id = $1`, id))
	if err != nil && !db.IsNotFound(err) {
		s.d.Log.Error("whatsapp webhook connection", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	a, ok := s.adapters[c.Provider]
	if err != nil || !ok || c.Provider != chi.URLParam(r, "provider") {
		refuse(http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		refuse(http.StatusRequestEntityTooLarge)
		return
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		refuse(http.StatusBadRequest)
		return
	}
	creds, err := s.credentials(c)
	if err != nil {
		s.d.Log.Error("whatsapp webhook credentials", "connection_id", c.ID, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	status := strings.HasSuffix(r.URL.Path, "/status")
	signed := s.webhookURL(c)
	if status {
		signed = s.statusURL(c)
	}
	if err := a.Verify(provider.Request{URL: signed, Header: r.Header, Form: form}, creds); err != nil {
		refuse(http.StatusUnauthorized)
		return
	}
	if !s.d.Limiter.Allow("whatsapp-hook:"+c.ID.String(), webhookPerMinute, time.Minute) {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	var ev provider.Event
	if status {
		ev, err = a.ParseStatus(form)
	} else {
		ev, err = a.ParseMessage(form)
	}
	if errors.Is(err, provider.ErrIgnored) {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err != nil {
		// Verified but not understood: acknowledged, so the provider keeps the webhook on.
		s.d.Log.Warn("whatsapp event not understood", "provider", c.Provider, "connection_id", c.ID, "err", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := s.persist(r, c, ev); err != nil {
		s.d.Log.Error("whatsapp event not stored", "connection_id", c.ID, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Service) persist(r *http.Request, c WhatsAppConnection, ev provider.Event) error {
	ctx := r.Context()
	raw, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO whatsapp_inbound_events (connection_id, external_event_id, workspace_id, payload)
			VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, c.ID, ev.ID, c.WorkspaceID, raw)
		if err != nil || tag.RowsAffected() == 0 {
			return err // a retry of an event already stored
		}
		if _, err := tx.Exec(ctx, `UPDATE whatsapp_connections SET last_event_at = now() WHERE id = $1`, c.ID); err != nil {
			return err
		}
		// The webhook works once its first event came in.
		if c.HealthStatus == "action_required" {
			now := time.Now().UTC()
			c.LastEventAt = &now
			if _, err := s.storeHealth(ctx, tx, c, "healthy", HealthReceiving); err != nil {
				return err
			}
		}
		_, err = s.d.Jobs.InsertTx(ctx, tx, jobs.WhatsAppInboundArgs{WorkspaceID: c.WorkspaceID, ConnectionID: c.ID, ExternalEventID: ev.ID},
			&river.InsertOpts{MaxAttempts: 10})
		return err
	})
}
