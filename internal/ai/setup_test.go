package ai

import (
	"testing"

	"github.com/useopendesk/opendesk/internal/ai/agentsetup"
)

func setupSteps(t *testing.T, e *env) (agentsetup.Setup, map[string]agentsetup.SetupStep) {
	t.Helper()
	var s agentsetup.Setup
	e.Do("GET", "/v1/ai/agent/setup", e.admin(), nil).Want(t, 200).JSON(t, &s)
	m := map[string]agentsetup.SetupStep{}
	for _, st := range s.Steps {
		if st.Title == "" || st.Description == "" || st.Href == "" {
			t.Fatalf("step without copy: %+v", st)
		}
		m[st.ID] = st
	}
	return s, m
}

func TestAgentSetupFollowsTheWorkspace(t *testing.T) {
	e := newEnv(t, opts{})
	e.Do("GET", "/v1/ai/agent/setup", e.agent(), nil).Want(t, 403)

	s, m := setupSteps(t, e)
	if !m[agentsetup.StepAIOn].Done || s.Complete || s.DoneCount != 1 || s.Total != 6 {
		t.Fatalf("fresh workspace: %+v", s)
	}
	if !m[agentsetup.StepTools].Optional || m[agentsetup.StepIdentity].Optional {
		t.Fatalf("optional flags: %+v", s.Steps)
	}
	// Email answers are offered where mailboxes have the per-mailbox setting.
	var hasEmail bool
	e.Scalar(`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'mailboxes' AND column_name = 'ai_answers')`, &hasEmail)
	if st, ok := m[agentsetup.StepEmailAnswers]; ok != hasEmail || (ok && (!st.Optional || st.Done)) {
		t.Fatalf("email answers step: %v %+v", hasEmail, st)
	}

	// Telling the bot how to help (contracts/bots.md) is the identity step.
	e.Exec(`INSERT INTO bots (workspace_id, name, look, personality, is_default) VALUES ($1, 'Ollie', '{}', '{"instructions":"Answer like the Acme team."}', true)`, e.WS.ID)
	e.Exec(`INSERT INTO kb_articles (workspace_id, title, slug, status, published_at) VALUES ($1, 'Refunds', 'refunds', 'published', now())`, e.WS.ID)
	e.Exec(`INSERT INTO ai_usage (workspace_id, feature, micro_usd) VALUES ($1, 'playground', 0)`, e.WS.ID)
	e.Exec(`UPDATE workspaces SET widget_seen_at = now() WHERE id = $1`, e.WS.ID)
	s, m = setupSteps(t, e)
	if !m[agentsetup.StepIdentity].Done || !m[agentsetup.StepKnowledge].Done || !m[agentsetup.StepPlayground].Done ||
		!m[agentsetup.StepWidget].Done || m[agentsetup.StepAnswersOn].Done || s.Complete || s.DoneCount != 5 {
		t.Fatalf("most steps: %+v", s)
	}

	// Answers on completes it; optional steps don't count.
	e.Do("PATCH", "/v1/ai/settings", e.admin(), map[string]any{"widget_answers": true}).Want(t, 200)
	s, m = setupSteps(t, e)
	if !s.Complete || s.DoneCount != s.Total || m[agentsetup.StepTools].Done {
		t.Fatalf("complete: %+v", s)
	}
	e.Exec(`INSERT INTO ai_mcp_servers (workspace_id, name, slug, url, status) VALUES ($1, 'Shop', 'shop', 'https://shop.example.com/mcp', 'connected')`, e.WS.ID)
	if _, m = setupSteps(t, e); !m[agentsetup.StepTools].Done {
		t.Fatal("tools step after a connected server")
	}

	// AI off for the workspace undoes the first step.
	e.Do("PATCH", "/v1/ai/settings", e.admin(), map[string]any{"enabled": false}).Want(t, 200)
	if s, m = setupSteps(t, e); m[agentsetup.StepAIOn].Done || s.Complete {
		t.Fatalf("ai off: %+v", s)
	}
}

func TestAgentSetupWithoutProvider(t *testing.T) {
	e := newEnv(t, opts{disabled: true})
	if _, m := setupSteps(t, e); m[agentsetup.StepAIOn].Done {
		t.Fatal("ai_on without a provider")
	}
}
