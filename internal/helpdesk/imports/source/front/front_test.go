package front

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/useopendesk/opendesk/internal/helpdesk/imports/source"
)

func testSource(t *testing.T, handler http.HandlerFunc) *front {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	s, err := New(source.Credentials{"token": "good-token"}, source.Options{Dev: true, BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return s.(*front)
}

func jsonResponse(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

func TestNewRejectsInvalidToken(t *testing.T) {
	for _, token := range []string{"", "two words"} {
		if _, err := New(source.Credentials{"token": token}, source.Options{}); err == nil {
			t.Errorf("New accepted token %q", token)
		}
	}
}

func TestCheckAndTickets(t *testing.T) {
	s := testSource(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/me":
			jsonResponse(w, `{"id":"company-1","name":"Acme Support"}`)
		case "/inboxes":
			jsonResponse(w, `{"_results":[{"id":"shared","name":"Support"},{"id":"private","name":"Personal","is_private":true}]}`)
		case "/teammates":
			jsonResponse(w, `{"_results":[{"id":"agent-1","first_name":"Jo","last_name":"Agent","email":"jo@example.test"}]}`)
		case "/inboxes/shared/conversations":
			if r.URL.Query().Get("page_token") == "page-2" {
				jsonResponse(w, `{"_results":[{"id":"c2","subject":"Second","status":"open","created_at":1700001000,"updated_at":1700001100}]}`)
				return
			}
			jsonResponse(w, `{"_results":[{"id":"c1","subject":"Refund","status":"archived","status_category":"resolved","created_at":1700000000,"updated_at":1700000500,"tags":[{"name":"billing"}]}],"_pagination":{"next":"https://api.front.test/inboxes/shared/conversations?page_token=page-2"}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	scan, err := s.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.Account != (source.Account{Key: "company-1", Name: "Acme Support", URL: appURL}) {
		t.Errorf("account = %+v", scan.Account)
	}
	if len(scan.Groups) != 1 || scan.Groups[0] != (source.Group{ID: "shared", Name: "Support"}) {
		t.Errorf("groups = %+v", scan.Groups)
	}
	if len(scan.Agents) != 1 || scan.Agents[0] != (source.Agent{ID: "agent-1", Name: "Jo Agent", Email: "jo@example.test"}) {
		t.Errorf("agents = %+v", scan.Agents)
	}

	first, err := s.Tickets(context.Background(), "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Next == "" {
		t.Fatalf("first page = %+v", first)
	}
	if ticket := first.Items[0]; ticket.ID != "c1" || ticket.Status != "solved" || ticket.GroupID != "shared" || len(ticket.Tags) != 1 {
		t.Errorf("first ticket = %+v", ticket)
	}

	second, err := s.Tickets(context.Background(), first.Next, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != "c2" || second.Next != "" {
		t.Errorf("second page = %+v", second)
	}
}

func TestLoad(t *testing.T) {
	s := testSource(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/conversations/c1":
			jsonResponse(w, `{"id":"c1"}`)
		case "/conversations/c1/messages":
			jsonResponse(w, `{"_results":[
			 {"id":"m1","type":"email","is_inbound":true,"text":"Hello","created_at":1700000000,
			  "recipients":[{"role":"from","handle":"jo@example.test","name":"Jo"}],
			  "attachments":[{"filename":"photo.png","url":"https://files.example.test/photo.png","content_type":"image/png","size":42}]},
			 {"id":"draft","is_draft":true,"created_at":1700000100,"body":"Draft"},
			 {"id":"m2","type":"email","body":"<p>We can help.</p>","created_at":1700000200,
			  "author":{"id":"agent-1","first_name":"Jo","last_name":"Agent"}}]}`)
		case "/conversations/c1/comments":
			jsonResponse(w, `{"_results":[{"id":"note-1","body":"Check the order","created_at":1700000150,"author":{"id":"agent-1","username":"jo"}}]}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	ticket := &source.Ticket{ID: "c1"}
	if err := s.Load(context.Background(), ticket); err != nil {
		t.Fatal(err)
	}
	if !ticket.Loaded || ticket.Channel != "email" || ticket.Requester == nil || ticket.Requester.Email != "jo@example.test" {
		t.Fatalf("ticket = %+v", ticket)
	}
	if len(ticket.Messages) != 3 {
		t.Fatalf("got %d messages, want 3", len(ticket.Messages))
	}
	if got := ticket.Messages[0]; got.Agent || got.Note || got.Author.Name != "Jo" || got.Text != "Hello" ||
		!got.CreatedAt.Equal(time.Unix(1700000000, 0)) || len(got.Attachments) != 1 || got.Attachments[0].ID != "m1-1" {
		t.Errorf("inbound message = %+v", got)
	}
	if got := ticket.Messages[1]; !got.Agent || !got.Note || got.Text != "Check the order" {
		t.Errorf("comment = %+v", got)
	}
	if got := ticket.Messages[2]; !got.Agent || got.Note || got.HTML != "<p>We can help.</p>" {
		t.Errorf("reply = %+v", got)
	}
}

func TestPageTokenRejectsMalformedLinks(t *testing.T) {
	for _, link := range []string{"https://[", "https://front.test/conversations", "https://front.test/conversations?page_token=current"} {
		if _, err := pageToken(link, "current"); err == nil {
			t.Errorf("pageToken(%q) returned no error", link)
		}
	}
	if token, err := pageToken("", "current"); err != nil || token != "" {
		t.Errorf("pageToken(empty) = %q, %v", token, err)
	}
}
