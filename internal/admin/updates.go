package admin

// Releases and automatic updates (contracts/admin.md "Updates"). A release build looks for
// the newest opendesk release on GitHub every few hours, shows it on the health page and
// emails the super admins once per release. Installing it is the host's job: opendesk-ctl
// runs automatic updates from a timer and reports each run with `opendesk updates report`.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/emails"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/mailer"
	"github.com/useopendesk/opendesk/internal/version"
)

const (
	releaseRepo      = "useopendesk/opendesk"
	latestReleaseAPI = "https://api.github.com/repos/" + releaseRepo + "/releases/latest"
	// releaseCheckEvery is how often the worker asks GitHub; the job itself runs hourly so
	// a failed automatic update is reported soon after it happened.
	releaseCheckEvery = 6 * time.Hour
)

// Results of an automatic update run, as opendesk-ctl reports them.
const (
	AutoNone    = "none"
	AutoCurrent = "current" // already on the newest release
	AutoUpdated = "updated"
	AutoSkipped = "skipped" // a new major version, or a pin that is not a release
	AutoFailed  = "failed"  // the new release did not come up healthy; the old one runs again
)

var releaseTag = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?$`)

type semver struct {
	major, minor, patch int
	pre                 string
}

func parseVersion(v string) (semver, bool) {
	m := releaseTag.FindStringSubmatch(v)
	if m == nil {
		return semver{}, false
	}
	n := func(s string) int { i, _ := strconv.Atoi(s); return i }
	return semver{major: n(m[1]), minor: n(m[2]), patch: n(m[3]), pre: m[4]}, true
}

// isRelease reports whether v is a release version (v1.2.3 or v1.2.3-rc.1). Development
// and commit builds are not, and never look for updates.
func isRelease(v string) bool { _, ok := parseVersion(v); return ok }

// newer reports whether release a is newer than b. A pre-release comes before its release.
func newer(a, b string) bool {
	x, ok1 := parseVersion(a)
	y, ok2 := parseVersion(b)
	if !ok1 || !ok2 {
		return false
	}
	if x.major != y.major {
		return x.major > y.major
	}
	if x.minor != y.minor {
		return x.minor > y.minor
	}
	if x.patch != y.patch {
		return x.patch > y.patch
	}
	switch {
	case x.pre == y.pre:
		return false
	case x.pre == "":
		return true
	case y.pre == "":
		return false
	}
	return x.pre > y.pre
}

// majorBump reports whether release a has a higher major version than b.
func majorBump(a, b string) bool {
	x, ok1 := parseVersion(a)
	y, ok2 := parseVersion(b)
	return ok1 && ok2 && x.major > y.major
}

func releasePage(tag string) string {
	return "https://github.com/" + releaseRepo + "/releases/tag/" + tag
}

type AdminRelease struct {
	Version     string     `json:"version"`
	URL         string     `json:"url"`
	PublishedAt *time.Time `json:"published_at"`
}

type AdminAutoUpdate struct {
	Reported   bool       `json:"reported" doc:"opendesk-ctl has reported on this instance"`
	Enabled    bool       `json:"enabled"`
	LastRunAt  *time.Time `json:"last_run_at"`
	LastResult string     `json:"last_result" enum:"none,current,updated,skipped,failed"`
	LastDetail string     `json:"last_detail"`
}

type AdminUpdates struct {
	Check           bool            `json:"check" doc:"This instance looks for new releases"`
	Current         string          `json:"current"`
	Latest          *AdminRelease   `json:"latest"`
	Available       bool            `json:"available"`
	Major           bool            `json:"major" doc:"The newest release is a new major version, which automatic updates leave to the operator"`
	CheckedAt       *time.Time      `json:"checked_at"`
	Error           string          `json:"error"`
	PreviousVersion string          `json:"previous_version"`
	RunningSince    *time.Time      `json:"running_since"`
	Auto            AdminAutoUpdate `json:"auto"`
}

// updateRow is the instance_updates row; the zero value before anything was recorded.
type updateRow struct {
	running, previous, latest, checkErr, notified, autoResult, autoDetail  string
	runningSince, latestPublished, checkedAt, autoRanAt, failureNotifiedAt *time.Time
	autoEnabled                                                            *bool
}

func loadUpdateRow(ctx context.Context, q db.Querier, lock bool) (updateRow, error) {
	r := updateRow{autoResult: AutoNone}
	sql := `SELECT running_version, previous_version, latest_version, check_error, notified_version, auto_result, auto_detail,
		running_since, latest_published_at, checked_at, auto_ran_at, auto_failure_notified_at, auto_enabled
		FROM instance_updates WHERE id`
	if lock {
		sql += ` FOR UPDATE`
	}
	err := q.QueryRow(ctx, sql).Scan(&r.running, &r.previous, &r.latest, &r.checkErr, &r.notified, &r.autoResult, &r.autoDetail,
		&r.runningSince, &r.latestPublished, &r.checkedAt, &r.autoRanAt, &r.failureNotifiedAt, &r.autoEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return updateRow{autoResult: AutoNone}, nil
	}
	return r, err
}

// checksReleases reports whether this process looks for releases at all.
func (s *svc) checksReleases() bool { return s.d.Cfg.UpdateCheck && isRelease(version.Version) }

func (s *svc) updates(ctx context.Context) (AdminUpdates, error) {
	r, err := loadUpdateRow(ctx, s.d.DB.Pool, false)
	if err != nil {
		return AdminUpdates{}, err
	}
	return updatesFrom(r, version.Version, s.checksReleases()), nil
}

func updatesFrom(r updateRow, current string, check bool) AdminUpdates {
	u := AdminUpdates{Check: check, Current: current, CheckedAt: r.checkedAt, Error: r.checkErr,
		Auto: AdminAutoUpdate{Reported: r.autoEnabled != nil, Enabled: r.autoEnabled != nil && *r.autoEnabled,
			LastRunAt: r.autoRanAt, LastResult: r.autoResult, LastDetail: r.autoDetail}}
	if r.running == current {
		u.PreviousVersion, u.RunningSince = r.previous, r.runningSince
	}
	if r.latest != "" {
		u.Latest = &AdminRelease{Version: r.latest, URL: releasePage(r.latest), PublishedAt: r.latestPublished}
		u.Available = check && newer(r.latest, current)
		u.Major = u.Available && majorBump(r.latest, current)
	}
	return u
}

// autoFailed reports an automatic update that failed after this version started, so a
// manual upgrade since then clears it.
func (u AdminUpdates) autoFailed() bool {
	if !u.Auto.Enabled || u.Auto.LastResult != AutoFailed || u.Auto.LastRunAt == nil {
		return false
	}
	return u.RunningSince == nil || u.Auto.LastRunAt.After(*u.RunningSince)
}

// updateCheck is the health page's line about versions.
func updateCheck(u AdminUpdates) (status, detail string) {
	switch {
	case !u.Check && !isRelease(u.Current):
		return "ok", "This build is not a release, so it does not look for updates."
	case !u.Check:
		return "ok", "Update checks are off (UPDATE_CHECK=false)."
	case u.autoFailed():
		d := "The last automatic update failed, so the instance went back to " + u.Current + "."
		if u.Auto.LastDetail != "" {
			d += " " + u.Auto.LastDetail
		}
		return "warn", d
	case u.Available && u.Major:
		return "warn", fmt.Sprintf("%s is out, a new major version (this instance runs %s). Read the release notes, then run opendesk-ctl upgrade %s.",
			u.Latest.Version, u.Current, u.Latest.Version)
	case u.Available && u.Auto.Enabled:
		return "ok", fmt.Sprintf("%s is out (this instance runs %s). It installs at the next automatic update.", u.Latest.Version, u.Current)
	case u.Available:
		return "warn", fmt.Sprintf("%s is out (this instance runs %s). Upgrade with opendesk-ctl upgrade %s.", u.Latest.Version, u.Current, u.Latest.Version)
	case u.Latest == nil && u.Error != "":
		return "warn", "Could not look for releases: " + u.Error
	case u.Latest == nil:
		return "ok", "No release checked yet."
	case u.Auto.Enabled:
		return "ok", "This instance runs the newest release, " + u.Current + ". Automatic updates are on."
	default:
		return "ok", "This instance runs the newest release, " + u.Current + "."
	}
}

func (s *svc) registerUpdates(api huma.API) {
	op(s, api, huma.Operation{OperationID: "admin-check-updates", Method: http.MethodPost, Path: "/v1/admin/updates/check",
		Summary: "Look for a new release now"},
		func(ctx context.Context, _ *struct{}) (*struct{ Body AdminUpdates }, error) {
			a, err := require(ctx, PermRead)
			if err != nil {
				return nil, err
			}
			if err := s.limit("admin-updates:"+a.UserID.String(), 6, time.Minute); err != nil {
				return nil, err
			}
			if !s.checksReleases() {
				return nil, httpx.E(409, "update_check_off", "This instance does not look for releases: it is not a release build, or UPDATE_CHECK is false.")
			}
			if err := s.checkRelease(ctx); err != nil {
				return nil, httpx.Wrap(err)
			}
			u, err := s.updates(ctx)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body AdminUpdates }{u}, nil
		})
}

// ── the worker ───────────────────────────────────────────────────────────────

// updatesArgs records the running version, looks for a release when the last look is
// old enough, and sends the emails that are due.
type updatesArgs struct{}

func (updatesArgs) Kind() string { return "admin.updates" }

type updatesWorker struct {
	river.WorkerDefaults[updatesArgs]
	s *svc
}

func (w *updatesWorker) Timeout(*river.Job[updatesArgs]) time.Duration { return time.Minute }

func (w *updatesWorker) Work(ctx context.Context, _ *river.Job[updatesArgs]) error {
	s := w.s
	if !isRelease(version.Version) {
		return nil
	}
	if _, err := s.d.DB.Pool.Exec(ctx, `INSERT INTO instance_updates (id, running_version, running_since) VALUES (true, $1, now())
		ON CONFLICT (id) DO UPDATE SET previous_version = instance_updates.running_version, running_version = $1, running_since = now()
		WHERE instance_updates.running_version <> $1`, version.Version); err != nil {
		return err
	}
	if s.d.Cfg.UpdateCheck {
		r, err := loadUpdateRow(ctx, s.d.DB.Pool, false)
		if err != nil {
			return err
		}
		if r.checkedAt == nil || time.Since(*r.checkedAt) >= releaseCheckEvery {
			if err := s.checkRelease(ctx); err != nil {
				return err
			}
		}
	}
	return s.sendUpdateEmails(ctx)
}

// checkRelease asks GitHub for the newest release and stores the answer. A failed look is
// stored as the error, not returned: the next look tries again.
func (s *svc) checkRelease(ctx context.Context) error {
	rel, err := s.fetchLatest(ctx)
	if err != nil {
		s.d.Log.Warn("admin: release check failed", "err", err)
		_, err := s.d.DB.Pool.Exec(ctx, `INSERT INTO instance_updates (id, checked_at, check_error) VALUES (true, now(), $1)
			ON CONFLICT (id) DO UPDATE SET checked_at = now(), check_error = $1`, truncate(err.Error(), 300))
		return err
	}
	if rel.Version != "" && newer(rel.Version, version.Version) {
		s.d.Log.Info("a new opendesk release is available", "version", rel.Version, "running", version.Version, "notes", releasePage(rel.Version))
	}
	_, err = s.d.DB.Pool.Exec(ctx, `INSERT INTO instance_updates (id, latest_version, latest_published_at, checked_at, check_error)
		VALUES (true, $1, $2, now(), '')
		ON CONFLICT (id) DO UPDATE SET latest_version = $1, latest_published_at = $2, checked_at = now(), check_error = ''`,
		rel.Version, rel.PublishedAt)
	return err
}

type latestRelease struct {
	Version     string
	PublishedAt *time.Time
}

// fetchLatest reads GitHub's newest release (pre-releases and drafts are never "latest").
// No releases yet is an empty answer, not an error. The request carries nothing about
// the instance.
func (s *svc) fetchLatest(ctx context.Context) (latestRelease, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.releaseAPI, nil)
	if err != nil {
		return latestRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "opendesk-update-check")
	res, err := s.http.Do(req)
	if err != nil {
		return latestRelease{}, errors.New("GitHub did not answer")
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return latestRelease{}, nil
	}
	if res.StatusCode != http.StatusOK {
		return latestRelease{}, fmt.Errorf("GitHub answered %d", res.StatusCode)
	}
	var body struct {
		TagName     string     `json:"tag_name"`
		Draft       bool       `json:"draft"`
		Prerelease  bool       `json:"prerelease"`
		PublishedAt *time.Time `json:"published_at"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return latestRelease{}, errors.New("GitHub's answer was not a release")
	}
	if body.Draft || body.Prerelease || !isRelease(body.TagName) {
		return latestRelease{}, errors.New("the newest release on GitHub has no release version")
	}
	return latestRelease{Version: body.TagName, PublishedAt: body.PublishedAt}, nil
}

// sendUpdateEmails emails the super admins about a new release (once per release, and
// not when automatic updates will install it) and about a failed automatic update (once
// per failed run).
func (s *svc) sendUpdateEmails(ctx context.Context) error {
	site := emails.SiteFrom(s.d.Cfg)
	panel := strings.TrimRight(s.d.Cfg.AdminURL, "/") + "/health"
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		r, err := loadUpdateRow(ctx, tx, true)
		if err != nil || r.running == "" {
			return err
		}
		var out []mailer.Mail
		u := updatesFrom(r, version.Version, s.checksReleases())
		if u.Available && r.notified != r.latest {
			if !u.Auto.Enabled || u.Major {
				out = append(out, emails.UpdateAvailable(site, panel, u.Current, u.Latest.Version, u.Latest.URL, u.Major))
			}
			if _, err := tx.Exec(ctx, `UPDATE instance_updates SET notified_version = $1 WHERE id`, r.latest); err != nil {
				return err
			}
		}
		if r.autoResult == AutoFailed && r.autoRanAt != nil && (r.failureNotifiedAt == nil || r.failureNotifiedAt.Before(*r.autoRanAt)) {
			out = append(out, emails.AutoUpdateFailed(site, panel, r.autoDetail))
			if _, err := tx.Exec(ctx, `UPDATE instance_updates SET auto_failure_notified_at = now() WHERE id`); err != nil {
				return err
			}
		}
		if len(out) == 0 {
			return nil
		}
		rows, err := tx.Query(ctx, `SELECT u.email FROM instance_admins a JOIN users u ON u.id = a.user_id
			WHERE a.role = 'super' AND u.suspended_at IS NULL ORDER BY u.email`)
		if err != nil {
			return err
		}
		to, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, m := range out {
			for _, addr := range to {
				if _, err := s.d.Jobs.InsertTx(ctx, tx, jobs.SystemEmailArgs{To: addr, Subject: m.Subject, HTML: m.HTML, Text: m.Text},
					&river.InsertOpts{Queue: jobs.QueueEmail, MaxAttempts: 8}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// ── opendesk-ctl's reports ───────────────────────────────────────────────────

// ReportAutoUpdate records an automatic update run of opendesk-ctl (`opendesk updates
// report`). An empty result records only whether automatic updates are on.
func ReportAutoUpdate(ctx context.Context, q db.Querier, enabled bool, result, detail string) error {
	switch result {
	case "":
		_, err := q.Exec(ctx, `INSERT INTO instance_updates (id, auto_enabled) VALUES (true, $1)
			ON CONFLICT (id) DO UPDATE SET auto_enabled = $1`, enabled)
		return err
	case AutoCurrent, AutoUpdated, AutoSkipped, AutoFailed:
	default:
		return fmt.Errorf("result must be %s, %s, %s or %s", AutoCurrent, AutoUpdated, AutoSkipped, AutoFailed)
	}
	_, err := q.Exec(ctx, `INSERT INTO instance_updates (id, auto_enabled, auto_ran_at, auto_result, auto_detail) VALUES (true, $1, now(), $2, $3)
		ON CONFLICT (id) DO UPDATE SET auto_enabled = $1, auto_ran_at = now(), auto_result = $2, auto_detail = $3`,
		enabled, result, cleanDetail(detail))
	return err
}

// cleanDetail keeps a report's detail to one short plain line.
func cleanDetail(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 500 {
		s = string(r[:500])
	}
	return s
}
