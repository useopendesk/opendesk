package tickets_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
)

type relationOut struct {
	ID       string `json:"id"`
	TicketID string `json:"ticket_id"`
	Related  *struct {
		ID      string `json:"id"`
		Number  int64  `json:"number"`
		Subject string `json:"subject"`
		Status  string `json:"status"`
		Channel string `json:"channel"`
	} `json:"related"`
	ActorType string `json:"actor_type"`
}

func relatedPath(tk domain.Ticket) string { return "/v1/tickets/" + tk.ID.String() + "/related" }

func listRelated(t *testing.T, e *hdtest.Env, as hdtest.Auth, tk domain.Ticket) []relationOut {
	t.Helper()
	var out struct {
		Data []relationOut `json:"data"`
	}
	e.Do("GET", relatedPath(tk), as, nil).Want(t, 200).JSON(t, &out)
	return out.Data
}

func relate(t *testing.T, e *hdtest.Env, as hdtest.Auth, from domain.Ticket, to string) relationOut {
	t.Helper()
	var r relationOut
	e.Do("POST", relatedPath(from), as, map[string]any{"ticket_id": to}).Want(t, 201).JSON(t, &r)
	return r
}

// Two tickets linked from either side show each other, keep everything of their own, and say so
// on both timelines; removing the link from the other side removes it on both.
func TestTicketRelationsShowOnBothTickets(t *testing.T) {
	e, ws := newEnv(t)
	agent, viewer := ws.Agent.As(ws), ws.Viewer.As(ws)
	mail := createTicket(t, e, ws, map[string]any{"subject": "Invoice question"})
	call := createTicket(t, e, ws, map[string]any{"subject": "Call about the invoice"})
	_, seq := e.Events(ws.ID, 0)

	e.Do("POST", relatedPath(mail), viewer, map[string]any{"ticket_id": fmt.Sprintf("#%d", call.Number)}).Want(t, 403)
	r := relate(t, e, agent, mail, fmt.Sprintf("#%d", call.Number))
	if r.TicketID != mail.ID.String() || r.Related == nil || r.Related.ID != call.ID.String() || r.Related.Number != call.Number ||
		r.Related.Subject != "Call about the invoice" || r.Related.Status != "open" || r.Related.Channel != string(call.Channel) || r.ActorType != "user" {
		t.Fatalf("relation = %+v", r)
	}
	types, _ := e.Events(ws.ID, seq)
	if n := countOf(types, "ticket_relation.created"); n != 2 {
		t.Fatalf("ticket_relation.created published %d times, want once per ticket: %v", n, types)
	}

	fromMail, fromCall := listRelated(t, e, viewer, mail), listRelated(t, e, viewer, call)
	if len(fromMail) != 1 || fromMail[0].Related.ID != call.ID.String() || len(fromCall) != 1 || fromCall[0].Related.ID != mail.ID.String() {
		t.Fatalf("mail sees %+v, call sees %+v", fromMail, fromCall)
	}
	if fromMail[0].ID != fromCall[0].ID {
		t.Fatalf("one relation read from both sides, got %s and %s", fromMail[0].ID, fromCall[0].ID)
	}
	if tk := getTicket(t, e, ws, call.ID); tk.Status != "open" || tk.Subject != "Call about the invoice" {
		t.Fatalf("the linked ticket changed: %+v", tk)
	}
	for _, id := range []uuid.UUID{mail.ID, call.ID} {
		if n := countRows(t, e, `SELECT count(*) FROM activities WHERE ticket_id = $1 AND type = 'linked_ticket'`, id); n != 1 {
			t.Fatalf("ticket %s has %d linked_ticket activities", id, n)
		}
	}
	var named *string
	e.Scalar(`SELECT data->>'subject' FROM activities WHERE ticket_id = $1 AND type = 'linked_ticket'`, &named, mail.ID)
	if named == nil || *named != "Call about the invoice" {
		t.Fatalf("the timeline names %v", named)
	}

	// The same pair again, from the other side, answers the relation that is there.
	again := relate(t, e, agent, call, mail.ID.String())
	if again.ID != r.ID || countRows(t, e, `SELECT count(*) FROM ticket_relations WHERE workspace_id = $1`, ws.ID) != 1 {
		t.Fatalf("second link %+v", again)
	}
	e.Do("POST", relatedPath(mail), agent, map[string]any{"ticket_id": mail.ID.String()}).Want(t, 422)
	e.Do("POST", relatedPath(mail), agent, map[string]any{"ticket_id": "#999999"}).Want(t, 404)

	_, seq = e.Events(ws.ID, 0)
	e.Do("DELETE", relatedPath(call)+"/"+r.ID, agent, nil).Want(t, 204)
	if len(listRelated(t, e, agent, mail)) != 0 || len(listRelated(t, e, agent, call)) != 0 {
		t.Fatal("the relation is still listed after removing it")
	}
	for _, id := range []uuid.UUID{mail.ID, call.ID} {
		if n := countRows(t, e, `SELECT count(*) FROM activities WHERE ticket_id = $1 AND type = 'unlinked_ticket'`, id); n != 1 {
			t.Fatalf("ticket %s has %d unlinked_ticket activities", id, n)
		}
	}
	if types, _ = e.Events(ws.ID, seq); countOf(types, "ticket_relation.deleted") != 2 {
		t.Fatalf("events after removing: %v", types)
	}
	e.Do("DELETE", relatedPath(call)+"/"+r.ID, agent, nil).Want(t, 404)
}

// A ticket in a private team is linked only by someone who sees it, shows to everyone else as
// a private ticket with no subject, and is not named on the other ticket's timeline.
func TestTicketRelationsFollowVisibility(t *testing.T) {
	e, ws := newEnv(t)
	hidden, _ := privateTicket(t, e, ws)
	open := createTicket(t, e, ws, map[string]any{"subject": "Public ticket"})

	e.Do("POST", relatedPath(open), ws.Agent.As(ws), map[string]any{"ticket_id": hidden.ID.String()}).Want(t, 404)
	r := relate(t, e, ws.Admin.As(ws), open, hidden.ID.String())
	if r.Related == nil || r.Related.Subject != hidden.Subject {
		t.Fatalf("a member of the team sees %+v", r)
	}

	outsider := listRelated(t, e, ws.Agent.As(ws), open)
	if len(outsider) != 1 || outsider[0].Related != nil {
		t.Fatalf("an outsider sees %+v, want a private ticket without details", outsider)
	}
	e.Do("GET", relatedPath(hidden), ws.Agent.As(ws), nil).Want(t, 404)
	e.Do("DELETE", relatedPath(open)+"/"+r.ID, ws.Agent.As(ws), nil).Want(t, 204)

	var subject, number *string
	e.Scalar(`SELECT data->>'subject' FROM activities WHERE ticket_id = $1 AND type = 'linked_ticket'`, &subject, open.ID)
	e.Scalar(`SELECT data->>'number' FROM activities WHERE ticket_id = $1 AND type = 'linked_ticket'`, &number, open.ID)
	if subject != nil || number != nil {
		t.Fatalf("the public ticket's timeline names a private ticket: %v %v", subject, number)
	}
}

// A merged ticket's links move to the ticket it went into, except the one between the two and
// ones the target has already.
func TestTicketRelationsFollowAMerge(t *testing.T) {
	e, ws := newEnv(t)
	agent := ws.Agent.As(ws)
	src := createTicket(t, e, ws, map[string]any{"subject": "Source"})
	dst := createTicket(t, e, ws, map[string]any{"subject": "Target"})
	shared := createTicket(t, e, ws, map[string]any{"subject": "Shared"})
	only := createTicket(t, e, ws, map[string]any{"subject": "Only the source's"})
	relate(t, e, agent, src, dst.ID.String())
	relate(t, e, agent, src, shared.ID.String())
	relate(t, e, agent, only, src.ID.String())
	relate(t, e, agent, shared, dst.ID.String())

	e.Do("POST", "/v1/tickets/"+src.ID.String()+"/merge", agent, map[string]any{"into_id": dst.ID.String()}).Want(t, 200)

	if got := listRelated(t, e, agent, src); len(got) != 0 {
		t.Fatalf("the merged ticket still has %+v", got)
	}
	var ids []string
	for _, r := range listRelated(t, e, agent, dst) {
		ids = append(ids, r.Related.ID)
	}
	slices.Sort(ids)
	want := []string{only.ID.String(), shared.ID.String()}
	slices.Sort(want)
	if !slices.Equal(ids, want) {
		t.Fatalf("the target is related to %v, want %v", ids, want)
	}
	if n := countRows(t, e, `SELECT count(*) FROM ticket_relations WHERE workspace_id = $1 AND ticket_id = related_ticket_id`, ws.ID); n != 0 {
		t.Fatalf("%d relations point a ticket at itself", n)
	}
}

// A deleted ticket's links are hidden and come back when it is restored.
func TestTicketRelationsHideADeletedTicket(t *testing.T) {
	e, ws := newEnv(t)
	agent := ws.Agent.As(ws)
	a := createTicket(t, e, ws, map[string]any{"subject": "A"})
	b := createTicket(t, e, ws, map[string]any{"subject": "B"})
	relate(t, e, agent, a, b.ID.String())

	e.Do("DELETE", "/v1/tickets/"+b.ID.String(), ws.Admin.As(ws), nil).Want(t, 204)
	if got := listRelated(t, e, agent, a); len(got) != 0 {
		t.Fatalf("a deleted ticket is listed: %+v", got)
	}
	e.Do("POST", "/v1/tickets/"+b.ID.String()+"/restore", ws.Admin.As(ws), nil).Want(t, 200)
	if got := listRelated(t, e, agent, a); len(got) != 1 || got[0].Related.ID != b.ID.String() {
		t.Fatalf("after the restore: %+v", got)
	}
}

// The 50 links a ticket may have count the links to other tools and to other tickets together.
func TestTicketRelationsCountTowardsTheLinkLimit(t *testing.T) {
	e, ws := newEnv(t)
	agent := ws.Agent.As(ws)
	a := createTicket(t, e, ws, map[string]any{"subject": "A"})
	first := createTicket(t, e, ws, map[string]any{"subject": "First"})
	second := createTicket(t, e, ws, map[string]any{"subject": "Second"})
	e.Exec(`INSERT INTO ticket_links (workspace_id, ticket_id, provider, url, title)
		SELECT $1, $2, 'link', 'https://example.com/' || n, 'Item ' || n FROM generate_series(1, 49) n`, ws.ID, a.ID)

	relate(t, e, agent, a, first.ID.String())
	e.Do("POST", relatedPath(a), agent, map[string]any{"ticket_id": second.ID.String()}).Want(t, 409)
}

func countOf(types []string, want string) int {
	n := 0
	for _, typ := range types {
		if typ == want {
			n++
		}
	}
	return n
}
