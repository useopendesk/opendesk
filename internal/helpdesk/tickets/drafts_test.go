package tickets_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/helpdesk/tickets"
)

func TestSharedDraft(t *testing.T) {
	e, ws := newEnv(t)
	tk := createTicket(t, e, ws, map[string]any{"channel": "api", "kind": "note"})
	path := "/v1/tickets/" + tk.ID.String() + "/draft"
	key := e.APIKey(ws, auth.ScopeTicketsWrite)
	_, seq := e.Events(ws.ID, 0)

	// Nothing staged yet.
	var got struct{ Draft *tickets.TicketDraft }
	e.Do("GET", path, ws.Viewer.As(ws), nil).Want(t, 200).JSON(t, &got)
	if got.Draft != nil {
		t.Fatalf("draft before staging: %+v", got.Draft)
	}

	// An API key stages one: sanitized, attributed to the key by name, visible to every teammate.
	var d tickets.TicketDraft
	e.Do("PUT", path, hdtest.Auth{Bearer: key}, map[string]any{"body_html": "<p>Hi <b>there</b></p><script>alert(1)</script>"}).Want(t, 200).JSON(t, &d)
	if d.TicketID != tk.ID || d.BodyText != "Hi there" || strings.Contains(d.BodyHTML, "script") || d.By.Type != "api" || d.By.Name != "test" {
		t.Fatalf("staged draft %+v", d)
	}
	e.Do("GET", path, ws.Viewer.As(ws), nil).Want(t, 200).JSON(t, &got)
	if got.Draft == nil || got.Draft.BodyText != "Hi there" {
		t.Fatalf("draft after staging: %+v", got.Draft)
	}
	types, seq := e.Events(ws.ID, seq)
	if !slices.Contains(types, "ticket_draft.updated") {
		t.Fatalf("stage events %v", types)
	}

	// Staging again replaces it, now as the teammate who did.
	e.Do("PUT", path, ws.Agent.As(ws), map[string]any{"body_html": "<p>Second take</p>"}).Want(t, 200).JSON(t, &d)
	if d.BodyText != "Second take" || d.By.Type != "user" || d.By.User == nil || d.By.User.ID != ws.Agent.ID {
		t.Fatalf("replaced draft %+v", d)
	}
	if n := countRows(t, e, `SELECT count(*) FROM ticket_drafts WHERE ticket_id = $1`, tk.ID); n != 1 {
		t.Fatalf("draft rows %d", n)
	}

	// Empty bodies, viewers and read-only keys are refused.
	e.Do("PUT", path, ws.Agent.As(ws), map[string]any{"body_html": "<p> </p>"}).Want(t, 422)
	e.Do("PUT", path, ws.Viewer.As(ws), map[string]any{"body_html": "<p>x</p>"}).Want(t, 403)
	e.Do("PUT", path, hdtest.Auth{Bearer: e.APIKey(ws, auth.ScopeTicketsRead)}, map[string]any{"body_html": "<p>x</p>"}).Want(t, 403)

	// A reply going out clears it and says so; a note does not.
	_, seq = e.Events(ws.ID, seq)
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Agent.As(ws), map[string]any{"kind": "note", "body_html": "<p>Looking</p>"}).Want(t, 201)
	e.Do("GET", path, ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &got)
	if got.Draft == nil {
		t.Fatal("a note cleared the draft")
	}
	// A customer's message leaves it for the team to adjust.
	inbound(t, e, ws, tk.ID, "Any news?")
	e.Do("GET", path, ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &got)
	if got.Draft == nil {
		t.Fatal("a customer message cleared the draft")
	}
	// A reply sent outside the composer (Ollie, a support channel thread) clears it too.
	if err := e.D.DB.Tx(e.Ctx, func(tx pgx.Tx) error {
		_, err := domain.AddMessage(e.Ctx, tx, e.D.Jobs, domain.NewMessage{WorkspaceID: ws.ID, TicketID: tk.ID, Kind: "reply",
			Direction: "outbound", AuthorType: "system", BodyHTML: "<p>Ollie here</p>", BodyText: "Ollie here", Trigger: "none"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.Do("GET", path, ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &got)
	if got.Draft != nil {
		t.Fatalf("draft after a reply from outside the composer: %+v", got.Draft)
	}
	e.Do("PUT", path, ws.Agent.As(ws), map[string]any{"body_html": "<p>Second take</p>"}).Want(t, 200)
	_, seq = e.Events(ws.ID, seq)
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Agent.As(ws), map[string]any{"kind": "reply", "body_html": "<p>Second take, edited</p>"}).Want(t, 201)
	e.Do("GET", path, ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &got)
	if got.Draft != nil {
		t.Fatalf("draft after the reply: %+v", got.Draft)
	}
	if types, _ := e.Events(ws.ID, seq); !slices.Contains(types, "ticket_draft.updated") {
		t.Fatalf("reply events %v", types)
	}

	// Discarding is idempotent.
	e.Do("PUT", path, ws.Agent.As(ws), map[string]any{"body_html": "<p>Again</p>"}).Want(t, 200)
	e.Do("DELETE", path, ws.Agent.As(ws), nil).Want(t, 204)
	e.Do("DELETE", path, ws.Agent.As(ws), nil).Want(t, 204)
	e.Do("GET", path, ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &got)
	if got.Draft != nil {
		t.Fatalf("draft after discarding: %+v", got.Draft)
	}

	// Another workspace never sees it.
	other := e.NewWorkspace()
	e.Do("GET", path, other.Owner.As(other), nil).Want(t, 404)
	e.Do("PUT", path, other.Owner.As(other), map[string]any{"body_html": "<p>x</p>"}).Want(t, 404)
}
