package admin

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/version"
)

type AdminHealthPool struct {
	Total int32 `json:"total"`
	Idle  int32 `json:"idle"`
	InUse int32 `json:"in_use"`
	Max   int32 `json:"max"`
}

type AdminHealthDatabase struct {
	OK            bool            `json:"ok"`
	LatencyMS     int64           `json:"latency_ms"`
	SchemaVersion int64           `json:"schema_version"`
	Dirty         bool            `json:"dirty"`
	Pool          AdminHealthPool `json:"pool"`
}

type AdminHealthNATS struct {
	OK       bool `json:"ok"`
	Embedded bool `json:"embedded"`
}

type AdminHealthRelay struct {
	Unpublished        int    `json:"unpublished"`
	OldestUnpublishedS *int64 `json:"oldest_unpublished_s"`
}

type AdminHealthQueue struct {
	Queue     string `json:"queue"`
	Available int    `json:"available"`
	Running   int    `json:"running"`
	Retryable int    `json:"retryable"`
}

type AdminHealthJobs struct {
	Available        int                `json:"available"`
	Running          int                `json:"running"`
	Retryable        int                `json:"retryable"`
	Scheduled        int                `json:"scheduled"`
	Discarded24h     int                `json:"discarded_24h"`
	OldestAvailableS *int64             `json:"oldest_available_s"`
	ByQueue          []AdminHealthQueue `json:"by_queue"`
}

type AdminHealthMail struct {
	Driver           string `json:"driver"`
	InboundFailed24h int    `json:"inbound_failed_24h"`
	InboundPending   int    `json:"inbound_pending"`
	MailboxesInError int    `json:"mailboxes_in_error"`
}

type AdminHealthWebhooks struct {
	Failing24h int `json:"failing_24h"`
}

type AdminHealthConfig struct {
	Billing           bool   `json:"billing"`
	AIProvider        string `json:"ai_provider"`
	DecisionsProvider string `json:"decisions_provider"`
	Embeddings        string `json:"embeddings"`
	Storage           string `json:"storage"`
	SignupEnabled     bool   `json:"signup_enabled"`
	MailboxIMAP       bool   `json:"mailbox_imap"`
	MailboxSystemSend bool   `json:"mailbox_system_send"`
	Push              string `json:"push"`
	HelpDomain        string `json:"help_domain"`
}

type AdminHealthCheck struct {
	Name   string `json:"name"`
	Status string `json:"status" enum:"ok,warn,fail"`
	Detail string `json:"detail"`
}

type AdminHealth struct {
	Version   string              `json:"version"`
	Commit    string              `json:"commit"`
	Role      string              `json:"role"`
	StartedAt time.Time           `json:"started_at"`
	Database  AdminHealthDatabase `json:"database"`
	NATS      AdminHealthNATS     `json:"nats"`
	Relay     AdminHealthRelay    `json:"relay"`
	Jobs      AdminHealthJobs     `json:"jobs"`
	Mail      AdminHealthMail     `json:"mail"`
	Webhooks  AdminHealthWebhooks `json:"webhooks"`
	Config    AdminHealthConfig   `json:"config"`
	Checks    []AdminHealthCheck  `json:"checks"`
}

// Thresholds for the health checks.
const (
	relayWarnAfter = 30 * time.Second
	relayFailAfter = 5 * time.Minute
	jobsWarnAfter  = 5 * time.Minute
	jobsFailAfter  = 30 * time.Minute
)

func (s *svc) registerHealth(api huma.API) {
	op(s, api, huma.Operation{OperationID: "admin-health", Method: http.MethodGet, Path: "/v1/admin/health", Summary: "Instance health"},
		func(ctx context.Context, _ *struct{}) (*struct{ Body AdminHealth }, error) {
			if _, err := require(ctx, PermRead); err != nil {
				return nil, err
			}
			h, err := s.health(ctx)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body AdminHealth }{h}, nil
		})
}

func (s *svc) health(ctx context.Context) (AdminHealth, error) {
	cfg := s.d.Cfg
	h := AdminHealth{Version: version.Version, Commit: version.Commit, Role: cfg.Role, StartedAt: s.started,
		Config: AdminHealthConfig{Billing: cfg.BillingEnabled(), AIProvider: cfg.AIProvider, DecisionsProvider: cfg.DecisionsProvider,
			Embeddings: cfg.EmbeddingsDriver, Storage: cfg.StorageDriver, SignupEnabled: cfg.SignupEnabled, MailboxIMAP: cfg.MailboxIMAP,
			MailboxSystemSend: cfg.MailboxSystemSend, Push: cfg.PushMode(), HelpDomain: cfg.HelpDomain},
		Mail: AdminHealthMail{Driver: cfg.MailDriver}}
	q := s.d.DB.Pool

	pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	start := time.Now()
	err := q.Ping(pctx)
	cancel()
	h.Database.LatencyMS = time.Since(start).Milliseconds()
	h.Database.OK = err == nil
	st := q.Stat()
	h.Database.Pool = AdminHealthPool{Total: st.TotalConns(), Idle: st.IdleConns(), InUse: st.AcquiredConns(), Max: st.MaxConns()}
	if err := q.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&h.Database.SchemaVersion, &h.Database.Dirty); err != nil && err != pgx.ErrNoRows {
		return h, err
	}

	h.NATS = AdminHealthNATS{OK: s.d.NATS != nil && s.d.NATS.IsConnected(), Embedded: cfg.NATSURL == ""}

	if err := q.QueryRow(ctx, `SELECT count(*), extract(epoch FROM now() - min(created_at))::bigint FROM events WHERE published_at IS NULL`).
		Scan(&h.Relay.Unpublished, &h.Relay.OldestUnpublishedS); err != nil {
		return h, err
	}

	j := &h.Jobs
	if err := q.QueryRow(ctx, `SELECT count(*) FILTER (WHERE state = 'available'), count(*) FILTER (WHERE state = 'running'),
		count(*) FILTER (WHERE state = 'retryable'), count(*) FILTER (WHERE state = 'scheduled'),
		count(*) FILTER (WHERE state = 'discarded' AND finalized_at > now() - interval '24 hours'),
		extract(epoch FROM now() - min(scheduled_at) FILTER (WHERE state = 'available'))::bigint
		FROM river_job WHERE state IN ('available', 'running', 'retryable', 'scheduled', 'discarded')`).
		Scan(&j.Available, &j.Running, &j.Retryable, &j.Scheduled, &j.Discarded24h, &j.OldestAvailableS); err != nil {
		return h, err
	}
	rows, err := q.Query(ctx, `SELECT queue, count(*) FILTER (WHERE state = 'available'), count(*) FILTER (WHERE state = 'running'),
		count(*) FILTER (WHERE state = 'retryable') FROM river_job WHERE state IN ('available', 'running', 'retryable')
		GROUP BY queue ORDER BY queue`)
	if err != nil {
		return h, err
	}
	if j.ByQueue, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (AdminHealthQueue, error) {
		var x AdminHealthQueue
		err := r.Scan(&x.Queue, &x.Available, &x.Running, &x.Retryable)
		return x, err
	}); err != nil {
		return h, err
	}
	if j.ByQueue == nil {
		j.ByQueue = []AdminHealthQueue{}
	}

	if err := q.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM inbound_emails WHERE status = 'failed' AND received_at > now() - interval '24 hours'),
		(SELECT count(*) FROM inbound_emails WHERE status = 'pending'),
		(SELECT count(*) FROM mailboxes WHERE status = 'error'),
		(SELECT count(*) FROM webhook_deliveries WHERE status = 'failed' AND created_at > now() - interval '24 hours')`).
		Scan(&h.Mail.InboundFailed24h, &h.Mail.InboundPending, &h.Mail.MailboxesInError, &h.Webhooks.Failing24h); err != nil {
		return h, err
	}

	h.Checks = checks(h)
	return h, nil
}

// checks turns the numbers into what needs attention.
func checks(h AdminHealth) []AdminHealthCheck {
	var out []AdminHealthCheck
	add := func(name, status, detail string) {
		out = append(out, AdminHealthCheck{Name: name, Status: status, Detail: detail})
	}

	switch {
	case !h.Database.OK:
		add("Database", "fail", "The database did not answer a ping.")
	case h.Database.Dirty:
		add("Database", "fail", fmt.Sprintf("Migration %d stopped halfway. Fix it by hand, then run opendesk migrate.", h.Database.SchemaVersion))
	case h.Database.LatencyMS > 200:
		add("Database", "warn", fmt.Sprintf("A ping took %d ms.", h.Database.LatencyMS))
	default:
		add("Database", "ok", fmt.Sprintf("Schema version %d, ping %d ms.", h.Database.SchemaVersion, h.Database.LatencyMS))
	}

	if h.NATS.OK {
		add("NATS", "ok", "Connected.")
	} else {
		add("NATS", "fail", "Not connected: live updates and webhooks are stalled.")
	}

	switch age := secs(h.Relay.OldestUnpublishedS); {
	case h.Relay.Unpublished == 0:
		add("Event relay", "ok", "Every event is published.")
	case age > relayFailAfter:
		add("Event relay", "fail", fmt.Sprintf("%d events wait, the oldest for %s. Is a worker running?", h.Relay.Unpublished, age.Round(time.Second)))
	case age > relayWarnAfter:
		add("Event relay", "warn", fmt.Sprintf("%d events wait, the oldest for %s.", h.Relay.Unpublished, age.Round(time.Second)))
	default:
		add("Event relay", "ok", fmt.Sprintf("%d events on their way.", h.Relay.Unpublished))
	}

	switch age := secs(h.Jobs.OldestAvailableS); {
	case age > jobsFailAfter:
		add("Background jobs", "fail", fmt.Sprintf("%d jobs wait, the oldest for %s. Is a worker running?", h.Jobs.Available, age.Round(time.Second)))
	case age > jobsWarnAfter:
		add("Background jobs", "warn", fmt.Sprintf("%d jobs wait, the oldest for %s.", h.Jobs.Available, age.Round(time.Second)))
	case h.Jobs.Discarded24h > 0:
		add("Background jobs", "warn", fmt.Sprintf("%d jobs gave up in the last 24 hours.", h.Jobs.Discarded24h))
	default:
		add("Background jobs", "ok", fmt.Sprintf("%d waiting, %d running, %d to retry.", h.Jobs.Available, h.Jobs.Running, h.Jobs.Retryable))
	}

	switch {
	case h.Mail.Driver == "log":
		add("Platform mail", "warn", "MAIL_DRIVER is log: sign-in and notification emails are written to the log, not sent.")
	case h.Mail.InboundFailed24h > 0:
		add("Platform mail", "warn", fmt.Sprintf("%d inbound emails failed in the last 24 hours.", h.Mail.InboundFailed24h))
	default:
		add("Platform mail", "ok", "Sending with "+h.Mail.Driver+".")
	}

	if h.Mail.MailboxesInError > 0 {
		add("Mailboxes", "warn", fmt.Sprintf("%d mailboxes cannot connect.", h.Mail.MailboxesInError))
	} else {
		add("Mailboxes", "ok", "Every mailbox connects.")
	}

	if h.Webhooks.Failing24h > 0 {
		add("Webhooks", "warn", fmt.Sprintf("%d webhook deliveries failed in the last 24 hours.", h.Webhooks.Failing24h))
	} else {
		add("Webhooks", "ok", "No failed deliveries in the last 24 hours.")
	}
	return out
}

func secs(v *int64) time.Duration {
	if v == nil {
		return 0
	}
	return time.Duration(*v) * time.Second
}
