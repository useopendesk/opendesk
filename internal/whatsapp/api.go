package whatsapp

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/audit"
	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/displayname"
	"github.com/useopendesk/opendesk/internal/events"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/ids"
	"github.com/useopendesk/opendesk/internal/whatsapp/provider"
)

// Health codes the settings page words in the reader's language (health_code).
const (
	HealthReceiving   = "receiving"           // a message or a delivery status came in at the webhook
	HealthNoWebhook   = "webhook_missing"     // nothing has come in yet
	HealthRefused     = "credentials_refused" // the provider refused the credentials
	HealthUnreachable = "provider_unreachable"
)

var healthDetails = map[string]string{
	HealthReceiving:   "Messages are sent to opendesk.",
	HealthNoWebhook:   "No message has come in yet. Add the webhook URL in the provider, then send a message to the number.",
	HealthRefused:     "The provider refused the credentials. Replace them with working ones.",
	HealthUnreachable: "Couldn't reach the provider. Try again in a moment.",
}

// Provider is a provider the instance can connect.
type WhatsAppProvider struct {
	Key    string                  `json:"key"`
	Name   string                  `json:"name"`
	Fields []WhatsAppProviderField `json:"fields"`
	Docs   string                  `json:"docs" doc:"Where an admin sets the webhook URL in the provider"`
}

// WhatsAppProviderField is a credential the admin pastes.
type WhatsAppProviderField struct {
	Name   string `json:"name"`
	Label  string `json:"label"`
	Secret bool   `json:"secret"`
}

// WhatsAppConnectionView is a connection with the code its health detail is worded by.
type WhatsAppConnectionView struct {
	WhatsAppConnection
	HealthCode string `json:"health_code" enum:",receiving,webhook_missing,credentials_refused,provider_unreachable"`
}

type whatsappConnectionList struct {
	Data []WhatsAppConnectionView `json:"data"`
}

var senderRe = regexp.MustCompile(`^\+[0-9]{6,15}$`)

// NormalizeNumber reads a number as E.164: "whatsapp:" dropped, spaces, dots, dashes and
// brackets dropped, a 00 prefix as +. "" when it is not a number with its country code.
func NormalizeNumber(s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "whatsapp:")
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r == '+':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '.' || r == '(' || r == ')':
		default:
			return ""
		}
	}
	n := b.String()
	if strings.HasPrefix(n, "00") {
		n = "+" + n[2:]
	}
	if !senderRe.MatchString(n) {
		return ""
	}
	return n
}

func (s *Service) webhookURL(c WhatsAppConnection) string {
	return strings.TrimRight(s.d.Cfg.APIURL, "/") + "/v1/whatsapp/hooks/" + c.Provider + "/" + c.ID.String()
}

func (s *Service) statusURL(c WhatsAppConnection) string { return s.webhookURL(c) + "/status" }

func seesWebhook(p *auth.Principal) bool {
	switch p.Kind {
	case auth.KindAPIKey:
		return p.HasScope(auth.ScopeSettingsWrite)
	case auth.KindUser:
		return p.Role.AtLeast(auth.RoleAdmin) && (p.GrantID == uuid.Nil || p.HasScope(auth.ScopeSettingsWrite))
	}
	return false
}

func (s *Service) view(c WhatsAppConnection, admin bool) WhatsAppConnectionView {
	c.WebhookURL, c.StatusURL = "", ""
	if admin {
		c.WebhookURL, c.StatusURL = s.webhookURL(c), s.statusURL(c)
	}
	v := WhatsAppConnectionView{WhatsAppConnection: c}
	for code, detail := range healthDetails {
		if detail == c.HealthDetail {
			v.HealthCode = code
		}
	}
	return v
}

func requireAdminUser(ctx context.Context) (*auth.Principal, error) {
	p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsWrite)
	if err != nil {
		return nil, httpx.Wrap(err)
	}
	if p.Kind != auth.KindUser {
		return nil, httpx.Forbidden()
	}
	return p, nil
}

func cleanCredentials(d provider.Descriptor, in map[string]string) map[string]string {
	out := map[string]string{}
	for _, f := range d.Fields {
		v := strings.TrimSpace(in[f.Name])
		if v == "" || len(v) > 500 {
			return nil
		}
		out[f.Name] = v
	}
	return out
}

func credentialsRefused(err error) error {
	if provider.Refused(err) {
		return httpx.E(422, "whatsapp_credentials_refused", "The provider refused these credentials.")
	}
	return httpx.E(422, "whatsapp_credentials_refused", "The provider could not check these credentials. Try again in a moment.")
}

// healthOf is the health a validation's outcome means for a connection.
func healthOf(c WhatsAppConnection, err error) (status, code string) {
	switch {
	case err == nil && c.LastEventAt != nil:
		return "healthy", HealthReceiving
	case err == nil:
		return "action_required", HealthNoWebhook
	case provider.Refused(err):
		return "error", HealthRefused
	default:
		return "error", HealthUnreachable
	}
}

// storeHealth records a connection's health and tells admins.
func (s *Service) storeHealth(ctx context.Context, tx pgx.Tx, c WhatsAppConnection, status, code string) (WhatsAppConnection, error) {
	now := time.Now().UTC()
	c.HealthStatus, c.HealthDetail, c.HealthCheckedAt = status, healthDetails[code], &now
	c.Status, c.LastError = "connected", ""
	if status == "error" {
		c.Status, c.LastError = "error", healthDetails[code]
	}
	if _, err := tx.Exec(ctx, `UPDATE whatsapp_connections SET health_status = $3, health_detail = $4, health_checked_at = $5, status = $6, last_error = $7,
		updated_at = now() WHERE workspace_id = $1 AND id = $2`, c.WorkspaceID, c.ID, c.HealthStatus, c.HealthDetail, now, c.Status, c.LastError); err != nil {
		return c, err
	}
	_, err := events.Publish(ctx, tx, events.Event{WorkspaceID: c.WorkspaceID, Type: "whatsapp_connection.updated", Admins: true,
		Data: map[string]any{"whatsapp_connection": s.view(c, true)}})
	return c, err
}

// check validates a connection's credentials with its provider and stores the health.
func (s *Service) check(ctx context.Context, c WhatsAppConnection) (WhatsAppConnection, error) {
	a, ok := s.adapters[c.Provider]
	if !ok {
		return c, httpx.Validation("Unknown WhatsApp provider.")
	}
	creds, err := s.credentials(c)
	if err != nil {
		return c, err
	}
	account, verr := a.Validate(ctx, s.http, creds)
	if verr == nil {
		c.AccountName = displayname.CleanOr(account.Name, displayname.Workspace, "")
	}
	status, code := healthOf(c, verr)
	err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		if verr == nil {
			if _, err := tx.Exec(ctx, `UPDATE whatsapp_connections SET account_name = $3 WHERE workspace_id = $1 AND id = $2`, c.WorkspaceID, c.ID, c.AccountName); err != nil {
				return err
			}
		}
		var err error
		c, err = s.storeHealth(ctx, tx, c, status, code)
		return err
	})
	return c, err
}

func (s *Service) workspaceConnection(ctx context.Context, wid uuid.UUID, raw string) (WhatsAppConnection, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return WhatsAppConnection{}, httpx.NotFound("WhatsApp connection")
	}
	c, err := s.connection(ctx, s.d.DB.Pool, wid, id)
	if db.IsNotFound(err) {
		return WhatsAppConnection{}, httpx.NotFound("WhatsApp connection")
	}
	if err != nil {
		return WhatsAppConnection{}, httpx.Wrap(err)
	}
	return c, nil
}

func (s *Service) brandBelongs(ctx context.Context, wid uuid.UUID, brand *uuid.UUID) error {
	if brand == nil {
		return nil
	}
	var one int
	err := s.d.DB.Pool.QueryRow(ctx, `SELECT 1 FROM brands WHERE workspace_id = $1 AND id = $2`, wid, *brand).Scan(&one)
	if db.IsNotFound(err) {
		return httpx.Validation("Unknown brand.")
	}
	return err
}

// Register mounts the WhatsApp API and the providers' webhooks.
func (s *Service) Register(api huma.API, r chi.Router) {
	tags := []string{"WhatsApp"}
	huma.Register(api, huma.Operation{OperationID: "list-whatsapp-providers", Method: http.MethodGet, Path: "/v1/whatsapp/providers", Tags: tags, Summary: "List WhatsApp providers"},
		func(ctx context.Context, _ *struct{}) (*struct {
			Body struct {
				Data []WhatsAppProvider `json:"data"`
			}
		}, error) {
			if _, err := auth.RequireWorkspace(ctx, auth.RoleViewer, auth.ScopeSettingsRead); err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct {
				Body struct {
					Data []WhatsAppProvider `json:"data"`
				}
			}{}
			out.Body.Data = []WhatsAppProvider{}
			for _, a := range s.adapters {
				d := a.Describe()
				p := WhatsAppProvider{Key: d.Key, Name: d.Name, Docs: d.Docs, Fields: []WhatsAppProviderField{}}
				for _, f := range d.Fields {
					p.Fields = append(p.Fields, WhatsAppProviderField{Name: f.Name, Label: f.Label, Secret: f.Secret})
				}
				out.Body.Data = append(out.Body.Data, p)
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "list-whatsapp-connections", Method: http.MethodGet, Path: "/v1/whatsapp/connections", Tags: tags, Summary: "List WhatsApp connections"},
		func(ctx context.Context, _ *struct{}) (*struct{ Body whatsappConnectionList }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleViewer, auth.ScopeSettingsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			list, err := s.listConnections(ctx, p.WorkspaceID)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct{ Body whatsappConnectionList }{whatsappConnectionList{Data: []WhatsAppConnectionView{}}}
			for _, c := range list {
				out.Body.Data = append(out.Body.Data, s.view(c, seesWebhook(p)))
			}
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "create-whatsapp-connection", Method: http.MethodPost, Path: "/v1/whatsapp/connections", Tags: tags, Summary: "Connect a WhatsApp number", DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *struct {
			Body struct {
				Provider    string            `json:"provider"`
				Name        string            `json:"name,omitempty" maxLength:"80"`
				Sender      string            `json:"sender" doc:"The WhatsApp number customers write to, with its country code"`
				BrandID     *uuid.UUID        `json:"brand_id,omitempty"`
				Credentials map[string]string `json:"credentials"`
			}
		}) (*struct{ Body WhatsAppConnectionView }, error) {
			p, err := requireAdminUser(ctx)
			if err != nil {
				return nil, err
			}
			a, ok := s.adapters[strings.TrimSpace(in.Body.Provider)]
			if !ok {
				return nil, httpx.Validation("Unknown WhatsApp provider.")
			}
			sender := NormalizeNumber(in.Body.Sender)
			if sender == "" {
				return nil, httpx.Validation("Enter the WhatsApp number with its country code, like +4915123456789.")
			}
			creds := cleanCredentials(a.Describe(), in.Body.Credentials)
			if creds == nil {
				return nil, httpx.Validation("Provide every required credential.")
			}
			if err := s.brandBelongs(ctx, p.WorkspaceID, in.Body.BrandID); err != nil {
				return nil, httpx.Wrap(err)
			}
			account, err := a.Validate(ctx, s.http, creds)
			if err != nil {
				s.d.Log.Info("whatsapp connect refused", "provider", a.Describe().Key, "err", err)
				return nil, credentialsRefused(err)
			}
			sealed, err := s.d.Box.SealJSON(creds)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			name := strings.TrimSpace(in.Body.Name)
			if name == "" {
				name = displayname.CleanOr(account.Name, displayname.Workspace, a.Describe().Name)
			} else if name, err = displayname.Validate("Name", name, displayname.Workspace, false); err != nil {
				return nil, httpx.Wrap(err)
			}
			c := WhatsAppConnection{ID: ids.New(), WorkspaceID: p.WorkspaceID, Provider: a.Describe().Key, Name: name, Sender: sender,
				AccountName: displayname.CleanOr(account.Name, displayname.Workspace, ""), Status: "connected", HealthStatus: "unknown",
				BrandID: in.Body.BrandID, ReuseDays: 7, CreatedAt: time.Now().UTC(), credentialsEnc: sealed}
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `INSERT INTO whatsapp_connections (id, workspace_id, provider, name, sender, account_name, credentials_enc, brand_id, created_by, created_at)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, c.ID, c.WorkspaceID, c.Provider, c.Name, c.Sender, c.AccountName, c.credentialsEnc, c.BrandID, p.UserID, c.CreatedAt); err != nil {
					return err
				}
				var err error
				if c, err = s.storeHealth(ctx, tx, c, "action_required", HealthNoWebhook); err != nil {
					return err
				}
				if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: p.WorkspaceID, Type: "whatsapp_connection.created", Admins: true,
					Data: map[string]any{"whatsapp_connection": s.view(c, true)}}); err != nil {
					return err
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, "whatsapp.connected", "whatsapp_connection", &c.ID, map[string]any{"provider": c.Provider})
			})
			if db.IsUniqueViolation(err, "") {
				return nil, httpx.Conflict("This workspace already has a connection for that number.")
			}
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body WhatsAppConnectionView }{s.view(c, true)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "update-whatsapp-connection", Method: http.MethodPatch, Path: "/v1/whatsapp/connections/{id}", Tags: tags, Summary: "Update a WhatsApp connection"},
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body struct {
				Name        *string           `json:"name,omitempty" maxLength:"80"`
				BrandID     *uuid.UUID        `json:"brand_id,omitempty"`
				ClearBrand  bool              `json:"clear_brand,omitempty" doc:"Take the number off its brand"`
				ReuseDays   *int              `json:"reuse_days,omitempty" minimum:"0" maximum:"90" doc:"A message goes on the customer's latest ticket solved within this many days when they have no open one; 0 opens a new ticket instead"`
				Credentials map[string]string `json:"credentials,omitempty" doc:"Replacement credentials, checked with the provider"`
			}
		}) (*struct{ Body WhatsAppConnectionView }, error) {
			p, err := requireAdminUser(ctx)
			if err != nil {
				return nil, err
			}
			c, err := s.workspaceConnection(ctx, p.WorkspaceID, in.ID)
			if err != nil {
				return nil, err
			}
			b := in.Body
			changes := map[string]any{}
			if b.Name != nil {
				name, err := displayname.Validate("Name", strings.TrimSpace(*b.Name), displayname.Workspace, false)
				if err != nil {
					return nil, httpx.Wrap(err)
				}
				c.Name = name
				changes["name"] = name
			}
			if b.BrandID != nil || b.ClearBrand {
				if b.BrandID != nil && b.ClearBrand {
					return nil, httpx.Validation("Choose a brand or clear it, not both.")
				}
				if err := s.brandBelongs(ctx, p.WorkspaceID, b.BrandID); err != nil {
					return nil, httpx.Wrap(err)
				}
				c.BrandID = b.BrandID
				changes["brand_id"] = b.BrandID
			}
			if b.ReuseDays != nil {
				c.ReuseDays = *b.ReuseDays
				changes["reuse_days"] = *b.ReuseDays
			}
			replaced := false
			if len(b.Credentials) > 0 {
				a, ok := s.adapters[c.Provider]
				if !ok {
					return nil, httpx.Validation("Unknown WhatsApp provider.")
				}
				creds := cleanCredentials(a.Describe(), b.Credentials)
				if creds == nil {
					return nil, httpx.Validation("Provide every required credential.")
				}
				account, err := a.Validate(ctx, s.http, creds)
				if err != nil {
					return nil, credentialsRefused(err)
				}
				if c.credentialsEnc, err = s.d.Box.SealJSON(creds); err != nil {
					return nil, httpx.Wrap(err)
				}
				c.AccountName = displayname.CleanOr(account.Name, displayname.Workspace, "")
				replaced = true
				changes["credentials"] = "replaced"
				s.templates.Delete(c.ID)
			}
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `UPDATE whatsapp_connections SET name = $3, brand_id = $4, reuse_days = $5, credentials_enc = $6, account_name = $7, updated_at = now()
					WHERE workspace_id = $1 AND id = $2`, p.WorkspaceID, c.ID, c.Name, c.BrandID, c.ReuseDays, c.credentialsEnc, c.AccountName); err != nil {
					return err
				}
				if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: p.WorkspaceID, Type: "whatsapp_connection.updated", Admins: true,
					Data: map[string]any{"whatsapp_connection": s.view(c, true)}}); err != nil {
					return err
				}
				action := "whatsapp.updated"
				if replaced {
					action = "whatsapp.credentials_rotated"
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, action, "whatsapp_connection", &c.ID, changes)
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body WhatsAppConnectionView }{s.view(c, true)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "test-whatsapp-connection", Method: http.MethodPost, Path: "/v1/whatsapp/connections/{id}/test", Tags: tags,
		Summary: "Test a WhatsApp connection", Description: "Checks the credentials with the provider. The webhook counts as working once a message or a delivery status came in at its URL."},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct{ Body WhatsAppConnectionView }, error) {
			p, err := requireAdminUser(ctx)
			if err != nil {
				return nil, err
			}
			c, err := s.workspaceConnection(ctx, p.WorkspaceID, in.ID)
			if err != nil {
				return nil, err
			}
			if c, err = s.check(ctx, c); err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body WhatsAppConnectionView }{s.view(c, true)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "delete-whatsapp-connection", Method: http.MethodDelete, Path: "/v1/whatsapp/connections/{id}", Tags: tags,
		Summary: "Disconnect a WhatsApp number", DefaultStatus: http.StatusNoContent,
		Description: "Removes the connection. Conversations already on tickets stay. The webhook URL is the admin's to remove in the provider."},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct{}, error) {
			p, err := requireAdminUser(ctx)
			if err != nil {
				return nil, err
			}
			c, err := s.workspaceConnection(ctx, p.WorkspaceID, in.ID)
			if err != nil {
				return nil, err
			}
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `DELETE FROM whatsapp_connections WHERE workspace_id = $1 AND id = $2`, p.WorkspaceID, c.ID); err != nil {
					return err
				}
				if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: p.WorkspaceID, Type: "whatsapp_connection.deleted", Admins: true,
					Data: map[string]any{"whatsapp_connection_id": c.ID}}); err != nil {
					return err
				}
				return audit.Log(ctx, tx, p, p.WorkspaceID, "whatsapp.disconnected", "whatsapp_connection", &c.ID, map[string]any{"provider": c.Provider})
			})
			s.templates.Delete(c.ID)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return nil, nil
		})

	s.registerTicketRoutes(api)
	r.Post("/v1/whatsapp/hooks/{provider}/{id}", s.webhook)
	r.Post("/v1/whatsapp/hooks/{provider}/{id}/status", s.webhook)
}
