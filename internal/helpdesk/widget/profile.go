package widget

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/displayname"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/contacts"
	"github.com/useopendesk/opendesk/internal/ids"
)

// FieldPrefix starts the key of a contact field a signed-in visitor's page may set: the
// workspace opts a field in by naming it so, and a field without the prefix is never written
// from the widget.
const FieldPrefix = "widget_"

// syncProfile applies what a verified session carries beyond who the user is: the company
// they belong to, found or created by the host product's own id, and the opted-in contact
// fields. A value that does not fit its field is skipped, so one bad attribute never keeps a
// visitor from the chat.
func syncProfile(ctx context.Context, tx pgx.Tx, wid, contactID uuid.UUID, company *widgetCompany, fields map[string]any) error {
	changed := false
	if company != nil && strings.TrimSpace(company.ID) != "" {
		linked, err := linkCompany(ctx, tx, wid, contactID, *company)
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

// linkCompany points the contact at the company with the product's account id, creating the
// company the first time. An existing company is never renamed or re-domained from here, and
// is found only by that id, so a page cannot attach a contact to a company by naming it.
func linkCompany(ctx context.Context, tx pgx.Tx, wid, contactID uuid.UUID, in widgetCompany) (bool, error) {
	extID := strings.TrimSpace(in.ID)
	var companyID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM companies WHERE workspace_id = $1 AND external_id = $2`, wid, extID).Scan(&companyID)
	if db.IsNotFound(err) {
		if companyID, err = createCompany(ctx, tx, wid, extID, in); err != nil {
			return false, err
		}
	} else if err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `UPDATE contacts SET company_id = $3, updated_at = now()
		WHERE workspace_id = $1 AND id = $2 AND company_id IS DISTINCT FROM $3`, wid, contactID, companyID)
	return tag.RowsAffected() > 0, err
}

func createCompany(ctx context.Context, tx pgx.Tx, wid uuid.UUID, extID string, in widgetCompany) (uuid.UUID, error) {
	name := displayname.Clean(in.Name, displayname.Workspace)
	if name == "" {
		name = displayname.Clean(extID, displayname.Workspace)
	}
	if name == "" {
		name = "Company"
	}
	domainName := strings.ToLower(strings.TrimSpace(in.Domain))
	if domainName != "" {
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM companies WHERE workspace_id = $1 AND domain = $2)`, wid, domainName).Scan(&taken); err != nil {
			return uuid.Nil, err
		}
		if taken {
			domainName = ""
		}
	}
	id := ids.New()
	tag, err := tx.Exec(ctx, `INSERT INTO companies (id, workspace_id, name, domain, external_id) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT DO NOTHING`, id, wid, name, domainName, extID)
	if err != nil {
		return uuid.Nil, err
	}
	if tag.RowsAffected() == 0 {
		// Another request created it first.
		err := tx.QueryRow(ctx, `SELECT id FROM companies WHERE workspace_id = $1 AND external_id = $2`, wid, extID).Scan(&id)
		return id, err
	}
	if err := contacts.PublishCompany(ctx, tx, wid, id, "company.created"); err != nil {
		return uuid.Nil, err
	}
	return id, nil
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
