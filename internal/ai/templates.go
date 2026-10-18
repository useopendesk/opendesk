package ai

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/templates"
)

// fillAnswer fills the template tags of an answer the AI agent sends, such as those of a
// saved reply it drew on (contracts/templates.md), with its bot as the agent. An answer
// that cannot be filled goes out as written.
func (s *Service) fillAnswer(ctx context.Context, tx pgx.Tx, wid, tid uuid.UUID, bot, body string) string {
	if !templates.HasTags(body) {
		return body
	}
	filled, err := hd.FillReply(ctx, tx, s.d.Cfg, templates.Ref{WorkspaceID: wid, TicketID: &tid, BotName: bot}, body)
	if err != nil {
		s.d.Log.Warn("answer not filled", "ticket_id", tid, "error", err)
		return body
	}
	return filled
}
