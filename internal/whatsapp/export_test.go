package whatsapp

import (
	"context"

	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/jobs"
)

// Send runs the send job's work, which the tests drive in place of the queue.
func (s *Service) Send(ctx context.Context, a jobs.WhatsAppSendArgs) error { return s.send(ctx, a) }

// Install makes s the engine For(d) answers, so the ticket routes reach a test's provider.
func Install(d *deps.Deps, s *Service) { services.Store(d, s) }
