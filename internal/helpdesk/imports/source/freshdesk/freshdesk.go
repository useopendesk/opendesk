// Package freshdesk reads a Freshdesk account through its API v2 (contracts/imports.md):
// tickets with their conversations, requesters and companies, agents, groups and solution
// articles.
package freshdesk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/useopendesk/opendesk/internal/helpdesk/imports/source"
)

// perPage is the most Freshdesk returns in one page.
const perPage = 100

// maxPage is the last page a Freshdesk listing serves (300 pages, 30000 tickets).
const maxPage = 300

// checkCalls bounds the requests Check spends counting articles.
const checkCalls = 40

// treeCalls bounds the requests spent walking the solutions tree.
const treeCalls = 5000

type src struct {
	c      *source.Client
	domain string
	web    string // https://acme.freshdesk.com

	agents    map[string]source.Agent // nil until read
	companies map[int64]company
	folders   []folder // nil until read
}

type company struct {
	Name   string
	Domain string
}

// folder is one solution folder in tree order (categories, then folders depth first).
type folder struct {
	ID       int64
	Name     string
	Articles int // -1 when Freshdesk does not say
}

// New builds a Freshdesk source from the domain and an agent's API key.
func New(c source.Credentials, o source.Options) (source.Source, error) {
	domain, err := source.Subdomain(c["domain"], ".freshdesk.com", "domain")
	if err != nil {
		return nil, err
	}
	key, err := source.Required(c, "api_key", "API key")
	if err != nil {
		return nil, err
	}
	web := "https://" + domain + ".freshdesk.com"
	cl := source.NewClient(web, o)
	cl.Auth = func(r *http.Request) { r.SetBasicAuth(key, "X") }
	return &src{c: cl, domain: domain, web: web, companies: map[int64]company{}}, nil
}

// Check signs in as the key's agent and counts what the account has.
func (s *src) Check(ctx context.Context) (source.Scan, error) {
	// An unknown domain answers 404, so a 404 here is no such account.
	acct := *s.c
	acct.NotFound = source.ErrNotFound
	if err := acct.Get(ctx, "/api/v2/agents/me", nil, &struct{}{}); err != nil {
		return source.Scan{}, err
	}
	scan := source.Scan{
		Account: source.Account{Key: s.domain, Name: s.domain, URL: s.web},
		Tickets: -1, Articles: -1,
	}

	// The account's name needs an admin key; without one the domain is the name.
	var a struct {
		Name string `json:"account_name"`
	}
	if err := s.c.Get(ctx, "/api/v2/account", nil, &a); err != nil {
		if fatal(err) {
			return source.Scan{}, err
		}
	} else if n := strings.TrimSpace(a.Name); n != "" {
		scan.Account.Name = n
	}

	var found struct {
		Total int `json:"total"`
	}
	q := url.Values{"query": {`"created_at:>'1970-01-01'"`}}
	if err := s.c.Get(ctx, "/api/v2/search/tickets", q, &found); err != nil {
		if fatal(err) {
			return source.Scan{}, err
		}
	} else {
		scan.Tickets = found.Total
	}

	agents, err := s.agentList(ctx)
	if err != nil {
		return source.Scan{}, err
	}
	scan.Agents = agents

	groups, err := list(ctx, s.c, "/api/v2/groups", nil, func(g fdGroup) int64 { return g.ID })
	if err != nil {
		return source.Scan{}, err
	}
	for _, g := range groups {
		scan.Groups = append(scan.Groups, source.Group{ID: strconv.FormatInt(g.ID, 10), Name: strings.TrimSpace(g.Name)})
	}

	fs, err := s.tree(ctx, checkCalls)
	switch {
	case err == nil:
		scan.HasArticles = true
		scan.Articles = 0
		for _, f := range fs {
			if f.Articles < 0 {
				scan.Articles = -1
				break
			}
			scan.Articles += f.Articles
		}
	case errors.Is(err, errTooBig):
		scan.HasArticles = true
	case fatal(err):
		return source.Scan{}, err
	default:
		scan.Articles = 0
	}
	return scan, nil
}

// fatal says an error must stop the caller: a rate limit, a cancel or the source gone.
func fatal(err error) bool {
	var rl *source.RateLimited
	var un *source.Unreachable
	return errors.As(err, &rl) || errors.As(err, &un) || errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

type fdAgent struct {
	ID      int64 `json:"id"`
	Contact struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"contact"`
}

type fdGroup struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// agentList reads every agent once and keeps them for message authors.
func (s *src) agentList(ctx context.Context) ([]source.Agent, error) {
	raw, err := list(ctx, s.c, "/api/v2/agents", nil, func(a fdAgent) int64 { return a.ID })
	if err != nil {
		return nil, err
	}
	out := make([]source.Agent, 0, len(raw))
	s.agents = make(map[string]source.Agent, len(raw))
	for _, a := range raw {
		ag := source.Agent{
			ID:    strconv.FormatInt(a.ID, 10),
			Name:  strings.TrimSpace(a.Contact.Name),
			Email: strings.TrimSpace(a.Contact.Email),
		}
		s.agents[ag.ID] = ag
		out = append(out, ag)
	}
	return out, nil
}

// list reads every page of a listing. A page that is short or brings nothing new ends it,
// so a listing that ignores paging is read once.
func list[T any](ctx context.Context, c *source.Client, path string, q url.Values, id func(T) int64) ([]T, error) {
	return listed(ctx, c, path, id, func(v *url.Values) error {
		for k, vals := range q {
			(*v)[k] = vals
		}
		return nil
	})
}

// listed is list with a hook run before each page request; an error from it stops the read.
func listed[T any](ctx context.Context, c *source.Client, path string, id func(T) int64, before func(*url.Values) error) ([]T, error) {
	var out []T
	seen := map[int64]bool{}
	for page := 1; page <= maxPage; page++ {
		v := url.Values{"per_page": {strconv.Itoa(perPage)}, "page": {strconv.Itoa(page)}}
		if err := before(&v); err != nil {
			return nil, err
		}
		var items []T
		if err := c.Get(ctx, path, v, &items); err != nil {
			return nil, err
		}
		fresh := 0
		for _, it := range items {
			if k := id(it); !seen[k] {
				seen[k] = true
				out = append(out, it)
				fresh++
			}
		}
		if len(items) < perPage || fresh == 0 {
			break
		}
	}
	return out, nil
}

type fdTicket struct {
	ID          int64    `json:"id"`
	Subject     string   `json:"subject"`
	Status      int      `json:"status"`
	Priority    int      `json:"priority"`
	Source      int      `json:"source"`
	RequesterID int64    `json:"requester_id"`
	ResponderID *int64   `json:"responder_id"`
	GroupID     *int64   `json:"group_id"`
	CompanyID   *int64   `json:"company_id"`
	Tags        []string `json:"tags"`
	Spam        bool     `json:"spam"`
	Deleted     bool     `json:"deleted"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	Requester   *struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Email  string `json:"email"`
		Phone  string `json:"phone"`
		Mobile string `json:"mobile"`
	} `json:"requester"`
	Stats *struct {
		ResolvedAt *string `json:"resolved_at"`
		ClosedAt   *string `json:"closed_at"`
	} `json:"stats"`
	Description     string         `json:"description"`
	DescriptionText string         `json:"description_text"`
	Attachments     []fdAttachment `json:"attachments"`
}

type fdAttachment struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	FileSize    int64  `json:"file_size"`
	URL         string `json:"attachment_url"`
}

// cursor is a position in the ticket listing: tickets updated on or after U, page P.
// Freshdesk serves at most 300 pages of one listing, so the listing moves forward by
// raising updated_since to the last ticket seen rather than by page; P counts up only
// while a whole page shares one second.
type cursor struct {
	U time.Time `json:"u"`
	P int       `json:"p"`
}

func readCursor(raw string) (cursor, error) {
	var c cursor
	if err := json.Unmarshal([]byte(raw), &c); err != nil || c.U.IsZero() || c.P < 1 {
		return cursor{}, fmt.Errorf("freshdesk: malformed ticket cursor")
	}
	return c, nil
}

func (c cursor) String() string {
	b, _ := json.Marshal(cursor{U: c.U.UTC(), P: c.P})
	return string(b)
}

// next is the cursor after a full page from c whose last ticket was updated at last. The
// next listing starts a second before it, so tickets that share its second are listed
// again (the engine dedups) and none is passed over.
func (c cursor) next(last time.Time) cursor {
	u := last.Add(-time.Second)
	if u.After(c.U) {
		return cursor{U: u, P: 1}
	}
	if c.P >= maxPage {
		// More than 30000 tickets updated within one second: move past it.
		return cursor{U: c.U.Add(time.Second), P: 1}
	}
	return cursor{U: c.U, P: c.P + 1}
}

var epoch = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

// Tickets lists tickets by when they were last updated, oldest first. Freshdesk lists
// tickets older than 30 days only with updated_since, and a ticket created on or after
// since was also updated on or after it, so since bounds the listing too.
func (s *src) Tickets(ctx context.Context, raw string, since, updatedSince *time.Time) (source.Page[source.Ticket], error) {
	var cur cursor
	if raw == "" {
		cur = cursor{U: epoch, P: 1}
		for _, t := range []*time.Time{since, updatedSince} {
			if t != nil && t.After(cur.U) {
				cur.U = t.UTC().Truncate(time.Second)
			}
		}
	} else {
		var err error
		if cur, err = readCursor(raw); err != nil {
			return source.Page[source.Ticket]{}, err
		}
	}
	q := url.Values{
		"include":       {"requester,stats"},
		"order_by":      {"updated_at"},
		"order_type":    {"asc"},
		"updated_since": {cur.U.UTC().Format(time.RFC3339)},
		"per_page":      {strconv.Itoa(perPage)},
		"page":          {strconv.Itoa(cur.P)},
	}
	var items []fdTicket
	if err := s.c.Get(ctx, "/api/v2/tickets", q, &items); err != nil {
		return source.Page[source.Ticket]{}, err
	}
	var page source.Page[source.Ticket]
	for _, it := range items {
		t, err := s.ticket(ctx, it)
		if err != nil {
			return source.Page[source.Ticket]{}, err
		}
		if since != nil && t.CreatedAt.Before(*since) {
			continue
		}
		page.Items = append(page.Items, t)
	}
	if len(items) == perPage {
		page.Next = cur.next(source.Time(items[len(items)-1].UpdatedAt)).String()
	}
	return page, nil
}

func (s *src) ticket(ctx context.Context, it fdTicket) (source.Ticket, error) {
	id := strconv.FormatInt(it.ID, 10)
	t := source.Ticket{
		ID:        id,
		Ref:       "#" + id,
		URL:       s.web + "/a/tickets/" + id,
		Subject:   strings.TrimSpace(it.Subject),
		Status:    status(it.Status),
		Skip:      it.Spam || it.Deleted,
		Priority:  priority(it.Priority),
		Channel:   channel(it.Source),
		Tags:      source.Strings(it.Tags),
		CreatedAt: source.Time(it.CreatedAt),
		UpdatedAt: source.Time(it.UpdatedAt),
	}
	if it.ResponderID != nil && *it.ResponderID > 0 {
		t.AssigneeID = strconv.FormatInt(*it.ResponderID, 10)
	}
	if it.GroupID != nil && *it.GroupID > 0 {
		t.GroupID = strconv.FormatInt(*it.GroupID, 10)
	}
	if it.Stats != nil {
		at := it.Stats.ResolvedAt
		if at == nil && (t.Status == "solved" || t.Status == "closed") {
			at = it.Stats.ClosedAt
		}
		if at != nil {
			if v := source.Time(*at); !v.IsZero() {
				t.SolvedAt = &v
			}
		}
	}
	if r := it.Requester; r != nil {
		p := &source.Person{
			ID:    strconv.FormatInt(it.RequesterID, 10),
			Name:  strings.TrimSpace(r.Name),
			Email: strings.TrimSpace(r.Email),
			Phone: strings.TrimSpace(r.Phone),
		}
		if p.Phone == "" {
			p.Phone = strings.TrimSpace(r.Mobile)
		}
		if p.Name != "" || p.Email != "" || p.Phone != "" {
			t.Requester = p
		}
	}
	if t.Requester != nil && it.CompanyID != nil && *it.CompanyID > 0 {
		co, err := s.company(ctx, *it.CompanyID)
		if err != nil {
			return source.Ticket{}, err
		}
		t.Requester.Company, t.Requester.Domain = co.Name, co.Domain
	}
	return t, nil
}

// company reads a company once per source; one that cannot be read is remembered as none.
func (s *src) company(ctx context.Context, id int64) (company, error) {
	if co, ok := s.companies[id]; ok {
		return co, nil
	}
	var raw struct {
		Name    string   `json:"name"`
		Domains []string `json:"domains"`
	}
	var co company
	if err := s.c.Get(ctx, "/api/v2/companies/"+strconv.FormatInt(id, 10), nil, &raw); err != nil {
		if fatal(err) || errors.Is(err, source.ErrAuth) {
			return company{}, err
		}
	} else {
		co.Name = strings.TrimSpace(raw.Name)
		if d := source.Strings(raw.Domains); len(d) > 0 {
			co.Domain = d[0]
		}
	}
	s.companies[id] = co
	return co, nil
}

func status(v int) string {
	switch v {
	case 2:
		return "open"
	case 3:
		return "pending"
	case 4:
		return "solved"
	case 5:
		return "closed"
	}
	return "pending" // custom statuses
}

func priority(v int) string {
	switch v {
	case 1:
		return "low"
	case 3:
		return "high"
	case 4:
		return "urgent"
	}
	return "normal"
}

func channel(v int) string {
	switch v {
	case 2, 9: // portal, feedback widget
		return "portal"
	case 3: // phone
		return "api"
	case 7:
		return "chat"
	}
	return "email" // email, outbound email and the rest
}

// sourceOutbound is a ticket an agent started by email (Outbound Email).
const sourceOutbound = 10

type fdConversation struct {
	ID          int64          `json:"id"`
	Body        string         `json:"body"`
	BodyText    string         `json:"body_text"`
	Incoming    bool           `json:"incoming"`
	Private     bool           `json:"private"`
	UserID      int64          `json:"user_id"`
	FromEmail   string         `json:"from_email"`
	ToEmails    []string       `json:"to_emails"`
	CcEmails    []string       `json:"cc_emails"`
	CreatedAt   string         `json:"created_at"`
	Attachments []fdAttachment `json:"attachments"`
}

// Load reads the ticket's description, its first message, and then its conversations.
func (s *src) Load(ctx context.Context, t *source.Ticket) error {
	id, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("freshdesk: malformed ticket id")
	}
	if s.agents == nil {
		if _, err := s.agentList(ctx); err != nil {
			return err
		}
	}
	path := "/api/v2/tickets/" + strconv.FormatInt(id, 10)
	var q url.Values
	if t.Requester == nil {
		q = url.Values{"include": {"requester"}}
	}
	var view fdTicket
	if err := s.c.Get(ctx, path, q, &view); err != nil {
		return err
	}
	if view.Spam || view.Deleted {
		t.Skip = true
	}
	if t.Requester == nil {
		full, err := s.ticket(ctx, view)
		if err != nil {
			return err
		}
		t.Requester = full.Requester
	}

	var msgs []source.Message
	first := source.Message{
		ID:          "description-" + t.ID,
		HTML:        view.Description,
		Text:        view.DescriptionText,
		CreatedAt:   source.Time(view.CreatedAt),
		Attachments: attachments(view.Attachments),
	}
	if first.CreatedAt.IsZero() {
		first.CreatedAt = t.CreatedAt
	}
	if t.Requester != nil {
		first.Author = *t.Requester
	}
	if view.Source == sourceOutbound {
		first.Agent = true
		first.Author = source.Person{}
		if view.ResponderID != nil {
			if ag, ok := s.agents[strconv.FormatInt(*view.ResponderID, 10)]; ok {
				first.AuthorID, first.Author = ag.ID, source.Person{ID: ag.ID, Name: ag.Name, Email: ag.Email}
			}
		}
	}
	if strings.TrimSpace(first.HTML) != "" || strings.TrimSpace(first.Text) != "" || len(first.Attachments) > 0 {
		msgs = append(msgs, first)
	}

	convs, err := list(ctx, s.c, path+"/conversations", nil, func(c fdConversation) int64 { return c.ID })
	if err != nil {
		return err
	}
	for _, c := range convs {
		msgs = append(msgs, s.message(c, view.RequesterID, t.Requester))
	}
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].CreatedAt.Before(msgs[j].CreatedAt) })
	t.Messages = msgs
	t.Loaded = true
	return nil
}

func (s *src) message(c fdConversation, requesterID int64, requester *source.Person) source.Message {
	m := source.Message{
		ID:          strconv.FormatInt(c.ID, 10),
		Note:        c.Private,
		HTML:        c.Body,
		Text:        c.BodyText,
		To:          source.Strings(c.ToEmails),
		Cc:          source.Strings(c.CcEmails),
		CreatedAt:   source.Time(c.CreatedAt),
		Attachments: attachments(c.Attachments),
	}
	uid := strconv.FormatInt(c.UserID, 10)
	if ag, ok := s.agents[uid]; ok && c.UserID > 0 {
		m.Agent = true
		m.AuthorID = ag.ID
		m.Author = source.Person{ID: ag.ID, Name: ag.Name, Email: ag.Email}
		return m
	}
	// A note is the team's; otherwise an outgoing message is an agent's no longer listed.
	m.Agent = c.Private || !c.Incoming
	if !m.Agent && requester != nil && c.UserID == requesterID {
		m.Author = *requester
		return m
	}
	m.Author = address(c.FromEmail)
	if c.UserID > 0 {
		m.Author.ID = uid
	}
	return m
}

// address reads "Name <email>" or a bare address.
func address(raw string) source.Person {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return source.Person{}
	}
	if a, err := mail.ParseAddress(raw); err == nil {
		return source.Person{Name: strings.TrimSpace(a.Name), Email: a.Address}
	}
	return source.Person{Email: raw}
}

func attachments(in []fdAttachment) []source.Attachment {
	var out []source.Attachment
	for _, a := range in {
		if strings.TrimSpace(a.URL) == "" {
			continue
		}
		size := a.Size
		if size == 0 {
			size = a.FileSize
		}
		out = append(out, source.Attachment{
			ID:          strconv.FormatInt(a.ID, 10),
			Name:        strings.TrimSpace(a.Name),
			ContentType: strings.TrimSpace(a.ContentType),
			Size:        size,
			URL:         a.URL,
		})
	}
	return out
}

type fdCategory struct {
	ID int64 `json:"id"`
}

type fdFolder struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Articles   *int   `json:"articles_count"`
	SubFolders int    `json:"sub_folders_count"`
}

var errTooBig = errors.New("freshdesk: solutions tree too large to count")

// tree reads the solution folders once: every category's folders, each followed by its
// subfolders. It stops with errTooBig after calls requests.
func (s *src) tree(ctx context.Context, calls int) ([]folder, error) {
	if s.folders != nil {
		return s.folders, nil
	}
	spent := 0
	budget := func(*url.Values) error {
		if spent >= calls {
			return errTooBig
		}
		spent++
		return nil
	}
	cats, err := listed(ctx, s.c, "/api/v2/solutions/categories", func(c fdCategory) int64 { return c.ID }, budget)
	if err != nil {
		return nil, err
	}
	out := []folder{}
	seen := map[int64]bool{}
	var walk func(path string, depth int) error
	walk = func(path string, depth int) error {
		fs, err := listed(ctx, s.c, path, func(f fdFolder) int64 { return f.ID }, budget)
		if err != nil {
			return err
		}
		for _, f := range fs {
			if seen[f.ID] {
				continue
			}
			seen[f.ID] = true
			n := -1
			if f.Articles != nil {
				n = *f.Articles
			}
			out = append(out, folder{ID: f.ID, Name: strings.TrimSpace(f.Name), Articles: n})
			if f.SubFolders > 0 && depth < 8 {
				if err := walk("/api/v2/solutions/folders/"+strconv.FormatInt(f.ID, 10)+"/subfolders", depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, c := range cats {
		if err := walk("/api/v2/solutions/categories/"+strconv.FormatInt(c.ID, 10)+"/folders", 0); err != nil {
			return nil, err
		}
	}
	s.folders = out
	return out, nil
}

// articleCursor is a position in the solutions tree: folder F (at index I when the
// cursor was made), page P of its articles.
type articleCursor struct {
	F int64 `json:"f"`
	I int   `json:"i"`
	P int   `json:"p"`
}

type fdArticle struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      int    `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// emptyFolders bounds how many empty folders one call passes before it answers.
const emptyFolders = 25

// Articles lists solution articles folder by folder.
func (s *src) Articles(ctx context.Context, raw string) (source.Page[source.Article], error) {
	fs, err := s.tree(ctx, treeCalls)
	if err != nil {
		return source.Page[source.Article]{}, s.solutionsRefused(ctx, err)
	}
	i, p := 0, 1
	if raw != "" {
		var c articleCursor
		if err := json.Unmarshal([]byte(raw), &c); err != nil || c.P < 1 || c.I < 0 {
			return source.Page[source.Article]{}, fmt.Errorf("freshdesk: malformed article cursor")
		}
		i, p = c.I, c.P
		// The tree is read again after a restart; find the folder where it is now.
		if i >= len(fs) || fs[i].ID != c.F {
			i, p = c.I, 1
			for k, f := range fs {
				if f.ID == c.F {
					i, p = k, c.P
					break
				}
			}
		}
	}
	for empty := 0; i < len(fs); empty++ {
		f := fs[i]
		q := url.Values{"per_page": {strconv.Itoa(perPage)}, "page": {strconv.Itoa(p)}}
		var items []fdArticle
		if err := s.c.Get(ctx, "/api/v2/solutions/folders/"+strconv.FormatInt(f.ID, 10)+"/articles", q, &items); err != nil {
			return source.Page[source.Article]{}, err
		}
		var page source.Page[source.Article]
		for _, a := range items {
			page.Items = append(page.Items, source.Article{
				ID:         strconv.FormatInt(a.ID, 10),
				Title:      strings.TrimSpace(a.Title),
				HTML:       a.Description,
				Collection: f.Name,
				Published:  a.Status == 2,
				CreatedAt:  source.Time(a.CreatedAt),
				UpdatedAt:  source.Time(a.UpdatedAt),
			})
		}
		next := articleCursor{F: f.ID, I: i, P: p + 1}
		if len(items) < perPage || p >= maxPage {
			i, p = i+1, 1
			next = articleCursor{I: i, P: 1}
			if i < len(fs) {
				next.F = fs[i].ID
			}
		}
		if i < len(fs) {
			b, _ := json.Marshal(next)
			page.Next = string(b)
		}
		if len(page.Items) > 0 || page.Next == "" || empty >= emptyFolders {
			return page, nil
		}
	}
	return source.Page[source.Article]{}, nil
}

// solutionsRefused tells an agent who may not read solutions (ErrUnsupported) from a key
// the account refuses (ErrAuth).
func (s *src) solutionsRefused(ctx context.Context, err error) error {
	var re *source.RemoteError
	if !errors.Is(err, source.ErrAuth) && !(errors.As(err, &re) && re.Status == http.StatusNotFound) {
		return err
	}
	if perr := s.c.Get(ctx, "/api/v2/agents/me", nil, &struct{}{}); perr != nil {
		return perr
	}
	return source.ErrUnsupported
}

// Download opens an attachment. Freshdesk's attachment links are signed storage links;
// one storage refuses has expired, which says nothing about the credentials.
func (s *src) Download(ctx context.Context, a source.Attachment) (io.ReadCloser, error) {
	rc, err := s.c.Open(ctx, a.URL)
	if errors.Is(err, source.ErrAuth) && !s.apiHost(a.URL) {
		return nil, &source.RemoteError{Status: http.StatusForbidden, Detail: "the attachment link was refused or has expired"}
	}
	return rc, err
}

func (s *src) apiHost(raw string) bool {
	u, err := url.Parse(raw)
	b, berr := url.Parse(s.c.Base)
	return err == nil && berr == nil && strings.EqualFold(u.Host, b.Host)
}
