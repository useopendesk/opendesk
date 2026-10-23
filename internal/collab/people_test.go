package collab

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/httpx"
)

// People (contracts/collab.md "People"): one address across the conversations the caller sees.

// withPerson puts the address on a seeded conversation the way a sync does: in its
// participants and addresses, and as the sender of its message, dated at.
func (w *world) withPerson(tid, mid uuid.UUID, name, addr string, at time.Time) {
	w.t.Helper()
	mustExec(w.t, w.d, `UPDATE collab_threads SET participants = $2, addresses = $3, last_message_at = $4 WHERE id = $1`,
		tid, []Address{{Name: name, Email: addr}}, []string{addr}, at)
	mustExec(w.t, w.d, `UPDATE collab_messages SET from_addr = $2, sent_at = $3 WHERE id = $1`, mid, Address{Name: name, Email: addr}, at)
}

func personOf(t *testing.T, srvURL, addr, exclude string) (int, MailPerson) {
	t.Helper()
	var out MailPerson
	path := srvURL + "/v1/collab/person?email=" + url.QueryEscape(addr)
	if exclude != "" {
		path += "&exclude=" + exclude
	}
	st := doJSON(t, "GET", path, nil, &out)
	return st, out
}

func TestPersonOnlyWhatTheCallerSees(t *testing.T) {
	w := newWorld(t)
	dana := "dana@client.test"
	now := time.Now().UTC().Truncate(time.Second)

	mine := w.account(w.agent, ProviderGoogle, VisibilityPrivate, Audience{})
	t1, m1 := w.seedThread(mine, "Renewal", w.folder(mine, RoleInbox))
	w.withPerson(t1, m1, "Dana Lee", dana, now.Add(-time.Hour))
	t2, m2 := w.seedThread(mine, "Kickoff", w.folder(mine, RoleArchive))
	w.withPerson(t2, m2, "", dana, now.Add(-72*time.Hour))
	// Trash and spam are not conversations with her.
	t3, m3 := w.seedThread(mine, "Old junk", w.folder(mine, RoleTrash))
	w.withPerson(t3, m3, "Dana Lee", dana, now.Add(-24*time.Hour))
	// Someone else's private account never counts, for anyone but its owner.
	theirs := w.account(w.agent2, ProviderGoogle, VisibilityPrivate, Audience{})
	t4, m4 := w.seedThread(theirs, "Secret pricing", w.folder(theirs, RoleInbox))
	w.withPerson(t4, m4, "Dana L.", dana, now)
	// A conversation of that account shared with the agent on its own does.
	t5, m5 := w.seedThread(theirs, "Shared intro", w.folder(theirs, RoleInbox))
	w.withPerson(t5, m5, "Dana L.", dana, now.Add(-200*time.Hour))
	if err := w.d.DB.Tx(w.ctx, func(tx pgx.Tx) error {
		return saveAudience(w.ctx, tx, w.wid, nil, &t5, Audience{UserIDs: []uuid.UUID{w.agent.UserID}}, w.agent2.UserID)
	}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, w.d, `UPDATE collab_messages SET attachments = $2 WHERE id = $1`, m1,
		`[{"index":0,"filename":"contract.pdf","content_type":"application/pdf","size":2048,"inline":false,"content_id":""},
		  {"index":1,"filename":"logo.png","content_type":"image/png","size":10,"inline":true,"content_id":"logo"}]`)
	mustExec(t, w.d, `UPDATE collab_messages SET attachments = $2 WHERE id = $1`, m4,
		`[{"index":0,"filename":"secret.xlsx","content_type":"application/vnd.ms-excel","size":10,"inline":false,"content_id":""}]`)

	srv := w.api(w.agent)
	st, p := personOf(t, srv.URL, "Dana Lee <DANA@client.test>", t1.String())
	if st != 200 {
		t.Fatalf("person: %d", st)
	}
	if p.Email != dana || p.Name != "Dana Lee" || p.Conversations != 3 || p.Others != 2 {
		t.Fatalf("person = %q %q %d %d, want dana, Dana Lee, 3, 2", p.Email, p.Name, p.Conversations, p.Others)
	}
	// A conversation in the trash left out is not among them to begin with.
	if _, x := personOf(t, srv.URL, dana, t3.String()); x.Others != 3 {
		t.Fatalf("others without a trashed one = %d, want 3", x.Others)
	}
	got := []uuid.UUID{}
	for _, x := range p.Threads {
		got = append(got, x.ID)
	}
	// Newest first, the open one left out, trash and the hidden one never there.
	if len(got) != 2 || got[0] != t2 || got[1] != t5 {
		t.Fatalf("threads = %v, want [%s %s]", got, t2, t5)
	}
	if p.LastAt == nil || !p.LastAt.Equal(now.Add(-time.Hour)) || p.FirstAt == nil || !p.FirstAt.Equal(now.Add(-200*time.Hour)) {
		t.Fatalf("first, last = %v, %v", p.FirstAt, p.LastAt)
	}
	if len(p.Files) != 1 || p.Files[0].Attachment.Filename != "contract.pdf" || p.Files[0].ThreadID != t1 ||
		!strings.Contains(p.Files[0].Attachment.URL, "/v1/collab/attachments/"+m1.String()+"/0?") {
		t.Fatalf("files = %+v, want only contract.pdf", p.Files)
	}

	// The account's owner sees their own conversations; the viewer sees nothing at all.
	_, p2 := personOf(t, w.api(w.agent2).URL, dana, "")
	if p2.Conversations != 2 || p2.Name != "Dana L." || len(p2.Files) != 1 || p2.Files[0].Attachment.Filename != "secret.xlsx" {
		t.Fatalf("owner of the other account: %d %q %+v", p2.Conversations, p2.Name, p2.Files)
	}
	st, p3 := personOf(t, w.api(w.viewer).URL, dana, "")
	if st != 200 || p3.Conversations != 0 || p3.Name != "" || len(p3.Threads) != 0 || len(p3.Files) != 0 || p3.FirstAt != nil {
		t.Fatalf("viewer: %d %+v", st, p3)
	}

	// The list's with= is the same conversations, the shared one included.
	var page httpx.Page[MailThread]
	if st := doJSON(t, "GET", srv.URL+"/v1/collab/threads?box=all&with="+url.QueryEscape("Dana@Client.test"), nil, &page); st != 200 || len(page.Data) != 3 {
		t.Fatalf("with list: %d %d, want 3", st, len(page.Data))
	}
	if st := doJSON(t, "GET", srv.URL+"/v1/collab/threads?box=all&account_id="+mine.ID.String()+"&with="+dana, nil, &page); st != 200 || len(page.Data) != 2 {
		t.Fatalf("with one account: %d %d, want 2", st, len(page.Data))
	}

	if st, _ := personOf(t, srv.URL, "not an address", ""); st != 422 {
		t.Fatalf("bad address: %d, want 422", st)
	}
}

func TestPersonNameFromAnOlderConversation(t *testing.T) {
	w := newWorld(t)
	a := w.account(w.agent, ProviderIMAP, VisibilityPrivate, Audience{})
	now := time.Now().UTC().Truncate(time.Second)
	for i := range personThreads + 2 {
		tid, mid := w.seedThread(a, "Note", w.folder(a, RoleInbox))
		name := ""
		if i == 0 {
			name = "Sam Older"
		}
		w.withPerson(tid, mid, name, "sam@client.test", now.Add(-time.Duration(personThreads+2-i)*time.Hour))
	}
	_, p := personOf(t, w.api(w.agent).URL, "sam@client.test", "")
	if p.Name != "Sam Older" || p.Conversations != personThreads+2 || len(p.Threads) != personThreads {
		t.Fatalf("person = %q %d %d", p.Name, p.Conversations, len(p.Threads))
	}
}
