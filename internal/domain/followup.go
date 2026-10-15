package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/jobs"
)

// Chat follow-up (contracts/teams.md "Chat follow-up"): an agent's public reply on a chat
// goes out by email too when the visitor left an email and is not on the page. The API
// cannot see sockets, so "on the page" means the visitor's widget made a request lately:
// every widget request refreshes widget_sessions.last_seen_at (at most once a minute),
// fetching a conversation's messages and POST .../seen refresh it at once.
const (
	// ChatAwayAfter is how long a visitor's widget has been quiet before they count as gone.
	ChatAwayAfter = 2 * time.Minute
	// ChatFollowUpDelay is how long after the reply the email goes, unless the visitor is back.
	ChatFollowUpDelay = time.Minute
)

// ChatFollowUpDue reports whether the chat ticket's visitor should get a reply by email:
// the widget's email_follow_up is on (the ticket's widget, else the workspace's primary
// widget), the contact has an email and is not blocked, and none of their widget sessions
// was seen at or after since.
func ChatFollowUpDue(ctx context.Context, q db.Querier, wid, ticketID uuid.UUID, since time.Time) (bool, error) {
	var due bool
	err := q.QueryRow(ctx, `SELECT COALESCE((g.settings ->> 'email_follow_up')::boolean, (w.settings -> 'widget' ->> 'email_follow_up')::boolean, true)
		AND COALESCE(c.email, '') <> '' AND NOT c.blocked
		AND NOT EXISTS (SELECT 1 FROM widget_sessions s WHERE s.contact_id = c.id AND s.workspace_id = t.workspace_id AND s.last_seen_at >= $3)
		FROM tickets t
		JOIN workspaces w ON w.id = t.workspace_id
		JOIN contacts c ON c.id = t.contact_id AND c.workspace_id = t.workspace_id
		LEFT JOIN widgets g ON g.id = t.widget_id AND g.workspace_id = t.workspace_id
		WHERE t.workspace_id = $1 AND t.id = $2 AND t.channel = 'chat' AND t.deleted_at IS NULL`, wid, ticketID, since).Scan(&due)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return due, err
}

// queueChatFollowUp schedules the email of an agent's chat reply when its visitor is away.
func queueChatFollowUp(ctx context.Context, tx pgx.Tx, jc Jobs, wid, ticketID, messageID uuid.UUID, at time.Time) error {
	due, err := ChatFollowUpDue(ctx, tx, wid, ticketID, at.Add(-ChatAwayAfter))
	if err != nil || !due {
		return err
	}
	_, err = jc.InsertTx(ctx, tx, jobs.ChatFollowUpArgs{WorkspaceID: wid, TicketID: ticketID, MessageID: messageID},
		&river.InsertOpts{Queue: jobs.QueueEmail, MaxAttempts: 3, ScheduledAt: at.Add(ChatFollowUpDelay)})
	return err
}
