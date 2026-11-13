//go:build integration

package whatsapp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/helpdesk/resources"
	"github.com/useopendesk/opendesk/internal/helpdesk/tickets"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/whatsapp"
	"github.com/useopendesk/opendesk/internal/whatsapp/provider/twilio"
)

const (
	sid   = "AC00000000000000000000000000000000"
	token = "auth-token"
)

// fakeTwilio is the part of Twilio the engine talks to.
type fakeTwilio struct {
	mu    sync.Mutex
	sent  []url.Values
	fail  string // a Twilio error code the next send answers with
	count int
}

func (f *fakeTwilio) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != sid || p != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Accounts/"+sid+".json":
			_, _ = w.Write([]byte(`{"friendly_name":"Margin Support","status":"active"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/Accounts/"+sid+"/Messages.json":
			_ = r.ParseForm()
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.fail != "" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"code":` + f.fail + `,"message":"x"}`))
				f.fail = ""
				return
			}
			f.sent = append(f.sent, r.PostForm)
			f.count++
			_, _ = w.Write([]byte(`{"sid":"SMout` + string(rune('0'+f.count)) + `","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/ContentAndApprovals":
			_, _ = w.Write([]byte(`{"contents":[
				{"sid":"HX1","friendly_name":"follow_up","language":"en","types":{"twilio/text":{"body":"Hi {{1}}, did our answer about {{2}} help?"}},"approval_requests":{"whatsapp":{"status":"approved"}}},
				{"sid":"HX2","friendly_name":"pending_one","language":"en","types":{"twilio/text":{"body":"Not approved"}},"approval_requests":{"whatsapp":{"status":"pending"}}}]}`))
		default:
			t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

type harness struct {
	e    *hdtest.Env
	ws   hdtest.WS
	s    *whatsapp.Service
	f    *fakeTwilio
	conn whatsapp.WhatsAppConnectionView
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	f := &fakeTwilio{}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	var s *whatsapp.Service
	e := hdtest.New(t, tickets.Register, resources.Register, func(d *deps.Deps, api huma.API, r chi.Router) {
		s = whatsapp.New(d, twilio.Adapter{APIURL: srv.URL, ContentURL: srv.URL})
		whatsapp.Install(d, s)
		s.Register(api, r)
	})
	ws := e.NewWorkspace()
	h := &harness{e: e, ws: ws, s: s, f: f}
	e.Do("POST", "/v1/whatsapp/connections", ws.Admin.As(ws), map[string]any{"provider": "twilio", "sender": "+1 (415) 555-0100",
		"credentials": map[string]string{"account_sid": sid, "auth_token": token}}).Want(t, 201).JSON(t, &h.conn)
	return h
}

func (h *harness) path(t *testing.T, status bool) (full, path string) {
	t.Helper()
	full = h.conn.WebhookURL
	if status {
		full = h.conn.StatusURL
	}
	u, err := url.Parse(full)
	if err != nil {
		t.Fatal(err)
	}
	return full, u.Path
}

// post sends a form to the connection's webhook, signed as Twilio signs it unless sig is set.
func (h *harness) post(t *testing.T, status bool, form url.Values, sig string) int {
	t.Helper()
	full, path := h.path(t, status)
	if sig == "" {
		sig = twilio.Signature(token, full, form)
	}
	req, _ := http.NewRequest("POST", h.e.Srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if sig != "-" {
		req.Header.Set("X-Twilio-Signature", sig)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func (h *harness) customer(t *testing.T, id, from, body string) {
	t.Helper()
	form := url.Values{"MessageSid": {id}, "From": {"whatsapp:" + from}, "To": {"whatsapp:+14155550100"}, "Body": {body}, "NumMedia": {"0"}, "ProfileName": {"Ana Souza"}}
	if got := h.post(t, false, form, ""); got != 200 {
		t.Fatalf("webhook answered %d", got)
	}
	h.drain(t)
}

func (h *harness) status(t *testing.T, id, status, code string) {
	t.Helper()
	form := url.Values{"MessageSid": {id}, "MessageStatus": {status}, "To": {"whatsapp:+4915123456789"}}
	if code != "" {
		form.Set("ErrorCode", code)
	}
	if got := h.post(t, true, form, ""); got != 200 {
		t.Fatalf("status webhook answered %d", got)
	}
	h.drain(t)
}

// drain runs the queued inbound events, sends and ticket triggers until none are left.
func (h *harness) drain(t *testing.T) {
	t.Helper()
	for i := 0; i < 20; i++ {
		n := 0
		for _, raw := range h.e.Jobs(jobs.WhatsAppInboundArgs{}.Kind(), h.ws.ID) {
			var a jobs.WhatsAppInboundArgs
			_ = json.Unmarshal(raw, &a)
			if err := h.s.ProcessInbound(h.e.Ctx, a); err != nil {
				t.Fatalf("inbound: %v", err)
			}
			n++
		}
		for _, raw := range h.e.Jobs(jobs.WhatsAppSendArgs{}.Kind(), h.ws.ID) {
			var a jobs.WhatsAppSendArgs
			_ = json.Unmarshal(raw, &a)
			if err := h.s.Send(h.e.Ctx, a); err != nil {
				t.Fatalf("send: %v", err)
			}
			n++
		}
		for _, raw := range h.e.Jobs(jobs.TicketTriggerArgs{}.Kind(), h.ws.ID) {
			var a jobs.TicketTriggerArgs
			_ = json.Unmarshal(raw, &a)
			if err := tickets.RunTrigger(h.e.Ctx, h.e.D, a); err != nil {
				t.Fatalf("trigger %s: %v", a.Trigger, err)
			}
			n++
		}
		if n == 0 {
			return
		}
	}
	t.Fatal("jobs did not settle")
}

func (h *harness) ticketOf(t *testing.T, number string) domain.Ticket {
	t.Helper()
	var id uuid.UUID
	h.e.Scalar(`SELECT ticket_id FROM whatsapp_messages WHERE workspace_id = $1 AND number = $2 ORDER BY created_at DESC LIMIT 1`, &id, h.ws.ID, number)
	var tk domain.Ticket
	h.e.Do("GET", "/v1/tickets/"+id.String(), h.ws.Agent.As(h.ws), nil).Want(t, 200).JSON(t, &tk)
	return tk
}

func (h *harness) messages(t *testing.T, tid uuid.UUID) []domain.Message {
	t.Helper()
	var out struct {
		Data []domain.Message `json:"data"`
	}
	h.e.Do("GET", "/v1/tickets/"+tid.String()+"/messages", h.ws.Agent.As(h.ws), nil).Want(t, 200).JSON(t, &out)
	return out.Data
}

func (h *harness) reply(t *testing.T, tid uuid.UUID, body map[string]any) *hdtest.Resp {
	t.Helper()
	body["kind"] = "reply"
	return h.e.Do("POST", "/v1/tickets/"+tid.String()+"/messages", h.ws.Agent.As(h.ws), body)
}

func TestConnect(t *testing.T) {
	h := newHarness(t)
	c := h.conn
	if c.Provider != "twilio" || c.Sender != "+14155550100" || c.AccountName != "Margin Support" || c.HealthStatus != "action_required" || c.HealthCode != whatsapp.HealthNoWebhook {
		t.Fatalf("connection %+v", c)
	}
	if !strings.HasSuffix(c.WebhookURL, "/v1/whatsapp/hooks/twilio/"+c.ID.String()) || !strings.HasSuffix(c.StatusURL, "/status") {
		t.Fatalf("urls %q %q", c.WebhookURL, c.StatusURL)
	}
	body := string(h.e.Do("GET", "/v1/whatsapp/connections", h.ws.Agent.As(h.ws), nil).Want(t, 200).Body)
	if strings.Contains(body, "/v1/whatsapp/hooks/") || strings.Contains(body, token) {
		t.Fatal("an agent reads the webhook or the credentials")
	}
	if strings.Contains(string(h.e.Do("GET", "/v1/whatsapp/connections", h.ws.Admin.As(h.ws), nil).Want(t, 200).Body), token) {
		t.Fatal("the credentials are read back")
	}
	creds := map[string]string{"account_sid": sid, "auth_token": "wrong"}
	h.e.Do("POST", "/v1/whatsapp/connections", h.ws.Admin.As(h.ws), map[string]any{"provider": "twilio", "sender": "+4915123456789", "credentials": creds}).Want(t, 422)
	good := map[string]string{"account_sid": sid, "auth_token": token}
	h.e.Do("POST", "/v1/whatsapp/connections", h.ws.Admin.As(h.ws), map[string]any{"provider": "twilio", "sender": "0151 23456", "credentials": good}).Want(t, 422)
	h.e.Do("POST", "/v1/whatsapp/connections", h.ws.Admin.As(h.ws), map[string]any{"provider": "twilio", "sender": "+14155550100", "credentials": good}).Want(t, 409)
	h.e.Do("POST", "/v1/whatsapp/connections", h.ws.Agent.As(h.ws), map[string]any{"provider": "twilio", "sender": "+4915123456789", "credentials": good}).Want(t, 403)
	h.e.Do("POST", "/v1/whatsapp/connections/"+c.ID.String()+"/test", h.ws.Admin.As(h.ws), nil).Want(t, 200).JSON(t, &c)
	if c.HealthCode != whatsapp.HealthNoWebhook {
		t.Fatalf("health after test %+v", c)
	}
}

func TestWebhookRefusals(t *testing.T) {
	h := newHarness(t)
	form := url.Values{"MessageSid": {"SMin1"}, "From": {"whatsapp:+4915123456789"}, "Body": {"hello"}}
	for name, sig := range map[string]string{"unsigned": "-", "wrong": "AAAA", "another token": twilio.Signature("other", h.conn.WebhookURL, form)} {
		if got := h.post(t, false, form, sig); got != 401 {
			t.Errorf("%s: %d", name, got)
		}
	}
	// Signed for the other URL: a status signature is no message signature.
	if got := h.post(t, false, form, twilio.Signature(token, h.conn.StatusURL, form)); got != 401 {
		t.Errorf("signed for the status URL: %d", got)
	}
	var n int
	h.e.Scalar(`SELECT count(*) FROM whatsapp_inbound_events WHERE workspace_id = $1`, &n, h.ws.ID)
	if n != 0 {
		t.Fatalf("%d events stored from refused requests", n)
	}
	req, _ := http.NewRequest("POST", h.e.Srv.URL+"/v1/whatsapp/hooks/twilio/"+uuid.NewString(), strings.NewReader(form.Encode()))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("unknown connection: %d", res.StatusCode)
	}
	// An SMS is not a WhatsApp message: acknowledged and dropped.
	sms := url.Values{"MessageSid": {"SMsms"}, "From": {"+4915123456789"}, "Body": {"hi"}}
	if got := h.post(t, false, sms, ""); got != 200 {
		t.Fatalf("sms: %d", got)
	}
	h.e.Scalar(`SELECT count(*) FROM whatsapp_inbound_events WHERE workspace_id = $1`, &n, h.ws.ID)
	if n != 0 {
		t.Fatal("an SMS was stored")
	}
}

func TestInboundLandsOnOneTicket(t *testing.T) {
	h := newHarness(t)
	h.customer(t, "SMin1", "+4915123456789", "Hi, my invoice is wrong\nAnd late too")
	tk := h.ticketOf(t, "+4915123456789")
	if tk.Channel != "whatsapp" || tk.Subject != "Hi, my invoice is wrong" || tk.Contact == nil || tk.Contact.Name != "Ana Souza" {
		t.Fatalf("ticket %+v", tk)
	}
	h.customer(t, "SMin2", "+4915123456789", "Still there?")
	// Twilio retries the same message: nothing is added.
	h.customer(t, "SMin2", "+4915123456789", "Still there?")
	ms := h.messages(t, tk.ID)
	if len(ms) != 2 || ms[0].Direction != "inbound" || ms[1].BodyText != "Still there?" {
		t.Fatalf("messages %+v", ms)
	}
	var n int
	h.e.Scalar(`SELECT count(*) FROM tickets WHERE workspace_id = $1 AND channel = 'whatsapp'`, &n, h.ws.ID)
	if n != 1 {
		t.Fatalf("%d tickets", n)
	}
	var health string
	h.e.Scalar(`SELECT health_status FROM whatsapp_connections WHERE id = $1`, &health, h.conn.ID)
	if health != "healthy" {
		t.Fatalf("health %q after an event", health)
	}
	// Another customer opens their own ticket; the same person solved is reused within reuse_days.
	h.customer(t, "SMin3", "+4915999999999", "Hello")
	h.e.Scalar(`SELECT count(*) FROM tickets WHERE workspace_id = $1 AND channel = 'whatsapp'`, &n, h.ws.ID)
	if n != 2 {
		t.Fatalf("%d tickets", n)
	}
	h.e.Do("PATCH", "/v1/tickets/"+tk.ID.String(), h.ws.Agent.As(h.ws), map[string]any{"status": "solved"}).Want(t, 200)
	h.customer(t, "SMin4", "+4915123456789", "One more thing")
	h.e.Scalar(`SELECT count(*) FROM tickets WHERE workspace_id = $1 AND channel = 'whatsapp'`, &n, h.ws.ID)
	if n != 2 {
		t.Fatalf("%d tickets: a solved one within reuse_days takes the message", n)
	}
}

func TestReplyAndDelivery(t *testing.T) {
	h := newHarness(t)
	h.customer(t, "SMin1", "+4915123456789", "Hi")
	tk := h.ticketOf(t, "+4915123456789")
	var st whatsapp.WhatsAppTicketState
	h.e.Do("GET", "/v1/tickets/"+tk.ID.String()+"/whatsapp", h.ws.Viewer.As(h.ws), nil).Want(t, 200).JSON(t, &st)
	if st.Connection == nil || st.Connection.Sender != "+14155550100" || !st.Window.Open || st.Window.ClosesAt == nil {
		t.Fatalf("state %+v", st)
	}
	var msg domain.Message
	h.reply(t, tk.ID, map[string]any{"body_html": "<p>Hello Ana, looking now</p>"}).Want(t, 201).JSON(t, &msg)
	if msg.DeliveryStatus != "pending" {
		t.Fatalf("queued reply is %q", msg.DeliveryStatus)
	}
	h.drain(t)
	if len(h.f.sent) != 1 || h.f.sent[0].Get("To") != "whatsapp:+4915123456789" || h.f.sent[0].Get("From") != "whatsapp:+14155550100" ||
		h.f.sent[0].Get("Body") != "Hello Ana, looking now" || h.f.sent[0].Get("StatusCallback") != h.conn.StatusURL {
		t.Fatalf("sent %+v", h.f.sent)
	}
	status := func() string {
		for _, m := range h.messages(t, tk.ID) {
			if m.ID == msg.ID {
				return m.DeliveryStatus
			}
		}
		return ""
	}
	if status() != "sent" {
		t.Fatalf("status %q", status())
	}
	// A status can beat the send's own write: it waits for it. Read arrives before delivered, which never goes back.
	h.status(t, "SMout1", "read", "")
	h.status(t, "SMout1", "delivered", "")
	h.status(t, "SMout1", "sent", "")
	if status() != "read" {
		t.Fatalf("status %q after read", status())
	}
	// A status for a message nobody stored is waited on, then dropped by the job's deadline; the event is kept.
	if err := h.s.ProcessInbound(h.e.Ctx, jobs.WhatsAppInboundArgs{WorkspaceID: h.ws.ID, ConnectionID: h.conn.ID, ExternalEventID: "none"}); err != nil {
		t.Fatalf("unknown event: %v", err)
	}
}

func TestFailures(t *testing.T) {
	h := newHarness(t)
	h.customer(t, "SMin1", "+4915123456789", "Hi")
	tk := h.ticketOf(t, "+4915123456789")
	send := func(code string) domain.Message {
		h.f.fail = code
		var msg domain.Message
		h.reply(t, tk.ID, map[string]any{"body_html": "<p>Hello</p>"}).Want(t, 201).JSON(t, &msg)
		h.drain(t)
		for _, m := range h.messages(t, tk.ID) {
			if m.ID == msg.ID {
				return m
			}
		}
		t.Fatal("message gone")
		return msg
	}
	if m := send("63003"); m.DeliveryStatus != "failed" || m.DeliveryError != "This number is not on WhatsApp." {
		t.Fatalf("not on WhatsApp: %+v", m)
	}
	if m := send("63016"); m.DeliveryStatus != "failed" || !strings.Contains(m.DeliveryError, "24-hour") {
		t.Fatalf("outside the window at the provider: %+v", m)
	}
	// A delivery failure the provider reports later.
	m := send("")
	h.status(t, "SMout1", "failed", "63024")
	for _, x := range h.messages(t, tk.ID) {
		if x.ID == m.ID && (x.DeliveryStatus != "failed" || x.DeliveryError != "This number is not on WhatsApp.") {
			t.Fatalf("reported failure: %+v", x)
		}
	}
}

func TestWindowAndTemplates(t *testing.T) {
	h := newHarness(t)
	h.customer(t, "SMin1", "+4915123456789", "Hi")
	tk := h.ticketOf(t, "+4915123456789")
	h.e.Exec(`UPDATE whatsapp_messages SET created_at = now() - interval '25 hours' WHERE ticket_id = $1`, tk.ID)
	var st whatsapp.WhatsAppTicketState
	h.e.Do("GET", "/v1/tickets/"+tk.ID.String()+"/whatsapp", h.ws.Agent.As(h.ws), nil).Want(t, 200).JSON(t, &st)
	if st.Window.Open || st.Connection == nil {
		t.Fatalf("state %+v", st)
	}
	res := h.reply(t, tk.ID, map[string]any{"body_html": "<p>Hello</p>"}).Want(t, 422)
	if !strings.Contains(string(res.Body), "whatsapp_window_closed") {
		t.Fatalf("answer %s", res.Body)
	}
	var list struct {
		Data []struct{ ID, Name string }
	}
	h.e.Do("GET", "/v1/tickets/"+tk.ID.String()+"/whatsapp/templates", h.ws.Agent.As(h.ws), nil).Want(t, 200).JSON(t, &list)
	if len(list.Data) != 1 || list.Data[0].ID != "HX1" {
		t.Fatalf("templates %+v (only an approved one is offered)", list.Data)
	}
	h.e.Do("GET", "/v1/tickets/"+tk.ID.String()+"/whatsapp/templates", h.ws.Viewer.As(h.ws), nil).Want(t, 403)
	tpl := func(id string, vars map[string]string) map[string]any {
		return map[string]any{"body_html": "<p>x</p>", "whatsapp_template": map[string]any{"id": id, "variables": vars}}
	}
	h.reply(t, tk.ID, tpl("HX2", map[string]string{})).Want(t, 422)
	h.reply(t, tk.ID, tpl("HX1", map[string]string{"1": "Ana"})).Want(t, 422)
	var msg domain.Message
	h.reply(t, tk.ID, tpl("HX1", map[string]string{"1": "Ana", "2": "your invoice"})).Want(t, 201).JSON(t, &msg)
	if msg.BodyText != "Hi Ana, did our answer about your invoice help?" {
		t.Fatalf("body %q", msg.BodyText)
	}
	h.drain(t)
	if len(h.f.sent) != 1 || h.f.sent[0].Get("ContentSid") != "HX1" || h.f.sent[0].Get("Body") != "" || !strings.Contains(h.f.sent[0].Get("ContentVariables"), `"2":"your invoice"`) {
		t.Fatalf("sent %+v", h.f.sent)
	}
	// An attachment is not sent yet.
	h.e.Exec(`UPDATE whatsapp_messages SET created_at = now() WHERE ticket_id = $1 AND direction = 'inbound'`, tk.ID)
	att := uuid.NewString()
	h.e.Exec(`INSERT INTO attachments (id, workspace_id, filename, content_type, size_bytes, storage_key, uploaded_by) VALUES ($1, $2, 'a.txt', 'text/plain', 5, $3, $4)`,
		att, h.ws.ID, "workspaces/"+h.ws.ID.String()+"/attachments/"+att, h.ws.Agent.ID)
	h.reply(t, tk.ID, map[string]any{"body_html": "<p>See</p>", "attachment_ids": []string{att}}).Want(t, 422)
}

func TestBlockedAndOtherWorkspace(t *testing.T) {
	h := newHarness(t)
	h.customer(t, "SMin1", "+4915123456789", "Hi")
	h.e.Exec(`UPDATE contacts SET blocked = true WHERE workspace_id = $1 AND phone = '+4915123456789'`, h.ws.ID)
	h.customer(t, "SMin2", "+4915123456789", "Hello again")
	var n int
	h.e.Scalar(`SELECT count(*) FROM whatsapp_messages WHERE workspace_id = $1`, &n, h.ws.ID)
	if n != 1 {
		t.Fatalf("a blocked contact's message was stored (%d rows)", n)
	}
	other := h.e.NewWorkspace()
	tk := h.ticketOf(t, "+4915123456789")
	h.e.Do("GET", "/v1/tickets/"+tk.ID.String()+"/whatsapp", other.Admin.As(other), nil).Want(t, 404)
	h.e.Do("DELETE", "/v1/whatsapp/connections/"+h.conn.ID.String(), other.Admin.As(other), nil).Want(t, 404)
	h.e.Do("DELETE", "/v1/whatsapp/connections/"+h.conn.ID.String(), h.ws.Admin.As(h.ws), nil).Want(t, 204)
	h.e.Do("GET", "/v1/tickets/"+tk.ID.String()+"/whatsapp", h.ws.Agent.As(h.ws), nil).Want(t, 200)
	if r := h.reply(t, tk.ID, map[string]any{"body_html": "<p>x</p>"}).Want(t, 422); !strings.Contains(string(r.Body), "whatsapp_disconnected") {
		t.Fatalf("reply after disconnect: %s", r.Body)
	}
}
