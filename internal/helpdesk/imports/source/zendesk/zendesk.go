// Package zendesk reads a Zendesk Support account and its help center (Guide) through the
// Zendesk API (contracts/imports.md). It signs in with an admin's email and API token.
package zendesk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// maxPeople bounds the agents and groups Check lists.
	maxPeople = 1000
	// maxCommentPages bounds one ticket's comments (100 per page).
	maxCommentPages = 200
	// maxCached bounds the users and organizations kept between pages.
	maxCached = 50000
)

type zendesk struct {
	c   *source.Client
	sub string
	web string // https://{sub}.zendesk.com, also when tests point the API elsewhere

	users    map[int64]user
	orgs     map[int64]org
	sections map[int64]string // nil until Articles first loads them
}

// New builds a Zendesk source. It checks the credentials' shape only; Check signs in.
func New(c source.Credentials, o source.Options) (source.Source, error) {
	sub, err := source.Subdomain(c["subdomain"], ".zendesk.com", "subdomain")
	if err != nil {
		return nil, err
	}
	email, err := source.Required(c, "email", "email address")
	if err != nil {
		return nil, err
	}
	if at := strings.LastIndex(email, "@"); at < 1 || at == len(email)-1 || strings.ContainsAny(email, " \t\r\n/") {
		return nil, source.Invalid("email", "Enter the email address of a Zendesk admin.")
	}
	token, err := source.Required(c, "token", "API token")
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(token, " \t\r\n") {
		return nil, source.Invalid("token", "The API token has no spaces; paste it again.")
	}
	web := "https://" + sub + ".zendesk.com"
	cl := source.NewClient(web, o)
	cl.Auth = func(r *http.Request) { r.SetBasicAuth(email+"/token", token) }
	return &zendesk{c: cl, sub: sub, web: web, users: map[int64]user{}, orgs: map[int64]org{}}, nil
}

type user struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	Role           string `json:"role"`
	OrganizationID int64  `json:"organization_id"`
}

func (u user) agent() bool { return u.Role == "agent" || u.Role == "admin" }

type org struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	DomainNames []string `json:"domain_names"`
}

// cursorMeta is the cursor pagination of list endpoints (page[size], page[after]).
type cursorMeta struct {
	HasMore     bool   `json:"has_more"`
	AfterCursor string `json:"after_cursor"`
}

func (m cursorMeta) next() string {
	if !m.HasMore {
		return ""
	}
	return m.AfterCursor
}

// optional says an error on a call Check can do without: the endpoint is missing or
// refused. Rate limits, an unreachable source and cancellation still stop it.
func optional(err error) bool {
	var re *source.RemoteError
	return errors.As(err, &re) || errors.Is(err, source.ErrAuth)
}

func notFound(err error) bool {
	var re *source.RemoteError
	return errors.As(err, &re) && re.Status == http.StatusNotFound
}

func (z *zendesk) Check(ctx context.Context) (source.Scan, error) {
	scan := source.Scan{Tickets: -1, Articles: -1}

	var me struct {
		User *user `json:"user"`
	}
	z.c.NotFound = source.ErrNotFound
	err := z.c.Get(ctx, "/api/v2/users/me.json", nil, &me)
	z.c.NotFound = nil
	if err != nil {
		return scan, err
	}
	// Zendesk answers an unauthenticated request with an anonymous user, no id.
	if me.User == nil || me.User.ID == 0 {
		return scan, source.ErrAuth
	}
	// The incremental ticket export is for admins only.
	if me.User.Role != "admin" {
		return scan, fmt.Errorf("%w: sign in as a Zendesk admin", source.ErrAuth)
	}

	scan.Account = source.Account{Key: z.sub, Name: z.sub, URL: z.web}
	var brands struct {
		Brands []struct {
			Name    string `json:"name"`
			Default bool   `json:"default"`
		} `json:"brands"`
	}
	if err := z.c.Get(ctx, "/api/v2/brands.json", nil, &brands); err != nil {
		if !optional(err) {
			return scan, err
		}
	} else {
		for i, b := range brands.Brands {
			if name := strings.TrimSpace(b.Name); name != "" && (b.Default || i == 0) {
				scan.Account.Name = name
			}
			if b.Default {
				break
			}
		}
	}

	var count struct {
		Count struct {
			Value int `json:"value"`
		} `json:"count"`
	}
	if err := z.c.Get(ctx, "/api/v2/tickets/count.json", nil, &count); err != nil {
		if !optional(err) {
			return scan, err
		}
	} else {
		scan.Tickets = count.Count.Value
	}

	// Offset pagination answers a count; a help center that is off answers 404 or 403.
	var arts struct {
		Count int `json:"count"`
	}
	if err := z.c.Get(ctx, "/api/v2/help_center/articles.json", url.Values{"per_page": {"1"}}, &arts); err != nil {
		if !optional(err) {
			return scan, err
		}
		scan.Articles = 0
	} else {
		scan.HasArticles = true
		scan.Articles = arts.Count
	}

	q := url.Values{"role[]": {"agent", "admin"}, "page[size]": {"100"}}
	for len(scan.Agents) < maxPeople {
		var out struct {
			Users []user     `json:"users"`
			Meta  cursorMeta `json:"meta"`
		}
		if err := z.c.Get(ctx, "/api/v2/users.json", q, &out); err != nil {
			return scan, err
		}
		for _, u := range out.Users {
			z.remember(u)
			if len(scan.Agents) < maxPeople {
				scan.Agents = append(scan.Agents, source.Agent{ID: id(u.ID), Name: u.Name, Email: u.Email})
			}
		}
		next := out.Meta.next()
		if next == "" {
			break
		}
		q.Set("page[after]", next)
	}

	q = url.Values{"page[size]": {"100"}}
	for len(scan.Groups) < maxPeople {
		var out struct {
			Groups []struct {
				ID      int64  `json:"id"`
				Name    string `json:"name"`
				Deleted bool   `json:"deleted"`
			} `json:"groups"`
			Meta cursorMeta `json:"meta"`
		}
		if err := z.c.Get(ctx, "/api/v2/groups.json", q, &out); err != nil {
			return scan, err
		}
		for _, g := range out.Groups {
			if !g.Deleted && len(scan.Groups) < maxPeople {
				scan.Groups = append(scan.Groups, source.Group{ID: id(g.ID), Name: g.Name})
			}
		}
		next := out.Meta.next()
		if next == "" {
			break
		}
		q.Set("page[after]", next)
	}
	return scan, nil
}

type via struct {
	Channel string          `json:"channel"`
	Source  json.RawMessage `json:"source"`
}

type ticket struct {
	ID             int64           `json:"id"`
	Subject        string          `json:"subject"`
	RawSubject     string          `json:"raw_subject"`
	Status         string          `json:"status"`
	Priority       string          `json:"priority"`
	RequesterID    int64           `json:"requester_id"`
	AssigneeID     int64           `json:"assignee_id"`
	GroupID        int64           `json:"group_id"`
	OrganizationID int64           `json:"organization_id"`
	Tags           []string        `json:"tags"`
	Via            via             `json:"via"`
	Satisfaction   json.RawMessage `json:"satisfaction_rating"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

type exportPage struct {
	Tickets       []ticket `json:"tickets"`
	Users         []user   `json:"users"`
	Organizations []org    `json:"organizations"`
	MetricSets    []struct {
		TicketID int64  `json:"ticket_id"`
		SolvedAt string `json:"solved_at"`
	} `json:"metric_sets"`
	AfterCursor string `json:"after_cursor"`
	EndOfStream bool   `json:"end_of_stream"`
}

// startTime is where the export starts: the export answers nothing newer than a minute ago.
func startTime(since, updatedSince *time.Time) int64 {
	var t time.Time
	switch {
	case updatedSince != nil:
		t = *updatedSince
	case since != nil:
		t = *since
	}
	if t.IsZero() || t.Unix() <= 0 {
		return 0
	}
	if latest := time.Now().Add(-61 * time.Second); t.After(latest) {
		t = latest
	}
	return t.Unix()
}

func (z *zendesk) Tickets(ctx context.Context, cursor string, since, updatedSince *time.Time) (source.Page[source.Ticket], error) {
	var page source.Page[source.Ticket]
	q := url.Values{"per_page": {"100"}, "include": {"users,organizations,metric_sets"}}
	if cursor != "" {
		q.Set("cursor", cursor)
	} else {
		q.Set("start_time", strconv.FormatInt(startTime(since, updatedSince), 10))
	}
	var out exportPage
	if err := z.c.Get(ctx, "/api/v2/incremental/tickets/cursor.json", q, &out); err != nil {
		return page, err
	}
	for _, u := range out.Users {
		z.remember(u)
	}
	for _, o := range out.Organizations {
		z.rememberOrg(o)
	}
	solved := map[int64]time.Time{}
	for _, m := range out.MetricSets {
		if t := source.Time(m.SolvedAt); !t.IsZero() {
			solved[m.TicketID] = t
		}
	}

	var keep []ticket
	var userIDs, orgIDs []int64
	for _, t := range out.Tickets {
		created, updated := source.Time(t.CreatedAt), source.Time(t.UpdatedAt)
		if since != nil && created.Before(*since) {
			continue
		}
		if updatedSince != nil && updated.Before(*updatedSince) {
			continue
		}
		keep = append(keep, t)
		if t.Status != "deleted" {
			userIDs = append(userIDs, t.RequesterID)
		}
	}
	if err := z.loadUsers(ctx, userIDs); err != nil {
		return page, err
	}
	for _, t := range keep {
		if t.Status == "deleted" {
			continue
		}
		if t.OrganizationID != 0 {
			orgIDs = append(orgIDs, t.OrganizationID)
		} else if u, ok := z.users[t.RequesterID]; ok {
			orgIDs = append(orgIDs, u.OrganizationID)
		}
	}
	if err := z.loadOrgs(ctx, orgIDs); err != nil {
		return page, err
	}

	for _, t := range keep {
		page.Items = append(page.Items, z.ticket(t, solved))
	}
	if !out.EndOfStream {
		page.Next = out.AfterCursor
	}
	return page, nil
}

func (z *zendesk) ticket(t ticket, solved map[int64]time.Time) source.Ticket {
	out := source.Ticket{
		ID:        id(t.ID),
		Ref:       "#" + id(t.ID),
		URL:       z.web + "/agent/tickets/" + id(t.ID),
		Subject:   strings.TrimSpace(t.Subject),
		Priority:  priority(t.Priority),
		Tags:      source.Strings(t.Tags),
		CSAT:      csat(t.Satisfaction),
		CreatedAt: source.Time(t.CreatedAt),
		UpdatedAt: source.Time(t.UpdatedAt),
	}
	if out.Subject == "" {
		out.Subject = strings.TrimSpace(t.RawSubject)
	}
	out.Status, out.Skip = status(t.Status)
	if t.AssigneeID != 0 {
		out.AssigneeID = id(t.AssigneeID)
	}
	if t.GroupID != 0 {
		out.GroupID = id(t.GroupID)
	}
	if s, ok := solved[t.ID]; ok && (out.Status == "solved" || out.Status == "closed") {
		out.SolvedAt = &s
	}
	if t.RequesterID != 0 {
		p := &source.Person{ID: id(t.RequesterID)}
		orgID := t.OrganizationID
		if u, ok := z.users[t.RequesterID]; ok {
			p.Name, p.Email, p.Phone = u.Name, u.Email, u.Phone
			if orgID == 0 {
				orgID = u.OrganizationID
			}
		}
		if o, ok := z.orgs[orgID]; ok && orgID != 0 {
			p.Company = o.Name
			if d := source.Strings(o.DomainNames); len(d) > 0 {
				p.Domain = d[0]
			}
		}
		out.Requester = p
	}
	out.Channel = channel(t.Via.Channel, out.Requester)
	return out
}

func status(s string) (string, bool) {
	switch s {
	case "new", "open":
		return "open", false
	case "pending":
		return "pending", false
	case "hold":
		return "on_hold", false
	case "solved":
		return "solved", false
	case "closed":
		return "closed", false
	case "deleted":
		return "closed", true
	}
	return "open", false
}

func priority(p string) string {
	switch p {
	case "low", "high", "urgent":
		return p
	}
	return "normal"
}

func channel(c string, requester *source.Person) string {
	switch strings.ToLower(c) {
	case "email":
		return "email"
	case "chat", "chat_transcript", "messaging", "native_messaging":
		return "chat"
	case "api":
		return "api"
	case "web", "web_form", "web_widget", "help_center", "helpcenter":
		return "portal"
	}
	if requester != nil && requester.Email != "" {
		return "email"
	}
	return "api"
}

// csat reads satisfaction_rating: an object whose score is good, bad, offered or unoffered.
func csat(raw json.RawMessage) int {
	var r struct {
		Score string `json:"score"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &r) != nil {
		return 0
	}
	switch r.Score {
	case "good":
		return 5
	case "bad":
		return 1
	}
	return 0
}

type attachment struct {
	ID          int64  `json:"id"`
	FileName    string `json:"file_name"`
	ContentURL  string `json:"content_url"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Inline      bool   `json:"inline"`
	Deleted     bool   `json:"deleted"`
}

type comment struct {
	ID          int64        `json:"id"`
	Body        string       `json:"body"`
	HTMLBody    string       `json:"html_body"`
	Public      bool         `json:"public"`
	AuthorID    int64        `json:"author_id"`
	Attachments []attachment `json:"attachments"`
	Via         via          `json:"via"`
	CreatedAt   string       `json:"created_at"`
}

func (z *zendesk) Load(ctx context.Context, t *source.Ticket) error {
	tid, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil || tid <= 0 {
		return fmt.Errorf("zendesk ticket id %q is not a number", t.ID)
	}
	path := "/api/v2/tickets/" + strconv.FormatInt(tid, 10) + "/comments.json"
	q := url.Values{"include": {"users"}, "include_inline_images": {"true"}, "page[size]": {"100"}}
	var comments []comment
	for pages := 0; ; pages++ {
		if pages == maxCommentPages {
			return fmt.Errorf("ticket #%d has more than %d comments", tid, maxCommentPages*100)
		}
		var out struct {
			Comments []comment  `json:"comments"`
			Users    []user     `json:"users"`
			Meta     cursorMeta `json:"meta"`
		}
		if err := z.c.Get(ctx, path, q, &out); err != nil {
			return err
		}
		for _, u := range out.Users {
			z.remember(u)
		}
		comments = append(comments, out.Comments...)
		next := out.Meta.next()
		if next == "" {
			break
		}
		q.Set("page[after]", next)
	}

	var missing []int64
	for _, c := range comments {
		if _, ok := z.users[c.AuthorID]; !ok && c.AuthorID > 0 {
			missing = append(missing, c.AuthorID)
		}
	}
	if err := z.loadUsers(ctx, missing); err != nil {
		return err
	}

	msgs := make([]source.Message, 0, len(comments))
	for _, c := range comments {
		u := z.users[c.AuthorID]
		m := source.Message{
			ID:        id(c.ID),
			Agent:     u.agent(),
			Note:      !c.Public,
			Author:    source.Person{Name: u.Name, Email: u.Email, Phone: u.Phone},
			HTML:      strings.TrimSpace(c.HTMLBody),
			CreatedAt: source.Time(c.CreatedAt),
		}
		if c.AuthorID > 0 {
			m.Author.ID = id(c.AuthorID)
			if m.Agent {
				m.AuthorID = id(c.AuthorID)
			}
		}
		if m.HTML == "" {
			m.Text = c.Body
		}
		if to := recipient(c.Via); to != "" {
			m.To = []string{to}
		}
		for _, a := range c.Attachments {
			if a.Deleted || a.ContentURL == "" {
				continue
			}
			m.Attachments = append(m.Attachments, source.Attachment{
				ID: id(a.ID), Name: a.FileName, ContentType: a.ContentType,
				Size: a.Size, URL: a.ContentURL, Inline: a.Inline,
			})
		}
		msgs = append(msgs, m)
	}
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].CreatedAt.Before(msgs[j].CreatedAt) })
	t.Messages = msgs
	t.Loaded = true
	return nil
}

// recipient is the address an email comment was written to (via.source.to.address).
func recipient(v via) string {
	if strings.ToLower(v.Channel) != "email" || len(v.Source) == 0 {
		return ""
	}
	var s struct {
		To struct {
			Address string `json:"address"`
		} `json:"to"`
	}
	if json.Unmarshal(v.Source, &s) != nil {
		return ""
	}
	return strings.TrimSpace(s.To.Address)
}

func (z *zendesk) Articles(ctx context.Context, cursor string) (source.Page[source.Article], error) {
	var page source.Page[source.Article]
	if z.sections == nil {
		if err := z.loadSections(ctx); err != nil {
			return page, err
		}
	}
	q := url.Values{"page[size]": {"100"}}
	if cursor != "" {
		q.Set("page[after]", cursor)
	}
	var out struct {
		Articles []struct {
			ID        int64  `json:"id"`
			Title     string `json:"title"`
			Body      string `json:"body"`
			Draft     bool   `json:"draft"`
			SectionID int64  `json:"section_id"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
		} `json:"articles"`
		Meta cursorMeta `json:"meta"`
	}
	if err := z.c.Get(ctx, "/api/v2/help_center/articles.json", q, &out); err != nil {
		if notFound(err) {
			return page, source.ErrUnsupported
		}
		return page, err
	}
	for _, a := range out.Articles {
		page.Items = append(page.Items, source.Article{
			ID: id(a.ID), Title: strings.TrimSpace(a.Title), HTML: a.Body,
			Collection: z.sections[a.SectionID], Published: !a.Draft,
			CreatedAt: source.Time(a.CreatedAt), UpdatedAt: source.Time(a.UpdatedAt),
		})
	}
	page.Next = out.Meta.next()
	return page, nil
}

func (z *zendesk) loadSections(ctx context.Context) error {
	sections := map[int64]string{}
	q := url.Values{"page[size]": {"100"}}
	for pages := 0; pages < 100; pages++ {
		var out struct {
			Sections []struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"sections"`
			Meta cursorMeta `json:"meta"`
		}
		if err := z.c.Get(ctx, "/api/v2/help_center/sections.json", q, &out); err != nil {
			if notFound(err) {
				return source.ErrUnsupported
			}
			return err
		}
		for _, s := range out.Sections {
			sections[s.ID] = strings.TrimSpace(s.Name)
		}
		next := out.Meta.next()
		if next == "" {
			break
		}
		q.Set("page[after]", next)
	}
	z.sections = sections
	return nil
}

func (z *zendesk) Download(ctx context.Context, a source.Attachment) (io.ReadCloser, error) {
	if a.URL == "" {
		return nil, errors.New("attachment has no link")
	}
	return z.c.Open(ctx, a.URL)
}

func (z *zendesk) remember(u user) {
	if u.ID == 0 {
		return
	}
	if len(z.users) >= maxCached {
		z.users = map[int64]user{}
	}
	z.users[u.ID] = u
}

func (z *zendesk) rememberOrg(o org) {
	if o.ID == 0 {
		return
	}
	if len(z.orgs) >= maxCached {
		z.orgs = map[int64]org{}
	}
	z.orgs[o.ID] = o
}

// loadUsers fetches users the sideloads left out, 100 at a time.
func (z *zendesk) loadUsers(ctx context.Context, ids []int64) error {
	for _, chunk := range chunks(ids, func(i int64) bool { _, ok := z.users[i]; return ok }) {
		var out struct {
			Users []user `json:"users"`
		}
		if err := z.c.Get(ctx, "/api/v2/users/show_many.json", url.Values{"ids": {chunk}}, &out); err != nil {
			return err
		}
		for _, u := range out.Users {
			z.remember(u)
		}
	}
	return nil
}

// loadOrgs fetches organizations the sideloads left out, 100 at a time.
func (z *zendesk) loadOrgs(ctx context.Context, ids []int64) error {
	for _, chunk := range chunks(ids, func(i int64) bool { _, ok := z.orgs[i]; return ok }) {
		var out struct {
			Organizations []org `json:"organizations"`
		}
		if err := z.c.Get(ctx, "/api/v2/organizations/show_many.json", url.Values{"ids": {chunk}}, &out); err != nil {
			return err
		}
		for _, o := range out.Organizations {
			z.rememberOrg(o)
		}
	}
	return nil
}

// chunks joins the distinct ids not yet known into comma lists of at most 100.
func chunks(ids []int64, known func(int64) bool) []string {
	seen := map[int64]bool{}
	var out []string
	var cur []string
	for _, i := range ids {
		if i <= 0 || seen[i] || known(i) {
			continue
		}
		seen[i] = true
		cur = append(cur, strconv.FormatInt(i, 10))
		if len(cur) == 100 {
			out = append(out, strings.Join(cur, ","))
			cur = nil
		}
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, ","))
	}
	return out
}

func id(i int64) string { return strconv.FormatInt(i, 10) }
