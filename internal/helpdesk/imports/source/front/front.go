// Package front reads shared inbox conversations from Front's Core API.
package front

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/useopendesk/opendesk/internal/helpdesk/imports/source"
)

const (
	apiURL        = "https://api2.frontapp.com"
	appURL        = "https://app.frontapp.com"
	pageSize      = 100
	eventPageSize = 15
	requestGap    = time.Minute / 45
)

var htmlTag = regexp.MustCompile(`(?i)<(p|br|div|span|a|b|strong|em|i|ul|ol|li|table|img|h[1-6]|blockquote|pre)(\s|>|/>)`)

type front struct {
	c       *source.Client
	account account
	inboxes []inbox
}

type account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type inbox struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Private bool   `json:"is_private"`
}

type teammate struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

func (t teammate) name() string {
	if name := strings.TrimSpace(strings.TrimSpace(t.FirstName) + " " + strings.TrimSpace(t.LastName)); name != "" {
		return name
	}
	return strings.TrimSpace(t.Username)
}

type page[T any] struct {
	Results    []T `json:"_results"`
	Pagination struct {
		Next string `json:"next"`
	} `json:"_pagination"`
}

type cursor struct {
	Mode  string `json:"mode"`
	Inbox int    `json:"inbox"`
	Token string `json:"token"`
}

type conversation struct {
	ID             string            `json:"id"`
	Subject        string            `json:"subject"`
	Status         string            `json:"status"`
	StatusCategory string            `json:"status_category"`
	CreatedAt      float64           `json:"created_at"`
	UpdatedAt      float64           `json:"updated_at"`
	Private        bool              `json:"is_private"`
	Assignee       *teammate         `json:"assignee"`
	Tags           []tag             `json:"tags"`
	Reminders      []json.RawMessage `json:"scheduled_reminders"`
}

type tag struct {
	Name string `json:"name"`
}

type participant struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Handle    string `json:"handle"`
	Role      string `json:"role"`
}

func (p participant) displayName() string {
	if name := strings.TrimSpace(p.Name); name != "" {
		return name
	}
	if name := strings.TrimSpace(strings.TrimSpace(p.FirstName) + " " + strings.TrimSpace(p.LastName)); name != "" {
		return name
	}
	return strings.TrimSpace(p.Username)
}

type attachment struct {
	ID          string `json:"id"`
	Name        string `json:"filename"`
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Metadata    struct {
		CID string `json:"cid"`
	} `json:"metadata"`
}

type message struct {
	ID          string        `json:"id"`
	Type        string        `json:"type"`
	Inbound     bool          `json:"is_inbound"`
	Draft       bool          `json:"is_draft"`
	Body        string        `json:"body"`
	Text        string        `json:"text"`
	CreatedAt   float64       `json:"created_at"`
	Author      participant   `json:"author"`
	Recipients  []participant `json:"recipients"`
	Attachments []attachment  `json:"attachments"`
}

type comment struct {
	ID          string       `json:"id"`
	Body        string       `json:"body"`
	CreatedAt   float64      `json:"created_at"`
	Author      participant  `json:"author"`
	Attachments []attachment `json:"attachments"`
}

type event struct {
	EmittedAt    float64      `json:"emitted_at"`
	Conversation conversation `json:"conversation"`
}

// New builds a Front source from a read-only Core API token.
func New(credentials source.Credentials, options source.Options) (source.Source, error) {
	token, err := source.Required(credentials, "token", "API token")
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(token, " \t\r\n") {
		return nil, source.Invalid("token", "The API token has no spaces; paste it again.")
	}
	c := source.NewClient(apiURL, options)
	c.Auth = func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+token) }
	if options.BaseURL == "" {
		p := &pacer{}
		pace(c.HTTP, p)
		pace(c.Files, p)
	}
	return &front{c: c}, nil
}

type pacer struct {
	mu   sync.Mutex
	next time.Time
}

func (p *pacer) wait(ctx context.Context) error {
	p.mu.Lock()
	wait := time.Until(p.next)
	p.next = time.Now().Add(requestGap)
	p.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type pacedTransport struct {
	next http.RoundTripper
	p    *pacer
}

func (t pacedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.p.wait(req.Context()); err != nil {
		return nil, err
	}
	return t.next.RoundTrip(req)
}

func pace(client *http.Client, p *pacer) {
	cp := *client
	next := cp.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	cp.Transport = pacedTransport{next: next, p: p}
	*client = cp
}

func (s *front) Check(ctx context.Context) (source.Scan, error) {
	scan := source.Scan{Tickets: -1, Articles: 0}
	if err := s.c.Get(ctx, "/me", nil, &s.account); err != nil {
		return scan, err
	}
	if s.account.ID == "" {
		return scan, source.ErrAuth
	}
	if s.account.Name == "" {
		s.account.Name = s.account.ID
	}
	inboxes, err := listAll[inbox](ctx, s.c, "/inboxes")
	if err != nil {
		return scan, err
	}
	s.inboxes = make([]inbox, 0, len(inboxes))
	for _, box := range inboxes {
		if box.ID == "" || box.Private {
			continue
		}
		s.inboxes = append(s.inboxes, box)
		scan.Groups = append(scan.Groups, source.Group{ID: box.ID, Name: strings.TrimSpace(box.Name)})
	}
	teammates, err := listAll[teammate](ctx, s.c, "/teammates")
	if err != nil {
		return scan, err
	}
	for _, person := range teammates {
		if person.ID == "" {
			continue
		}
		agent := source.Agent{ID: person.ID, Name: person.name(), Email: strings.TrimSpace(person.Email)}
		scan.Agents = append(scan.Agents, agent)
	}
	scan.Account = source.Account{Key: s.account.ID, Name: s.account.Name, URL: appURL}
	return scan, nil
}

func listAll[T any](ctx context.Context, c *source.Client, path string) ([]T, error) {
	var all []T
	token := ""
	for {
		q := url.Values{"limit": {fmt.Sprint(pageSize)}}
		if token != "" {
			q.Set("page_token", token)
		}
		var response page[T]
		if err := c.Get(ctx, path, q, &response); err != nil {
			return nil, err
		}
		all = append(all, response.Results...)
		next, err := pageToken(response.Pagination.Next, token)
		if err != nil {
			return nil, err
		}
		if next == "" {
			return all, nil
		}
		token = next
	}
}

func pageToken(raw, current string) (string, error) {
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("front: malformed pagination URL: %w", err)
	}
	token := u.Query().Get("page_token")
	if token == "" {
		return "", errors.New("front: pagination URL has no page token")
	}
	if token == current {
		return "", errors.New("front: pagination cursor did not advance")
	}
	return token, nil
}

func (s *front) Tickets(ctx context.Context, raw string, since, updatedSince *time.Time) (source.Page[source.Ticket], error) {
	cur := cursor{Mode: "list"}
	if updatedSince != nil {
		cur.Mode = "events"
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &cur); err != nil || cur.Inbox < 0 ||
			(cur.Mode != "list" && cur.Mode != "events" && cur.Mode != "fallback") {
			return source.Page[source.Ticket]{}, fmt.Errorf("front: malformed ticket cursor")
		}
	}
	if cur.Inbox >= len(s.inboxes) {
		return source.Page[source.Ticket]{}, nil
	}
	if cur.Mode == "events" {
		out, err := s.events(ctx, cur, updatedSince)
		if errors.Is(err, source.ErrAuth) {
			return s.inboxPage(ctx, cursor{Mode: "fallback"}, since, updatedSince)
		}
		return out, err
	}
	return s.inboxPage(ctx, cur, since, updatedSince)
}

func (s *front) inboxPage(ctx context.Context, cur cursor, since, updatedSince *time.Time) (source.Page[source.Ticket], error) {
	box := s.inboxes[cur.Inbox]
	q := url.Values{"limit": {fmt.Sprint(pageSize)}}
	if cur.Token != "" {
		q.Set("page_token", cur.Token)
	}
	var response page[conversation]
	path := "/inboxes/" + url.PathEscape(box.ID) + "/conversations"
	if err := s.c.Get(ctx, path, q, &response); err != nil {
		return source.Page[source.Ticket]{}, err
	}
	var out source.Page[source.Ticket]
	for _, conversation := range response.Results {
		ticket := s.ticket(conversation, box.ID)
		if since != nil && ticket.CreatedAt.Before(*since) {
			continue
		}
		if updatedSince != nil && !ticket.UpdatedAt.IsZero() && ticket.UpdatedAt.Before(*updatedSince) {
			continue
		}
		out.Items = append(out.Items, ticket)
	}
	next, err := pageToken(response.Pagination.Next, cur.Token)
	if err != nil {
		return source.Page[source.Ticket]{}, err
	}
	if updatedSince != nil && cur.Mode == "fallback" && len(response.Results) > 0 {
		last := s.ticket(response.Results[len(response.Results)-1], box.ID)
		if !last.UpdatedAt.IsZero() && last.UpdatedAt.Before(*updatedSince) {
			next = ""
		}
	}
	if next != "" {
		cur.Token = next
	} else {
		cur.Inbox++
		cur.Token = ""
	}
	if cur.Inbox < len(s.inboxes) {
		out.Next = encodeCursor(cur)
	}
	return out, nil
}

func (s *front) events(ctx context.Context, cur cursor, updatedSince *time.Time) (source.Page[source.Ticket], error) {
	box := s.inboxes[cur.Inbox]
	q := url.Values{
		"limit":      {fmt.Sprint(eventPageSize)},
		"q[after]":   {frontTimestamp(*updatedSince)},
		"q[inboxes]": {box.ID},
		"sort_by":    {"created_at"},
		"sort_order": {"asc"},
	}
	if cur.Token != "" {
		q.Set("page_token", cur.Token)
	}
	var response page[event]
	if err := s.c.Get(ctx, "/events", q, &response); err != nil {
		return source.Page[source.Ticket]{}, err
	}
	byID := map[string]source.Ticket{}
	for _, item := range response.Results {
		if item.Conversation.ID == "" {
			continue
		}
		ticket := s.ticket(item.Conversation, box.ID)
		if at := frontTime(item.EmittedAt); at.After(ticket.UpdatedAt) {
			ticket.UpdatedAt = at
		}
		byID[ticket.ID] = ticket
	}
	var out source.Page[source.Ticket]
	for _, ticket := range byID {
		out.Items = append(out.Items, ticket)
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].UpdatedAt.Before(out.Items[j].UpdatedAt) })
	next, err := pageToken(response.Pagination.Next, cur.Token)
	if err != nil {
		return source.Page[source.Ticket]{}, err
	}
	if next != "" {
		cur.Token = next
	} else {
		cur.Inbox++
		cur.Token = ""
	}
	if cur.Inbox < len(s.inboxes) {
		out.Next = encodeCursor(cur)
	}
	return out, nil
}

func encodeCursor(cur cursor) string {
	b, _ := json.Marshal(cur)
	return string(b)
}

func frontTimestamp(t time.Time) string {
	return fmt.Sprintf("%d.%03d", t.Unix(), t.Nanosecond()/int(time.Millisecond))
}

func frontTime(seconds float64) time.Time {
	if seconds <= 0 {
		return time.Time{}
	}
	whole := int64(seconds)
	nanos := int64((seconds - float64(whole)) * float64(time.Second))
	return time.Unix(whole, nanos).UTC()
}

func (s *front) ticket(cv conversation, inboxID string) source.Ticket {
	created := frontTime(cv.CreatedAt)
	updated := frontTime(cv.UpdatedAt)
	if updated.IsZero() {
		updated = created
	}
	t := source.Ticket{
		ID:        cv.ID,
		Ref:       cv.ID,
		URL:       appURL + "/open/" + url.PathEscape(cv.ID),
		Subject:   strings.TrimSpace(cv.Subject),
		Status:    status(cv),
		Skip:      cv.Private || cv.Status == "deleted" || cv.Status == "trashed" || cv.Status == "spam",
		Priority:  "normal",
		Channel:   "email",
		GroupID:   inboxID,
		CreatedAt: created,
		UpdatedAt: updated,
	}
	if cv.Assignee != nil {
		t.AssigneeID = cv.Assignee.ID
	}
	for _, item := range cv.Tags {
		t.Tags = append(t.Tags, item.Name)
	}
	t.Tags = source.Strings(t.Tags)
	return t
}

func status(cv conversation) string {
	switch cv.Status {
	case "assigned", "unassigned", "open":
		return "open"
	case "archived":
		if len(cv.Reminders) > 0 {
			return "on_hold"
		}
		switch cv.StatusCategory {
		case "resolved":
			return "solved"
		case "waiting":
			return "pending"
		}
		return "closed"
	case "deleted", "trashed", "spam":
		return "closed"
	default:
		return "open"
	}
}

func (s *front) Load(ctx context.Context, ticket *source.Ticket) error {
	var cv conversation
	if err := s.c.Get(ctx, "/conversations/"+url.PathEscape(ticket.ID), nil, &cv); err != nil {
		return err
	}
	if cv.ID != "" && cv.ID != ticket.ID {
		ticket.Skip = true
		return nil
	}
	messages, err := listConversation[message](ctx, s.c, "/conversations/"+url.PathEscape(ticket.ID)+"/messages")
	if err != nil {
		return err
	}
	var commentPage page[comment]
	if err := s.c.Get(ctx, "/conversations/"+url.PathEscape(ticket.ID)+"/comments", nil, &commentPage); err != nil {
		return err
	}
	for _, item := range messages {
		if item.Draft {
			continue
		}
		m := messageFrom(item)
		ticket.Messages = append(ticket.Messages, m)
	}
	for _, item := range commentPage.Results {
		ticket.Messages = append(ticket.Messages, commentFrom(item))
	}
	sort.SliceStable(ticket.Messages, func(i, j int) bool {
		return ticket.Messages[i].CreatedAt.Before(ticket.Messages[j].CreatedAt)
	})
	for i := range ticket.Messages {
		m := &ticket.Messages[i]
		for j := range m.Attachments {
			if m.Attachments[j].ID == "" {
				m.Attachments[j].ID = fmt.Sprintf("%s-%d", m.ID, j+1)
			}
		}
		if ticket.Requester == nil && !m.Agent {
			person := m.Author
			ticket.Requester = &person
		}
	}
	if len(ticket.Messages) > 0 {
		ticket.Channel = channel(messages)
	}
	ticket.Loaded = true
	return nil
}

func listConversation[T any](ctx context.Context, c *source.Client, path string) ([]T, error) {
	var all []T
	token := ""
	for {
		q := url.Values{"limit": {fmt.Sprint(pageSize)}, "sort_by": {"created_at"}, "sort_order": {"asc"}}
		if token != "" {
			q.Set("page_token", token)
		}
		var response page[T]
		if err := c.Get(ctx, path, q, &response); err != nil {
			return nil, err
		}
		all = append(all, response.Results...)
		next, err := pageToken(response.Pagination.Next, token)
		if err != nil {
			return nil, err
		}
		if next == "" {
			return all, nil
		}
		token = next
	}
}

func messageFrom(item message) source.Message {
	participant := item.Author
	if item.Inbound && participant.ID == "" && participant.Email == "" && participant.displayName() == "" && participant.Handle == "" {
		for _, recipient := range item.Recipients {
			if strings.EqualFold(recipient.Role, "from") {
				participant = recipient
				participant.ID = strings.TrimSpace(participant.Handle)
				if participant.Email == "" && strings.Contains(participant.Handle, "@") {
					participant.Email = participant.Handle
				}
				break
			}
		}
	}
	author := person(participant)
	m := source.Message{
		ID:        item.ID,
		Agent:     !item.Inbound,
		Author:    author,
		Text:      stripReadReceipt(item.Text),
		CreatedAt: frontTime(item.CreatedAt),
	}
	if body := strings.TrimSpace(item.Body); htmlTag.MatchString(body) {
		m.HTML = body
	} else if m.Text == "" {
		m.Text = stripReadReceipt(body)
	}
	if m.Agent {
		m.AuthorID = author.ID
	}
	for _, recipient := range item.Recipients {
		switch strings.ToLower(recipient.Role) {
		case "to":
			m.To = append(m.To, recipient.Handle)
		case "cc":
			m.Cc = append(m.Cc, recipient.Handle)
		}
	}
	m.Attachments = attachments(item.Attachments)
	return m
}

func commentFrom(item comment) source.Message {
	author := person(item.Author)
	return source.Message{
		ID:          item.ID,
		Agent:       true,
		Note:        true,
		AuthorID:    author.ID,
		Author:      author,
		Text:        strings.TrimSpace(item.Body),
		CreatedAt:   frontTime(item.CreatedAt),
		Attachments: attachments(item.Attachments),
	}
}

func person(author participant) source.Person {
	return source.Person{ID: author.ID, Email: strings.TrimSpace(author.Email), Name: author.displayName()}
}

func attachments(files []attachment) []source.Attachment {
	out := make([]source.Attachment, 0, len(files))
	for _, file := range files {
		out = append(out, source.Attachment{
			ID: file.ID, Name: strings.TrimSpace(file.Name), ContentType: strings.TrimSpace(file.ContentType),
			Size: file.Size, URL: file.URL, Inline: file.Metadata.CID != "",
		})
	}
	return out
}

func stripReadReceipt(text string) string {
	return strings.TrimSpace(strings.ReplaceAll(text, "Sent from Front", ""))
}

func channel(messages []message) string {
	for _, item := range messages {
		switch strings.ToLower(item.Type) {
		case "email":
			return "email"
		case "front_chat", "sms", "whatsapp", "facebook", "twitter", "twitter_dm", "tweet", "tweet_dm", "googleplay", "intercom", "smooch", "custom":
			return "chat"
		case "call", "api":
			return "api"
		}
	}
	return "email"
}

func (s *front) Articles(context.Context, string) (source.Page[source.Article], error) {
	return source.Page[source.Article]{}, source.ErrUnsupported
}

func (s *front) Download(ctx context.Context, file source.Attachment) (io.ReadCloser, error) {
	if file.URL == "" {
		return nil, fmt.Errorf("front: attachment without a link")
	}
	return s.c.Open(ctx, file.URL)
}
