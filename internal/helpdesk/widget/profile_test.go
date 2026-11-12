package widget_test

import (
	"testing"

	"github.com/useopendesk/opendesk/internal/crypto"
)

func TestSessionCompanyAndFields(t *testing.T) {
	e, ws := setup(t)
	other := e.NewWorkspace()
	e.Exec(`INSERT INTO custom_fields (workspace_id, entity, key, name, type) VALUES
		($1, 'contact', 'widget_plan', 'Plan', 'text'),
		($1, 'contact', 'widget_seats', 'Seats', 'number'),
		($1, 'contact', 'vip', 'VIP', 'text')`, ws.ID)
	e.Exec(`INSERT INTO companies (workspace_id, name, domain, external_id) VALUES ($1, 'Acme Ltd', 'acme.example', 'acct-9')`, ws.ID)
	e.Exec(`INSERT INTO companies (workspace_id, name, external_id) VALUES ($1, 'Elsewhere', 'acct-other')`, other.ID)
	companies := func() int {
		var n int
		e.Scalar(`SELECT count(*) FROM companies WHERE workspace_id = $1`, &n, ws.ID)
		return n
	}
	linked := func(contactID any) *string {
		var ext *string
		e.Scalar(`SELECT co.external_id FROM contacts c LEFT JOIN companies co ON co.id = c.company_id WHERE c.id = $1`, &ext, contactID)
		return ext
	}

	// A known external id links the contact; the company's name and domain stay the workspace's.
	hash := crypto.HMACHex([]byte("s3cret"), "user-7")
	signed := map[string]any{"key": ws.WidgetKey, "user_id": "user-7", "user_hash": hash, "email": "seven@example.com",
		"company": map[string]any{"id": "acct-9", "name": "Renamed", "domain": "evil.example"},
		"fields":  map[string]any{"widget_plan": "pro", "widget_seats": 12, "vip": "yes", "widget_unknown": "x", "widget_bad": "x"}}
	s := start(t, e, signed)
	if ext := linked(s.Contact.ID); ext == nil || *ext != "acct-9" {
		t.Fatalf("linked to %v", ext)
	}
	var name, domainName string
	e.Scalar(`SELECT name FROM companies WHERE workspace_id = $1 AND external_id = 'acct-9'`, &name, ws.ID)
	e.Scalar(`SELECT domain FROM companies WHERE workspace_id = $1 AND external_id = 'acct-9'`, &domainName, ws.ID)
	if n := companies(); n != 1 || name != "Acme Ltd" || domainName != "acme.example" {
		t.Fatalf("company: %d %q %q", n, name, domainName)
	}
	var fields map[string]any
	e.Scalar(`SELECT fields FROM contacts WHERE id = $1`, &fields, s.Contact.ID)
	if fields["widget_plan"] != "pro" || fields["widget_seats"] != float64(12) || fields["vip"] != nil || len(fields) != 2 {
		t.Fatalf("fields %v", fields)
	}

	// Again with new fields on the same session: the same contact, updated.
	signed["company"] = map[string]any{"id": "acct-9"}
	signed["fields"] = map[string]any{"widget_plan": "team"}
	signed["visitor_token"] = s.VisitorToken
	again := start(t, e, signed)
	e.Scalar(`SELECT fields FROM contacts WHERE id = $1`, &fields, s.Contact.ID)
	if again.Contact.ID != s.Contact.ID || fields["widget_plan"] != "team" || fields["widget_seats"] != float64(12) {
		t.Fatalf("update: %s %v", again.Contact.ID, fields)
	}

	// An unknown id, or one only another workspace has, creates nothing and links nothing,
	// and the session still starts.
	for _, id := range []string{"acct-unknown", "acct-other"} {
		h := crypto.HMACHex([]byte("s3cret"), "user-"+id)
		u := start(t, e, map[string]any{"key": ws.WidgetKey, "user_id": "user-" + id, "user_hash": h,
			"company": map[string]any{"id": id, "name": "New Co", "domain": "new.example"}})
		if ext := linked(u.Contact.ID); ext != nil || companies() != 1 {
			t.Fatalf("%s: linked to %v, %d companies", id, ext, companies())
		}
	}
	var otherLinks int
	e.Scalar(`SELECT count(*) FROM contacts WHERE workspace_id = $1 AND company_id IS NOT NULL`, &otherLinks, other.ID)
	if otherLinks != 0 {
		t.Fatal("linked a contact in another workspace")
	}

	// Without a verified identity none of it is applied.
	anon := start(t, e, map[string]any{"key": ws.WidgetKey, "company": map[string]any{"id": "acct-9", "name": "Evil"},
		"fields": map[string]any{"widget_plan": "enterprise"}})
	var co *string
	e.Scalar(`SELECT company_id::text FROM contacts WHERE id = $1`, &co, anon.Contact.ID)
	if companies() != 1 || co != nil {
		t.Fatalf("anonymous visitor applied a company: %d %v", companies(), co)
	}
}

// TestSessionPhone: a verified page gives a contact a phone number once; what a teammate typed,
// or the page sent earlier, is not replaced, and an unverified visitor sets none.
func TestSessionPhone(t *testing.T) {
	e, ws := setup(t)
	phone := func(id any) string {
		var p string
		e.Scalar(`SELECT phone FROM contacts WHERE id = $1`, &p, id)
		return p
	}
	e.Exec(`INSERT INTO custom_fields (workspace_id, entity, key, name, type) VALUES ($1, 'contact', 'widget_signed_up', 'Signed up', 'date')`, ws.ID)
	hash := crypto.HMACHex([]byte("s3cret"), "user-phone")
	body := map[string]any{"key": ws.WidgetKey, "user_id": "user-phone", "user_hash": hash, "email": "phone@example.com", "phone": " +34 600 111 222 ",
		"fields": map[string]any{"widget_signed_up": "2024-05-14"}}
	s := start(t, e, body)
	// The sign-up date a page sends lands in a date field, as the docs say.
	var signedUp *string
	e.Scalar(`SELECT fields->>'widget_signed_up' FROM contacts WHERE id = $1`, &signedUp, s.Contact.ID)
	if signedUp == nil || *signedUp != "2024-05-14" {
		t.Fatalf("sign-up date %v", signedUp)
	}
	if got := phone(s.Contact.ID); got != "+34 600 111 222" {
		t.Fatalf("phone %q", got)
	}
	var events int
	e.Scalar(`SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'contact.updated'`, &events, ws.ID)
	if events == 0 {
		t.Fatal("the change was not announced")
	}

	// The page changing it later, with the same session, leaves it: only an empty number is filled.
	body["phone"], body["visitor_token"] = "+34 999 999 999", s.VisitorToken
	start(t, e, body)
	if got := phone(s.Contact.ID); got != "+34 600 111 222" {
		t.Fatalf("replaced with %q", got)
	}
	// A teammate's number is kept, and an emptied one is filled again.
	e.Exec(`UPDATE contacts SET phone = '020 7946 0000' WHERE id = $1`, s.Contact.ID)
	start(t, e, body)
	if got := phone(s.Contact.ID); got != "020 7946 0000" {
		t.Fatalf("a teammate's number was replaced by %q", got)
	}

	// Anything that is not a phone number as kept is ignored, and the session still starts.
	for name, bad := range map[string]string{"control": "060\x00 1", "long": "0123456789012345678901234567890123456789012345678901"} {
		h := crypto.HMACHex([]byte("s3cret"), "user-"+name)
		u := start(t, e, map[string]any{"key": ws.WidgetKey, "user_id": "user-" + name, "user_hash": h, "phone": bad})
		if got := phone(u.Contact.ID); got != "" {
			t.Fatalf("%s: stored %q", name, got)
		}
	}

	// Without a verified identity none is set.
	anon := start(t, e, map[string]any{"key": ws.WidgetKey, "phone": "+1 555 0100"})
	if got := phone(anon.Contact.ID); got != "" {
		t.Fatalf("anonymous visitor set %q", got)
	}
}
