package helpdomains

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/audit"
	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/helpdesk/brands"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/ids"
	"github.com/useopendesk/opendesk/internal/jobs"
)

// checkTimeout bounds the check an admin waits for.
const checkTimeout = 12 * time.Second

type brandOut struct{ Body brands.Brand }

// Register adds the custom domain routes of a brand.
func Register(d *deps.Deps, api huma.API) { RegisterService(d, api, New(d)) }

// RegisterService adds the routes, checking domains with s.
func RegisterService(d *deps.Deps, api huma.API, s *Service) {
	h := &handler{d: d, s: s}
	tags := []string{"Brands"}

	huma.Register(api, huma.Operation{OperationID: "set-help-domain", Method: http.MethodPut, Path: "/v1/brands/{id}/help-domain",
		Summary: "Set the help center's custom domain", Tags: tags,
		Description: "Sets a subdomain the customer controls, such as help.example.com, and checks it right away. " +
			"The answer's help_center.domain.records are the DNS records to add."},
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body struct {
				Hostname string `json:"hostname" maxLength:"300"`
			}
		}) (*brandOut, error) {
			return h.set(ctx, in.ID, in.Body.Hostname)
		})

	huma.Register(api, huma.Operation{OperationID: "check-help-domain", Method: http.MethodPost, Path: "/v1/brands/{id}/help-domain/check",
		Summary: "Check the help center's custom domain now", Tags: tags},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*brandOut, error) {
			return h.check(ctx, in.ID)
		})

	huma.Register(api, huma.Operation{OperationID: "remove-help-domain", Method: http.MethodDelete, Path: "/v1/brands/{id}/help-domain",
		Summary: "Remove the help center's custom domain", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*hd.Empty, error) {
			return nil, h.remove(ctx, in.ID)
		})
}

type handler struct {
	d *deps.Deps
	s *Service
}

func (h *handler) brand(ctx context.Context, raw string) (*auth.Principal, uuid.UUID, error) {
	p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsWrite)
	if err != nil {
		return nil, uuid.Nil, httpx.Wrap(err)
	}
	id, err := hd.ParseID(raw, "Brand")
	if err != nil {
		return nil, uuid.Nil, err
	}
	return p, id, nil
}

// lockBrand locks a brand of the workspace and returns its domain, if any.
func lockBrand(ctx context.Context, tx pgx.Tx, wid, brandID uuid.UUID) (*snapshot, error) {
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT true FROM brands WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, wid, brandID).Scan(&ok); err != nil {
		if db.IsNotFound(err) {
			return nil, httpx.NotFound("Brand")
		}
		return nil, err
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM help_domains WHERE workspace_id = $1 AND brand_id = $2`, wid, brandID).Scan(&id)
	if db.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s, err := loadSnapshot(ctx, tx, id)
	return &s, err
}

func (h *handler) set(ctx context.Context, rawID, rawHost string) (*brandOut, error) {
	p, brandID, err := h.brand(ctx, rawID)
	if err != nil {
		return nil, err
	}
	host, err := CleanHostname(h.d.Cfg, rawHost)
	if err != nil {
		return nil, err
	}
	if !h.d.Limiter.Allow("helpdomain:"+p.WorkspaceID.String(), 30, time.Hour) {
		return nil, httpx.RateLimited()
	}
	var domainID uuid.UUID
	err = h.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		cur, err := lockBrand(ctx, tx, p.WorkspaceID, brandID)
		if err != nil {
			return err
		}
		if cur != nil && cur.hostname == host {
			domainID = cur.id
			return nil
		}
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM help_domains WHERE hostname = $1 AND verified_at IS NOT NULL AND brand_id <> $2)`,
			host, brandID).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return httpx.E(http.StatusConflict, "domain_taken", "Another help center already uses this domain.")
		}
		previous := ""
		if cur != nil {
			previous = cur.hostname
			if err := h.drop(ctx, tx, *cur); err != nil {
				return err
			}
		}
		domainID = ids.New()
		if _, err := tx.Exec(ctx, `INSERT INTO help_domains (id, workspace_id, brand_id, hostname, token) VALUES ($1, $2, $3, $4, $5)`,
			domainID, p.WorkspaceID, brandID, host, crypto.RandomString(32)); err != nil {
			return err
		}
		if err := audit.Log(ctx, tx, p, p.WorkspaceID, "brand.help_domain_set", "brand", &brandID,
			map[string]any{"hostname": host, "previous": previous}); err != nil {
			return err
		}
		if err := enqueueCheck(ctx, h.d, tx, domainID); err != nil {
			return err
		}
		return brands.Publish(ctx, tx, h.d.Cfg, p.WorkspaceID, brandID, "brand.updated")
	})
	if err != nil {
		return nil, httpx.Wrap(err)
	}
	return h.checkNow(ctx, p.WorkspaceID, brandID, domainID)
}

func (h *handler) check(ctx context.Context, rawID string) (*brandOut, error) {
	p, brandID, err := h.brand(ctx, rawID)
	if err != nil {
		return nil, err
	}
	if !h.d.Limiter.Allow("helpdomain-check:"+brandID.String(), 6, time.Minute) {
		return nil, httpx.RateLimited()
	}
	var domainID uuid.UUID
	err = h.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		cur, err := lockBrand(ctx, tx, p.WorkspaceID, brandID)
		if err != nil {
			return err
		}
		if cur == nil {
			return httpx.NotFound("Help center domain")
		}
		domainID = cur.id
		// A failed domain starts over.
		_, err = tx.Exec(ctx, `UPDATE help_domains SET status = 'pending', pending_since = now(), problem = CASE WHEN problem = 'domain_taken' THEN '' ELSE problem END,
			updated_at = now() WHERE id = $1 AND status = 'failed'`, cur.id)
		return err
	})
	if err != nil {
		return nil, httpx.Wrap(err)
	}
	return h.checkNow(ctx, p.WorkspaceID, brandID, domainID)
}

// checkNow checks the domain while the admin waits, then answers the brand. A check that
// cannot finish in time is left to the scheduled one.
func (h *handler) checkNow(ctx context.Context, wid, brandID, domainID uuid.UUID) (*brandOut, error) {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), checkTimeout)
	defer cancel()
	if err := h.s.Check(cctx, domainID, true); err != nil {
		h.d.Log.Warn("help domain check", "domain_id", domainID, "err", err)
	}
	b, err := brands.Load(ctx, h.d.DB.Pool, h.d.Cfg, wid, brandID)
	if err != nil {
		return nil, httpx.Wrap(err)
	}
	return &brandOut{b}, nil
}

func (h *handler) remove(ctx context.Context, rawID string) error {
	p, brandID, err := h.brand(ctx, rawID)
	if err != nil {
		return err
	}
	err = h.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		cur, err := lockBrand(ctx, tx, p.WorkspaceID, brandID)
		if err != nil {
			return err
		}
		if cur == nil {
			return nil
		}
		if err := h.drop(ctx, tx, *cur); err != nil {
			return err
		}
		if err := audit.Log(ctx, tx, p, p.WorkspaceID, "brand.help_domain_removed", "brand", &brandID,
			map[string]any{"hostname": cur.hostname}); err != nil {
			return err
		}
		return brands.Publish(ctx, tx, h.d.Cfg, p.WorkspaceID, brandID, "brand.updated")
	})
	return httpx.Wrap(err)
}

// drop deletes a domain and releases its Cloudflare hostname.
func (h *handler) drop(ctx context.Context, tx pgx.Tx, cur snapshot) error {
	if _, err := tx.Exec(ctx, `DELETE FROM help_domains WHERE id = $1`, cur.id); err != nil {
		return err
	}
	if cur.providerID == nil {
		return nil
	}
	_, err := h.d.Jobs.InsertTx(ctx, tx, jobs.HelpDomainReleaseArgs{ProviderID: *cur.providerID}, nil)
	return err
}
