package tickets_test

import (
	"testing"

	"github.com/useopendesk/opendesk/internal/domain"
)

func TestTicketAliasUpdateSearchAndLookup(t *testing.T) {
	e, ws := newEnv(t)
	agent := ws.Agent.As(ws)
	first := createTicket(t, e, ws, map[string]any{"subject": "Shipping update"})
	second := createTicket(t, e, ws, map[string]any{"subject": "Another ticket"})

	e.Do("PATCH", "/v1/tickets/"+first.ID.String(), agent, map[string]any{"alias": " order-42 "}).Want(t, 200)
	if got := getTicket(t, e, ws, first.ID).Alias; got == nil || *got != "ORDER-42" {
		t.Fatalf("alias = %v, want ORDER-42", got)
	}
	e.Do("PATCH", "/v1/tickets/"+second.ID.String(), agent, map[string]any{"alias": "order-42"}).Want(t, 409)
	e.Do("PATCH", "/v1/tickets/"+second.ID.String(), agent, map[string]any{"alias": "42"}).Want(t, 422)
	e.Do("PATCH", "/v1/tickets/"+second.ID.String(), agent, map[string]any{"alias": "order 42"}).Want(t, 422)

	if got := list(t, e, agent, "q=order-42"); len(got.Data) != 1 || got.Data[0].ID != first.ID {
		t.Fatalf("alias search returned %v", idsOf(got))
	}
	var found domain.Ticket
	e.Do("GET", "/v1/tickets/order-42", ws.Viewer.As(ws), nil).Want(t, 200).JSON(t, &found)
	if found.ID != first.ID {
		t.Fatalf("alias lookup returned %s, want %s", found.ID, first.ID)
	}

	e.Do("PATCH", "/v1/tickets/"+first.ID.String(), agent, map[string]any{"alias": nil}).Want(t, 200)
	if got := getTicket(t, e, ws, first.ID).Alias; got != nil {
		t.Fatalf("cleared alias = %v", *got)
	}
}

func TestTicketAliasLookupKeepsPrivateTeamVisibility(t *testing.T) {
	e, ws := newFullEnv(t)
	private := addTeam(t, e, ws, ws.Agent.ID)
	e.Exec(`UPDATE teams SET private = true WHERE id = $1`, private)
	secret := createTicket(t, e, ws, map[string]any{"subject": "Private case", "team_id": private.String()})
	e.Do("PATCH", "/v1/tickets/"+secret.ID.String(), ws.Agent.As(ws), map[string]any{"alias": "PRIVATE-42"}).Want(t, 200)
	e.Do("GET", "/v1/tickets/private-42", ws.Viewer.As(ws), nil).Want(t, 404)
	page := list(t, e, ws.Viewer.As(ws), "q=private-42")
	if len(page.Data) != 0 {
		t.Fatalf("hidden alias appeared in search: %v", idsOf(page))
	}
}
