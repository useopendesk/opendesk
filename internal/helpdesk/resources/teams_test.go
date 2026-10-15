package resources_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/helpdesk/resources"
	"github.com/useopendesk/opendesk/internal/ids"
)

type teamScoped struct {
	ID     string  `json:"id"`
	TeamID *string `json:"team_id"`
	Shared bool    `json:"shared"`
}

func listIDs(t *testing.T, r *hdtest.Resp) map[string]bool {
	t.Helper()
	var p page[teamScoped]
	r.JSON(t, &p)
	out := map[string]bool{}
	for _, x := range p.Data {
		out[x.ID] = true
	}
	return out
}

func TestTeamScopedResources(t *testing.T) {
	e := hdtest.New(t, resources.Register)
	ws := e.NewWorkspace()
	lead := e.AddUser(ws, auth.RoleAgent, "Lena Lead")
	team := ids.New()
	e.Exec(`INSERT INTO teams (id, workspace_id, name, private) VALUES ($1, $2, 'Security', true)`, team, ws.ID)
	e.Exec(`INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, 'lead')`, team, lead.ID)
	tk, other := ids.New(), ids.New()
	e.Exec(`INSERT INTO tickets (id, workspace_id, number, channel, brand_id, team_id) VALUES ($1, $2, 1, 'api', $3, $4), ($5, $2, 2, 'api', $3, NULL)`,
		tk, ws.ID, ws.BrandID, team, other)

	// Saved replies: only the team writes its replies; others never see them.
	body := map[string]any{"title": "Incident", "body_html": "<p>We are on it</p>", "team_id": team.String()}
	e.Do("POST", "/v1/saved-replies", ws.Agent.As(ws), body).Want(t, 403)
	var reply teamScoped
	e.Do("POST", "/v1/saved-replies", lead.As(ws), body).Want(t, 201).JSON(t, &reply)
	if reply.TeamID == nil || *reply.TeamID != team.String() {
		t.Fatalf("reply team %v", reply.TeamID)
	}
	var everyone teamScoped
	e.Do("POST", "/v1/saved-replies", ws.Agent.As(ws), map[string]any{"title": "Hi", "body_html": "<p>Hi</p>"}).Want(t, 201).JSON(t, &everyone)
	if got := listIDs(t, e.Do("GET", "/v1/saved-replies", ws.Agent.As(ws), nil).Want(t, 200)); got[reply.ID] || !got[everyone.ID] {
		t.Fatalf("outsider list %v", got)
	}
	e.Do("GET", "/v1/saved-replies/"+reply.ID, ws.Agent.As(ws), nil).Want(t, 404)
	if got := listIDs(t, e.Do("GET", "/v1/saved-replies?ticket_id="+tk.String(), lead.As(ws), nil).Want(t, 200)); !got[reply.ID] || !got[everyone.ID] {
		t.Fatalf("ticket list %v", got)
	}
	if got := listIDs(t, e.Do("GET", "/v1/saved-replies?ticket_id="+other.String(), lead.As(ws), nil).Want(t, 200)); got[reply.ID] || !got[everyone.ID] {
		t.Fatalf("other ticket list %v", got)
	}
	if got := listIDs(t, e.Do("GET", "/v1/saved-replies?team_id=none", lead.As(ws), nil).Want(t, 200)); got[reply.ID] || !got[everyone.ID] {
		t.Fatalf("team none %v", got)
	}

	// Views: admins and the team's leads share a view with the team.
	view := map[string]any{"name": "Escalations", "team_id": team.String(), "filters": map[string]any{"status": []string{"open"}}}
	e.Do("POST", "/v1/views", ws.Agent.As(ws), view).Want(t, 403)
	e.Do("POST", "/v1/views", ws.Admin.As(ws), map[string]any{"name": "Both", "team_id": team.String(), "shared": true}).Want(t, 422)
	var v teamScoped
	e.Do("POST", "/v1/views", lead.As(ws), view).Want(t, 201).JSON(t, &v)
	if v.Shared || v.TeamID == nil {
		t.Fatalf("team view %+v", v)
	}
	member := e.AddUser(ws, auth.RoleAgent, "Mo Member")
	e.Exec(`INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, 'member')`, team, member.ID)
	if got := listIDs(t, e.Do("GET", "/v1/views", member.As(ws), nil).Want(t, 200)); !got[v.ID] {
		t.Fatal("member missed the team view")
	}
	if got := listIDs(t, e.Do("GET", "/v1/views", ws.Agent.As(ws), nil).Want(t, 200)); got[v.ID] {
		t.Fatal("outsider saw the team view")
	}
	e.Do("PATCH", "/v1/views/"+v.ID, member.As(ws), map[string]any{"name": "Mine now"}).Want(t, 403)
	// A lead takes a view off the team only as their own, never as a workspace-shared view.
	e.Do("PATCH", "/v1/views/"+v.ID, lead.As(ws), map[string]any{"team_id": nil, "shared": true}).Want(t, 403)
	var still teamScoped
	e.Do("GET", "/v1/views/"+v.ID, lead.As(ws), nil).Want(t, 200).JSON(t, &still)
	if still.Shared || still.TeamID == nil {
		t.Fatalf("view changed by a refused request: %+v", still)
	}

	// SLA policies: a team's policy, listed by team.
	var sla teamScoped
	e.Do("POST", "/v1/sla-policies", ws.Admin.As(ws), map[string]any{"name": "Security", "first_response_minutes": 30,
		"resolution_minutes": 240, "team_id": team.String()}).Want(t, 201).JSON(t, &sla)
	if sla.TeamID == nil || *sla.TeamID != team.String() {
		t.Fatalf("sla team %v", sla.TeamID)
	}
	if got := listIDs(t, e.Do("GET", "/v1/sla-policies?team_id="+team.String(), ws.Admin.As(ws), nil).Want(t, 200)); len(got) != 1 || !got[sla.ID] {
		t.Fatalf("sla by team %v", got)
	}
	e.Do("POST", "/v1/sla-policies", ws.Admin.As(ws), map[string]any{"name": "Bad", "first_response_minutes": 30,
		"resolution_minutes": 240, "team_id": uuid.NewString()}).Want(t, 422)
}
