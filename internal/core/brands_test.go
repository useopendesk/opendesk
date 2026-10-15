package core

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/crypto"
)

// TestWorkspaceDefaultBrand checks what a new workspace gets (its default brand and
// widget), the settings the default brand mirrors and the widget secret rotations.
func TestWorkspaceDefaultBrand(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner, _ := e.signup("brander")

	// A workspace slug never takes another brand's help center address.
	base := "brandco-" + strings.ToLower(crypto.RandomString(8))
	if _, err := e.d.DB.Pool.Exec(ctx, `INSERT INTO workspaces (name, slug, widget_key) VALUES ('Taken', $1, $2)`,
		"x-"+base, "wk_"+crypto.RandomString(20)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.DB.Pool.Exec(ctx, `INSERT INTO brands (workspace_id, name, slug, is_default)
		SELECT id, 'Taken', $2, true FROM workspaces WHERE slug = $1`, "x-"+base, base); err != nil {
		t.Fatal(err)
	}
	wid := owner.newWorkspace(base)
	ws := owner.do("GET", "/v1/workspace", nil)
	slug := ws.str("slug")
	if slug == base || !strings.HasPrefix(slug, base) {
		t.Fatalf("slug %q", slug)
	}

	var brandSlug, brandName, title, widgetName, widgetKey string
	var enabled, sealed bool
	if err := e.d.DB.Pool.QueryRow(ctx, `SELECT b.slug, b.name, b.help_center_title, b.help_center_enabled, g.name, g.key, g.secret_enc IS NOT NULL
		FROM brands b JOIN widgets g ON g.brand_id = b.id WHERE b.workspace_id = $1 AND b.is_default`, wid).
		Scan(&brandSlug, &brandName, &title, &enabled, &widgetName, &widgetKey, &sealed); err != nil {
		t.Fatal(err)
	}
	if brandSlug != slug || brandName != base || !enabled || title != base+" help center" || widgetName != "Website" ||
		widgetKey != ws.str("widget_key") || !sealed {
		t.Fatal(brandSlug, brandName, title, enabled, widgetName, widgetKey, ws.str("widget_key"), sealed)
	}

	// Workspace settings the default brand mirrors.
	if r := owner.do("PATCH", "/v1/workspace", map[string]any{"settings": map[string]any{"help_center_title": "Help", "brand_color": "#AABBCC",
		"locale": "fr", "help_center_enabled": false}}); r.Status != 200 {
		t.Fatalf("patch: %d %s", r.Status, r.Raw)
	}
	var color, locale string
	if err := e.d.DB.Pool.QueryRow(ctx, `SELECT help_center_title, help_center_enabled, color, locale FROM brands WHERE workspace_id = $1 AND is_default`, wid).
		Scan(&title, &enabled, &color, &locale); err != nil {
		t.Fatal(err)
	}
	if title != "Help" || enabled || color != "#aabbcc" || locale != "fr" {
		t.Fatal(title, enabled, color, locale)
	}

	// Widget pictures of the workspace live on the default widget.
	r := owner.upload("PUT", "/v1/workspace/images/widget_avatar", testPNG(t, 64, 64))
	if r.Status != 200 || r.Body["images"].(map[string]any)["widget_avatar"] == nil {
		t.Fatalf("widget avatar: %d %s", r.Status, r.Raw)
	}
	var avatar string
	if err := e.d.DB.Pool.QueryRow(ctx, `SELECT COALESCE(images->>'avatar', '') FROM widgets WHERE key = $1`, widgetKey).Scan(&avatar); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(avatar, "img:") {
		t.Fatalf("widget avatar %q", avatar)
	}

	// Rotation: the old route rotates the default widget, the new one any widget of the workspace.
	var widgetID uuid.UUID
	var before []byte
	if err := e.d.DB.Pool.QueryRow(ctx, `SELECT id, secret_enc FROM widgets WHERE key = $1`, widgetKey).Scan(&widgetID, &before); err != nil {
		t.Fatal(err)
	}
	r = owner.do("POST", "/v1/workspace/widget-secret/rotate", nil)
	if r.Status != 200 || !strings.HasPrefix(r.str("secret"), "wsec_") || r.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("rotate: %d %s", r.Status, r.Raw)
	}
	var after []byte
	if err := e.d.DB.Pool.QueryRow(ctx, `SELECT secret_enc FROM widgets WHERE id = $1`, widgetID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	plain, err := e.d.Box.Open(after)
	if err != nil || string(plain) != r.str("secret") || string(before) == string(after) {
		t.Fatal("rotated secret not stored")
	}
	r = owner.do("POST", "/v1/widgets/"+widgetID.String()+"/secret/rotate", nil)
	if r.Status != 200 || !strings.HasPrefix(r.str("secret"), "wsec_") {
		t.Fatalf("rotate by id: %d %s", r.Status, r.Raw)
	}
	if r := owner.do("POST", "/v1/widgets/"+uuid.NewString()+"/secret/rotate", nil); r.Status != 404 {
		t.Fatalf("unknown widget: %d", r.Status)
	}
	other, _ := e.signup("stranger")
	other.newWorkspace("Elsewhere")
	if r := other.do("POST", "/v1/widgets/"+widgetID.String()+"/secret/rotate", nil); r.Status != 404 {
		t.Fatalf("foreign widget: %d", r.Status)
	}
	// An old session is refused.
	if _, err := e.d.DB.Pool.Exec(ctx, `UPDATE sessions SET created_at = now() - interval '13 hours' WHERE user_id =
		(SELECT user_id FROM members WHERE workspace_id = $1 AND role = 'owner')`, wid); err != nil {
		t.Fatal(err)
	}
	if r := owner.do("POST", "/v1/widgets/"+widgetID.String()+"/secret/rotate", nil); r.Status != 403 || !strings.Contains(string(r.Raw), "reauth_required") {
		t.Fatalf("stale session: %d %s", r.Status, r.Raw)
	}
	var audits int
	if err := e.d.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE workspace_id = $1
		AND action IN ('workspace.widget_secret_rotated', 'widget.secret_rotated')`, wid).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Fatalf("audits %d", audits)
	}
}
