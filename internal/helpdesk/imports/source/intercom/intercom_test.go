package intercom

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/useopendesk/opendesk/internal/helpdesk/imports/source"
)

type fake struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	searches []map[string]any
	hits     map[string]int
	limited  bool
	reset    int64
}

func (f *fake) hit(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

const meJSON = `{"type":"admin","id":"991267459","name":"Ciaran Lee","email":"ciaran@acme.test",
 "app":{"type":"app","id_code":"abc123xy","name":"Acme Support","region":"US","timezone":"Europe/London"}}`

const page1 = `{"type":"conversation.list","total_count":3,
 "pages":{"type":"pages","page":1,"per_page":50,"total_pages":2,"next":{"page":2,"starting_after":"WzE3MDk2NTMxODUwMDAsMTkwLDJd"}},
 "conversations":[
  {"type":"conversation","id":"101","title":null,"created_at":1700000000,"updated_at":1700000500,"state":"closed","priority":"priority",
   "source":{"type":"email","id":"403918334","delivered_as":"customer_initiated","subject":"<b>Refund</b> for order 42","body":"<p>Hi, I want a refund.</p>",
     "author":{"type":"user","id":"c1","name":"Jo Customer","email":"jo@customer.test"},"attachments":[]},
   "contacts":{"type":"contact.list","contacts":[{"type":"contact","id":"c1","external_id":"70"}]},
   "admin_assignee_id":991267466,"team_assignee_id":5017691,
   "tags":{"type":"tag.list","tags":[{"type":"tag","id":"1","name":"billing"},{"type":"tag","id":"2","name":" "}]},
   "conversation_rating":{"rating":4,"remark":"ok"},
   "statistics":{"type":"conversation_statistics","last_close_at":1700000400}},
  {"type":"conversation","id":"102","title":"Login trouble","created_at":1700001000,"updated_at":1700001100,"state":"snoozed","priority":"not_priority",
   "source":{"type":"conversation","delivered_as":"customer_initiated","subject":"","body":"<p>Cannot log in</p>",
     "author":{"type":"user","id":"c1","name":"Jo Customer","email":"jo@customer.test"},"attachments":[]},
   "contacts":{"type":"contact.list","contacts":[{"type":"contact","id":"c1"}]},
   "admin_assignee_id":null,"team_assignee_id":null,
   "tags":{"type":"tag.list","tags":[]},"conversation_rating":null,"statistics":null}
 ]}`

const page2 = `{"type":"conversation.list","total_count":3,
 "pages":{"type":"pages","page":2,"per_page":50,"total_pages":2},
 "conversations":[
  {"type":"conversation","id":"103","title":"","created_at":1700002000,"updated_at":1700002000,"state":"open","priority":"not_priority",
   "source":{"type":"conversation","delivered_as":"admin_initiated","subject":"","body":"<p>Welcome aboard! We noticed you signed up yesterday and wanted to say hello to you</p>",
     "author":{"type":"admin","id":"991267466","name":"Sam Agent","email":"sam@acme.test"},"attachments":[]},
   "contacts":{"type":"contact.list","contacts":[{"type":"contact","id":"gone"}]},
   "admin_assignee_id":"991267466","team_assignee_id":0,
   "tags":{"type":"tag.list","tags":[]}}
 ]}`

const conv101 = `{"type":"conversation","id":"101","created_at":1700000000,"updated_at":1700000500,"state":"closed",
 "source":{"type":"email","id":"403918334","delivered_as":"customer_initiated","subject":"Refund","body":"<p>Hi, I want a refund.</p>",
   "author":{"type":"user","id":"c1","name":"Jo Customer","email":"jo@customer.test"},
   "attachments":[{"type":"upload","name":"receipt.pdf","url":"https://downloads.intercomcdn.test/receipt.pdf","content_type":"application/pdf","filesize":2048}]},
 "contacts":{"type":"contact.list","contacts":[{"type":"contact","id":"c1"}]},
 "conversation_parts":{"type":"conversation_part.list","total_count":7,"conversation_parts":[
  {"type":"conversation_part","id":"1","part_type":"assignment","body":null,"created_at":1700000010,"author":{"type":"admin","id":"991267466","name":"Sam Agent","email":"sam@acme.test"},"attachments":[]},
  {"type":"conversation_part","id":"2","part_type":"comment","body":"<p>Sorry about that, looking now.</p>","created_at":1700000100,"author":{"type":"bot","id":"278","name":"Fin","email":"operator+abc@intercom.io"},"attachments":[]},
  {"type":"conversation_part","id":"3","part_type":"note","body":"<p>Check the payment provider.</p>","created_at":1700000200,"author":{"type":"admin","id":"991267466","name":"Sam Agent","email":"sam@acme.test"},"attachments":[]},
  {"type":"conversation_part","id":"4","part_type":"comment","body":"","created_at":1700000250,"author":{"type":"user","id":"c1","name":"Jo Customer","email":"jo@customer.test"},
   "attachments":[{"type":"upload","name":"screen.png","url":"https://downloads.intercomcdn.test/screen.png","content_type":"image/png","filesize":512}]},
  {"type":"conversation_part","id":"5","part_type":"comment","body":"<p>Refunded.</p>","created_at":1700000300,"author":{"type":"admin","id":"991267466","name":"Sam Agent","email":"sam@acme.test"},"attachments":[]},
  {"type":"conversation_part","id":"6","part_type":"close","body":"<p>Closing, thanks!</p>","created_at":1700000400,"author":{"type":"admin","id":"991267466","name":"Sam Agent","email":"sam@acme.test"},"attachments":[]},
  {"type":"conversation_part","id":"7","part_type":"open","body":null,"created_at":1700000450,"author":{"type":"lead","id":"c1","name":"Jo Customer","email":"jo@customer.test"},"attachments":[]}
 ]}}`

func newFake(t *testing.T) *fake {
	f := &fake{t: t, hits: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.hits[r.URL.Path]++
	limited, reset := f.limited, f.reset
	f.mu.Unlock()

	if v := r.Header.Get("Intercom-Version"); v != version {
		f.t.Errorf("%s: Intercom-Version %q", r.URL.Path, v)
	}
	if r.Header.Get("Authorization") != "Bearer good-token" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"type":"error.list","errors":[{"code":"unauthorized","message":"Access Token Invalid"}]}`)
		return
	}
	if limited {
		w.Header().Set("X-RateLimit-Limit", "10000")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/me":
		_, _ = io.WriteString(w, meJSON)
	case "/admins":
		_, _ = io.WriteString(w, `{"type":"admin.list","admins":[
		 {"type":"admin","id":"991267466","name":"Sam Agent","email":"sam@acme.test","team_ids":[5017691]},
		 {"type":"admin","id":"991267467","name":"No Mail","email":""}]}`)
	case "/teams":
		_, _ = io.WriteString(w, `{"type":"team.list","teams":[{"type":"team","id":"5017691","name":"Billing","admin_ids":[991267466]}]}`)
	case "/conversations/search":
		if r.Method != http.MethodPost {
			f.t.Errorf("search with %s", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			f.t.Errorf("search body: %v", err)
		}
		f.mu.Lock()
		f.searches = append(f.searches, body)
		f.mu.Unlock()
		pg, _ := body["pagination"].(map[string]any)
		switch {
		case pg["per_page"] == float64(1):
			_, _ = io.WriteString(w, `{"type":"conversation.list","total_count":1234,"conversations":[],"pages":{"type":"pages","page":1,"per_page":1,"total_pages":1234}}`)
		case pg["starting_after"] == "WzE3MDk2NTMxODUwMDAsMTkwLDJd":
			_, _ = io.WriteString(w, page2)
		default:
			_, _ = io.WriteString(w, page1)
		}
	case "/contacts/c1":
		_, _ = io.WriteString(w, `{"type":"contact","id":"c1","role":"user","email":"jo@customer.test","name":"Jo Customer","phone":"+15551234",
		 "companies":{"type":"list","data":[{"type":"company","id":"co1","url":"/companies/co1"}],"url":"/contacts/c1/companies","total_count":1,"has_more":false}}`)
	case "/contacts/c1/companies":
		_, _ = io.WriteString(w, `{"type":"list","data":[{"type":"company","id":"co1","company_id":"1","name":"Customer Co","website":"https://www.customer.test/about"}],"total_count":1}`)
	case "/contacts/gone":
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"type":"error.list","errors":[{"code":"not_found","message":"User Not Found"}]}`)
	case "/conversations/101":
		if r.URL.Query().Get("display_as") != "" {
			f.t.Errorf("conversation asked as %q", r.URL.Query().Get("display_as"))
		}
		_, _ = io.WriteString(w, conv101)
	case "/articles":
		q := r.URL.Query()
		if q.Get("per_page") == "1" {
			_, _ = io.WriteString(w, `{"type":"list","total_count":3,"data":[],"pages":{"type":"pages","page":1,"per_page":1,"total_pages":3}}`)
			return
		}
		if q.Get("per_page") != "50" {
			f.t.Errorf("articles per_page %q", q.Get("per_page"))
		}
		if q.Get("page") == "2" {
			_, _ = io.WriteString(w, `{"type":"list","total_count":3,"pages":{"type":"pages","page":2,"per_page":50,"total_pages":2},"data":[
			 {"type":"article","id":"41","title":"Loose article","body":"<p>On its own</p>","state":"published","parent_id":null,"parent_ids":[],"created_at":1700000000,"updated_at":1700000000}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"type":"list","total_count":3,"pages":{"type":"pages","page":1,"per_page":50,"total_pages":2,"next":"https://api.intercom.io/articles?page=2&per_page=50"},"data":[
		 {"type":"article","id":"39","title":"Billing FAQ","body":"<p>How billing works</p>","state":"published","parent_id":160,"parent_type":"collection","parent_ids":[159,160],"created_at":1700000000,"updated_at":1700000100},
		 {"type":"article","id":"40","title":"Draft one","body":"<p>WIP</p>","state":"draft","parent_ids":[159],"created_at":1700000000,"updated_at":1700000200}]}`)
	case "/help_center/collections":
		if r.URL.Query().Get("page") == "2" {
			_, _ = io.WriteString(w, `{"type":"list","total_count":2,"pages":{"type":"pages","page":2,"per_page":1,"total_pages":2},"data":[
			 {"id":"160","name":"Payments","parent_id":"159"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"type":"list","total_count":2,"pages":{"type":"pages","page":1,"per_page":1,"total_pages":2},"data":[
		 {"id":"159","name":"Billing","parent_id":null}]}`)
	default:
		f.t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fake) source(t *testing.T, token string) source.Source {
	t.Helper()
	s, err := New(source.Credentials{"token": token}, source.Options{Dev: true, BaseURL: f.srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNew(t *testing.T) {
	for _, tc := range []struct {
		creds source.Credentials
		field string
	}{
		{source.Credentials{"token": ""}, "token"},
		{source.Credentials{"token": "two words"}, "token"},
		{source.Credentials{"token": "tok", "region": "mars"}, "region"},
	} {
		_, err := New(tc.creds, source.Options{})
		var inv *source.InvalidError
		if !errors.As(err, &inv) || inv.Field != tc.field || !errors.Is(err, source.ErrInvalid) {
			t.Errorf("%v: got %v, want invalid %s", tc.creds, err, tc.field)
		}
		if err != nil && strings.Contains(err.Error(), "two words") {
			t.Errorf("error carries the token: %v", err)
		}
	}
	for region, want := range map[string]string{"": "https://api.intercom.io", "US": "https://api.intercom.io", " eu ": "https://api.eu.intercom.io", "au": "https://api.au.intercom.io"} {
		s, err := New(source.Credentials{"token": "tok", "region": region}, source.Options{})
		if err != nil {
			t.Fatalf("region %q: %v", region, err)
		}
		if got := s.(*intercom).c.Base; got != want {
			t.Errorf("region %q: base %s, want %s", region, got, want)
		}
	}
}

func TestCheck(t *testing.T) {
	f := newFake(t)
	scan, err := f.source(t, "good-token").Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.Account != (source.Account{Key: "abc123xy", Name: "Acme Support", URL: "https://app.intercom.com/a/apps/abc123xy"}) {
		t.Errorf("account %+v", scan.Account)
	}
	if scan.Tickets != 1234 || scan.Articles != 3 || !scan.HasArticles {
		t.Errorf("totals %d tickets, %d articles, has %v", scan.Tickets, scan.Articles, scan.HasArticles)
	}
	if len(scan.Agents) != 1 || scan.Agents[0] != (source.Agent{ID: "991267466", Name: "Sam Agent", Email: "sam@acme.test"}) {
		t.Errorf("agents %+v", scan.Agents)
	}
	if len(scan.Groups) != 1 || scan.Groups[0] != (source.Group{ID: "5017691", Name: "Billing"}) {
		t.Errorf("groups %+v", scan.Groups)
	}
	q := f.searches[0]["query"].(map[string]any)
	if q["field"] != "created_at" || q["operator"] != ">" || q["value"] != "0" {
		t.Errorf("count query %v", q)
	}
}

func TestCheckRefused(t *testing.T) {
	f := newFake(t)
	_, err := f.source(t, "bad-token").Check(context.Background())
	if !errors.Is(err, source.ErrAuth) {
		t.Fatalf("got %v, want ErrAuth", err)
	}
	if strings.Contains(err.Error(), "bad-token") {
		t.Errorf("error carries the token: %v", err)
	}
}

func TestTickets(t *testing.T) {
	f := newFake(t)
	s := f.source(t, "good-token")
	ctx := context.Background()
	since := time.Date(2023, 11, 1, 0, 0, 0, 0, time.UTC)

	p1, err := s.Tickets(ctx, "", &since, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p1.Next != "WzE3MDk2NTMxODUwMDAsMTkwLDJd" || len(p1.Items) != 2 {
		t.Fatalf("page 1: next %q, %d items", p1.Next, len(p1.Items))
	}
	body := f.searches[0]
	if q := body["query"].(map[string]any); q["field"] != "created_at" || q["operator"] != ">" || q["value"] != strconv.FormatInt(since.Unix(), 10) {
		t.Errorf("since query %v", q)
	}
	if srt := body["sort"].(map[string]any); srt["field"] != "created_at" || srt["order"] != "ascending" {
		t.Errorf("sort %v", srt)
	}
	if pg := body["pagination"].(map[string]any); pg["per_page"] != float64(50) || pg["starting_after"] != nil {
		t.Errorf("pagination %v", pg)
	}

	a := p1.Items[0]
	if a.ID != "101" || a.Ref != "#101" || a.URL != "https://app.intercom.com/a/inbox/abc123xy/inbox/shared/all/conversation/101" {
		t.Errorf("ticket ids %q %q %q", a.ID, a.Ref, a.URL)
	}
	if a.Subject != "Refund for order 42" || a.Status != "closed" || a.Priority != "high" || a.Channel != "email" {
		t.Errorf("ticket 101 %q %s %s %s", a.Subject, a.Status, a.Priority, a.Channel)
	}
	if a.AssigneeID != "991267466" || a.GroupID != "5017691" || a.CSAT != 4 || len(a.Tags) != 1 || a.Tags[0] != "billing" {
		t.Errorf("ticket 101 assignee %q group %q csat %d tags %v", a.AssigneeID, a.GroupID, a.CSAT, a.Tags)
	}
	if a.SolvedAt == nil || !a.SolvedAt.Equal(time.Unix(1700000400, 0)) || !a.CreatedAt.Equal(time.Unix(1700000000, 0)) || !a.UpdatedAt.Equal(time.Unix(1700000500, 0)) {
		t.Errorf("ticket 101 times %v %v %v", a.SolvedAt, a.CreatedAt, a.UpdatedAt)
	}
	want := source.Person{ID: "c1", Email: "jo@customer.test", Name: "Jo Customer", Phone: "+15551234", Company: "Customer Co", Domain: "customer.test"}
	if a.Requester == nil || *a.Requester != want {
		t.Errorf("requester %+v", a.Requester)
	}
	if a.Loaded {
		t.Error("listing says loaded")
	}

	b := p1.Items[1]
	if b.Subject != "Login trouble" || b.Status != "on_hold" || b.Priority != "normal" || b.Channel != "chat" || b.AssigneeID != "" || b.GroupID != "" || b.SolvedAt != nil || b.CSAT != 0 {
		t.Errorf("ticket 102 %+v", b)
	}
	if f.hit("/contacts/c1") != 1 || f.hit("/contacts/c1/companies") != 1 {
		t.Errorf("contact read %d times, companies %d", f.hit("/contacts/c1"), f.hit("/contacts/c1/companies"))
	}

	upd := time.Unix(1700001500, 0)
	p2, err := s.Tickets(ctx, p1.Next, &since, &upd)
	if err != nil {
		t.Fatal(err)
	}
	if p2.Next != "" || len(p2.Items) != 1 {
		t.Fatalf("page 2: next %q, %d items", p2.Next, len(p2.Items))
	}
	body = f.searches[1]
	q := body["query"].(map[string]any)
	fs, _ := q["value"].([]any)
	if q["operator"] != "AND" || len(fs) != 2 {
		t.Fatalf("sync query %v", q)
	}
	if u := fs[1].(map[string]any); u["field"] != "updated_at" || u["operator"] != ">" || u["value"] != "1700001500" {
		t.Errorf("updated filter %v", u)
	}
	if pg := body["pagination"].(map[string]any); pg["starting_after"] != p1.Next {
		t.Errorf("pagination %v", pg)
	}
	c := p2.Items[0]
	if c.Channel != "api" || c.Status != "open" || c.AssigneeID != "991267466" || c.GroupID != "" {
		t.Errorf("ticket 103 %+v", c)
	}
	if !strings.HasPrefix(c.Subject, "Welcome aboard!") || !strings.HasSuffix(c.Subject, "…") || len([]rune(c.Subject)) > subjectLen+1 {
		t.Errorf("subject from message %q", c.Subject)
	}
	// The contact is gone and an admin started it: no requester.
	if c.Requester != nil {
		t.Errorf("requester %+v", c.Requester)
	}
}

func TestLoad(t *testing.T) {
	f := newFake(t)
	s := f.source(t, "good-token")
	tk := &source.Ticket{ID: "101"}
	if err := s.Load(context.Background(), tk); err != nil {
		t.Fatal(err)
	}
	if !tk.Loaded {
		t.Error("not loaded")
	}
	ms := tk.Messages
	if len(ms) != 6 {
		for _, m := range ms {
			t.Logf("%+v", m)
		}
		t.Fatalf("%d messages, want 6", len(ms))
	}
	first := ms[0]
	if first.Agent || first.Note || first.HTML != "<p>Hi, I want a refund.</p>" || !first.CreatedAt.Equal(time.Unix(1700000000, 0)) ||
		first.Author != (source.Person{ID: "c1", Name: "Jo Customer", Email: "jo@customer.test"}) {
		t.Errorf("source message %+v", first)
	}
	if len(first.Attachments) != 1 || first.Attachments[0].Name != "receipt.pdf" || first.Attachments[0].ContentType != "application/pdf" ||
		first.Attachments[0].Size != 2048 || first.Attachments[0].URL != "https://downloads.intercomcdn.test/receipt.pdf" || first.Attachments[0].ID == "" {
		t.Errorf("source attachments %+v", first.Attachments)
	}
	bot := ms[1]
	if !bot.Agent || bot.AuthorID != "" || bot.Author.Name != "Fin" || bot.Note {
		t.Errorf("bot message %+v", bot)
	}
	note := ms[2]
	if !note.Agent || !note.Note || note.AuthorID != "991267466" || note.Author.Email != "sam@acme.test" {
		t.Errorf("note %+v", note)
	}
	files := ms[3]
	if files.Agent || files.HTML != "" || len(files.Attachments) != 1 || files.Attachments[0].Name != "screen.png" {
		t.Errorf("attachment only part %+v", files)
	}
	if reply := ms[4]; !reply.Agent || reply.Note || reply.AuthorID != "991267466" || reply.HTML != "<p>Refunded.</p>" {
		t.Errorf("reply %+v", reply)
	}
	if closing := ms[5]; closing.ID != "6" || closing.HTML != "<p>Closing, thanks!</p>" || !closing.Agent {
		t.Errorf("close with body %+v", closing)
	}
	for i := 1; i < len(ms); i++ {
		if ms[i].CreatedAt.Before(ms[i-1].CreatedAt) {
			t.Errorf("message %d out of order", i)
		}
	}
}

func TestArticles(t *testing.T) {
	f := newFake(t)
	s := f.source(t, "good-token")
	ctx := context.Background()
	p1, err := s.Articles(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if p1.Next != "2" || len(p1.Items) != 2 {
		t.Fatalf("page 1: next %q, %d items", p1.Next, len(p1.Items))
	}
	if a := p1.Items[0]; a.ID != "39" || a.Title != "Billing FAQ" || a.HTML != "<p>How billing works</p>" || !a.Published || a.Collection != "Payments" ||
		!a.UpdatedAt.Equal(time.Unix(1700000100, 0)) {
		t.Errorf("article 39 %+v", a)
	}
	if a := p1.Items[1]; a.Published || a.Collection != "Billing" {
		t.Errorf("article 40 %+v", a)
	}
	p2, err := s.Articles(ctx, p1.Next)
	if err != nil {
		t.Fatal(err)
	}
	if p2.Next != "" || len(p2.Items) != 1 || p2.Items[0].Collection != "" {
		t.Errorf("page 2 %+v", p2)
	}
	if n := f.hit("/help_center/collections"); n != 2 {
		t.Errorf("collections read %d times, want 2 (two pages, once)", n)
	}
}

func TestRateLimited(t *testing.T) {
	f := newFake(t)
	f.limited, f.reset = true, time.Now().Add(42*time.Second).Unix()
	_, err := f.source(t, "good-token").Check(context.Background())
	var rl *source.RateLimited
	if !errors.As(err, &rl) {
		t.Fatalf("got %v, want RateLimited", err)
	}
	if rl.RetryAfter < 40*time.Second || rl.RetryAfter > 44*time.Second {
		t.Errorf("retry after %s, want about 42s", rl.RetryAfter)
	}
}

func TestDownload(t *testing.T) {
	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("credentials sent to the file host")
		}
		_, _ = io.WriteString(w, "PDF")
	}))
	defer files.Close()
	f := newFake(t)
	rc, err := f.source(t, "good-token").Download(context.Background(), source.Attachment{URL: files.URL + "/receipt.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if string(b) != "PDF" {
		t.Errorf("read %q", b)
	}
}
