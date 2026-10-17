package helpscout

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/useopendesk/opendesk/internal/helpdesk/imports/source"
)

// fake serves the Inbox API and the Docs API from one server, shaped like Help Scout's docs.
type fake struct {
	mu        sync.Mutex
	tokens    int    // tokens issued
	valid     string // the token the API accepts
	rejectOne bool   // refuse the next valid token once, as an expired one
	queries   map[string]string
	limited   bool
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/hal+json")
	if f.limited {
		w.Header().Set("X-RateLimit-Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	if r.URL.Path == "/v2/oauth2/token" {
		_ = r.ParseForm()
		if r.Method != http.MethodPost || r.PostForm.Get("grant_type") != "client_credentials" ||
			r.PostForm.Get("client_id") != "app" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_request"}`)
			return
		}
		if r.PostForm.Get("client_secret") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"invalid_client"}`)
			return
		}
		f.tokens++
		f.valid = fmt.Sprintf("tok-%d", f.tokens)
		fmt.Fprintf(w, `{"token_type":"bearer","access_token":%q,"expires_in":172800}`, f.valid)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/") {
		if u, p, ok := r.BasicAuth(); !ok || u != "docs-key" || p != "X" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.docs(w, r)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+f.valid || f.valid == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if f.rejectOne {
		f.rejectOne = false
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	f.inbox(w, r)
}

func hal(key, items string, number, totalElements, totalPages int) string {
	return fmt.Sprintf(`{"_embedded":{%q:[%s]},"_links":{"self":{"href":"..."}},"page":{"number":%d,"size":25,"totalElements":%d,"totalPages":%d}}`,
		key, items, number, totalElements, totalPages)
}

const thread10 = `[
 {"id":104,"type":"lineitem","status":"closed","state":"published","action":{"type":"default","text":"Marked as active"},"createdBy":{"id":4,"type":"user"},"createdAt":"2024-03-01T12:00:00Z"},
 {"id":103,"type":"note","status":"active","state":"published","body":"<p>Checked the logs</p>","createdBy":{"id":4,"type":"user","first":"Vernon","last":"Bear","email":"bear@acme.com"},"createdAt":"2024-03-01T11:00:00Z"},
 {"id":105,"type":"message","status":"active","state":"draft","body":"<p>unsent</p>","createdBy":{"id":4,"type":"user"},"createdAt":"2024-03-01T13:00:00Z"},
 {"id":102,"type":"message","status":"active","state":"published","body":"<p>Fixed it</p>","createdBy":{"id":4,"type":"user","first":"Vernon","last":"Bear","email":"bear@acme.com"},"to":["rob@example.com"],"cc":["boss@example.com"],"createdAt":"2024-03-01T10:30:00Z","rating":{"customerId":238604,"rating":"great","comments":"Thanks"}},
 {"id":101,"type":"customer","status":"active","state":"published","body":"<p>It broke</p>","customer":{"id":238604,"first":"Rob","last":"Robertovic","email":"rob@example.com"},"createdBy":{"id":238604,"type":"customer","email":"rob@example.com"},"to":["help@acme.com"],"createdAt":"2024-03-01T10:00:00Z",
  "_embedded":{"attachments":[{"_links":{"self":{"href":"..."},"data":{"href":"..."},"download":{"href":"..."}},"id":500,"filename":"photo1.jpg","mimeType":"image/jpeg","width":600,"height":800,"size":20191,"state":"valid"}]}}
]`

func conv(id, number int, typ, status, state string, mailbox int, threads string, count int, extra string) string {
	return fmt.Sprintf(`{"id":%d,"number":%d,"type":%q,"status":%q,"state":%q,"subject":"Subject %d","mailboxId":%d,"threads":%d,
 "assignee":{"id":4,"type":"user","first":"Vernon","last":"Bear","email":"bear@acme.com"},
 "createdBy":{"id":238604,"type":"customer","email":"rob@example.com"},
 "createdAt":"2024-03-%02dT10:00:00Z","userUpdatedAt":"2024-03-%02dT14:00:00Z"%s,
 "primaryCustomer":{"id":238604,"type":"customer","first":"Rob","last":"Robertovic","email":"rob@example.com"},
 "tags":[{"id":9150,"color":"#929499","tag":"vip"},{"id":9151,"tag":" "}],
 "source":{"type":"email","via":"customer"},
 "_embedded":{"threads":%s}}`, id, number, typ, status, state, id, mailbox, count, id%10+1, id%10+1, extra, threads)
}

func (f *fake) inbox(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pg := q.Get("page")
	switch r.URL.Path {
	case "/v2/users/me":
		_, _ = io.WriteString(w, `{"_links":{"self":{"href":"..."}},"id":4,"firstName":"Vernon","lastName":"Bear","email":"bear@acme.com","role":"owner","type":"user","companyId":77}`)
	case "/v2/users":
		if pg == "2" {
			_, _ = io.WriteString(w, hal("users", `{"id":5,"firstName":"","lastName":"","email":"ann@acme.com","type":"user"}`, 2, 2, 2))
			return
		}
		_, _ = io.WriteString(w, hal("users", `{"id":4,"firstName":"Vernon","lastName":"Bear","email":"bear@acme.com","role":"owner","type":"user"}`, 1, 2, 2))
	case "/v2/mailboxes":
		_, _ = io.WriteString(w, hal("mailboxes", `{"id":2,"name":"Billing","slug":"b","email":"billing@acme.com"},{"id":1,"name":"Acme Support","slug":"a","email":"help@acme.com"}`, 1, 2, 1))
	case "/v2/conversations":
		if f.queries == nil {
			f.queries = map[string]string{}
		}
		for _, k := range []string{"status", "embed", "sortField", "sortOrder", "query", "modifiedSince"} {
			f.queries[k] = q.Get(k)
		}
		if q.Get("embed") == "" {
			_, _ = io.WriteString(w, hal("conversations", conv(1, 1, "email", "active", "published", 1, "[]", 0, ""), 1, 42, 2))
			return
		}
		if pg == "2" {
			chat := `[{"id":302,"type":"chat","state":"published","body":"trunc","createdBy":{"id":4,"type":"user"},"createdAt":"2024-03-03T10:05:00Z"},{"id":301,"type":"chat","state":"published","body":"trunc","createdBy":{"id":238604,"type":"customer"},"createdAt":"2024-03-03T10:00:00Z"}]`
			phone := `[{"id":401,"type":"phone","state":"published","body":"Called about billing","createdBy":{"id":5,"type":"user","email":"ann@acme.com"},"createdAt":"2024-03-04T10:00:00Z"}]`
			_, _ = io.WriteString(w, hal("conversations",
				conv(12, 3, "chat", "closed", "published", 2, chat, 2, `,"closedAt":"2024-03-03T15:00:00Z","closedBy":4`)+","+
					conv(13, 4, "phone", "pending", "published", 1, phone, 1, ""), 2, 4, 2))
			return
		}
		_, _ = io.WriteString(w, hal("conversations",
			conv(10, 1, "email", "active", "published", 1, thread10, 5, "")+","+
				conv(11, 2, "email", "spam", "published", 1, "[]", 0, ""), 1, 4, 2))
	case "/v2/conversations/12/threads":
		if pg == "2" {
			_, _ = io.WriteString(w, hal("threads", `{"id":303,"type":"chat","state":"published","body":"Glad to help","createdBy":{"id":4,"type":"user","first":"Vernon","last":"Bear","email":"bear@acme.com"},"createdAt":"2024-03-03T10:09:00Z","rating":{"rating":"not good"}}`, 2, 3, 2))
			return
		}
		_, _ = io.WriteString(w, hal("threads",
			`{"id":302,"type":"chat","state":"published","body":"How can I help?","createdBy":{"id":4,"type":"user","first":"Vernon","last":"Bear","email":"bear@acme.com"},"createdAt":"2024-03-03T10:05:00Z"},
			 {"id":301,"type":"chat","state":"published","body":"Hello there","customer":{"id":238604,"first":"Rob","last":"Robertovic","email":"rob@example.com"},"createdBy":{"id":238604,"type":"customer","email":"rob@example.com"},"createdAt":"2024-03-03T10:00:00Z"}`, 1, 3, 2))
	case "/v2/conversations/10/attachments/500/data":
		enc := base64.StdEncoding.EncodeToString([]byte("hello? world, this is the file"))
		_, _ = io.WriteString(w, `{ "data" : "`+strings.ReplaceAll(enc, "/", `\/`)+`" }`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fake) docs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pg := q.Get("page")
	ref := func(id, status string) string {
		return fmt.Sprintf(`{"id":%q,"number":1,"collectionId":"c","status":%q,"hasDraft":false,"name":"ref","createdAt":"2023-08-21T19:34:13Z","updatedAt":null}`, id, status)
	}
	switch r.URL.Path {
	case "/v1/collections":
		_, _ = io.WriteString(w, `{"collections":{"page":1,"pages":1,"count":2,"items":[
			{"id":"c1","siteId":"s","number":33,"slug":"getting-started","visibility":"public","order":1,"name":"Getting started","articleCount":3,"publishedArticleCount":2,"createdAt":"2013-08-21T14:01:33Z","updatedAt":"2013-08-21T14:01:33Z"},
			{"id":"c2","siteId":"s","number":34,"slug":"billing","visibility":"public","order":2,"name":"Billing","articleCount":1,"publishedArticleCount":1,"createdAt":"2013-08-22T14:01:33Z","updatedAt":"2013-08-22T14:01:33Z"}]}}`)
	case "/v1/collections/c1/articles":
		if q.Get("status") != "all" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if pg == "2" {
			fmt.Fprintf(w, `{"articles":{"page":2,"pages":2,"count":3,"items":[%s]}}`, ref("a3", "notpublished"))
			return
		}
		fmt.Fprintf(w, `{"articles":{"page":1,"pages":2,"count":3,"items":[%s,%s]}}`, ref("a1", "published"), ref("a2", "published"))
	case "/v1/collections/c2/articles":
		fmt.Fprintf(w, `{"articles":{"page":1,"pages":1,"count":1,"items":[%s,%s]}}`, ref("a4", "published"), ref("gone", "published"))
	default:
		id, ok := strings.CutPrefix(r.URL.Path, "/v1/articles/")
		if !ok || id == "gone" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		status := "published"
		if id == "a3" {
			status = "notpublished"
		}
		fmt.Fprintf(w, `{"article":{"id":%q,"number":121,"slug":"s","status":%q,"hasDraft":false,"name":"Article %s","text":"<p>Body %s</p>","collectionId":"c","categories":[],"related":[],"createdAt":"2023-08-21T19:34:13Z","updatedAt":null}}`, id, status, id, id)
	}
}

func setup(t *testing.T, creds source.Credentials) (*fake, source.Source) {
	t.Helper()
	f := &fake{}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	if creds == nil {
		creds = source.Credentials{"app_id": "app", "app_secret": "secret", "docs_key": "docs-key"}
	}
	s, err := New(creds, source.Options{Dev: true, BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return f, s
}

func TestNewValidates(t *testing.T) {
	for _, c := range []struct {
		creds source.Credentials
		field string
	}{
		{source.Credentials{"app_secret": "s"}, "app_id"},
		{source.Credentials{"app_id": " ", "app_secret": "s"}, "app_id"},
		{source.Credentials{"app_id": "a"}, "app_secret"},
		{source.Credentials{"app_id": "a", "app_secret": "s", "docs_key": strings.Repeat("k", 5000)}, "docs_key"},
	} {
		_, err := New(c.creds, source.Options{})
		var inv *source.InvalidError
		if !errors.As(err, &inv) || inv.Field != c.field || !errors.Is(err, source.ErrInvalid) {
			t.Errorf("New(%v) = %v, want invalid %s", c.creds, err, c.field)
		}
	}
	if _, err := New(source.Credentials{"app_id": "a", "app_secret": "s"}, source.Options{}); err != nil {
		t.Errorf("New without a docs key: %v", err)
	}
}

func TestBadSecret(t *testing.T) {
	for _, creds := range []source.Credentials{
		{"app_id": "app", "app_secret": "wrong-secret-value"},   // 401
		{"app_id": "other", "app_secret": "wrong-secret-value"}, // 400
	} {
		_, s := setup(t, creds)
		_, err := s.Check(context.Background())
		if !errors.Is(err, source.ErrAuth) {
			t.Fatalf("Check with a bad secret = %v, want ErrAuth", err)
		}
		if strings.Contains(err.Error(), "wrong-secret-value") {
			t.Fatalf("the error carries the secret: %v", err)
		}
	}
}

func TestCheck(t *testing.T) {
	f, s := setup(t, nil)
	scan, err := s.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.Account != (source.Account{Key: "77", Name: "Acme Support", URL: "https://secure.helpscout.net"}) {
		t.Errorf("account = %+v", scan.Account)
	}
	if scan.Tickets != 42 {
		t.Errorf("tickets = %d, want 42", scan.Tickets)
	}
	if !scan.HasArticles || scan.Articles != 4 {
		t.Errorf("articles = %v %d, want true 4", scan.HasArticles, scan.Articles)
	}
	want := []source.Agent{{ID: "4", Name: "Vernon Bear", Email: "bear@acme.com"}, {ID: "5", Name: "ann@acme.com", Email: "ann@acme.com"}}
	if fmt.Sprint(scan.Agents) != fmt.Sprint(want) {
		t.Errorf("agents = %+v", scan.Agents)
	}
	if fmt.Sprint(scan.Groups) != fmt.Sprint([]source.Group{{ID: "1", Name: "Acme Support"}, {ID: "2", Name: "Billing"}}) {
		t.Errorf("groups = %+v", scan.Groups)
	}
	if f.tokens != 1 {
		t.Errorf("tokens issued = %d, want the cached one", f.tokens)
	}

	// An expired token is renewed once.
	f.rejectOne = true
	if _, err := s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.tokens != 2 {
		t.Errorf("tokens issued = %d, want 2 after a refusal", f.tokens)
	}
}

func TestCheckDocsKey(t *testing.T) {
	for _, key := range []string{"", "bad-key"} {
		_, s := setup(t, source.Credentials{"app_id": "app", "app_secret": "secret", "docs_key": key})
		scan, err := s.Check(context.Background())
		if err != nil {
			t.Fatalf("docs key %q: %v", key, err)
		}
		if scan.HasArticles || scan.Articles != -1 {
			t.Errorf("docs key %q: articles = %v %d", key, scan.HasArticles, scan.Articles)
		}
		if _, err := s.Articles(context.Background(), ""); key == "" && !errors.Is(err, source.ErrUnsupported) {
			t.Errorf("articles without a key = %v", err)
		} else if key != "" && !errors.Is(err, source.ErrAuth) {
			t.Errorf("articles with a bad key = %v", err)
		}
	}
}

func TestTickets(t *testing.T) {
	f, s := setup(t, nil)
	ctx := context.Background()
	since := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	p, err := s.Tickets(ctx, "", &since, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.queries["status"] != "all" || f.queries["embed"] != "threads" || f.queries["sortField"] != "createdAt" ||
		f.queries["sortOrder"] != "asc" || f.queries["query"] != "(createdAt:[2024-01-02T00:00:00Z TO *])" || f.queries["modifiedSince"] != "" {
		t.Errorf("query = %v", f.queries)
	}
	if p.Next != "2" || len(p.Items) != 2 {
		t.Fatalf("page 1 = next %q, %d items", p.Next, len(p.Items))
	}
	tk := p.Items[0]
	if tk.ID != "10" || tk.Ref != "#1" || tk.URL != "https://secure.helpscout.net/conversation/10/1" || tk.Subject != "Subject 10" ||
		tk.Status != "open" || tk.Skip || tk.Channel != "email" || tk.Priority != "normal" || tk.GroupID != "1" || tk.AssigneeID != "4" ||
		!tk.Loaded || tk.CSAT != 5 || tk.SolvedAt != nil {
		t.Errorf("ticket = %+v", tk)
	}
	if tk.Requester == nil || *tk.Requester != (source.Person{ID: "238604", Email: "rob@example.com", Name: "Rob Robertovic"}) {
		t.Errorf("requester = %+v", tk.Requester)
	}
	if fmt.Sprint(tk.Tags) != "[vip]" || !tk.CreatedAt.Equal(time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)) ||
		!tk.UpdatedAt.Equal(time.Date(2024, 3, 1, 14, 0, 0, 0, time.UTC)) {
		t.Errorf("tags = %v, updated = %v", tk.Tags, tk.UpdatedAt)
	}
	// Oldest first; the line item and the draft are left out.
	if len(tk.Messages) != 3 {
		t.Fatalf("messages = %+v", tk.Messages)
	}
	in, reply, note := tk.Messages[0], tk.Messages[1], tk.Messages[2]
	if in.ID != "101" || in.Agent || in.Note || in.Author.Email != "rob@example.com" || in.Author.Name != "Rob Robertovic" || in.HTML != "<p>It broke</p>" {
		t.Errorf("customer thread = %+v", in)
	}
	if len(in.Attachments) != 1 || fmt.Sprintf("%+v", in.Attachments[0]) != fmt.Sprintf("%+v", source.Attachment{ID: "500", Name: "photo1.jpg", ContentType: "image/jpeg", Size: 20191, URL: in.Attachments[0].URL}) ||
		!strings.HasSuffix(in.Attachments[0].URL, "/v2/conversations/10/attachments/500/data") {
		t.Errorf("attachments = %+v", in.Attachments)
	}
	if reply.ID != "102" || !reply.Agent || reply.Note || reply.AuthorID != "4" || reply.Author.Name != "Vernon Bear" ||
		fmt.Sprint(reply.To, reply.Cc) != "[rob@example.com] [boss@example.com]" {
		t.Errorf("reply = %+v", reply)
	}
	if note.ID != "103" || !note.Agent || !note.Note || note.AuthorID != "4" {
		t.Errorf("note = %+v", note)
	}
	if spam := p.Items[1]; spam.ID != "11" || !spam.Skip {
		t.Errorf("spam = %+v", spam)
	}

	upd := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	p, err = s.Tickets(ctx, p.Next, nil, &upd)
	if err != nil {
		t.Fatal(err)
	}
	if f.queries["modifiedSince"] != "2024-03-01T00:00:00Z" || f.queries["query"] != "" {
		t.Errorf("query = %v", f.queries)
	}
	if p.Next != "" || len(p.Items) != 2 {
		t.Fatalf("page 2 = next %q, %d items", p.Next, len(p.Items))
	}
	chat, phone := p.Items[0], p.Items[1]
	if chat.Status != "closed" || chat.Channel != "chat" || chat.GroupID != "2" || chat.Loaded ||
		chat.SolvedAt == nil || !chat.SolvedAt.Equal(time.Date(2024, 3, 3, 15, 0, 0, 0, time.UTC)) {
		t.Errorf("chat = %+v", chat)
	}
	if err := s.Load(ctx, &chat); err != nil {
		t.Fatal(err)
	}
	if !chat.Loaded || len(chat.Messages) != 3 || chat.CSAT != 1 {
		t.Fatalf("loaded chat = %+v", chat)
	}
	if m := chat.Messages[0]; m.ID != "301" || m.Agent || m.HTML != "Hello there" || m.Author.Name != "Rob Robertovic" {
		t.Errorf("chat customer line = %+v", m)
	}
	if m := chat.Messages[1]; m.ID != "302" || !m.Agent || m.AuthorID != "4" {
		t.Errorf("chat agent line = %+v", m)
	}
	if chat.Messages[2].ID != "303" {
		t.Errorf("chat order = %+v", chat.Messages)
	}
	if phone.Status != "pending" || phone.Channel != "api" || !phone.Loaded || len(phone.Messages) != 1 ||
		!phone.Messages[0].Agent || phone.Messages[0].AuthorID != "5" {
		t.Errorf("phone = %+v", phone)
	}

	if _, err := s.Tickets(ctx, "x", nil, nil); err == nil {
		t.Error("a malformed cursor was accepted")
	}
}

func TestStatuses(t *testing.T) {
	var s helpscout
	for status, want := range map[string]string{"active": "open", "pending": "pending", "closed": "closed"} {
		if got := s.ticket(conversation{Status: status, State: "published"}); got.Status != want || got.Skip {
			t.Errorf("%s = %s skip %v, want %s", status, got.Status, got.Skip, want)
		}
	}
	if !s.ticket(conversation{Status: "spam"}).Skip {
		t.Error("spam is not skipped")
	}
	if !s.ticket(conversation{Status: "closed", State: "deleted"}).Skip {
		t.Error("deleted is not skipped")
	}
}

func TestArticles(t *testing.T) {
	_, s := setup(t, nil)
	ctx := context.Background()
	var got []source.Article
	var cursors []string
	cursor := ""
	for i := 0; i < 10; i++ {
		p, err := s.Articles(ctx, cursor)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, p.Items...)
		cursors = append(cursors, p.Next)
		if p.Next == "" {
			break
		}
		cursor = p.Next
	}
	if fmt.Sprint(cursors) != "[c1:2 c2:1 ]" {
		t.Errorf("cursors = %v", cursors)
	}
	if len(got) != 4 {
		t.Fatalf("articles = %+v", got)
	}
	a := got[0]
	if a.ID != "a1" || a.Title != "Article a1" || a.HTML != "<p>Body a1</p>" || a.Collection != "Getting started" || !a.Published ||
		!a.CreatedAt.Equal(time.Date(2023, 8, 21, 19, 34, 13, 0, time.UTC)) || !a.UpdatedAt.Equal(a.CreatedAt) {
		t.Errorf("article = %+v", a)
	}
	if got[2].ID != "a3" || got[2].Published {
		t.Errorf("draft article = %+v", got[2])
	}
	if got[3].ID != "a4" || got[3].Collection != "Billing" {
		t.Errorf("second collection = %+v", got[3])
	}
	if _, err := s.Articles(ctx, "nonsense"); err == nil {
		t.Error("a malformed cursor was accepted")
	}
}

func TestRateLimited(t *testing.T) {
	f, s := setup(t, nil)
	f.limited = true
	_, err := s.Check(context.Background())
	var rl *source.RateLimited
	if !errors.As(err, &rl) || rl.RetryAfter != 7*time.Second {
		t.Fatalf("Check = %v, want rate limited for 7s", err)
	}
}

func TestDownload(t *testing.T) {
	_, s := setup(t, nil)
	ctx := context.Background()
	p, err := s.Tickets(ctx, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	att := p.Items[0].Messages[0].Attachments[0]
	rc, err := s.Download(ctx, att)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello? world, this is the file" {
		t.Errorf("data = %q", b)
	}

	if _, err := s.Download(ctx, source.Attachment{URL: att.URL[:strings.Index(att.URL, "/v2/")] + "/v2/conversations/10/attachments/999/data"}); err == nil {
		t.Error("a missing attachment downloaded")
	}
}
