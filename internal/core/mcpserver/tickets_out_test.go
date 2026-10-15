package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/ids"
)

func TestCompactTicketCarriesRouting(t *testing.T) {
	var a apiTicket
	raw := `{"id":"t1","number":7,"subject":"Refund","status":"open","priority":"normal","channel":"email",
		"mailbox_id":"m1","team_id":"team1","brand_id":"b1","received_at":"billing@acme.test"}`
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatal(err)
	}
	o := compactTicket(a, false)
	if o.BrandID != "b1" || o.ReceivedAt != "billing@acme.test" || o.ReplyFrom == nil || o.ReplyFrom.MailboxID != "m1" ||
		o.TeamID == nil || *o.TeamID != "team1" {
		t.Fatalf("%+v", o)
	}
	c := &caller{names: &refNames{loaded: true, teams: map[string]string{"team1": "Billing"}, brands: map[string]string{"b1": "Acme EU"}}}
	c.name(t.Context(), &o)
	out, _ := json.Marshal(o)
	for _, want := range []string{`"team":"Billing"`, `"brand":"Acme EU"`, `"received_at":"billing@acme.test"`, `"reply_from":{"mailbox_id":"m1"}`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("%s lacks %s", out, want)
		}
	}
	// Names the caller may not list stay ids only.
	c = &caller{names: &refNames{loaded: true}}
	o = compactTicket(a, false)
	c.name(t.Context(), &o)
	if o.Team != "" || o.Brand != "" || o.TeamID == nil {
		t.Fatalf("%+v", o)
	}
}

func TestPrivateTeamTicketsThroughMCP(t *testing.T) {
	e := newEnv(t)
	e.Exec(`INSERT INTO brands (workspace_id, name, slug, is_default) SELECT w.id, w.name, w.slug, true FROM workspaces w
		WHERE w.id = $1 AND NOT EXISTS (SELECT 1 FROM brands b WHERE b.workspace_id = w.id AND b.is_default)`, e.WS.ID)
	team := ids.New()
	e.Exec(`INSERT INTO teams (id, workspace_id, name, private) VALUES ($1, $2, 'Security', true)`, team, e.WS.ID)
	tk := e.ticket("Vulnerability xyzzy", "I found an xyzzy vulnerability.")
	e.Exec(`UPDATE tickets SET team_id = $3 WHERE workspace_id = $1 AND id = $2`, e.WS.ID, tk.ID, team)

	plain := e.APIKey(e.WS, auth.ScopeTicketsRead, auth.ScopeTicketsWrite, auth.ScopeSettingsRead)
	granted := e.APIKey(e.WS, auth.ScopeTicketsRead, auth.ScopeSettingsRead)
	e.Exec(`UPDATE api_keys SET private_team_ids = ARRAY[$2::uuid] WHERE key_hash = $1`, crypto.HashToken(granted), team)

	// A key without the team neither finds, reads nor writes the ticket.
	s := e.session(plain)
	var list struct {
		Count int `json:"count"`
	}
	call(t, s, "search_tickets", map[string]any{"query": "xyzzy"}, &list)
	if list.Count != 0 {
		t.Fatalf("search found a private team's ticket: %d", list.Count)
	}
	for name, args := range map[string]map[string]any{
		"get_ticket":    {"ticket": tk.ID.String()},
		"add_note":      {"ticket": tk.ID.String(), "body": "hello"},
		"update_ticket": {"ticket": tk.ID.String(), "priority": "high"},
	} {
		if res := call(t, s, name, args, nil); !res.IsError || !strings.Contains(text(res), "404") {
			t.Fatalf("%s: %s", name, text(res))
		}
	}

	// A key granted the team reads it, with its team and brand by name.
	s = e.session(granted)
	var th struct {
		Ticket struct {
			ID    string `json:"id"`
			Team  string `json:"team"`
			Brand string `json:"brand"`
		} `json:"ticket"`
	}
	call(t, s, "get_ticket", map[string]any{"ticket": tk.ID.String()}, &th)
	if th.Ticket.ID != tk.ID.String() || th.Ticket.Team != "Security" || th.Ticket.Brand == "" {
		t.Fatalf("granted get_ticket: %+v", th)
	}
	call(t, s, "search_tickets", map[string]any{"query": "xyzzy"}, &list)
	if list.Count != 1 {
		t.Fatalf("granted search: %d", list.Count)
	}
}
