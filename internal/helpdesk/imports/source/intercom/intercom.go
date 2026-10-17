// Package intercom reads an Intercom workspace's conversations and help center articles
// through the Intercom REST API (contracts/imports.md). It signs in with an access token
// from the workspace's Developer Hub app.
package intercom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/helpdesk/imports/source"
)

// version is the Intercom API version every request asks for.
const version = "2.14"

const (
	// perPage is the size of a conversations or articles page.
	perPage = 50
	// maxCached bounds the contacts kept between pages.
	maxCached = 50000
	// maxCollectionPages bounds the help center collections listing (150 per page).
	maxCollectionPages = 100
	// subjectLen is the length of a subject taken from the first message.
	subjectLen = 80
)

// regions are the API and web app hosts of each Intercom data region.
var regions = map[string]struct{ api, web string }{
	"us": {"https://api.intercom.io", "https://app.intercom.com"},
	"eu": {"https://api.eu.intercom.io", "https://app.eu.intercom.com"},
	"au": {"https://api.au.intercom.io", "https://app.au.intercom.com"},
}

type intercom struct {
	c   *source.Client
	web string // the region's web app, https

	app         *app // nil until /me answers
	contacts    map[string]*source.Person
	collections map[string]string // nil until Articles first loads them
}

// New builds an Intercom source. It checks the credentials' shape only; Check signs in.
func New(c source.Credentials, o source.Options) (source.Source, error) {
	token, err := source.Required(c, "token", "access token")
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(token, " \t\r\n") {
		return nil, source.Invalid("token", "The access token has no spaces; paste it again.")
	}
	region := strings.ToLower(strings.TrimSpace(c["region"]))
	if region == "" {
		region = "us"
	}
	r, ok := regions[region]
	if !ok {
		return nil, source.Invalid("region", "Choose the region your workspace is hosted in: us, eu or au.")
	}
	cl := source.NewClient(r.api, o)
	cl.Auth = func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+token) }
	cl.Header = http.Header{"Intercom-Version": {version}}
	hc := *cl.HTTP
	hc.Transport = resetAfter{next: hc.Transport}
	cl.HTTP = &hc
	return &intercom{c: cl, web: r.web, contacts: map[string]*source.Person{}}, nil
}

// resetAfter turns Intercom's X-RateLimit-Reset (a unix time) on a 429 into the
// Retry-After that source.Status reads; without it Retry-After stands.
type resetAfter struct{ next http.RoundTripper }

func (t resetAfter) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}
	resp, err := next.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusTooManyRequests {
		return resp, err
	}
	if reset, err := strconv.ParseInt(strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")), 10, 64); err == nil && reset > 0 {
		wait := time.Until(time.Unix(reset, 0))
		secs := int64((wait + time.Second - 1) / time.Second)
		if secs < 1 {
			secs = 1
		}
		resp.Header.Set("Retry-After", strconv.FormatInt(secs, 10))
	}
	return resp, nil
}

// id is an Intercom id, sent as a string or a number depending on the field.
type id string

func (v *id) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*v = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*v = id(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*v = id(n.String())
	return nil
}

type app struct {
	IDCode string `json:"id_code"`
	Name   string `json:"name"`
}

type admin struct {
	ID    id     `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// optional says an error on a call Check can do without: the endpoint is missing or the
// token's scopes leave it out. Rate limits, an unreachable source and cancellation still
// stop it.
func optional(err error) bool {
	var re *source.RemoteError
	return errors.As(err, &re) || errors.Is(err, source.ErrAuth)
}

func notFound(err error) bool {
	var re *source.RemoteError
	return errors.As(err, &re) && re.Status == http.StatusNotFound
}

// me signs in and learns the workspace (app) the token belongs to.
func (s *intercom) me(ctx context.Context) (*app, error) {
	if s.app != nil {
		return s.app, nil
	}
	var me struct {
		App *app `json:"app"`
	}
	if err := s.c.Get(ctx, "/me", nil, &me); err != nil {
		return nil, err
	}
	if me.App == nil || strings.TrimSpace(me.App.IDCode) == "" {
		return nil, source.ErrAuth
	}
	s.app = me.App
	return s.app, nil
}

func (s *intercom) Check(ctx context.Context) (source.Scan, error) {
	scan := source.Scan{Tickets: -1, Articles: -1}
	a, err := s.me(ctx)
	if err != nil {
		return scan, err
	}
	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = a.IDCode
	}
	scan.Account = source.Account{Key: a.IDCode, Name: name, URL: s.web + "/a/apps/" + url.PathEscape(a.IDCode)}

	var found searchAnswer
	err = s.c.Do(ctx, http.MethodPost, "/conversations/search", nil, searchBody(nil, nil, "", 1), &found)
	switch {
	case err == nil:
		scan.Tickets = found.TotalCount
	case !optional(err):
		return scan, err
	}

	var admins struct {
		Admins []admin `json:"admins"`
	}
	if err := s.c.Get(ctx, "/admins", nil, &admins); err != nil && !optional(err) {
		return scan, err
	}
	for _, ad := range admins.Admins {
		email := strings.TrimSpace(ad.Email)
		if ad.ID == "" || email == "" {
			continue
		}
		scan.Agents = append(scan.Agents, source.Agent{ID: string(ad.ID), Name: strings.TrimSpace(ad.Name), Email: email})
	}

	var teams struct {
		Teams []struct {
			ID   id     `json:"id"`
			Name string `json:"name"`
		} `json:"teams"`
	}
	if err := s.c.Get(ctx, "/teams", nil, &teams); err != nil && !optional(err) {
		return scan, err
	}
	for _, t := range teams.Teams {
		if t.ID != "" {
			scan.Groups = append(scan.Groups, source.Group{ID: string(t.ID), Name: strings.TrimSpace(t.Name)})
		}
	}

	var arts articleList
	err = s.c.Get(ctx, "/articles", url.Values{"per_page": {"1"}}, &arts)
	switch {
	case err == nil:
		scan.HasArticles = true
		scan.Articles = arts.TotalCount
	case !optional(err):
		return scan, err
	}
	return scan, nil
}

// Tickets

type filter struct {
	Field    string `json:"field,omitempty"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

type searchRequest struct {
	Query filter `json:"query"`
	Sort  struct {
		Field string `json:"field"`
		Order string `json:"order"`
	} `json:"sort"`
	Pagination struct {
		PerPage       int    `json:"per_page"`
		StartingAfter string `json:"starting_after,omitempty"`
	} `json:"pagination"`
}

// searchBody asks for conversations oldest first. Intercom's ">" on a date is "on or
// after"; with no bound the query matches every conversation.
func searchBody(since, updatedSince *time.Time, cursor string, n int) searchRequest {
	var fs []filter
	if since != nil {
		fs = append(fs, filter{Field: "created_at", Operator: ">", Value: strconv.FormatInt(since.Unix(), 10)})
	}
	if updatedSince != nil {
		fs = append(fs, filter{Field: "updated_at", Operator: ">", Value: strconv.FormatInt(updatedSince.Unix(), 10)})
	}
	var r searchRequest
	switch len(fs) {
	case 0:
		r.Query = filter{Field: "created_at", Operator: ">", Value: "0"}
	case 1:
		r.Query = fs[0]
	default:
		r.Query = filter{Operator: "AND", Value: fs}
	}
	r.Sort.Field, r.Sort.Order = "created_at", "ascending"
	r.Pagination.PerPage = n
	r.Pagination.StartingAfter = cursor
	return r
}

type searchAnswer struct {
	TotalCount    int            `json:"total_count"`
	Conversations []conversation `json:"conversations"`
	Pages         struct {
		Next *struct {
			StartingAfter string `json:"starting_after"`
		} `json:"next"`
	} `json:"pages"`
}

type author struct {
	Type        string `json:"type"`
	ID          id     `json:"id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	FromAIAgent bool   `json:"from_ai_agent"`
}

type attachment struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Filesize    int64  `json:"filesize"`
}

type part struct {
	ID          id           `json:"id"`
	PartType    string       `json:"part_type"`
	Body        string       `json:"body"`
	CreatedAt   int64        `json:"created_at"`
	Author      author       `json:"author"`
	Attachments []attachment `json:"attachments"`
}

type conversation struct {
	ID        id     `json:"id"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	State     string `json:"state"`
	Priority  string `json:"priority"`
	Source    struct {
		Type        string       `json:"type"`
		ID          id           `json:"id"`
		DeliveredAs string       `json:"delivered_as"`
		Subject     string       `json:"subject"`
		Body        string       `json:"body"`
		Author      author       `json:"author"`
		Attachments []attachment `json:"attachments"`
	} `json:"source"`
	Contacts struct {
		Contacts []struct {
			ID id `json:"id"`
		} `json:"contacts"`
	} `json:"contacts"`
	AdminAssigneeID id `json:"admin_assignee_id"`
	TeamAssigneeID  id `json:"team_assignee_id"`
	Tags            struct {
		Tags []struct {
			Name string `json:"name"`
		} `json:"tags"`
	} `json:"tags"`
	Rating *struct {
		Rating int `json:"rating"`
	} `json:"conversation_rating"`
	Statistics *struct {
		LastCloseAt int64 `json:"last_close_at"`
	} `json:"statistics"`
	// Parts come only with a single conversation, at most 500 of them: Intercom keeps
	// the rest out of the API.
	Parts struct {
		Parts []part `json:"conversation_parts"`
	} `json:"conversation_parts"`
}

func (s *intercom) Tickets(ctx context.Context, cursor string, since, updatedSince *time.Time) (source.Page[source.Ticket], error) {
	var page source.Page[source.Ticket]
	a, err := s.me(ctx)
	if err != nil {
		return page, err
	}
	var found searchAnswer
	if err := s.c.Do(ctx, http.MethodPost, "/conversations/search", nil, searchBody(since, updatedSince, cursor, perPage), &found); err != nil {
		return page, err
	}
	for _, cv := range found.Conversations {
		t, err := s.ticket(ctx, a, cv)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, t)
	}
	if found.Pages.Next != nil && found.Pages.Next.StartingAfter != "" && found.Pages.Next.StartingAfter != cursor && len(found.Conversations) > 0 {
		page.Next = found.Pages.Next.StartingAfter
	}
	return page, nil
}

var statuses = map[string]string{"open": "open", "snoozed": "on_hold", "closed": "closed"}

func (s *intercom) ticket(ctx context.Context, a *app, cv conversation) (source.Ticket, error) {
	cid := string(cv.ID)
	t := source.Ticket{
		ID:         cid,
		Ref:        "#" + cid,
		URL:        s.web + "/a/inbox/" + url.PathEscape(a.IDCode) + "/inbox/shared/all/conversation/" + url.PathEscape(cid),
		Subject:    subject(cv),
		Status:     statuses[cv.State],
		Priority:   "normal",
		Channel:    channel(cv.Source.Type, cv.Source.DeliveredAs),
		AssigneeID: string(cv.AdminAssigneeID),
		GroupID:    string(cv.TeamAssigneeID),
		CreatedAt:  source.Unix(cv.CreatedAt),
		UpdatedAt:  source.Unix(cv.UpdatedAt),
	}
	if t.Status == "" {
		t.Status = "open"
	}
	if cv.Priority == "priority" {
		t.Priority = "high"
	}
	if t.AssigneeID == "0" {
		t.AssigneeID = ""
	}
	if t.GroupID == "0" {
		t.GroupID = ""
	}
	for _, tag := range cv.Tags.Tags {
		t.Tags = append(t.Tags, tag.Name)
	}
	t.Tags = source.Strings(t.Tags)
	if cv.Rating != nil && cv.Rating.Rating >= 1 && cv.Rating.Rating <= 5 {
		t.CSAT = cv.Rating.Rating
	}
	if t.Status == "closed" && cv.Statistics != nil && cv.Statistics.LastCloseAt > 0 {
		at := source.Unix(cv.Statistics.LastCloseAt)
		t.SolvedAt = &at
	}
	req, err := s.requester(ctx, cv)
	if err != nil {
		return t, err
	}
	t.Requester = req
	return t, nil
}

// subject is the conversation's title, else its email subject, else the start of its
// first message.
func subject(cv conversation) string {
	for _, raw := range []string{cv.Title, cv.Source.Subject} {
		if s := strings.Join(strings.Fields(hd.HTMLToText(raw)), " "); s != "" {
			return s
		}
	}
	return hd.Excerpt(hd.HTMLToText(cv.Source.Body), subjectLen)
}

// channel is how the conversation arrived: an email, a message an admin started (an
// outbound message, a ticket made for the customer), or a chat in the Messenger or a
// messaging app.
func channel(typ, deliveredAs string) string {
	switch {
	case typ == "email":
		return "email"
	case deliveredAs == "admin_initiated" || typ == "api":
		return "api"
	}
	return "chat"
}

func customer(a author) bool {
	switch a.Type {
	case "user", "lead", "contact":
		return true
	}
	return false
}

// requester is the conversation's first contact, read once per contact; a contact that
// is gone falls back to the conversation's customer author.
func (s *intercom) requester(ctx context.Context, cv conversation) (*source.Person, error) {
	if len(cv.Contacts.Contacts) > 0 && cv.Contacts.Contacts[0].ID != "" {
		p, err := s.contact(ctx, string(cv.Contacts.Contacts[0].ID))
		if err != nil {
			return nil, err
		}
		if p != nil {
			return p, nil
		}
	}
	if au := cv.Source.Author; customer(au) && (au.Email != "" || au.Name != "") {
		return &source.Person{ID: string(au.ID), Email: strings.TrimSpace(au.Email), Name: strings.TrimSpace(au.Name)}, nil
	}
	return nil, nil
}

type contact struct {
	ID        id     `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	Companies struct {
		TotalCount int `json:"total_count"`
	} `json:"companies"`
}

// contact reads a contact and its first company; nil when the contact is gone.
func (s *intercom) contact(ctx context.Context, cid string) (*source.Person, error) {
	if p, ok := s.contacts[cid]; ok {
		return p, nil
	}
	var c contact
	err := s.c.Get(ctx, "/contacts/"+url.PathEscape(cid), nil, &c)
	if notFound(err) {
		s.remember(cid, nil)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p := &source.Person{ID: cid, Email: strings.TrimSpace(c.Email), Name: strings.TrimSpace(c.Name), Phone: strings.TrimSpace(c.Phone)}
	if c.Companies.TotalCount > 0 {
		var cos struct {
			Data []struct {
				Name    string `json:"name"`
				Website string `json:"website"`
			} `json:"data"`
		}
		err := s.c.Get(ctx, "/contacts/"+url.PathEscape(cid)+"/companies", nil, &cos)
		if err != nil && !optional(err) {
			return nil, err
		}
		for _, co := range cos.Data {
			if name := strings.TrimSpace(co.Name); name != "" {
				p.Company, p.Domain = name, domain(co.Website)
				break
			}
		}
	}
	s.remember(cid, p)
	return p, nil
}

func (s *intercom) remember(cid string, p *source.Person) {
	if len(s.contacts) >= maxCached {
		s.contacts = map[string]*source.Person{}
	}
	s.contacts[cid] = p
}

// domain is a company website's host without "www.".
func domain(website string) string {
	w := strings.TrimSpace(website)
	if w == "" {
		return ""
	}
	if !strings.Contains(w, "://") {
		w = "https://" + w
	}
	u, err := url.Parse(w)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// Load

func (s *intercom) Load(ctx context.Context, t *source.Ticket) error {
	var cv conversation
	if err := s.c.Get(ctx, "/conversations/"+url.PathEscape(t.ID), nil, &cv); err != nil {
		return err
	}
	var msgs []source.Message
	if src := cv.Source; strings.TrimSpace(src.Body) != "" || len(src.Attachments) > 0 {
		m := message(src.Author, src.Body, src.Attachments)
		m.ID = "source-" + string(src.ID)
		if src.ID == "" {
			m.ID = "source-" + string(cv.ID)
		}
		m.CreatedAt = source.Unix(cv.CreatedAt)
		msgs = append(msgs, m)
	}
	// Assignments, closes and other events carry a body only when the agent wrote one.
	for _, p := range cv.Parts.Parts {
		if strings.TrimSpace(p.Body) == "" && len(p.Attachments) == 0 {
			continue
		}
		m := message(p.Author, p.Body, p.Attachments)
		m.ID = string(p.ID)
		m.Note = strings.HasPrefix(p.PartType, "note")
		m.CreatedAt = source.Unix(p.CreatedAt)
		msgs = append(msgs, m)
	}
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].CreatedAt.Before(msgs[j].CreatedAt) })
	for i := range msgs {
		for j := range msgs[i].Attachments {
			msgs[i].Attachments[j].ID = msgs[i].ID + "-" + strconv.Itoa(j+1)
		}
	}
	t.Messages = msgs
	t.Loaded = true
	return nil
}

// message is a part by its author: a teammate, a bot (Fin, a workflow) or the team writes
// for the team; a user or lead is the customer.
func message(a author, body string, files []attachment) source.Message {
	m := source.Message{HTML: strings.TrimSpace(body), Author: source.Person{Name: strings.TrimSpace(a.Name)}}
	switch {
	case customer(a):
		m.Author.ID = string(a.ID)
		m.Author.Email = strings.TrimSpace(a.Email)
	case a.Type == "admin" && !a.FromAIAgent:
		m.Agent = true
		m.AuthorID = string(a.ID)
		m.Author.Email = strings.TrimSpace(a.Email)
	default:
		m.Agent = true
		if m.Author.Name == "" && (a.Type == "bot" || a.FromAIAgent) {
			m.Author.Name = "Fin"
		}
	}
	for _, f := range files {
		if strings.TrimSpace(f.URL) == "" {
			continue
		}
		m.Attachments = append(m.Attachments, source.Attachment{
			Name: strings.TrimSpace(f.Name), ContentType: strings.TrimSpace(f.ContentType), Size: f.Filesize, URL: f.URL,
		})
	}
	return m
}

// Articles

type articleList struct {
	TotalCount int `json:"total_count"`
	Data       []struct {
		ID        id     `json:"id"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		State     string `json:"state"`
		ParentID  id     `json:"parent_id"`
		ParentIDs []id   `json:"parent_ids"`
		CreatedAt int64  `json:"created_at"`
		UpdatedAt int64  `json:"updated_at"`
	} `json:"data"`
	Pages pages `json:"pages"`
}

// pages is page-number pagination; next is a link or a cursor depending on the
// endpoint, so the page numbers decide.
type pages struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
}

func (p pages) more() bool { return p.Page > 0 && p.Page < p.TotalPages }

func (s *intercom) Articles(ctx context.Context, cursor string) (source.Page[source.Article], error) {
	var page source.Page[source.Article]
	n := 1
	if cursor != "" {
		v, err := strconv.Atoi(cursor)
		if err != nil || v < 1 {
			return page, errors.New("malformed articles cursor")
		}
		n = v
	}
	if err := s.loadCollections(ctx); err != nil {
		return page, err
	}
	var list articleList
	q := url.Values{"page": {strconv.Itoa(n)}, "per_page": {strconv.Itoa(perPage)}}
	if err := s.c.Get(ctx, "/articles", q, &list); err != nil {
		return page, err
	}
	for _, a := range list.Data {
		parent := a.ParentID
		if parent == "" && len(a.ParentIDs) > 0 {
			parent = a.ParentIDs[0]
		}
		page.Items = append(page.Items, source.Article{
			ID:         string(a.ID),
			Title:      strings.TrimSpace(a.Title),
			HTML:       a.Body,
			Collection: s.collections[string(parent)],
			Published:  a.State == "published",
			CreatedAt:  source.Unix(a.CreatedAt),
			UpdatedAt:  source.Unix(a.UpdatedAt),
		})
	}
	if list.Pages.more() && len(list.Data) > 0 {
		page.Next = strconv.Itoa(n + 1)
	}
	return page, nil
}

// loadCollections reads the help center's collections and sections once.
func (s *intercom) loadCollections(ctx context.Context) error {
	if s.collections != nil {
		return nil
	}
	names := map[string]string{}
	for n := 1; n <= maxCollectionPages; n++ {
		var list struct {
			Data []struct {
				ID   id     `json:"id"`
				Name string `json:"name"`
			} `json:"data"`
			Pages pages `json:"pages"`
		}
		q := url.Values{"page": {strconv.Itoa(n)}, "per_page": {"150"}}
		if err := s.c.Get(ctx, "/help_center/collections", q, &list); err != nil {
			return err
		}
		for _, c := range list.Data {
			names[string(c.ID)] = strings.TrimSpace(c.Name)
		}
		if !list.Pages.more() || len(list.Data) == 0 {
			break
		}
	}
	s.collections = names
	return nil
}

// Download opens an attachment: Intercom links to signed files on its file hosts, which
// get no credentials.
func (s *intercom) Download(ctx context.Context, a source.Attachment) (io.ReadCloser, error) {
	if a.URL == "" {
		return nil, errors.New("attachment without a link")
	}
	return s.c.Open(ctx, a.URL)
}
