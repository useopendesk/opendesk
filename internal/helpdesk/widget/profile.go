package widget

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/domain"
)

// FieldPrefix starts the key of a contact field a signed-in visitor's page may set: the
// workspace opts a field in by naming it so, and a field without the prefix is never written
// from the widget.
const FieldPrefix = "widget_"

// syncProfile applies what a verified session carries beyond who the user is: a link to the
// company the user belongs to, when the workspace already has it, the opted-in contact
// fields, and a phone number for a contact that has none. Nothing here creates or changes a
// company, and a value that does not fit is skipped, so one bad attribute never keeps a
// visitor from the chat.
func syncProfile(ctx context.Context, tx pgx.Tx, wid, contactID uuid.UUID, company *widgetCompany, fields map[string]any, phone string) error {
	changed := false
	if phone = cleanPhone(phone); phone != "" {
		set, err := setPhone(ctx, tx, wid, contactID, phone)
		if err != nil {
			return err
		}
		changed = changed || set
	}
	if company != nil {
		linked, err := linkCompany(ctx, tx, wid, contactID, company.ID)
		if err != nil {
			return err
		}
		changed = changed || linked
	}
	if len(fields) > 0 {
		set, err := setFields(ctx, tx, wid, contactID, fields)
		if err != nil {
			return err
		}
		changed = changed || set
	}
	if changed {
		return publishContact(ctx, tx, wid, contactID, "contact.updated")
	}
	return nil
}

// cleanPhone is the phone number as it is kept: trimmed, at most 50 characters, none of them
// a control character; "" for anything else, which is then ignored.
func cleanPhone(s string) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > 50 {
		return ""
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return s
}

// setPhone gives a contact a phone number only while it has none: a number a teammate typed
// is never replaced by what a page says, and the page's own later changes wait for a teammate
// to clear it.
func setPhone(ctx context.Context, tx pgx.Tx, wid, contactID uuid.UUID, phone string) (bool, error) {
	tag, err := tx.Exec(ctx, `UPDATE contacts SET phone = $3, updated_at = now() WHERE workspace_id = $1 AND id = $2 AND phone = ''`, wid, contactID, phone)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// linkCompany points the contact at the workspace's company whose external_id is the host
// product's account id. The user_hash covers only user_id, so the page's company id is a
// claim, not a proof: it links only to a company the workspace gave that id, and an unknown
// id links nothing.
func linkCompany(ctx context.Context, tx pgx.Tx, wid, contactID uuid.UUID, externalID string) (bool, error) {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `UPDATE contacts c SET company_id = co.id, updated_at = now()
		FROM companies co
		WHERE c.workspace_id = $1 AND c.id = $2 AND co.workspace_id = $1 AND co.external_id = $3
			AND c.company_id IS DISTINCT FROM co.id`, wid, contactID, externalID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// setFields writes the opted-in contact fields the visitor's page sent and reports whether any changed.
func setFields(ctx context.Context, tx pgx.Tx, wid, contactID uuid.UUID, in map[string]any) (bool, error) {
	defs, err := domain.LoadFieldDefs(ctx, tx, wid, domain.FieldEntityContact)
	if err != nil {
		return false, err
	}
	var cur map[string]any
	if err := tx.QueryRow(ctx, `SELECT fields FROM contacts WHERE workspace_id = $1 AND id = $2`, wid, contactID).Scan(&cur); err != nil {
		return false, err
	}
	if cur == nil {
		cur = map[string]any{}
	}
	noMembers := func(uuid.UUID) error { return fmt.Errorf("not allowed here") }
	next, changed := cur, false
	for k, v := range in {
		def, ok := defs[k]
		if !ok || def.Archived || def.Type == "user" || !strings.HasPrefix(k, FieldPrefix) {
			continue
		}
		merged, changes, err := domain.MergeFields(defs, next, map[string]any{k: v}, noMembers)
		if err != nil {
			continue
		}
		next, changed = merged, changed || len(changes) > 0
	}
	if !changed {
		return false, nil
	}
	_, err = tx.Exec(ctx, `UPDATE contacts SET fields = $3, updated_at = now() WHERE workspace_id = $1 AND id = $2`, wid, contactID, next)
	return true, err
}
