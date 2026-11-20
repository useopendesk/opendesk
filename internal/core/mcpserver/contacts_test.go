package mcpserver

import (
	"slices"
	"strings"
	"testing"

	"github.com/useopendesk/opendesk/internal/auth"
)

func TestContactAndCompanyTools(t *testing.T) {
	e := newEnv(t)
	rw := e.session(e.APIKey(e.WS, auth.ScopeContactsRead, auth.ScopeContactsWrite))

	type company struct {
		Company struct {
			ID         string   `json:"id"`
			Name       string   `json:"name"`
			Domain     string   `json:"domain"`
			ExternalID string   `json:"external_id"`
			Labels     []string `json:"labels"`
		} `json:"company"`
	}
	var co struct {
		ID         string   `json:"id"`
		Name       string   `json:"name"`
		Domain     string   `json:"domain"`
		ExternalID string   `json:"external_id"`
		Labels     []string `json:"labels"`
	}
	call(t, rw, "create_company", map[string]any{"name": "Acme", "domain": "acme.com", "external_id": "acct_1", "labels": []string{"vip"}}, &co)
	if co.ID == "" || co.Domain != "acme.com" || co.ExternalID != "acct_1" || !slices.Contains(co.Labels, "vip") {
		t.Fatalf("create_company: %+v", co)
	}
	// A taken external id is refused.
	if res := call(t, rw, "create_company", map[string]any{"name": "Acme 2", "external_id": "acct_1"}, nil); !res.IsError || !contains(text(res), "external_id") {
		t.Fatalf("duplicate external id: %v %s", res.IsError, text(res))
	}

	type contact struct {
		ID         string   `json:"id"`
		Email      string   `json:"email"`
		Name       string   `json:"name"`
		Phone      string   `json:"phone"`
		ExternalID string   `json:"external_id"`
		Labels     []string `json:"labels"`
		Company    *struct {
			ID string `json:"id"`
		} `json:"company"`
	}
	var ct contact
	// A company by its name.
	call(t, rw, "create_contact", map[string]any{"email": "ana@acme.com", "name": "Ana", "phone": "+31 20 555 0100", "company": "acme",
		"external_id": "u_1", "labels": []string{"beta"}}, &ct)
	if ct.ID == "" || ct.Phone == "" || ct.ExternalID != "u_1" || ct.Company == nil || ct.Company.ID != co.ID || !slices.Contains(ct.Labels, "beta") {
		t.Fatalf("create_contact: %+v", ct)
	}
	// An address that belongs to a contact names that contact.
	if res := call(t, rw, "create_contact", map[string]any{"email": "ANA@acme.com"}, nil); !res.IsError || !contains(text(res), ct.ID) {
		t.Fatalf("duplicate email: %v %s", res.IsError, text(res))
	}
	if res := call(t, rw, "create_contact", map[string]any{"phone": "1"}, nil); !res.IsError {
		t.Fatal("a contact needs an email or a name")
	}
	if res := call(t, rw, "create_contact", map[string]any{"email": "x@example.com", "company": "No such"}, nil); !res.IsError {
		t.Fatal("an unknown company name was accepted")
	}

	// Labels merge, a phone clears, the company goes.
	var upd contact
	call(t, rw, "update_contact", map[string]any{"contact": "ana@acme.com", "add_labels": []string{"renewal"}, "phone": "", "company": "none"}, &upd)
	if upd.ID != ct.ID || upd.Phone != "" || upd.Company != nil || !slices.Contains(upd.Labels, "beta") || !slices.Contains(upd.Labels, "renewal") {
		t.Fatalf("update_contact: %+v", upd)
	}
	if res := call(t, rw, "update_contact", map[string]any{"contact": ct.ID}, nil); !res.IsError {
		t.Fatal("an update of nothing was accepted")
	}

	var got company
	call(t, rw, "update_company", map[string]any{"company": "Acme", "name": "Acme Inc", "remove_labels": []string{"vip"}}, nil)
	call(t, rw, "get_company", map[string]any{"company": co.ID}, &got)
	if got.Company.Name != "Acme Inc" || slices.Contains(got.Company.Labels, "vip") || got.Company.ExternalID != "acct_1" {
		t.Fatalf("get_company: %+v", got)
	}
	var found struct {
		Count int `json:"count"`
	}
	call(t, rw, "search_companies", map[string]any{"query": "acme"}, &found)
	if found.Count != 1 {
		t.Fatalf("search_companies: %+v", found)
	}

	// The scopes are the API's: reading alone cannot write.
	ro := e.session(e.APIKey(e.WS, auth.ScopeContactsRead))
	if names := toolNames(t, ro); slices.Contains(names, "create_contact") || !slices.Contains(names, "get_company") {
		t.Fatalf("contacts:read tools: %v", names)
	}
	wo := e.session(e.APIKey(e.WS, auth.ScopeContactsWrite))
	if res := call(t, wo, "update_contact", map[string]any{"contact": ct.ID, "company": "Acme Inc"}, nil); !res.IsError || !contains(text(res), "contacts:read") {
		t.Fatalf("a name without contacts:read: %v %s", res.IsError, text(res))
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
