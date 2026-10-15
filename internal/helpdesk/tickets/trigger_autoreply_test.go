package tickets_test

import (
	"testing"

	"github.com/useopendesk/opendesk/internal/jobs"
)

func TestAutomationEmailRepliesDoNotLoop(t *testing.T) {
	e, ws := newEnv(t)
	mb := addMailbox(t, e, ws)
	tk := createTicket(t, e, ws, map[string]any{"channel": "email", "mailbox_id": mb.String(), "kind": "note",
		"contact": map[string]any{"email": "customer@example.com", "name": "Carla"}})
	runTriggers(t, e, ws.ID)
	addAutomation(t, e, ws, jobs.TriggerCustomerReplied, 0, []map[string]string{},
		[]map[string]string{{"type": "send_reply", "value": "<p>Thanks, we are on it.</p>"}})
	replies := func() int {
		return countRows(t, e, `SELECT count(*) FROM messages WHERE ticket_id = $1 AND author_type = 'system' AND kind = 'reply'`, tk.ID)
	}

	// Mail nothing automatic may answer (automatic or list mail, a workspace address, a
	// sender that failed authentication) gets no automation reply.
	for _, col := range []string{"no_auto_reply", "auth_failed"} {
		m := inbound(t, e, ws, tk.ID, "Automatic notice")
		e.Exec(`UPDATE messages SET `+col+` = true WHERE id = $1`, m.ID)
		runTriggers(t, e, ws.ID)
		if n := replies(); n != 0 {
			t.Fatalf("%s: automation answered (%d)", col, n)
		}
	}

	// A person's mail is answered, at most three times per ticket per hour.
	for i := 0; i < 5; i++ {
		inbound(t, e, ws, tk.ID, "Any news?")
		runTriggers(t, e, ws.ID)
	}
	if n := replies(); n != 3 {
		t.Fatalf("automation replies %d, want 3", n)
	}
	var pending int
	e.Scalar(`SELECT count(*) FROM messages WHERE ticket_id = $1 AND author_type = 'system' AND delivery_status = 'pending'`, &pending, tk.ID)
	if pending != 3 {
		t.Fatalf("replies queued to send %d", pending)
	}
}
