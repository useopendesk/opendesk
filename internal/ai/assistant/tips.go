package assistant

import (
	"context"
	"fmt"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/domain"
)

// Tip is one thing worth doing now, offered under the header blob.
type Tip struct {
	Key      string `json:"key" doc:"Stable kind of tip, for dismissal"`
	Text     string `json:"text"`
	Prompt   string `json:"prompt" doc:"What to ask the assistant to act on it"`
	Severity string `json:"severity" enum:"high,medium,low"`
	Count    int    `json:"count"`
}

// TipList is GET /v1/assistant/tips.
type TipList struct {
	Tips []Tip `json:"tips"`
}

// tips are plain counts over the caller's workspace, most pressing first. No AI.
func (s *Service) tips(ctx context.Context, p *auth.Principal) ([]Tip, error) {
	var soon, atRisk, breached, staleUnassigned, mineWaiting int
	hidden, err := domain.HiddenTeams(ctx, s.d.DB.Pool, p)
	if err != nil {
		return nil, err
	}
	err = s.d.DB.Pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE due > now() AND due <= now() + interval '1 hour'),
		count(*) FILTER (WHERE due > now() + interval '1 hour' AND due <= now() + interval '4 hours' AND sla_risk >= 0.6),
		count(*) FILTER (WHERE due <= now()),
		count(*) FILTER (WHERE assignee_id IS NULL AND status = 'open' AND created_at < now() - interval '1 day'),
		count(*) FILTER (WHERE assignee_id = $2 AND status = 'open' AND last_customer_message_at IS NOT NULL
			AND last_customer_message_at >= last_message_at AND last_customer_message_at < now() - interval '4 hours')
		FROM (SELECT assignee_id, status, created_at, last_message_at, last_customer_message_at,
				CASE WHEN jsonb_typeof(custom #> '{ai_decisions,fields,sla_risk,value}') = 'number'
					THEN (custom #>> '{ai_decisions,fields,sla_risk,value}')::float8 END AS sla_risk,
				LEAST(CASE WHEN first_responded_at IS NULL THEN first_response_due_at END, resolution_due_at) AS due
			 FROM tickets tk WHERE workspace_id = $1 AND deleted_at IS NULL AND NOT spam AND status IN ('open', 'pending', 'on_hold')
			 AND `+domain.VisibleSQLUser("tk", 3, 2)+`) t`,
		p.WorkspaceID, p.UserID, hidden).Scan(&soon, &atRisk, &breached, &staleUnassigned, &mineWaiting)
	if err != nil {
		return nil, err
	}
	tips := []Tip{}
	if soon > 0 {
		tips = append(tips, Tip{Key: "sla_due_soon", Severity: "high", Count: soon,
			Text:   fmt.Sprintf("%s in the next hour. Triage %s?", pluralVerb(soon, "ticket breaches", "tickets breach")+" SLA", them(soon)),
			Prompt: "Which open tickets breach SLA in the next hour? Help me triage them: who should take each, and what priority."})
	}
	if atRisk > 0 {
		// sla_risk is Ollie's triage estimate of the work left (contracts/ai.md "Decisions").
		tips = append(tips, Tip{Key: "sla_at_risk", Severity: "medium", Count: atRisk,
			Text:   fmt.Sprintf("%s due within 4 hours and %s hard to finish in time.", pluralVerb(atRisk, "ticket is", "tickets are"), lookText(atRisk)),
			Prompt: "Which open tickets are due within 4 hours but look like a lot of work? Suggest who should take each and what to do first."})
	}
	if breached > 0 {
		tips = append(tips, Tip{Key: "sla_breached", Severity: "high", Count: breached,
			Text:   fmt.Sprintf("%s past SLA. Want a plan?", pluralVerb(breached, "open ticket is", "open tickets are")),
			Prompt: "List the open tickets that are past their SLA, oldest first, and suggest who should take each."})
	}
	if staleUnassigned > 0 {
		tips = append(tips, Tip{Key: "unassigned_old", Severity: "medium", Count: staleUnassigned,
			Text:   fmt.Sprintf("%s older than a day.", pluralVerb(staleUnassigned, "unassigned ticket is", "unassigned tickets are")),
			Prompt: "Show the open unassigned tickets older than a day and suggest an assignee for each, based on the team's workload."})
	}
	if mineWaiting > 0 {
		tips = append(tips, Tip{Key: "mine_waiting", Severity: "low", Count: mineWaiting,
			Text:   fmt.Sprintf("%s waiting on your reply for over 4 hours.", pluralVerb(mineWaiting, "customer is", "customers are")),
			Prompt: "Which of my open tickets have a customer waiting on my reply for more than 4 hours? Summarize each in one line."})
	}
	return tips, nil
}

func pluralVerb(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func them(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

func lookText(n int) string {
	if n == 1 {
		return "looks"
	}
	return "look"
}
