package admin

import (
	"context"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/core"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/plan"
)

type AdminWorkspaceRow struct {
	AdminWorkspaceRef
	AvatarURL      domain.Avatar `json:"avatar_url"`
	Members        int           `json:"members"`
	Plan           string        `json:"plan" enum:"free,pro,self_hosted"`
	Status         string        `json:"status"`
	SuspendedAt    *time.Time    `json:"suspended_at"`
	LastActivityAt *time.Time    `json:"last_activity_at"`
	CreatedAt      time.Time     `json:"created_at"`
}

type AdminMember struct {
	User struct {
		ID    uuid.UUID `json:"id"`
		Email string    `json:"email"`
		Name  string    `json:"name"`
	} `json:"user"`
	Role      string    `json:"role" enum:"owner,admin,agent,viewer"`
	CreatedAt time.Time `json:"created_at"`
}

type AdminCounts struct {
	Tickets     int `json:"tickets"`
	OpenTickets int `json:"open_tickets"`
	Contacts    int `json:"contacts"`
	KBArticles  int `json:"kb_articles"`
	APIKeys     int `json:"api_keys"`
	Webhooks    int `json:"webhooks"`
	Brands      int `json:"brands"`
}

type AdminMailbox struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	InboundKind  string     `json:"inbound_kind"`
	OutboundKind string     `json:"outbound_kind"`
	Status       string     `json:"status" enum:"active,error,disabled"`
	LastError    string     `json:"last_error"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

type AdminEntitlements struct {
	MaxMailboxes int     `json:"max_mailboxes"`
	MaxMembers   int     `json:"max_members"`
	MonthlyAIUSD float64 `json:"monthly_ai_usd"`
	UnlimitedAI  bool    `json:"unlimited_ai"`
}

// AdminLimits are the overrides; null keeps the plan's.
type AdminLimits struct {
	MaxMailboxes *int     `json:"max_mailboxes" minimum:"0" maximum:"100000"`
	MaxMembers   *int     `json:"max_members" minimum:"0" maximum:"100000"`
	MonthlyAIUSD *float64 `json:"monthly_ai_usd" minimum:"0" maximum:"100000"`
}

type AdminSubscription struct {
	Plan                 string     `json:"plan" enum:"free,pro"`
	Status               string     `json:"status"`
	Interval             string     `json:"interval" enum:"month,year"`
	Seats                int        `json:"seats"`
	BilledSeats          int        `json:"billed_seats"`
	CurrentPeriodEnd     *time.Time `json:"current_period_end"`
	CancelAtPeriodEnd    bool       `json:"cancel_at_period_end"`
	StripeCustomerID     *string    `json:"stripe_customer_id"`
	StripeSubscriptionID *string    `json:"stripe_subscription_id"`
}

type AdminAIFeature struct {
	Feature         string  `json:"feature"`
	ChargedUSD      float64 `json:"charged_usd"`
	ProviderCostUSD float64 `json:"provider_cost_usd"`
	Calls           int     `json:"calls"`
}

type AdminAIMonth struct {
	ChargedUSD      float64          `json:"charged_usd"`
	ProviderCostUSD float64          `json:"provider_cost_usd"`
	Calls           int              `json:"calls"`
	ByFeature       []AdminAIFeature `json:"by_feature"`
}

type AdminAI struct {
	RemainingUSD    float64      `json:"remaining_usd"`
	MonthlyUSD      float64      `json:"monthly_usd"`
	IncludedUsedUSD float64      `json:"included_used_usd"`
	PurchasedUSD    float64      `json:"purchased_usd"`
	Unlimited       bool         `json:"unlimited"`
	PeriodStart     time.Time    `json:"period_start"`
	PeriodEnd       time.Time    `json:"period_end"`
	Month           AdminAIMonth `json:"month"`
}

type AdminWorkspace struct {
	AdminWorkspaceRow
	SuspendedReason    string              `json:"suspended_reason"`
	MembersList        []AdminMember       `json:"members_list"`
	PendingInvitations int                 `json:"pending_invitations"`
	Counts             AdminCounts         `json:"counts"`
	Mailboxes          []AdminMailbox      `json:"mailboxes"`
	Entitlements       AdminEntitlements   `json:"entitlements"`
	Limits             AdminLimits         `json:"limits"`
	Subscription       *AdminSubscription  `json:"subscription"`
	AI                 AdminAI             `json:"ai"`
	RecentAudit        []core.CoreAuditLog `json:"recent_audit"`
}

// workspaceRowCols reads a workspace row; the plan is resolved after (self_hosted without billing).
const workspaceRowCols = `w.id, w.name, w.slug, w.avatar_url, (SELECT count(*) FROM members m WHERE m.workspace_id = w.id),
	COALESCE(sub.plan, 'free'), COALESCE(sub.status, 'none'), w.suspended_at,
	(SELECT e.created_at FROM events e WHERE e.workspace_id = w.id ORDER BY e.seq DESC LIMIT 1), w.created_at`

func (s *svc) scanWorkspaceRow(r pgx.Row, extra ...any) (AdminWorkspaceRow, error) {
	var x AdminWorkspaceRow
	err := r.Scan(append([]any{&x.ID, &x.Name, &x.Slug, &x.AvatarURL, &x.Members, &x.Plan, &x.Status, &x.SuspendedAt,
		&x.LastActivityAt, &x.CreatedAt}, extra...)...)
	switch {
	case !s.d.Cfg.BillingEnabled():
		x.Plan, x.Status = plan.SelfHosted, "active"
	case x.Plan == plan.Pro && !plan.Paid(x.Status):
		x.Plan = plan.Free
	}
	return x, err
}

func usd(m plan.Micro) float64 { return m.USD() }

func (s *svc) loadWorkspace(ctx context.Context, q db.Querier, wid uuid.UUID) (AdminWorkspace, error) {
	var w AdminWorkspace
	row, err := s.scanWorkspaceRow(q.QueryRow(ctx, `SELECT `+workspaceRowCols+`, w.suspended_reason FROM workspaces w
		LEFT JOIN subscriptions sub ON sub.workspace_id = w.id WHERE w.id = $1`, wid), &w.SuspendedReason)
	if db.IsNotFound(err) {
		return w, httpx.NotFound("Workspace")
	}
	if err != nil {
		return w, err
	}
	w.AdminWorkspaceRow = row
	rows, err := q.Query(ctx, `SELECT u.id, u.email, u.name, m.role, m.created_at FROM members m JOIN users u ON u.id = m.user_id
		WHERE m.workspace_id = $1 ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 WHEN 'agent' THEN 2 ELSE 3 END, m.created_at
		LIMIT 500`, wid)
	if err != nil {
		return w, err
	}
	if w.MembersList, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (AdminMember, error) {
		var x AdminMember
		err := r.Scan(&x.User.ID, &x.User.Email, &x.User.Name, &x.Role, &x.CreatedAt)
		return x, err
	}); err != nil {
		return w, err
	}
	c := &w.Counts
	if err := q.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM invitations WHERE workspace_id = $1 AND accepted_at IS NULL AND expires_at > now()),
		(SELECT count(*) FROM tickets WHERE workspace_id = $1),
		(SELECT count(*) FROM tickets WHERE workspace_id = $1 AND status IN ('open', 'pending', 'on_hold')),
		(SELECT count(*) FROM contacts WHERE workspace_id = $1),
		(SELECT count(*) FROM kb_articles WHERE workspace_id = $1),
		(SELECT count(*) FROM api_keys WHERE workspace_id = $1 AND revoked_at IS NULL),
		(SELECT count(*) FROM webhooks WHERE workspace_id = $1),
		(SELECT count(*) FROM brands WHERE workspace_id = $1)`, wid).
		Scan(&w.PendingInvitations, &c.Tickets, &c.OpenTickets, &c.Contacts, &c.KBArticles, &c.APIKeys, &c.Webhooks, &c.Brands); err != nil {
		return w, err
	}
	rows, err = q.Query(ctx, `SELECT id, name, email, inbound_kind, outbound_kind, status, last_error, last_synced_at, created_at
		FROM mailboxes WHERE workspace_id = $1 ORDER BY created_at`, wid)
	if err != nil {
		return w, err
	}
	if w.Mailboxes, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (AdminMailbox, error) {
		var x AdminMailbox
		err := r.Scan(&x.ID, &x.Name, &x.Email, &x.InboundKind, &x.OutboundKind, &x.Status, &x.LastError, &x.LastSyncedAt, &x.CreatedAt)
		return x, err
	}); err != nil {
		return w, err
	}
	e, err := s.d.Plan.Entitlements(ctx, q, wid)
	if err != nil {
		return w, err
	}
	w.Entitlements = AdminEntitlements{MaxMailboxes: e.MaxMailboxes, MaxMembers: e.MaxMembers, MonthlyAIUSD: usd(e.MonthlyUSD), UnlimitedAI: e.UnlimitedAI}
	o, err := plan.LoadOverrides(ctx, q, wid)
	if err != nil {
		return w, err
	}
	w.Limits = AdminLimits{MaxMailboxes: o.MaxMailboxes, MaxMembers: o.MaxMembers}
	if o.MonthlyUSD != nil {
		v := usd(*o.MonthlyUSD)
		w.Limits.MonthlyAIUSD = &v
	}
	if s.d.Cfg.BillingEnabled() {
		sub := &AdminSubscription{}
		if err := q.QueryRow(ctx, `SELECT plan, status, interval, seats, current_period_end, cancel_at_period_end, stripe_customer_id, stripe_subscription_id
			FROM subscriptions WHERE workspace_id = $1`, wid).Scan(&sub.Plan, &sub.Status, &sub.Interval, &sub.Seats, &sub.CurrentPeriodEnd,
			&sub.CancelAtPeriodEnd, &sub.StripeCustomerID, &sub.StripeSubscriptionID); err != nil {
			return w, err
		}
		if sub.BilledSeats, err = plan.BilledSeats(ctx, q, wid); err != nil {
			return w, err
		}
		w.Subscription = sub
	}
	b, err := s.d.Plan.Balance(ctx, q, wid)
	if err != nil {
		return w, err
	}
	w.AI = AdminAI{RemainingUSD: usd(b.Remaining), MonthlyUSD: usd(b.Monthly), IncludedUsedUSD: usd(b.IncludedUsed), PurchasedUSD: usd(b.Purchased),
		Unlimited: b.Unlimited, PeriodStart: b.PeriodStart, PeriodEnd: b.PeriodEnd}
	rows, err = q.Query(ctx, `SELECT feature, COALESCE(sum(micro_usd), 0)::bigint, COALESCE(sum(cost_usd), 0)::float8, count(*) FROM ai_usage
		WHERE workspace_id = $1 AND created_at >= $2 GROUP BY feature ORDER BY 2 DESC, 1`, wid, b.PeriodStart)
	if err != nil {
		return w, err
	}
	w.AI.Month.ByFeature, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (AdminAIFeature, error) {
		var x AdminAIFeature
		var micro plan.Micro
		err := r.Scan(&x.Feature, &micro, &x.ProviderCostUSD, &x.Calls)
		x.ChargedUSD = usd(micro)
		return x, err
	})
	if err != nil {
		return w, err
	}
	for _, f := range w.AI.Month.ByFeature {
		w.AI.Month.ChargedUSD += f.ChargedUSD
		w.AI.Month.ProviderCostUSD += f.ProviderCostUSD
		w.AI.Month.Calls += f.Calls
	}
	w.AI.Month.ChargedUSD = round6(w.AI.Month.ChargedUSD)
	w.AI.Month.ProviderCostUSD = round6(w.AI.Month.ProviderCostUSD)
	w.RecentAudit, err = core.AuditLogs(ctx, q, wid, time.Now().Add(time.Hour), uuid.Max, "", "", 20)
	if w.RecentAudit == nil {
		w.RecentAudit = []core.CoreAuditLog{}
	}
	return w, err
}

func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }

type workspacePath struct {
	ID string `path:"id"`
}

func (s *svc) registerWorkspaces(api huma.API) {
	op(s, api, huma.Operation{OperationID: "admin-list-workspaces", Method: http.MethodGet, Path: "/v1/admin/workspaces", Summary: "List workspaces"},
		func(ctx context.Context, in *struct {
			httpx.ListParams
			Q      string `query:"q" maxLength:"200"`
			Filter string `query:"filter" enum:"suspended,paid,free,"`
		}) (*struct{ Body httpx.Page[AdminWorkspaceRow] }, error) {
			if _, err := require(ctx, PermRead); err != nil {
				return nil, err
			}
			var cur struct {
				T  time.Time `json:"t"`
				ID uuid.UUID `json:"id"`
			}
			if err := httpx.DecodeCursor(in.Cursor, &cur); err != nil {
				return nil, err
			}
			if cur.T.IsZero() {
				cur.T, cur.ID = time.Now().Add(time.Hour), uuid.Max
			}
			q := strings.TrimSpace(in.Q)
			var byID *uuid.UUID
			if id, err := uuid.Parse(q); err == nil {
				byID = &id
			}
			paid := `(sub.plan = 'pro' AND sub.status IN ('active', 'trialing', 'past_due'))`
			rows, err := s.d.DB.Pool.Query(ctx, `SELECT `+workspaceRowCols+` FROM workspaces w
				LEFT JOIN subscriptions sub ON sub.workspace_id = w.id
				WHERE (w.created_at, w.id) < ($1, $2)
				AND ($3 = '' OR ($4::uuid IS NOT NULL AND w.id = $4) OR w.name ILIKE $5 OR w.slug ILIKE $5
					OR EXISTS (SELECT 1 FROM members m JOIN users u ON u.id = m.user_id WHERE m.workspace_id = w.id AND u.email ILIKE $5))
				AND ($6 = '' OR ($6 = 'suspended' AND w.suspended_at IS NOT NULL)
					OR ($6 = 'paid' AND `+paid+`) OR ($6 = 'free' AND NOT COALESCE(`+paid+`, false)))
				ORDER BY w.created_at DESC, w.id DESC LIMIT $7`, cur.T, cur.ID, q, byID, likePattern(q), in.Filter, in.Limit+1)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (AdminWorkspaceRow, error) { return s.scanWorkspaceRow(r) })
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			page := httpx.NewPage(list, in.Limit, func(x AdminWorkspaceRow) any { return map[string]any{"t": x.CreatedAt, "id": x.ID} })
			return &struct{ Body httpx.Page[AdminWorkspaceRow] }{page}, nil
		})

	op(s, api, huma.Operation{OperationID: "admin-get-workspace", Method: http.MethodGet, Path: "/v1/admin/workspaces/{id}", Summary: "Get a workspace"},
		func(ctx context.Context, in *workspacePath) (*struct{ Body AdminWorkspace }, error) {
			if _, err := require(ctx, PermRead); err != nil {
				return nil, err
			}
			wid, err := parseID(in.ID, "Workspace")
			if err != nil {
				return nil, err
			}
			w, err := s.loadWorkspace(ctx, s.d.DB.Pool, wid)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body AdminWorkspace }{w}, nil
		})

	op(s, api, huma.Operation{OperationID: "admin-suspend-workspace", Method: http.MethodPost, Path: "/v1/admin/workspaces/{id}/suspend", Summary: "Suspend a workspace"},
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body reasonBody
		}) (*struct{ Body AdminWorkspace }, error) {
			a, err := require(ctx, PermWorkspaceSuspend)
			if err != nil {
				return nil, err
			}
			wid, err := parseID(in.ID, "Workspace")
			if err != nil {
				return nil, err
			}
			reason := strings.TrimSpace(in.Body.Reason)
			if reason == "" {
				return nil, httpx.Validation("Give a reason.")
			}
			var w AdminWorkspace
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				tag, err := tx.Exec(ctx, `UPDATE workspaces SET suspended_at = now(), suspended_reason = $2, updated_at = now()
					WHERE id = $1 AND suspended_at IS NULL`, wid, reason)
				if err != nil {
					return err
				}
				if tag.RowsAffected() == 0 {
					if err := workspaceExists(ctx, tx, wid); err != nil {
						return err
					}
					return httpx.E(409, "conflict", "This workspace is already suspended.")
				}
				if err := s.ops.AuditWorkspace(ctx, tx, a.principal(), wid, "workspace.suspended", nil); err != nil {
					return err
				}
				if err := s.audit(ctx, tx, a, "workspace.suspended", "workspace", wid.String(), map[string]any{"reason": reason}); err != nil {
					return err
				}
				w, err = s.loadWorkspace(ctx, tx, wid)
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			// Members' apps reconnect and find the workspace suspended.
			for _, m := range w.MembersList {
				s.ops.Disconnect(m.User.ID, uuid.Nil)
			}
			return &struct{ Body AdminWorkspace }{w}, nil
		})

	op(s, api, huma.Operation{OperationID: "admin-unsuspend-workspace", Method: http.MethodPost, Path: "/v1/admin/workspaces/{id}/unsuspend", Summary: "Restore a suspended workspace"},
		func(ctx context.Context, in *workspacePath) (*struct{ Body AdminWorkspace }, error) {
			a, err := require(ctx, PermWorkspaceSuspend)
			if err != nil {
				return nil, err
			}
			wid, err := parseID(in.ID, "Workspace")
			if err != nil {
				return nil, err
			}
			var w AdminWorkspace
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				tag, err := tx.Exec(ctx, `UPDATE workspaces SET suspended_at = NULL, suspended_reason = '', updated_at = now()
					WHERE id = $1 AND suspended_at IS NOT NULL`, wid)
				if err != nil {
					return err
				}
				if tag.RowsAffected() == 0 {
					if err := workspaceExists(ctx, tx, wid); err != nil {
						return err
					}
					return httpx.E(409, "conflict", "This workspace is not suspended.")
				}
				if err := s.ops.AuditWorkspace(ctx, tx, a.principal(), wid, "workspace.unsuspended", nil); err != nil {
					return err
				}
				if err := s.audit(ctx, tx, a, "workspace.unsuspended", "workspace", wid.String(), nil); err != nil {
					return err
				}
				w, err = s.loadWorkspace(ctx, tx, wid)
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			for _, m := range w.MembersList {
				s.ops.Disconnect(m.User.ID, uuid.Nil)
			}
			return &struct{ Body AdminWorkspace }{w}, nil
		})

	op(s, api, huma.Operation{OperationID: "admin-set-workspace-limits", Method: http.MethodPut, Path: "/v1/admin/workspaces/{id}/limits",
		Summary: "Set a workspace's limit overrides"},
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body AdminLimits
		}) (*struct{ Body AdminWorkspace }, error) {
			a, err := require(ctx, PermWorkspaceLimits)
			if err != nil {
				return nil, err
			}
			wid, err := parseID(in.ID, "Workspace")
			if err != nil {
				return nil, err
			}
			o := plan.Overrides{MaxMailboxes: in.Body.MaxMailboxes, MaxMembers: in.Body.MaxMembers}
			if v := in.Body.MonthlyAIUSD; v != nil {
				m := plan.FromUSD(*v)
				o.MonthlyUSD = &m
			}
			var w AdminWorkspace
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if err := workspaceExists(ctx, tx, wid); err != nil {
					return err
				}
				if err := plan.SetOverrides(ctx, tx, wid, o); err != nil {
					return err
				}
				data := map[string]any{"max_mailboxes": in.Body.MaxMailboxes, "max_members": in.Body.MaxMembers, "monthly_ai_usd": in.Body.MonthlyAIUSD}
				if err := s.ops.AuditWorkspace(ctx, tx, a.principal(), wid, "workspace.limits_changed", data); err != nil {
					return err
				}
				if err := s.audit(ctx, tx, a, "workspace.limits_changed", "workspace", wid.String(), data); err != nil {
					return err
				}
				w, err = s.loadWorkspace(ctx, tx, wid)
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body AdminWorkspace }{w}, nil
		})

	op(s, api, huma.Operation{OperationID: "admin-grant-workspace-ai", Method: http.MethodPost, Path: "/v1/admin/workspaces/{id}/ai-grants",
		Summary: "Grant a workspace AI usage"},
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body struct {
				AmountUSD float64 `json:"amount_usd" minimum:"0.01" maximum:"10000"`
				Note      string  `json:"note" minLength:"1" maxLength:"200"`
			}
		}) (*struct{ Body AdminWorkspace }, error) {
			a, err := require(ctx, PermWorkspaceLimits)
			if err != nil {
				return nil, err
			}
			wid, err := parseID(in.ID, "Workspace")
			if err != nil {
				return nil, err
			}
			amount := plan.FromUSD(math.Round(in.Body.AmountUSD*100) / 100)
			note := strings.TrimSpace(in.Body.Note)
			if amount <= 0 || note == "" {
				return nil, httpx.Validation("Give an amount of at least $0.01 and a note.")
			}
			var w AdminWorkspace
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if err := workspaceExists(ctx, tx, wid); err != nil {
					return err
				}
				grant := uuid.New()
				if _, err := plan.Grant(ctx, tx, wid, amount, "admin", "admin:"+grant.String()); err != nil {
					return err
				}
				data := map[string]any{"amount_usd": usd(amount), "note": note, "grant": grant}
				if err := s.ops.AuditWorkspace(ctx, tx, a.principal(), wid, "workspace.ai_granted", map[string]any{"amount_usd": usd(amount)}); err != nil {
					return err
				}
				if err := s.audit(ctx, tx, a, "workspace.ai_granted", "workspace", wid.String(), data); err != nil {
					return err
				}
				w, err = s.loadWorkspace(ctx, tx, wid)
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body AdminWorkspace }{w}, nil
		})

	op(s, api, huma.Operation{OperationID: "admin-delete-workspace", Method: http.MethodDelete, Path: "/v1/admin/workspaces/{id}",
		Summary: "Delete a workspace and all its data", DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body struct {
				Code        string `json:"code,omitempty" maxLength:"64" doc:"A current authenticator code (or a recovery code) of the acting admin"`
				ConfirmName string `json:"confirm_name" maxLength:"200"`
			}
		}) (*struct{}, error) {
			a, err := require(ctx, PermWorkspaceDelete)
			if err != nil {
				return nil, err
			}
			wid, err := parseID(in.ID, "Workspace")
			if err != nil {
				return nil, err
			}
			var removal core.Removal
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if err := s.stepUp(ctx, tx, a, in.Body.Code); err != nil {
					return err
				}
				var name string
				err := tx.QueryRow(ctx, `SELECT name FROM workspaces WHERE id = $1 FOR UPDATE`, wid).Scan(&name)
				if db.IsNotFound(err) {
					return httpx.NotFound("Workspace")
				}
				if err != nil {
					return err
				}
				if strings.TrimSpace(in.Body.ConfirmName) != name {
					return httpx.E(422, "confirm_name_mismatch", "Type the workspace name exactly to confirm.")
				}
				if removal, err = s.ops.DeleteWorkspace(ctx, tx, wid); err != nil {
					return err
				}
				return s.audit(ctx, tx, a, "workspace.deleted", "workspace", wid.String(), nil)
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			s.d.Log.Info("workspace deleted by an instance admin", "workspace_id", wid, "admin_user_id", a.UserID, "ip", a.IP)
			s.ops.Finish(removal)
			return nil, nil
		})
}

func workspaceExists(ctx context.Context, q db.Querier, wid uuid.UUID) error {
	var ok bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspaces WHERE id = $1)`, wid).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return httpx.NotFound("Workspace")
	}
	return nil
}
