package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/jobs"
)

// sendWorker delivers one outbound WhatsApp reply.
type sendWorker struct {
	river.WorkerDefaults[jobs.WhatsAppSendArgs]
	s *Service
}

func (w *sendWorker) Work(ctx context.Context, job *river.Job[jobs.WhatsAppSendArgs]) error {
	s := w.s
	var m struct {
		ID             uuid.UUID
		BodyText       string
		BodyHTML       string
		TicketID       uuid.UUID
		Channel        string
		ContactID      *uuid.UUID
		Custom         []byte
		Kind           string
		Direction      string
		DeliveryStatus string
		Attachments    int64
	}
	err := s.d.DB.Pool.QueryRow(ctx, `SELECT m.id, m.body_text, m.body_html, m.kind, m.direction, m.delivery_status,
			t.id, t.channel, t.contact_id, t.custom,
			(SELECT count(*) FROM attachments a WHERE a.workspace_id = m.workspace_id AND a.message_id = m.id)
		FROM messages m JOIN tickets t ON t.id = m.ticket_id AND t.workspace_id = m.workspace_id
		WHERE m.workspace_id = $1 AND m.id = $2`, job.Args.WorkspaceID, job.Args.MessageID).Scan(
		&m.ID, &m.BodyText, &m.BodyHTML, &m.Kind, &m.Direction, &m.DeliveryStatus, &m.TicketID,
		&m.Channel, &m.ContactID, &m.Custom, &m.Attachments)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if m.Channel != domain.ChannelWhatsApp || m.Kind != "reply" || m.Direction != "outbound" || m.DeliveryStatus != "pending" {
		return nil
	}
	if m.Attachments > 0 {
		// One message carries one file on WhatsApp, and the channel's media path arrives in
		// the next phase; refusing beats dropping the file silently.
		return s.failMessage(ctx, job.Args.WorkspaceID, m.ID, "Attachments are not supported on WhatsApp yet.")
	}
	c, err := connectionForContact(ctx, s.d.DB.Pool, job.Args.WorkspaceID, m.Custom)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.failMessage(ctx, job.Args.WorkspaceID, m.ID, "This workspace has no WhatsApp connection.")
		}
		return err
	}
	if m.ContactID == nil {
		return s.failMessage(ctx, job.Args.WorkspaceID, m.ID, "This ticket has no contact to reply to on WhatsApp.")
	}
	waID, err := waIDForContact(ctx, s.d.DB.Pool, c.ID, *m.ContactID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.failMessage(ctx, job.Args.WorkspaceID, m.ID, "This contact has no WhatsApp number on this connection.")
		}
		return err
	}
	// The window is checked again here: the send can run seconds after the reply was written,
	// and the window may have closed in between.
	if _, open, err := contactWindow(ctx, s.d.DB.Pool, c.WorkspaceID, c.ID, *m.ContactID); err == nil && !open {
		return s.failMessage(ctx, job.Args.WorkspaceID, m.ID,
			"The 24-hour WhatsApp reply window has closed. The customer must message again before a free-form reply can go out.")
	} else if err != nil {
		return err
	}
	body := m.BodyText
	if body == "" {
		body = hd.HTMLToText(m.BodyHTML)
	}
	if len([]rune(body)) > maxTextRunes {
		return s.failMessage(ctx, job.Args.WorkspaceID, m.ID, "This message is longer than WhatsApp's limit of 4096 characters.")
	}
	acc, _, err := s.account(c)
	if err != nil {
		return s.failMessage(ctx, job.Args.WorkspaceID, m.ID, "This WhatsApp connection's credentials could not be read.")
	}
	wamid, err := NewClient(s.http).SendText(ctx, acc, waID, body)
	if err != nil {
		reason := whatsappSendError(err)
		if mErr, ok := err.(*MetaAPIError); ok && (mErr.StatusCode == http.StatusTooManyRequests || mErr.StatusCode >= 500) && job.Attempt < job.MaxAttempts {
			// Meta refused it before accepting: safe to let River try again.
			return err
		}
		return s.failMessage(ctx, job.Args.WorkspaceID, m.ID, reason)
	}
	meta := map[string]any{"provider_status": "sent", "provider_sent_at": time.Now().UTC().Format(time.RFC3339)}
	raw, _ := json.Marshal(meta)
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE messages SET delivery_status = 'sent', provider_message_id = $3,
				delivery_error = '', delivery_meta = delivery_meta || $4::jsonb
			WHERE workspace_id = $1 AND id = $2`, c.WorkspaceID, m.ID, wamid, raw); err != nil {
			return err
		}
		return domain.PublishMessageUpdated(ctx, tx, c.WorkspaceID, m.ID)
	})
}

// failMessage records why a reply could not be sent and tells the team through the message's
// event. It never retries: a send may have reached Meta before the error surfaced.
func (s *Service) failMessage(ctx context.Context, wid, msgID uuid.UUID, reason string) error {
	reason = truncate(reason, 500)
	meta := map[string]any{"provider_status": "failed", "provider_failed_at": time.Now().UTC().Format(time.RFC3339)}
	if reason != "" {
		meta["provider_error"] = reason
	}
	raw, _ := json.Marshal(meta)
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE messages SET delivery_status = 'failed', delivery_error = $3,
				delivery_meta = delivery_meta || $4::jsonb
			WHERE workspace_id = $1 AND id = $2`, wid, msgID, reason, raw); err != nil {
			return err
		}
		return domain.PublishMessageUpdated(ctx, tx, wid, msgID)
	})
}

// whatsappSendError is the safe text of a failed send: Meta's own reason when it gave one.
func whatsappSendError(err error) string {
	var me *MetaAPIError
	if errors.As(err, &me) {
		if me.UserMsg != "" {
			return me.UserMsg
		}
		if me.Message != "" {
			return me.Message
		}
	}
	return "WhatsApp could not deliver this message."
}
