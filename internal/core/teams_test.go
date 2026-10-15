package core

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// teamFixture is a workspace with an owner, an agent, a second agent and a viewer.
type teamFixture struct {
	e                         *testEnv
	wid                       uuid.UUID
	owner, agent, other, view *client
	ownerID, agentID, otherID uuid.UUID
	viewID                    uuid.UUID
	brand                     uuid.UUID
}

func newTeamFixture(t *testing.T) *teamFixture {
	e := newEnv(t)
	f := &teamFixture{e: e}
	var ownerEmail string
	f.owner, ownerEmail = e.signup("teamowner")
	f.wid = uuid.MustParse(f.owner.newWorkspace("Teams Co"))
	f.ownerID = e.userID(ownerEmail)
	f.agent, f.agentID = e.memberOf(f.wid, "teamagent", "agent")
	f.other, f.otherID = e.memberOf(f.wid, "teamother", "agent")
	f.view, f.viewID = e.memberOf(f.wid, "teamviewer", "viewer")
	if err := e.d.DB.Pool.QueryRow(context.Background(), `SELECT id FROM brands WHERE workspace_id = $1 AND is_default`, f.wid).Scan(&f.brand); err != nil {
		t.Fatalf("default brand: %v", err)
	}
	return f
}

func (e *testEnv) userID(email string) uuid.UUID {
	e.t.Helper()
	var id uuid.UUID
	if err := e.d.DB.Pool.QueryRow(context.Background(), `SELECT id FROM users WHERE email = $1`, email).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// memberOf signs up a user and adds them to the workspace with role.
func (e *testEnv) memberOf(wid uuid.UUID, tag, role string) (*client, uuid.UUID) {
	e.t.Helper()
	c, email := e.signup(tag)
	c.workspace = wid.String()
	id := e.userID(email)
	if _, err := e.d.DB.Pool.Exec(context.Background(), `INSERT INTO members (workspace_id, user_id, role) VALUES ($1, $2, $3)`, wid, id, role); err != nil {
		e.t.Fatal(err)
	}
	return c, id
}

func (f *teamFixture) exec(sql string, args ...any) {
	f.e.t.Helper()
	if _, err := f.e.d.DB.Pool.Exec(context.Background(), sql, args...); err != nil {
		f.e.t.Fatalf("%s: %v", sql, err)
	}
}

func (f *teamFixture) scalar(sql string, dst any, args ...any) {
	f.e.t.Helper()
	if err := f.e.d.DB.Pool.QueryRow(context.Background(), sql, args...).Scan(dst); err != nil {
		f.e.t.Fatalf("%s: %v", sql, err)
	}
}

// ticket inserts an open ticket of team (nil for none).
func (f *teamFixture) ticket(team *uuid.UUID, assignee *uuid.UUID) uuid.UUID {
	id := uuid.New()
	f.exec(`INSERT INTO tickets (id, workspace_id, number, subject, channel, team_id, assignee_id, brand_id, last_message_at)
		VALUES ($1, $2, (SELECT COALESCE(max(number), 0) + 1 FROM tickets WHERE workspace_id = $2), 'Help', 'api', $3, $4, $5, now())`,
		id, f.wid, team, assignee, f.brand)
	return id
}

func members(r resp) map[string]string {
	out := map[string]string{}
	list, _ := r.Body["members"].([]any)
	for _, v := range list {
		m, _ := v.(map[string]any)
		id, _ := m["user_id"].(string)
		role, _ := m["role"].(string)
		out[id] = role
	}
	return out
}

func TestTeamCreateAndUpdate(t *testing.T) {
	f := newTeamFixture(t)

	// Only admins create teams.
	if r := f.agent.do("POST", "/v1/teams", map[string]any{"name": "Nope"}); r.Status != 403 {
		t.Fatalf("agent created a team: %d", r.Status)
	}
	// Viewers can only observe.
	if r := f.owner.do("POST", "/v1/teams", map[string]any{"name": "Billing", "member_ids": []string{f.viewID.String()}}); r.Status != 422 {
		t.Fatalf("viewer joined as member: %d %s", r.Status, r.Raw)
	}
	// Invalid business hours are refused.
	if r := f.owner.do("POST", "/v1/teams", map[string]any{"name": "Billing",
		"business_hours": map[string]any{"enabled": true, "timezone": "Mars/Base", "schedule": map[string]any{}}}); r.Status != 422 {
		t.Fatalf("bad timezone: %d %s", r.Status, r.Raw)
	}

	r := f.owner.do("POST", "/v1/teams", map[string]any{
		"name": "Billing", "description": "Invoices and refunds", "color": "teal",
		"members":        []map[string]any{{"user_id": f.agentID, "role": "lead"}, {"user_id": f.viewID, "role": "observer"}},
		"member_ids":     []string{f.otherID.String()},
		"signature_html": `<p>Billing team</p><script>alert(1)</script>`,
		"business_hours": map[string]any{"enabled": true, "timezone": "Europe/Budapest",
			"schedule": map[string]any{"mon": []map[string]string{{"start": "09:00", "end": "17:00"}}}},
		"assign_mode": "round_robin",
	})
	if r.Status != 201 {
		t.Fatalf("create: %d %s", r.Status, r.Raw)
	}
	team := r.str("id")
	m := members(r)
	if m[f.agentID.String()] != "lead" || m[f.otherID.String()] != "member" || m[f.viewID.String()] != "observer" {
		t.Fatalf("members %v", m)
	}
	if ids, _ := r.Body["member_ids"].([]any); len(ids) != 3 {
		t.Fatalf("member_ids %v", r.Body["member_ids"])
	}
	if sig := r.str("signature_html"); strings.Contains(sig, "script") || !strings.Contains(sig, "Billing team") {
		t.Fatalf("signature %q", sig)
	}
	if r.str("assign_mode") != "round_robin" || r.str("color") != "teal" {
		t.Fatalf("team %s", r.Raw)
	}
	if bh, _ := r.Body["business_hours"].(map[string]any); bh == nil || bh["timezone"] != "Europe/Budapest" {
		t.Fatalf("business_hours %v", r.Body["business_hours"])
	}
	if f.e.auditCount(f.wid.String(), "team.created", "") != 1 {
		t.Fatal("team.created not audited")
	}

	// A lead edits the team, but not its privacy, and cannot leave it without a lead.
	if r := f.agent.do("PATCH", "/v1/teams/"+team, map[string]any{"description": "Money things"}); r.Status != 200 || r.str("description") != "Money things" {
		t.Fatalf("lead edit: %d %s", r.Status, r.Raw)
	}
	if r := f.agent.do("PATCH", "/v1/teams/"+team, map[string]any{"private": true}); r.Status != 403 {
		t.Fatalf("lead made the team private: %d", r.Status)
	}
	if r := f.agent.do("PATCH", "/v1/teams/"+team, map[string]any{"members": []map[string]any{
		{"user_id": f.agentID, "role": "member"}, {"user_id": f.otherID, "role": "member"}}}); r.Status != 409 || r.str("code") != "last_lead" {
		t.Fatalf("lead left no lead: %d %s", r.Status, r.Raw)
	}
	// A plain member cannot edit it.
	if r := f.other.do("PATCH", "/v1/teams/"+team, map[string]any{"description": "x"}); r.Status != 403 {
		t.Fatalf("member edited: %d", r.Status)
	}
	// Nulls reset to the workspace defaults.
	r = f.owner.do("PATCH", "/v1/teams/"+team, map[string]any{"business_hours": nil, "assign_mode": nil})
	if r.Status != 200 || r.Body["business_hours"] != nil || r.Body["assign_mode"] != nil {
		t.Fatalf("reset: %d %s", r.Status, r.Raw)
	}
	var rules int
	f.scalar(`SELECT count(*) FROM assignment_rules WHERE team_id = $1`, &rules, uuid.MustParse(team))
	if rules != 0 {
		t.Fatal("assignment rule kept after assign_mode null")
	}

	// The 200-team limit.
	f.exec(`INSERT INTO teams (workspace_id, name) SELECT $1, 'Bulk ' || g FROM generate_series(1, 199) g`, f.wid)
	if r := f.owner.do("POST", "/v1/teams", map[string]any{"name": "One too many"}); r.Status != 422 || r.str("code") != "limit_reached" {
		t.Fatalf("limit: %d %s", r.Status, r.Raw)
	}
}

func TestPrivateTeamMembership(t *testing.T) {
	f := newTeamFixture(t)
	r := f.owner.do("POST", "/v1/teams", map[string]any{"name": "Security", "private": true,
		"members": []map[string]any{{"user_id": f.agentID, "role": "lead"}}})
	if r.Status != 201 {
		t.Fatalf("create: %d %s", r.Status, r.Raw)
	}
	team := r.str("id")

	// Members are listed to members and admins only.
	if r := f.other.do("GET", "/v1/teams/"+team, nil); r.Status != 200 || len(members(r)) != 0 {
		t.Fatalf("outsider saw members: %d %s", r.Status, r.Raw)
	}
	if r := f.agent.do("GET", "/v1/teams/"+team, nil); len(members(r)) != 1 {
		t.Fatalf("member did not see members: %s", r.Raw)
	}
	if r := f.owner.do("GET", "/v1/teams/"+team, nil); len(members(r)) != 1 {
		t.Fatalf("admin did not see members: %s", r.Raw)
	}

	// Joining is for admins; it makes them an observer and tells the leads.
	if r := f.other.do("POST", "/v1/teams/"+team+"/join", nil); r.Status != 403 {
		t.Fatalf("agent joined: %d", r.Status)
	}
	r = f.owner.do("POST", "/v1/teams/"+team+"/join", nil)
	if r.Status != 200 || members(r)[f.ownerID.String()] != "observer" {
		t.Fatalf("join: %d %s", r.Status, r.Raw)
	}
	var n int
	f.scalar(`SELECT count(*) FROM notifications WHERE workspace_id = $1 AND user_id = $2 AND type = 'team_joined'`, &n, f.wid, f.agentID)
	if n != 1 {
		t.Fatalf("lead notifications %d", n)
	}
	// The leads learn who joined and that the team is private, so the join is about seeing its tickets.
	var private bool
	var joiner string
	f.scalar(`SELECT (data->'team'->>'private')::boolean FROM notifications WHERE workspace_id = $1 AND user_id = $2 AND type = 'team_joined'`, &private, f.wid, f.agentID)
	f.scalar(`SELECT data->'user'->>'id' FROM notifications WHERE workspace_id = $1 AND user_id = $2 AND type = 'team_joined'`, &joiner, f.wid, f.agentID)
	if !private || joiner != f.ownerID.String() {
		t.Fatalf("team_joined data: private %v, user %s", private, joiner)
	}
	if f.e.auditCount(f.wid.String(), "team.joined", "") != 1 {
		t.Fatal("team.joined not audited")
	}
	// Joining again changes nothing.
	if r := f.owner.do("POST", "/v1/teams/"+team+"/join", nil); r.Status != 200 {
		t.Fatalf("join again: %d", r.Status)
	}
	f.scalar(`SELECT count(*) FROM notifications WHERE workspace_id = $1 AND user_id = $2 AND type = 'team_joined'`, &n, f.wid, f.agentID)
	if n != 1 {
		t.Fatal("joining twice notified twice")
	}

	// The last lead of a team with others cannot leave; others can.
	if r := f.agent.do("POST", "/v1/teams/"+team+"/leave", nil); r.Status != 409 || r.str("code") != "last_lead" {
		t.Fatalf("last lead left: %d %s", r.Status, r.Raw)
	}
	if r := f.owner.do("POST", "/v1/teams/"+team+"/leave", nil); r.Status != 204 {
		t.Fatalf("leave: %d %s", r.Status, r.Raw)
	}
	if r := f.owner.do("POST", "/v1/teams/"+team+"/leave", nil); r.Status != 204 {
		t.Fatalf("leave again: %d", r.Status)
	}

	// Making the team public tells clients about its tickets; private again hides them.
	tk := f.ticket(ptr(uuid.MustParse(team)), nil)
	if r := f.owner.do("PATCH", "/v1/teams/"+team, map[string]any{"private": false}); r.Status != 200 {
		t.Fatalf("public: %d %s", r.Status, r.Raw)
	}
	if r := f.owner.do("PATCH", "/v1/teams/"+team, map[string]any{"private": true}); r.Status != 200 {
		t.Fatalf("private: %d %s", r.Status, r.Raw)
	}
	f.scalar(`SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'ticket.hidden' AND data->>'ticket_id' = $2`, &n, f.wid, tk.String())
	if n != 1 {
		t.Fatalf("ticket.hidden events %d", n)
	}
	if f.e.auditCount(f.wid.String(), "team.privacy_changed", "") != 2 {
		t.Fatal("privacy changes not audited")
	}
	// team.* events never carry a private team's members.
	var leaked int
	f.scalar(`SELECT count(*) FROM events WHERE workspace_id = $1 AND type LIKE 'team.%' AND (data->'team'->>'private')::boolean
		AND jsonb_array_length(data->'team'->'members') > 0`, &leaked, f.wid)
	if leaked != 0 {
		t.Fatalf("%d team events listed private members", leaked)
	}
}

func TestDeleteTeamMovesWork(t *testing.T) {
	f := newTeamFixture(t)
	mk := func(name string, private bool) uuid.UUID {
		r := f.owner.do("POST", "/v1/teams", map[string]any{"name": name, "private": private,
			"members": []map[string]any{{"user_id": f.agentID, "role": "member"}}})
		if r.Status != 201 {
			t.Fatalf("create %s: %d %s", name, r.Status, r.Raw)
		}
		return uuid.MustParse(r.str("id"))
	}
	src, dst := mk("Tier 1", false), mk("Tier 2", true)
	open := f.ticket(&src, &f.otherID) // other is not on the private destination
	solved := f.ticket(&src, nil)
	f.exec(`UPDATE tickets SET status = 'solved' WHERE id = $1`, solved)
	mb := uuid.New()
	f.exec(`INSERT INTO mailboxes (id, workspace_id, name, email, inbound_kind, forwarding_token, outbound_kind, brand_id, team_id)
		VALUES ($1, $2, 'Support', $3, 'forwarding', $4, 'smtp', $5, $6)`, mb, f.wid, "support-"+mb.String()[:8]+"@example.com",
		mb.String(), f.brand, src)

	if r := f.agent.do("DELETE", "/v1/teams/"+src.String()+"?move_to="+dst.String(), nil); r.Status != 403 {
		t.Fatalf("agent deleted: %d", r.Status)
	}
	if r := f.owner.do("DELETE", "/v1/teams/"+src.String()+"?move_to="+uuid.NewString(), nil); r.Status != 422 {
		t.Fatalf("unknown move_to: %d %s", r.Status, r.Raw)
	}
	if r := f.owner.do("DELETE", "/v1/teams/"+src.String()+"?move_to="+dst.String(), nil); r.Status != 204 {
		t.Fatalf("delete: %d %s", r.Status, r.Raw)
	}
	var team *uuid.UUID
	var assignee *uuid.UUID
	f.scalar(`SELECT team_id FROM tickets WHERE id = $1`, &team, open)
	if team == nil || *team != dst {
		t.Fatalf("open ticket team %v", team)
	}
	f.scalar(`SELECT assignee_id FROM tickets WHERE id = $1`, &assignee, open)
	if assignee != nil {
		t.Fatal("assignee kept on a private team they are not on")
	}
	f.scalar(`SELECT team_id FROM tickets WHERE id = $1`, &team, solved)
	if team != nil {
		t.Fatal("solved ticket kept the deleted team")
	}
	f.scalar(`SELECT team_id FROM mailboxes WHERE id = $1`, &team, mb)
	if team == nil || *team != dst {
		t.Fatal("mailbox did not move")
	}
	var via string
	f.scalar(`SELECT data->>'via' FROM activities WHERE ticket_id = $1 AND type = 'team_changed'`, &via, open)
	if via != "agent" {
		t.Fatalf("team_changed via %q", via)
	}
	var n int
	f.scalar(`SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'ticket.hidden' AND data->>'ticket_id' = $2`, &n, f.wid, open.String())
	if n != 1 {
		t.Fatal("moving into a private team did not hide the ticket")
	}
	if f.e.auditCount(f.wid.String(), "team.deleted", "") != 1 {
		t.Fatal("team.deleted not audited")
	}

	// Without move_to the tickets are left without a team.
	other := mk("Tier 3", false)
	tk := f.ticket(&other, nil)
	if r := f.owner.do("DELETE", "/v1/teams/"+other.String(), nil); r.Status != 204 {
		t.Fatalf("delete: %d %s", r.Status, r.Raw)
	}
	f.scalar(`SELECT team_id FROM tickets WHERE id = $1`, &team, tk)
	if team != nil {
		t.Fatal("ticket kept the deleted team")
	}
}

func TestViewerBecomesObserver(t *testing.T) {
	f := newTeamFixture(t)
	r := f.owner.do("POST", "/v1/teams", map[string]any{"name": "Billing",
		"members": []map[string]any{{"user_id": f.agentID, "role": "lead"}, {"user_id": f.otherID, "role": "member"}}})
	if r.Status != 201 {
		t.Fatalf("create: %d %s", r.Status, r.Raw)
	}
	team := r.str("id")
	var before int64
	f.scalar(`SELECT event_seq FROM workspaces WHERE id = $1`, &before, f.wid)

	if r := f.owner.do("PATCH", "/v1/members/"+f.agentID.String(), map[string]any{"role": "viewer"}); r.Status != 200 {
		t.Fatalf("role: %d %s", r.Status, r.Raw)
	}
	got := members(f.owner.do("GET", "/v1/teams/"+team, nil))
	if got[f.agentID.String()] != "observer" || got[f.otherID.String()] != "member" {
		t.Fatalf("roles after demotion %v", got)
	}
	var n int
	f.scalar(`SELECT count(*) FROM events WHERE workspace_id = $1 AND seq > $2 AND type = 'team.updated' AND data->'team'->>'id' = $3`,
		&n, f.wid, before, team)
	if n != 1 {
		t.Fatalf("team.updated events %d", n)
	}
	if f.e.auditCount(f.wid.String(), "team.updated", "") != 1 {
		t.Fatal("team change not audited")
	}
	// An observer already: nothing more changes.
	if r := f.owner.do("PATCH", "/v1/members/"+f.agentID.String(), map[string]any{"role": "agent"}); r.Status != 200 {
		t.Fatalf("role back: %d", r.Status)
	}
	if members(f.owner.do("GET", "/v1/teams/"+team, nil))[f.agentID.String()] != "observer" {
		t.Fatal("promotion changed the team role")
	}
}

// Mailbox aliases that route to a deleted team follow it to move_to, else go back to their mailbox's team.
func TestDeleteTeamMovesAliasTeams(t *testing.T) {
	f := newTeamFixture(t)
	mk := func(name string) uuid.UUID {
		r := f.owner.do("POST", "/v1/teams", map[string]any{"name": name})
		if r.Status != 201 {
			t.Fatalf("create %s: %d %s", name, r.Status, r.Raw)
		}
		return uuid.MustParse(r.str("id"))
	}
	a, b, c, keep := mk("Projects"), mk("Projects 2"), mk("Ops"), mk("Sales")
	mb := uuid.New()
	f.exec(`INSERT INTO mailboxes (id, workspace_id, name, email, inbound_kind, forwarding_token, outbound_kind, brand_id, aliases, alias_teams)
		VALUES ($1, $2, 'Support', $3, 'forwarding', $4, 'smtp', $5, '{p@example.com,o@example.com,s@example.com}',
		jsonb_build_object('p@example.com', $6::text, 'o@example.com', $7::text, 's@example.com', $8::text))`,
		mb, f.wid, "support-"+mb.String()[:8]+"@example.com", mb.String(), f.brand, a.String(), c.String(), keep.String())

	if r := f.owner.do("DELETE", "/v1/teams/"+a.String()+"?move_to="+b.String(), nil); r.Status != 204 {
		t.Fatalf("delete with move_to: %d %s", r.Status, r.Raw)
	}
	if r := f.owner.do("DELETE", "/v1/teams/"+c.String(), nil); r.Status != 204 {
		t.Fatalf("delete: %d %s", r.Status, r.Raw)
	}
	var got map[string]string
	f.scalar(`SELECT alias_teams FROM mailboxes WHERE id = $1`, &got, mb)
	if len(got) != 2 || got["p@example.com"] != b.String() || got["s@example.com"] != keep.String() {
		t.Fatalf("alias_teams %v", got)
	}
}
