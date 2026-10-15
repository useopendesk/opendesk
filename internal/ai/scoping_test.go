package ai

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/ids"
)

// Drafts ground on saved replies for everyone and for the ticket's own team, never on
// another team's or a private team the agent can't see.
func TestDraftSavedRepliesByTeam(t *testing.T) {
	e := newEnv(t, opts{noEmbed: true})
	own, other, private := ids.New(), ids.New(), ids.New()
	e.Exec(`INSERT INTO teams (id, workspace_id, name) VALUES ($1, $3, 'Billing'), ($2, $3, 'Shipping')`, own, other, e.WS.ID)
	e.Exec(`INSERT INTO teams (id, workspace_id, name, private) VALUES ($1, $2, 'Security', true)`, private, e.WS.ID)
	add := func(title string, team *uuid.UUID) {
		e.Exec(`INSERT INTO saved_replies (workspace_id, title, body_html, team_id) VALUES ($1, $2, '<p>Refund policy for invoices and refunds.</p>', $3)`,
			e.WS.ID, title, team)
	}
	add("Everyone refund", nil)
	add("Billing refund", &own)
	add("Shipping refund", &other)
	add("Security refund", &private)

	p := &auth.Principal{Kind: auth.KindUser, WorkspaceID: e.WS.ID, UserID: e.WS.Agent.ID, Role: auth.RoleAgent}
	check := func(team *uuid.UUID, want, not []string) {
		t.Helper()
		got, err := e.S.savedReplies(e.Ctx, p, team, "refund policy for invoices", 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Fatalf("team %v: %q lacks %q", team, got, w)
			}
		}
		for _, n := range not {
			if strings.Contains(got, n) {
				t.Fatalf("team %v: %q has %q", team, got, n)
			}
		}
	}
	check(nil, []string{"Everyone refund"}, []string{"Billing refund", "Shipping refund", "Security refund"})
	check(&own, []string{"Everyone refund", "Billing refund"}, []string{"Shipping refund", "Security refund"})
	// A ticket of the private team the agent isn't on (for instance assigned to them).
	check(&private, []string{"Everyone refund"}, []string{"Security refund", "Billing refund"})
	e.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ($1, $2)`, private, e.WS.Agent.ID)
	p = &auth.Principal{Kind: auth.KindUser, WorkspaceID: e.WS.ID, UserID: e.WS.Agent.ID, Role: auth.RoleAgent}
	check(&private, []string{"Everyone refund", "Security refund"}, []string{"Billing refund"})
}

// The review queue's dismiss and improve act only on replies of tickets the admin may see.
func TestAgentReviewOfPrivateTeamTicket(t *testing.T) {
	e := newEnv(t, opts{billing: true})
	sn := &fakeSnippets{}
	e.S.SetKnowledge(sn)
	t.Cleanup(func() { e.S.SetKnowledge(nil) })
	e.settings(map[string]any{"widget_answers": true})

	e.F.queue("widget", "A person from the team will reply here.\n@tool handoff_to_human {\"reason\":\"invoice\"}")
	_, tid, _ := widgetConversation(e, "Can I get an invoice with my VAT number?")
	e.runWidgetJobs()
	handed := lastMessage(e, tid)
	team := ids.New()
	e.Exec(`INSERT INTO teams (id, workspace_id, name, private) VALUES ($1, $2, 'Finance', true)`, team, e.WS.ID)
	e.Exec(`UPDATE tickets SET team_id = $3 WHERE workspace_id = $1 AND id = $2`, e.WS.ID, tid, team)

	var q reviewList
	e.Do("GET", "/v1/ai/agent/review", e.admin(), nil).Want(t, 200).JSON(t, &q)
	if len(q.Data) != 0 {
		t.Fatalf("queue %+v", q.Data)
	}
	e.Do("POST", "/v1/ai/agent/review/"+handed.ID.String()+"/dismiss", e.admin(), nil).Want(t, 404)
	e.Do("POST", "/v1/ai/agent/improve", e.admin(), map[string]any{"question": "Can I get an invoice?", "answer": "Yes.",
		"message_id": handed.ID}).Want(t, 404)
	if len(sn.snippets) != 0 {
		t.Fatalf("snippets %+v", sn.snippets)
	}

	// On the team, the admin works the reply.
	e.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ($1, $2)`, team, e.WS.Admin.ID)
	e.Do("GET", "/v1/ai/agent/review", e.admin(), nil).Want(t, 200).JSON(t, &q)
	if len(q.Data) != 1 || q.Data[0].ID != handed.ID {
		t.Fatalf("queue %+v", q.Data)
	}
	e.Do("POST", "/v1/ai/agent/review/"+handed.ID.String()+"/dismiss", e.admin(), nil).Want(t, 204)
}
