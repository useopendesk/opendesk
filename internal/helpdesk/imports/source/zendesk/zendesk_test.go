package zendesk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/useopendesk/opendesk/internal/helpdesk/imports/source"
)

const (
	testEmail = "admin@acme.test"
	testToken = "tok_abc123"
)

// fake is a Zendesk account: it answers the endpoints the source calls with fixtures
// shaped like the API reference.
type fake struct {
	t        *testing.T
	srv      *httptest.Server
	role     string // users/me role; "" answers the anonymous user
	missing  bool   // the account does not exist: 404 everywhere
	noHelp   bool   // the help center is off
	limited  bool   // every request answers 429
	storage  string // where attachment links redirect: file storage on another host
	requests []string
}

func newFake(t *testing.T) *fake {
	f := &fake{t: t, role: "admin"}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) source(t *testing.T) source.Source {
	t.Helper()
	s, err := New(source.Credentials{"subdomain": "acme", "email": testEmail, "token": testToken},
		source.Options{Dev: true, BaseURL: f.srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *fake) json(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = io.WriteString(w, strings.ReplaceAll(body, "{base}", f.srv.URL))
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	f.requests = append(f.requests, r.URL.RequestURI())
	q := r.URL.Query()

	if f.limited {
		w.Header().Set("Retry-After", "42")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	if f.missing {
		http.NotFound(w, r)
		return
	}
	user, pass, ok := r.BasicAuth()
	if !ok || user != testEmail+"/token" || pass != testToken {
		w.WriteHeader(http.StatusUnauthorized)
		f.json(w, `{"error":"Couldn't authenticate you"}`)
		return
	}

	switch r.URL.Path {
	case "/api/v2/users/me.json":
		if f.role == "" {
			f.json(w, `{"user":{"id":null,"url":null,"name":"Anonymous user","email":"invalid@example.com","role":"end-user"}}`)
			return
		}
		f.json(w, fmt.Sprintf(`{"user":{"id":900,"name":"Ada Admin","email":%q,"role":%q}}`, testEmail, f.role))
	case "/api/v2/brands.json":
		f.json(w, `{"brands":[{"id":1,"name":"Acme Labs","default":false},{"id":2,"name":"Acme Support","default":true,"subdomain":"acme"}],"meta":{"has_more":false}}`)
	case "/api/v2/tickets/count.json":
		f.json(w, `{"count":{"value":1234,"refreshed_at":"2026-10-01T10:00:00Z"}}`)
	case "/api/v2/help_center/articles.json":
		if f.noHelp {
			http.NotFound(w, r)
			return
		}
		if q.Get("per_page") == "1" {
			f.json(w, `{"articles":[{"id":1}],"count":3,"next_page":null,"page":1,"page_count":3}`)
			return
		}
		if q.Get("page[after]") == "" {
			f.json(w, `{"articles":[
				{"id":501,"title":"Billing FAQ","body":"<p>Pay</p>","draft":false,"section_id":71,"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-02-01T00:00:00Z"},
				{"id":502,"title":"Draft notes","body":"<p>Soon</p>","draft":true,"section_id":72,"created_at":"2025-01-02T00:00:00Z","updated_at":"2025-01-02T00:00:00Z"}
			],"meta":{"has_more":true,"after_cursor":"art2"},"links":{"next":"{base}/api/v2/help_center/articles.json?page[after]=art2"}}`)
			return
		}
		f.json(w, `{"articles":[{"id":503,"title":"Orphan","body":"<p>x</p>","draft":false,"section_id":99,"created_at":"2025-01-03T00:00:00Z","updated_at":"2025-01-03T00:00:00Z"}],"meta":{"has_more":false,"after_cursor":"art3"}}`)
	case "/api/v2/help_center/sections.json":
		if f.noHelp {
			http.NotFound(w, r)
			return
		}
		if q.Get("page[after]") == "" {
			f.json(w, `{"sections":[{"id":71,"name":"Billing","category_id":1}],"meta":{"has_more":true,"after_cursor":"sec2"}}`)
			return
		}
		f.json(w, `{"sections":[{"id":72,"name":"Internal","category_id":1}],"meta":{"has_more":false}}`)
	case "/api/v2/users.json":
		if got := q["role[]"]; len(got) != 2 || got[0] != "agent" || got[1] != "admin" {
			http.Error(w, "bad roles", http.StatusBadRequest)
			return
		}
		if q.Get("page[after]") == "" {
			f.json(w, `{"users":[{"id":900,"name":"Ada Admin","email":"admin@acme.test","role":"admin"}],"meta":{"has_more":true,"after_cursor":"u2"}}`)
			return
		}
		f.json(w, `{"users":[{"id":901,"name":"Bo Agent","email":"bo@acme.test","role":"agent"}],"meta":{"has_more":false}}`)
	case "/api/v2/groups.json":
		f.json(w, `{"groups":[{"id":31,"name":"Support","deleted":false},{"id":32,"name":"Old","deleted":true}],"meta":{"has_more":false}}`)
	case "/api/v2/incremental/tickets/cursor.json":
		f.export(w, q)
	case "/api/v2/users/show_many.json":
		if q.Get("ids") != "702" && q.Get("ids") != "903" {
			http.Error(w, "unexpected ids "+q.Get("ids"), http.StatusBadRequest)
			return
		}
		if q.Get("ids") == "903" {
			f.json(w, `{"users":[{"id":903,"name":"Cy Agent","email":"cy@acme.test","role":"agent"}]}`)
			return
		}
		f.json(w, `{"users":[{"id":702,"name":"Late Larry","email":"larry@example.com","role":"end-user","organization_id":81}]}`)
	case "/api/v2/organizations/show_many.json":
		f.json(w, `{"organizations":[{"id":81,"name":"Larry Ltd","domain_names":["larry.example"]}]}`)
	case "/api/v2/tickets/11/comments.json":
		f.comments(w, q)
	case "/attachments/token/abc/":
		if _, _, ok := r.BasicAuth(); !ok {
			http.Error(w, "no auth", http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, f.storage+"/files/report.pdf?sig=1", http.StatusFound)
	default:
		http.NotFound(w, r)
	}
}

func (f *fake) export(w http.ResponseWriter, q map[string][]string) {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	if get("include") != "users,organizations,metric_sets" || get("per_page") != "100" {
		http.Error(w, "bad params", http.StatusBadRequest)
		return
	}
	switch get("cursor") {
	case "":
		if get("start_time") == "" {
			http.Error(w, "start_time required", http.StatusBadRequest)
			return
		}
		f.json(w, `{
			"tickets":[
				{"id":10,"subject":"Old one","status":"closed","priority":null,"requester_id":701,"assignee_id":null,"group_id":null,"organization_id":null,"tags":[],"via":{"channel":"email","source":{}},"satisfaction_rating":null,"created_at":"2024-01-01T00:00:00Z","updated_at":"2025-06-01T00:00:00Z"},
				{"id":11,"subject":"Cannot log in","status":"open","priority":"high","requester_id":701,"assignee_id":901,"group_id":31,"organization_id":80,"tags":["login","vip"],"via":{"channel":"web","source":{"from":{},"to":{},"rel":null}},"satisfaction_rating":{"score":"good","id":5},"created_at":"2025-06-02T00:00:00Z","updated_at":"2025-06-03T00:00:00Z"},
				{"id":12,"subject":"Spam","status":"deleted","priority":"low","requester_id":799,"tags":[],"via":{"channel":"email"},"created_at":"2025-06-02T00:00:00Z","updated_at":"2025-06-04T00:00:00Z"}
			],
			"users":[{"id":701,"name":"Cara Customer","email":"cara@example.com","phone":"+15550100","role":"end-user","organization_id":80}],
			"organizations":[{"id":80,"name":"Cara Corp","domain_names":["cara.example"]}],
			"after_cursor":"c2","before_cursor":null,"after_url":"{base}/api/v2/incremental/tickets/cursor.json?cursor=c2","end_of_stream":false}`)
	case "c2":
		f.json(w, `{
			"tickets":[
				{"id":13,"subject":"Chat","status":"hold","priority":"urgent","requester_id":702,"via":{"channel":"native_messaging"},"satisfaction_rating":{"score":"bad"},"created_at":"2025-07-01T00:00:00Z","updated_at":"2025-07-02T00:00:00Z"},
				{"id":14,"subject":"Done","status":"solved","priority":"normal","requester_id":701,"via":{"channel":"voice"},"satisfaction_rating":{"score":"offered"},"created_at":"2025-07-01T00:00:00Z","updated_at":"2025-07-03T00:00:00Z"},
				{"id":15,"subject":"Integration","status":"pending","priority":"low","requester_id":0,"via":{"channel":"sample_ticket"},"created_at":"2025-07-01T00:00:00Z","updated_at":"2025-07-04T00:00:00Z"},
				{"id":16,"subject":"API","status":"new","requester_id":701,"via":{"channel":"api"},"created_at":"2025-07-01T00:00:00Z","updated_at":"2025-07-05T00:00:00Z"}
			],
			"users":[],"organizations":[],
			"metric_sets":[{"ticket_id":14,"solved_at":"2025-07-03T00:00:00Z"}],
			"after_cursor":"c3","before_cursor":"c1","end_of_stream":true}`)
	default:
		http.Error(w, "bad cursor", http.StatusBadRequest)
	}
}

func (f *fake) comments(w http.ResponseWriter, q map[string][]string) {
	if v := q["include"]; len(v) == 0 || v[0] != "users" {
		http.Error(w, "include users", http.StatusBadRequest)
		return
	}
	if v := q["page[after]"]; len(v) == 0 {
		f.json(w, `{"comments":[
			{"id":1001,"type":"Comment","body":"I cannot log in","html_body":"<div class=\"zd-comment\"><p>I cannot log in</p></div>","plain_body":"I cannot log in","public":true,"author_id":701,
			 "attachments":[{"id":61,"file_name":"report.pdf","content_url":"{base}/attachments/token/abc/?name=report.pdf","content_type":"application/pdf","size":15,"inline":false,"deleted":false},
			                {"id":62,"file_name":"gone.png","content_url":"{base}/attachments/token/gone/","content_type":"image/png","size":10,"inline":false,"deleted":true}],
			 "via":{"channel":"email","source":{"from":{"address":"cara@example.com","name":"Cara"},"to":{"address":"support@acme.zendesk.com","name":"Acme"},"rel":null}},"created_at":"2025-06-02T00:00:00Z"},
			{"id":1002,"type":"Comment","body":"Checking the logs","html_body":"","public":false,"author_id":901,"attachments":[],"via":{"channel":"web"},"created_at":"2025-06-02T01:00:00Z"}
		],"users":[
			{"id":701,"name":"Cara Customer","email":"cara@example.com","role":"end-user"},
			{"id":901,"name":"Bo Agent","email":"bo@acme.test","role":"agent"}
		],"meta":{"has_more":true,"after_cursor":"cm2"},"links":{"next":"{base}/api/v2/tickets/11/comments.json?page[after]=cm2"}}`)
		return
	}
	f.json(w, `{"comments":[
		{"id":1003,"type":"Comment","body":"Fixed","html_body":"<p>Fixed</p>","public":true,"author_id":903,
		 "attachments":[{"id":63,"file_name":"shot.png","content_url":"{base}/attachments/token/shot/","content_type":"image/png","size":2048,"inline":true}],
		 "via":{"channel":"web"},"created_at":"2025-06-02T02:00:00Z"}
	],"users":[],"meta":{"has_more":false,"after_cursor":"cm3"}}`)
}

func TestNewValidatesShape(t *testing.T) {
	ok := source.Credentials{"subdomain": "https://Acme.zendesk.com/agent", "email": "a@acme.test", "token": "t"}
	if _, err := New(ok, source.Options{}); err != nil {
		t.Fatalf("valid credentials: %v", err)
	}
	for name, tc := range map[string]struct {
		creds source.Credentials
		field string
	}{
		"no subdomain":  {source.Credentials{"email": "a@acme.test", "token": "t"}, "subdomain"},
		"bad subdomain": {source.Credentials{"subdomain": "ac_me", "email": "a@acme.test", "token": "t"}, "subdomain"},
		"no email":      {source.Credentials{"subdomain": "acme", "token": "t"}, "email"},
		"bad email":     {source.Credentials{"subdomain": "acme", "email": "admin", "token": "t"}, "email"},
		"no token":      {source.Credentials{"subdomain": "acme", "email": "a@acme.test"}, "token"},
		"spaced token":  {source.Credentials{"subdomain": "acme", "email": "a@acme.test", "token": "a b"}, "token"},
	} {
		_, err := New(tc.creds, source.Options{})
		var inv *source.InvalidError
		if !errors.As(err, &inv) || inv.Field != tc.field || !errors.Is(err, source.ErrInvalid) {
			t.Errorf("%s: got %v, want invalid %s", name, err, tc.field)
		}
	}
}

func TestCheck(t *testing.T) {
	f := newFake(t)
	scan, err := f.source(t).Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := source.Account{Key: "acme", Name: "Acme Support", URL: "https://acme.zendesk.com"}
	if scan.Account != want {
		t.Errorf("account = %+v", scan.Account)
	}
	if scan.Tickets != 1234 || scan.Articles != 3 || !scan.HasArticles {
		t.Errorf("totals = %d tickets, %d articles, help center %v", scan.Tickets, scan.Articles, scan.HasArticles)
	}
	if len(scan.Agents) != 2 || scan.Agents[0] != (source.Agent{ID: "900", Name: "Ada Admin", Email: "admin@acme.test"}) || scan.Agents[1].ID != "901" {
		t.Errorf("agents = %+v", scan.Agents)
	}
	if len(scan.Groups) != 1 || scan.Groups[0] != (source.Group{ID: "31", Name: "Support"}) {
		t.Errorf("groups = %+v", scan.Groups)
	}
}

func TestCheckHelpCenterOff(t *testing.T) {
	f := newFake(t)
	f.noHelp = true
	s := f.source(t)
	scan, err := s.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.HasArticles || scan.Articles != 0 {
		t.Errorf("help center off: has %v, %d articles", scan.HasArticles, scan.Articles)
	}
	if _, err := s.Articles(context.Background(), ""); !errors.Is(err, source.ErrUnsupported) {
		t.Errorf("articles with the help center off: %v", err)
	}
}

func TestCheckRefused(t *testing.T) {
	f := newFake(t)
	s, err := New(source.Credentials{"subdomain": "acme", "email": testEmail, "token": "wrong"},
		source.Options{Dev: true, BaseURL: f.srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Check(context.Background())
	if !errors.Is(err, source.ErrAuth) {
		t.Fatalf("wrong token: %v", err)
	}
	if strings.Contains(err.Error(), "wrong") || strings.Contains(err.Error(), testEmail) {
		t.Errorf("error carries credentials: %v", err)
	}

	for _, role := range []string{"", "agent", "end-user"} {
		f.role = role
		if _, err := f.source(t).Check(context.Background()); !errors.Is(err, source.ErrAuth) {
			t.Errorf("role %q: %v", role, err)
		}
	}
}

func TestCheckNoAccount(t *testing.T) {
	f := newFake(t)
	f.missing = true
	s := f.source(t)
	if _, err := s.Check(context.Background()); !errors.Is(err, source.ErrNotFound) {
		t.Fatalf("missing account: %v", err)
	}
	// Past the account lookup, a 404 is the source's own error.
	_, err := s.Tickets(context.Background(), "", nil, nil)
	var re *source.RemoteError
	if !errors.As(err, &re) || re.Status != http.StatusNotFound {
		t.Errorf("404 after check: %v", err)
	}
}

func TestTickets(t *testing.T) {
	f := newFake(t)
	s := f.source(t)
	ctx := context.Background()

	p1, err := s.Tickets(ctx, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p1.Next != "c2" || len(p1.Items) != 3 {
		t.Fatalf("page 1: next %q, %d tickets", p1.Next, len(p1.Items))
	}
	if !strings.Contains(f.requests[0], "start_time=0") {
		t.Errorf("first request %s", f.requests[0])
	}
	t11 := p1.Items[1]
	if t11.ID != "11" || t11.Ref != "#11" || t11.URL != "https://acme.zendesk.com/agent/tickets/11" ||
		t11.Status != "open" || t11.Priority != "high" || t11.Channel != "portal" || t11.CSAT != 5 ||
		t11.AssigneeID != "901" || t11.GroupID != "31" || strings.Join(t11.Tags, ",") != "login,vip" || t11.Loaded {
		t.Errorf("ticket 11 = %+v", t11)
	}
	wantReq := source.Person{ID: "701", Name: "Cara Customer", Email: "cara@example.com", Phone: "+15550100", Company: "Cara Corp", Domain: "cara.example"}
	if t11.Requester == nil || *t11.Requester != wantReq {
		t.Errorf("requester = %+v", t11.Requester)
	}
	if !t11.CreatedAt.Equal(time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("created = %v", t11.CreatedAt)
	}
	t10 := p1.Items[0]
	if t10.Status != "closed" || t10.Priority != "normal" || t10.Channel != "email" || t10.AssigneeID != "" || t10.GroupID != "" || t10.CSAT != 0 {
		t.Errorf("ticket 10 = %+v", t10)
	}
	if !p1.Items[2].Skip {
		t.Errorf("deleted ticket not skipped: %+v", p1.Items[2])
	}

	p2, err := s.Tickets(ctx, p1.Next, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p2.Next != "" || len(p2.Items) != 4 {
		t.Fatalf("page 2: next %q, %d tickets", p2.Next, len(p2.Items))
	}
	t13, t14, t15, t16 := p2.Items[0], p2.Items[1], p2.Items[2], p2.Items[3]
	if t13.Status != "on_hold" || t13.Priority != "urgent" || t13.Channel != "chat" || t13.CSAT != 1 {
		t.Errorf("ticket 13 = %+v", t13)
	}
	// Requester 702 was not sideloaded: fetched with show_many, its organization too.
	if t13.Requester == nil || t13.Requester.Name != "Late Larry" || t13.Requester.Company != "Larry Ltd" || t13.Requester.Domain != "larry.example" {
		t.Errorf("ticket 13 requester = %+v", t13.Requester)
	}
	if t14.Status != "solved" || t14.Channel != "email" || t14.CSAT != 0 || t14.SolvedAt == nil || !t14.SolvedAt.Equal(time.Date(2025, 7, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("ticket 14 = %+v", t14)
	}
	if t15.Status != "pending" || t15.Priority != "low" || t15.Channel != "api" || t15.Requester != nil {
		t.Errorf("ticket 15 = %+v", t15)
	}
	if t16.Status != "open" || t16.Channel != "api" {
		t.Errorf("ticket 16 = %+v", t16)
	}
}

func TestTicketsSince(t *testing.T) {
	f := newFake(t)
	s := f.source(t)
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	p, err := s.Tickets(context.Background(), "", &since, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.requests[0], fmt.Sprintf("start_time=%d", since.Unix())) {
		t.Errorf("request %s", f.requests[0])
	}
	for _, tk := range p.Items {
		if tk.ID == "10" {
			t.Errorf("ticket created before since kept")
		}
	}
	if len(p.Items) != 2 {
		t.Errorf("%d tickets", len(p.Items))
	}

	updated := time.Date(2025, 6, 3, 12, 0, 0, 0, time.UTC)
	f.requests = nil
	p, err = s.Tickets(context.Background(), "", nil, &updated)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.requests[0], fmt.Sprintf("start_time=%d", updated.Unix())) {
		t.Errorf("request %s", f.requests[0])
	}
	if len(p.Items) != 1 || p.Items[0].ID != "12" {
		t.Errorf("updated since: %+v", p.Items)
	}

	if got := startTime(nil, ptr(time.Now())); got > time.Now().Add(-time.Minute).Unix() {
		t.Errorf("start time within the last minute: %d", got)
	}
}

func ptr[T any](v T) *T { return &v }

func TestLoad(t *testing.T) {
	f := newFake(t)
	s := f.source(t)
	tk := &source.Ticket{ID: "11"}
	if err := s.Load(context.Background(), tk); err != nil {
		t.Fatal(err)
	}
	if !tk.Loaded || len(tk.Messages) != 3 {
		t.Fatalf("loaded %v, %d messages", tk.Loaded, len(tk.Messages))
	}
	m1, m2, m3 := tk.Messages[0], tk.Messages[1], tk.Messages[2]
	if m1.ID != "1001" || m1.Agent || m1.Note || m1.AuthorID != "" || m1.Author.Email != "cara@example.com" ||
		!strings.Contains(m1.HTML, "I cannot log in") || m1.Text != "" || len(m1.To) != 1 || m1.To[0] != "support@acme.zendesk.com" {
		t.Errorf("message 1 = %+v", m1)
	}
	if len(m1.Attachments) != 1 {
		t.Fatalf("message 1 attachments = %+v", m1.Attachments)
	}
	a := m1.Attachments[0]
	if a.ID != "61" || a.Name != "report.pdf" || a.ContentType != "application/pdf" || a.Size != 15 || a.Inline ||
		a.URL != f.srv.URL+"/attachments/token/abc/?name=report.pdf" {
		t.Errorf("attachment = %+v", a)
	}
	if !m2.Agent || !m2.Note || m2.AuthorID != "901" || m2.Author.Name != "Bo Agent" || m2.HTML != "" || m2.Text != "Checking the logs" {
		t.Errorf("message 2 = %+v", m2)
	}
	// Author 903 came from show_many; the comments' second page came from page[after].
	if !m3.Agent || m3.AuthorID != "903" || m3.Author.Name != "Cy Agent" || len(m3.Attachments) != 1 || !m3.Attachments[0].Inline {
		t.Errorf("message 3 = %+v", m3)
	}

	if err := s.Load(context.Background(), &source.Ticket{ID: "../users"}); err == nil {
		t.Error("a ticket id that is not a number was loaded")
	}
}

func TestArticles(t *testing.T) {
	f := newFake(t)
	s := f.source(t)
	ctx := context.Background()
	p1, err := s.Articles(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if p1.Next != "art2" || len(p1.Items) != 2 {
		t.Fatalf("page 1: next %q, %d", p1.Next, len(p1.Items))
	}
	a := p1.Items[0]
	if a.ID != "501" || a.Title != "Billing FAQ" || a.HTML != "<p>Pay</p>" || a.Collection != "Billing" || !a.Published ||
		!a.UpdatedAt.Equal(time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("article = %+v", a)
	}
	if p1.Items[1].Published || p1.Items[1].Collection != "Internal" {
		t.Errorf("draft = %+v", p1.Items[1])
	}
	p2, err := s.Articles(ctx, p1.Next)
	if err != nil {
		t.Fatal(err)
	}
	if p2.Next != "" || len(p2.Items) != 1 || p2.Items[0].Collection != "" {
		t.Errorf("page 2 = %+v", p2)
	}
	sections := 0
	for _, r := range f.requests {
		if strings.HasPrefix(r, "/api/v2/help_center/sections.json") {
			sections++
		}
	}
	if sections != 2 {
		t.Errorf("sections fetched %d times, want 2 pages once", sections)
	}
}

func TestRateLimited(t *testing.T) {
	f := newFake(t)
	f.limited = true
	s := f.source(t)
	for name, call := range map[string]func() error{
		"check":    func() error { _, err := s.Check(context.Background()); return err },
		"tickets":  func() error { _, err := s.Tickets(context.Background(), "", nil, nil); return err },
		"load":     func() error { return s.Load(context.Background(), &source.Ticket{ID: "11"}) },
		"articles": func() error { _, err := s.Articles(context.Background(), ""); return err },
	} {
		var rl *source.RateLimited
		if err := call(); !errors.As(err, &rl) || rl.RetryAfter != 42*time.Second {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestDownload(t *testing.T) {
	f := newFake(t)
	// File storage is another host and must never see the account's credentials.
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.URL.Path != "/files/report.pdf" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, "%PDF-1.7 report")
	}))
	defer storage.Close()
	f.storage = storage.URL
	s := f.source(t)
	rc, err := s.Download(context.Background(), source.Attachment{URL: f.srv.URL + "/attachments/token/abc/?name=report.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if string(body) != "%PDF-1.7 report" {
		t.Errorf("body = %q", body)
	}
}
