package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/helpdesk/tickets"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// Bounds on what one template send may carry: Meta's own limits, so a refused request says
// what to fix instead of being cut short by a provider error.
const (
	// maxTemplateParams bounds how many variables one send may fill.
	maxTemplateParams = 50
	// maxTemplateParamRunes is Meta's cap on one variable's value.
	maxTemplateParamRunes = 1024
	// maxTemplateHeaderRunes is Meta's cap on a text header's value.
	maxTemplateHeaderRunes = 60
	// maxTemplateURLRunes bounds one URL button's value.
	maxTemplateURLRunes = 2000
)

// templateList is the synced templates of a workspace, or of one connection.
type templateList struct {
	Data []TemplateView `json:"data"`
}

// templateSyncBody names the connection to sync; empty or absent syncs every connection of
// the workspace.
type templateSyncBody struct {
	ConnectionID string `json:"connection_id,omitempty" doc:"Sync only this connection; empty syncs every connection of the workspace"`
}

// templateSyncResult is the sync's outcome: the templates afterwards and how many were stored.
type templateSyncResult struct {
	Data   []TemplateView `json:"data"`
	Synced int            `json:"synced"`
}

// templateSendBody picks a template and fills in what it needs.
type templateSendBody struct {
	TemplateID string            `json:"template_id" doc:"The synced template to send"`
	Params     map[string]string `json:"params,omitempty" doc:"Values for the template's body variables, by placeholder name or number"`
	// HeaderText is the value filled into the header's variable, not the template's approved
	// header text: the approval already holds that text, and Meta takes only the value.
	HeaderText string            `json:"header_text,omitempty" doc:"The value (not the approved header text) of the template's text header variable, when it has one"`
	ButtonURLs map[string]string `json:"button_urls,omitempty" doc:"Values for the template's URL buttons, by button number"`
}

// registerTemplateAPI mounts the templates' API: the synced list, an admin sync, and sending
// a template on a ticket, which is the only way to reach a customer outside Meta's window.
func (s *Service) registerTemplateAPI(api huma.API, _ chi.Router) {
	tags := []string{"WhatsApp"}

	huma.Register(api, huma.Operation{OperationID: "list-whatsapp-templates", Method: http.MethodGet,
		Path: "/v1/whatsapp/templates", Tags: tags, Summary: "List WhatsApp templates"},
		func(ctx context.Context, in *struct {
			ConnectionID string `query:"connection_id" doc:"Only this connection's templates"`
		}) (*struct{ Body templateList }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleViewer, auth.ScopeSettingsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			var connectionID *uuid.UUID
			if raw := strings.TrimSpace(in.ConnectionID); raw != "" {
				id, err := uuid.Parse(raw)
				if err != nil {
					return nil, httpx.Validation("connection_id is not a valid id.")
				}
				connectionID = &id
			}
			rows, err := listTemplates(ctx, s.d.DB.Pool, p.WorkspaceID, connectionID)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct{ Body templateList }{templateList{Data: make([]TemplateView, 0, len(rows))}}
			for _, row := range rows {
				out.Body.Data = append(out.Body.Data, templateView(row))
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "sync-whatsapp-templates", Method: http.MethodPost,
		Path: "/v1/whatsapp/templates/sync", Tags: tags, Summary: "Sync WhatsApp templates",
		Description: "Reads the message templates of a connection's WhatsApp Business account, or of every connection of the workspace, from Meta and stores them."},
		func(ctx context.Context, in *struct{ Body *templateSyncBody }) (*struct{ Body templateSyncResult }, error) {
			p, err := requireAdminUser(ctx)
			if err != nil {
				return nil, err
			}
			connectionID := ""
			if in.Body != nil {
				connectionID = strings.TrimSpace(in.Body.ConnectionID)
			}
			var targets []connection
			if connectionID != "" {
				c, err := s.loadWorkspaceConnection(ctx, p.WorkspaceID, connectionID)
				if err != nil {
					return nil, err
				}
				targets = append(targets, c)
			} else if targets, err = listConnections(ctx, s.d.DB.Pool, p.WorkspaceID); err != nil {
				return nil, httpx.Wrap(err)
			}
			// One connection whose token Meta rejects must not keep the others' templates
			// stale; the sync answers with an error only when none of them succeeded.
			synced, succeeded := 0, 0
			var firstErr error
			for _, c := range targets {
				n, err := s.syncConnectionTemplates(ctx, c)
				if err != nil {
					s.d.Log.Warn("whatsapp template sync failed", "connection_id", c.ID, "err", err)
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				synced += n
				succeeded++
			}
			if succeeded == 0 && firstErr != nil {
				// The sync answers like the connection's own test and subscribe do: Meta's
				// refusal in words the admin can act on, a retry for a transport failure.
				var me *MetaAPIError
				if errors.As(firstErr, &me) {
					if msg := metaRefusal(firstErr); msg != "" {
						s.d.Log.Info("whatsapp template sync refused", "detail", msg)
						return nil, httpx.E(http.StatusUnprocessableEntity, "whatsapp_templates_refused",
							"Meta refused the template sync: "+msg)
					}
					return nil, httpx.E(http.StatusBadGateway, "whatsapp_templates_failed",
						"Couldn't reach Meta to sync the templates. Try again in a moment.")
				}
				return nil, httpx.Wrap(firstErr)
			}
			var (
				rows []whatsappTemplateRow
				only *uuid.UUID
			)
			if len(targets) == 1 && connectionID != "" {
				only = &targets[0].ID
			}
			if rows, err = listTemplates(ctx, s.d.DB.Pool, p.WorkspaceID, only); err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct{ Body templateSyncResult }{templateSyncResult{Data: make([]TemplateView, 0, len(rows)), Synced: synced}}
			for _, row := range rows {
				out.Body.Data = append(out.Body.Data, templateView(row))
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "send-whatsapp-template", Method: http.MethodPost,
		Path: "/v1/tickets/{id}/whatsapp-template", Tags: tags, Summary: "Send a WhatsApp template",
		Description:   "Sends an approved template on a WhatsApp ticket, inside or outside Meta's 24-hour window, with the values an agent filled in. It becomes a reply in the ticket's timeline.",
		DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body templateSendBody
		}) (*struct{ Body domain.Message }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAgent, auth.ScopeTicketsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			ticketID, err := hd.ParseID(in.ID, "Ticket")
			if err != nil {
				return nil, err
			}
			t, err := domain.LoadVisibleTicket(ctx, s.d.DB.Pool, p, ticketID)
			if db.IsNotFound(err) {
				return nil, httpx.NotFound("Ticket")
			}
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if t.Channel != domain.ChannelWhatsApp {
				return nil, httpx.E(http.StatusUnprocessableEntity, "not_whatsapp_ticket", "This ticket is not a WhatsApp conversation.")
			}
			row, err := s.templateForTicket(ctx, p.WorkspaceID, t, in.Body.TemplateID)
			if err != nil {
				return nil, err
			}
			// The number the template goes to is the wa_id the contact wrote from.
			if t.Contact == nil {
				return nil, httpx.E(http.StatusUnprocessableEntity, "whatsapp_no_contact",
					"This ticket has no contact to send the WhatsApp template to.")
			}
			if _, err := waIDForContact(ctx, s.d.DB.Pool, row.ConnectionID, t.Contact.ID); err != nil {
				if db.IsNotFound(err) {
					return nil, httpx.E(http.StatusUnprocessableEntity, "whatsapp_no_number",
						"This contact has no WhatsApp number on this connection.")
				}
				return nil, httpx.Wrap(err)
			}
			params, err := cleanTemplateParams(in.Body.Params, row)
			if err != nil {
				return nil, err
			}
			headerText, err := templateHeaderValue(in.Body.HeaderText, row)
			if err != nil {
				return nil, err
			}
			buttonURLs, err := cleanButtonURLs(in.Body.ButtonURLs, row.Buttons)
			if err != nil {
				return nil, err
			}
			// The message shows what the customer gets; the stored template keeps Meta's own
			// text with its placeholders, which is what the send components are built from.
			bodyText := renderTemplateBody(row.BodyContent, params)
			if strings.TrimSpace(bodyText) == "" {
				return nil, httpx.Validation("This template has no body text to send.")
			}
			tmpl := &whatsappTemplate{ID: row.ID.String(), Name: row.Name, Language: row.Language,
				Body: row.BodyContent, Params: params, HeaderType: row.HeaderType,
				HeaderText: headerText, ButtonURLs: buttonURLs}
			var msg domain.Message
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				var err error
				msg, err = tickets.AddWhatsAppTemplateReply(ctx, tx, s.d.Jobs, p, t.ID, bodyText,
					map[string]any{"whatsapp_template": tmpl})
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body domain.Message }{msg}, nil
		})
}

// templateForTicket reads the template an agent picked and checks it may go out on this
// ticket: the same workspace, approved at Meta, and sent through the connection the ticket
// was opened on.
func (s *Service) templateForTicket(ctx context.Context, wid uuid.UUID, t domain.Ticket, raw string) (whatsappTemplateRow, error) {
	id, err := hd.ParseRef(raw, "template_id")
	if err != nil {
		return whatsappTemplateRow{}, err
	}
	row, err := loadTemplate(ctx, s.d.DB.Pool, wid, id)
	if db.IsNotFound(err) {
		return row, httpx.NotFound("WhatsApp template")
	}
	if err != nil {
		return row, httpx.Wrap(err)
	}
	connID, err := s.templateTicketConnection(ctx, wid, t)
	if err != nil {
		return row, err
	}
	if row.ConnectionID != connID {
		return row, httpx.Validation("That template belongs to another WhatsApp connection.")
	}
	if !strings.EqualFold(strings.TrimSpace(row.Status), templateApproved) {
		return row, httpx.E(http.StatusUnprocessableEntity, "whatsapp_template_not_approved", templateApprovalDetail(row.Status))
	}
	return row, nil
}

// templateTicketConnection returns the connection a ticket's template goes out from: the one
// the ticket was opened on, else the workspace's only connection.
func (s *Service) templateTicketConnection(ctx context.Context, wid uuid.UUID, t domain.Ticket) (uuid.UUID, error) {
	if raw, ok := t.Custom["whatsapp_connection_id"].(string); ok {
		if id, err := uuid.Parse(raw); err == nil && id != uuid.Nil {
			if _, err := loadConnection(ctx, s.d.DB.Pool, wid, id); err == nil {
				return id, nil
			} else if !db.IsNotFound(err) {
				return uuid.Nil, httpx.Wrap(err)
			}
			// The connection is gone: fall back to the workspace's only one below.
		}
	}
	all, err := listConnections(ctx, s.d.DB.Pool, wid)
	if err != nil {
		return uuid.Nil, httpx.Wrap(err)
	}
	switch len(all) {
	case 0:
		return uuid.Nil, httpx.E(http.StatusConflict, "whatsapp_not_configured", "This workspace has no WhatsApp connection.")
	case 1:
		return all[0].ID, nil
	default:
		return uuid.Nil, httpx.Validation("This ticket does not name the WhatsApp connection its template should go out from.")
	}
}

// templateApprovalDetail says why a template cannot be sent, from the status Meta reported.
func templateApprovalDetail(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "":
		return "This template has not been synced from Meta yet."
	case "PENDING":
		return "This template is still waiting for Meta's review."
	case "REJECTED":
		return "Meta rejected this template. Edit it in Meta's template manager and sync again."
	default:
		return "Meta reports this template as " + strings.ToLower(strings.TrimSpace(status)) + ". Only an approved template can be sent."
	}
}

// cleanTemplateParams checks the body variables an agent filled in: their sizes stay inside
// Meta's caps, and every variable the body names has a value.
func cleanTemplateParams(given map[string]string, row whatsappTemplateRow) (map[string]string, error) {
	if len(given) > maxTemplateParams {
		return nil, httpx.Validation("Too many template variables.")
	}
	params := make(map[string]string, len(given))
	for key, value := range given {
		if utf8.RuneCountInString(value) > maxTemplateParamRunes {
			return nil, httpx.Validation("One template variable is longer than WhatsApp's 1024-character limit.")
		}
		params[key] = value
	}
	var missing []string
	for _, key := range orderedPlaceholders(row.BodyContent) {
		if strings.TrimSpace(params[key]) == "" {
			missing = append(missing, "{{"+key+"}}")
		}
	}
	if len(missing) > 0 {
		return nil, httpx.E(http.StatusUnprocessableEntity, "whatsapp_template_params_missing",
			"Fill in the template's variables: "+strings.Join(missing, ", ")+".")
	}
	return params, nil
}

// templateHeaderValue checks the value of a text header's one variable, and refuses a header
// this channel cannot fill (an image, video or document header). given is the value the agent
// filled in, never the template's approved header text.
func templateHeaderValue(given string, row whatsappTemplateRow) (string, error) {
	if kind := strings.TrimSpace(row.HeaderType); kind != "" && !strings.EqualFold(kind, "TEXT") {
		return "", httpx.E(http.StatusUnprocessableEntity, "whatsapp_template_header_unsupported",
			"Templates with a "+strings.ToLower(kind)+" header can't be sent from opendesk yet.")
	}
	keys := orderedPlaceholders(row.HeaderContent)
	if len(keys) == 0 {
		// A static header uses Meta's approved text; a value makes no difference.
		return "", nil
	}
	if strings.TrimSpace(given) == "" {
		return "", httpx.E(http.StatusUnprocessableEntity, "whatsapp_template_params_missing",
			"Fill in the template's header value ({{"+keys[0]+"}}).")
	}
	if utf8.RuneCountInString(given) > maxTemplateHeaderRunes {
		return "", httpx.Validation("The template's header value is longer than WhatsApp's 60-character limit.")
	}
	return given, nil
}

// cleanButtonURLs checks the values of a template's URL buttons. Meta keeps a button's index
// as its place in the template's buttons array, and only a URL with a variable takes one.
func cleanButtonURLs(given map[string]string, buttons []byte) (map[string]string, error) {
	needed := templateURLButtonParams(buttons)
	if len(given) == 0 {
		if len(needed) > 0 {
			return nil, httpx.E(http.StatusUnprocessableEntity, "whatsapp_template_params_missing",
				"Fill in the template's URL buttons: "+strings.Join(needed, ", ")+".")
		}
		return nil, nil
	}
	neededSet := make(map[string]bool, len(needed))
	for _, index := range needed {
		neededSet[index] = true
	}
	out := make(map[string]string, len(given))
	for key, value := range given {
		index := strings.TrimSpace(key)
		if _, err := strconv.Atoi(index); err != nil {
			return nil, httpx.Validation("button_urls is keyed by the number of the button it fills.")
		}
		if !neededSet[index] {
			return nil, httpx.Validation("button_urls names a button this template does not use.")
		}
		if strings.TrimSpace(value) == "" {
			return nil, httpx.Validation("A URL button value is empty.")
		}
		if utf8.RuneCountInString(value) > maxTemplateURLRunes {
			return nil, httpx.Validation("A URL button value is too long.")
		}
		out[index] = value
	}
	var missing []string
	for _, index := range needed {
		if _, ok := out[index]; !ok {
			missing = append(missing, index)
		}
	}
	if len(missing) > 0 {
		return nil, httpx.E(http.StatusUnprocessableEntity, "whatsapp_template_params_missing",
			"Fill in the template's URL buttons: "+strings.Join(missing, ", ")+".")
	}
	return out, nil
}

// templateURLButtonParams returns the indexes of a template's URL buttons whose URL holds a
// variable: those need a value to send.
func templateURLButtonParams(buttons []byte) []string {
	if len(buttons) == 0 {
		return nil
	}
	var list []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	if err := json.Unmarshal(buttons, &list); err != nil {
		return nil
	}
	var out []string
	for i, button := range list {
		if strings.EqualFold(strings.TrimSpace(button.Type), "URL") && placeholderPattern.MatchString(button.URL) {
			out = append(out, strconv.Itoa(i))
		}
	}
	return out
}
