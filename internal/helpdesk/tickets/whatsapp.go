package tickets

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// whatsAppWindow is Meta's customer service window: free-form replies are allowed for 24
// hours after the customer's latest message.
const whatsAppWindow = 24 * time.Hour

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
