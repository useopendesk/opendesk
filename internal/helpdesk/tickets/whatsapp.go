package tickets

import (
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// whatsAppWindow is Meta's customer service window: free-form replies are allowed for 24
// hours after the customer's latest message.
const whatsAppWindow = 24 * time.Hour

// Meta's caps on a free-form reply: 4096 characters of text, or 1024 of caption when the
// reply carries one file.
const (
	whatsAppMaxTextRunes    = 4096
	whatsAppMaxCaptionRunes = 1024
)

// whatsAppConnectionForTicket returns the WhatsApp connection a ticket was opened through:
// tickets.custom.whatsapp_connection_id, else the workspace's first connection.
func whatsAppConnectionForTicket(ctx context.Context, tx pgx.Tx, wid, ticketID uuid.UUID) (uuid.UUID, error) {
	var custom []byte
	if err := tx.QueryRow(ctx, `SELECT custom FROM tickets WHERE workspace_id = $1 AND id = $2`, wid, ticketID).Scan(&custom); err != nil {
		return uuid.Nil, err
	}
	var meta map[string]any
	if len(custom) > 0 {
		_ = json.Unmarshal(custom, &meta)
	}
	if raw, ok := meta["whatsapp_connection_id"].(string); ok {
		if id, err := uuid.Parse(raw); err == nil && id != uuid.Nil {
			return id, nil
		}
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM whatsapp_connections WHERE workspace_id = $1 ORDER BY created_at LIMIT 1`, wid).Scan(&id)
	if db.IsNotFound(err) {
		return uuid.Nil, httpx.E(409, "whatsapp_not_configured", "This workspace has no WhatsApp connection.")
	}
	return id, err
}

// whatsAppWindowOpen reports whether the contact wrote through this connection within the
// last 24 hours, so a free-form reply may go out.
func whatsAppWindowOpen(ctx context.Context, tx pgx.Tx, wid, connID, contactID uuid.UUID) (bool, error) {
	var last *time.Time
	err := tx.QueryRow(ctx, `SELECT last_inbound_at FROM whatsapp_contacts
		WHERE workspace_id = $1 AND connection_id = $2 AND contact_id = $3`, wid, connID, contactID).Scan(&last)
	if db.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return last != nil && time.Since(*last) < whatsAppWindow, nil
}

// validateWhatsAppReply holds a free-form WhatsApp reply to Meta's limits: one file per
// message, and a caption of at most 1024 characters when the reply carries it. Text alone
// may be 4096. The 24-hour window is checked before this.
func validateWhatsAppReply(attIDs []uuid.UUID, bodyText string) error {
	switch {
	case len(attIDs) > 1:
		return httpx.Validation("WhatsApp takes one file per message. Send the files one at a time.")
	case len(attIDs) == 1 && utf8.RuneCountInString(bodyText) > whatsAppMaxCaptionRunes:
		return httpx.Validation("a WhatsApp caption can be at most 1024 characters")
	case len(attIDs) == 0 && utf8.RuneCountInString(bodyText) > whatsAppMaxTextRunes:
		return httpx.Validation("the message is longer than WhatsApp's limit of 4096 characters")
	}
	return nil
}
