package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/helpdesk/routing"
	"github.com/useopendesk/opendesk/internal/jobs"
)

// inboundWorker applies one verified Meta delivery: the messages in it become ticket
// messages, the statuses in it update what we sent.
type inboundWorker struct {
	river.WorkerDefaults[jobs.WhatsAppInboundArgs]
	s *Service
}

func (w *inboundWorker) Work(ctx context.Context, job *river.Job[jobs.WhatsAppInboundArgs]) error {
	s := w.s
	connID, payload, err := loadInboundEvent(ctx, s.d.DB.Pool, job.Args.WorkspaceID, job.Args.EventID)
	if errors.Is(err, pgx.ErrNoRows) {
		// The prune job removed it between queueing and running.
		return nil
	}
	if err != nil {
		return err
	}
	c, err := loadConnection(ctx, s.d.DB.Pool, job.Args.WorkspaceID, connID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	payloadData, err := ParsePayload(payload)
	if err != nil {
		// Verified but not understood: acknowledged so Meta keeps the webhook on.
		s.d.Log.Warn("whatsapp delivery not understood", "connection_id", c.ID, "err", err)
		return nil
	}
	var errs []error
	for _, m := range payloadData.ExtractMessages() {
		if err := w.ingest(ctx, c, m); err != nil {
			s.d.Log.Error("whatsapp message not applied", "connection_id", c.ID, "wa_message_id", m.ID, "err", err)
			errs = append(errs, err)
		}
	}
	for _, st := range payloadData.ExtractStatuses() {
		if err := w.applyStatus(ctx, c, st); err != nil {
			s.d.Log.Error("whatsapp status not applied", "connection_id", c.ID, "wa_message_id", st.MessageID, "err", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ingest applies one inbound WhatsApp message: the contact, the 24-hour window, the ticket
// (reusing an open one), and the message itself. All of it is one transaction with the
// ticket's and message's events.
func (w *inboundWorker) ingest(ctx context.Context, c connection, m ParsedMessage) error {
	s := w.s
	if m.ID == "" || m.From == "" {
		return nil
	}
	// Kinds that carry nothing to answer, or that would open a ticket on their own.
	switch m.Type {
	case "reaction", "ephemeral", "request_welcome", "system":
		return nil
	}
	// Meta posts every number of an app to one callback; the payload's number decides the
	// connection. A mismatch means the URL named another number of the same app.
	if m.PhoneNumberID != "" && m.PhoneNumberID != c.PhoneNumberID {
		if other, err := loadConnectionByPhoneNumber(ctx, s.d.DB.Pool, m.PhoneNumberID); err == nil && other.WorkspaceID == c.WorkspaceID {
			c = other
		} else {
			return nil
		}
	}
	at := m.Timestamp
	if at.IsZero() {
		at = time.Now().UTC()
	}
	text := strings.TrimSpace(m.Text)
	if text == "" {
		text = strings.TrimSpace(m.Caption)
	}
	if text == "" {
		text = placeholderFor(m.Type)
	}
	text = truncate(text, 10000)
	// Meta's retry of a message already applied: the window was advanced with it.
	var exists bool
	if err := s.d.DB.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM messages WHERE workspace_id = $1 AND provider_message_id = $2)`,
		c.WorkspaceID, m.ID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	subject := truncate(text, 80)
	if subject == "" {
		subject = "WhatsApp message"
	}
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		pc, err := domain.ContactByPhone(ctx, tx, c.WorkspaceID, m.From, m.ContactName)
		if err != nil {
			return err
		}
		if pc.Blocked {
			return nil
		}
		if err := upsertContactWindow(ctx, tx, c.WorkspaceID, c.ID, pc.ID, m.From, m.ContactName, at); err != nil {
			return err
		}
		ticketID, err := openTicketFor(ctx, tx, c.WorkspaceID, pc.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			ticketID, err = createTicketFor(ctx, s, tx, c, pc.ID, subject, text)
		}
		if err != nil {
			return err
		}
		msg, err := domain.AddMessage(ctx, tx, s.d.Jobs, domain.NewMessage{
			WorkspaceID:     c.WorkspaceID,
			TicketID:        ticketID,
			Kind:            "reply",
			Direction:       "inbound",
			AuthorType:      "contact",
			AuthorContactID: &pc.ID,
			BodyHTML:        hd.TextToHTML(text),
			BodyText:        text,
			DeliveryStatus:  "none",
			CreatedAt:       at,
		})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE messages SET provider_message_id = $3 WHERE workspace_id = $1 AND id = $2`,
			c.WorkspaceID, msg.ID, m.ID)
		return err
	})
}

// openTicketFor returns the contact's open WhatsApp ticket on this workspace, else pgx.ErrNoRows.
func openTicketFor(ctx context.Context, tx pgx.Tx, wid, contactID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM tickets
		WHERE workspace_id = $1 AND channel = $2 AND contact_id = $3 AND deleted_at IS NULL
			AND status IN ('open', 'pending', 'on_hold')
		ORDER BY last_message_at DESC, id DESC LIMIT 1`, wid, domain.ChannelWhatsApp, contactID).Scan(&id)
	return id, err
}

// createTicketFor opens a WhatsApp ticket for the contact, routing it like any other ticket.
func createTicketFor(ctx context.Context, s *Service, tx pgx.Tx, c connection, contactID uuid.UUID, subject, body string) (uuid.UUID, error) {
	res, err := routing.Resolve(ctx, tx, routing.Input{WorkspaceID: c.WorkspaceID, Channel: domain.ChannelWhatsApp,
		BrandID: c.BrandID, ContactID: &contactID, Subject: subject, Body: body})
	if err != nil {
		// Routing is best effort: a ticket with no team is better than a dropped message.
		s.d.Log.Warn("whatsapp ticket routing failed", "connection_id", c.ID, "err", err)
		res = routing.Result{}
	}
	brandID := c.BrandID
	t, err := domain.CreateTicket(ctx, tx, domain.NewTicket{WorkspaceID: c.WorkspaceID, Subject: subject,
		Channel: domain.ChannelWhatsApp, ContactID: &contactID, BrandID: &brandID, TeamID: res.TeamID,
		Routed: res.Routed, RequiredSkillIDs: res.RequiredSkillIDs,
		Custom: map[string]any{"whatsapp_connection_id": c.ID.String()}})
	if err != nil {
		return uuid.Nil, err
	}
	if rule := res.RuleID(); rule != nil {
		if err := routing.Count(ctx, tx, *rule); err != nil {
			return uuid.Nil, err
		}
	}
	return t.ID, nil
}

// applyStatus applies Meta's delivery report to the message it is about.
func (w *inboundWorker) applyStatus(ctx context.Context, c connection, st ParsedStatus) error {
	s := w.s
	if st.MessageID == "" || st.Status == "" {
		return nil
	}
	at := st.Timestamp
	if at.IsZero() {
		at = time.Now().UTC()
	}
	patch := map[string]any{"provider_status": st.Status, "provider_status_updated_at": at.Format(time.RFC3339)}
	if st.ErrorCode != 0 {
		patch["provider_error_code"] = st.ErrorCode
	}
	if st.UserMsg != "" {
		patch["provider_error"] = st.UserMsg
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	status, detail := "sent", ""
	if st.Status == "failed" {
		status, detail = "failed", truncate(st.UserMsg, 500)
	}
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		var msgID uuid.UUID
		err := tx.QueryRow(ctx, `UPDATE messages SET
				delivery_status = CASE WHEN $3 = 'failed' THEN 'failed'
					WHEN messages.delivery_status = 'failed' THEN 'failed' ELSE 'sent' END,
				delivery_error = CASE WHEN $3 = 'failed' THEN $4 ELSE messages.delivery_error END,
				delivery_meta = messages.delivery_meta || $5::jsonb
			WHERE workspace_id = $1 AND provider_message_id = $2 AND direction = 'outbound'
				AND delivery_status <> 'failed'
			RETURNING id`, c.WorkspaceID, st.MessageID, status, detail, raw).Scan(&msgID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return domain.PublishMessageUpdated(ctx, tx, c.WorkspaceID, msgID)
	})
}

// placeholderFor names a message kind whose payload carries no text the agent can read.
func placeholderFor(kind string) string {
	switch kind {
	case "image":
		return "[image]"
	case "video":
		return "[video]"
	case "audio", "voice":
		return "[audio]"
	case "document":
		return "[document]"
	case "sticker":
		return "[sticker]"
	case "location":
		return "[location]"
	default:
		return "[" + kind + "]"
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
