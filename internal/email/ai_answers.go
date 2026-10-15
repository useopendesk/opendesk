package email

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/jobs"
)

// Ollie answers email (contracts/agent.md "Email"): the email side decides which
// inbound mail may reach the agent at all and queues the job; internal/ai decides
// whether and how to answer.

// aiAnswerDelay lets the ticket's automations and assignment run first and gives a
// burst of quick emails one answer: the job of an earlier message sees it is no longer
// the latest and does nothing.
const aiAnswerDelay = 30 * time.Second

var errAIOff = httpx.E(http.StatusUnprocessableEntity, "ai_off", "Turn on AI in Settings > AI before Ollie answers emails.")

// checkAIOn refuses turning email answers on while AI is off for the instance or the
// workspace.
func (s *service) checkAIOn(ctx context.Context, tx pgx.Tx, wid uuid.UUID) error {
	if s.d.Cfg == nil || !s.d.Cfg.AIEnabled() {
		return errAIOff
	}
	st, err := hd.LoadSettings(ctx, tx, wid)
	if err != nil {
		return err
	}
	if !st.AI().Enabled {
		return errAIOff
	}
	return nil
}

// noReplyLocals are sender local parts that never read a reply.
var noReplyLocals = []string{"noreply", "no-reply", "no_reply", "donotreply", "do-not-reply", "do_not_reply",
	"mailer-daemon", "postmaster", "bounce", "bounces", "notifications", "notification", "alerts", "newsletter"}

// aiAnswerable reports whether an inbound message may be answered by the agent. On top
// of the noise inbound already drops (bounces, Auto-Submitted, Precedence bulk/junk,
// auto-replies), it refuses list and bulk mail, no-reply senders and anything that
// looks automatic, because an automatic answer to automatic mail can loop.
func aiAnswerable(pm *parsedMail, sender string) bool {
	if v := strings.ToLower(strings.TrimSpace(pm.header("Auto-Submitted"))); v != "" && v != "no" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(pm.header("Precedence"))) {
	case "bulk", "list", "junk", "auto_reply":
		return false
	}
	for _, h := range []string{"List-Id", "List-Unsubscribe", "List-Post", "X-Autoreply", "X-Autorespond", "X-Auto-Response-Suppress",
		"X-Loop", "Feedback-ID", "X-Campaign", "X-Mailchimp-Campaign"} {
		if strings.TrimSpace(pm.header(h)) != "" {
			return false
		}
	}
	if rp := strings.TrimSpace(pm.header("Return-Path")); rp == "<>" {
		return false
	}
	if looksAutomatic(pm) {
		return false
	}
	for _, addr := range []string{sender, pm.FromAddr} {
		if noReplySender(addr) {
			return false
		}
	}
	return true
}

// queueAIAnswer queues the agent for an inbound customer message when the mailbox has
// email answers on and the message is one it may answer. It only inserts a job.
func (s *service) queueAIAnswer(ctx context.Context, tx pgx.Tx, m *mailboxRow, ticketID, messageID uuid.UUID, pm *parsedMail, sender string) error {
	if !m.AIAnswers || s.d.Jobs == nil || s.d.Cfg == nil || !s.d.Cfg.AIEnabled() || !aiAnswerable(pm, sender) {
		return nil
	}
	_, err := s.d.Jobs.InsertTx(ctx, tx, jobs.AIEmailAnswerArgs{WorkspaceID: m.WorkspaceID, TicketID: ticketID, MessageID: messageID},
		&river.InsertOpts{MaxAttempts: 2, ScheduledAt: time.Now().Add(aiAnswerDelay), UniqueOpts: river.UniqueOpts{ByArgs: true}})
	return err
}
