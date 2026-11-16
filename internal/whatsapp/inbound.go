package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/displayname"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/helpdesk/routing"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/products"
	"github.com/useopendesk/opendesk/internal/whatsapp/provider"
)

const (
	// maxMediaBytes is the largest file taken from a customer's message.
	maxMediaBytes = 16 << 20
	// statusWait is how long a delivery status waits for the message it is about: the
	// provider can report before the send job stored the provider's id.
	statusWait = 15 * time.Minute
	subjectMax = 80
)

type inboundWorker struct {
	river.WorkerDefaults[jobs.WhatsAppInboundArgs]
	s *Service
}

// errUnknownMessage is a delivery status for a message not stored (yet).
var errUnknownMessage = errors.New("delivery status for a message not stored yet")

func (w *inboundWorker) Work(ctx context.Context, job *river.Job[jobs.WhatsAppInboundArgs]) error {
	err := w.s.ProcessInbound(ctx, job.Args)
	if errors.Is(err, products.ErrHeld) {
		return products.SnoozeHeld(err)
	}
	if errors.Is(err, errUnknownMessage) {
		if time.Since(job.CreatedAt) > statusWait {
			return river.JobCancel(err)
		}
		return river.JobSnooze(20 * time.Second)
	}
	var limited *provider.RateLimited
	if errors.As(err, &limited) {
		if time.Since(job.CreatedAt) > 24*time.Hour {
			return river.JobCancel(errors.New("whatsapp provider retry deadline exceeded"))
		}
		return river.JobSnooze(limited.RetryAfter)
	}
	return err
}

// ProcessInbound applies one stored provider event, once: the event row is locked and
// marked processed in the same transaction as everything it changed.
func (s *Service) ProcessInbound(ctx context.Context, a jobs.WhatsAppInboundArgs) error {
	var raw []byte
	var done *time.Time
	err := s.d.DB.Pool.QueryRow(ctx, `SELECT payload, processed_at FROM whatsapp_inbound_events WHERE connection_id = $1 AND external_event_id = $2`,
		a.ConnectionID, a.ExternalEventID).Scan(&raw, &done)
	if db.IsNotFound(err) || err == nil && done != nil {
		return nil // the connection is gone, or the event was applied
	}
	if err != nil {
		return err
	}
	var ev provider.Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		return fmt.Errorf("whatsapp event payload: %w", err)
	}
	c, err := s.connection(ctx, s.d.DB.Pool, a.WorkspaceID, a.ConnectionID)
	if db.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	// Files come from the provider before the transaction, which holds the ticket's lock.
	var attachments []uuid.UUID
	if ev.Kind == provider.KindMessage && len(ev.Media) > 0 {
		var known bool
		if err := s.d.DB.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM whatsapp_messages WHERE connection_id = $1 AND external_id = $2)`,
			c.ID, ev.ExternalID).Scan(&known); err != nil {
			return err
		}
		if !known {
			if attachments, err = s.fetchMedia(ctx, c, ev); err != nil {
				return err
			}
		}
	}
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT processed_at FROM whatsapp_inbound_events WHERE connection_id = $1 AND external_event_id = $2 FOR UPDATE`,
			a.ConnectionID, a.ExternalEventID).Scan(&done)
		if db.IsNotFound(err) || err == nil && done != nil {
			return nil
		}
		if err != nil {
			return err
		}
		suspended, err := domain.WorkspaceSuspended(ctx, tx, c.WorkspaceID)
		if err != nil {
			return err
		}
		if !suspended {
			// The event stays stored and unapplied until Tickets is on.
			if err := products.Hold(ctx, tx, c.WorkspaceID); err != nil {
				return err
			}
			switch ev.Kind {
			case provider.KindMessage:
				err = s.applyMessage(ctx, tx, c, ev, attachments)
			case provider.KindStatus:
				err = s.applyStatus(ctx, tx, c, ev)
			}
			if err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE whatsapp_inbound_events SET processed_at = now() WHERE connection_id = $1 AND external_event_id = $2`,
			a.ConnectionID, a.ExternalEventID)
		return err
	})
}

// fetchMedia stores the files of a customer's message as unattached attachments. A file
// that can't be taken (refused type, too large, gone) is left out; the message still lands.
func (s *Service) fetchMedia(ctx context.Context, c WhatsAppConnection, ev provider.Event) ([]uuid.UUID, error) {
	a, ok := s.adapters[c.Provider]
	if !ok {
		return nil, nil
	}
	creds, err := s.credentials(c)
	if err != nil {
		return nil, err
	}
	var out []uuid.UUID
	for i, m := range ev.Media {
		body, err := a.Media(ctx, s.http, creds, m, maxMediaBytes+1)
		var limited *provider.RateLimited
		if errors.As(err, &limited) {
			return nil, err
		}
		if err != nil {
			s.d.Log.Info("whatsapp media not taken", "connection_id", c.ID, "err", err)
			continue
		}
		data, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil || len(data) == 0 || len(data) > maxMediaBytes {
			s.d.Log.Info("whatsapp media not taken", "connection_id", c.ID)
			continue
		}
		att, err := hd.SaveUpload(ctx, s.d.Storage, s.d.DB.Pool, hd.Upload{WorkspaceID: c.WorkspaceID,
			Filename: mediaName(m.ContentType, i), Size: int64(len(data)), Body: bytes.NewReader(data)})
		if err != nil {
			s.d.Log.Info("whatsapp media refused", "connection_id", c.ID, "err", err)
			continue
		}
		out = append(out, att.ID)
	}
	return out, nil
}

// mediaName names a file a customer sent, which WhatsApp gives no name: by its type.
func mediaName(contentType string, i int) string {
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif", "application/pdf": ".pdf",
		"audio/ogg": ".ogg", "audio/mpeg": ".mp3", "audio/amr": ".amr", "video/mp4": ".mp4", "video/3gpp": ".3gp"}[strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))]
	return fmt.Sprintf("whatsapp-%d%s", i+1, ext)
}

// textHTML is a customer's plain text as the paragraph the ticket shows.
func textHTML(s string) string {
	return "<p>" + strings.ReplaceAll(html.EscapeString(s), "\n", "<br>") + "</p>"
}

// applyMessage puts a customer's message on their ticket: their open one on WhatsApp, else
// their latest one solved within the connection's reuse_days, else a new one.
func (s *Service) applyMessage(ctx context.Context, tx pgx.Tx, c WhatsAppConnection, ev provider.Event, atts []uuid.UUID) error {
	wid := c.WorkspaceID
	var known bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM whatsapp_messages WHERE connection_id = $1 AND external_id = $2)`, c.ID, ev.ExternalID).Scan(&known); err != nil {
		return err
	}
	if known {
		return nil
	}
	contact, err := domain.ContactByPhone(ctx, tx, wid, ev.Number, ev.Name)
	if err != nil {
		return err
	}
	if contact.Blocked {
		return nil // like their email, a blocked contact's message reaches no ticket
	}
	var name string
	if err := tx.QueryRow(ctx, `SELECT name FROM contacts WHERE workspace_id = $1 AND id = $2`, wid, contact.ID).Scan(&name); err != nil {
		return err
	}
	var tid uuid.UUID
	created := false
	err = tx.QueryRow(ctx, `SELECT id FROM tickets WHERE workspace_id = $1 AND contact_id = $2 AND channel = 'whatsapp' AND deleted_at IS NULL
		AND merged_into_id IS NULL AND NOT spam
		AND (status IN ('open', 'pending', 'on_hold') OR ($3::int > 0 AND status = 'solved' AND solved_at > now() - make_interval(days => $3::int)))
		ORDER BY (status <> 'solved') DESC, CASE WHEN status = 'solved' THEN solved_at END DESC NULLS LAST, updated_at DESC LIMIT 1 FOR NO KEY UPDATE`,
		wid, contact.ID, c.ReuseDays).Scan(&tid)
	if db.IsNotFound(err) {
		subject := subjectOf(ev.Text, name, ev.Number)
		brand, err := domain.ResolveBrand(ctx, tx, wid, c.BrandID, nil, nil)
		if err != nil {
			return err
		}
		res, err := routing.Resolve(ctx, tx, routing.Input{WorkspaceID: wid, Channel: domain.ChannelWhatsApp, BrandID: brand, Subject: subject, ContactID: &contact.ID})
		if err != nil {
			return err
		}
		t, err := domain.CreateTicket(ctx, tx, domain.NewTicket{WorkspaceID: wid, Subject: subject, Channel: domain.ChannelWhatsApp,
			ContactID: &contact.ID, TeamID: res.TeamID, Routed: res.Routed, BrandID: &brand, RequiredSkillIDs: res.RequiredSkillIDs})
		if err != nil {
			return err
		}
		if rule := res.RuleID(); rule != nil {
			if err := routing.Count(ctx, tx, *rule); err != nil {
				return err
			}
		}
		tid, created = t.ID, true
	} else if err != nil {
		return fmt.Errorf("find ticket: %w", err)
	}
	trigger := ""
	if created {
		trigger = jobs.TriggerTicketCreated
	}
	cid := contact.ID
	msg, err := domain.AddMessage(ctx, tx, s.d.Jobs, domain.NewMessage{WorkspaceID: wid, TicketID: tid, Kind: "reply", Direction: "inbound",
		AuthorType: "contact", AuthorContactID: &cid, BodyHTML: textHTML(ev.Text), BodyText: ev.Text,
		FromName: displayname.Clean(name, displayname.Person), AttachmentIDs: atts, Trigger: trigger})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO whatsapp_messages (message_id, workspace_id, ticket_id, connection_id, direction, external_id, number)
		VALUES ($1, $2, $3, $4, 'inbound', $5, $6)`, msg.ID, wid, tid, c.ID, ev.ExternalID, ev.Number)
	return err
}

// subjectOf names a ticket a message opened: its first words, else who wrote.
func subjectOf(text, name, number string) string {
	first := strings.TrimSpace(strings.SplitN(strings.TrimSpace(text), "\n", 2)[0])
	if r := []rune(first); len(r) > subjectMax {
		first = strings.TrimSpace(string(r[:subjectMax-1])) + "…"
	}
	if first != "" {
		return first
	}
	who := name
	if who == "" {
		who = number
	}
	return "WhatsApp from " + who
}

// statusRank orders what happens to an outbound message: a status only moves it forward.
var statusRank = map[string]int{"none": 0, "pending": 1, provider.StatusSent: 2, provider.StatusDelivered: 3, provider.StatusRead: 4}

// failureReasons are what a message's delivery error says for the provider's codes.
var failureReasons = map[string]string{
	"63016": "Outside the 24-hour window. Send a template message instead.",
	"63003": "This number is not on WhatsApp.",
	"63024": "This number is not on WhatsApp.",
	"21610": "This customer stopped messages to this number.",
	"63049": "WhatsApp refused to send this message.",
}

func failureText(code string) string {
	if r, ok := failureReasons[code]; ok {
		return r
	}
	if code != "" {
		return "WhatsApp could not deliver this message (error " + code + ")."
	}
	return "WhatsApp could not deliver this message."
}

// applyStatus moves an outbound message to the status the provider reports, forward only:
// delivered and read can arrive before sent, or twice.
func (s *Service) applyStatus(ctx context.Context, tx pgx.Tx, c WhatsAppConnection, ev provider.Event) error {
	var mid uuid.UUID
	var current string
	err := tx.QueryRow(ctx, `SELECT w.message_id, m.delivery_status FROM whatsapp_messages w JOIN messages m ON m.id = w.message_id
		WHERE w.connection_id = $1 AND w.external_id = $2 AND w.direction = 'outbound' FOR UPDATE OF m`, c.ID, ev.ExternalID).Scan(&mid, &current)
	if db.IsNotFound(err) {
		return errUnknownMessage
	}
	if err != nil {
		return err
	}
	var next, detail string
	switch ev.Status {
	case provider.StatusFailed:
		if current == "failed" || statusRank[current] >= statusRank[provider.StatusDelivered] {
			return nil
		}
		next, detail = "failed", failureText(ev.Code)
	default:
		if statusRank[ev.Status] <= statusRank[current] {
			return nil
		}
		next = ev.Status
	}
	if _, err := tx.Exec(ctx, `UPDATE messages SET delivery_status = $3, delivery_error = $4 WHERE workspace_id = $1 AND id = $2`,
		c.WorkspaceID, mid, next, detail); err != nil {
		return err
	}
	return domain.PublishMessageUpdated(ctx, tx, c.WorkspaceID, mid)
}

type pruneWorker struct {
	river.WorkerDefaults[jobs.WhatsAppPruneArgs]
	s *Service
}

// Work removes provider events applied more than eventRetention ago, and any older than 30 days.
func (w *pruneWorker) Work(ctx context.Context, _ *river.Job[jobs.WhatsAppPruneArgs]) error {
	_, err := w.s.d.DB.Pool.Exec(ctx, `DELETE FROM whatsapp_inbound_events
		WHERE (processed_at IS NOT NULL AND processed_at < now() - make_interval(secs => $1)) OR received_at < now() - interval '30 days'`,
		eventRetention.Seconds())
	return err
}
