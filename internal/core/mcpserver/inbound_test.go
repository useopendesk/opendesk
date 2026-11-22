package mcpserver

import (
	"fmt"
	"testing"

	"github.com/useopendesk/opendesk/internal/auth"
)

// A request that came in somewhere else is logged as the customer's own message, as Log
// request does in the app: nothing is sent and the team's answer is still owed.
func TestInboundMessagesThroughMCP(t *testing.T) {
	e := newEnv(t)
	s := e.session(e.APIKey(e.WS, auth.ScopeTicketsRead, auth.ScopeTicketsWrite))

	var created struct {
		ID     string `json:"id"`
		Number int64  `json:"number"`
	}
	call(t, s, "create_ticket", map[string]any{"email": "ana@example.com", "name": "Ana", "subject": "Phone call",
		"body": "My invoice PDF is empty.", "direction": "inbound"}, &created)
	if created.Number == 0 {
		t.Fatalf("create_ticket: %+v", created)
	}
	var dir, author string
	var n int
	e.Scalar(`SELECT count(*) FROM messages WHERE ticket_id = $1 AND kind = 'reply' AND direction = 'inbound' AND author_type = 'contact'`, &n, created.ID)
	e.Scalar(`SELECT direction FROM messages WHERE ticket_id = $1`, &dir, created.ID)
	e.Scalar(`SELECT author_type FROM messages WHERE ticket_id = $1`, &author, created.ID)
	if n != 1 || dir != "inbound" || author != "contact" {
		t.Fatalf("first message: %d %s %s", n, dir, author)
	}

	// A second call on the same ticket.
	call(t, s, "reply_to_ticket", map[string]any{"ticket": fmt.Sprintf("#%d", created.Number), "body": "Called again, still empty.", "direction": "inbound"}, nil)
	e.Scalar(`SELECT count(*) FROM messages WHERE ticket_id = $1 AND direction = 'inbound' AND kind = 'reply'`, &n, created.ID)
	if n != 2 {
		t.Fatalf("inbound messages: %d", n)
	}
	var out int
	e.Scalar(`SELECT count(*) FROM messages WHERE ticket_id = $1 AND direction = 'outbound'`, &out, created.ID)
	if out != 0 {
		t.Fatalf("an inbound message sent something: %d outbound", out)
	}

	// The API's rules hold: replies only, no cc, no mailbox, and something said.
	for name, args := range map[string]map[string]any{
		"create_ticket":   {"email": "b@example.com", "subject": "x", "body": "hi", "direction": "inbound", "kind": "note"},
		"create_ticket ":  {"email": "b@example.com", "subject": "x", "direction": "inbound"},
		"create_ticket  ": {"email": "b@example.com", "subject": "x", "body": "hi", "direction": "sideways"},
		"reply_to_ticket": {"ticket": created.ID, "body": "hi", "direction": "inbound", "cc": []string{"c@example.com"}},
	} {
		tool := name
		for tool[len(tool)-1] == ' ' {
			tool = tool[:len(tool)-1]
		}
		if res := call(t, s, tool, args, nil); !res.IsError {
			t.Errorf("%s %v was accepted", tool, args)
		}
	}
}
