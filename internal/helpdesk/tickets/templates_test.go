package tickets_test

import (
	"strings"
	"testing"

	"github.com/useopendesk/opendesk/internal/domain"
)

func TestRepliesFillTemplates(t *testing.T) {
	e, ws := newEnv(t)
	e.Exec(`UPDATE members SET title = 'Support lead', signature_html = '<p>{{ agent.first_name }} from {{ team.name }}</p>' WHERE workspace_id = $1 AND user_id = $2`,
		ws.ID, ws.Agent.ID)
	var agentName string
	e.Scalar(`SELECT name FROM users WHERE id = $1`, &agentName, ws.Agent.ID)
	first, _, _ := strings.Cut(agentName, " ")
	tk := createTicket(t, e, ws, map[string]any{"contact": map[string]any{"email": "rita@example.com", "name": "Rita Moreno"}, "kind": "note"})

	var msg domain.Message
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Agent.As(ws), map[string]any{"kind": "reply",
		"body_html": `<p>Hi {{ contact.first_name }}, {{ agent.first_name }} ({{ agent.title | downcase }}) on #{{ticket.number}}. <a href="{{ brand.help_center_url | default: 'https://help.example.com' }}">Help</a> {{ not.a_variable }}</p>`}).
		Want(t, 201).JSON(t, &msg)
	want := "Hi Rita, " + first + " (support lead) on #"
	if !strings.Contains(msg.BodyHTML, want) || !strings.Contains(msg.BodyHTML, `href="https://help.example.com"`) ||
		!strings.Contains(msg.BodyHTML, "{{ not.a_variable }}") || !strings.Contains(msg.BodyText, want) {
		t.Fatalf("reply %q / %q", msg.BodyHTML, msg.BodyText)
	}
	// Notes are never filled.
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/messages", ws.Agent.As(ws), map[string]any{"kind": "note", "body_html": "<p>{{ contact.first_name }}</p>"}).
		Want(t, 201).JSON(t, &msg)
	if !strings.Contains(msg.BodyHTML, "{{ contact.first_name }}") {
		t.Fatalf("note filled: %q", msg.BodyHTML)
	}

	var vars struct {
		Values        map[string]string `json:"values"`
		SignatureHTML string            `json:"signature_html"`
	}
	e.Do("GET", "/v1/tickets/"+tk.ID.String()+"/variables", ws.Agent.As(ws), nil).Want(t, 200).JSON(t, &vars)
	if vars.Values["contact.name"] != "Rita Moreno" || vars.Values["agent.title"] != "Support lead" || vars.SignatureHTML != "<p>"+first+" from "+vars.Values["team.name"]+"</p>" {
		t.Fatalf("variables %+v", vars)
	}
	e.Do("GET", "/v1/tickets/"+tk.ID.String()+"/variables", ws.Viewer.As(ws), nil).Want(t, 403)

	var rendered struct {
		BodyHTML string `json:"body_html"`
	}
	e.Do("POST", "/v1/tickets/"+tk.ID.String()+"/render", ws.Agent.As(ws),
		map[string]any{"body_html": `<p>{% if contact.phone %}Call?{% else %}Hello {{ contact.first_name | upcase }}{% endif %}</p><script>x</script>`}).
		Want(t, 200).JSON(t, &rendered)
	if rendered.BodyHTML != "<p>Hello RITA</p>" {
		t.Fatalf("rendered %q", rendered.BodyHTML)
	}
}
