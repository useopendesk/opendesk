package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/whatsapp/provider"
)

type sendWorker struct {
	river.WorkerDefaults[jobs.WhatsAppSendArgs]
	s *Service
}

func (w *sendWorker) Work(ctx context.Context, job *river.Job[jobs.WhatsAppSendArgs]) error {
	err := w.s.send(ctx, job.Args)
	var limited *provider.RateLimited
	if errors.As(err, &limited) {
		return river.JobSnooze(limited.RetryAfter)
	}
	if err != nil && job.Attempt >= job.MaxAttempts {
		// The last attempt: the agent is told, not left with a reply that never leaves.
		_ = w.s.fail(ctx, job.Args.WorkspaceID, job.Args.MessageID, "WhatsApp could not be reached. Send the message again.")
		return river.JobCancel(err)
	}
	return err
}

// send hands a pending reply to the provider and records the provider's id of it, which the
// delivery statuses that follow are matched by.
func (s *Service) send(ctx context.Context, a jobs.WhatsAppSendArgs) error {
	var text, status, template string
	var deleted *time.Time
	var cid *uuid.UUID
	var number string
	var rawVars []byte
	err := s.d.DB.Pool.QueryRow(ctx, `SELECT m.body_text, m.delivery_status, m.deleted_at, w.connection_id, w.number, w.template_id, w.variables
		FROM messages m JOIN whatsapp_messages w ON w.message_id = m.id WHERE m.workspace_id = $1 AND m.id = $2`, a.WorkspaceID, a.MessageID).
		Scan(&text, &status, &deleted, &cid, &number, &template, &rawVars)
	if db.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if deleted != nil || status != "pending" {
		return nil // deleted before it went out, or already sent
	}
	if cid == nil {
		return s.fail(ctx, a.WorkspaceID, a.MessageID, "The WhatsApp number is no longer connected.")
	}
	c, err := s.connection(ctx, s.d.DB.Pool, a.WorkspaceID, *cid)
	if db.IsNotFound(err) {
		return s.fail(ctx, a.WorkspaceID, a.MessageID, "The WhatsApp number is no longer connected.")
	}
	if err != nil {
		return err
	}
	ad, ok := s.adapters[c.Provider]
	if !ok {
		return s.fail(ctx, a.WorkspaceID, a.MessageID, "The WhatsApp provider is not available.")
	}
	creds, err := s.credentials(c)
	if err != nil {
		return err
	}
	vars := map[string]string{}
	_ = json.Unmarshal(rawVars, &vars)
	id, err := ad.Send(ctx, s.http, creds, provider.Outbound{From: c.Sender, To: number, Text: text, TemplateID: template, Variables: vars, StatusURL: s.statusURL(c)})
	var rejected *provider.Rejected
	switch {
	case errors.Is(err, provider.OutsideWindow):
		return s.fail(ctx, a.WorkspaceID, a.MessageID, failureText("63016"))
	case errors.As(err, &rejected):
		return s.fail(ctx, a.WorkspaceID, a.MessageID, rejected.Reason)
	case provider.Refused(err):
		return s.fail(ctx, a.WorkspaceID, a.MessageID, "The provider refused the connection's credentials. An admin can replace them in settings.")
	case err != nil:
		return err
	}
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE whatsapp_messages SET external_id = $2 WHERE message_id = $1`, a.MessageID, id); err != nil {
			return err
		}
		// A delivery status that beat this write is applied by its own job, which waits for the id.
		tag, err := tx.Exec(ctx, `UPDATE messages SET delivery_status = 'sent', delivery_error = '' WHERE workspace_id = $1 AND id = $2 AND delivery_status = 'pending'`,
			a.WorkspaceID, a.MessageID)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		return domain.PublishMessageUpdated(ctx, tx, a.WorkspaceID, a.MessageID)
	})
}

// fail marks a reply as not sent, with the reason the agent reads under it.
func (s *Service) fail(ctx context.Context, wid, mid uuid.UUID, reason string) error {
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE messages SET delivery_status = 'failed', delivery_error = $3 WHERE workspace_id = $1 AND id = $2 AND delivery_status = 'pending'`,
			wid, mid, reason)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		return domain.PublishMessageUpdated(ctx, tx, wid, mid)
	})
}
