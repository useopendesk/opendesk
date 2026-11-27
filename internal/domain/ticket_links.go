package domain

import (
	"context"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/db"
)

// MaxTicketLinks bounds what one ticket points at: its links to other tools and to other
// tickets together.
const MaxTicketLinks = 50

// TicketLinkCount is how many links a ticket has to other tools and to other tickets.
func TicketLinkCount(ctx context.Context, q db.Querier, wid, tid uuid.UUID) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM ticket_links WHERE workspace_id = $1 AND ticket_id = $2)
		+ (SELECT count(*) FROM ticket_relations WHERE workspace_id = $1 AND (ticket_id = $2 OR related_ticket_id = $2))`,
		wid, tid).Scan(&n)
	return n, err
}
