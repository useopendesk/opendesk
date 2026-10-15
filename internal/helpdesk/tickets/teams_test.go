package tickets_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/ids"
	"github.com/useopendesk/opendesk/internal/jobs"
)

// addPrivateTeam adds a private team with members as (user, role) pairs.
func addPrivateTeam(t *testing.T, e *hdtest.Env, ws hdtest.WS, members map[uuid.UUID]string) uuid.UUID {
	t.Helper()
	id := ids.New()
	e.Exec(`INSERT INTO teams (id, workspace_id, name, private) VALUES ($1, $2, $3, true)`, id, ws.ID, "Private "+id.String()[24:])
	for uid, role := range members {
		e.Exec(`INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)`, id, uid, role)
	}
	return id
}

func ticketIDs(t *testing.T, r *hdtest.Resp) []string {
	t.Helper()
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	r.JSON(t, &page)
	out := []string{}
	for _, d := range page.Data {
		out = append(out, d.ID)
	}
	return out
}

func errCode(t *testing.T, r *hdtest.Resp) string {
	t.Helper()
	c, _ := r.Map(t)["code"].(string)
	return c
}

func TestPrivateTeamVisibility(t *testing.T) {
	e, ws := newEnv(t)
	lead := e.AddUser(ws, auth.RoleAgent, "Lena Lead")
	priv := addPrivateTeam(t, e, ws, map[uuid.UUID]string{lead.ID: hd.TeamLead, ws.Viewer.ID: hd.TeamObserver})

	var tk domain.Ticket
	e.Do("POST", "/v1/tickets", ws.Admin.As(ws), map[string]any{"subject": "Breach", "body_html": "<p>x</p>", "kind": "note",
		"team_id": priv.String()}).Want(t, 201).JSON(t, &tk)
	public := createTicket(t, e, ws, map[string]any{"kind": "note"})
	path := "/v1/tickets/" + tk.ID.String()

	// Outsiders get the same 404 as for a missing ticket, everywhere.
	e.Do("GET", path, ws.Agent.As(ws), nil).Want(t, 404)
	e.Do("PATCH", path, ws.Agent.As(ws), map[string]any{"priority": "high"}).Want(t, 404)
	e.Do("GET", path+"/messages", ws.Agent.As(ws), nil).Want(t, 404)
	e.Do("POST", path+"/messages", ws.Agent.As(ws), map[string]any{"kind": "note", "body_html": "<p>hi</p>"}).Want(t, 404)
	if got := ticketIDs(t, e.Do("GET", "/v1/tickets", ws.Agent.As(ws), nil).Want(t, 200)); slices.Contains(got, tk.ID.String()) ||
		!slices.Contains(got, public.ID.String()) {
		t.Fatalf("outsider list %v", got)
	}
	// Members and observers see it; admins too, once they join.
	e.Do("GET", path, lead.As(ws), nil).Want(t, 200)
	e.Do("GET", path, ws.Viewer.As(ws), nil).Want(t, 200)
	e.Do("GET", path, ws.Owner.As(ws), nil).Want(t, 404)
	e.Exec(`INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, 'observer')`, priv, ws.Owner.ID)
	e.Do("GET", path, ws.Owner.As(ws), nil).Want(t, 200)

	// team_id=mine and none.
	if got := ticketIDs(t, e.Do("GET", "/v1/tickets?team_id=mine", lead.As(ws), nil).Want(t, 200)); len(got) != 1 || got[0] != tk.ID.String() {
		t.Fatalf("mine %v", got)
	}
	if got := ticketIDs(t, e.Do("GET", "/v1/tickets?team_id=none", lead.As(ws), nil).Want(t, 200)); slices.Contains(got, tk.ID.String()) ||
		!slices.Contains(got, public.ID.String()) {
		t.Fatalf("none %v", got)
	}

	// Counts: teams lists only visible teams; my_teams counts the caller's.
	var counts struct {
		Teams   map[string]struct{ Open, Unassigned int } `json:"teams"`
		MyTeams struct{ Open, Unassigned int }            `json:"my_teams"`
	}
	e.Do("GET", "/v1/tickets/counts", ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &counts)
	if _, ok := counts.Teams[priv.String()]; ok {
		t.Fatal("outsider counted a private team")
	}
	e.Do("GET", "/v1/tickets/counts", lead.As(ws), nil).Want(t, 200).JSON(t, &counts)
	if c := counts.Teams[priv.String()]; c.Open != 1 || c.Unassigned != 1 || counts.MyTeams.Open != 1 {
		t.Fatalf("lead counts %+v", counts)
	}

	// Only people on the team can be assigned its tickets.
	r := e.Do("PATCH", path, lead.As(ws), map[string]any{"assignee_id": ws.Agent.ID.String()}).Want(t, 422)
	if errCode(t, r) != "not_on_team" {
		t.Fatalf("code %s", r.Body)
	}
	e.Do("PATCH", path, lead.As(ws), map[string]any{"assignee_id": lead.ID.String()}).Want(t, 200)
	r = e.Do("POST", "/v1/tickets/bulk", lead.As(ws), map[string]any{"ids": []string{tk.ID.String()},
		"patch": map[string]any{"assignee_id": ws.Agent.ID.String()}}).Want(t, 422)
	if errCode(t, r) != "not_on_team" {
		t.Fatalf("bulk code %s", r.Body)
	}

	// Moving a ticket into the private team unassigns an outsider and hides it from others.
	e.Do("PATCH", "/v1/tickets/"+public.ID.String(), ws.Admin.As(ws), map[string]any{"assignee_id": ws.Agent.ID.String()}).Want(t, 200)
	_, seq := e.Events(ws.ID, 0)
	var moved domain.Ticket
	e.Do("PATCH", "/v1/tickets/"+public.ID.String(), lead.As(ws), map[string]any{"team_id": priv.String()}).Want(t, 200).JSON(t, &moved)
	if moved.Assignee != nil {
		t.Fatal("outsider kept the ticket after it moved to a private team")
	}
	if moved.Routed == nil || moved.Routed.Via != "agent" {
		t.Fatalf("routed %+v", moved.Routed)
	}
	types, _ := e.Events(ws.ID, seq)
	if !slices.Contains(types, "ticket.hidden") {
		t.Fatalf("events %v", types)
	}
	e.Do("GET", "/v1/tickets/"+public.ID.String(), ws.Agent.As(ws), nil).Want(t, 404)

	// Notifications skip people who cannot see the ticket.
	err := e.D.DB.Tx(e.Ctx, func(tx pgx.Tx) error {
		if err := hd.Notify(e.Ctx, tx, ws.ID, ws.Agent.ID, hd.NotifyMentioned, &tk.ID, nil); err != nil {
			return err
		}
		return hd.Notify(e.Ctx, tx, ws.ID, lead.ID, hd.NotifyMentioned, &tk.ID, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, e, `SELECT count(*) FROM notifications WHERE ticket_id = $1 AND user_id = $2`, tk.ID, ws.Agent.ID); n != 0 {
		t.Fatal("outsider notified about a private ticket")
	}
	if n := countRows(t, e, `SELECT count(*) FROM notifications WHERE ticket_id = $1 AND user_id = $2`, tk.ID, lead.ID); n != 1 {
		t.Fatal("member not notified")
	}
}

func TestReplyAddressFollowsTeam(t *testing.T) {
	e, ws := newEnv(t)
	sales, support := addTeam(t, e, ws, ws.Agent.ID), addTeam(t, e, ws, ws.Agent.ID)
	mbSales, mbSupport := addMailbox(t, e, ws), addMailbox(t, e, ws)
	e.Exec(`UPDATE mailboxes SET team_id = $2 WHERE id = $1`, mbSales, sales)
	e.Exec(`UPDATE mailboxes SET team_id = $2 WHERE id = $1`, mbSupport, support)

	tk := createTicket(t, e, ws, map[string]any{"kind": "note", "mailbox_id": mbSales.String()})
	if tk.TeamID == nil || *tk.TeamID != sales {
		t.Fatalf("not routed to the address's team: %v", tk.TeamID)
	}
	var got domain.Ticket
	e.Do("PATCH", "/v1/tickets/"+tk.ID.String(), ws.Agent.As(ws), map[string]any{"team_id": support.String()}).Want(t, 200).JSON(t, &got)
	if got.MailboxID == nil || *got.MailboxID != mbSupport {
		t.Fatalf("reply address %v, want the new team's", got.MailboxID)
	}
	var from string
	e.Scalar(`SELECT data->>'mailbox_from' FROM activities WHERE ticket_id = $1 AND type = 'team_changed'
		ORDER BY created_at DESC LIMIT 1`, &from, tk.ID)
	if from != mbSales.String() {
		t.Fatalf("team_changed mailbox_from %q", from)
	}

	// Addresses of a private team are only for its members.
	priv := addPrivateTeam(t, e, ws, map[uuid.UUID]string{ws.Admin.ID: hd.TeamLead})
	mbPriv := addMailbox(t, e, ws)
	e.Exec(`UPDATE mailboxes SET team_id = $2 WHERE id = $1`, mbPriv, priv)
	e.Do("PATCH", "/v1/tickets/"+tk.ID.String(), ws.Agent.As(ws), map[string]any{"mailbox_id": mbPriv.String()}).Want(t, 403)
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Agent.As(ws),
		map[string]any{"kind": "reply", "body_html": "<p>hi</p>", "mailbox_id": mbPriv.String()}).Want(t, 403)
	// An address of another brand is refused.
	brand2 := ids.New()
	e.Exec(`INSERT INTO brands (id, workspace_id, name, slug) VALUES ($1, $2, 'Other', $3)`, brand2, ws.ID, "b-"+brand2.String()[24:])
	mbOther := addMailbox(t, e, ws)
	e.Exec(`UPDATE mailboxes SET brand_id = $2 WHERE id = $1`, mbOther, brand2)
	e.Do("PATCH", "/v1/tickets/"+tk.ID.String(), ws.Agent.As(ws), map[string]any{"mailbox_id": mbOther.String()}).Want(t, 422)
	// Switching the brand switches to its address.
	e.Do("PATCH", "/v1/tickets/"+tk.ID.String(), ws.Agent.As(ws), map[string]any{"brand_id": brand2.String()}).Want(t, 200).JSON(t, &got)
	if got.BrandID != brand2 || got.MailboxID == nil || *got.MailboxID != mbOther {
		t.Fatalf("brand switch %v %v", got.BrandID, got.MailboxID)
	}
	if got := ticketIDs(t, e.Do("GET", "/v1/tickets?brand_id="+brand2.String(), ws.Agent.As(ws), nil).Want(t, 200)); len(got) != 1 {
		t.Fatalf("brand filter %v", got)
	}
}

func TestTeamAutomationsAndPool(t *testing.T) {
	e, ws := newEnv(t)
	support := addTeam(t, e, ws)
	e.Exec(`INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, 'observer'), ($1, $3, 'member')`, support, ws.Admin.ID, ws.Agent.ID)
	mb, mb2 := addMailbox(t, e, ws), addMailbox(t, e, ws)
	addAutomation(t, e, ws, jobs.TriggerTicketCreated, 0,
		[]map[string]string{{"field": "brand_id", "op": "equals", "value": ws.BrandID.String()}, {"field": "team_id", "op": "equals", "value": "none"}},
		[]map[string]string{{"type": "assign_team", "value": support.String()}, {"type": "set_reply_address", "value": mb2.String()}})
	e.SetSettings(ws, map[string]any{"auto_assign": "round_robin"})

	for i := 0; i < 3; i++ {
		tk := createTicket(t, e, ws, map[string]any{"kind": "note", "mailbox_id": mb.String()})
		runTriggers(t, e, ws.ID)
		got := getTicket(t, e, ws, tk.ID)
		if got.TeamID == nil || *got.TeamID != support {
			t.Fatalf("team %v", got.TeamID)
		}
		if got.MailboxID == nil || *got.MailboxID != mb2 {
			t.Fatalf("reply address %v", got.MailboxID)
		}
		if got.Routed == nil || got.Routed.Via != "automation" {
			t.Fatalf("routed %+v", got.Routed)
		}
		// Observers are never auto-assigned.
		if got.Assignee == nil || got.Assignee.ID != ws.Agent.ID {
			t.Fatalf("assignee %+v", got.Assignee)
		}
	}
}

func TestAIHandoffNotifiesTeamWithoutObservers(t *testing.T) {
	e, ws := newEnv(t)
	lead := e.AddUser(ws, auth.RoleAgent, "Lena Lead")
	member := e.AddUser(ws, auth.RoleAgent, "Max Member")
	observer := e.AddUser(ws, auth.RoleAdmin, "Olga Observer")
	priv := addPrivateTeam(t, e, ws, map[uuid.UUID]string{lead.ID: hd.TeamLead, member.ID: hd.TeamMember, observer.ID: hd.TeamObserver})
	var tk domain.Ticket
	e.Do("POST", "/v1/tickets", lead.As(ws), map[string]any{"subject": "Chat", "body_html": "<p>x</p>", "kind": "note",
		"team_id": priv.String()}).Want(t, 201).JSON(t, &tk)
	e.Exec(`UPDATE tickets SET assignee_id = NULL WHERE id = $1`, tk.ID)
	if err := e.D.DB.Tx(e.Ctx, func(tx pgx.Tx) error {
		_, err := hd.MarkAIHandoff(e.Ctx, tx, ws.ID, tk.ID, "contact", hd.HandoffBot{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var got []uuid.UUID
	e.Scalar(`SELECT COALESCE(array_agg(user_id ORDER BY user_id), '{}') FROM notifications WHERE ticket_id = $1 AND type = 'ai_handoff'`, &got, tk.ID)
	want := []uuid.UUID{lead.ID, member.ID}
	slices.SortFunc(want, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	if !slices.Equal(got, want) {
		t.Fatalf("handoff notified %v, want the lead and member %v", got, want)
	}
}
