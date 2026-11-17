package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/ids"
)

// connectionColumns is the select list scanConnection reads.
const connectionColumns = `c.id, c.workspace_id, c.brand_id, c.name, c.phone_number_id, c.waba_id,
	c.display_phone, c.verified_name, c.api_version, c.status, c.last_error, c.last_event_at,
	c.created_at, c.updated_at, c.credentials_enc`

func scanConnection(row pgx.Row) (connection, error) {
	var c connection
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.BrandID, &c.Name, &c.PhoneNumberID, &c.WABAID,
		&c.DisplayPhone, &c.VerifiedName, &c.APIVersion, &c.Status, &c.LastError, &c.LastEventAt,
		&c.CreatedAt, &c.UpdatedAt, &c.credentialsEnc)
	return c, err
}

// loadConnectionByID reads one connection by id, for the public webhook before a workspace
// is known.
func loadConnectionByID(ctx context.Context, q db.Querier, id uuid.UUID) (connection, error) {
	return scanConnection(q.QueryRow(ctx, `SELECT `+connectionColumns+` FROM whatsapp_connections c WHERE c.id = $1`, id))
}

// loadConnection reads one connection by id.
func loadConnection(ctx context.Context, q db.Querier, wid, id uuid.UUID) (connection, error) {
	return scanConnection(q.QueryRow(ctx, `SELECT `+connectionColumns+` FROM whatsapp_connections c
		WHERE c.workspace_id = $1 AND c.id = $2`, wid, id))
}

// listConnections lists a workspace's connections.
func listConnections(ctx context.Context, q db.Querier, wid uuid.UUID) ([]connection, error) {
	rows, err := q.Query(ctx, `SELECT `+connectionColumns+` FROM whatsapp_connections c
		WHERE c.workspace_id = $1 ORDER BY c.created_at`, wid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []connection
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// loadConnectionByPhoneNumber reads the connection a Meta delivery names. Meta posts every
// number of an app to one callback URL, so the payload's phone_number_id decides the connection.
func loadConnectionByPhoneNumber(ctx context.Context, q db.Querier, phoneNumberID string) (connection, error) {
	return scanConnection(q.QueryRow(ctx, `SELECT `+connectionColumns+` FROM whatsapp_connections c
		WHERE c.phone_number_id = $1 ORDER BY c.created_at LIMIT 1`, phoneNumberID))
}

// insertConnection stores a new connection.
func insertConnection(ctx context.Context, q db.Querier, c connection) error {
	_, err := q.Exec(ctx, `INSERT INTO whatsapp_connections (id, workspace_id, brand_id, name, phone_number_id,
		waba_id, display_phone, verified_name, api_version, status, last_error, created_by, created_at, updated_at, credentials_enc)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, c.ID, c.WorkspaceID, c.BrandID, c.Name,
		c.PhoneNumberID, c.WABAID, c.DisplayPhone, c.VerifiedName, c.APIVersion, c.Status, c.LastError,
		c.CreatedBy, c.CreatedAt, c.UpdatedAt, c.credentialsEnc)
	return err
}

// updateConnection stores a connection's mutable fields: name, credentials, the account it
// points at and its health.
func updateConnection(ctx context.Context, q db.Querier, c connection) error {
	_, err := q.Exec(ctx, `UPDATE whatsapp_connections SET name = $3, phone_number_id = $4, waba_id = $5,
		display_phone = $6, verified_name = $7, api_version = $8, status = $9, last_error = $10,
		credentials_enc = $11, updated_at = $12 WHERE workspace_id = $1 AND id = $2`, c.WorkspaceID, c.ID,
		c.Name, c.PhoneNumberID, c.WABAID, c.DisplayPhone, c.VerifiedName, c.APIVersion, c.Status,
		c.LastError, c.credentialsEnc, c.UpdatedAt)
	return err
}

// deleteConnection removes a connection, its inbound events and its contact windows.
func deleteConnection(ctx context.Context, q db.Querier, wid, id uuid.UUID) error {
	_, err := q.Exec(ctx, `DELETE FROM whatsapp_connections WHERE workspace_id = $1 AND id = $2`, wid, id)
	return err
}

// insertInboundEvent stores a verified Meta delivery. false means Meta's retry of one already
// stored, so the dispatcher stops here.
func insertInboundEvent(ctx context.Context, q db.Querier, wid, connID uuid.UUID, externalID string, payload []byte) (uuid.UUID, bool, error) {
	id := ids.New()
	tag, err := q.Exec(ctx, `INSERT INTO whatsapp_inbound_events (id, workspace_id, connection_id, external_event_id, payload, received_at)
		VALUES ($1,$2,$3,$4,$5,now()) ON CONFLICT (connection_id, external_event_id) DO NOTHING`, id, wid, connID, externalID, json.RawMessage(payload))
	if err != nil {
		return uuid.Nil, false, err
	}
	return id, tag.RowsAffected() == 1, nil
}

// loadInboundEvent reads one stored delivery by id.
func loadInboundEvent(ctx context.Context, q db.Querier, wid, id uuid.UUID) (connID uuid.UUID, payload []byte, err error) {
	err = q.QueryRow(ctx, `SELECT connection_id, payload FROM whatsapp_inbound_events
		WHERE workspace_id = $1 AND id = $2`, wid, id).Scan(&connID, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil, pgx.ErrNoRows
	}
	return connID, payload, err
}

// touchConnection records the latest delivery on a connection and clears its error.
func touchConnection(ctx context.Context, q db.Querier, id uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE whatsapp_connections SET last_event_at = now(), status = 'connected', last_error = '' WHERE id = $1`, id)
	return err
}

// markConnectionError records why a connection cannot send, so the settings page can say it.
func markConnectionError(ctx context.Context, q db.Querier, id uuid.UUID, msg string) error {
	_, err := q.Exec(ctx, `UPDATE whatsapp_connections SET status = 'error', last_error = $2 WHERE id = $1`, id, msg)
	return err
}

// upsertContactWindow links a wa_id to the contact it writes as, and advances the 24-hour
// window. Every inbound message advances it, duplicates included (Meta's window counts them).
func upsertContactWindow(ctx context.Context, q db.Querier, wid, connID, contactID uuid.UUID, waID, name string, at time.Time) error {
	_, err := q.Exec(ctx, `INSERT INTO whatsapp_contacts (workspace_id, connection_id, wa_id, contact_id, profile_name, last_inbound_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,now(),now())
		ON CONFLICT (connection_id, wa_id) DO UPDATE SET contact_id = EXCLUDED.contact_id,
			profile_name = CASE WHEN EXCLUDED.profile_name <> '' THEN EXCLUDED.profile_name ELSE whatsapp_contacts.profile_name END,
			last_inbound_at = GREATEST(COALESCE(whatsapp_contacts.last_inbound_at, EXCLUDED.last_inbound_at), EXCLUDED.last_inbound_at),
			updated_at = now()`, wid, connID, waID, contactID, name, at)
	return err
}

// contactWindow reports when the contact last wrote through this connection, and whether they
// are inside Meta's 24-hour window.
func contactWindow(ctx context.Context, q db.Querier, wid, connID, contactID uuid.UUID) (last time.Time, open bool, err error) {
	err = q.QueryRow(ctx, `SELECT last_inbound_at FROM whatsapp_contacts
		WHERE workspace_id = $1 AND connection_id = $2 AND contact_id = $3`, wid, connID, contactID).Scan(&last)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return last, time.Since(last) < WindowDuration, nil
}

// connectionForContact returns the connection the ticket's messages go out from: the one the
// ticket was opened through (tickets.custom.whatsapp_connection_id), else the workspace's
// first connection. A recorded connection that was disconnected is skipped, so a ticket whose
// number was removed and connected again keeps replying through the new connection.
func connectionForContact(ctx context.Context, q db.Querier, wid uuid.UUID, custom []byte) (connection, error) {
	var meta map[string]any
	if len(custom) > 0 {
		_ = json.Unmarshal(custom, &meta)
	}
	if raw, ok := meta["whatsapp_connection_id"].(string); ok {
		if id, err := uuid.Parse(raw); err == nil {
			c, err := loadConnection(ctx, q, wid, id)
			if err == nil {
				return c, nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return connection{}, err
			}
			// The ticket's connection was disconnected: the workspace's current one speaks
			// for it, with the contact window and number it has.
		}
	}
	all, err := listConnections(ctx, q, wid)
	if err != nil {
		return connection{}, err
	}
	if len(all) == 0 {
		return connection{}, pgx.ErrNoRows
	}
	return all[0], nil
}

// waIDForContact returns the wa_id a contact writes from on a connection.
func waIDForContact(ctx context.Context, q db.Querier, connID, contactID uuid.UUID) (string, error) {
	var waID string
	err := q.QueryRow(ctx, `SELECT wa_id FROM whatsapp_contacts WHERE connection_id = $1 AND contact_id = $2 LIMIT 1`, connID, contactID).Scan(&waID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", pgx.ErrNoRows
	}
	return waID, err
}

// pruneEvents removes applied deliveries older than eventRetention.
func pruneEvents(ctx context.Context, q db.Querier) error {
	_, err := q.Exec(ctx, `DELETE FROM whatsapp_inbound_events WHERE received_at < now() - interval '7 days'`)
	return err
}
