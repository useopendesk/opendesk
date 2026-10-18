package integrations

// Icons of custom servers (contracts/integrations.md): the icon the server advertises
// when its session opens, else its site's icon, downloaded and re-encoded by
// internal/images in a job after each successful sync. Catalog products are drawn by
// the app. Icon requests never carry the connection's credentials.

import (
	"context"
	"errors"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"golang.org/x/net/publicsuffix"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/images"
	"github.com/useopendesk/opendesk/internal/jobs"
	"github.com/useopendesk/opendesk/internal/netsafe"
)

const (
	// maxIconTries bounds the advertised icons downloaded per lookup.
	maxIconTries = 3
	// iconSide is the side an icon is wanted at; the closest larger one is preferred.
	iconSide = 64
)

// enqueueIcon queues an icon lookup for a connected custom server in tx, once while one
// is waiting or running.
func (s *Service) enqueueIcon(ctx context.Context, tx pgx.Tx, x *row) error {
	if s.d.Jobs == nil || x.Provider != CustomKey || x.Status != "connected" {
		return nil
	}
	_, err := s.d.Jobs.InsertTx(ctx, tx, jobs.IntegrationIconArgs{WorkspaceID: x.WorkspaceID, ID: x.ID}, &river.InsertOpts{
		MaxAttempts: 3, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable,
			rivertype.JobStatePending, rivertype.JobStateRetryable, rivertype.JobStateRunning, rivertype.JobStateScheduled}}})
	return err
}

// iconWorker runs integrations.icon.
type iconWorker struct {
	river.WorkerDefaults[jobs.IntegrationIconArgs]
	s *Service
	f *images.Fetcher
}

func (w *iconWorker) Timeout(*river.Job[jobs.IntegrationIconArgs]) time.Duration { return time.Minute }

func (w *iconWorker) Work(ctx context.Context, job *river.Job[jobs.IntegrationIconArgs]) error {
	return w.s.refreshIcon(ctx, w.f, job.Args.WorkspaceID, job.Args.ID)
}

var errSameIcon = errors.New("integrations: icon unchanged")

// refreshIcon looks up and stores a connection's icon. A miss leaves the stored one.
func (s *Service) refreshIcon(ctx context.Context, f *images.Fetcher, wid, id uuid.UUID) error {
	x, err := loadRow(ctx, s.d.DB.Pool, wid, id)
	if db.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if x.Provider != CustomKey || x.Status != "connected" {
		return nil
	}
	p, source, err := s.findIcon(ctx, f, x)
	if err != nil {
		if images.IsMiss(err) {
			return nil
		}
		return err
	}
	o := images.Owner{Type: images.OwnerIntegration, ID: x.ID, WorkspaceID: &x.WorkspaceID, Slot: images.SlotAvatar}
	err = images.Set(ctx, s.d, o, p, source, images.SetOptions{Quiet: true,
		Before: func(ctx context.Context, tx pgx.Tx, old domain.Avatar, _ *string) error {
			oid, ok := old.ImageID()
			if !ok {
				return nil
			}
			var sha string
			if err := tx.QueryRow(ctx, `SELECT sha256 FROM images WHERE id = $1`, oid).Scan(&sha); err != nil && !db.IsNotFound(err) {
				return err
			}
			if sha == p.SHA256 {
				return errSameIcon
			}
			return nil
		},
		After: func(ctx context.Context, tx pgx.Tx) error {
			cur, err := loadRow(ctx, tx, wid, id)
			if err != nil {
				return err
			}
			return s.publish(ctx, tx, "integration.updated", wid, map[string]any{"integration": cur.api()})
		}})
	if errors.Is(err, errSameIcon) || errors.Is(err, images.ErrOwnerGone) {
		return nil
	}
	return err
}

// findIcon returns the server's advertised icon ("server"), else its site's ("domain").
func (s *Service) findIcon(ctx context.Context, f *images.Fetcher, x *row) (images.Processed, string, error) {
	// A session refused here only means no advertised icons: the connection's state is
	// left to sync and tool calls.
	var icons []ServerIcon
	sctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	sess, err := s.sessionFor(sctx, x, false)
	cancel()
	if err == nil {
		icons = sess.Icons()
	} else {
		s.d.Log.Info("integration icon: session not opened", "integration_id", x.ID, "host", hostOf(x.URL), "err", err.Error())
	}
	for _, src := range iconCandidates(x.URL, icons, f.Dev) {
		p, err := f.Icon(ctx, src)
		if err == nil {
			return p, "server", nil
		}
		if ctx.Err() != nil {
			return images.Processed{}, "", ctx.Err()
		}
	}
	site := siteDomain(x.URL)
	if site == "" {
		return images.Processed{}, "", images.ErrNotFound
	}
	p, err := f.DomainLogo(ctx, site)
	if err != nil {
		return images.Processed{}, "", err
	}
	return p, "domain", nil
}

// siteDomain is the registrable domain of a server URL (mcp.example.com -> example.com),
// or "" for addresses and names without one.
func siteDomain(server string) string {
	u, err := url.Parse(server)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if _, err := netip.ParseAddr(host); err == nil {
		return ""
	}
	site, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil || !images.ValidDomain(site) {
		return ""
	}
	return site
}

// sameSite reports whether two hosts are the same or share a registrable domain.
func sameSite(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSuffix(a, ".")), strings.ToLower(strings.TrimSuffix(b, "."))
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if _, err := netip.ParseAddr(a); err == nil {
		return false
	}
	if _, err := netip.ParseAddr(b); err == nil {
		return false
	}
	sa, err1 := publicsuffix.EffectiveTLDPlusOne(a)
	sb, err2 := publicsuffix.EffectiveTLDPlusOne(b)
	return err1 == nil && err2 == nil && sa == sb
}

var rasterData = regexp.MustCompile(`(?i)^data:image/(png|jpeg|jpg|gif|webp);base64,`)

// iconCandidates picks at most maxIconTries of a server's icons to download, best
// first: raster images only (never SVG), data: URIs or URLs on the server's own site
// (https; http to loopback on development instances), icons for any or the light theme
// before dark ones, then the smallest at least iconSide, then the largest.
func iconCandidates(server string, icons []ServerIcon, dev bool) []string {
	su, err := url.Parse(server)
	if err != nil {
		return nil
	}
	type cand struct {
		src  string
		dark bool
		side int
	}
	var out []cand
	for _, ic := range icons {
		src := strings.TrimSpace(ic.Src)
		mt := strings.ToLower(strings.TrimSpace(ic.MimeType))
		if src == "" || strings.Contains(mt, "svg") || (mt != "" && !strings.HasPrefix(mt, "image/")) {
			continue
		}
		if len(src) >= 5 && strings.EqualFold(src[:5], "data:") {
			if !rasterData.MatchString(src) {
				continue
			}
		} else {
			u, err := url.Parse(src)
			if err != nil || u.User != nil || u.Host == "" || strings.HasSuffix(strings.ToLower(u.Path), ".svg") {
				continue
			}
			switch {
			case u.Scheme == "https":
			case u.Scheme == "http" && dev && netsafe.IsLoopbackHost(u.Hostname()):
			default:
				continue
			}
			if !sameSite(su.Hostname(), u.Hostname()) {
				continue
			}
			src = u.String()
		}
		out = append(out, cand{src: src, dark: ic.Theme != "" && !strings.EqualFold(ic.Theme, "light"), side: largestSide(ic.Sizes)})
	}
	// Within a theme: sides >= iconSide (smallest first), then unknown, then smaller
	// ones (largest first).
	rank := func(c cand) int {
		switch {
		case c.side >= iconSide:
			return 0
		case c.side == 0:
			return 1
		}
		return 2
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.dark != b.dark {
			return !a.dark
		}
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra < rb
		}
		if a.side >= iconSide {
			return a.side < b.side
		}
		return a.side > b.side
	})
	var srcs []string
	seen := map[string]bool{}
	for _, c := range out {
		if len(srcs) >= maxIconTries {
			break
		}
		if !seen[c.src] {
			seen[c.src] = true
			srcs = append(srcs, c.src)
		}
	}
	return srcs
}

// largestSide reads sizes such as "48x48" and returns the largest side, 0 when none
// is given ("any" included).
func largestSide(sizes []string) int {
	best := 0
	for _, sz := range sizes {
		for _, part := range strings.Fields(strings.ToLower(sz)) {
			w, _, ok := strings.Cut(part, "x")
			if !ok {
				continue
			}
			if n, err := strconv.Atoi(w); err == nil && n > best && n <= 4096 {
				best = n
			}
		}
	}
	return best
}
