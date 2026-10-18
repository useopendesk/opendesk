package images

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/domain"
)

// RefreshAfter is how long an automatic picture is kept before it is looked up again.
const RefreshAfter = 30 * 24 * time.Hour

// Backoff is the wait after the n-th consecutive miss (n >= 1): 1, 2, 4, 8, 16, then 30 days.
func Backoff(n int) time.Duration {
	if n < 1 {
		n = 1
	}
	if n > 5 {
		return RefreshAfter
	}
	return time.Duration(1<<(n-1)) * 24 * time.Hour
}

// current reads the stored picture of an owner and locks its row.
func current(ctx context.Context, tx pgx.Tx, o Owner) (domain.Avatar, *string, error) {
	var val string
	var src *string
	var err error
	switch o.Type {
	case OwnerUser:
		err = tx.QueryRow(ctx, `SELECT avatar_url, avatar_source FROM users WHERE id = $1 FOR UPDATE`, o.ID).Scan(&val, &src)
	case OwnerContact:
		err = tx.QueryRow(ctx, `SELECT avatar_url, avatar_source FROM contacts WHERE workspace_id = $1 AND id = $2 FOR UPDATE`,
			*o.WorkspaceID, o.ID).Scan(&val, &src)
	case OwnerCompany:
		err = tx.QueryRow(ctx, `SELECT avatar_url, avatar_source FROM companies WHERE workspace_id = $1 AND id = $2 FOR UPDATE`,
			*o.WorkspaceID, o.ID).Scan(&val, &src)
	case OwnerWorkspace:
		if o.Slot == SlotLogo || o.Slot == "" {
			err = tx.QueryRow(ctx, `SELECT avatar_url FROM workspaces WHERE id = $1 FOR UPDATE`, o.ID).Scan(&val)
		} else {
			err = tx.QueryRow(ctx, `SELECT COALESCE(brand_images ->> $2, '') FROM workspaces WHERE id = $1 FOR UPDATE`, o.ID, o.Slot).Scan(&val)
		}
	case OwnerBrand, OwnerWidget:
		table := "brands"
		if o.Type == OwnerWidget {
			table = "widgets"
		}
		err = tx.QueryRow(ctx, `SELECT COALESCE(images ->> $3, '') FROM `+table+` WHERE workspace_id = $1 AND id = $2 FOR UPDATE`,
			*o.WorkspaceID, o.ID, o.Slot).Scan(&val)
	case OwnerBot:
		err = tx.QueryRow(ctx, `SELECT avatar FROM bots WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, *o.WorkspaceID, o.ID).Scan(&val)
	case OwnerIntegration:
		err = tx.QueryRow(ctx, `SELECT icon FROM integrations WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, *o.WorkspaceID, o.ID).Scan(&val)
	default:
		return "", nil, fmt.Errorf("images: unknown owner %q", o.Type)
	}
	if db.IsNotFound(err) {
		return "", nil, ErrOwnerGone
	}
	return domain.Avatar(val), src, err
}

// write stores val (and source, nil = none) on the owner. Automatic sources also set
// the next check.
func write(ctx context.Context, tx pgx.Tx, o Owner, val domain.Avatar, source *string) error {
	var err error
	auto := source != nil && *source != "upload"
	switch o.Type {
	case OwnerUser:
		_, err = tx.Exec(ctx, `UPDATE users SET avatar_url = $2, avatar_source = $3, updated_at = now() WHERE id = $1`, o.ID, string(val), source)
	case OwnerContact, OwnerCompany:
		table := "contacts"
		if o.Type == OwnerCompany {
			table = "companies"
		}
		var retry *time.Time
		if auto {
			t := time.Now().Add(RefreshAfter)
			retry = &t
		}
		_, err = tx.Exec(ctx, `UPDATE `+table+` SET avatar_url = $3, avatar_source = $4, avatar_checked_at = now(),
			avatar_retry_at = $5, avatar_failures = 0, updated_at = now() WHERE workspace_id = $1 AND id = $2`,
			*o.WorkspaceID, o.ID, string(val), source, retry)
	case OwnerWorkspace:
		if o.Slot == SlotLogo || o.Slot == "" {
			_, err = tx.Exec(ctx, `UPDATE workspaces SET avatar_url = $2, updated_at = now() WHERE id = $1`, o.ID, string(val))
		} else if !slices.Contains(WorkspaceSlots, o.Slot) {
			return fmt.Errorf("images: unknown slot %q", o.Slot)
		} else if val == "" {
			_, err = tx.Exec(ctx, `UPDATE workspaces SET brand_images = brand_images - $2, updated_at = now() WHERE id = $1`, o.ID, o.Slot)
		} else {
			_, err = tx.Exec(ctx, `UPDATE workspaces SET brand_images = brand_images || jsonb_build_object($2::text, $3::text), updated_at = now() WHERE id = $1`,
				o.ID, o.Slot, string(val))
		}
	case OwnerBot:
		_, err = tx.Exec(ctx, `UPDATE bots SET avatar = $3, updated_at = now() WHERE workspace_id = $1 AND id = $2`, *o.WorkspaceID, o.ID, string(val))
	case OwnerIntegration:
		_, err = tx.Exec(ctx, `UPDATE integrations SET icon = $3, updated_at = now() WHERE workspace_id = $1 AND id = $2`, *o.WorkspaceID, o.ID, string(val))
	case OwnerBrand, OwnerWidget:
		table, slots := "brands", BrandSlots
		if o.Type == OwnerWidget {
			table, slots = "widgets", WidgetSlots
		}
		if !slices.Contains(slots, o.Slot) {
			return fmt.Errorf("images: unknown slot %q", o.Slot)
		}
		if val == "" {
			_, err = tx.Exec(ctx, `UPDATE `+table+` SET images = images - $3, updated_at = now() WHERE workspace_id = $1 AND id = $2`,
				*o.WorkspaceID, o.ID, o.Slot)
		} else {
			_, err = tx.Exec(ctx, `UPDATE `+table+` SET images = images || jsonb_build_object($3::text, $4::text), updated_at = now()
				WHERE workspace_id = $1 AND id = $2`, *o.WorkspaceID, o.ID, o.Slot, string(val))
		}
	}
	return err
}

// changed runs the change hook of o, and the workspace's when o stands in for a
// workspace slot.
func changed(ctx context.Context, tx pgx.Tx, o Owner, moved bool) error {
	if err := Changed(ctx, tx, o); err != nil {
		return err
	}
	if moved && o.WorkspaceID != nil {
		return Changed(ctx, tx, Owner{Type: OwnerWorkspace, ID: *o.WorkspaceID, WorkspaceID: o.WorkspaceID})
	}
	return nil
}

// SetOptions tune Set and Clear.
type SetOptions struct {
	// Before runs first in the transaction (permission re-checks, audit). It may refuse.
	Before func(ctx context.Context, tx pgx.Tx, old domain.Avatar, oldSource *string) error
	// After runs after the write, before the change hook (audit).
	After func(ctx context.Context, tx pgx.Tx) error
	// Quiet skips the change hook (the caller publishes its own event).
	Quiet bool
}

// Set stores a processed picture as the owner's, replacing the previous one, and runs
// the change hook in the same transaction.
func Set(ctx context.Context, d *deps.Deps, o Owner, p Processed, source string, opt SetOptions) error {
	o, moved, err := ownerOf(ctx, d.DB.Pool, o)
	if err != nil {
		return err
	}
	id, prefix, err := Put(ctx, d.Storage, o, p)
	if err != nil {
		return err
	}
	var oldPrefix string
	err = d.DB.Tx(ctx, func(tx pgx.Tx) error {
		old, oldSrc, err := current(ctx, tx, o)
		if err != nil {
			return err
		}
		if opt.Before != nil {
			if err := opt.Before(ctx, tx, old, oldSrc); err != nil {
				return err
			}
		}
		if oldPrefix, err = Forget(ctx, tx, old); err != nil {
			return err
		}
		if err := Insert(ctx, tx, o, id, prefix, source, p.SHA256); err != nil {
			return err
		}
		src := source
		if err := write(ctx, tx, o, domain.AvatarRef(id), &src); err != nil {
			return err
		}
		if opt.After != nil {
			if err := opt.After(ctx, tx); err != nil {
				return err
			}
		}
		if opt.Quiet {
			return nil
		}
		return changed(ctx, tx, o, moved)
	})
	if err != nil {
		Discard(d.Storage, prefix)
		return err
	}
	if oldPrefix != "" {
		Discard(d.Storage, oldPrefix)
	}
	return nil
}

// Clear removes the owner's picture. With retry, contacts and companies are looked up
// again by the refresh job right away.
func Clear(ctx context.Context, d *deps.Deps, o Owner, retry bool, opt SetOptions) error {
	var oldPrefix string
	err := d.DB.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		if oldPrefix, err = ClearTx(ctx, tx, o, retry, opt); err != nil {
			return err
		}
		return nil
	})
	if err == nil && oldPrefix != "" {
		Discard(d.Storage, oldPrefix)
	}
	return err
}

// ClearTx is Clear inside a caller's transaction. It returns the key prefix of the
// removed image, to Discard after commit.
func ClearTx(ctx context.Context, tx pgx.Tx, o Owner, retry bool, opt SetOptions) (string, error) {
	o, moved, err := ownerOf(ctx, tx, o)
	if err != nil {
		return "", err
	}
	old, oldSrc, err := current(ctx, tx, o)
	if err != nil {
		return "", err
	}
	if opt.Before != nil {
		if err := opt.Before(ctx, tx, old, oldSrc); err != nil {
			return "", err
		}
	}
	prefix, err := Forget(ctx, tx, old)
	if err != nil {
		return "", err
	}
	if err := write(ctx, tx, o, "", nil); err != nil {
		return "", err
	}
	// A removed manual picture also drops the URL it was copied from.
	if oldSrc != nil && *oldSrc == "upload" {
		switch o.Type {
		case OwnerUser:
			_, err = tx.Exec(ctx, `UPDATE users SET avatar_remote_url = '' WHERE id = $1`, o.ID)
		case OwnerContact:
			_, err = tx.Exec(ctx, `UPDATE contacts SET avatar_remote_url = '' WHERE workspace_id = $1 AND id = $2`, *o.WorkspaceID, o.ID)
		}
		if err != nil {
			return "", err
		}
	}
	if retry && (o.Type == OwnerContact || o.Type == OwnerCompany) {
		table := "contacts"
		if o.Type == OwnerCompany {
			table = "companies"
		}
		if _, err := tx.Exec(ctx, `UPDATE `+table+` SET avatar_retry_at = now(), avatar_failures = 0 WHERE workspace_id = $1 AND id = $2`,
			*o.WorkspaceID, o.ID); err != nil {
			return "", err
		}
	}
	if opt.After != nil {
		if err := opt.After(ctx, tx); err != nil {
			return "", err
		}
	}
	if opt.Quiet {
		return prefix, nil
	}
	return prefix, changed(ctx, tx, o, moved)
}
