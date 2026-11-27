package tickets

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/events"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/ids"
)

// Related tickets (contracts/api.md "Related tickets"): two tickets that belong together
// without being merged. Each keeps its own status, assignee, team, SLA and timeline; the link
// is one row read from either side.

// RelatedTicket is the other end of a relation, as much of it as the caller may know.
type RelatedTicket struct {
	ID      uuid.UUID `json:"id"`
	Number  int64     `json:"number"`
	Subject string    `json:"subject"`
	Status  string    `json:"status"`
	Channel string    `json:"channel" enum:"email,chat,api,portal,phone"`
}

// Relation is a link between the ticket a list is of and another ticket.
type Relation struct {
	ID        uuid.UUID      `json:"id"`
	TicketID  uuid.UUID      `json:"ticket_id" doc:"The ticket this relation is read from"`
	Related   *RelatedTicket `json:"related" doc:"The other ticket; null when it is in a private team the caller cannot see"`
	ActorType string         `json:"actor_type" enum:"user,ai,api"`
	CreatedBy *uuid.UUID     `json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
}

type relationTicket struct {
	ID       uuid.UUID
	Number   int64
	Subject  string
	Status   string
	Channel  string
	Team     *uuid.UUID
	Assignee *uuid.UUID
}

const relationSelect = `SELECT r.id, r.created_by, r.actor_type, r.created_at,
	o.id, o.number, o.subject, o.status, o.channel, o.team_id, o.assignee_id
	FROM ticket_relations r
	JOIN tickets o ON o.workspace_id = r.workspace_id
		AND o.id = CASE WHEN r.ticket_id = $2 THEN r.related_ticket_id ELSE r.ticket_id END
	WHERE r.workspace_id = $1 AND (r.ticket_id = $2 OR r.related_ticket_id = $2) AND o.deleted_at IS NULL`

func scanRelations(ctx context.Context, q db.Querier, p *auth.Principal, tid uuid.UUID, rows pgx.Rows) ([]Relation, error) {
	defer rows.Close()
	out := []Relation{}
	for rows.Next() {
		r := Relation{TicketID: tid}
		var o relationTicket
		if err := rows.Scan(&r.ID, &r.CreatedBy, &r.ActorType, &r.CreatedAt, &o.ID, &o.Number, &o.Subject, &o.Status, &o.Channel, &o.Team, &o.Assignee); err != nil {
			return nil, err
		}
		ok, err := domain.CanSee(ctx, q, p, o.Team, o.Assignee)
		if err != nil {
			return nil, err
		}
		if ok {
			r.Related = &RelatedTicket{ID: o.ID, Number: o.Number, Subject: o.Subject, Status: o.Status, Channel: o.Channel}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func listRelations(ctx context.Context, q db.Querier, p *auth.Principal, tid uuid.UUID) ([]Relation, error) {
	rows, err := q.Query(ctx, relationSelect+` ORDER BY r.created_at, r.id`, p.WorkspaceID, tid)
	if err != nil {
		return nil, err
	}
	return scanRelations(ctx, q, p, tid, rows)
}

func loadRelation(ctx context.Context, q db.Querier, p *auth.Principal, tid, id uuid.UUID) (Relation, error) {
	rows, err := q.Query(ctx, relationSelect+` AND r.id = $3`, p.WorkspaceID, tid, id)
	if err != nil {
		return Relation{}, err
	}
	list, err := scanRelations(ctx, q, p, tid, rows)
	if err != nil {
		return Relation{}, err
	}
	if len(list) == 0 {
		return Relation{}, httpx.NotFound("Link")
	}
	return list[0], nil
}

// relationEnds loads what a relation's timeline entries and checks need to know of a ticket,
// deleted or merged ones included.
func relationEnds(ctx context.Context, q db.Querier, wid, id uuid.UUID) (relationTicket, *uuid.UUID, error) {
	t := relationTicket{ID: id}
	var merged *uuid.UUID
	err := q.QueryRow(ctx, `SELECT number, subject, status, team_id, assignee_id, merged_into_id FROM tickets WHERE workspace_id = $1 AND id = $2`,
		wid, id).Scan(&t.Number, &t.Subject, &t.Status, &t.Team, &t.Assignee, &merged)
	if db.IsNotFound(err) {
		return t, nil, httpx.NotFound("Ticket")
	}
	return t, merged, err
}

// namedOn is the data of a timeline entry on source about target: the target's number and
// subject when everyone who sees source may see target too, nothing for one in another,
// private team (as a merge's into_number is left out).
func namedOn(ctx context.Context, q db.Querier, wid uuid.UUID, relationID uuid.UUID, source, target relationTicket) (map[string]any, error) {
	data := map[string]any{"relation_id": relationID.String(), "ticket_id": target.ID.String(), "number": target.Number, "subject": target.Subject}
	if target.Team != nil && !sameID(target.Team, source.Team) {
		private, err := hd.TeamPrivate(ctx, q, wid, target.Team)
		if err != nil {
			return nil, err
		}
		if private {
			data = map[string]any{"relation_id": relationID.String(), "ticket_id": nil, "number": nil, "subject": nil}
		}
	}
	return data, nil
}

// announceRelation tells both tickets about a change to a relation: the event reaches whoever
// sees that ticket, and carries ids only, so the list is read again with each reader's own
// visibility.
func announceRelation(ctx context.Context, tx pgx.Tx, wid uuid.UUID, typ string, relationID uuid.UUID, tickets ...uuid.UUID) error {
	for _, tid := range tickets {
		tid := tid
		if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: wid, Type: typ, TicketID: &tid,
			Data: map[string]any{"ticket_id": tid, "relation_id": relationID}}); err != nil {
			return err
		}
	}
	return nil
}

// logRelation records the change on both tickets' timelines, each naming the other.
func logRelation(ctx context.Context, tx pgx.Tx, wid, relationID uuid.UUID, typ string, actor Actor, a, b relationTicket) error {
	for _, pair := range [][2]relationTicket{{a, b}, {b, a}} {
		data, err := namedOn(ctx, tx, wid, relationID, pair[0], pair[1])
		if err != nil {
			return err
		}
		if _, err := domain.LogActivity(ctx, tx, wid, pair[0].ID, actor.Type, actor.UserID, typ, data); err != nil {
			return err
		}
	}
	return nil
}

// moveRelations gives dst the relations of src, which a merge closes: the one between the two
// and any dst has already go away.
func moveRelations(ctx context.Context, tx pgx.Tx, wid, src, dst uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT id, ticket_id, related_ticket_id FROM ticket_relations
		WHERE workspace_id = $1 AND (ticket_id = $2 OR related_ticket_id = $2)`, wid, src)
	if err != nil {
		return err
	}
	type row struct{ ID, A, B uuid.UUID }
	moved, err := pgx.CollectRows(rows, pgx.RowToStructByPos[row])
	if err != nil {
		return err
	}
	touched := []uuid.UUID{src, dst}
	for _, r := range moved {
		other := r.A
		if other == src {
			other = r.B
		}
		var taken bool
		if other != dst {
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ticket_relations WHERE workspace_id = $1
				AND LEAST(ticket_id, related_ticket_id) = LEAST($2::uuid, $3::uuid) AND GREATEST(ticket_id, related_ticket_id) = GREATEST($2::uuid, $3::uuid))`,
				wid, dst, other).Scan(&taken); err != nil {
				return err
			}
		}
		if other == dst || taken {
			if _, err := tx.Exec(ctx, `DELETE FROM ticket_relations WHERE workspace_id = $1 AND id = $2`, wid, r.ID); err != nil {
				return err
			}
		} else if _, err := tx.Exec(ctx, `UPDATE ticket_relations SET
			ticket_id = CASE WHEN ticket_id = $3 THEN $4::uuid ELSE ticket_id END,
			related_ticket_id = CASE WHEN related_ticket_id = $3 THEN $4::uuid ELSE related_ticket_id END
			WHERE workspace_id = $1 AND id = $2`, wid, r.ID, src, dst); err != nil {
			return err
		}
		touched = append(touched, other)
	}
	if len(moved) == 0 {
		return nil
	}
	for _, tid := range touched {
		tid := tid
		if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: wid, Type: "ticket_relation.updated", TicketID: &tid,
			Data: map[string]any{"ticket_id": tid}}); err != nil {
			return err
		}
	}
	return nil
}

type relationCreateBody struct {
	TicketID string `json:"ticket_id" maxLength:"100" doc:"The other ticket: its id, number (#123 or 123) or alias"`
}

func registerRelations(d *deps.Deps, api huma.API) {
	tags := []string{"Tickets"}

	huma.Register(api, huma.Operation{OperationID: "list-related-tickets", Method: http.MethodGet, Path: "/v1/tickets/{id}/related",
		Summary: "List the tickets a ticket is linked to", Tags: tags,
		Description: "Oldest first. A ticket in a private team the caller cannot see is listed with related null."},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct {
			Body struct {
				Data []Relation `json:"data"`
			}
		}, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleViewer, auth.ScopeTicketsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			tid, err := domain.ResolveVisibleTicket(ctx, d.DB.Pool, p, in.ID)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct {
				Body struct {
					Data []Relation `json:"data"`
				}
			}{}
			if out.Body.Data, err = listRelations(ctx, d.DB.Pool, p, tid); err != nil {
				return nil, httpx.Wrap(err)
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "create-related-ticket", Method: http.MethodPost, Path: "/v1/tickets/{id}/related",
		Summary: "Link a ticket to another ticket", Tags: tags, DefaultStatus: http.StatusCreated,
		Description: "Both tickets keep their own status, assignee, team and timeline and show each other. Linking the same pair again, from either side, answers the link that is there."},
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body relationCreateBody
		}) (*struct{ Body Relation }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAgent, auth.ScopeTicketsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			var out Relation
			err = d.DB.Tx(ctx, func(tx pgx.Tx) error {
				a, err := domain.ResolveVisibleTicket(ctx, tx, p, in.ID)
				if err != nil {
					return err
				}
				b, err := domain.ResolveVisibleTicket(ctx, tx, p, in.Body.TicketID)
				if err != nil {
					return err
				}
				if a == b {
					return httpx.Validation("a ticket cannot be linked to itself")
				}
				first, second := a, b
				if second.String() < first.String() {
					first, second = second, first
				}
				for _, id := range []uuid.UUID{first, second} {
					if _, err := lockTicket(ctx, tx, p.WorkspaceID, id); err != nil {
						return err
					}
				}
				ta, mergedA, err := relationEnds(ctx, tx, p.WorkspaceID, a)
				if err != nil {
					return err
				}
				tb, mergedB, err := relationEnds(ctx, tx, p.WorkspaceID, b)
				if err != nil {
					return err
				}
				if mergedA != nil || mergedB != nil {
					return httpx.Validation("a merged ticket cannot be linked; link the ticket it was merged into")
				}
				var existing uuid.UUID
				err = tx.QueryRow(ctx, `SELECT id FROM ticket_relations WHERE workspace_id = $1
					AND LEAST(ticket_id, related_ticket_id) = LEAST($2::uuid, $3::uuid) AND GREATEST(ticket_id, related_ticket_id) = GREATEST($2::uuid, $3::uuid)`,
					p.WorkspaceID, a, b).Scan(&existing)
				if err == nil {
					out, err = loadRelation(ctx, tx, p, a, existing)
					return err
				} else if !db.IsNotFound(err) {
					return err
				}
				for _, id := range []uuid.UUID{a, b} {
					n, err := domain.TicketLinkCount(ctx, tx, p.WorkspaceID, id)
					if err != nil {
						return err
					}
					if n >= domain.MaxTicketLinks {
						return httpx.Conflict("A ticket can have at most 50 links.")
					}
				}
				actor := actorOf(p)
				id := ids.New()
				if _, err := tx.Exec(ctx, `INSERT INTO ticket_relations (id, workspace_id, ticket_id, related_ticket_id, created_by, actor_type)
					VALUES ($1,$2,$3,$4,$5,$6)`, id, p.WorkspaceID, a, b, actor.UserID, relationActor(actor)); err != nil {
					return err
				}
				if err := announceRelation(ctx, tx, p.WorkspaceID, "ticket_relation.created", id, a, b); err != nil {
					return err
				}
				if err := logRelation(ctx, tx, p.WorkspaceID, id, "linked_ticket", actor, ta, tb); err != nil {
					return err
				}
				out, err = loadRelation(ctx, tx, p, a, id)
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body Relation }{out}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "delete-related-ticket", Method: http.MethodDelete, Path: "/v1/tickets/{id}/related/{relationId}",
		Summary: "Remove a link between two tickets", Tags: tags, DefaultStatus: http.StatusNoContent,
		Description: "Removes it on both tickets. The tickets themselves are left as they are."},
		func(ctx context.Context, in *struct {
			ID         string `path:"id"`
			RelationID string `path:"relationId"`
		}) (*struct{}, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAgent, auth.ScopeTicketsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			rid, ok := ids.Parse(in.RelationID)
			if !ok {
				return nil, httpx.NotFound("Link")
			}
			err = d.DB.Tx(ctx, func(tx pgx.Tx) error {
				a, err := domain.ResolveVisibleTicket(ctx, tx, p, in.ID)
				if err != nil {
					return err
				}
				var x, y uuid.UUID
				err = tx.QueryRow(ctx, `DELETE FROM ticket_relations WHERE workspace_id = $1 AND id = $2 AND (ticket_id = $3 OR related_ticket_id = $3)
					RETURNING ticket_id, related_ticket_id`, p.WorkspaceID, rid, a).Scan(&x, &y)
				if db.IsNotFound(err) {
					return httpx.NotFound("Link")
				} else if err != nil {
					return err
				}
				b := x
				if b == a {
					b = y
				}
				ta, _, err := relationEnds(ctx, tx, p.WorkspaceID, a)
				if err != nil {
					return err
				}
				tb, _, err := relationEnds(ctx, tx, p.WorkspaceID, b)
				if err != nil {
					return err
				}
				if err := announceRelation(ctx, tx, p.WorkspaceID, "ticket_relation.deleted", rid, a, b); err != nil {
					return err
				}
				return logRelation(ctx, tx, p.WorkspaceID, rid, "unlinked_ticket", actorOf(p), ta, tb)
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{}{}, nil
		})
}

// relationActor is the actor_type a relation is stored with.
func relationActor(a Actor) string {
	if a.Type == "user" || a.Type == "ai" {
		return a.Type
	}
	return "api"
}
