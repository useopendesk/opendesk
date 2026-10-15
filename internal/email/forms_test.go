package email

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jhillyerd/enmime/v2"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/ids"
)

// formMail is a website form's message: From the site, Reply-To the visitor.
func formMail(authResults, from, replyTo, to string) string {
	return authResults + "From: Website <" + from + ">\r\nReply-To: " + replyTo + "\r\nTo: " + to +
		"\r\nSubject: Contact form\r\nMessage-ID: <f" + crypto.RandomString(10) + "@site.test>\r\n\r\nI have a question about pricing.\r\n"
}

const dmarcPass = "Authentication-Results: mx.in.test; spf=pass smtp.mailfrom=company.test; dkim=pass header.d=company.test; dmarc=pass header.from=company.test\r\n"

func (w *world) trustForms(m *mailboxRow, list ...string) *mailboxRow {
	w.t.Helper()
	mustExec(w.t, w.d, `UPDATE mailboxes SET trusted_form_senders = $2 WHERE id = $1`, m.ID, list)
	out, err := loadMailbox(w.ctx, w.d.DB.Pool, w.wid, m.ID)
	if err != nil {
		w.t.Fatal(err)
	}
	return out
}

func (w *world) loadMsg(id uuid.UUID) domain.Message {
	w.t.Helper()
	msg, err := domain.LoadMessage(w.ctx, w.d.DB.Pool, w.wid, id)
	if err != nil {
		w.t.Fatal(err)
	}
	return msg
}

func TestTrustedFormSender(t *testing.T) {
	w := newWorld(t)
	m := w.trustForms(w.mailbox(InboundForwarding, OutboundSystem, nil), "website@company.test", "@forms.company.test")
	visitor := "vera-" + crypto.RandomString(5) + "@visitor.test"

	// Authenticated mail from the trusted form: the visitor is the customer.
	msg := w.loadMsg(w.messageFor(w.receive(m, formMail(dmarcPass, "website@company.test", "Vera Visitor <"+visitor+">", m.Email))).ID)
	tk := w.ticket(msg.TicketID)
	if tk.Contact == nil || tk.Contact.Email == nil || *tk.Contact.Email != visitor || tk.Contact.Name != "Vera Visitor" {
		t.Fatalf("contact %+v, want the visitor", tk.Contact)
	}
	if msg.FromEmail != visitor || msg.FromName != "Vera Visitor" || msg.FormSender != "website@company.test" || msg.ReplyTo != "" || msg.AuthFailed {
		t.Fatalf("message from %q %q form %q reply_to %q auth_failed %v", msg.FromEmail, msg.FromName, msg.FormSender, msg.ReplyTo, msg.AuthFailed)
	}
	if pub := domain.PublicMessage(msg); pub.FormSender != "" {
		t.Fatal("the form address reached the public view")
	}
	// Agent replies go to the visitor.
	var raw []byte
	var rcpts []string
	w.s.rawSend = func(_ context.Context, _ string, to []string, b []byte) error { raw, rcpts = b, to; return nil }
	if err := w.s.sendMessage(w.ctx, w.wid, w.agentReply(tk.ID, "Here are our prices.", ""), false); err != nil {
		t.Fatal(err)
	}
	env, err := enmime.ReadEnvelope(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(rcpts) != 1 || rcpts[0] != visitor || !strings.Contains(env.GetHeader("To"), visitor) {
		t.Fatalf("reply went to %v (To %q), want the visitor", rcpts, env.GetHeader("To"))
	}

	// A domain entry matches that domain only.
	other := "omar-" + crypto.RandomString(5) + "@visitor.test"
	if got := w.messageFor(w.receive(m, formMail(dmarcPass, "noreply@forms.company.test", other, m.Email))); got.FromEmail != other {
		t.Fatalf("@domain entry: from %q", got.FromEmail)
	}
	if got := w.messageFor(w.receive(m, formMail(dmarcPass, "noreply@x.forms.company.test", other, m.Email))); got.FromEmail != "noreply@x.forms.company.test" {
		t.Fatalf("subdomain trusted: from %q", got.FromEmail)
	}
}

func TestFormSenderNeedsAuthentication(t *testing.T) {
	w := newWorld(t)
	m := w.trustForms(w.mailbox(InboundForwarding, OutboundSystem, nil), "website@company.test")
	visitor := "vera-" + crypto.RandomString(5) + "@visitor.test"

	for name, auth := range map[string]string{
		"no results":       "",
		"dmarc none":       "Authentication-Results: mx.in.test; spf=pass smtp.mailfrom=company.test; dmarc=none\r\n",
		"untrusted server": "Authentication-Results: spoofed.example; dmarc=pass header.from=company.test\r\n",
		"dmarc fail":       "Authentication-Results: mx.in.test; dmarc=fail header.from=company.test\r\n",
	} {
		msg := w.loadMsg(w.messageFor(w.receive(m, formMail(auth, "website@company.test", visitor, m.Email))).ID)
		tk := w.ticket(msg.TicketID)
		if msg.FromEmail != "website@company.test" || tk.Contact == nil || *tk.Contact.Email != "website@company.test" {
			t.Errorf("%s: the visitor became the contact (%q)", name, msg.FromEmail)
		}
		if msg.FormSender != "" || msg.ReplyTo != visitor {
			t.Errorf("%s: form %q reply_to %q", name, msg.FormSender, msg.ReplyTo)
		}
	}

	// A Reply-To naming one of the workspace's own addresses never becomes the contact.
	msg := w.messageFor(w.receive(m, formMail(dmarcPass, "website@company.test", m.Email, m.Email)))
	if msg.FromEmail != "website@company.test" {
		t.Fatalf("own address as the visitor: from %q", msg.FromEmail)
	}
}

func TestUntrustedSenderKeepsFromAsContact(t *testing.T) {
	w := newWorld(t)
	m := w.trustForms(w.mailbox(InboundForwarding, OutboundSystem, nil), "website@company.test")
	visitor := "vera-" + crypto.RandomString(5) + "@visitor.test"
	msg := w.loadMsg(w.messageFor(w.receive(m, formMail(dmarcPass, "shop@company.test", visitor, m.Email))).ID)
	if msg.FromEmail != "shop@company.test" || msg.FormSender != "" || msg.ReplyTo != visitor {
		t.Fatalf("untrusted sender: from %q form %q reply_to %q", msg.FromEmail, msg.FormSender, msg.ReplyTo)
	}
	// Replies go to From: the Reply-To is at another domain.
	var rcpts []string
	w.s.rawSend = func(_ context.Context, _ string, to []string, _ []byte) error { rcpts = to; return nil }
	if err := w.s.sendMessage(w.ctx, w.wid, w.agentReply(msg.TicketID, "Hello", ""), false); err != nil {
		t.Fatal(err)
	}
	if len(rcpts) != 1 || rcpts[0] != "shop@company.test" {
		t.Fatalf("reply went to %v", rcpts)
	}
}

func TestCleanFormSenders(t *testing.T) {
	got, err := cleanFormSenders([]string{" Website@Company.test ", "@Forms.Company.test", "website@company.test"}, []string{"support@acme.test"})
	if err != nil || len(got) != 2 || got[0] != "website@company.test" || got[1] != "@forms.company.test" {
		t.Fatalf("%v %v", got, err)
	}
	if got, err := cleanFormSenders(nil, nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
	many := make([]string, 51)
	for i := range many {
		many[i] = "f" + itoa(int64(i)) + "@company.test"
	}
	for name, list := range map[string][]string{
		"too many":        many,
		"not an address":  {"website"},
		"bad domain":      {"@company"},
		"domain and user": {"@a@company.test"},
		"public provider": {"@gmail.com"},
		"own address":     {"SUPPORT@acme.test"},
	} {
		if _, err := cleanFormSenders(list, []string{"support@acme.test"}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// An exact address at a public provider is fine.
	if _, err := cleanFormSenders([]string{"myshop@gmail.com"}, nil); err != nil {
		t.Fatal(err)
	}
	if !formSenderTrusted([]string{"@company.test"}, "A@Company.test") || formSenderTrusted([]string{"@company.test"}, "a@sub.company.test") ||
		formSenderTrusted([]string{"website@company.test"}, "other@company.test") || formSenderTrusted(nil, "") {
		t.Fatal("formSenderTrusted")
	}
}

func TestTrustedFormSendersAPI(t *testing.T) {
	w := newWorld(t)
	m := w.mailbox(InboundForwarding, OutboundSystem, nil)
	srv := w.api(w.admin)
	var out Mailbox
	if code := doJSON(t, http.MethodPatch, srv.URL+"/v1/mailboxes/"+m.ID.String(),
		map[string]any{"trusted_form_senders": []string{"Website@Company.test", "@forms.company.test"}}, &out); code != 200 {
		t.Fatalf("patch: %d", code)
	}
	if len(out.TrustedFormSenders) != 2 || out.TrustedFormSenders[0] != "website@company.test" {
		t.Fatalf("trusted %v", out.TrustedFormSenders)
	}
	if n := w.count(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'mailbox.updated' AND data->'changes' ? 'trusted_form_senders'`, w.wid); n != 1 {
		t.Fatalf("audit %d", n)
	}
	for _, bad := range [][]string{{"@gmail.com"}, {"nope"}, {strings.ToUpper(m.Email)}} {
		if code := doJSON(t, http.MethodPatch, srv.URL+"/v1/mailboxes/"+m.ID.String(), map[string]any{"trusted_form_senders": bad}, nil); code != 422 {
			t.Errorf("%v: %d, want 422", bad, code)
		}
	}
	// Agents read the mailbox without the list, and cannot change it.
	agentID := ids.New()
	mustExec(t, w.d, `INSERT INTO users (id, email, name) VALUES ($1, $2, 'Andy Agent')`, agentID, "andy-"+crypto.RandomString(6)+"@acme.test")
	mustExec(t, w.d, `INSERT INTO members (workspace_id, user_id, role) VALUES ($1, $2, 'agent')`, w.wid, agentID)
	agent := w.api(&auth.Principal{Kind: auth.KindUser, UserID: agentID, SessionID: ids.New(), WorkspaceID: w.wid, Role: auth.RoleAgent, IP: "127.0.0.1"})
	var seen Mailbox
	if code := doJSON(t, http.MethodGet, agent.URL+"/v1/mailboxes/"+m.ID.String(), nil, &seen); code != 200 || seen.TrustedFormSenders == nil || len(seen.TrustedFormSenders) != 0 {
		t.Fatalf("agent read: %d %v", code, seen.TrustedFormSenders)
	}
	if code := doJSON(t, http.MethodPatch, agent.URL+"/v1/mailboxes/"+m.ID.String(), map[string]any{"trusted_form_senders": []string{}}, nil); code != http.StatusForbidden {
		t.Fatalf("agent patch: %d", code)
	}
	// Clearing.
	if code := doJSON(t, http.MethodPatch, srv.URL+"/v1/mailboxes/"+m.ID.String(), map[string]any{"trusted_form_senders": []string{}}, &out); code != 200 || len(out.TrustedFormSenders) != 0 {
		t.Fatalf("clear: %d %v", code, out.TrustedFormSenders)
	}
}

// A reply that failed sender authentication keeps the ticket where it was, without
// reopening it first, and its message carries the verdict.
func TestUnverifiedReplyKeepsStatus(t *testing.T) {
	w := newWorld(t)
	m := w.mailbox(InboundForwarding, OutboundSystem, nil)
	from := customer()
	tk, ours := firstTicket(t, w, m, from)
	mustExec(t, w.d, `UPDATE tickets SET status = 'on_hold', snoozed_until = now() + interval '2 days' WHERE id = $1`, tk.ID)
	updates := w.count(`SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'ticket.updated' AND data->'changes' ? 'status'`, w.wid)

	raw := "Authentication-Results: mx.in.test; dmarc=fail header.from=customer.test\r\n" +
		"From: " + from + "\r\nReply-To: billing@customer.test\r\nTo: " + m.Email + "\r\nSubject: Re: hello\r\nIn-Reply-To: " + ours +
		"\r\nMessage-ID: <df" + crypto.RandomString(8) + "@spoof.test>\r\n\r\nPlease update our bank details.\r\n"
	msg := w.loadMsg(w.messageFor(w.receive(m, raw)).ID)
	if !msg.AuthFailed || msg.ReplyTo != "billing@customer.test" {
		t.Fatalf("auth_failed %v reply_to %q", msg.AuthFailed, msg.ReplyTo)
	}
	got := w.ticket(tk.ID)
	if got.Status != domain.StatusOnHold || got.SnoozedUntil == nil {
		t.Fatalf("status %s snoozed %v", got.Status, got.SnoozedUntil)
	}
	if n := w.count(`SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'ticket.updated' AND data->'changes' ? 'status'`, w.wid); n != updates {
		t.Fatal("the status changed and changed back")
	}
	if n := w.count(`SELECT count(*) FROM activities WHERE ticket_id = $1 AND type = 'status_changed'
		AND data->>'reason' = 'sender_unverified' AND data->>'from' = 'on_hold' AND data->>'to' = 'on_hold' AND data->>'message_id' = $2`, tk.ID, msg.ID.String()); n != 1 {
		t.Fatalf("kept-status activity %d", n)
	}
	if n := w.count(`SELECT count(*) FROM activities WHERE ticket_id = $1 AND type = 'status_changed' AND data->>'to' = 'open'`, tk.ID); n != 0 {
		t.Fatal("the ticket was reopened")
	}
	// The event carries the verdict for agents.
	if n := w.count(`SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'message.created'
		AND data->'message'->>'id' = $2 AND (data->'message'->>'auth_failed')::boolean
		AND data->'message'->>'reply_to' = 'billing@customer.test'`, w.wid, msg.ID.String()); n != 1 {
		t.Fatal("message.created without auth_failed and reply_to")
	}
	if pub := domain.PublicMessage(msg); pub.AuthFailed || pub.ReplyTo != "" {
		t.Fatal("the public view carries sender checks")
	}

	// A verified reply still reopens.
	mustExec(t, w.d, `UPDATE tickets SET status = 'solved', solved_at = now() WHERE id = $1`, tk.ID)
	w.receive(m, refMail(from, m.Email, "Re: hello", ours))
	if got := w.ticket(tk.ID); got.Status != domain.StatusOpen {
		t.Fatalf("verified reply left status %s", got.Status)
	}
}

// Addresses picked for a ticket with no reply address are never another private team's.
func TestOutboundNeverFromAnotherPrivateTeamsAddress(t *testing.T) {
	w := newWorld(t)
	secret := w.team("Legal", true, "")
	sales := w.team("Sales", false, "")
	legalBox := w.mailbox(InboundForwarding, OutboundSystem, func(m *mailboxRow, _ *credentials) { m.TeamID = &secret })
	publicBox := w.mailbox(InboundForwarding, OutboundSystem, nil)
	var raw []byte
	w.s.rawSend = func(_ context.Context, _ string, _ []string, b []byte) error { raw = b; return nil }
	contact := ids.New()
	mustExec(t, w.d, `INSERT INTO contacts (id, workspace_id, email, name) VALUES ($1, $2, $3, 'Carla')`, contact, w.wid, customer())

	// Email: a Sales ticket (no Sales address) goes out from the public address, though
	// Legal's is older.
	tid := ids.New()
	mustExec(t, w.d, `INSERT INTO tickets (id, workspace_id, number, subject, channel, contact_id, team_id, brand_id)
		VALUES ($1, $2, 11, 'API ticket', 'email', $3, $4, $5)`, tid, w.wid, contact, sales, w.brand)
	if err := w.s.sendMessage(w.ctx, w.wid, w.agentReply(tid, "Hello", ""), false); err != nil {
		t.Fatal(err)
	}
	if raw == nil {
		t.Fatal("not sent")
	}
	if tk := w.ticket(tid); tk.MailboxID == nil || *tk.MailboxID != publicBox.ID {
		t.Fatalf("ticket took %v, want the public address %v (not %v)", tk.MailboxID, publicBox.ID, legalBox.ID)
	}

	// Chat follow-up: the same pick, and Legal's own tickets keep Legal's address.
	for _, c := range []struct {
		team *uuid.UUID
		want uuid.UUID
	}{{nil, publicBox.ID}, {&sales, publicBox.ID}, {&secret, legalBox.ID}} {
		got, err := followUpMailbox(w.ctx, w.d.DB.Pool, w.wid, nil, w.brand, c.team)
		if err != nil || got == nil || *got != c.want {
			t.Errorf("team %v: follow-up from %v, want %v (%v)", c.team, got, c.want, err)
		}
	}

	// With only a private team's address, a ticket of another team has nothing to send from.
	mustExec(t, w.d, `UPDATE mailboxes SET status = 'disabled' WHERE id = $1`, publicBox.ID)
	if got, err := followUpMailbox(w.ctx, w.d.DB.Pool, w.wid, nil, w.brand, &sales); err != nil || got != nil {
		t.Fatalf("follow-up from %v (%v), want none", got, err)
	}
	mustExec(t, w.d, `DELETE FROM mailboxes WHERE id = $1`, publicBox.ID)
	tid2 := ids.New()
	mustExec(t, w.d, `INSERT INTO tickets (id, workspace_id, number, subject, channel, contact_id, team_id, brand_id)
		VALUES ($1, $2, 12, 'API ticket', 'email', $3, $4, $5)`, tid2, w.wid, contact, sales, w.brand)
	raw = nil
	_ = w.s.sendMessage(w.ctx, w.wid, w.agentReply(tid2, "Hello", ""), false)
	if raw != nil {
		t.Fatal("sent from another private team's address")
	}
}

func TestFormMailStillGetsAutomaticAnswers(t *testing.T) {
	w := newWorld(t)
	m := w.trustForms(w.mailbox(InboundForwarding, OutboundSystem, nil), "website@company.test")
	noAuto := func(raw string) bool {
		t.Helper()
		var v bool
		if inbound := w.receive(m, raw); func() bool { st, _ := w.inbound(inbound); return st == "ignored" }() {
			return true
		} else if err := w.d.DB.Pool.QueryRow(w.ctx, `SELECT no_auto_reply FROM messages WHERE inbound_email_id = $1`, inbound).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	visitor := func() string { return "v-" + crypto.RandomString(6) + "@visitor.test" }

	// Form plugins mark their mail as generated; the visitor still gets the answer.
	if noAuto(formMail(dmarcPass+"Auto-Submitted: auto-generated\r\n", "website@company.test", visitor(), m.Email)) {
		t.Fatal("a trusted form's generated mail was treated as automatic")
	}
	// An autoresponder or a loop marker still stops it.
	if !noAuto(formMail(dmarcPass+"Auto-Submitted: auto-replied\r\n", "website@company.test", visitor(), m.Email)) {
		t.Fatal("an auto-reply through the form was answered")
	}
	if !noAuto(formMail(dmarcPass+"X-Loop: someone-else\r\n", "website@company.test", visitor(), m.Email)) {
		t.Fatal("looped form mail was answered")
	}
	// Generated mail from a sender that isn't a trusted form stays automatic.
	if !noAuto(formMail(dmarcPass+"Auto-Submitted: auto-generated\r\n", "alerts@company.test", visitor(), m.Email)) {
		t.Fatal("untrusted generated mail was answered")
	}
}
