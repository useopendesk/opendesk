package helpdomains

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/helpdesk/brands"
	"github.com/useopendesk/opendesk/internal/jobs"
)

// Problems a domain can show (HelpDomain.problem).
const (
	problemTXT         = "txt_missing"
	problemCNAME       = "cname_missing"
	problemTaken       = "domain_taken"
	problemCertPending = "certificate_pending"
	problemCertFailed  = "certificate_failed"
	problemProvider    = "provider_error"
)

// Checking pace (contracts/teams.md "Help center addresses").
const (
	giveUpAfter   = 72 * time.Hour
	activeRecheck = 6 * time.Hour
)

// Service checks and provisions custom domains.
type Service struct {
	d   *deps.Deps
	cf  *Cloudflare // nil: the instance's own proxy issues certificates
	dns Resolver
	now func() time.Time
}

// New returns the service for the instance's configuration.
func New(d *deps.Deps) *Service {
	var cf *Cloudflare
	if d.Cfg.CloudflareSaaS() {
		cf = NewCloudflare(d.Cfg.CloudflareSaaSZoneID, d.Cfg.CloudflareSaaSToken)
	}
	return NewService(d, net.DefaultResolver, cf)
}

// NewService returns a service that reads DNS through dns and provisions certificates on
// cf (nil: the instance's own proxy issues them).
func NewService(d *deps.Deps, dns Resolver, cf *Cloudflare) *Service {
	return &Service{d: d, cf: cf, dns: dns, now: time.Now}
}

type snapshot struct {
	id, workspaceID, brandID uuid.UUID
	hostname, token          string
	status, problem          string
	txtOK, cnameOK           bool
	providerID               *string
	pendingSince             time.Time
	verifiedAt               *time.Time
}

func loadSnapshot(ctx context.Context, q db.Querier, id uuid.UUID) (snapshot, error) {
	var s snapshot
	err := q.QueryRow(ctx, `SELECT id, workspace_id, brand_id, hostname, token, status, problem, txt_ok, cname_ok, provider_id,
		pending_since, verified_at FROM help_domains WHERE id = $1`, id).
		Scan(&s.id, &s.workspaceID, &s.brandID, &s.hostname, &s.token, &s.status, &s.problem, &s.txtOK, &s.cnameOK,
			&s.providerID, &s.pendingSince, &s.verifiedAt)
	return s, err
}

type outcome struct {
	status, problem string
	txtOK, cnameOK  bool
	verified        bool
	providerID      *string
	created         bool // providerID was created by this check
}

// Check reads a domain's DNS records and, once they are in place, its certificate, and
// stores what it found. A manual check also publishes when nothing changed (the checked
// time did), and asks Cloudflare to validate a failed certificate again. A domain that
// is gone, or was replaced meanwhile, is left alone.
func (s *Service) Check(ctx context.Context, id uuid.UUID, manual bool) error {
	snap, err := loadSnapshot(ctx, s.d.DB.Pool, id)
	if db.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if snap.status == "failed" && !manual {
		return nil
	}
	out := outcome{status: snap.status, verified: snap.verifiedAt != nil, providerID: snap.providerID}
	out.txtOK = out.verified || txtFound(ctx, s.dns, snap.hostname, snap.token)
	out.cnameOK = pointsHere(ctx, s.dns, snap.hostname, s.d.Cfg.HelpCNAMETarget)

	if !out.verified && out.txtOK && out.cnameOK {
		taken, err := s.claim(ctx, snap)
		if err != nil {
			return err
		}
		if taken {
			out.status, out.problem = "failed", problemTaken
			return s.store(ctx, snap, out, manual)
		}
		out.verified = true
	}

	switch {
	case !out.verified:
		out.status = "pending"
		out.problem = problemTXT
		if out.txtOK {
			out.problem = problemCNAME
		}
	case s.cf == nil:
		// The proxy issues the certificate on the first visit.
		out.status, out.problem = "active", ""
		if !out.cnameOK {
			out.problem = problemCNAME
		}
	default:
		s.certificate(ctx, snap, &out, manual)
	}
	if out.status != "active" && s.now().Sub(snap.pendingSince) >= giveUpAfter {
		out.status = "failed"
	}
	return s.store(ctx, snap, out, manual)
}

// claim marks a domain verified. At most one claim of a hostname is verified: when
// another brand holds it, taken is true.
func (s *Service) claim(ctx context.Context, snap snapshot) (taken bool, err error) {
	_, err = s.d.DB.Pool.Exec(ctx, `UPDATE help_domains SET verified_at = now(), updated_at = now()
		WHERE id = $1 AND token = $2 AND verified_at IS NULL`, snap.id, snap.token)
	if db.IsUniqueViolation(err, "help_domains_verified_uq") {
		return true, nil
	}
	return false, err
}

// certificate makes sure Cloudflare has the hostname and reads its state.
func (s *Service) certificate(ctx context.Context, snap snapshot, out *outcome, manual bool) {
	wasActive := snap.status == "active"
	fail := func(err error) {
		s.d.Log.Warn("help domain: cloudflare", "domain_id", snap.id, "err", err)
		out.problem = problemProvider
		if !wasActive {
			out.status = "pending"
		}
	}
	var h CustomHostname
	var err error
	if out.providerID != nil {
		h, err = s.cf.Get(ctx, *out.providerID)
	}
	if out.providerID == nil || errors.Is(err, ErrNotFound) {
		if h, err = s.cf.Create(ctx, snap.hostname); err == nil {
			out.providerID, out.created = &h.ID, true
		}
	}
	if err != nil {
		fail(err)
		return
	}
	certFailed := false
	switch h.SSL.Status {
	case "expired", "initializing_timed_out", "validation_timed_out", "issuance_timed_out", "deployment_timed_out", "deletion_timed_out":
		certFailed = true
	}
	if certFailed && manual {
		if h2, err := s.cf.Revalidate(ctx, h.ID); err == nil {
			h, certFailed = h2, false
		}
	}
	switch {
	case h.Status == "active" && h.SSL.Status == "active":
		out.status, out.problem = "active", ""
		if !out.cnameOK {
			out.problem = problemCNAME
		}
		return
	case h.Status == "moved":
		out.problem = problemCNAME
	case h.Status == "blocked" || h.Status == "pending_blocked" || h.Status == "deleted" || h.Status == "pending_deletion":
		out.problem = problemProvider
	case certFailed:
		out.problem = problemCertFailed
	default:
		out.problem = problemCertPending
	}
	if !wasActive {
		out.status = "pending"
	}
}

// nextCheck is when a domain is checked next.
func nextCheck(now, pendingSince time.Time, o outcome) time.Time {
	if o.status == "active" {
		return now.Add(activeRecheck)
	}
	age := now.Sub(pendingSince)
	switch {
	case o.problem == problemCertPending && age < time.Hour:
		return now.Add(time.Minute)
	case age < time.Hour:
		return now.Add(2 * time.Minute)
	case age < 24*time.Hour:
		return now.Add(15 * time.Minute)
	}
	return now.Add(time.Hour)
}

// store writes a check's outcome and publishes the brand when it changed (or always,
// for a manual check). A hostname created at Cloudflare for a claim that was replaced
// meanwhile is released again.
func (s *Service) store(ctx context.Context, snap snapshot, o outcome, manual bool) error {
	now := s.now()
	next := nextCheck(now, snap.pendingSince, o)
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE help_domains SET status = $3, problem = $4, txt_ok = $5, cname_ok = $6, provider_id = $7,
			checked_at = now(), next_check_at = $8, active_at = CASE WHEN $3 = 'active' THEN COALESCE(active_at, now()) END, updated_at = now()
			WHERE id = $1 AND token = $2`, snap.id, snap.token, o.status, o.problem, o.txtOK, o.cnameOK, o.providerID, next)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			if o.created {
				_, err = s.d.Jobs.InsertTx(ctx, tx, jobs.HelpDomainReleaseArgs{ProviderID: *o.providerID}, nil)
			}
			return err
		}
		changed := o.status != snap.status || o.problem != snap.problem || o.txtOK != snap.txtOK || o.cnameOK != snap.cnameOK ||
			o.verified != (snap.verifiedAt != nil)
		if !changed && !manual {
			return nil
		}
		return brands.Publish(ctx, tx, s.d.Cfg, snap.workspaceID, snap.brandID, "brand.updated")
	})
}

// Release removes a custom hostname from Cloudflare unless a domain holds it again.
func (s *Service) Release(ctx context.Context, providerID string) error {
	if s.cf == nil || providerID == "" {
		return nil
	}
	var held bool
	if err := s.d.DB.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM help_domains WHERE provider_id = $1)`, providerID).Scan(&held); err != nil {
		return err
	}
	if held {
		return nil
	}
	return s.cf.Delete(ctx, providerID)
}

// Reconcile releases the zone's custom hostnames that no domain holds. The zone is
// dedicated to help centers; a hostname created in the last hour is left alone, as its
// check may not have stored it yet.
func (s *Service) Reconcile(ctx context.Context) error {
	if s.cf == nil {
		return nil
	}
	cutoff := s.now().Add(-time.Hour)
	var stray []string
	for page := 1; page <= 200; page++ {
		list, more, err := s.cf.List(ctx, page)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(list))
		for _, h := range list {
			if h.CreatedAt.Before(cutoff) {
				ids = append(ids, h.ID)
			}
		}
		rows, err := s.d.DB.Pool.Query(ctx, `SELECT x FROM unnest($1::text[]) x
			WHERE NOT EXISTS (SELECT 1 FROM help_domains hd WHERE hd.provider_id = x)`, ids)
		if err != nil {
			return err
		}
		found, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		stray = append(stray, found...)
		if !more {
			break
		}
	}
	for _, id := range stray {
		if err := s.cf.Delete(ctx, id); err != nil {
			return err
		}
	}
	if len(stray) > 0 {
		s.d.Log.Info("help domains: released stray custom hostnames", "count", len(stray))
	}
	return nil
}
