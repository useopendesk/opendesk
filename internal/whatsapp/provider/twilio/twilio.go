// Package twilio is the Twilio adapter of the WhatsApp engine (contracts/whatsapp.md). It
// speaks the Programmable Messaging API (https://www.twilio.com/docs/whatsapp/api) with an
// Account SID and an Auth Token: a customer's message and its delivery status arrive as
// form posts signed with the token (X-Twilio-Signature), replies go out through the
// Messages resource, and approved templates come from the Content API.
package twilio

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" // #nosec G505 -- Twilio signs webhooks with HMAC-SHA1
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/useopendesk/opendesk/internal/whatsapp/provider"
)

const (
	// API and Content are Twilio's; tests point them elsewhere, users never do.
	API     = "https://api.twilio.com/2010-04-01"
	Content = "https://content.twilio.com/v1"

	maxBody      = 1 << 20
	maxText      = 4096
	maxMedia     = 10
	maxTemplates = 200
)

// Adapter is Twilio. The URLs are set by tests only.
type Adapter struct{ APIURL, ContentURL string }

func (a Adapter) api() string {
	if a.APIURL != "" {
		return strings.TrimRight(a.APIURL, "/")
	}
	return API
}

func (a Adapter) content() string {
	if a.ContentURL != "" {
		return strings.TrimRight(a.ContentURL, "/")
	}
	return Content
}

func (Adapter) Describe() provider.Descriptor {
	return provider.Descriptor{Key: "twilio", Name: "Twilio",
		Fields: []provider.Field{
			{Name: "account_sid", Label: "Account SID", Secret: false},
			{Name: "auth_token", Label: "Auth token", Secret: true},
		},
		Docs: "https://www.twilio.com/docs/whatsapp/self-sign-up"}
}

var sidRe = regexp.MustCompile(`^AC[0-9a-fA-F]{32}$`)

func (Adapter) creds(c map[string]string) (sid, token string, err error) {
	sid, token = strings.TrimSpace(c["account_sid"]), strings.TrimSpace(c["auth_token"])
	if !sidRe.MatchString(sid) || token == "" {
		return "", "", &provider.StatusError{Op: "credentials", Status: http.StatusUnauthorized}
	}
	return sid, token, nil
}

func (a Adapter) do(ctx context.Context, hc *http.Client, op, method, u string, c map[string]string, form url.Values) (*http.Response, error) {
	sid, token, err := a.creds(c)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(sid, token)
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return resp, nil
}

// failure is the error a Twilio answer without a success status becomes.
func failure(op string, resp *http.Response, raw []byte) error {
	if resp.StatusCode == http.StatusTooManyRequests {
		wait := 30 * time.Second
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
			wait = time.Duration(s) * time.Second
		}
		return &provider.RateLimited{RetryAfter: wait}
	}
	var e struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(raw, &e)
	code := ""
	if e.Code != 0 {
		code = strconv.Itoa(e.Code)
	}
	return &provider.StatusError{Op: op, Status: resp.StatusCode, Code: code}
}

func (a Adapter) Validate(ctx context.Context, hc *http.Client, c map[string]string) (provider.Account, error) {
	sid, _, err := a.creds(c)
	if err != nil {
		return provider.Account{}, err
	}
	resp, err := a.do(ctx, hc, "account", http.MethodGet, a.api()+"/Accounts/"+sid+".json", c, nil)
	if err != nil {
		return provider.Account{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if resp.StatusCode != http.StatusOK {
		return provider.Account{}, failure("account", resp, raw)
	}
	var acct struct {
		Name   string `json:"friendly_name"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &acct); err != nil {
		return provider.Account{}, fmt.Errorf("account: %w", err)
	}
	if acct.Status != "" && acct.Status != "active" {
		return provider.Account{}, &provider.StatusError{Op: "account", Status: http.StatusForbidden}
	}
	return provider.Account{Name: acct.Name}, nil
}

// Signature is what Twilio sends in X-Twilio-Signature: the base64 HMAC-SHA1 of the URL
// it called followed by each form key and value, keys in order, under the auth token.
func Signature(token, rawURL string, form url.Values) string {
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(rawURL)
	for _, k := range keys {
		vals := append([]string(nil), form[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			b.WriteString(k)
			b.WriteString(v)
		}
	}
	m := hmac.New(sha1.New, []byte(token))
	m.Write([]byte(b.String()))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

func (a Adapter) Verify(r provider.Request, c map[string]string) error {
	_, token, err := a.creds(c)
	if err != nil {
		return provider.ErrUnauthorized
	}
	got := r.Header.Get("X-Twilio-Signature")
	if got == "" || !hmac.Equal([]byte(got), []byte(Signature(token, r.URL, r.Form))) {
		return provider.ErrUnauthorized
	}
	return nil
}

const waPrefix = "whatsapp:"

// number reads "whatsapp:+4915..." as "+4915...". Anything else (an SMS, a messenger id)
// is not a WhatsApp number.
func number(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, waPrefix) {
		return ""
	}
	n := strings.TrimPrefix(s, waPrefix)
	if len(n) < 7 || len(n) > 16 || n[0] != '+' || strings.Trim(n[1:], "0123456789") != "" {
		return ""
	}
	return n
}

func (Adapter) ParseMessage(f url.Values) (provider.Event, error) {
	sid, from := strings.TrimSpace(f.Get("MessageSid")), number(f.Get("From"))
	if sid == "" || from == "" {
		return provider.Event{}, provider.ErrIgnored
	}
	ev := provider.Event{ID: sid, Kind: provider.KindMessage, ExternalID: sid, Number: from, Name: strings.TrimSpace(f.Get("ProfileName")),
		Text: truncate(strings.TrimSpace(f.Get("Body")), maxText), At: time.Now().UTC()}
	n, _ := strconv.Atoi(f.Get("NumMedia"))
	for i := 0; i < n && i < maxMedia; i++ {
		u := f.Get("MediaUrl" + strconv.Itoa(i))
		if u != "" {
			ev.Media = append(ev.Media, provider.Media{URL: u, ContentType: f.Get("MediaContentType" + strconv.Itoa(i))})
		}
	}
	if ev.Text == "" && len(ev.Media) == 0 {
		// A location, a contact card, a reaction: nothing a ticket can show yet.
		return provider.Event{}, provider.ErrIgnored
	}
	return ev, nil
}

func (Adapter) ParseStatus(f url.Values) (provider.Event, error) {
	sid, to := strings.TrimSpace(f.Get("MessageSid")), number(f.Get("To"))
	if sid == "" || to == "" {
		return provider.Event{}, provider.ErrIgnored
	}
	var status string
	switch strings.ToLower(f.Get("MessageStatus")) {
	case "sent":
		status = provider.StatusSent
	case "delivered":
		status = provider.StatusDelivered
	case "read":
		status = provider.StatusRead
	case "failed", "undelivered":
		status = provider.StatusFailed
	default:
		return provider.Event{}, provider.ErrIgnored // queued, sending, accepted
	}
	return provider.Event{ID: sid + ":" + status, Kind: provider.KindStatus, ExternalID: sid, Number: to, Status: status,
		Code: strings.TrimSpace(f.Get("ErrorCode")), At: time.Now().UTC()}, nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// reasons are what an agent is told when Twilio refuses a message for good.
var reasons = map[string]string{
	"63003": "This number is not on WhatsApp.",
	"63007": "The sender number is not a WhatsApp sender of this Twilio account.",
	"63015": "This number has not joined the Twilio sandbox.",
	"63018": "WhatsApp's rate limit for this sender was reached. Try again later.",
	"63024": "This number is not on WhatsApp.",
	"63025": "The template's variables are not valid.",
	"63028": "This template is not approved.",
	"63049": "WhatsApp refused to send this message.",
	"21211": "This is not a valid phone number.",
	"21610": "This customer stopped messages to this number.",
	"21614": "This number can't receive WhatsApp messages.",
}

func (a Adapter) Send(ctx context.Context, hc *http.Client, c map[string]string, m provider.Outbound) (string, error) {
	sid, _, err := a.creds(c)
	if err != nil {
		return "", err
	}
	form := url.Values{"From": {waPrefix + m.From}, "To": {waPrefix + m.To}}
	if m.StatusURL != "" {
		form.Set("StatusCallback", m.StatusURL)
	}
	if m.TemplateID != "" {
		form.Set("ContentSid", m.TemplateID)
		vars, _ := json.Marshal(m.Variables)
		if len(m.Variables) > 0 {
			form.Set("ContentVariables", string(vars))
		}
	} else {
		form.Set("Body", truncate(m.Text, maxText))
	}
	resp, err := a.do(ctx, hc, "send", http.MethodPost, a.api()+"/Accounts/"+sid+"/Messages.json", c, form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if resp.StatusCode/100 != 2 {
		err := failure("send", resp, raw)
		var se *provider.StatusError
		if errors.As(err, &se) && (se.Status == http.StatusBadRequest || se.Status == http.StatusNotFound) {
			if se.Code == "63016" {
				return "", provider.OutsideWindow
			}
			if r, ok := reasons[se.Code]; ok {
				return "", &provider.Rejected{Reason: r}
			}
			return "", &provider.Rejected{Reason: "WhatsApp refused this message."}
		}
		return "", err
	}
	var out struct {
		SID string `json:"sid"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.SID == "" {
		return "", fmt.Errorf("send: unreadable answer")
	}
	return out.SID, nil
}

var placeholder = regexp.MustCompile(`\{\{\s*([0-9A-Za-z_]+)\s*\}\}`)

func (a Adapter) Templates(ctx context.Context, hc *http.Client, c map[string]string) ([]provider.WhatsAppTemplate, error) {
	resp, err := a.do(ctx, hc, "templates", http.MethodGet, a.content()+"/ContentAndApprovals?PageSize=200", c, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4*maxBody))
	if resp.StatusCode != http.StatusOK {
		return nil, failure("templates", resp, raw)
	}
	var list struct {
		Contents []struct {
			SID       string                    `json:"sid"`
			Name      string                    `json:"friendly_name"`
			Language  string                    `json:"language"`
			Types     map[string]map[string]any `json:"types"`
			Approvals map[string]map[string]any `json:"approval_requests"`
			Variables map[string]string         `json:"variables"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("templates: %w", err)
	}
	var out []provider.WhatsAppTemplate
	for _, ct := range list.Contents {
		// Only a template WhatsApp approved can be sent outside the window.
		if st, _ := ct.Approvals["whatsapp"]["status"].(string); st != "approved" {
			continue
		}
		body := ""
		for _, kind := range []string{"twilio/text", "twilio/quick-reply", "twilio/call-to-action", "twilio/media", "twilio/card"} {
			if t, ok := ct.Types[kind]; ok {
				for _, key := range []string{"body", "title"} {
					if s, _ := t[key].(string); s != "" {
						body = s
						break
					}
				}
			}
			if body != "" {
				break
			}
		}
		if body == "" {
			continue
		}
		seen := map[string]bool{}
		vars := []string{}
		for _, m := range placeholder.FindAllStringSubmatch(body, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				vars = append(vars, m[1])
			}
		}
		sort.Strings(vars)
		out = append(out, provider.WhatsAppTemplate{ID: ct.SID, Name: ct.Name, Language: ct.Language, Body: body, Variables: vars})
		if len(out) >= maxTemplates {
			break
		}
	}
	return out, nil
}

// mediaHost reports whether a media link is Twilio's or the storage it redirects to.
func mediaHost(u *url.URL) bool {
	if u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, suffix := range []string{".twilio.com", ".twiliocdn.com", ".amazonaws.com"} {
		if strings.HasSuffix(h, suffix) {
			return true
		}
	}
	return false
}

func (a Adapter) Media(ctx context.Context, hc *http.Client, c map[string]string, m provider.Media, limit int64) (io.ReadCloser, error) {
	u, err := url.Parse(m.URL)
	if err != nil || !mediaHost(u) {
		return nil, provider.ErrIgnored
	}
	next := u.String()
	for hop := 0; hop < 3; hop++ {
		resp, err := a.do(ctx, hc, "media", http.MethodGet, next, c, nil)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode/100 == 3 {
			loc, perr := resp.Location()
			resp.Body.Close()
			// The storage link carries its own authorization: only Twilio's own hosts
			// and its storage are followed, never with the credentials.
			if perr != nil || !mediaHost(loc) {
				return nil, provider.ErrIgnored
			}
			next = loc.String()
			if !strings.HasSuffix(strings.ToLower(loc.Hostname()), ".twilio.com") {
				return plain(ctx, hc, next, limit)
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, &provider.StatusError{Op: "media", Status: resp.StatusCode}
		}
		return limited(resp, limit), nil
	}
	return nil, provider.ErrIgnored
}

// plain fetches a storage link without credentials.
func plain(ctx context.Context, hc *http.Client, u string, limit int64) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("media: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, &provider.StatusError{Op: "media", Status: resp.StatusCode}
	}
	return limited(resp, limit), nil
}

type limitedBody struct {
	io.Reader
	io.Closer
}

func limited(resp *http.Response, limit int64) io.ReadCloser {
	return limitedBody{io.LimitReader(resp.Body, limit), resp.Body}
}
