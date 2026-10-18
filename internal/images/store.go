package images

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/storage"
)

// Owner types and workspace image slots.
const (
	OwnerUser      = "user"
	OwnerContact   = "contact"
	OwnerCompany   = "company"
	OwnerWorkspace = "workspace"
	OwnerBrand     = "brand"
	OwnerWidget    = "widget"
	OwnerBot       = "bot"
	// OwnerIntegration is a custom MCP server's icon (integrations.icon).
	OwnerIntegration = "integration"

	SlotAvatar = "avatar"
	SlotLogo   = "logo"
)

// WorkspaceSlots are the workspace images besides the logo (workspaces.brand_images).
// help_center_* belong to the default brand and widget_avatar/widget_launcher to the
// default brand's first widget: writes to them go there (see ownerOf).
var WorkspaceSlots = []string{"help_center_logo", "help_center_favicon", "widget_avatar", "widget_launcher"}

// BrandSlots are a brand's images (brands.images).
var BrandSlots = []string{"logo", "help_center_logo", "help_center_favicon"}

// WidgetSlots are a chat widget's images (widgets.images).
var WidgetSlots = []string{"avatar", "launcher"}

// Owner identifies what a picture belongs to.
type Owner struct {
	Type        string
	ID          uuid.UUID
	WorkspaceID *uuid.UUID // nil for a user's own picture
	Slot        string
}

// Hooks publish the change events of each owner type (installed by the owning areas,
// so this package depends on none of them). Each runs in the write's transaction.
type Hooks struct {
	User      func(ctx context.Context, tx pgx.Tx, uid uuid.UUID) error
	Contact   func(ctx context.Context, tx pgx.Tx, wid, id uuid.UUID) error
	Company   func(ctx context.Context, tx pgx.Tx, wid, id uuid.UUID) error
	Workspace func(ctx context.Context, tx pgx.Tx, wid uuid.UUID) error
	Brand     func(ctx context.Context, tx pgx.Tx, wid, id uuid.UUID) error
	Widget    func(ctx context.Context, tx pgx.Tx, wid, id uuid.UUID) error
	Bot       func(ctx context.Context, tx pgx.Tx, wid, id uuid.UUID) error
}

var hooks atomic.Pointer[Hooks]

// SetHooks merges non-nil hooks into the installed set.
func SetHooks(h Hooks) {
	cur := Hooks{}
	if p := hooks.Load(); p != nil {
		cur = *p
	}
	if h.User != nil {
		cur.User = h.User
	}
	if h.Contact != nil {
		cur.Contact = h.Contact
	}
	if h.Company != nil {
		cur.Company = h.Company
	}
	if h.Workspace != nil {
		cur.Workspace = h.Workspace
	}
	if h.Brand != nil {
		cur.Brand = h.Brand
	}
	if h.Widget != nil {
		cur.Widget = h.Widget
	}
	if h.Bot != nil {
		cur.Bot = h.Bot
	}
	hooks.Store(&cur)
}

// Changed runs the hook for an owner.
func Changed(ctx context.Context, tx pgx.Tx, o Owner) error {
	h := hooks.Load()
	if h == nil {
		return nil
	}
	switch o.Type {
	case OwnerUser:
		if h.User != nil {
			return h.User(ctx, tx, o.ID)
		}
	case OwnerContact:
		if h.Contact != nil && o.WorkspaceID != nil {
			return h.Contact(ctx, tx, *o.WorkspaceID, o.ID)
		}
	case OwnerCompany:
		if h.Company != nil && o.WorkspaceID != nil {
			return h.Company(ctx, tx, *o.WorkspaceID, o.ID)
		}
	case OwnerWorkspace:
		if h.Workspace != nil {
			return h.Workspace(ctx, tx, o.ID)
		}
	case OwnerBrand:
		if h.Brand != nil && o.WorkspaceID != nil {
			return h.Brand(ctx, tx, *o.WorkspaceID, o.ID)
		}
	case OwnerWidget:
		if h.Widget != nil && o.WorkspaceID != nil {
			return h.Widget(ctx, tx, *o.WorkspaceID, o.ID)
		}
	case OwnerBot:
		if h.Bot != nil && o.WorkspaceID != nil {
			return h.Bot(ctx, tx, *o.WorkspaceID, o.ID)
		}
	}
	return nil
}

// legacySlots maps the workspace slots that now live on the default brand or its first
// widget to that owner's slot.
var legacySlots = map[string]struct{ owner, slot string }{
	"help_center_logo":    {OwnerBrand, "help_center_logo"},
	"help_center_favicon": {OwnerBrand, "help_center_favicon"},
	"widget_avatar":       {OwnerWidget, "avatar"},
	"widget_launcher":     {OwnerWidget, "launcher"},
}

// ownerOf returns the owner a picture is stored on. A workspace slot kept for older
// clients resolves to the default brand or the default brand's first widget; moved
// reports that, so the workspace's change hook runs too.
func ownerOf(ctx context.Context, q db.Querier, o Owner) (Owner, bool, error) {
	m, ok := legacySlots[o.Slot]
	if o.Type != OwnerWorkspace || !ok {
		return o, false, nil
	}
	var id uuid.UUID
	var err error
	if m.owner == OwnerBrand {
		err = q.QueryRow(ctx, `SELECT id FROM brands WHERE workspace_id = $1 AND is_default`, o.ID).Scan(&id)
	} else {
		err = q.QueryRow(ctx, `SELECT g.id FROM widgets g JOIN brands b ON b.id = g.brand_id AND b.is_default
			WHERE g.workspace_id = $1 ORDER BY g.position, g.created_at, g.id LIMIT 1`, o.ID).Scan(&id)
	}
	if db.IsNotFound(err) {
		if m.owner == OwnerBrand {
			return o, false, httpx.NotFound("Brand")
		}
		return o, false, httpx.NotFound("Widget")
	}
	if err != nil {
		return o, false, err
	}
	wid := o.ID
	return Owner{Type: m.owner, ID: id, WorkspaceID: &wid, Slot: m.slot}, true, nil
}

func keyPrefix(o Owner, id uuid.UUID) string {
	if o.WorkspaceID != nil {
		return "workspaces/" + o.WorkspaceID.String() + "/images/" + id.String()
	}
	return "users/" + o.ID.String() + "/images/" + id.String()
}

// Keys returns the stored objects of an image.
func Keys(prefix string) []string { return []string{prefix + "/256.png", prefix + "/64.png"} }

// Put stores both sizes and returns the new image id and key prefix. Insert the row
// with Insert inside the write's transaction; on failure call Discard.
func Put(ctx context.Context, st storage.Storage, o Owner, p Processed) (uuid.UUID, string, error) {
	id := uuid.New() // random v4: image URLs are capabilities and must not be guessable
	prefix := keyPrefix(o, id)
	if err := st.Put(ctx, prefix+"/256.png", bytes.NewReader(p.Large), int64(len(p.Large)), contentTypeP); err != nil {
		return uuid.Nil, "", fmt.Errorf("store image: %w", err)
	}
	if err := st.Put(ctx, prefix+"/64.png", bytes.NewReader(p.Small), int64(len(p.Small)), contentTypeP); err != nil {
		Discard(st, prefix)
		return uuid.Nil, "", fmt.Errorf("store image: %w", err)
	}
	return id, prefix, nil
}

// Discard removes stored objects (best effort).
func Discard(st storage.Storage, prefix string) {
	for _, k := range Keys(prefix) {
		_ = st.Delete(context.Background(), k)
	}
}

// Insert records an image row.
func Insert(ctx context.Context, tx pgx.Tx, o Owner, id uuid.UUID, prefix, source, sha string) error {
	slot := o.Slot
	if slot == "" {
		slot = SlotAvatar
	}
	_, err := tx.Exec(ctx, `INSERT INTO images (id, workspace_id, owner_type, owner_id, slot, source, storage_key, sha256)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, id, o.WorkspaceID, o.Type, o.ID, slot, source, prefix, sha)
	return err
}

// Forget deletes the row of a replaced image in tx and returns its key prefix, so the
// objects can be removed after commit. "" when the value was not an image.
func Forget(ctx context.Context, tx pgx.Tx, old domain.Avatar) (string, error) {
	id, ok := old.ImageID()
	if !ok {
		return "", nil
	}
	var prefix string
	err := tx.QueryRow(ctx, `DELETE FROM images WHERE id = $1 RETURNING storage_key`, id).Scan(&prefix)
	if db.IsNotFound(err) {
		return "", nil
	}
	return prefix, err
}

// Lookup returns an image's key prefix.
func Lookup(ctx context.Context, q db.Querier, id uuid.UUID) (string, error) {
	var prefix string
	err := q.QueryRow(ctx, `SELECT storage_key FROM images WHERE id = $1`, id).Scan(&prefix)
	return prefix, err
}

// ErrOwnerGone means the record was deleted while its picture was processed.
var ErrOwnerGone = errors.New("images: owner not found")
