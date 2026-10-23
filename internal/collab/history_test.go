package collab

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"

	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/plan"
)

// History imports, Collab storage and search (contracts/collab.md "History", "Storage",
// "Search") against the provider fakes.

// history loads the account's import.
func (w *world) history(a *accountRow) *historyRow {
	w.t.Helper()
	h, err := loadHistory(w.ctx, w.d.DB.Pool, a.ID)
	if err != nil || h == nil {
		w.t.Fatalf("history: %v %v", h, err)
	}
	return h
}

func (w *world) messages(a *accountRow) int {
	return w.count(`SELECT count(*) FROM collab_messages WHERE account_id = $1`, a.ID)
}

// oldGmail adds n messages from days ago and older, one a day, oldest first, in pairs of
// a conversation; the newest of them gets an inline image.
func oldGmail(g *fakeGmail, prefix string, n, days int, labels []string, to string) {
	now := time.Now()
	for i := n - 1; i >= 0; i-- {
		id := fmt.Sprintf("%s-%02d", prefix, i)
		g.add(id, fmt.Sprintf("t-%s-%d", prefix, i/2), labels, testMail{from: "Frank <frank@example.com>", to: to,
			subject: "Old " + id, msgID: id + "@example.com", body: "Archive note " + id, image: i == 0,
			date: now.Add(-time.Duration(days+i) * 24 * time.Hour)})
	}
}

func TestGmailHistoryImport(t *testing.T) {
	w := newWorld(t)
	g := newFakeGmail(t)
	w.useGmail(g)
	a := w.account(w.agent, ProviderGoogle, VisibilityShared, Audience{Everyone: true})
	oldGmail(g, "old", 60, 40, []string{"INBOX"}, a.Email)
	oldGmail(g, "junk", 1, 45, []string{"SPAM"}, a.Email)
	for i := range 3 {
		g.add(fmt.Sprintf("new-%d", i), fmt.Sprintf("t-new-%d", i), []string{"INBOX"}, testMail{from: "Frank <frank@example.com>",
			to: a.Email, subject: "New", msgID: fmt.Sprintf("new-%d@example.com", i), body: "Fresh", date: time.Now().Add(-time.Duration(3-i) * time.Hour)})
	}
	w.sync(a)
	if n := w.messages(a); n != 3 {
		t.Fatalf("first sync: %d messages, want the 3 recent", n)
	}

	owner, other, viewer := w.api(w.agent), w.api(w.agent2), w.api(w.viewer)
	path := "/v1/collab/accounts/" + a.ID.String() + "/history"
	var acc MailAccount
	if code := doJSON(t, "GET", owner.URL+"/v1/collab/accounts", nil, nil); code != 200 {
		t.Fatal(code)
	}
	if code := doJSON(t, "PUT", other.URL+path, map[string]any{}, nil); code != 403 {
		t.Fatalf("someone else's account: %d", code)
	}
	if code := doJSON(t, "PUT", viewer.URL+path, map[string]any{}, nil); code != 403 {
		t.Fatalf("viewer: %d", code)
	}
	if code := doJSON(t, "PUT", owner.URL+path, map[string]any{"since": time.Now().Add(time.Hour)}, nil); code != 422 {
		t.Fatalf("future since: %d", code)
	}
	if code := doJSON(t, "PATCH", owner.URL+path, map[string]any{"paused": true}, nil); code != 404 {
		t.Fatalf("pause without an import: %d", code)
	}
	if code := doJSON(t, "PUT", owner.URL+path, map[string]any{}, &acc); code != 200 || acc.History == nil || acc.History.Status != HistoryRunning ||
		acc.History.Since != nil || acc.OldestMessageAt == nil {
		t.Fatalf("start: %d %+v", code, acc.History)
	}
	if n := w.count(`SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'collab.history_import'`, w.wid); n != 1 {
		t.Fatalf("audit: %d", n)
	}
	if n := w.count(`SELECT count(*) FROM river_job WHERE kind = 'collab.sync_account' AND args->>'account_id' = $1`, a.ID.String()); n == 0 {
		t.Fatal("starting did not queue a sync")
	}

	// The provider fails on the second page: the first is kept, the error noted, the live
	// sync unharmed; the next pass goes on from the second page.
	g.failRawAfter = g.gets + historyPage + 3
	w.sync(a)
	h := w.history(a)
	if h.Status != HistoryRunning || h.LastError == "" || h.Imported+h.Present != historyPage || h.Total == nil {
		t.Fatalf("after a failure: %+v", h)
	}
	g.failRawAfter = 0
	w.sync(a)
	h = w.history(a)
	if h.Status != HistoryDone || h.FinishedAt == nil || h.LastError != "" || h.Imported != 60 || h.Present != 3 || h.TooLarge != 0 {
		t.Fatalf("done: %+v", h)
	}
	if n := w.messages(a); n != 63 {
		t.Fatalf("%d messages, want 63: each once, spam left out", n)
	}
	if h.Reached == nil || time.Since(*h.Reached) < 98*24*time.Hour {
		t.Fatalf("reached %v", h.Reached)
	}
	// Self-hosted: unlimited, and history keeps its raw copy like synced mail.
	if m := w.messageByRemote(a, "old-00"); m.RawKey == "" {
		t.Fatal("self-hosted history kept no raw copy")
	}
	if n := w.count(`SELECT count(*) FROM collab_accounts WHERE id = $1 AND stored_bytes > 0`, a.ID); n != 1 {
		t.Fatal("stored bytes not summed")
	}
	var page httpx.Page[MailThread]
	if code := doJSON(t, "GET", owner.URL+"/v1/collab/threads?box=all&q="+url.QueryEscape("old-59"), nil, &page); code != 200 || len(page.Data) != 1 {
		t.Fatalf("search old mail: %d %+v", code, page.Data)
	}
	var status CollabStatus
	if code := doJSON(t, "GET", owner.URL+"/v1/collab", nil, &status); code != 200 || status.Storage.LimitBytes != 0 || status.Storage.UsedBytes == 0 || status.Storage.Full {
		t.Fatalf("status: %d %+v", code, status.Storage)
	}
	if code := doJSON(t, "PATCH", owner.URL+path, map[string]any{"paused": true}, nil); code != 422 {
		t.Fatalf("pause an ended import: %d", code)
	}

	// Again with spam: only what is missing is read.
	gets := g.gets
	since := time.Now().Add(-50 * 24 * time.Hour)
	if code := doJSON(t, "PUT", owner.URL+path, map[string]any{"since": since, "include_spam_trash": true}, &acc); code != 200 ||
		acc.History.Since == nil || !acc.History.IncludeSpamTrash || acc.History.Imported != 0 {
		t.Fatalf("again: %d %+v", code, acc.History)
	}
	w.sync(a)
	h = w.history(a)
	if h.Status != HistoryDone || h.Imported != 1 || g.gets-gets != 1 {
		t.Fatalf("again: %+v, %d reads", h, g.gets-gets)
	}

	// Pause, resume, stop.
	if code := doJSON(t, "PUT", owner.URL+path, map[string]any{}, nil); code != 200 {
		t.Fatal(code)
	}
	if code := doJSON(t, "PATCH", owner.URL+path, map[string]any{"paused": true}, &acc); code != 200 || acc.History.Status != HistoryPaused {
		t.Fatalf("pause: %d %+v", code, acc.History)
	}
	gets = g.gets
	w.sync(a)
	if g.gets != gets || w.history(a).Status != HistoryPaused {
		t.Fatal("a paused import read mail")
	}
	if code := doJSON(t, "PATCH", owner.URL+path, map[string]any{"paused": false}, &acc); code != 200 || acc.History.Status != HistoryRunning {
		t.Fatalf("resume: %d %+v", code, acc.History)
	}
	if code := doJSON(t, "DELETE", owner.URL+path, nil, &acc); code != 200 || acc.History.Status != HistoryCanceled || acc.History.FinishedAt == nil {
		t.Fatalf("stop: %d %+v", code, acc.History)
	}
	if n := w.messages(a); n != 64 {
		t.Fatalf("stopping removed mail: %d", n)
	}
}

func TestGmailHistoryRefusedPageListsFromTheDateReached(t *testing.T) {
	w := newWorld(t)
	g := newFakeGmail(t)
	w.useGmail(g)
	a := w.account(w.agent, ProviderGoogle, VisibilityPrivate, Audience{})
	oldGmail(g, "old", 40, 40, []string{"INBOX"}, a.Email)
	w.sync(a)
	mustExec(t, w.d, `INSERT INTO collab_history (account_id, workspace_id, status) VALUES ($1, $2, 'running')`, a.ID, w.wid)
	g.failRawAfter = g.gets + historyPage
	w.sync(a)
	// Gmail no longer knows the saved page.
	b, _ := json.Marshal(gmailHistoryCursor{PageToken: "refused", Started: true})
	mustExec(t, w.d, `UPDATE collab_history SET cursor = $2 WHERE account_id = $1`, a.ID, b)
	g.failRawAfter = 0
	w.sync(a)
	if h := w.history(a); h.Status != HistoryDone || h.Imported != 40 || w.messages(a) != 40 {
		t.Fatalf("after a refused page: %+v, %d messages", h, w.messages(a))
	}
}

// billed makes the world's workspace a collab plan subscriber with this Collab storage
// allowance, on a service with billing on.
func (w *world) billed(allowance int64) {
	w.t.Helper()
	dd := *w.d
	dd.Plan = plan.New(true, 1)
	gmail := w.s.gmailBase
	w.s = newService(&dd)
	w.s.gmailBase = gmail
	if err := plan.Ensure(w.ctx, w.d.DB.Pool, w.wid); err != nil {
		w.t.Fatal(err)
	}
	w.allow(allowance)
}

func (w *world) allow(allowance int64) {
	mustExec(w.t, w.d, `UPDATE subscriptions SET plan = 'collab', status = 'active', seats = 1, collab_storage_override = $2
		WHERE workspace_id = $1`, w.wid, allowance)
}

func TestHistoryStorageAllowance(t *testing.T) {
	w := newWorld(t)
	g := newFakeGmail(t)
	w.useGmail(g)
	w.billed(0)
	a := w.account(w.agent, ProviderGoogle, VisibilityPrivate, Audience{})
	oldGmail(g, "old", 60, 40, []string{"INBOX"}, a.Email)
	g.add("new-1", "t-new", []string{"INBOX"}, testMail{from: "frank@example.com", to: a.Email, subject: "New", msgID: "new-1@example.com", body: "Fresh"})
	w.sync(a)
	var used int64
	if err := w.d.DB.Pool.QueryRow(w.ctx, `SELECT stored_bytes FROM collab_accounts WHERE id = $1`, a.ID).Scan(&used); err != nil || used == 0 {
		t.Fatalf("stored bytes %d %v", used, err)
	}

	// Room for about one page: the import stops at the allowance.
	w.allow(used + 2000)
	mustExec(t, w.d, `INSERT INTO collab_history (account_id, workspace_id, status) VALUES ($1, $2, 'running')`, a.ID, w.wid)
	w.sync(a)
	h := w.history(a)
	if h.Status != HistoryLimited || h.Imported == 0 || h.Imported >= 60 {
		t.Fatalf("at the allowance: %+v", h)
	}
	// On a billed instance history keeps no raw copy; its files come from the provider.
	m := w.messageByRemote(a, "old-00")
	if m == nil || m.RawKey != "" || len(m.Attachments) != 1 {
		t.Fatalf("history files: %+v", m)
	}
	raw, err := w.s.loadRaw(w.ctx, a, m, "t-old-0")
	if err != nil || !strings.Contains(string(raw), "fake-image-bytes") && !strings.Contains(string(raw), "image/png") {
		t.Fatalf("files from the provider: %v", err)
	}
	var status CollabStatus
	if code := doJSON(t, "GET", w.api(w.agent).URL+"/v1/collab", nil, &status); code != 200 || !status.Storage.HistoryLimited || status.Storage.Full {
		t.Fatalf("status: %d %+v", code, status.Storage)
	}
	// A limited import waits; with room it goes on by itself.
	w.sync(a)
	if w.history(a).Status != HistoryLimited {
		t.Fatal("went on without room")
	}
	w.allow(0)
	w.sync(a)
	if h := w.history(a); h.Status != HistoryDone || w.messages(a) != 61 {
		t.Fatalf("with room: %+v, %d messages", h, w.messages(a))
	}

	// Past twice the allowance new mail waits at the provider, then comes in.
	w.allow(1)
	w.sync(a) // sums the storage
	g.add("new-2", "t-new-2", []string{"INBOX"}, testMail{from: "frank@example.com", to: a.Email, subject: "Newer", msgID: "new-2@example.com", body: "Fresher"})
	g.record("messagesAdded", "new-2")
	w.sync(a)
	newer := `SELECT count(*) FROM collab_messages WHERE account_id = $1 AND remote_id = 'new-2'`
	if w.count(newer, a.ID) != 0 {
		t.Fatal("new mail imported past twice the allowance")
	}
	if st := gmailStateOf(t, w.reload(a)); len(st.Backlog) != 1 {
		t.Fatalf("backlog %v", st.Backlog)
	}
	if code := doJSON(t, "GET", w.api(w.agent).URL+"/v1/collab", nil, &status); code != 200 || !status.Storage.Full {
		t.Fatalf("full: %d %+v", code, status.Storage)
	}
	w.allow(0)
	w.sync(a)
	if w.count(newer, a.ID) != 1 {
		t.Fatal("waiting mail did not come in with room")
	}
}

func TestSearchFindsLateText(t *testing.T) {
	w := newWorld(t)
	g := newFakeGmail(t)
	w.useGmail(g)
	a := w.account(w.agent, ProviderGoogle, VisibilityPrivate, Audience{})
	long := strings.Repeat("filler words go here ", 4000) + " zanzibarquay"
	g.add("long", "t-long", []string{"INBOX"}, testMail{from: "frank@example.com", to: a.Email, subject: "Long", msgID: "long@example.com", body: long})
	for i := range 40 {
		body := strings.Repeat("a long conversation ", 200)
		if i == 39 {
			body += " quokkaledger"
		}
		g.add(fmt.Sprintf("conv-%d", i), "t-conv", []string{"INBOX"}, testMail{from: fmt.Sprintf("p%d@example.com", i), to: a.Email,
			subject: "Thread", msgID: fmt.Sprintf("conv-%d@example.com", i), body: body})
	}
	w.sync(a)
	api := w.api(w.agent)
	for _, q := range []string{"zanzibarquay", "quokkaledger", "p35@example.com"} {
		var page httpx.Page[MailThread]
		if code := doJSON(t, "GET", api.URL+"/v1/collab/threads?box=all&q="+url.QueryEscape(q), nil, &page); code != 200 || len(page.Data) != 1 {
			t.Fatalf("search %q: %d %d", q, code, len(page.Data))
		}
	}
}

func TestIMAPHistoryImport(t *testing.T) {
	w := newWorld(t)
	imapAddr := startIMAP(t)
	ih, ip := hostPort(t, imapAddr)
	a := w.account(w.agent, ProviderIMAP, VisibilityPrivate, Audience{})
	cfg := accountConfig{IMAPHost: ih, IMAPPort: ip, IMAPTLS: "none", IMAPUsername: "user@acme.test"}
	mustExec(t, w.d, `UPDATE collab_accounts SET config = $2 WHERE id = $1`, a.ID, cfg)
	a = w.reload(a)
	srv := dialServer(t, imapAddr)
	srv.create("Projects")
	srv.create("Trash")
	now := time.Now()
	for i := range 30 {
		srv.append("INBOX", testMail{from: "old@example.com", to: a.Email, subject: "Old", msgID: fmt.Sprintf("old-%d@example.com", i),
			body: "Long ago"}.raw(), now.Add(-time.Duration(60+i)*24*time.Hour))
	}
	both := testMail{from: "pm@example.com", to: a.Email, subject: "Plan", msgID: "plan@example.com", body: "The plan"}.raw()
	srv.append("INBOX", both, now.Add(-90*24*time.Hour))
	srv.append("Projects", both, now.Add(-90*24*time.Hour))
	srv.append("Trash", testMail{from: "x@example.com", to: a.Email, subject: "Gone", msgID: "gone@example.com", body: "x"}.raw(), now.Add(-80*24*time.Hour))
	srv.append("INBOX", testMail{from: "new@example.com", to: a.Email, subject: "New", msgID: "new@example.com", body: "Now"}.raw(), now.Add(-time.Hour))
	w.sync(a)
	if n := w.messages(a); n != 1 {
		t.Fatalf("first sync: %d", n)
	}
	mustExec(t, w.d, `INSERT INTO collab_history (account_id, workspace_id, status) VALUES ($1, $2, 'running')`, a.ID, w.wid)
	w.sync(a)
	h := w.history(a)
	if h.Status != HistoryDone || h.Imported != 31 || h.Present != 1 || h.Total == nil || *h.Total != 32 {
		t.Fatalf("done: %+v total %v", h, h.Total)
	}
	if n := w.messages(a); n != 32 {
		t.Fatalf("%d messages, want 32: trash left out, the copy once", n)
	}
	if m := w.messageByRemote(a, "plan@example.com"); len(parseLocation(m.Location)) != 2 {
		t.Fatalf("both copies: %s", m.Location)
	}
	// Older mail stays current: read elsewhere.
	uids, _ := srv.find("INBOX", "old-3@example.com")
	if err := srv.c.Store(imap.UIDSetNum(uids...), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Flags: []imap.Flag{imap.FlagSeen}}, nil).Close(); err != nil {
		t.Fatal(err)
	}
	w.sync(a)
	if m := w.messageByRemote(a, "old-3@example.com"); m.Unread {
		t.Fatal("older mail did not follow the server")
	}
}

func TestGraphHistoryImport(t *testing.T) {
	w := newWorld(t)
	g := newFakeGraph(t)
	g.pageSize = 7
	w.s.graphBase = g.srv.URL
	a := w.account(w.agent, ProviderMicrosoft, VisibilityPrivate, Audience{})
	g.burst("old", "f-inbox", 20, time.Now().Add(-200*24*time.Hour), a.Email)
	g.burst("arch", "f-archive", 5, time.Now().Add(-300*24*time.Hour), a.Email)
	g.burst("junk", "f-junk", 2, time.Now().Add(-100*24*time.Hour), a.Email)
	g.burst("new", "f-inbox", 2, time.Now().Add(-time.Hour), a.Email)
	w.sync(a)
	if n := w.messages(a); n != 2 {
		t.Fatalf("first sync: %d", n)
	}
	g.unfiltered = true
	mustExec(t, w.d, `INSERT INTO collab_history (account_id, workspace_id, status) VALUES ($1, $2, 'running')`, a.ID, w.wid)
	w.sync(a)
	h := w.history(a)
	if h.Status != HistoryDone || h.Imported != 25 || h.Present != 2 || h.Total == nil || *h.Total != 27 {
		t.Fatalf("done: %+v", h)
	}
	if n := w.messages(a); n != 27 {
		t.Fatalf("%d messages, junk left out", n)
	}
	// The folder's own round took the history's link: older mail moved elsewhere follows.
	var st folderDeltaState
	_ = json.Unmarshal(w.folderByRemote(a, "f-inbox").SyncState, &st)
	if st.Delta == "" || strings.Contains(st.Delta, "ge=1") {
		t.Fatalf("inbox place: %+v", st)
	}
	g.move("old-3", "f-archive")
	w.sync(a)
	if m := w.messageByRemote(a, "old-3"); m == nil || len(m.FolderIDs) != 1 || m.FolderIDs[0] != w.folderByRemote(a, "f-archive").ID {
		t.Fatalf("moved older mail: %+v", m)
	}
}
