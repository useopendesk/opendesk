// Package helpscout reads a Help Scout account (contracts/imports.md): conversations
// through the Inbox API 2.0 with an app's client credentials, and Docs articles through
// the Docs API v1 when a Docs API key is given.
package helpscout

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/useopendesk/opendesk/internal/helpdesk/imports/source"
)

const (
	apiBase  = "https://api.helpscout.net"
	docsBase = "https://docsapi.helpscout.net"
	appURL   = "https://secure.helpscout.net"

	// maxPages bounds a listing read whole (users, mailboxes, collections, threads).
	maxPages = 200
)

type helpscout struct {
	id, secret string
	login      *source.Client // the token endpoint; sends no bearer token
	api        *source.Client // Inbox API, bearer token
	docs       *source.Client // Docs API, Basic auth; nil without a Docs key
	token      string
	expires    time.Time
}

// New builds a Help Scout source. It checks the credentials' shape only.
func New(c source.Credentials, o source.Options) (source.Source, error) {
	id, err := source.Required(c, "app_id", "App ID")
	if err != nil {
		return nil, err
	}
	secret, err := source.Required(c, "app_secret", "App Secret")
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(c["docs_key"])
	if len(key) > 4096 {
		return nil, source.Invalid("docs_key", "The Docs API key is too long.")
	}
	s := &helpscout{id: id, secret: secret}
	s.login = client(apiBase, o)
	s.api = client(apiBase, o)
	s.api.Auth = func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+s.token) }
	if key != "" {
		s.docs = client(docsBase, o)
		s.docs.Auth = func(r *http.Request) { r.SetBasicAuth(key, "X") }
	}
	return s, nil
}

// client is a source.Client whose 429 answers carry Help Scout's wait as Retry-After.
func client(base string, o source.Options) *source.Client {
	c := source.NewClient(base, o)
	c.HTTP = withRetryAfter(c.HTTP)
	c.Files = withRetryAfter(c.Files)
	return c
}

func withRetryAfter(hc *http.Client) *http.Client {
	cp := *hc
	next := cp.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	cp.Transport = retryAfter{next: next}
	return &cp
}

// retryAfter copies X-RateLimit-Retry-After (Inbox API, seconds) or X-RateLimit-Reset
// (Docs API) into Retry-After on a 429, which is the header source.Status reads.
type retryAfter struct{ next http.RoundTripper }

func (t retryAfter) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(r)
	if err != nil || resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") != "" {
		return resp, err
	}
	if secs, ok := seconds(resp.Header.Get("X-RateLimit-Retry-After")); ok {
		resp.Header.Set("Retry-After", strconv.FormatInt(secs, 10))
	} else if secs, ok := seconds(resp.Header.Get("X-RateLimit-Reset")); ok {
		// A reset far beyond any window is a Unix time rather than a wait.
		if secs > 1_000_000_000 {
			secs -= time.Now().Unix()
		}
		resp.Header.Set("Retry-After", strconv.FormatInt(max(secs, 0), 10))
	}
	return resp, nil
}

func seconds(v string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	return n, err == nil && n >= 0
}

// authorize gets an access token when there is none or it expired (or renew is set).
// fresh says the token was obtained just now.
func (s *helpscout) authorize(ctx context.Context, renew bool) (fresh bool, err error) {
	if !renew && s.token != "" && time.Now().Before(s.expires) {
		return false, nil
	}
	s.token = ""
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {s.id},
		"client_secret": {s.secret},
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	err = s.login.Do(ctx, http.MethodPost, "/v2/oauth2/token", nil, form, &out)
	var re *source.RemoteError
	if errors.As(err, &re) && re.Status == http.StatusBadRequest {
		return false, source.ErrAuth
	}
	if err != nil {
		return false, err
	}
	if out.AccessToken == "" {
		return false, errors.New("help scout answered without an access token")
	}
	life := time.Duration(out.ExpiresIn) * time.Second
	if life <= 0 {
		life = time.Hour
	}
	if life > 2*time.Minute {
		life -= time.Minute
	}
	s.token, s.expires = out.AccessToken, time.Now().Add(life)
	return true, nil
}

// do runs fn with a token, renewing it once when the API refuses it.
func (s *helpscout) do(ctx context.Context, fn func() error) error {
	fresh, err := s.authorize(ctx, false)
	if err != nil {
		return err
	}
	err = fn()
	if errors.Is(err, source.ErrAuth) && !fresh {
		if _, err := s.authorize(ctx, true); err != nil {
			return err
		}
		return fn()
	}
	return err
}

func (s *helpscout) get(ctx context.Context, path string, q url.Values, out any) error {
	return s.do(ctx, func() error { return s.api.Get(ctx, path, q, out) })
}

type page struct {
	Number        int `json:"number"`
	Size          int `json:"size"`
	TotalElements int `json:"totalElements"`
	TotalPages    int `json:"totalPages"`
}

// listAll reads every page of a HAL listing whose items sit in _embedded.<key>.
func listAll[T any](ctx context.Context, s *helpscout, path, key string) ([]T, error) {
	var all []T
	for n := 1; n <= maxPages; n++ {
		var out struct {
			Embedded map[string][]T `json:"_embedded"`
			Page     page           `json:"page"`
		}
		if err := s.get(ctx, path, url.Values{"page": {strconv.Itoa(n)}}, &out); err != nil {
			return nil, err
		}
		items := out.Embedded[key]
		all = append(all, items...)
		if len(items) == 0 || n >= out.Page.TotalPages {
			break
		}
	}
	return all, nil
}

type user struct {
	ID        int64  `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Type      string `json:"type"`
}

type mailbox struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (s *helpscout) Check(ctx context.Context) (source.Scan, error) {
	scan := source.Scan{Tickets: -1, Articles: -1}
	var me struct {
		CompanyID int64 `json:"companyId"`
	}
	if err := s.get(ctx, "/v2/users/me", nil, &me); err != nil {
		return scan, err
	}
	boxes, err := listAll[mailbox](ctx, s, "/v2/mailboxes", "mailboxes")
	if err != nil {
		return scan, err
	}
	sort.Slice(boxes, func(i, j int) bool { return boxes[i].ID < boxes[j].ID })
	switch {
	case me.CompanyID > 0:
		scan.Account.Key = strconv.FormatInt(me.CompanyID, 10)
	case len(boxes) > 0:
		// Mailbox ids are unique across Help Scout, and the oldest one stays with the account.
		scan.Account.Key = "mailbox-" + strconv.FormatInt(boxes[0].ID, 10)
	default:
		return scan, errors.New("help scout did not say which account this is")
	}
	scan.Account.Name = "Help Scout"
	if len(boxes) > 0 && strings.TrimSpace(boxes[0].Name) != "" {
		scan.Account.Name = strings.TrimSpace(boxes[0].Name)
	}
	scan.Account.URL = appURL
	for _, b := range boxes {
		scan.Groups = append(scan.Groups, source.Group{ID: strconv.FormatInt(b.ID, 10), Name: strings.TrimSpace(b.Name)})
	}

	users, err := listAll[user](ctx, s, "/v2/users", "users")
	if err != nil {
		return scan, err
	}
	for _, u := range users {
		if u.Type != "" && u.Type != "user" {
			continue
		}
		name := fullName(u.FirstName, u.LastName)
		if name == "" {
			name = u.Email
		}
		scan.Agents = append(scan.Agents, source.Agent{ID: strconv.FormatInt(u.ID, 10), Name: name, Email: strings.TrimSpace(u.Email)})
	}

	var count struct {
		Page page `json:"page"`
	}
	if err := s.get(ctx, "/v2/conversations", url.Values{"status": {"all"}, "page": {"1"}}, &count); err != nil {
		return scan, err
	}
	scan.Tickets = count.Page.TotalElements

	if s.docs != nil {
		cols, err := s.collections(ctx)
		switch {
		case errors.Is(err, source.ErrAuth):
			// The Docs key is optional: a refused one leaves the tickets importable.
		case err != nil:
			return scan, err
		default:
			scan.HasArticles = true
			scan.Articles = 0
			for _, c := range cols {
				scan.Articles += c.ArticleCount
			}
		}
	}
	return scan, nil
}

type person struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	First string `json:"first"`
	Last  string `json:"last"`
	Email string `json:"email"`
}

type conversation struct {
	ID              int64   `json:"id"`
	Number          int64   `json:"number"`
	Type            string  `json:"type"`
	Status          string  `json:"status"`
	State           string  `json:"state"`
	Subject         string  `json:"subject"`
	MailboxID       int64   `json:"mailboxId"`
	Threads         int     `json:"threads"`
	Assignee        *person `json:"assignee"`
	PrimaryCustomer *person `json:"primaryCustomer"`
	CreatedAt       string  `json:"createdAt"`
	ClosedAt        string  `json:"closedAt"`
	UserUpdatedAt   string  `json:"userUpdatedAt"`
	Tags            []struct {
		Tag string `json:"tag"`
	} `json:"tags"`
	Source struct {
		Type string `json:"type"`
	} `json:"source"`
	Embedded struct {
		Threads []thread `json:"threads"`
	} `json:"_embedded"`
}

type thread struct {
	ID        int64    `json:"id"`
	Type      string   `json:"type"`
	State     string   `json:"state"`
	Body      string   `json:"body"`
	Customer  *person  `json:"customer"`
	CreatedBy *person  `json:"createdBy"`
	To        []string `json:"to"`
	Cc        []string `json:"cc"`
	CreatedAt string   `json:"createdAt"`
	Rating    *struct {
		Rating string `json:"rating"`
	} `json:"rating"`
	Embedded struct {
		Attachments []attachment `json:"attachments"`
	} `json:"_embedded"`
}

type attachment struct {
	ID       int64  `json:"id"`
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
	State    string `json:"state"`
}

func pageNumber(cursor string) (int, error) {
	if cursor == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(cursor)
	if err != nil || n < 1 {
		return 0, errors.New("help scout: malformed cursor")
	}
	return n, nil
}

func (s *helpscout) Tickets(ctx context.Context, cursor string, since, updatedSince *time.Time) (source.Page[source.Ticket], error) {
	var res source.Page[source.Ticket]
	n, err := pageNumber(cursor)
	if err != nil {
		return res, err
	}
	// Oldest first by creation: new conversations land on later pages, so a page number
	// stays a resumable cursor.
	q := url.Values{
		"status":    {"all"},
		"embed":     {"threads"},
		"sortField": {"createdAt"},
		"sortOrder": {"asc"},
		"page":      {strconv.Itoa(n)},
	}
	if since != nil {
		q.Set("query", "(createdAt:["+since.UTC().Format(time.RFC3339)+" TO *])")
	}
	if updatedSince != nil {
		q.Set("modifiedSince", updatedSince.UTC().Format(time.RFC3339))
	}
	var out struct {
		Embedded struct {
			Conversations []conversation `json:"conversations"`
		} `json:"_embedded"`
		Page page `json:"page"`
	}
	if err := s.get(ctx, "/v2/conversations", q, &out); err != nil {
		return res, err
	}
	for _, c := range out.Embedded.Conversations {
		res.Items = append(res.Items, s.ticket(c))
	}
	if len(out.Embedded.Conversations) > 0 && n < out.Page.TotalPages {
		res.Next = strconv.Itoa(n + 1)
	}
	return res, nil
}

func (s *helpscout) ticket(c conversation) source.Ticket {
	id := strconv.FormatInt(c.ID, 10)
	t := source.Ticket{
		ID:        id,
		Ref:       "#" + strconv.FormatInt(c.Number, 10),
		URL:       appURL + "/conversation/" + id + "/" + strconv.FormatInt(c.Number, 10),
		Subject:   strings.TrimSpace(c.Subject),
		Priority:  "normal",
		Channel:   channel(c),
		CreatedAt: source.Time(c.CreatedAt),
	}
	switch c.Status {
	case "pending":
		t.Status = "pending"
	case "closed":
		t.Status = "closed"
		if at := source.Time(c.ClosedAt); !at.IsZero() {
			t.SolvedAt = &at
		}
	case "spam":
		t.Status, t.Skip = "closed", true
	default: // active
		t.Status = "open"
	}
	if c.State == "deleted" || c.State == "draft" {
		t.Skip = true
	}
	if c.MailboxID > 0 {
		t.GroupID = strconv.FormatInt(c.MailboxID, 10)
	}
	if a := c.Assignee; a != nil && a.ID > 0 && (a.Type == "" || a.Type == "user") {
		t.AssigneeID = strconv.FormatInt(a.ID, 10)
	}
	if p := c.PrimaryCustomer; p != nil && (p.ID > 0 || p.Email != "") {
		t.Requester = &source.Person{ID: personID(p), Email: strings.TrimSpace(p.Email), Name: fullName(p.First, p.Last)}
	}
	for _, tag := range c.Tags {
		t.Tags = append(t.Tags, tag.Tag)
	}
	t.Tags = source.Strings(t.Tags)
	t.UpdatedAt = source.Time(c.UserUpdatedAt)
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = source.Time(c.ClosedAt)
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = t.CreatedAt
	}

	threads := c.Embedded.Threads
	t.Messages, t.CSAT = s.messages(id, threads)
	// The listing truncates chat threads, and may hold fewer threads than the conversation.
	t.Loaded = c.Type != "chat" && len(threads) >= c.Threads
	for _, th := range threads {
		if th.Type == "chat" || th.Type == "beaconchat" {
			t.Loaded = false
		}
	}
	return t
}

func channel(c conversation) string {
	switch c.Type {
	case "chat":
		return "chat"
	case "phone":
		return "api"
	}
	switch c.Source.Type {
	case "beacon":
		return "portal"
	case "api":
		return "api"
	}
	return "email"
}

func (s *helpscout) Load(ctx context.Context, t *source.Ticket) error {
	if t.Loaded {
		return nil
	}
	if _, err := strconv.ParseInt(t.ID, 10, 64); err != nil {
		return errors.New("help scout: malformed conversation id")
	}
	var all []thread
	for n := 1; n <= maxPages; n++ {
		var out struct {
			Embedded struct {
				Threads []thread `json:"threads"`
			} `json:"_embedded"`
			Page page `json:"page"`
		}
		if err := s.get(ctx, "/v2/conversations/"+t.ID+"/threads", url.Values{"page": {strconv.Itoa(n)}}, &out); err != nil {
			return err
		}
		all = append(all, out.Embedded.Threads...)
		if len(out.Embedded.Threads) == 0 || n >= out.Page.TotalPages {
			break
		}
	}
	msgs, csat := s.messages(t.ID, all)
	t.Messages = msgs
	if csat > 0 {
		t.CSAT = csat
	}
	t.Loaded = true
	return nil
}

// messages maps threads (Help Scout lists them newest first) to messages oldest first,
// with the latest satisfaction rating.
func (s *helpscout) messages(cid string, threads []thread) ([]source.Message, int) {
	ts := append([]thread(nil), threads...)
	sort.SliceStable(ts, func(i, j int) bool {
		a, b := source.Time(ts[i].CreatedAt), source.Time(ts[j].CreatedAt)
		if !a.Equal(b) {
			return a.Before(b)
		}
		return ts[i].ID < ts[j].ID
	})
	var out []source.Message
	csat := 0
	for _, th := range ts {
		if th.Rating != nil {
			if r := rating(th.Rating.Rating); r > 0 {
				csat = r
			}
		}
		if m, ok := s.message(cid, th); ok {
			out = append(out, m)
		}
	}
	return out, csat
}

func rating(r string) int {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "great", "good":
		return 5
	case "okay", "ok":
		return 3
	case "not good", "notgood", "bad":
		return 1
	}
	return 0
}

func (s *helpscout) message(cid string, th thread) (source.Message, bool) {
	switch th.State {
	case "draft", "hidden", "review":
		return source.Message{}, false
	}
	byCustomer := th.CreatedBy != nil && th.CreatedBy.Type == "customer"
	m := source.Message{ID: strconv.FormatInt(th.ID, 10), HTML: th.Body, To: source.Strings(th.To), Cc: source.Strings(th.Cc), CreatedAt: source.Time(th.CreatedAt)}
	switch th.Type {
	case "customer":
		byCustomer = true
	case "message", "reply", "forwardchild":
		byCustomer = false
	case "note", "forwardparent":
		byCustomer, m.Note = false, true
	case "chat", "beaconchat", "phone":
		// Written by whoever created the thread.
	default: // lineitem and other status lines
		return source.Message{}, false
	}
	if byCustomer {
		p := th.Customer
		if p == nil || (p.ID == 0 && p.Email == "") {
			p = th.CreatedBy
		}
		if p != nil {
			m.Author = source.Person{ID: personID(p), Email: strings.TrimSpace(p.Email), Name: fullName(p.First, p.Last)}
		}
	} else {
		m.Agent = true
		if p := th.CreatedBy; p != nil {
			m.Author = source.Person{ID: personID(p), Email: strings.TrimSpace(p.Email), Name: fullName(p.First, p.Last)}
			if p.Type == "user" && p.ID > 0 {
				m.AuthorID = strconv.FormatInt(p.ID, 10)
			}
		}
	}
	for _, a := range th.Embedded.Attachments {
		if a.ID <= 0 || (a.State != "" && a.State != "valid") {
			continue
		}
		aid := strconv.FormatInt(a.ID, 10)
		m.Attachments = append(m.Attachments, source.Attachment{
			ID:          aid,
			Name:        strings.TrimSpace(a.Filename),
			ContentType: strings.TrimSpace(a.MimeType),
			Size:        max(a.Size, 0),
			URL:         s.api.Base + "/v2/conversations/" + cid + "/attachments/" + aid + "/data",
		})
	}
	return m, true
}

func personID(p *person) string {
	if p == nil || p.ID <= 0 {
		return ""
	}
	return strconv.FormatInt(p.ID, 10)
}

func fullName(first, last string) string {
	return strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
}

type collection struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ArticleCount int    `json:"articleCount"`
}

type docsPage[T any] struct {
	Page  int `json:"page"`
	Pages int `json:"pages"`
	Count int `json:"count"`
	Items []T `json:"items"`
}

func (s *helpscout) collections(ctx context.Context) ([]collection, error) {
	var all []collection
	for n := 1; n <= maxPages; n++ {
		var out struct {
			Collections docsPage[collection] `json:"collections"`
		}
		q := url.Values{"page": {strconv.Itoa(n)}, "sort": {"createdAt"}, "order": {"asc"}}
		if err := s.docs.Get(ctx, "/v1/collections", q, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Collections.Items...)
		if len(out.Collections.Items) == 0 || n >= out.Collections.Pages {
			break
		}
	}
	return all, nil
}

type article struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Text      string `json:"text"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Articles reads one page of one collection's articles. The cursor is
// "<collection id>:<page>".
func (s *helpscout) Articles(ctx context.Context, cursor string) (source.Page[source.Article], error) {
	var res source.Page[source.Article]
	if s.docs == nil {
		return res, source.ErrUnsupported
	}
	cols, err := s.collections(ctx)
	if err != nil {
		return res, err
	}
	ci, n := 0, 1
	if cursor != "" {
		id, p, ok := strings.Cut(cursor, ":")
		n, err = strconv.Atoi(p)
		if !ok || err != nil || n < 1 {
			return res, errors.New("help scout: malformed cursor")
		}
		ci = -1
		for i, c := range cols {
			if c.ID == id {
				ci = i
				break
			}
		}
		if ci < 0 {
			// The collection is gone: start over, since imported articles are not written twice.
			ci, n = 0, 1
		}
	}
	if len(cols) == 0 {
		return res, nil
	}
	col := cols[ci]
	var out struct {
		Articles docsPage[article] `json:"articles"`
	}
	q := url.Values{"status": {"all"}, "page": {strconv.Itoa(n)}, "pageSize": {"50"}, "sort": {"createdAt"}, "order": {"asc"}}
	if err := s.docs.Get(ctx, "/v1/collections/"+url.PathEscape(col.ID)+"/articles", q, &out); err != nil {
		return res, err
	}
	for _, ref := range out.Articles.Items {
		var full struct {
			Article article `json:"article"`
		}
		err := s.docs.Get(ctx, "/v1/articles/"+url.PathEscape(ref.ID), nil, &full)
		var re *source.RemoteError
		if errors.As(err, &re) && re.Status == http.StatusNotFound {
			continue // deleted since the listing
		}
		if err != nil {
			return res, err
		}
		a := full.Article
		created := source.Time(a.CreatedAt)
		updated := source.Time(a.UpdatedAt)
		if updated.IsZero() {
			updated = created
		}
		res.Items = append(res.Items, source.Article{
			ID:         a.ID,
			Title:      strings.TrimSpace(a.Name),
			HTML:       a.Text,
			Collection: strings.TrimSpace(col.Name),
			Published:  a.Status == "published",
			CreatedAt:  created,
			UpdatedAt:  updated,
		})
	}
	switch {
	case len(out.Articles.Items) > 0 && n < out.Articles.Pages:
		res.Next = col.ID + ":" + strconv.Itoa(n+1)
	case ci+1 < len(cols):
		res.Next = cols[ci+1].ID + ":1"
	}
	return res, nil
}

// Download streams an attachment. Help Scout answers {"data": "<base64>"}; the file is
// decoded as it is read, never held whole.
func (s *helpscout) Download(ctx context.Context, a source.Attachment) (io.ReadCloser, error) {
	if a.Data != nil {
		return io.NopCloser(bytes.NewReader(a.Data)), nil
	}
	if a.URL == "" {
		return nil, errors.New("the attachment has no link")
	}
	var body io.ReadCloser
	err := s.do(ctx, func() error {
		var err error
		body, err = s.api.Open(ctx, a.URL)
		return err
	})
	if err != nil {
		return nil, err
	}
	br := bufio.NewReader(body)
	if err := seekData(br); err != nil {
		_ = body.Close()
		return nil, err
	}
	return struct {
		io.Reader
		io.Closer
	}{base64.NewDecoder(base64.StdEncoding, &jsonString{r: br}), body}, nil
}

var errNoData = errors.New("help scout answered an attachment without its data")

// seekData reads up to the opening quote of the "data" string.
func seekData(r *bufio.Reader) error {
	const key = `"data"`
	for {
		matched := 0
		for matched < len(key) {
			b, err := r.ReadByte()
			if err != nil {
				return errNoData
			}
			switch {
			case b == key[matched]:
				matched++
			case b == key[0]:
				matched = 1
			default:
				matched = 0
			}
		}
		if next(r) != ':' {
			continue
		}
		if next(r) != '"' {
			return errNoData
		}
		return nil
	}
}

// next is the next byte that is not JSON whitespace; 0 at the end.
func next(r *bufio.Reader) byte {
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0
		}
		if b != ' ' && b != '\n' && b != '\r' && b != '\t' {
			return b
		}
	}
}

// jsonString reads the rest of a JSON string holding base64, unescaping "\/" and dropping
// escaped line breaks, up to its closing quote.
type jsonString struct {
	r    *bufio.Reader
	done bool
}

func (j *jsonString) Read(p []byte) (int, error) {
	if j.done {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) {
		b, err := j.r.ReadByte()
		if err != nil {
			return n, io.ErrUnexpectedEOF
		}
		switch b {
		case '"':
			j.done = true
			if n == 0 {
				return 0, io.EOF
			}
			return n, nil
		case '\\':
			e, err := j.r.ReadByte()
			if err != nil {
				return n, io.ErrUnexpectedEOF
			}
			switch e {
			case '/':
				p[n] = '/'
				n++
			case 'n', 'r', 't':
			default:
				return n, errors.New("unexpected escape in attachment data")
			}
		default:
			p[n] = b
			n++
		}
		if n > 0 && j.r.Buffered() == 0 {
			break
		}
	}
	return n, nil
}
