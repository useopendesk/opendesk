// Package whatsapp connects WhatsApp, through a business solution provider (Twilio first),
// to the helpdesk as a customer channel (contracts/whatsapp.md): a customer's message lands
// on their ticket, and the team answers from the ticket. Provider adapters
// (internal/whatsapp/provider) only translate protocols: the engine owns connections,
// sealed credentials, matching, placement, the customer-service window and every ticket write.
package whatsapp

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/whatsapp/provider"
	"github.com/useopendesk/opendesk/internal/whatsapp/provider/twilio"
)

// Window is how long after a customer's last message a free-form reply is allowed: WhatsApp's
// customer-service window. After it only an approved template may be sent.
const Window = 24 * time.Hour

// Service is the WhatsApp engine of one set of dependencies.
type Service struct {
	d        *deps.Deps
	adapters map[string]provider.Adapter
	http     *http.Client
	// templates caches each connection's approved templates for templatesTTL.
	templates sync.Map
}

const templatesTTL = 5 * time.Minute

type cachedTemplates struct {
	at   time.Time
	list []provider.WhatsAppTemplate
}

var services sync.Map

// For returns the engine for d, with every provider compiled in.
func For(d *deps.Deps) *Service {
	if got, ok := services.Load(d); ok {
		return got.(*Service)
	}
	got, _ := services.LoadOrStore(d, New(d, twilio.Adapter{}))
	return got.(*Service)
}

// New builds an engine with these adapters (tests give theirs).
func New(d *deps.Deps, adapters ...provider.Adapter) *Service {
	s := &Service{d: d, adapters: map[string]provider.Adapter{}, http: &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
	for _, a := range adapters {
		s.adapters[a.Describe().Key] = a
	}
	return s
}

func Register(d *deps.Deps, api huma.API, r chi.Router) { For(d).Register(api, r) }

func RegisterWorkers(d *deps.Deps, w *river.Workers) {
	s := For(d)
	river.AddWorker(w, &inboundWorker{s: s})
	river.AddWorker(w, &sendWorker{s: s})
	river.AddWorker(w, &pruneWorker{s: s})
}

func Periodic(*deps.Deps) []*river.PeriodicJob {
	return []*river.PeriodicJob{river.NewPeriodicJob(river.PeriodicInterval(6*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) { return jobs.WhatsAppPruneArgs{}, nil }, nil)}
}

func Background(context.Context, *deps.Deps) {}

// WhatsAppConnection is a workspace's connection to a WhatsApp provider (contracts/whatsapp.md).
type WhatsAppConnection struct {
	ID              uuid.UUID  `json:"id"`
	WorkspaceID     uuid.UUID  `json:"-"`
	Provider        string     `json:"provider"`
	Name            string     `json:"name"`
	Sender          string     `json:"sender" doc:"The WhatsApp number customers write to, in E.164"`
	AccountName     string     `json:"account_name"`
	Status          string     `json:"status" enum:"connected,error"`
	LastError       string     `json:"last_error"`
	HealthStatus    string     `json:"health_status" enum:"unknown,healthy,action_required,error"`
	HealthDetail    string     `json:"health_detail"`
	HealthCheckedAt *time.Time `json:"health_checked_at"`
	BrandID         *uuid.UUID `json:"brand_id" doc:"The brand this number's conversations belong to"`
	ReuseDays       int        `json:"reuse_days"`
	LastEventAt     *time.Time `json:"last_event_at"`
	// WebhookURL is where the provider sends messages: admins only.
	WebhookURL string `json:"webhook_url,omitempty" doc:"Admins only"`
	// StatusURL is where the provider reports delivery of what opendesk sends: admins only.
	StatusURL string    `json:"status_url,omitempty" doc:"Admins only"`
	CreatedAt time.Time `json:"created_at"`

	credentialsEnc []byte
}

const connectionSelect = `SELECT id, workspace_id, provider, name, sender, account_name, credentials_enc, status, last_error,
	health_status, health_detail, health_checked_at, brand_id, reuse_days, last_event_at, created_at FROM whatsapp_connections`

func scanConnection(row pgx.Row) (WhatsAppConnection, error) {
	var c WhatsAppConnection
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.Provider, &c.Name, &c.Sender, &c.AccountName, &c.credentialsEnc, &c.Status, &c.LastError,
		&c.HealthStatus, &c.HealthDetail, &c.HealthCheckedAt, &c.BrandID, &c.ReuseDays, &c.LastEventAt, &c.CreatedAt)
	return c, err
}

func (s *Service) listConnections(ctx context.Context, wid uuid.UUID) ([]WhatsAppConnection, error) {
	rows, err := s.d.DB.Pool.Query(ctx, connectionSelect+` WHERE workspace_id = $1 ORDER BY created_at, id`, wid)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (WhatsAppConnection, error) { return scanConnection(r) })
}

func (s *Service) connection(ctx context.Context, q db.Querier, wid, id uuid.UUID) (WhatsAppConnection, error) {
	return scanConnection(q.QueryRow(ctx, connectionSelect+` WHERE workspace_id = $1 AND id = $2`, wid, id))
}

func (s *Service) credentials(c WhatsAppConnection) (map[string]string, error) {
	out := map[string]string{}
	return out, s.d.Box.OpenJSON(c.credentialsEnc, &out)
}

func (s *Service) describe(key string) provider.Descriptor {
	if a, ok := s.adapters[key]; ok {
		return a.Describe()
	}
	return provider.Descriptor{Key: key}
}
