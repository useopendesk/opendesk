package tickets_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
)

// assignNew creates n tickets (with extra body members) and returns who got each.
func assignNew(t *testing.T, e *hdtest.Env, ws hdtest.WS, n int, body map[string]any) []*uuid.UUID {
	t.Helper()
	var out []*uuid.UUID
	for i := 0; i < n; i++ {
		b := map[string]any{"kind": "note"}
		for k, v := range body {
			b[k] = v
		}
		tk := createTicket(t, e, ws, b)
		runTriggers(t, e, ws.ID)
		if a := getTicket(t, e, ws, tk.ID).Assignee; a != nil {
			id := a.ID
			out = append(out, &id)
		} else {
			out = append(out, nil)
		}
	}
	return out
}

func tally(got []*uuid.UUID) map[uuid.UUID]int {
	m := map[uuid.UUID]int{}
	for _, g := range got {
		if g != nil {
			m[*g]++
		}
	}
	return m
}

func TestRoundRobinFairnessAndAvailability(t *testing.T) {
	e, ws := newEnv(t)
	e.SetSettings(ws, map[string]any{"auto_assign": "round_robin"})
	got := tally(assignNew(t, e, ws, 9, nil))
	for _, u := range []uuid.UUID{ws.Owner.ID, ws.Admin.ID, ws.Agent.ID} {
		if got[u] != 3 {
			t.Fatalf("round robin is not fair: %v", got)
		}
	}
	if got[ws.Viewer.ID] != 0 {
		t.Fatal("viewer assigned")
	}

	// Offline and away members are skipped.
	e.Exec(`UPDATE members SET availability = 'offline' WHERE workspace_id = $1 AND user_id = $2`, ws.ID, ws.Admin.ID)
	e.Exec(`UPDATE members SET availability = 'away' WHERE workspace_id = $1 AND user_id = $2`, ws.ID, ws.Owner.ID)
	got = tally(assignNew(t, e, ws, 3, nil))
	if got[ws.Agent.ID] != 3 {
		t.Fatalf("only the online agent should be picked: %v", got)
	}

	// Auto-away: online but not seen within auto_away_minutes counts as away.
	e.Exec(`UPDATE members SET availability = 'online' WHERE workspace_id = $1`, ws.ID)
	e.SetSettings(ws, map[string]any{"auto_away_minutes": 30})
	e.Exec(`UPDATE users SET last_seen_at = now() - interval '2 hours' WHERE id IN ($1, $2)`, ws.Owner.ID, ws.Admin.ID)
	e.Exec(`UPDATE users SET last_seen_at = now() WHERE id = $1`, ws.Agent.ID)
	got = tally(assignNew(t, e, ws, 2, nil))
	if got[ws.Agent.ID] != 2 {
		t.Fatalf("auto-away members were assigned: %v", got)
	}
	e.SetSettings(ws, map[string]any{"auto_away_minutes": 0})

	// Nobody eligible: the ticket stays unassigned.
	e.Exec(`UPDATE members SET availability = 'offline' WHERE workspace_id = $1`, ws.ID)
	if got := assignNew(t, e, ws, 1, nil); got[0] != nil {
		t.Fatal("assigned with nobody online")
	}
}

func TestAssignmentCapsAndLoadBalancing(t *testing.T) {
	e, ws := newEnv(t)
	// Give the owner three open tickets and the admin one.
	for i := 0; i < 3; i++ {
		createTicket(t, e, ws, map[string]any{"kind": "note", "assignee_id": ws.Owner.ID.String()})
	}
	createTicket(t, e, ws, map[string]any{"kind": "note", "assignee_id": ws.Admin.ID.String()})
	runTriggers(t, e, ws.ID)

	e.SetSettings(ws, map[string]any{"auto_assign": "load_balanced"})
	// The agent (0 open) gets the next one, then the agent and the admin share the load.
	got := assignNew(t, e, ws, 3, nil)
	if *got[0] != ws.Agent.ID {
		t.Fatalf("load balanced picked %v, want the agent", got[0])
	}
	counts := tally(got)
	if counts[ws.Owner.ID] != 0 || counts[ws.Agent.ID]+counts[ws.Admin.ID] != 3 {
		t.Fatalf("load balanced %v", counts)
	}

	// A cap removes a member from the pool once reached.
	e.Exec(`UPDATE members SET max_open_tickets = 2 WHERE workspace_id = $1 AND user_id IN ($2, $3)`, ws.ID, ws.Agent.ID, ws.Admin.ID)
	e.Exec(`UPDATE members SET max_open_tickets = 3 WHERE workspace_id = $1 AND user_id = $2`, ws.ID, ws.Owner.ID)
	if got := assignNew(t, e, ws, 1, nil); got[0] != nil {
		t.Fatalf("assigned over every cap: %v", *got[0])
	}
	e.Exec(`UPDATE members SET max_open_tickets = NULL WHERE workspace_id = $1 AND user_id = $2`, ws.ID, ws.Owner.ID)
	if got := assignNew(t, e, ws, 1, nil); got[0] == nil || *got[0] != ws.Owner.ID {
		t.Fatal("uncapped owner should take it")
	}

	// The activity is attributed to the system with the mode.
	var actor, via string
	e.Scalar(`SELECT actor_type || ':' || (data ->> 'via') FROM activities WHERE workspace_id = $1 AND type = 'assigned'
		ORDER BY created_at DESC LIMIT 1`, &actor, ws.ID)
	via = actor
	if via != "system:load_balanced" {
		t.Fatalf("activity %s", via)
	}
}

func TestAssignmentRules(t *testing.T) {
	e, ws := newEnv(t)
	e.SetSettings(ws, map[string]any{"auto_assign": "round_robin"})
	team := addTeam(t, e, ws, ws.Agent.ID)
	mb := addMailbox(t, e, ws)

	// A team rule of none overrides the workspace default.
	e.Exec(`INSERT INTO assignment_rules (workspace_id, team_id, mode) VALUES ($1, $2, 'none')`, ws.ID, team)
	if got := assignNew(t, e, ws, 1, map[string]any{"team_id": team.String()}); got[0] != nil {
		t.Fatal("team rule none was ignored")
	}
	// A mailbox rule applies to its tickets; the mailbox's team is the pool.
	e.Exec(`UPDATE mailboxes SET team_id = $2 WHERE id = $1`, mb, team)
	e.Exec(`INSERT INTO assignment_rules (workspace_id, mailbox_id, mode) VALUES ($1, $2, 'load_balanced')`, ws.ID, mb)
	e.SetSettings(ws, map[string]any{"auto_assign": "none"})
	// The address routes the ticket to its team, whose rule wins over the mailbox's.
	if got := assignNew(t, e, ws, 1, map[string]any{"mailbox_id": mb.String(), "channel": "api"}); got[0] != nil {
		t.Fatal("team rule none lost to the mailbox rule")
	}
	e.Exec(`DELETE FROM assignment_rules WHERE workspace_id = $1 AND team_id = $2`, ws.ID, team)
	got := assignNew(t, e, ws, 2, map[string]any{"mailbox_id": mb.String(), "channel": "api"})
	for _, g := range got {
		if g == nil || *g != ws.Agent.ID {
			t.Fatalf("mailbox rule %v", got)
		}
	}
	e.Exec(`INSERT INTO assignment_rules (workspace_id, team_id, mode) VALUES ($1, $2, 'none')`, ws.ID, team)
	// Deleting the team deletes its rule.
	e.Exec(`DELETE FROM teams WHERE id = $1`, team)
	var n int
	e.Scalar(`SELECT count(*) FROM assignment_rules WHERE workspace_id = $1 AND team_id IS NOT NULL`, &n, ws.ID)
	if n != 0 {
		t.Fatal("team rule survived the team")
	}
}

func TestAssignAfterTriage(t *testing.T) {
	e, ws := newEnv(t)
	prev := hd.GetAIHooks()
	t.Cleanup(func() { hd.SetAIHooks(prev) })
	hd.SetAIHooks(&hd.AIHooks{AutoTriage: func(ctx context.Context, wid, tid uuid.UUID) error {
		return e.D.DB.Tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE tickets SET custom = custom || jsonb_build_object('ai_triaged_at', $3::text)
				WHERE workspace_id = $1 AND id = $2`, wid, tid, time.Now().UTC().Format(time.RFC3339))
			return err
		})
	}})
	// Mode none, but Ollie assigns after triage (load balanced) when the workspace allows it.
	e.SetSettings(ws, map[string]any{"auto_assign": "none", "ai": map[string]any{"auto_triage": true}})
	if got := assignNew(t, e, ws, 1, nil); got[0] != nil {
		t.Fatal("assigned without assign_after_triage")
	}
	e.SetSettings(ws, map[string]any{"ai": map[string]any{"auto_triage": true, "assign_after_triage": true}})
	if got := assignNew(t, e, ws, 1, nil); got[0] == nil {
		t.Fatal("not assigned after triage")
	}
	var actor string
	e.Scalar(`SELECT actor_type || ':' || (data ->> 'via') FROM activities WHERE workspace_id = $1 AND type = 'assigned'
		ORDER BY created_at DESC LIMIT 1`, &actor, ws.ID)
	if actor != "ai:triage" {
		t.Fatalf("activity %s", actor)
	}
}

func TestBusinessHoursSLA(t *testing.T) {
	e, ws := newEnv(t)
	e.Exec(`UPDATE sla_policies SET enabled = false WHERE workspace_id = $1`, ws.ID)
	e.Exec(`INSERT INTO sla_policies (workspace_id, name, first_response_minutes, resolution_minutes, business_hours)
		VALUES ($1, 'Hours', 60, 120, true)`, ws.ID)
	// Open only on a single weekday that is not today, so the due date lands days away.
	day := []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}[(int(time.Now().UTC().Weekday())+3)%7]
	e.SetSettings(ws, map[string]any{"timezone": "UTC", "business_hours": map[string]any{"enabled": true,
		"schedule": map[string]any{day: []map[string]string{{"start": "09:00", "end": "17:00"}}}, "holidays": []any{}}})
	tk := createTicket(t, e, ws, map[string]any{"kind": "note"})
	runTriggers(t, e, ws.ID)
	got := getTicket(t, e, ws, tk.ID)
	if got.SLA == nil || got.SLA.FirstResponseDueAt == nil {
		t.Fatal("no sla")
	}
	due := got.SLA.FirstResponseDueAt.UTC()
	if due.Sub(time.Now()) < 24*time.Hour || due.Hour() != 10 || due.Minute() != 0 || day != []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}[due.Weekday()] {
		t.Fatalf("due %s, want 10:00 on %s", due, day)
	}
}
