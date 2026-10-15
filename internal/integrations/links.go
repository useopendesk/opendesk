package integrations

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/ids"
)

// maxLinks bounds the links on one ticket.
const maxLinks = 50

// Link is something in another tool that a ticket points to: an issue, a task, a page.
type Link struct {
	ID            uuid.UUID  `json:"id"`
	TicketID      uuid.UUID  `json:"ticket_id"`
	IntegrationID *uuid.UUID `json:"integration_id"`
	Provider      string     `json:"provider" doc:"Catalog key, custom, or link"`
	URL           string     `json:"url"`
	Title         string     `json:"title"`
	ActorType     string     `json:"actor_type" enum:"user,ai,api"`
	CreatedBy     *uuid.UUID `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
}

const linkSelect = `SELECT id, ticket_id, integration_id, provider, url, title, actor_type, created_by, created_at FROM ticket_links`

func scanLink(r pgx.Row) (Link, error) {
	var l Link
	err := r.Scan(&l.ID, &l.TicketID, &l.IntegrationID, &l.Provider, &l.URL, &l.Title, &l.ActorType, &l.CreatedBy, &l.CreatedAt)
	return l, err
}

func listLinks(ctx context.Context, q db.Querier, wid, ticketID uuid.UUID) ([]Link, error) {
	rows, err := q.Query(ctx, linkSelect+` WHERE workspace_id = $1 AND ticket_id = $2 ORDER BY created_at`, wid, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Link{}
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// visibleTicket answers not found for a ticket p may not see (contracts/teams.md "Visibility"),
// the same as for a missing one.
func visibleTicket(ctx context.Context, q db.Querier, p *auth.Principal, id uuid.UUID) error {
	var team, assignee *uuid.UUID
	err := q.QueryRow(ctx, `SELECT team_id, assignee_id FROM tickets WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL`,
		p.WorkspaceID, id).Scan(&team, &assignee)
	if db.IsNotFound(err) {
		return httpx.NotFound("Ticket")
	}
	if err != nil {
		return err
	}
	ok, err := domain.CanSee(ctx, q, p, team, assignee)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.NotFound("Ticket")
	}
	return nil
}

func actorType(p *auth.Principal) string {
	switch {
	case p.Kind == auth.KindUser && p.Assistant:
		return "ai"
	case p.Kind == auth.KindUser:
		return "user"
	}
	return "api"
}

// linkURL accepts an absolute https link (http on a development instance). The link is
// only shown, never fetched, so it is not checked against private addresses.
func (s *Service) linkURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && !(u.Scheme == "http" && s.guard.Dev)) {
		return "", httpx.Validation("Use a full https:// link.")
	}
	return u.String(), nil
}

// providerFor names the integration a link belongs to by its host.
func providerFor(rawURL string) string {
	h := strings.ToLower(hostOf(rawURL))
	for _, p := range Catalog {
		for _, host := range p.Hosts {
			if h == host || strings.HasSuffix(h, "."+host) {
				return p.Key
			}
		}
	}
	return "link"
}

func (s *Service) registerLinks(api huma.API) {
	tags := []string{"Tickets"}

	huma.Register(api, huma.Operation{OperationID: "list-ticket-links", Method: http.MethodGet, Path: "/v1/tickets/{id}/links",
		Summary: "List a ticket's links to other tools", Tags: tags},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct {
			Body struct {
				Data []Link `json:"data"`
			}
		}, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleViewer, auth.ScopeTicketsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			tid, ok := ids.Parse(in.ID)
			if !ok {
				return nil, httpx.NotFound("Ticket")
			}
			if err := visibleTicket(ctx, s.d.DB.Pool, p, tid); err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct {
				Body struct {
					Data []Link `json:"data"`
				}
			}{}
			if out.Body.Data, err = listLinks(ctx, s.d.DB.Pool, p.WorkspaceID, tid); err != nil {
				return nil, httpx.Wrap(err)
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "create-ticket-link", Method: http.MethodPost, Path: "/v1/tickets/{id}/links",
		Summary: "Link a ticket to something in another tool", Tags: tags, DefaultStatus: http.StatusCreated,
		Description: "Linking the same URL again answers the existing link."},
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body struct {
				URL           string `json:"url" maxLength:"2000"`
				Title         string `json:"title,omitempty" maxLength:"300"`
				IntegrationID string `json:"integration_id,omitempty" doc:"The integration the item was made with"`
			}
		}) (*struct{ Body Link }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAgent, auth.ScopeTicketsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			tid, ok := ids.Parse(in.ID)
			if !ok {
				return nil, httpx.NotFound("Ticket")
			}
			link, err := s.linkURL(in.Body.URL)
			if err != nil {
				return nil, err
			}
			title := clip(strings.Join(strings.Fields(in.Body.Title), " "), 300)
			prov := providerFor(link)
			var iid *uuid.UUID
			if in.Body.IntegrationID != "" {
				id, ok := ids.Parse(in.Body.IntegrationID)
				if !ok {
					return nil, httpx.NotFound("Integration")
				}
				x, err := loadRow(ctx, s.d.DB.Pool, p.WorkspaceID, id)
				if err != nil {
					return nil, wrapRemote(err)
				}
				iid, prov = &x.ID, x.Provider
			}
			var l Link
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if err := visibleTicket(ctx, tx, p, tid); err != nil {
					return err
				}
				existing, err := scanLink(tx.QueryRow(ctx, linkSelect+` WHERE workspace_id = $1 AND ticket_id = $2 AND url = $3`, p.WorkspaceID, tid, link))
				if err == nil {
					l = existing
					return nil
				} else if !db.IsNotFound(err) {
					return err
				}
				var n int
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM ticket_links WHERE workspace_id = $1 AND ticket_id = $2`, p.WorkspaceID, tid).Scan(&n); err != nil {
					return err
				}
				if n >= maxLinks {
					return httpx.Conflict("A ticket can have at most 50 links.")
				}
				if title == "" {
					title = hostOf(link)
				}
				if l, err = scanLink(tx.QueryRow(ctx, `INSERT INTO ticket_links (id, workspace_id, ticket_id, integration_id, provider, url, title,
					created_by, actor_type) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
					RETURNING id, ticket_id, integration_id, provider, url, title, actor_type, created_by, created_at`,
					ids.New(), p.WorkspaceID, tid, iid, prov, link, title, p.ActorUserID(), actorType(p))); err != nil {
					return err
				}
				if err := s.publishTicket(ctx, tx, "ticket_link.created", p.WorkspaceID, tid, map[string]any{"ticket_id": tid, "link": l}); err != nil {
					return err
				}
				_, err = domain.LogActivity(ctx, tx, p.WorkspaceID, tid, actorType(p), p.ActorUserID(), "linked",
					map[string]any{"link_id": l.ID, "url": l.URL, "title": l.Title, "provider": l.Provider})
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body Link }{l}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "delete-ticket-link", Method: http.MethodDelete, Path: "/v1/tickets/{id}/links/{linkId}",
		Summary: "Remove a link from a ticket", Tags: tags, DefaultStatus: http.StatusNoContent,
		Description: "The item in the other tool is left as it is."},
		func(ctx context.Context, in *struct {
			ID     string `path:"id"`
			LinkID string `path:"linkId"`
		}) (*struct{}, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAgent, auth.ScopeTicketsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			tid, ok1 := ids.Parse(in.ID)
			lid, ok2 := ids.Parse(in.LinkID)
			if !ok1 || !ok2 {
				return nil, httpx.NotFound("Link")
			}
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if err := visibleTicket(ctx, tx, p, tid); err != nil {
					return err
				}
				l, err := scanLink(tx.QueryRow(ctx, `DELETE FROM ticket_links WHERE workspace_id = $1 AND ticket_id = $2 AND id = $3
					RETURNING id, ticket_id, integration_id, provider, url, title, actor_type, created_by, created_at`, p.WorkspaceID, tid, lid))
				if db.IsNotFound(err) {
					return httpx.NotFound("Link")
				} else if err != nil {
					return err
				}
				if err := s.publishTicket(ctx, tx, "ticket_link.deleted", p.WorkspaceID, tid, map[string]any{"ticket_id": tid, "link_id": lid}); err != nil {
					return err
				}
				_, err = domain.LogActivity(ctx, tx, p.WorkspaceID, tid, actorType(p), p.ActorUserID(), "unlinked",
					map[string]any{"url": l.URL, "title": l.Title, "provider": l.Provider})
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{}{}, nil
		})
}
