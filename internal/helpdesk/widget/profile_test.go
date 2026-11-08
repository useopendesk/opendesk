package widget_test

import (
	"testing"

	"github.com/useopendesk/opendesk/internal/crypto"
)

func TestSessionCompanyAndFields(t *testing.T) {
	e, ws := setup(t)
	e.Exec(`INSERT INTO custom_fields (workspace_id, entity, key, name, type) VALUES
		($1, 'contact', 'widget_plan', 'Plan', 'text'),
		($1, 'contact', 'widget_seats', 'Seats', 'number'),
		($1, 'contact', 'vip', 'VIP', 'text')`, ws.ID)
	hash := crypto.HMACHex([]byte("s3cret"), "user-7")
	signed := map[string]any{"key": ws.WidgetKey, "user_id": "user-7", "user_hash": hash, "email": "seven@example.com",
		"company": map[string]any{"id": "acct-9", "name": "Acme Ltd", "domain": "acme.example"},
		"fields":  map[string]any{"widget_plan": "pro", "widget_seats": 12, "vip": "yes", "widget_unknown": "x", "widget_bad": "x"}}
	s := start(t, e, signed)

	var companies int
	var name, extID, domainName string
	e.Scalar(`SELECT count(*) FROM companies WHERE workspace_id = $1`, &companies, ws.ID)
	e.Scalar(`SELECT name FROM companies WHERE workspace_id = $1 AND external_id = 'acct-9'`, &name, ws.ID)
	e.Scalar(`SELECT domain FROM companies WHERE workspace_id = $1 AND external_id = 'acct-9'`, &domainName, ws.ID)
	if companies != 1 || name != "Acme Ltd" || domainName != "acme.example" {
		t.Fatalf("company: %d %q %q", companies, name, domainName)
	}
	var linked string
	e.Scalar(`SELECT co.external_id FROM contacts c JOIN companies co ON co.id = c.company_id WHERE c.id = $1`, &linked, s.Contact.ID)
	extID = linked
	if extID != "acct-9" {
		t.Fatalf("linked to %q", extID)
	}
	var fields map[string]any
	e.Scalar(`SELECT fields FROM contacts WHERE id = $1`, &fields, s.Contact.ID)
	if fields["widget_plan"] != "pro" || fields["widget_seats"] != float64(12) || fields["vip"] != nil || len(fields) != 2 {
		t.Fatalf("fields %v", fields)
	}

	// Again with new values and another name: the same company and contact, updated, not renamed.
	signed["company"] = map[string]any{"id": "acct-9", "name": "Renamed"}
	signed["fields"] = map[string]any{"widget_plan": "team"}
	signed["visitor_token"] = s.VisitorToken
	again := start(t, e, signed)
	if again.Contact.ID != s.Contact.ID {
		t.Fatal("a second identify made a new contact")
	}
	e.Scalar(`SELECT count(*) FROM companies WHERE workspace_id = $1`, &companies, ws.ID)
	e.Scalar(`SELECT name FROM companies WHERE workspace_id = $1 AND external_id = 'acct-9'`, &name, ws.ID)
	e.Scalar(`SELECT fields FROM contacts WHERE id = $1`, &fields, s.Contact.ID)
	if companies != 1 || name != "Acme Ltd" || fields["widget_plan"] != "team" || fields["widget_seats"] != float64(12) {
		t.Fatalf("update: %d %q %v", companies, name, fields)
	}

	// Without a verified identity none of it is applied.
	anon := start(t, e, map[string]any{"key": ws.WidgetKey, "company": map[string]any{"id": "acct-evil", "name": "Evil"},
		"fields": map[string]any{"widget_plan": "enterprise"}})
	e.Scalar(`SELECT count(*) FROM companies WHERE workspace_id = $1`, &companies, ws.ID)
	var co *string
	e.Scalar(`SELECT company_id::text FROM contacts WHERE id = $1`, &co, anon.Contact.ID)
	if companies != 1 || co != nil {
		t.Fatalf("anonymous visitor applied a company: %d %v", companies, co)
	}
}
