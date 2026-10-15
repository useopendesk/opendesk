package core

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/audit"
	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/events"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// API keys and webhooks see a private team's tickets and events only when the team is in
// their private_team_ids (contracts/teams.md "Visibility"). Only a lead, member or observer
// of a team may add it; an admin may remove any. Each team of the list has a grant in
// private_team_grants naming who granted it, and lasts while that person is on the team:
// whatever removes someone from a team calls dropLapsedGrants in the same transaction (the
// grant's foreign key to team_members holds at commit).

// maxPrivateTeams bounds a key's or webhook's private_team_ids (the workspace's team limit).
const maxPrivateTeams = 200

// Grant owners: the private_team_grants column naming a key or a webhook.
const (
	grantAPIKey  = "api_key_id"
	grantWebhook = "webhook_id"
)

// PrivateTeamGrant is one private team of a key or webhook and who granted it.
type PrivateTeamGrant struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	GrantedBy *domain.UserRef `json:"granted_by" doc:"The lead, member or observer of the team whose grant this is. It lasts while they are on the team."`
}

// grantedTeamsSQL reads a row's private_team_ids as the private teams of the workspace they
// still name, in a stable order. col is the uuid[] column, wid the workspace id column.
func grantedTeamsSQL(col, wid string) string {
	return `ARRAY(SELECT gt.id FROM teams gt WHERE gt.workspace_id = ` + wid + ` AND gt.private AND gt.id = ANY(` + col + `) ORDER BY gt.id)`
}

// teamGrantsSQL reads the same teams as grantedTeamsSQL with their names and granters, as
// JSON for scanGrants, ordered by name. owner is grantAPIKey or grantWebhook, id the row's
// id column.
func teamGrantsSQL(col, wid, owner, id string) string {
	return `COALESCE((SELECT json_agg(json_build_object('id', gt.id, 'name', gt.name, 'user_id', gu.id, 'user_name', gu.name,
		'user_email', gu.email, 'user_avatar', gu.avatar_url) ORDER BY gt.name, gt.id)
		FROM teams gt LEFT JOIN private_team_grants pg ON pg.team_id = gt.id AND pg.` + owner + ` = ` + id + `
		LEFT JOIN users gu ON gu.id = pg.granted_by
		WHERE gt.workspace_id = ` + wid + ` AND gt.private AND gt.id = ANY(` + col + `)), '[]')`
}

// scanGrants decodes teamGrantsSQL.
func scanGrants(b []byte) ([]PrivateTeamGrant, error) {
	var rows []struct {
		ID         uuid.UUID  `json:"id"`
		Name       string     `json:"name"`
		UserID     *uuid.UUID `json:"user_id"`
		UserName   *string    `json:"user_name"`
		UserEmail  *string    `json:"user_email"`
		UserAvatar *string    `json:"user_avatar"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	out := make([]PrivateTeamGrant, 0, len(rows))
	for _, r := range rows {
		g := PrivateTeamGrant{ID: r.ID, Name: r.Name}
		if r.UserID != nil {
			g.GrantedBy = &domain.UserRef{ID: *r.UserID, Name: deref(r.UserName), Email: deref(r.UserEmail), AvatarURL: domain.Avatar(deref(r.UserAvatar))}
		}
		out = append(out, g)
	}
	return out, nil
}

// saveGrants records the grants of a key's or webhook's new private_team_ids teams, set by
// p: teams no longer listed lose their grant, and p grants every listed team they are on
// (taking over a grant someone else made). Teams p is not on keep their grant as it is.
func saveGrants(ctx context.Context, tx pgx.Tx, p *auth.Principal, owner string, ownerID uuid.UUID, teams []uuid.UUID) error {
	if teams == nil {
		teams = []uuid.UUID{}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM private_team_grants WHERE workspace_id = $1 AND `+owner+` = $2 AND NOT team_id = ANY($3::uuid[])`,
		p.WorkspaceID, ownerID, teams); err != nil {
		return err
	}
	if p.Kind != auth.KindUser || len(teams) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO private_team_grants (workspace_id, team_id, `+owner+`, granted_by)
		SELECT $1, tm.team_id, $2, tm.user_id FROM team_members tm JOIN teams t ON t.id = tm.team_id AND t.workspace_id = $1 AND t.private
		WHERE tm.user_id = $3 AND tm.team_id = ANY($4::uuid[])
		ON CONFLICT (`+owner+`, team_id) WHERE `+owner+` IS NOT NULL
		DO UPDATE SET granted_by = EXCLUDED.granted_by, created_at = now() WHERE private_team_grants.granted_by <> EXCLUDED.granted_by`,
		p.WorkspaceID, ownerID, p.UserID, teams)
	return err
}

// dropLapsedGrants removes the grants of workspace wid whose granter is no longer a lead,
// member or observer of the team, and the team from that key's or webhook's
// private_team_ids, in tx. Each changed key and webhook is audit-logged (api_key.updated,
// webhook.updated with reason granter_left_team); a webhook also publishes
// webhook.updated. It returns the changed keys: their sockets carry their teams and
// reconnect after the commit.
func dropLapsedGrants(ctx context.Context, tx pgx.Tx, p *auth.Principal, wid uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `DELETE FROM private_team_grants g WHERE g.workspace_id = $1
		AND NOT EXISTS (SELECT 1 FROM team_members tm WHERE tm.team_id = g.team_id AND tm.user_id = g.granted_by)
		RETURNING g.team_id, COALESCE(g.api_key_id, g.webhook_id), g.api_key_id IS NOT NULL`, wid)
	if err != nil {
		return nil, err
	}
	type lapsed struct {
		team, owner uuid.UUID
		key         bool
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (lapsed, error) {
		var l lapsed
		err := r.Scan(&l.team, &l.owner, &l.key)
		return l, err
	})
	if err != nil || len(list) == 0 {
		return nil, err
	}
	type change struct {
		owner uuid.UUID
		key   bool
		teams []uuid.UUID
	}
	var changes []*change
	for _, l := range list {
		i := slices.IndexFunc(changes, func(c *change) bool { return c.owner == l.owner })
		if i < 0 {
			changes = append(changes, &change{owner: l.owner, key: l.key})
			i = len(changes) - 1
		}
		changes[i].teams = append(changes[i].teams, l.team)
	}
	slices.SortFunc(changes, func(a, b *change) int { return compareUUID(a.owner, b.owner) })
	var keys []uuid.UUID
	for _, c := range changes {
		slices.SortFunc(c.teams, compareUUID)
		table, entity := "webhooks", "webhook"
		if c.key {
			table, entity = "api_keys", "api_key"
		}
		var left []uuid.UUID
		if err := tx.QueryRow(ctx, `UPDATE `+table+` SET private_team_ids = ARRAY(
				SELECT x FROM unnest(private_team_ids) x WHERE NOT x = ANY($3::uuid[]) ORDER BY x)
			WHERE workspace_id = $1 AND id = $2 RETURNING private_team_ids`, wid, c.owner, c.teams).Scan(&left); err != nil {
			return nil, err
		}
		if c.key {
			keys = append(keys, c.owner)
		} else {
			if _, err := tx.Exec(ctx, `UPDATE webhooks SET updated_at = now() WHERE id = $1`, c.owner); err != nil {
				return nil, err
			}
			w, err := loadWebhook(ctx, tx, wid, c.owner)
			if err != nil {
				return nil, err
			}
			if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: wid, Type: "webhook.updated", Data: map[string]any{"webhook": w}}); err != nil {
				return nil, err
			}
		}
		if err := audit.Log(ctx, tx, p, wid, entity+".updated", entity, &c.owner, map[string]any{
			"private_team_ids": uuidStrings(left), "removed_private_team_ids": uuidStrings(c.teams), "reason": "granter_left_team"}); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

// checkPrivateTeams validates the new private_team_ids of a key or webhook that currently has
// current, for principal p. Every id must be a private team of p's workspace; every id not
// already in current must be a team p is on. It returns the deduplicated list.
func checkPrivateTeams(ctx context.Context, q db.Querier, p *auth.Principal, current, next []string) ([]uuid.UUID, error) {
	if len(next) > maxPrivateTeams {
		return nil, httpx.Validation("Choose at most 200 private teams.")
	}
	out := make([]uuid.UUID, 0, len(next))
	for _, s := range next {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, httpx.Validation("private_team_ids must be team ids.")
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return out, nil
	}
	var private int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM teams WHERE workspace_id = $1 AND private AND id = ANY($2::uuid[])`,
		p.WorkspaceID, out).Scan(&private); err != nil {
		return nil, err
	}
	if private != len(out) {
		return nil, httpx.Validation("private_team_ids must be private teams of this workspace.")
	}
	var added []uuid.UUID
	for _, id := range out {
		if !slices.Contains(current, id.String()) {
			added = append(added, id)
		}
	}
	if len(added) == 0 {
		return out, nil
	}
	if err := requireOnTeams(ctx, q, p, added); err != nil {
		return nil, err
	}
	return out, nil
}

// requireOnTeams allows only a user who is a lead, member or observer of every team in
// teams (403 forbidden otherwise). An empty list allows anyone.
func requireOnTeams(ctx context.Context, q db.Querier, p *auth.Principal, teams []uuid.UUID) error {
	if len(teams) == 0 {
		return nil
	}
	if p.Kind != auth.KindUser {
		return httpx.Forbidden()
	}
	var on int
	if err := q.QueryRow(ctx, `SELECT count(DISTINCT tm.team_id) FROM team_members tm JOIN teams t ON t.id = tm.team_id
		WHERE t.workspace_id = $1 AND tm.user_id = $2 AND tm.team_id = ANY($3::uuid[])`,
		p.WorkspaceID, p.UserID, teams).Scan(&on); err != nil {
		return err
	}
	if on != len(teams) {
		return httpx.Forbidden()
	}
	return nil
}

// grantedTeams returns the private teams of p's workspace still named by ids.
func grantedTeams(ctx context.Context, q db.Querier, wid uuid.UUID, ids []string) ([]uuid.UUID, error) {
	var out []uuid.UUID
	err := q.QueryRow(ctx, `SELECT COALESCE(array_agg(id ORDER BY id), '{}') FROM teams
		WHERE workspace_id = $1 AND private AND id::text = ANY($2::text[])`, wid, ids).Scan(&out)
	return out, err
}

// dropTeamGrants removes a team from every API key's and webhook's private_team_ids, when it
// becomes public or is deleted: making it private again never restores their access.
func dropTeamGrants(ctx context.Context, q db.Querier, wid, teamID uuid.UUID) error {
	if _, err := q.Exec(ctx, `DELETE FROM private_team_grants WHERE workspace_id = $1 AND team_id = $2`, wid, teamID); err != nil {
		return err
	}
	for _, table := range []string{"api_keys", "webhooks"} {
		if _, err := q.Exec(ctx, `UPDATE `+table+` SET private_team_ids = array_remove(private_team_ids, $2)
			WHERE workspace_id = $1 AND $2 = ANY(private_team_ids)`, wid, teamID); err != nil {
			return err
		}
	}
	return nil
}

// apiKeyGranted reports whether p is an API key whose private_team_ids include teamID.
func apiKeyGranted(ctx context.Context, q db.Querier, p *auth.Principal, teamID uuid.UUID) (bool, error) {
	if p.Kind != auth.KindAPIKey {
		return false, nil
	}
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM api_keys WHERE workspace_id = $1 AND id = $2 AND $3 = ANY(private_team_ids))`,
		p.WorkspaceID, p.APIKeyID, teamID).Scan(&ok)
	return ok, err
}

// eventsWiden reports whether a webhook's event list next subscribes to anything prev did not.
func eventsWiden(prev, next []string) bool {
	if slices.Contains(prev, "*") {
		return false
	}
	for _, e := range next {
		if !slices.Contains(prev, e) {
			return true
		}
	}
	return false
}

// sameIDs reports whether the stored list a holds exactly the ids in b.
func sameIDs(a []string, b []uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	for _, id := range b {
		if !slices.Contains(a, id.String()) {
			return false
		}
	}
	return true
}

// uuidStrings renders ids as strings, never nil.
func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}
