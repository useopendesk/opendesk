package freshdesk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/useopendesk/opendesk/internal/helpdesk/imports/source"
)

const goodKey = "fd-key-123"

var base = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

type fixture struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	hits    map[string]int
	tickets []map[string]any
	limited bool
	unknown bool
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func newFixture(t *testing.T) *fixture {
	f := &fixture{t: t, hits: map[string]int{}}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits[r.URL.Path]++
		limited, unknown := f.limited, f.unknown
		f.mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/files/") {
			if _, _, ok := r.BasicAuth(); !ok {
				t.Errorf("file request without credentials on the API host")
			}
			mux.ServeHTTP(w, r)
			return
		}
		if unknown {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if limited {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if u, p, ok := r.BasicAuth(); !ok || u != goodKey || p != "X" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":"invalid_credentials","message":"You have to be logged in to perform this action."}`)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.srv.Close)

	js := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	page := func(r *http.Request) (int, int) {
		p, _ := strconv.Atoi(r.URL.Query().Get("page"))
		n, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		if p < 1 {
			p = 1
		}
		if n < 1 {
			n = 30
		}
		return p, n
	}
	slice := func(r *http.Request, all []map[string]any) []map[string]any {
		p, n := page(r)
		lo := (p - 1) * n
		if lo > len(all) {
			lo = len(all)
		}
		hi := lo + n
		if hi > len(all) {
			hi = len(all)
		}
		return all[lo:hi]
	}

	mux.HandleFunc("GET /api/v2/agents/me", func(w http.ResponseWriter, r *http.Request) {
		js(w, map[string]any{"id": 501, "contact": map[string]any{"name": "Ada Agent", "email": "ada@acme.test"}})
	})
	mux.HandleFunc("GET /api/v2/account", func(w http.ResponseWriter, r *http.Request) {
		js(w, map[string]any{"account_name": "Acme Support", "account_domain": "acme.freshdesk.com"})
	})
	mux.HandleFunc("GET /api/v2/search/tickets", func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query().Get("query"); q != `"created_at:>'1970-01-01'"` {
			t.Errorf("search query %q", q)
		}
		js(w, map[string]any{"total": 1234, "results": []any{}})
	})
	mux.HandleFunc("GET /api/v2/agents", func(w http.ResponseWriter, r *http.Request) {
		var all []map[string]any
		for i := 0; i < 101; i++ {
			all = append(all, map[string]any{"id": 500 + i, "contact": map[string]any{
				"name": fmt.Sprintf("Agent %d", i), "email": fmt.Sprintf("agent%d@acme.test", i)}})
		}
		all[1]["contact"] = map[string]any{"name": "Ada Agent", "email": "ada@acme.test"} // id 501
		js(w, slice(r, all))
	})
	mux.HandleFunc("GET /api/v2/groups", func(w http.ResponseWriter, r *http.Request) {
		js(w, slice(r, []map[string]any{{"id": 9001, "name": "Billing"}, {"id": 9002, "name": " Tier 2 "}}))
	})
	mux.HandleFunc("GET /api/v2/companies/{id}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("id") {
		case "77":
			js(w, map[string]any{"id": 77, "name": "Globex", "domains": []string{"globex.test"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("GET /api/v2/tickets", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("order_by") != "updated_at" || q.Get("order_type") != "asc" || !strings.Contains(q.Get("include"), "requester") {
			t.Errorf("ticket listing query %v", q)
		}
		p, _ := page(r)
		if p > maxPage {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		since, err := time.Parse(time.RFC3339, q.Get("updated_since"))
		if err != nil {
			t.Errorf("updated_since %q", q.Get("updated_since"))
		}
		f.mu.Lock()
		var match []map[string]any
		for _, tk := range f.tickets {
			if u, _ := time.Parse(time.RFC3339, tk["updated_at"].(string)); !u.Before(since) {
				match = append(match, tk)
			}
		}
		f.mu.Unlock()
		sort.SliceStable(match, func(i, j int) bool {
			a, b := match[i]["updated_at"].(string), match[j]["updated_at"].(string)
			if a != b {
				return a < b
			}
			return match[i]["id"].(int) < match[j]["id"].(int)
		})
		js(w, slice(r, match))
	})
	mux.HandleFunc("GET /api/v2/tickets/{id}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("id") {
		case "5":
			js(w, map[string]any{
				"id": 5, "subject": "Printer on fire", "status": 2, "priority": 3, "source": 1,
				"requester_id": 9, "responder_id": 501, "created_at": ts(base),
				"description":      "<p>It is on fire</p>",
				"description_text": "It is on fire",
				"attachments": []any{map[string]any{"id": 31, "name": "fire.jpg", "content_type": "image/jpeg",
					"size": 2048, "attachment_url": f.srv.URL + "/files/fire.jpg"}},
			})
		case "6":
			js(w, map[string]any{"id": 6, "status": 2, "source": 1, "requester_id": 9, "created_at": ts(base),
				"description": "x", "deleted": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("GET /api/v2/tickets/{id}/conversations", func(w http.ResponseWriter, r *http.Request) {
		var all []map[string]any
		if r.PathValue("id") == "5" {
			for i := 0; i < 102; i++ {
				c := map[string]any{"id": 1000 + i, "body": fmt.Sprintf("<p>reply %d</p>", i), "body_text": fmt.Sprintf("reply %d", i),
					"incoming": i%2 == 1, "private": false, "user_id": 9, "from_email": "Wile E <wile@acme-corp.test>",
					"to_emails": []string{"support@acme.test", ""}, "cc_emails": []string{}, "created_at": ts(base.Add(time.Duration(i+1) * time.Minute))}
				if i%2 == 0 {
					c["user_id"] = 501
					c["from_email"] = "support@acme.test"
				}
				all = append(all, c)
			}
			// Listed out of order: the source sorts.
			all[3]["created_at"] = ts(base.Add(500 * time.Minute))
			all[10]["private"] = true
			all[10]["incoming"] = false
			all[10]["user_id"] = 501
			all[20]["incoming"] = false
			all[20]["user_id"] = 888 // an agent no longer listed
			all[20]["from_email"] = "Old Agent <old@acme.test>"
			all[101]["attachments"] = []any{map[string]any{"id": 32, "name": "log.txt", "content_type": "text/plain",
				"file_size": 12, "attachment_url": f.srv.URL + "/files/log.txt"}}
		}
		js(w, slice(r, all))
	})
	mux.HandleFunc("GET /files/{name}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "file:"+r.PathValue("name"))
	})

	// Solutions: category 1 has folder 11 (101 articles, with subfolder 13) and folder 12
	// (empty); category 2 has folder 21 (one article).
	folders := map[string][]map[string]any{
		"1": {{"id": 11, "name": "Billing", "articles_count": 101, "sub_folders_count": 1},
			{"id": 12, "name": "Empty", "articles_count": 0}},
		"2": {{"id": 21, "name": "Setup", "articles_count": 1}},
	}
	articles := map[string][]map[string]any{"21": {{"id": 2101, "title": "Install", "description": "<p>Run it</p>", "status": 2,
		"created_at": ts(base), "updated_at": ts(base)}}, "13": {{"id": 1301, "title": "Refunds", "description": "<p>r</p>", "status": 1}}}
	for i := 0; i < 101; i++ {
		articles["11"] = append(articles["11"], map[string]any{"id": 1100 + i, "title": fmt.Sprintf("Billing %d", i),
			"description": "<p>b</p>", "status": 2})
	}
	mux.HandleFunc("GET /api/v2/solutions/categories", func(w http.ResponseWriter, r *http.Request) {
		js(w, []map[string]any{{"id": 1, "name": "General"}, {"id": 2, "name": "Getting started"}})
	})
	mux.HandleFunc("GET /api/v2/solutions/categories/{id}/folders", func(w http.ResponseWriter, r *http.Request) {
		js(w, folders[r.PathValue("id")])
	})
	mux.HandleFunc("GET /api/v2/solutions/folders/{id}/subfolders", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "11" {
			t.Errorf("subfolders of %s", r.PathValue("id"))
		}
		js(w, []map[string]any{{"id": 13, "name": "Refunds", "articles_count": 1}})
	})
	mux.HandleFunc("GET /api/v2/solutions/folders/{id}/articles", func(w http.ResponseWriter, r *http.Request) {
		js(w, slice(r, articles[r.PathValue("id")]))
	})
	return f
}

func (f *fixture) source(t *testing.T, key string) *src {
	t.Helper()
	s, err := New(source.Credentials{"domain": "https://Acme.freshdesk.com/a/tickets", "api_key": key},
		source.Options{Dev: true, BaseURL: f.srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return s.(*src)
}

func (f *fixture) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

func TestNewValidation(t *testing.T) {
	cases := []struct {
		c     source.Credentials
		field string
	}{
		{source.Credentials{"api_key": "k"}, "domain"},
		{source.Credentials{"domain": "ac_me", "api_key": "k"}, "domain"},
		{source.Credentials{"domain": "acme", "api_key": "  "}, "api_key"},
	}
	for _, c := range cases {
		_, err := New(c.c, source.Options{})
		var inv *source.InvalidError
		if !errors.As(err, &inv) || inv.Field != c.field || !errors.Is(err, source.ErrInvalid) {
			t.Errorf("%v: got %v, want invalid %s", c.c, err, c.field)
		}
	}
	s, err := New(source.Credentials{"domain": "acme.freshdesk.com", "api_key": "k"}, source.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.(*src); got.domain != "acme" || got.c.Base != "https://acme.freshdesk.com" {
		t.Errorf("domain %q base %q", got.domain, got.c.Base)
	}
}

func TestCheck(t *testing.T) {
	f := newFixture(t)
	scan, err := f.source(t, goodKey).Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.Account != (source.Account{Key: "acme", Name: "Acme Support", URL: "https://acme.freshdesk.com"}) {
		t.Errorf("account %+v", scan.Account)
	}
	if scan.Tickets != 1234 {
		t.Errorf("tickets %d", scan.Tickets)
	}
	if len(scan.Agents) != 101 || scan.Agents[1] != (source.Agent{ID: "501", Name: "Ada Agent", Email: "ada@acme.test"}) {
		t.Errorf("agents %d %+v", len(scan.Agents), scan.Agents[:2])
	}
	if len(scan.Groups) != 2 || scan.Groups[1] != (source.Group{ID: "9002", Name: "Tier 2"}) {
		t.Errorf("groups %+v", scan.Groups)
	}
	if !scan.HasArticles || scan.Articles != 103 {
		t.Errorf("articles %v %d", scan.HasArticles, scan.Articles)
	}
}

func TestCheckRefused(t *testing.T) {
	f := newFixture(t)
	if _, err := f.source(t, "wrong").Check(context.Background()); !errors.Is(err, source.ErrAuth) {
		t.Errorf("wrong key: %v", err)
	}
	f.unknown = true
	if _, err := f.source(t, goodKey).Check(context.Background()); !errors.Is(err, source.ErrNotFound) {
		t.Errorf("unknown domain: %v", err)
	}
}

func TestRateLimited(t *testing.T) {
	f := newFixture(t)
	f.limited = true
	_, err := f.source(t, goodKey).Tickets(context.Background(), "", nil, nil)
	var rl *source.RateLimited
	if !errors.As(err, &rl) || rl.RetryAfter != 7*time.Second {
		t.Fatalf("got %v", err)
	}
	if _, err := f.source(t, goodKey).Check(context.Background()); !errors.As(err, &rl) {
		t.Fatalf("check got %v", err)
	}
}

func ticket(id int, created, updated time.Time) map[string]any {
	return map[string]any{"id": id, "subject": fmt.Sprintf("Ticket %d", id), "status": 2, "priority": 2, "source": 1,
		"requester_id": 9, "requester": map[string]any{"id": 9, "name": "Wile E", "email": "wile@acme-corp.test", "mobile": "+1 555"},
		"tags": []string{"vip", " "}, "created_at": ts(created), "updated_at": ts(updated)}
}

func all(t *testing.T, s *src, since, updatedSince *time.Time) ([]source.Ticket, []string) {
	t.Helper()
	var out []source.Ticket
	var cursors []string
	cur := ""
	for i := 0; i < 50; i++ {
		p, err := s.Tickets(context.Background(), cur, since, updatedSince)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, p.Items...)
		if p.Next == "" {
			return out, cursors
		}
		cursors = append(cursors, p.Next)
		cur = p.Next
	}
	t.Fatal("listing did not end")
	return nil, nil
}

func TestTickets(t *testing.T) {
	f := newFixture(t)
	for i := 1; i <= 150; i++ {
		f.tickets = append(f.tickets, ticket(i, base.Add(time.Duration(i)*time.Hour), base.Add(time.Duration(i)*time.Hour+time.Minute)))
	}
	tk := func(id int) map[string]any { return f.tickets[id-1] }
	tk(1)["status"], tk(1)["priority"], tk(1)["source"] = 3, 1, 2
	tk(2)["status"], tk(2)["priority"], tk(2)["source"] = 4, 3, 3
	tk(3)["status"], tk(3)["priority"], tk(3)["source"] = 5, 4, 7
	tk(4)["status"], tk(4)["source"] = 6, 9
	tk(5)["source"] = 10
	tk(6)["spam"] = true
	tk(7)["deleted"] = true
	for _, id := range []int{1, 2, 3, 120} {
		tk(id)["company_id"] = 77
	}
	tk(8)["company_id"] = 78
	tk(2)["responder_id"], tk(2)["group_id"] = 501, 9001
	tk(2)["stats"] = map[string]any{"resolved_at": ts(base.Add(3 * time.Hour)), "closed_at": nil}
	tk(3)["stats"] = map[string]any{"resolved_at": nil, "closed_at": ts(base.Add(4 * time.Hour))}

	s := f.source(t, goodKey)
	got, cursors := all(t, s, nil, nil)
	ids := map[string]source.Ticket{}
	for _, x := range got {
		ids[x.ID] = x
	}
	if len(ids) != 150 {
		t.Fatalf("got %d distinct tickets, want 150", len(ids))
	}
	if len(cursors) != 1 {
		t.Errorf("cursors %v", cursors)
	}
	c, err := readCursor(cursors[0])
	if err != nil || !c.U.Equal(source.Time(tk(100)["updated_at"].(string)).Add(-time.Second)) || c.P != 1 {
		t.Errorf("cursor %+v %v", c, err)
	}

	want := []struct {
		id                        string
		status, priority, channel string
	}{
		{"1", "pending", "low", "portal"}, {"2", "solved", "high", "api"}, {"3", "closed", "urgent", "chat"},
		{"4", "pending", "normal", "portal"}, {"5", "open", "normal", "email"}, {"9", "open", "normal", "email"},
	}
	for _, w := range want {
		x := ids[w.id]
		if x.Status != w.status || x.Priority != w.priority || x.Channel != w.channel {
			t.Errorf("ticket %s: %s %s %s", w.id, x.Status, x.Priority, x.Channel)
		}
	}
	if !ids["6"].Skip || !ids["7"].Skip || ids["5"].Skip {
		t.Errorf("skip: spam %v deleted %v", ids["6"].Skip, ids["7"].Skip)
	}
	two := ids["2"]
	if two.Ref != "#2" || two.URL != "https://acme.freshdesk.com/a/tickets/2" || two.AssigneeID != "501" || two.GroupID != "9001" ||
		two.SolvedAt == nil || !two.SolvedAt.Equal(base.Add(3*time.Hour)) || len(two.Tags) != 1 || two.Tags[0] != "vip" {
		t.Errorf("ticket 2 %+v", two)
	}
	if ids["3"].SolvedAt == nil || !ids["3"].SolvedAt.Equal(base.Add(4*time.Hour)) {
		t.Errorf("ticket 3 solved at %v", ids["3"].SolvedAt)
	}
	r := two.Requester
	if r == nil || *r != (source.Person{ID: "9", Name: "Wile E", Email: "wile@acme-corp.test", Phone: "+1 555", Company: "Globex", Domain: "globex.test"}) {
		t.Errorf("requester %+v", r)
	}
	if ids["8"].Requester.Company != "" || ids["9"].Requester.Company != "" {
		t.Errorf("company on 8 or 9")
	}
	if n := f.count("/api/v2/companies/77"); n != 1 {
		t.Errorf("company 77 read %d times", n)
	}
	if n := f.count("/api/v2/companies/78"); n != 1 {
		t.Errorf("company 78 read %d times", n)
	}

	// since keeps tickets created on or after it.
	since := base.Add(140 * time.Hour)
	got, _ = all(t, f.source(t, goodKey), &since, nil)
	if len(got) != 11 || got[0].ID != "140" {
		t.Errorf("since: %d tickets, first %v", len(got), got)
	}
}

func TestTicketsRollover(t *testing.T) {
	f := newFixture(t)
	// 250 tickets share one second, then 30 follow: the cursor pages through the second
	// and then moves updated_since past it.
	same := base.Add(time.Hour)
	for i := 1; i <= 250; i++ {
		f.tickets = append(f.tickets, ticket(i, base, same))
	}
	for i := 251; i <= 280; i++ {
		f.tickets = append(f.tickets, ticket(i, base, same.Add(time.Duration(i)*time.Minute)))
	}
	got, cursors := all(t, f.source(t, goodKey), nil, nil)
	ids := map[string]bool{}
	for _, x := range got {
		ids[x.ID] = true
	}
	if len(ids) != 280 {
		t.Fatalf("got %d distinct tickets, want 280", len(ids))
	}
	var ps []int
	for _, c := range cursors {
		cur, _ := readCursor(c)
		ps = append(ps, cur.P)
	}
	if fmt.Sprint(ps) != "[1 2 3]" {
		t.Errorf("pages %v", ps)
	}

	// At the last page Freshdesk serves, the cursor moves past the second.
	c := cursor{U: same, P: maxPage}
	if n := c.next(same); n.P != 1 || !n.U.Equal(same.Add(time.Second)) {
		t.Errorf("at the cap: %+v", n)
	}
	if n := (cursor{U: same, P: 4}).next(same.Add(time.Minute)); n.P != 1 || !n.U.Equal(same.Add(59*time.Second)) {
		t.Errorf("moving on: %+v", n)
	}
	if _, err := f.source(t, goodKey).Tickets(context.Background(), "nonsense", nil, nil); err == nil {
		t.Error("malformed cursor accepted")
	}

	// A sync starts from updatedSince.
	from := same.Add(260 * time.Minute)
	got, _ = all(t, f.source(t, goodKey), nil, &from)
	if len(got) != 21 || got[0].ID != "260" {
		t.Errorf("updated since: %d", len(got))
	}
}

func TestLoad(t *testing.T) {
	f := newFixture(t)
	s := f.source(t, goodKey)
	req := &source.Person{ID: "9", Name: "Wile E", Email: "wile@acme-corp.test"}
	tk := &source.Ticket{ID: "5", CreatedAt: base, Requester: req}
	if err := s.Load(context.Background(), tk); err != nil {
		t.Fatal(err)
	}
	if !tk.Loaded || len(tk.Messages) != 103 {
		t.Fatalf("loaded %v, %d messages", tk.Loaded, len(tk.Messages))
	}
	for i := 1; i < len(tk.Messages); i++ {
		if tk.Messages[i].CreatedAt.Before(tk.Messages[i-1].CreatedAt) {
			t.Fatalf("message %d out of order", i)
		}
	}
	d := tk.Messages[0]
	if d.ID != "description-5" || d.Agent || d.Note || d.HTML != "<p>It is on fire</p>" || d.Text != "It is on fire" ||
		d.Author.Email != req.Email || !d.CreatedAt.Equal(base) || len(d.Attachments) != 1 {
		t.Errorf("description %+v", d)
	}
	if a := d.Attachments[0]; a.ID != "31" || a.Name != "fire.jpg" || a.ContentType != "image/jpeg" || a.Size != 2048 {
		t.Errorf("attachment %+v", a)
	}
	byID := map[string]source.Message{}
	for _, m := range tk.Messages {
		byID[m.ID] = m
	}
	agent := byID["1000"]
	if !agent.Agent || agent.Note || agent.AuthorID != "501" || agent.Author.Name != "Ada Agent" || agent.HTML != "<p>reply 0</p>" ||
		len(agent.To) != 1 || agent.To[0] != "support@acme.test" {
		t.Errorf("agent reply %+v", agent)
	}
	customer := byID["1001"]
	if customer.Agent || customer.Note || customer.AuthorID != "" || customer.Author.Email != "wile@acme-corp.test" || customer.Author.ID != "9" {
		t.Errorf("customer reply %+v", customer)
	}
	note := byID["1010"]
	if !note.Note || !note.Agent || note.AuthorID != "501" {
		t.Errorf("note %+v", note)
	}
	old := byID["1020"]
	if !old.Agent || old.AuthorID != "" || old.Author.Name != "Old Agent" || old.Author.Email != "old@acme.test" {
		t.Errorf("unlisted agent %+v", old)
	}
	last := byID["1101"]
	if len(last.Attachments) != 1 || last.Attachments[0].Size != 12 {
		t.Fatalf("paged conversation %+v", last)
	}
	if tk.Messages[len(tk.Messages)-1].ID != "1003" {
		t.Errorf("latest is %s", tk.Messages[len(tk.Messages)-1].ID)
	}

	rc, err := s.Download(context.Background(), last.Attachments[0])
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != "file:log.txt" {
		t.Errorf("download %q", b)
	}

	gone := &source.Ticket{ID: "6", Requester: req}
	if err := s.Load(context.Background(), gone); err != nil || !gone.Skip {
		t.Errorf("deleted ticket: %v skip %v", err, gone.Skip)
	}
	if err := s.Load(context.Background(), &source.Ticket{ID: "../agents"}); err == nil {
		t.Error("malformed id accepted")
	}
}

func TestArticles(t *testing.T) {
	f := newFixture(t)
	s := f.source(t, goodKey)
	var got []source.Article
	var cursors []string
	cur := ""
	for i := 0; i < 20; i++ {
		p, err := s.Articles(context.Background(), cur)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, p.Items...)
		if p.Next == "" {
			break
		}
		cursors = append(cursors, p.Next)
		cur = p.Next
	}
	if len(got) != 103 {
		t.Fatalf("got %d articles", len(got))
	}
	if got[0].Collection != "Billing" || !got[0].Published || got[100].ID != "1200" {
		t.Errorf("first %+v, 101st %+v", got[0], got[100])
	}
	if r := got[101]; r.ID != "1301" || r.Collection != "Refunds" || r.Published {
		t.Errorf("subfolder article %+v", r)
	}
	if in := got[102]; in.ID != "2101" || in.Collection != "Setup" || in.HTML != "<p>Run it</p>" || !in.CreatedAt.Equal(base) {
		t.Errorf("last %+v", in)
	}

	// A fresh source resumes from a cursor (after a restart).
	again := f.source(t, goodKey)
	p, err := again.Articles(context.Background(), cursors[0])
	if err != nil || len(p.Items) != 1 || p.Items[0].ID != "1200" {
		t.Errorf("resume: %v %+v", err, p.Items)
	}
}

func TestArticlesRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/agents/me" {
			_, _ = io.WriteString(w, `{"id":1}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	s, _ := New(source.Credentials{"domain": "acme", "api_key": goodKey}, source.Options{Dev: true, BaseURL: srv.URL})
	if _, err := s.Articles(context.Background(), ""); !errors.Is(err, source.ErrUnsupported) {
		t.Errorf("got %v", err)
	}
}
