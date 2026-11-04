package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/version"
)

// TestNewer: numeric order, a release after its pre-releases, and nothing that is not a
// release version compares at all.
func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"v0.1.1", "v0.1.0", true},
		{"v0.10.0", "v0.9.9", true},
		{"v1.0.0", "v0.99.0", true},
		{"v1.0.0", "v1.0.0-rc.2", true},
		{"v1.0.0-rc.2", "v1.0.0-rc.1", true},
		{"v1.0.0", "v1.0.0", false},
		{"v0.9.0", "v0.10.0", false},
		{"v1.0.0-rc.1", "v1.0.0", false},
		{"v1.2.3", "dev", false},
		{"v1.2.3", "3f9c1e0", false},
		{"1.2.3", "v1.0.0", false},
		{"v01.2.3", "v1.0.0", false},
	} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
	if !majorBump("v2.0.0", "v1.9.9") || majorBump("v1.10.0", "v1.9.0") || majorBump("v0.2.0", "v0.1.0") {
		t.Error("majorBump")
	}
}

// TestUpdateCheck: what the health page says in each state.
func TestUpdateCheck(t *testing.T) {
	on, off := true, false
	now := time.Now()
	before, after := now.Add(-time.Hour), now.Add(time.Hour)
	status := func(r updateRow, current string, check bool) (string, AdminUpdates) {
		u := updatesFrom(r, current, check)
		s, _ := updateCheck(u)
		return s, u
	}
	if s, _ := status(updateRow{latest: "v9.0.0"}, "3f9c1e0", false); s != "ok" {
		t.Errorf("commit build: %s", s)
	}
	if s, u := status(updateRow{latest: "v0.2.0"}, "v0.1.0", false); s != "ok" || u.Available {
		t.Errorf("checks off: %s %v", s, u.Available)
	}
	if s, u := status(updateRow{latest: "v0.2.0"}, "v0.1.0", true); s != "warn" || !u.Available || u.Major {
		t.Errorf("update out: %s %+v", s, u)
	}
	if s, _ := status(updateRow{latest: "v0.2.0", autoEnabled: &on}, "v0.1.0", true); s != "ok" {
		t.Errorf("update out, installs itself: %s", s)
	}
	if s, u := status(updateRow{latest: "v1.0.0", autoEnabled: &on}, "v0.9.0", true); s != "warn" || !u.Major {
		t.Errorf("new major with auto on: %s %+v", s, u)
	}
	// v1.5.0 out beside v2.0.0: the newest v1 release is offered, and installs itself with auto on.
	if s, u := status(updateRow{latest: "v2.0.0", inMajor: "v1.5.0", autoEnabled: &on}, "v1.2.0", true); s != "warn" || u.InMajor == nil || u.InMajor.Version != "v1.5.0" {
		t.Errorf("new major and a newer v1: %s %+v", s, u)
	} else if _, d := updateCheck(u); !strings.Contains(d, "v1.5.0, the newest v1 release, installs at the next automatic update") || !strings.Contains(d, "opendesk-ctl upgrade v2.0.0") {
		t.Errorf("detail: %s", d)
	}
	for _, in := range []string{"v1.2.0", "v1.1.0", "v2.0.0", "v0.9.0", ""} {
		if _, u := status(updateRow{latest: "v2.0.0", inMajor: in}, "v1.2.0", true); u.InMajor != nil {
			t.Errorf("in-major %q offered to v1.2.0", in)
		}
	}
	if s, _ := status(updateRow{latest: "v0.1.0", autoEnabled: &off}, "v0.1.0", true); s != "ok" {
		t.Errorf("current: %s", s)
	}
	if s, _ := status(updateRow{checkErr: "GitHub did not answer"}, "v0.1.0", true); s != "warn" {
		t.Errorf("never reached GitHub: %s", s)
	}
	failed := updateRow{latest: "v0.1.0", running: "v0.1.0", runningSince: &before, autoEnabled: &on, autoResult: AutoFailed, autoRanAt: &now}
	if s, u := status(failed, "v0.1.0", true); s != "warn" || !u.autoFailed() {
		t.Errorf("failed run: %s", s)
	}
	failed.runningSince = &after
	if _, u := status(failed, "v0.1.0", true); u.autoFailed() {
		t.Error("a later upgrade clears a failed run")
	}
	if _, u := status(updateRow{running: "v0.1.0", previous: "v0.0.9", runningSince: &now}, "v0.2.0", true); u.PreviousVersion != "" {
		t.Error("previous version of another running version")
	}
}

// TestFetchReleases: the newest release and the newest of the running major version from
// GitHub's list, no release yet, and answers that are not one.
func TestFetchReleases(t *testing.T) {
	answer, code := "", 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "opendesk-update-check" || r.Header.Get("Authorization") != "" {
			t.Errorf("headers: %v", r.Header)
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	s := &svc{releaseAPI: srv.URL, http: srv.Client()}
	ctx := context.Background()
	was := version.Version
	version.Version = "v1.2.0"
	t.Cleanup(func() { version.Version = was })

	answer = `[{"tag_name":"v2.1.0-rc.1","prerelease":true},{"tag_name":"v2.0.0","published_at":"2026-10-18T09:00:00Z","html_url":"https://evil.test"},
		{"tag_name":"v1.10.0","published_at":"2026-10-11T09:00:00Z"},{"tag_name":"v1.9.3"},{"tag_name":"v1.11.0","draft":true},{"tag_name":"latest"}]`
	if r, err := s.fetchReleases(ctx); err != nil || r.Version != "v2.0.0" || r.PublishedAt == nil || r.InMajor.Version != "v1.10.0" || r.InMajor.PublishedAt == nil {
		t.Errorf("releases: %+v %v", r, err)
	}
	if got := releasePage("v0.3.1"); got != "https://github.com/useopendesk/opendesk/releases/tag/v0.3.1" {
		t.Errorf("release page = %s", got)
	}
	version.Version = "3f9c1e0"
	if r, err := s.fetchReleases(ctx); err != nil || r.Version != "v2.0.0" || r.InMajor.Version != "" {
		t.Errorf("commit build: %+v %v", r, err)
	}
	for _, empty := range []struct {
		code   int
		answer string
	}{{404, `{"message":"Not Found"}`}, {200, `[]`}} {
		code, answer = empty.code, empty.answer
		if r, err := s.fetchReleases(ctx); err != nil || r.Version != "" {
			t.Errorf("no release yet (%d): %+v %v", code, r, err)
		}
	}
	code = 200
	for _, bad := range []string{`[{"tag_name":"latest"}]`, `[{"tag_name":"v1.0.0","prerelease":true}]`, `<html>`, `{"tag_name":"v1.0.0"}`, `[{"tag_name":"v1.0.0\" onload=x"}]`} {
		answer = bad
		if _, err := s.fetchReleases(ctx); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	code = 403
	if _, err := s.fetchReleases(ctx); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("rate limited: %v", err)
	}
}

func TestCleanDetail(t *testing.T) {
	if got := cleanDetail("v0.2.0 did not\n start:\t\x1b[31mhealthcheck\x1b[0m  "); got != "v0.2.0 did not start: [31mhealthcheck [0m" {
		t.Errorf("cleanDetail = %q", got)
	}
	if got := cleanDetail(strings.Repeat("é", 600)); len([]rune(got)) != 500 {
		t.Errorf("long detail kept %d runes", len([]rune(got)))
	}
}

// TestUpdatesFlow: the worker records the version and the newest release, emails the super
// admins once per release (not when automatic updates install it) and once per failed
// automatic run, and the health page shows it all.
func TestUpdatesFlow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	q := e.d.DB.Pool
	if _, err := q.Exec(ctx, `DELETE FROM instance_updates`); err != nil {
		t.Fatal(err)
	}
	was := version.Version
	version.Version = "v0.1.0"
	e.d.Cfg.UpdateCheck = true
	t.Cleanup(func() { version.Version = was; e.d.Cfg.UpdateCheck = false })

	latest, inMajor := "v0.2.0", "v0.1.0"
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"` + latest + `","published_at":"2026-10-18T09:00:00Z"},{"tag_name":"` + inMajor + `"}]`))
	}))
	defer gh.Close()
	s := newSvc(e.d)
	s.releaseAPI, s.http = gh.URL, gh.Client()
	w := &updatesWorker{s: s}
	work := func() {
		t.Helper()
		if err := w.Work(ctx, &river.Job[updatesArgs]{}); err != nil {
			t.Fatal(err)
		}
	}

	super := e.person("super")
	e.twoFactor(super)
	e.grant(super, RoleSuper)
	viewer := e.person("viewer")
	e.twoFactor(viewer)
	e.grant(viewer, RoleViewer)
	mails := func(to, subject string) int {
		t.Helper()
		var n int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind = 'mail.system' AND args->>'to' = $1 AND args->>'subject' LIKE $2`,
			to, subject+"%").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	work()
	if n := mails(super.email, "opendesk v0.2.0 is available"); n != 1 {
		t.Fatalf("super admin got %d release emails", n)
	}
	if n := mails(viewer.email, "opendesk"); n != 0 {
		t.Errorf("viewer got %d emails", n)
	}
	work()
	if n := mails(super.email, "opendesk v0.2.0 is available"); n != 1 {
		t.Errorf("the same release emailed again: %d", n)
	}

	// With automatic updates on, a minor release is left to them; a failed run is emailed once.
	if err := ReportAutoUpdate(ctx, q, true, "", ""); err != nil {
		t.Fatal(err)
	}
	latest = "v0.3.0"
	if _, err := q.Exec(ctx, `UPDATE instance_updates SET checked_at = now() - interval '7 hours'`); err != nil {
		t.Fatal(err)
	}
	work()
	if n := mails(super.email, "opendesk v0.3.0"); n != 0 {
		t.Errorf("auto update on, still emailed: %d", n)
	}
	if err := ReportAutoUpdate(ctx, q, true, "bogus", ""); err == nil {
		t.Error("an unknown result was accepted")
	}
	if err := ReportAutoUpdate(ctx, q, true, AutoFailed, "v0.3.0 did not answer its health check in 5 minutes."); err != nil {
		t.Fatal(err)
	}
	work()
	work()
	if n := mails(super.email, "An automatic opendesk update failed"); n != 1 {
		t.Errorf("failed run emails: %d", n)
	}

	panel := e.signIn(super)
	r := panel.do("GET", "/v1/admin/health", nil)
	want(t, "health", r, 200)
	u, _ := r.Body["updates"].(map[string]any)
	auto, _ := u["auto"].(map[string]any)
	if u["current"] != "v0.1.0" || u["available"] != true || auto["enabled"] != true || auto["last_result"] != "failed" {
		t.Errorf("updates = %v", u)
	}
	found := false
	for _, c := range r.Body["checks"].([]any) {
		if m := c.(map[string]any); m["name"] == "Updates" {
			found = m["status"] == "warn" && strings.Contains(m["detail"].(string), "did not answer")
		}
	}
	if !found {
		t.Errorf("checks = %v", r.Body["checks"])
	}

	// A new major version out beside a newer v0 release: with automatic updates on, one email
	// about the major version, naming the v0 release they install; nothing for later v0 releases.
	latest, inMajor = "v1.0.0", "v0.4.0"
	stale := func() {
		t.Helper()
		if _, err := q.Exec(ctx, `UPDATE instance_updates SET checked_at = now() - interval '7 hours'`); err != nil {
			t.Fatal(err)
		}
	}
	stale()
	work()
	if n := mails(super.email, "opendesk v0.4.0 and v1.0.0 are available"); n != 1 {
		t.Errorf("major with a newer v0: %d emails", n)
	}
	r = panel.do("GET", "/v1/admin/health", nil)
	if u, _ := r.Body["updates"].(map[string]any); u["major"] != true || u["in_major"].(map[string]any)["version"] != "v0.4.0" {
		t.Errorf("updates = %v", u)
	}
	inMajor = "v0.4.1"
	stale()
	work()
	if n := mails(super.email, "opendesk v0.4.1"); n != 0 {
		t.Errorf("auto update on, a v0 patch emailed: %d", n)
	}
	// Without automatic updates, the next v0 release is news.
	if err := ReportAutoUpdate(ctx, q, false, "", ""); err != nil {
		t.Fatal(err)
	}
	inMajor = "v0.4.2"
	stale()
	work()
	if n := mails(super.email, "opendesk v0.4.2 and v1.0.0 are available"); n != 1 {
		t.Errorf("auto update off, a v0 release: %d emails", n)
	}

	// A new running version is recorded with the one before it.
	version.Version = "v0.3.0"
	work()
	var running, previous string
	if err := q.QueryRow(ctx, `SELECT running_version, previous_version FROM instance_updates`).Scan(&running, &previous); err != nil {
		t.Fatal(err)
	}
	if running != "v0.3.0" || previous != "v0.1.0" {
		t.Errorf("running %s, previous %s", running, previous)
	}
}
