// Package agentsetup computes the AI agent's setup checklist (contracts/agent.md
// "Setup") from a workspace's real data. The ai area serves it at
// GET /v1/ai/agent/setup; core's "Get set up" checklist uses it for its AI agent step.
// It reads tables only, so any area can use it.
package agentsetup

import (
	"context"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/db"
)

// Step ids, in checklist order.
const (
	StepAIOn         = "ai_on"
	StepIdentity     = "identity"
	StepKnowledge    = "knowledge"
	StepTools        = "tools"
	StepPlayground   = "playground"
	StepWidget       = "widget"
	StepAnswersOn    = "answers_on"
	StepEmailAnswers = "email_answers"
)

// SetupStep is one item of the checklist.
type SetupStep struct {
	ID          string `json:"id" enum:"ai_on,identity,knowledge,tools,playground,widget,answers_on,email_answers"`
	Title       string `json:"title"`
	Description string `json:"description" doc:"Why the step matters, in one line"`
	Done        bool   `json:"done"`
	Optional    bool   `json:"optional" doc:"Optional steps do not count toward completion"`
	Href        string `json:"href" doc:"App path where the step is done"`
}

// Setup is the checklist. Completion counts the required steps only.
type Setup struct {
	Steps     []SetupStep `json:"steps"`
	DoneCount int         `json:"done_count" doc:"Required steps done"`
	Total     int         `json:"total" doc:"Required steps"`
	Complete  bool        `json:"complete" doc:"Every required step is done"`
}

// Load computes the checklist for wid. instanceAI reports an AI provider on the
// instance; a workspace that brought its own provider key counts as AI available too.
func Load(ctx context.Context, q db.Querier, wid uuid.UUID, instanceAI bool) (Setup, error) {
	var enabled, identity, knowledge, tools, playground, widget, answers bool
	err := q.QueryRow(ctx, `SELECT
		COALESCE(w.settings -> 'ai' ->> 'enabled', 'true') = 'true',
		EXISTS (SELECT 1 FROM bots WHERE workspace_id = $1 AND (NOT is_default OR name <> 'Ollie' OR avatar <> ''
			OR COALESCE(personality ->> 'instructions', '') <> '' OR updated_at > created_at)),
		EXISTS (SELECT 1 FROM kb_articles WHERE workspace_id = $1 AND status = 'published')
			OR EXISTS (SELECT 1 FROM ai_knowledge_sources WHERE workspace_id = $1 AND status = 'ready' AND enabled),
		EXISTS (SELECT 1 FROM ai_mcp_servers WHERE workspace_id = $1 AND enabled AND status = 'connected'),
		EXISTS (SELECT 1 FROM ai_usage WHERE workspace_id = $1 AND feature = 'playground'),
		w.widget_seen_at IS NOT NULL OR EXISTS (SELECT 1 FROM widgets WHERE workspace_id = $1 AND seen_at IS NOT NULL),
		COALESCE(w.settings -> 'ai' ->> 'widget_answers', 'false') = 'true'
		FROM workspaces w WHERE w.id = $1`, wid).
		Scan(&enabled, &identity, &knowledge, &tools, &playground, &widget, &answers)
	if err != nil {
		return Setup{}, err
	}
	available := instanceAI
	if !available {
		if available, err = ownKey(ctx, q, wid); err != nil {
			return Setup{}, err
		}
	}
	steps := []SetupStep{
		{ID: StepAIOn, Title: "Turn on AI", Description: "Ollie needs AI switched on for the workspace before it can answer anyone.",
			Done: available && enabled, Href: "/settings/ai"},
		{ID: StepIdentity, Title: "Build your bot and tell it how to help", Description: "A name, a look and instructions make answers sound like your team.",
			Done: identity, Href: "/settings/ai/bots"},
		{ID: StepKnowledge, Title: "Give it something to know", Description: "Ollie answers only from published help articles and ready knowledge sources.",
			Done: knowledge, Href: "/settings/ai/knowledge"},
		{ID: StepTools, Title: "Connect tools", Description: "Let Ollie look up orders or accounts in your own systems.",
			Done: tools, Optional: true, Href: "/settings/ai/tools"},
		{ID: StepPlayground, Title: "Test it in the playground", Description: "Ask a few real questions and check the answers before visitors do.",
			Done: playground, Href: "/settings/ai/playground"},
		{ID: StepWidget, Title: "Install the chat widget", Description: "Visitors reach Ollie through the widget on your site.",
			Done: widget, Href: "/settings/widget"},
		{ID: StepAnswersOn, Title: "Let Ollie answer chats", Description: "Turn on answers so Ollie replies to visitors and hands off when it should.",
			Done: answers, Href: "/settings/ai/behaviour"},
	}
	// Email answers are a per-mailbox setting; offer the step only where it exists.
	if ok, err := hasColumn(ctx, q, "mailboxes", "ai_answers"); err != nil {
		return Setup{}, err
	} else if ok {
		var email bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM mailboxes WHERE workspace_id = $1 AND ai_answers)`, wid).Scan(&email); err != nil {
			return Setup{}, err
		}
		steps = append(steps, SetupStep{ID: StepEmailAnswers, Title: "Answer email too", Description: "Turn on Ollie for a mailbox to draft or send email replies.",
			Done: email, Optional: true, Href: "/settings/mailboxes"})
	}
	s := Setup{Steps: steps}
	for _, st := range steps {
		if st.Optional {
			continue
		}
		s.Total++
		if st.Done {
			s.DoneCount++
		}
	}
	s.Complete = s.DoneCount == s.Total
	return s, nil
}

// ownKey reports a working provider key of the workspace's own, where that exists.
func ownKey(ctx context.Context, q db.Querier, wid uuid.UUID) (bool, error) {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT to_regclass('ai_provider_keys') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		return false, err
	}
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ai_provider_keys WHERE workspace_id = $1 AND status = 'ok')`, wid).Scan(&ok)
	return ok, err
}

func hasColumn(ctx context.Context, q db.Querier, table, column string) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2)`, table, column).Scan(&ok)
	return ok, err
}
