package whatsapp

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/whatsapp/provider"
)

// WhatsAppConnectionRef is the connection a ticket's replies go out through.
type WhatsAppConnectionRef struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Sender string    `json:"sender"`
}

// WhatsAppWindow is whether a free-form reply may be sent now (contracts/whatsapp.md "Window").
type WhatsAppWindow struct {
	Open     bool       `json:"open"`
	ClosesAt *time.Time `json:"closes_at"`
}

// WhatsAppTicketState is what the composer of a WhatsApp ticket needs.
type WhatsAppTicketState struct {
	Connection *WhatsAppConnectionRef `json:"connection"`
	Window     WhatsAppWindow         `json:"window"`
}

// WhatsAppTemplateChoice is an approved template and the values of its variables, sent instead of
// free-form words.
type WhatsAppTemplateChoice struct {
	ID        string            `json:"id" maxLength:"64"`
	Variables map[string]string `json:"variables,omitempty"`
}

// ReplyPlan is what a reply on a WhatsApp ticket goes out as.
type ReplyPlan struct {
	ConnectionID uuid.UUID
	Number       string
	TemplateID   string
	Variables    map[string]string
	// Text is a template's words with its variables filled; "" for a free-form reply.
	Text string
}

type target struct {
	conn *WhatsAppConnection
	num  string
	last time.Time
}

// target finds where a ticket's replies go: the connection and number of the customer's
// latest WhatsApp message on it, and when it came.
func (s *Service) target(ctx context.Context, q db.Querier, wid, tid uuid.UUID) (target, error) {
	var t target
	var cid *uuid.UUID
	err := q.QueryRow(ctx, `SELECT connection_id, number, created_at FROM whatsapp_messages
		WHERE workspace_id = $1 AND ticket_id = $2 AND direction = 'inbound' ORDER BY created_at DESC LIMIT 1`, wid, tid).Scan(&cid, &t.num, &t.last)
	if db.IsNotFound(err) {
		return t, httpx.E(422, "whatsapp_no_conversation", "This ticket has no WhatsApp message from the customer to answer.")
	}
	if err != nil {
		return t, err
	}
	if cid == nil {
		return t, httpx.E(422, "whatsapp_disconnected", "The WhatsApp number this conversation came in on is no longer connected.")
	}
	c, err := s.connection(ctx, q, wid, *cid)
	if err != nil {
		return t, err
	}
	t.conn = &c
	return t, nil
}

// templatesFor lists a connection's approved templates, kept for templatesTTL.
func (s *Service) templatesFor(ctx context.Context, c WhatsAppConnection) ([]provider.WhatsAppTemplate, error) {
	if got, ok := s.templates.Load(c.ID); ok {
		if ct := got.(cachedTemplates); time.Since(ct.at) < templatesTTL {
			return ct.list, nil
		}
	}
	a, ok := s.adapters[c.Provider]
	if !ok {
		return nil, httpx.Validation("Unknown WhatsApp provider.")
	}
	creds, err := s.credentials(c)
	if err != nil {
		return nil, err
	}
	list, err := a.Templates(ctx, s.http, creds)
	if err != nil {
		return nil, httpx.E(502, "whatsapp_provider_unavailable", "The WhatsApp provider could not list its templates. Try again in a moment.")
	}
	if list == nil {
		list = []provider.WhatsAppTemplate{}
	}
	s.templates.Store(c.ID, cachedTemplates{at: time.Now(), list: list})
	return list, nil
}

var placeholderRe = regexp.MustCompile(`\{\{\s*([0-9A-Za-z_]+)\s*\}\}`)

const maxVariable = 1024

// fill is a template's body with its variables put in.
func fill(body string, vars map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(body, func(m string) string {
		return vars[placeholderRe.FindStringSubmatch(m)[1]]
	})
}

// PlanReply decides how a reply on a WhatsApp ticket goes out: free-form while the
// customer-service window is open, a template when one is chosen, and nothing when the
// window is closed and none is.
func PlanReply(ctx context.Context, d *deps.Deps, tx pgx.Tx, wid, tid uuid.UUID, tpl *WhatsAppTemplateChoice) (ReplyPlan, error) {
	return For(d).planReply(ctx, tx, wid, tid, tpl)
}

// RecordOutbound links a reply to the conversation it goes out in.
func RecordOutbound(ctx context.Context, d *deps.Deps, tx pgx.Tx, wid, tid, mid uuid.UUID, plan ReplyPlan) error {
	return For(d).recordOutbound(ctx, tx, wid, tid, mid, plan)
}

func (s *Service) planReply(ctx context.Context, tx pgx.Tx, wid, tid uuid.UUID, tpl *WhatsAppTemplateChoice) (ReplyPlan, error) {
	t, err := s.target(ctx, tx, wid, tid)
	if err != nil {
		return ReplyPlan{}, err
	}
	plan := ReplyPlan{ConnectionID: t.conn.ID, Number: t.num}
	if tpl == nil {
		if time.Since(t.last) >= Window {
			return plan, httpx.E(422, "whatsapp_window_closed",
				"The customer last wrote more than 24 hours ago, so WhatsApp only allows an approved template message.")
		}
		return plan, nil
	}
	list, err := s.templatesFor(ctx, *t.conn)
	if err != nil {
		return plan, err
	}
	for _, x := range list {
		if x.ID != tpl.ID {
			continue
		}
		vars := map[string]string{}
		for _, name := range x.Variables {
			v := strings.TrimSpace(tpl.Variables[name])
			if v == "" || len([]rune(v)) > maxVariable {
				return plan, httpx.Validation("Fill in every variable of the template (up to 1024 characters each).")
			}
			vars[name] = v
		}
		plan.TemplateID, plan.Variables, plan.Text = x.ID, vars, fill(x.Body, vars)
		return plan, nil
	}
	return plan, httpx.Validation("This template is not an approved template of the connected number.")
}

func (s *Service) recordOutbound(ctx context.Context, tx pgx.Tx, wid, tid, mid uuid.UUID, plan ReplyPlan) error {
	vars := plan.Variables
	if vars == nil {
		vars = map[string]string{}
	}
	_, err := tx.Exec(ctx, `INSERT INTO whatsapp_messages (message_id, workspace_id, ticket_id, connection_id, direction, number, template_id, variables)
		VALUES ($1, $2, $3, $4, 'outbound', $5, $6, $7)`, mid, wid, tid, plan.ConnectionID, plan.Number, plan.TemplateID, vars)
	return err
}

func (s *Service) registerTicketRoutes(api huma.API) {
	tags := []string{"WhatsApp"}
	huma.Register(api, huma.Operation{OperationID: "get-ticket-whatsapp", Method: http.MethodGet, Path: "/v1/tickets/{id}/whatsapp", Tags: tags,
		Summary: "Get a WhatsApp ticket's window", Description: "The connection replies go out through and whether the customer-service window is open."},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct{ Body WhatsAppTicketState }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleViewer, auth.ScopeTicketsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			tid, err := domain.ResolveVisibleTicket(ctx, s.d.DB.Pool, p, in.ID)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct{ Body WhatsAppTicketState }{}
			t, err := s.target(ctx, s.d.DB.Pool, p.WorkspaceID, tid)
			if err != nil {
				var he *httpx.Error
				if errors.As(err, &he) && he.Status == 422 {
					return out, nil // no conversation, or its number disconnected: no window to show
				}
				return nil, httpx.Wrap(err)
			}
			closes := t.last.Add(Window)
			out.Body.Connection = &WhatsAppConnectionRef{ID: t.conn.ID, Name: t.conn.Name, Sender: t.conn.Sender}
			out.Body.Window = WhatsAppWindow{Open: time.Now().Before(closes), ClosesAt: &closes}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "list-ticket-whatsapp-templates", Method: http.MethodGet, Path: "/v1/tickets/{id}/whatsapp/templates", Tags: tags,
		Summary: "List the approved templates of a WhatsApp ticket's number"},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct {
			Body struct {
				Data []provider.WhatsAppTemplate `json:"data"`
			}
		}, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAgent, auth.ScopeTicketsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			tid, err := domain.ResolveVisibleTicket(ctx, s.d.DB.Pool, p, in.ID)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			t, err := s.target(ctx, s.d.DB.Pool, p.WorkspaceID, tid)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			list, err := s.templatesFor(ctx, *t.conn)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct {
				Body struct {
					Data []provider.WhatsAppTemplate `json:"data"`
				}
			}{}
			out.Body.Data = list
			return out, nil
		})
}
