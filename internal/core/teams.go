package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/audit"
	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/displayname"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/events"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/ids"
	"github.com/useopendesk/opendesk/internal/jobs"
)

// Teams (contracts/teams.md). A team's members hold a role: lead, member or observer. A
// private team's tickets are visible only to its members, its assignees and API keys
// granted it; its member list is shown only to its members and admins.

const (
	maxTeams       = 200
	maxTeamMembers = 500
	maxSignature   = 64 << 10
	maxTeamDesc    = 280
	// moveEventCap bounds the per-ticket activities and events of a team deletion; the
	// rest of the tickets move in one statement and clients refetch on team.deleted.
	moveEventCap = 500
)

var assignModes = []string{"none", "round_robin", "load_balanced"}

// TeamMember is one member of a team and their role.
type TeamMember struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role" enum:"lead,member,observer"`
}

type CoreTeam struct {
	ID          uuid.UUID    `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Color       string       `json:"color" enum:"gray,red,orange,amber,yellow,lime,green,teal,cyan,blue,indigo,violet,purple,pink,rose"`
	Private     bool         `json:"private"`
	Members     []TeamMember `json:"members" doc:"Of a private team, listed only to its members and admins"`
	MemberIDs   []uuid.UUID  `json:"member_ids" doc:"Every member, any role"`
	// SignatureHTML replaces the address's signature on this team's replies when not empty.
	SignatureHTML string                `json:"signature_html"`
	BusinessHours *hd.TeamBusinessHours `json:"business_hours" doc:"null follows the workspace's business hours"`
	AssignMode    *string               `json:"assign_mode" enum:"none,round_robin,load_balanced" nullable:"true" doc:"The team's assignment rule; null follows the workspace default"`
	MailboxIDs    []uuid.UUID           `json:"mailbox_ids" doc:"Addresses whose default team this is"`
	OpenCount     int                   `json:"open_count" doc:"Open and pending tickets, spam excluded"`
	// UnassignedCount counts the open and pending tickets without an assignee.
	UnassignedCount int       `json:"unassigned_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

const teamSelect = `SELECT t.id, t.name, t.description, t.color, t.private, t.signature_html, t.business_hours, t.created_at, t.updated_at,
	COALESCE((SELECT json_agg(json_build_object('user_id', tm.user_id, 'role', tm.role)
		ORDER BY CASE tm.role WHEN 'lead' THEN 0 WHEN 'member' THEN 1 ELSE 2 END, tm.created_at, tm.user_id)
		FROM team_members tm WHERE tm.team_id = t.id), '[]'),
	(SELECT ar.mode FROM assignment_rules ar WHERE ar.workspace_id = t.workspace_id AND ar.team_id = t.id),
	COALESCE((SELECT array_agg(m.id ORDER BY m.created_at, m.id) FROM mailboxes m WHERE m.workspace_id = t.workspace_id AND m.team_id = t.id), '{}'),
	(SELECT count(*) FROM tickets k WHERE k.workspace_id = t.workspace_id AND k.team_id = t.id AND k.deleted_at IS NULL
		AND NOT k.spam AND k.status IN ('open', 'pending')),
	(SELECT count(*) FROM tickets k WHERE k.workspace_id = t.workspace_id AND k.team_id = t.id AND k.deleted_at IS NULL
		AND NOT k.spam AND k.status IN ('open', 'pending') AND k.assignee_id IS NULL)
	FROM teams t`

func scanTeam(r pgx.Row) (CoreTeam, error) {
	var t CoreTeam
	var hours, members []byte
	err := r.Scan(&t.ID, &t.Name, &t.Description, &t.Color, &t.Private, &t.SignatureHTML, &hours, &t.CreatedAt, &t.UpdatedAt,
		&members, &t.AssignMode, &t.MailboxIDs, &t.OpenCount, &t.UnassignedCount)
	if err != nil {
		return t, err
	}
	t.BusinessHours = hd.ParseTeamHours(hours)
	if err := json.Unmarshal(members, &t.Members); err != nil {
		return t, err
	}
	if t.Members == nil {
		t.Members = []TeamMember{}
	}
	t.MemberIDs = make([]uuid.UUID, 0, len(t.Members))
	for _, m := range t.Members {
		t.MemberIDs = append(t.MemberIDs, m.UserID)
	}
	slices.SortFunc(t.MemberIDs, compareUUID)
	if t.MailboxIDs == nil {
		t.MailboxIDs = []uuid.UUID{}
	}
	return t, nil
}

func loadTeam(ctx context.Context, q db.Querier, wid, id uuid.UUID) (CoreTeam, error) {
	t, err := scanTeam(q.QueryRow(ctx, teamSelect+` WHERE t.workspace_id = $1 AND t.id = $2`, wid, id))
	if db.IsNotFound(err) {
		return t, httpx.NotFound("Team")
	}
	return t, err
}

// redacted is the team without its members: what everyone else sees of a private team.
func (t CoreTeam) redacted() CoreTeam {
	if !t.Private {
		return t
	}
	t.Members, t.MemberIDs = []TeamMember{}, []uuid.UUID{}
	return t
}

// forViewer returns the team as p may see it: a private team's members are listed only to
// its members and to admins.
func forViewer(ctx context.Context, q db.Querier, p *auth.Principal, t CoreTeam) (CoreTeam, error) {
	if !t.Private || (p.Kind == auth.KindUser && p.Role.AtLeast(auth.RoleAdmin)) {
		return t, nil
	}
	hidden, err := domain.HiddenTeams(ctx, q, p)
	if err != nil {
		return t, err
	}
	if slices.Contains(hidden, t.ID) {
		return t.redacted(), nil
	}
	return t, nil
}

// publishTeam publishes team.<verb>. A private team's members are left out: the event
// reaches the whole workspace, and members refetch the team.
func publishTeam(ctx context.Context, tx pgx.Tx, wid uuid.UUID, verb string, t CoreTeam) error {
	_, err := events.Publish(ctx, tx, events.Event{WorkspaceID: wid, Type: "team." + verb, Data: map[string]any{"team": t.redacted()}})
	return err
}

func compareUUID(a, b uuid.UUID) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func teamNameConflict(err error) error {
	if db.IsUniqueViolation(err, "") {
		return httpx.Conflict("A team with this name already exists.")
	}
	return err
}

type coreTeamBody struct {
	Name          *string                           `json:"name,omitempty" maxLength:"200"`
	Description   *string                           `json:"description,omitempty" maxLength:"280"`
	Color         *string                           `json:"color,omitempty" enum:"gray,red,orange,amber,yellow,lime,green,teal,cyan,blue,indigo,violet,purple,pink,rose"`
	Private       *bool                             `json:"private,omitempty" doc:"Admins only"`
	Members       *[]TeamMember                     `json:"members,omitempty" maxItems:"500" doc:"Replaces the members"`
	MemberIDs     *[]uuid.UUID                      `json:"member_ids,omitempty" maxItems:"500" doc:"Replaces the members; each joins as member unless members names them"`
	SignatureHTML *string                           `json:"signature_html,omitempty" maxLength:"65536"`
	BusinessHours hd.Nullable[hd.TeamBusinessHours] `json:"business_hours,omitempty" doc:"null follows the workspace's business hours"`
	AssignMode    hd.Nullable[string]               `json:"assign_mode,omitempty" doc:"none, round_robin, load_balanced, or null for the workspace default"`
}

// teamFields are the validated scalar changes of a body.
type teamFields struct {
	name, description, color, signature *string
	hours                               []byte // valid when setHours
	setHours                            bool
}

func (b coreTeamBody) fields(create bool) (teamFields, error) {
	var f teamFields
	if b.Name != nil || create {
		raw := ""
		if b.Name != nil {
			raw = *b.Name
		}
		n, err := displayname.Validate("Team name", raw, displayname.Workspace, false)
		if err != nil {
			return f, err
		}
		f.name = &n
	}
	if b.Description != nil {
		s := strings.Join(strings.Fields(*b.Description), " ")
		if len([]rune(s)) > maxTeamDesc {
			return f, httpx.Validation(fmt.Sprintf("description must be %d characters or less", maxTeamDesc))
		}
		if strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) || r == '<' || r == '>' }) {
			return f, httpx.Validation("description contains characters that are not allowed")
		}
		f.description = &s
	}
	if b.Color != nil {
		if !slices.Contains(domain.LabelColors, *b.Color) {
			return f, httpx.Validation("color must be one of " + strings.Join(domain.LabelColors, ", "))
		}
		f.color = b.Color
	}
	if b.SignatureHTML != nil {
		if len(*b.SignatureHTML) > maxSignature {
			return f, httpx.Validation("signature_html must be 64 KB or less")
		}
		s := hd.SanitizeSignature(*b.SignatureHTML)
		f.signature = &s
	}
	if b.BusinessHours.Set {
		f.setHours = true
		if !b.BusinessHours.Null {
			v, err := hd.ValidateTeamHours(b.BusinessHours.Value)
			if err != nil {
				return f, err
			}
			if f.hours, err = json.Marshal(v); err != nil {
				return f, err
			}
		}
	}
	if b.AssignMode.Set && !b.AssignMode.Null && !slices.Contains(assignModes, b.AssignMode.Value) {
		return f, httpx.Validation("assign_mode must be none, round_robin, load_balanced, or null")
	}
	return f, nil
}

// wantedMembers is the member list a body asks for; ok is false when it names none.
// member_ids join as member unless members gives them a role.
func (b coreTeamBody) wantedMembers() ([]TeamMember, bool, error) {
	if b.Members == nil && b.MemberIDs == nil {
		return nil, false, nil
	}
	out := []TeamMember{}
	seen := map[uuid.UUID]bool{}
	if b.Members != nil {
		for _, m := range *b.Members {
			if !slices.Contains(hd.TeamRoles, m.Role) {
				return nil, false, httpx.Validation("members[].role must be lead, member or observer")
			}
			if seen[m.UserID] {
				return nil, false, httpx.Validation("members lists a user twice")
			}
			seen[m.UserID] = true
			out = append(out, m)
		}
	}
	if b.MemberIDs != nil {
		for _, id := range *b.MemberIDs {
			if !seen[id] {
				seen[id] = true
				out = append(out, TeamMember{UserID: id, Role: hd.TeamMember})
			}
		}
	}
	if len(out) > maxTeamMembers {
		return nil, false, httpx.Validation(fmt.Sprintf("A team can have at most %d members.", maxTeamMembers))
	}
	return out, true, nil
}

// memberDiff is what a membership change did.
type memberDiff struct {
	Added   []uuid.UUID `json:"added,omitempty"`
	Removed []uuid.UUID `json:"removed,omitempty"`
	Changed []uuid.UUID `json:"role_changed,omitempty"`
}

func (d memberDiff) empty() bool { return len(d.Added)+len(d.Removed)+len(d.Changed) == 0 }

// teamRoles returns the team's members and their roles.
func teamRoles(ctx context.Context, q db.Querier, teamID uuid.UUID) (map[uuid.UUID]string, error) {
	rows, err := q.Query(ctx, `SELECT user_id, role FROM team_members WHERE team_id = $1`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]string{}
	for rows.Next() {
		var id uuid.UUID
		var role string
		if err := rows.Scan(&id, &role); err != nil {
			return nil, err
		}
		out[id] = role
	}
	return out, rows.Err()
}

// setTeamMembers replaces a team's members after checking each is a member of the
// workspace who may hold the role: viewers can only observe.
func setTeamMembers(ctx context.Context, tx pgx.Tx, wid, teamID uuid.UUID, want []TeamMember) (memberDiff, error) {
	var diff memberDiff
	userIDs := make([]uuid.UUID, 0, len(want))
	for _, m := range want {
		userIDs = append(userIDs, m.UserID)
	}
	rows, err := tx.Query(ctx, `SELECT user_id, role FROM members WHERE workspace_id = $1 AND user_id = ANY($2)`, wid, userIDs)
	if err != nil {
		return diff, err
	}
	wsRole := map[uuid.UUID]string{}
	for rows.Next() {
		var id uuid.UUID
		var role string
		if err := rows.Scan(&id, &role); err != nil {
			rows.Close()
			return diff, err
		}
		wsRole[id] = role
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return diff, err
	}
	for _, m := range want {
		role, ok := wsRole[m.UserID]
		if !ok {
			return diff, httpx.Validation("Team members must all be members of this workspace.")
		}
		if role == string(auth.RoleViewer) && m.Role != hd.TeamObserver {
			return diff, httpx.Validation("Viewers can only be observers of a team.")
		}
	}
	cur, err := teamRoles(ctx, tx, teamID)
	if err != nil {
		return diff, err
	}
	keep := map[uuid.UUID]bool{}
	for _, m := range want {
		keep[m.UserID] = true
		switch old, ok := cur[m.UserID]; {
		case !ok:
			diff.Added = append(diff.Added, m.UserID)
		case old != m.Role:
			diff.Changed = append(diff.Changed, m.UserID)
		default:
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)
			ON CONFLICT (team_id, user_id) DO UPDATE SET role = EXCLUDED.role`, teamID, m.UserID, m.Role); err != nil {
			return diff, err
		}
	}
	for id := range cur {
		if !keep[id] {
			diff.Removed = append(diff.Removed, id)
		}
	}
	if len(diff.Removed) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM team_members WHERE team_id = $1 AND user_id = ANY($2)`, teamID, diff.Removed); err != nil {
			return diff, err
		}
	}
	for _, s := range [][]uuid.UUID{diff.Added, diff.Removed, diff.Changed} {
		slices.SortFunc(s, compareUUID)
	}
	return diff, nil
}

// setAssignMode stores the team's assignment rule; nil removes it.
func setAssignMode(ctx context.Context, tx pgx.Tx, wid, teamID uuid.UUID, mode *string) error {
	if mode == nil {
		_, err := tx.Exec(ctx, `DELETE FROM assignment_rules WHERE workspace_id = $1 AND team_id = $2`, wid, teamID)
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO assignment_rules (workspace_id, team_id, mode) VALUES ($1, $2, $3)
		ON CONFLICT (workspace_id, team_id) WHERE team_id IS NOT NULL DO UPDATE SET mode = EXCLUDED.mode`, wid, teamID, *mode)
	return err
}

// privateSeers are the users a private team's tickets are visible to through membership.
func privateSeers(private bool, roles map[uuid.UUID]string) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	if private {
		for id := range roles {
			out[id] = true
		}
	}
	return out
}

// seersChanged returns the users whose set of visible private teams differs between
// before and after: their sockets reconnect with a fresh ws token.
func seersChanged(before, after map[uuid.UUID]bool) []uuid.UUID {
	var out []uuid.UUID
	for id := range before {
		if !after[id] {
			out = append(out, id)
		}
	}
	for id := range after {
		if !before[id] {
			out = append(out, id)
		}
	}
	return out
}

// lockTeam locks the workspace's team row and returns whether it is private.
func lockTeam(ctx context.Context, tx pgx.Tx, wid, id uuid.UUID) (bool, string, error) {
	var private bool
	var name string
	err := tx.QueryRow(ctx, `SELECT private, name FROM teams WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, wid, id).Scan(&private, &name)
	if db.IsNotFound(err) {
		return false, "", httpx.NotFound("Team")
	}
	return private, name, err
}

// openTeamTickets returns up to limit open, pending or on hold tickets of a team, most
// recent first, and whether there are more.
func openTeamTickets(ctx context.Context, q db.Querier, wid, teamID uuid.UUID, limit int) ([]uuid.UUID, bool, error) {
	rows, err := q.Query(ctx, `SELECT id FROM tickets WHERE workspace_id = $1 AND team_id = $2 AND deleted_at IS NULL
		AND status IN ('open', 'pending', 'on_hold') ORDER BY last_message_at DESC, id LIMIT $3`, wid, teamID, limit+1)
	if err != nil {
		return nil, false, err
	}
	list, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, false, err
	}
	if len(list) > limit {
		return list[:limit], true, nil
	}
	return list, false, nil
}

// publishPrivacyChange tells clients about a team's tickets after it turned private (they
// drop them: ticket.hidden) or public (they show them: ticket.updated). A team with more
// than hd.HiddenEventCap open tickets sends none; clients refetch on team.updated.
func publishPrivacyChange(ctx context.Context, tx pgx.Tx, wid, teamID uuid.UUID, private bool) error {
	list, more, err := openTeamTickets(ctx, tx, wid, teamID, hd.HiddenEventCap)
	if err != nil || more {
		return err
	}
	for _, tid := range list {
		if private {
			err = hd.PublishTicketHidden(ctx, tx, wid, tid, teamID)
		} else {
			err = domain.PublishTicketUpdated(ctx, tx, wid, tid, []string{"team_id"})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// requireTeamManager allows admins (and API keys with settings:write) and the team's leads.
func requireTeamManager(ctx context.Context, q db.Querier, id uuid.UUID) (*auth.Principal, bool, error) {
	p, err := auth.RequireWorkspace(ctx, auth.RoleAgent, auth.ScopeSettingsWrite)
	if err != nil {
		return nil, false, err
	}
	if p.Kind == auth.KindAPIKey || p.Role.AtLeast(auth.RoleAdmin) {
		return p, true, nil
	}
	role, err := hd.TeamRole(ctx, q, p.WorkspaceID, id, p.UserID)
	if err != nil {
		return nil, false, err
	}
	if role != hd.TeamLead {
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM teams WHERE workspace_id = $1 AND id = $2)`, p.WorkspaceID, id).Scan(&exists); err != nil {
			return nil, false, err
		}
		if !exists {
			return nil, false, httpx.NotFound("Team")
		}
		return nil, false, httpx.Forbidden()
	}
	return p, false, nil
}

func (s *svc) disconnectAll(uids []uuid.UUID) {
	for _, uid := range uids {
		s.disconnect(uid, uuid.Nil)
	}
}

func (s *svc) registerTeams(api huma.API) {
	tags := []string{"Teams"}

	register(s, api, huma.Operation{OperationID: "list-teams", Method: http.MethodGet, Path: "/v1/teams",
		Summary: "List teams", Tags: tags, Description: "Private teams are listed for everyone; their members only to their members and admins."},
		func(ctx context.Context, in *struct{ httpx.ListParams }) (*struct{ Body httpx.Page[CoreTeam] }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleViewer, auth.ScopeSettingsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			var cur struct {
				Name string    `json:"n"`
				ID   uuid.UUID `json:"id"`
			}
			if err := httpx.DecodeCursor(in.Cursor, &cur); err != nil {
				return nil, err
			}
			rows, err := s.d.DB.Pool.Query(ctx, teamSelect+` WHERE t.workspace_id = $1 AND (t.name, t.id) > ($2, $3)
				ORDER BY t.name, t.id LIMIT $4`, p.WorkspaceID, cur.Name, cur.ID, in.Limit+1)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (CoreTeam, error) { return scanTeam(r) })
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			for i := range list {
				if list[i], err = forViewer(ctx, s.d.DB.Pool, p, list[i]); err != nil {
					return nil, httpx.Wrap(err)
				}
			}
			return &struct{ Body httpx.Page[CoreTeam] }{httpx.NewPage(list, in.Limit, func(t CoreTeam) any {
				return map[string]any{"n": t.Name, "id": t.ID}
			})}, nil
		})

	register(s, api, huma.Operation{OperationID: "get-team", Method: http.MethodGet, Path: "/v1/teams/{id}",
		Summary: "Get a team", Tags: tags},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct{ Body CoreTeam }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleViewer, auth.ScopeSettingsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			id, err := parseID(in.ID, "Team")
			if err != nil {
				return nil, err
			}
			t, err := loadTeam(ctx, s.d.DB.Pool, p.WorkspaceID, id)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if t, err = forViewer(ctx, s.d.DB.Pool, p, t); err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body CoreTeam }{t}, nil
		})

	register(s, api, huma.Operation{OperationID: "create-team", Method: http.MethodPost, Path: "/v1/teams",
		Summary: "Create a team", Tags: tags, DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *struct{ Body coreTeamBody }) (*struct{ Body CoreTeam }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			f, err := in.Body.fields(true)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			want, _, err := in.Body.wantedMembers()
			if err != nil {
				return nil, err
			}
			private := in.Body.Private != nil && *in.Body.Private
			if private && p.Kind == auth.KindAPIKey {
				return nil, httpx.E(403, "forbidden", "API keys cannot create private teams.")
			}
			var t CoreTeam
			var reconnect []uuid.UUID
			err = s.tx(ctx, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1::text || ':teams'))`, p.WorkspaceID); err != nil {
					return err
				}
				var n int
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM teams WHERE workspace_id = $1`, p.WorkspaceID).Scan(&n); err != nil {
					return err
				}
				if n >= maxTeams {
					return httpx.E(422, "limit_reached", fmt.Sprintf("A workspace can have at most %d teams.", maxTeams))
				}
				id := ids.New()
				color := "gray"
				if f.color != nil {
					color = *f.color
				}
				if _, err := tx.Exec(ctx, `INSERT INTO teams (id, workspace_id, name, description, color, private, signature_html, business_hours)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, id, p.WorkspaceID, *f.name, deref(f.description), color, private,
					deref(f.signature), nilIfEmpty(f.hours)); err != nil {
					return teamNameConflict(err)
				}
				if len(want) > 0 {
					diff, err := setTeamMembers(ctx, tx, p.WorkspaceID, id, want)
					if err != nil {
						return err
					}
					if private {
						reconnect = diff.Added
					}
				}
				if in.Body.AssignMode.Set && !in.Body.AssignMode.Null {
					if err := setAssignMode(ctx, tx, p.WorkspaceID, id, &in.Body.AssignMode.Value); err != nil {
						return err
					}
				}
				var err error
				if t, err = loadTeam(ctx, tx, p.WorkspaceID, id); err != nil {
					return err
				}
				if err := publishTeam(ctx, tx, p.WorkspaceID, "created", t); err != nil {
					return err
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, "team.created", "team", &id,
					map[string]any{"name": t.Name, "private": t.Private, "members": len(t.Members)})
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			s.disconnectAll(reconnect)
			return &struct{ Body CoreTeam }{t}, nil
		})

	register(s, api, huma.Operation{OperationID: "update-team", Method: http.MethodPatch, Path: "/v1/teams/{id}",
		Summary: "Update a team", Tags: tags,
		Description: "Admins, and the team's leads. Only admins change private; a lead cannot leave the team without a lead."},
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body coreTeamBody
		}) (*struct{ Body CoreTeam }, error) {
			id, err := parseID(in.ID, "Team")
			if err != nil {
				return nil, err
			}
			p, admin, err := requireTeamManager(ctx, s.d.DB.Pool, id)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			f, err := in.Body.fields(false)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			want, setMembers, err := in.Body.wantedMembers()
			if err != nil {
				return nil, err
			}
			var t CoreTeam
			var reconnect, lapsedKeys []uuid.UUID
			err = s.tx(ctx, func(tx pgx.Tx) error {
				wasPrivate, _, err := lockTeam(ctx, tx, p.WorkspaceID, id)
				if err != nil {
					return err
				}
				if !admin {
					// The lead may have been removed since the check above.
					if role, err := hd.TeamRole(ctx, tx, p.WorkspaceID, id, p.UserID); err != nil {
						return err
					} else if role != hd.TeamLead {
						return httpx.Forbidden()
					}
				}
				private := wasPrivate
				if in.Body.Private != nil && *in.Body.Private != wasPrivate {
					if !admin {
						return httpx.E(403, "forbidden", "Only admins can make a team private or public.")
					}
					private = *in.Body.Private
				}
				// An API key changes a private team's privacy or members only when it is granted
				// the team: anything else could open the team's tickets to others.
				if p.Kind == auth.KindAPIKey && (private != wasPrivate || (setMembers && wasPrivate)) {
					ok, err := apiKeyGranted(ctx, tx, p, id)
					if err != nil {
						return err
					}
					if !ok {
						return httpx.E(403, "forbidden", "This API key cannot change the privacy or members of a private team it is not granted.")
					}
				}
				before, err := teamRoles(ctx, tx, id)
				if err != nil {
					return err
				}
				changes := []string{}
				for _, c := range []struct {
					set  bool
					name string
				}{{f.name != nil, "name"}, {f.description != nil, "description"}, {f.color != nil, "color"},
					{private != wasPrivate, "private"}, {f.signature != nil, "signature_html"}, {f.setHours, "business_hours"}} {
					if c.set {
						changes = append(changes, c.name)
					}
				}
				if _, err := tx.Exec(ctx, `UPDATE teams SET name = COALESCE($3, name), description = COALESCE($4, description),
					color = COALESCE($5, color), private = $6, signature_html = COALESCE($7, signature_html),
					business_hours = CASE WHEN $8 THEN $9::jsonb ELSE business_hours END, updated_at = now()
					WHERE workspace_id = $1 AND id = $2`, p.WorkspaceID, id, f.name, f.description, f.color, private, f.signature,
					f.setHours, nilIfEmpty(f.hours)); err != nil {
					return teamNameConflict(err)
				}
				var diff memberDiff
				if setMembers {
					if diff, err = setTeamMembers(ctx, tx, p.WorkspaceID, id, want); err != nil {
						return err
					}
					if !diff.empty() {
						changes = append(changes, "members")
					}
					// A grant to a key or webhook lasts while its granter is on the team.
					if len(diff.Removed) > 0 {
						if lapsedKeys, err = dropLapsedGrants(ctx, tx, p, p.WorkspaceID); err != nil {
							return err
						}
					}
				}
				after, err := teamRoles(ctx, tx, id)
				if err != nil {
					return err
				}
				if !admin && after[p.UserID] != hd.TeamLead {
					leads := 0
					for _, r := range after {
						if r == hd.TeamLead {
							leads++
						}
					}
					if leads == 0 {
						return httpx.E(409, "last_lead", "Make someone else a lead of this team first.")
					}
				}
				if in.Body.AssignMode.Set {
					if err := setAssignMode(ctx, tx, p.WorkspaceID, id, in.Body.AssignMode.Ptr()); err != nil {
						return err
					}
					changes = append(changes, "assign_mode")
				}
				if private && (!wasPrivate || len(diff.Removed) > 0) {
					if err := s.unassignOffTeam(ctx, tx, p, p.WorkspaceID, id); err != nil {
						return err
					}
				}
				if wasPrivate && !private {
					if err := dropTeamGrants(ctx, tx, p.WorkspaceID, id); err != nil {
						return err
					}
				}
				if private != wasPrivate {
					if err := publishPrivacyChange(ctx, tx, p.WorkspaceID, id, private); err != nil {
						return err
					}
					if err := audit.Log(ctx, tx, p, p.WorkspaceID, "team.privacy_changed", "team", &id,
						map[string]any{"from": wasPrivate, "to": private}); err != nil {
						return err
					}
				}
				reconnect = seersChanged(privateSeers(wasPrivate, before), privateSeers(private, after))
				if t, err = loadTeam(ctx, tx, p.WorkspaceID, id); err != nil {
					return err
				}
				if err := publishTeam(ctx, tx, p.WorkspaceID, "updated", t); err != nil {
					return err
				}
				data := map[string]any{"changes": changes}
				if !diff.empty() {
					data["members"] = diff
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, "team.updated", "team", &id, data)
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			s.disconnectAll(reconnect)
			s.disconnectAll(lapsedKeys)
			if t, err = forViewer(ctx, s.d.DB.Pool, p, t); err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body CoreTeam }{t}, nil
		})

	register(s, api, huma.Operation{OperationID: "delete-team", Method: http.MethodDelete, Path: "/v1/teams/{id}",
		Summary: "Delete a team", Tags: tags, DefaultStatus: http.StatusNoContent,
		Description: "move_to moves the team's open tickets, addresses, widgets and routing rules to another team; without it they are left without a team and its routing rules are deleted."},
		func(ctx context.Context, in *struct {
			ID     string `path:"id"`
			MoveTo string `query:"move_to" doc:"A team to move the open tickets, addresses, widgets and routing rules to"`
		}) (*struct{}, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			id, err := parseID(in.ID, "Team")
			if err != nil {
				return nil, err
			}
			var dst *uuid.UUID
			if strings.TrimSpace(in.MoveTo) != "" {
				v, err := hd.ParseRef(in.MoveTo, "move_to")
				if err != nil {
					return nil, err
				}
				if v == id {
					return nil, httpx.Validation("move_to must be another team")
				}
				dst = &v
			}
			var reconnect []uuid.UUID
			err = s.tx(ctx, func(tx pgx.Tx) error {
				private, name, err := lockTeam(ctx, tx, p.WorkspaceID, id)
				if err != nil {
					return err
				}
				if private && p.Kind == auth.KindAPIKey {
					if ok, err := apiKeyGranted(ctx, tx, p, id); err != nil {
						return err
					} else if !ok {
						return httpx.E(403, "forbidden", "This API key cannot delete a private team it is not granted.")
					}
				}
				if err := dropTeamGrants(ctx, tx, p.WorkspaceID, id); err != nil {
					return err
				}
				if dst != nil {
					if _, _, err := lockTeam(ctx, tx, p.WorkspaceID, *dst); err != nil {
						return httpx.Validation("move_to is not a team in this workspace")
					}
				}
				// A private team's tickets never fall back to no team, which everyone sees: they
				// all move (any status) to the team the admin picks.
				if private {
					var n int
					if err := tx.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE workspace_id = $1 AND team_id = $2 AND deleted_at IS NULL`,
						p.WorkspaceID, id).Scan(&n); err != nil {
						return err
					}
					if n > 0 && dst == nil {
						return httpx.E(422, "move_required", fmt.Sprintf("This private team has %d tickets. Choose a team to move them to (move_to).", n))
					}
				}
				roles, err := teamRoles(ctx, tx, id)
				if err != nil {
					return err
				}
				moved, err := s.moveTeamTickets(ctx, tx, p, id, dst, private)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE mailboxes SET team_id = $3 WHERE workspace_id = $1 AND team_id = $2`, p.WorkspaceID, id, dst); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE widgets SET team_id = $3, updated_at = now() WHERE workspace_id = $1 AND team_id = $2`,
					p.WorkspaceID, id, dst); err != nil {
					return err
				}
				// Widget topics that route to the team follow it.
				if _, err := tx.Exec(ctx, `UPDATE widgets SET settings = jsonb_set(settings, '{topics}', (
						SELECT COALESCE(jsonb_agg(CASE WHEN x.t ->> 'team_id' = $2::text
							THEN jsonb_set(x.t, '{team_id}', COALESCE(to_jsonb($3::text), 'null'::jsonb)) ELSE x.t END ORDER BY x.n), '[]'::jsonb)
						FROM jsonb_array_elements(settings -> 'topics') WITH ORDINALITY AS x(t, n))), updated_at = now()
					WHERE workspace_id = $1 AND jsonb_typeof(settings -> 'topics') = 'array'
					AND EXISTS (SELECT 1 FROM jsonb_array_elements(settings -> 'topics') e WHERE e ->> 'team_id' = $2::text)`,
					p.WorkspaceID, id, uuidText(dst)); err != nil {
					return err
				}
				if dst != nil {
					if _, err := tx.Exec(ctx, `UPDATE routing_rules SET team_id = $3, updated_at = now() WHERE workspace_id = $1 AND team_id = $2`,
						p.WorkspaceID, id, *dst); err != nil {
						return err
					}
				}
				if _, err := tx.Exec(ctx, `DELETE FROM round_robin_cursors WHERE workspace_id = $1 AND pool = $2`,
					p.WorkspaceID, "team:"+id.String()); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `DELETE FROM teams WHERE workspace_id = $1 AND id = $2`, p.WorkspaceID, id); err != nil {
					return err
				}
				reconnect = seersChanged(privateSeers(private, roles), nil)
				if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: p.WorkspaceID, Type: "team.deleted",
					Data: map[string]any{"team_id": id, "move_to": uuidOrNil(dst)}}); err != nil {
					return err
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, "team.deleted", "team", &id,
					map[string]any{"name": name, "move_to": uuidOrNil(dst), "tickets_moved": moved})
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			s.disconnectAll(reconnect)
			return nil, nil
		})

	register(s, api, huma.Operation{OperationID: "join-team", Method: http.MethodPost, Path: "/v1/teams/{id}/join",
		Summary: "Join a team as an observer", Tags: tags,
		Description: "Admins and owners. Joining a private team lets them see its tickets; its leads are notified."},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct{ Body CoreTeam }, error) {
			p, err := requireUserWorkspace(ctx, auth.RoleAdmin)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			id, err := parseID(in.ID, "Team")
			if err != nil {
				return nil, err
			}
			var t CoreTeam
			var private bool
			joined := false
			err = s.tx(ctx, func(tx pgx.Tx) error {
				var err error
				if private, _, err = lockTeam(ctx, tx, p.WorkspaceID, id); err != nil {
					return err
				}
				tag, err := tx.Exec(ctx, `INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, 'observer')
					ON CONFLICT (team_id, user_id) DO NOTHING`, id, p.UserID)
				if err != nil {
					return err
				}
				if joined = tag.RowsAffected() == 1; joined {
					if _, err := tx.Exec(ctx, `UPDATE teams SET updated_at = now() WHERE id = $1`, id); err != nil {
						return err
					}
				}
				if t, err = loadTeam(ctx, tx, p.WorkspaceID, id); err != nil {
					return err
				}
				if !joined {
					return nil
				}
				if err := publishTeam(ctx, tx, p.WorkspaceID, "updated", t); err != nil {
					return err
				}
				var name string
				if err := tx.QueryRow(ctx, `SELECT name FROM users WHERE id = $1`, p.UserID).Scan(&name); err != nil {
					return err
				}
				for _, m := range t.Members {
					if m.Role != hd.TeamLead || m.UserID == p.UserID {
						continue
					}
					if err := hd.Notify(ctx, tx, p.WorkspaceID, m.UserID, hd.NotifyTeamJoined, nil, map[string]any{
						"team_id": id.String(), "team": map[string]any{"id": id.String(), "name": t.Name, "private": private},
						"user": map[string]any{"id": p.UserID.String(), "name": name}, "by": p.UserID.String()}); err != nil {
						return err
					}
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, "team.joined", "team", &id, map[string]any{"role": hd.TeamObserver, "private": private})
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if joined && private {
				s.disconnect(p.UserID, uuid.Nil)
			}
			return &struct{ Body CoreTeam }{t}, nil
		})

	register(s, api, huma.Operation{OperationID: "leave-team", Method: http.MethodPost, Path: "/v1/teams/{id}/leave",
		Summary: "Leave a team", Tags: tags, DefaultStatus: http.StatusNoContent,
		Description: "The last lead of a team with other members cannot leave (409 last_lead)."},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct{}, error) {
			p, err := requireUserWorkspace(ctx, auth.RoleViewer)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			id, err := parseID(in.ID, "Team")
			if err != nil {
				return nil, err
			}
			var private, left bool
			var lapsedKeys []uuid.UUID
			err = s.tx(ctx, func(tx pgx.Tx) error {
				var err error
				if private, _, err = lockTeam(ctx, tx, p.WorkspaceID, id); err != nil {
					return err
				}
				roles, err := teamRoles(ctx, tx, id)
				if err != nil {
					return err
				}
				role, ok := roles[p.UserID]
				if !ok {
					return nil
				}
				if role == hd.TeamLead && len(roles) > 1 {
					leads := 0
					for _, r := range roles {
						if r == hd.TeamLead {
							leads++
						}
					}
					if leads == 1 {
						return httpx.E(409, "last_lead", "Make someone else a lead of this team before you leave it.")
					}
				}
				if _, err := tx.Exec(ctx, `DELETE FROM team_members WHERE team_id = $1 AND user_id = $2`, id, p.UserID); err != nil {
					return err
				}
				if lapsedKeys, err = dropLapsedGrants(ctx, tx, p, p.WorkspaceID); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE teams SET updated_at = now() WHERE id = $1`, id); err != nil {
					return err
				}
				left = true
				if err := s.unassignOffTeam(ctx, tx, p, p.WorkspaceID, id); err != nil {
					return err
				}
				t, err := loadTeam(ctx, tx, p.WorkspaceID, id)
				if err != nil {
					return err
				}
				if err := publishTeam(ctx, tx, p.WorkspaceID, "updated", t); err != nil {
					return err
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, "team.left", "team", &id, map[string]any{"role": role})
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if left && private {
				s.disconnect(p.UserID, uuid.Nil)
			}
			s.disconnectAll(lapsedKeys)
			return nil, nil
		})
}

// moveTeamTickets moves the open, pending and on hold tickets of a team being deleted to
// dst (nil: no team); with all (a private team), every ticket of the team. The first
// moveEventCap tickets get a team_changed activity, ticket.updated (and ticket.hidden when
// dst is private) and, when still open, a ticket_updated trigger so they are assigned in
// the new team; the rest move in one statement.
func (s *svc) moveTeamTickets(ctx context.Context, tx pgx.Tx, p *auth.Principal, from uuid.UUID, dst *uuid.UUID, all bool) (int, error) {
	wid := p.WorkspaceID
	var list []uuid.UUID
	var err error
	if all {
		rows, err := tx.Query(ctx, `SELECT id FROM tickets WHERE workspace_id = $1 AND team_id = $2 AND deleted_at IS NULL
			ORDER BY last_message_at DESC, id LIMIT $3`, wid, from, moveEventCap)
		if err != nil {
			return 0, err
		}
		if list, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID]); err != nil {
			return 0, err
		}
	} else if list, _, err = openTeamTickets(ctx, tx, wid, from, moveEventCap); err != nil {
		return 0, err
	}
	if len(list) == 0 && !all {
		return 0, nil
	}
	dstPrivate, err := hd.TeamPrivate(ctx, tx, wid, dst)
	if err != nil {
		return 0, err
	}
	via := "agent"
	if p.Kind == auth.KindAPIKey {
		via = "api"
	}
	var routed *domain.TicketRouted
	if dst != nil {
		routed = &domain.TicketRouted{Via: via}
		if p.Kind == auth.KindUser {
			u, err := domain.LoadUserRef(ctx, tx, p.UserID)
			if err != nil {
				return 0, err
			}
			routed.By = &u
		}
	}
	actorType, actorUser := "user", p.ActorUserID()
	if p.Kind == auth.KindAPIKey {
		actorType = "api"
	}
	activity := map[string]any{"from": from.String(), "to": uuidOrNil(dst), "via": via}
	// The assignee of a ticket that moves into a private team must be on that team.
	const move = `UPDATE tickets t SET team_id = $3, routed = $4, updated_at = now(),
		assignee_id = CASE WHEN $5 AND NOT EXISTS (SELECT 1 FROM team_members tm WHERE tm.team_id = $3 AND tm.user_id = t.assignee_id)
			THEN NULL ELSE t.assignee_id END
		WHERE t.workspace_id = $1 AND t.team_id = $2 AND ($6 OR (t.deleted_at IS NULL AND t.status IN ('open', 'pending', 'on_hold')))`
	moved := 0
	for _, tid := range list {
		var before *uuid.UUID
		var status string
		if err := tx.QueryRow(ctx, `SELECT assignee_id, status FROM tickets WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, wid, tid).
			Scan(&before, &status); err != nil {
			return 0, err
		}
		var after *uuid.UUID
		if err := tx.QueryRow(ctx, move+` AND t.id = $7 RETURNING t.assignee_id`, wid, from, dst, routed, dstPrivate, all, tid).Scan(&after); err != nil {
			return 0, err
		}
		moved++
		if _, err := domain.LogActivity(ctx, tx, wid, tid, actorType, actorUser, "team_changed", cloneMap(activity)); err != nil {
			return 0, err
		}
		changes := []string{"team_id"}
		if before != nil && after == nil {
			changes = append(changes, "assignee_id")
			if _, err := domain.LogActivity(ctx, tx, wid, tid, actorType, actorUser, "assigned",
				map[string]any{"from": before.String(), "to": nil}); err != nil {
				return 0, err
			}
		}
		if dstPrivate {
			if err := hd.PublishTicketHidden(ctx, tx, wid, tid, *dst); err != nil {
				return 0, err
			}
		}
		if err := domain.PublishTicketUpdated(ctx, tx, wid, tid, changes); err != nil {
			return 0, err
		}
		if status == domain.StatusSolved || status == domain.StatusClosed {
			continue
		}
		if _, err := s.d.Jobs.InsertTx(ctx, tx, jobs.TicketTriggerArgs{WorkspaceID: wid, TicketID: tid, Trigger: jobs.TriggerTicketUpdated,
			ActorUserID: actorUser, Changes: changes}, nil); err != nil {
			return 0, err
		}
	}
	// Beyond the cap (and, with all, deleted tickets): one statement, with the timeline
	// entries but no per-ticket events.
	rows, err := tx.Query(ctx, move+` RETURNING t.id`, wid, from, dst, routed, dstPrivate, all)
	if err != nil {
		return 0, err
	}
	rest, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, err
	}
	if len(rest) > 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO activities (id, workspace_id, ticket_id, actor_type, actor_user_id, type, data, created_at)
			SELECT gen_random_uuid(), $1, x, $3, $4, 'team_changed', $5, now() FROM unnest($2::uuid[]) AS x`,
			wid, rest, actorType, actorUser, activity); err != nil {
			return 0, err
		}
	}
	return moved + len(rest), nil
}

// unassignOffTeam unassigns the open, pending and on hold tickets of a private team whose
// assignee is not on it, after someone left the team, was removed from it or from the
// workspace, or the team became private: a private team's ticket is assigned only to its
// members (contracts/teams.md "Visibility"). The first moveEventCap tickets get an assigned
// activity, ticket.updated and a ticket_updated trigger so the team assigns them again; the
// rest get the activity only. A public team (or none) changes nothing.
func (s *svc) unassignOffTeam(ctx context.Context, tx pgx.Tx, p *auth.Principal, wid, teamID uuid.UUID) error {
	private, err := hd.TeamPrivate(ctx, tx, wid, &teamID)
	if err != nil || !private {
		return err
	}
	rows, err := tx.Query(ctx, `UPDATE tickets t SET assignee_id = NULL, updated_at = now() FROM tickets o
		WHERE o.id = t.id AND t.workspace_id = $1 AND t.team_id = $2 AND t.deleted_at IS NULL
		AND t.status IN ('open', 'pending', 'on_hold') AND t.assignee_id IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM team_members tm WHERE tm.team_id = $2 AND tm.user_id = t.assignee_id)
		RETURNING t.id, o.assignee_id`, wid, teamID)
	if err != nil {
		return err
	}
	type unassigned struct{ ticket, from uuid.UUID }
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (unassigned, error) {
		var u unassigned
		err := r.Scan(&u.ticket, &u.from)
		return u, err
	})
	if err != nil {
		return err
	}
	actorType, actorUser := "user", p.ActorUserID()
	if p.Kind == auth.KindAPIKey {
		actorType = "api"
	}
	for i, u := range list {
		data := map[string]any{"from": u.from.String(), "to": nil}
		if _, err := domain.LogActivity(ctx, tx, wid, u.ticket, actorType, actorUser, "assigned", data); err != nil {
			return err
		}
		if i >= moveEventCap {
			continue
		}
		changes := []string{"assignee_id"}
		if err := domain.PublishTicketUpdated(ctx, tx, wid, u.ticket, changes); err != nil {
			return err
		}
		if _, err := s.d.Jobs.InsertTx(ctx, tx, jobs.TicketTriggerArgs{WorkspaceID: wid, TicketID: u.ticket, Trigger: jobs.TriggerTicketUpdated,
			ActorUserID: actorUser, Changes: changes}, nil); err != nil {
			return err
		}
	}
	return nil
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func uuidOrNil(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return id.String()
}

func uuidText(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func nilIfEmpty(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}
