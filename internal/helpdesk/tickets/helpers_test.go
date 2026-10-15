package tickets_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/helpdesk/tickets"
	"github.com/useopendesk/opendesk/internal/ids"
	"github.com/useopendesk/opendesk/internal/jobs"
)

func newEnv(t *testing.T) (*hdtest.Env, hdtest.WS) {
	t.Helper()
	e := hdtest.New(t, tickets.Register)
	return e, e.NewWorkspace()
}

// runTriggers runs queued ticket triggers for the workspace until none are left.
func runTriggers(t *testing.T, e *hdtest.Env, wid uuid.UUID) int {
	t.Helper()
	n := 0
	for i := 0; i < 20; i++ {
		batch := e.Jobs(jobs.TicketTriggerArgs{}.Kind(), wid)
		if len(batch) == 0 {
			return n
		}
		for _, raw := range batch {
			var a jobs.TicketTriggerArgs
			if err := json.Unmarshal(raw, &a); err != nil {
				t.Fatal(err)
			}
			if err := tickets.RunTrigger(e.Ctx, e.D, a); err != nil {
				t.Fatalf("trigger %s: %v", a.Trigger, err)
			}
			n++
		}
	}
	t.Fatal("triggers did not settle")
	return n
}

func createTicket(t *testing.T, e *hdtest.Env, ws hdtest.WS, body map[string]any) domain.Ticket {
	t.Helper()
	if _, ok := body["subject"]; !ok {
		body["subject"] = "Help needed"
	}
	if _, ok := body["body_html"]; !ok {
		body["body_html"] = "<p>Hello there</p>"
	}
	var tk domain.Ticket
	e.Do("POST", "/v1/tickets", ws.Agent.As(ws), body).Want(t, 201).JSON(t, &tk)
	return tk
}

func getTicket(t *testing.T, e *hdtest.Env, ws hdtest.WS, id uuid.UUID) domain.Ticket {
	t.Helper()
	var tk domain.Ticket
	e.Do("GET", "/v1/tickets/"+id.String(), ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &tk)
	return tk
}

func addMailbox(t *testing.T, e *hdtest.Env, ws hdtest.WS) uuid.UUID {
	t.Helper()
	id := ids.New()
	e.Exec(`INSERT INTO mailboxes (id, workspace_id, name, email, display_name, inbound_kind, forwarding_token, outbound_kind, brand_id)
		VALUES ($1, $2, 'Support', $3, 'Acme Support', 'forwarding', $4, 'system', $5)`, id, ws.ID, "support-"+id.String()[24:]+"@acme.test", "fw-"+id.String(), ws.BrandID)
	return id
}

func addTeam(t *testing.T, e *hdtest.Env, ws hdtest.WS, members ...uuid.UUID) uuid.UUID {
	t.Helper()
	id := ids.New()
	e.Exec(`INSERT INTO teams (id, workspace_id, name) VALUES ($1, $2, $3)`, id, ws.ID, "Team "+id.String()[24:])
	for _, m := range members {
		e.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ($1, $2)`, id, m)
	}
	return id
}

// inbound adds a customer message through the domain, as the email pipeline would.
func inbound(t *testing.T, e *hdtest.Env, ws hdtest.WS, tid uuid.UUID, text string) domain.Message {
	t.Helper()
	var m domain.Message
	err := e.D.DB.Tx(e.Ctx, func(tx pgx.Tx) error {
		var cid *uuid.UUID
		if err := tx.QueryRow(e.Ctx, `SELECT contact_id FROM tickets WHERE id = $1`, tid).Scan(&cid); err != nil {
			return err
		}
		var err error
		m, err = domain.AddMessage(e.Ctx, tx, e.D.Jobs, domain.NewMessage{WorkspaceID: ws.ID, TicketID: tid, Kind: "reply",
			Direction: "inbound", AuthorType: "contact", AuthorContactID: cid, BodyHTML: "<p>" + text + "</p>", BodyText: text,
			FromEmail: "customer@example.com"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func countRows(t *testing.T, e *hdtest.Env, sql string, args ...any) int {
	t.Helper()
	var n int
	e.Scalar(sql, &n, args...)
	return n
}
