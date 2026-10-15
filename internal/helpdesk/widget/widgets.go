package widget

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/ai/bots"
	"github.com/useopendesk/opendesk/internal/audit"
	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/config"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/events"
	"github.com/useopendesk/opendesk/internal/helpdesk/brands"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/ids"
	"github.com/useopendesk/opendesk/internal/images"
)

// Widget is Widget from contracts/teams.md: one chat widget, one website.
type Widget struct {
	ID       uuid.UUID         `json:"id"`
	BrandID  uuid.UUID         `json:"brand_id"`
	Name     string            `json:"name"`
	Key      string            `json:"key" doc:"Public widget key (wk_...)"`
	TeamID   *uuid.UUID        `json:"team_id" doc:"New conversations go to this team"`
	BotID    *uuid.UUID        `json:"bot_id" doc:"The bot the AI agent answers as here (contracts/bots.md); null = the default bot"`
	Settings hd.WidgetSettings `json:"settings"`
	Images   struct {
		Avatar   domain.Avatar `json:"avatar"`
		Launcher domain.Avatar `json:"launcher"`
	} `json:"images"`
	Install struct {
		Snippet string `json:"snippet"`
	} `json:"install"`
	SeenAt    *time.Time `json:"seen_at" doc:"First visitor session"`
	Position  int        `json:"position"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

const widgetSelect = `SELECT g.id, g.brand_id, g.name, g.key, g.team_id, g.bot_id, g.settings, g.images, g.seen_at, g.position, g.created_at, g.updated_at
FROM widgets g`

func scanWidget(row pgx.Row, cfg *config.Config) (Widget, error) {
	var g Widget
	var raw []byte
	var imgs map[string]string
	if err := row.Scan(&g.ID, &g.BrandID, &g.Name, &g.Key, &g.TeamID, &g.BotID, &raw, &imgs, &g.SeenAt, &g.Position, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return g, err
	}
	g.Settings = hd.ParseWidgetSettings(raw)
	g.Images.Avatar, g.Images.Launcher = domain.Avatar(imgs["avatar"]), domain.Avatar(imgs["launcher"])
	g.Install.Snippet = Snippet(cfg, g.Key)
	return g, nil
}

func (h *handler) loadWidget(ctx context.Context, q db.Querier, wid, id uuid.UUID) (Widget, error) {
	g, err := scanWidget(q.QueryRow(ctx, widgetSelect+` WHERE g.workspace_id = $1 AND g.id = $2`, wid, id), h.d.Cfg)
	if db.IsNotFound(err) {
		return g, httpx.NotFound("Widget")
	}
	return g, err
}

// Snippet is the install tag of a widget key. data-api is added when the API is not where
// the script infers it: api.opendesk.sh for the hosted app, else the app's own origin.
func Snippet(cfg *config.Config, key string) string {
	app := strings.TrimRight(cfg.AppURL, "/")
	s := `<script src="` + app + `/widget/v1.js" data-key="` + key + `"`
	inferred := originOf(app)
	if inferred == "https://app.opendesk.sh" {
		inferred = "https://api.opendesk.sh"
	}
	if api := originOf(cfg.APIURL); api != "" && api != inferred {
		s += ` data-api="` + api + `"`
	}
	return s + ` async></script>`
}

func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// cleanWidgetName is a widget's name for agents (for example its website): 1..60
// characters of plain text.
func cleanWidgetName(s string) (string, error) { return cleanLabel("name", s, 1, 60) }

type widgetCreate struct {
	BrandID  string               `json:"brand_id"`
	Name     string               `json:"name" maxLength:"200"`
	TeamID   *string              `json:"team_id,omitempty" required:"false" nullable:"true" doc:"Team id, or null"`
	BotID    *string              `json:"bot_id,omitempty" required:"false" nullable:"true" doc:"Bot id, or null for the default bot"`
	Settings *widgetSettingsPatch `json:"settings,omitempty"`
}

type widgetPatch struct {
	Name     *string              `json:"name,omitempty" maxLength:"200"`
	BrandID  *string              `json:"brand_id,omitempty" doc:"Move the widget to another brand"`
	TeamID   hd.Nullable[string]  `json:"team_id,omitempty" doc:"Team id, or null"`
	BotID    hd.Nullable[string]  `json:"bot_id,omitempty" doc:"Bot id, or null for the default bot (contracts/bots.md)"`
	Settings *widgetSettingsPatch `json:"settings,omitempty" doc:"Merged like PATCH /v1/widget/settings"`
	Position *int                 `json:"position,omitempty" minimum:"0" maximum:"10000"`
}

// WidgetSummary is what every member reads of a widget: its name and brand.
type WidgetSummary struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	BrandID uuid.UUID `json:"brand_id"`
}

type widgetCreated struct {
	Widget
	Secret string `json:"secret,omitempty" doc:"The identity secret for user_hash, shown once; absent when secret_available is false"`
	// SecretAvailable is false when the caller did not pass the owner-level gate (a second
	// factor when they have one, and a recent sign-in): rotate the secret to see one.
	SecretAvailable bool `json:"secret_available"`
}

func checkTeam(ctx context.Context, q db.Querier, wid uuid.UUID, raw string) (*uuid.UUID, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	id, err := hd.ParseRef(raw, "team_id")
	if err != nil {
		return nil, err
	}
	if err := hd.CheckRef(ctx, q, wid, "team", id); err != nil {
		return nil, err
	}
	return &id, nil
}

// checkBrandRoom refuses a 51st widget in a brand.
func checkBrandRoom(ctx context.Context, q db.Querier, wid, brandID uuid.UUID) error {
	var n int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM widgets WHERE workspace_id = $1 AND brand_id = $2`, wid, brandID).Scan(&n); err != nil {
		return err
	}
	if n >= brands.MaxWidgetsPerBrand {
		return httpx.E(http.StatusUnprocessableEntity, "limit_reached", "A brand can have at most 50 widgets.")
	}
	return nil
}

func (h *handler) publishWidget(ctx context.Context, tx pgx.Tx, wid, id uuid.UUID, typ string) (Widget, error) {
	g, err := h.loadWidget(ctx, tx, wid, id)
	if err != nil {
		return g, err
	}
	_, err = events.Publish(ctx, tx, events.Event{WorkspaceID: wid, Type: typ, Data: map[string]any{"widget": g}})
	return g, err
}

// update applies a widget patch in tx, audits it as action and publishes widget.updated.
func (h *handler) update(ctx context.Context, tx pgx.Tx, p *auth.Principal, id uuid.UUID, b widgetPatch, action string) (Widget, error) {
	wid := p.WorkspaceID
	var name string
	var brandID uuid.UUID
	var team, bot *uuid.UUID
	var raw []byte
	var position int
	primary, err := brands.PrimaryWidget(ctx, tx, wid)
	if err != nil {
		return Widget{}, err
	}
	err = tx.QueryRow(ctx, `SELECT name, brand_id, team_id, bot_id, settings, position FROM widgets WHERE workspace_id = $1 AND id = $2 FOR UPDATE`,
		wid, id).Scan(&name, &brandID, &team, &bot, &raw, &position)
	if db.IsNotFound(err) {
		return Widget{}, httpx.NotFound("Widget")
	}
	if err != nil {
		return Widget{}, err
	}
	settings := hd.ParseWidgetSettings(raw)
	var changed []string
	if b.Name != nil {
		n, err := cleanWidgetName(*b.Name)
		if err != nil {
			return Widget{}, err
		}
		if n != name {
			name = n
			changed = append(changed, "name")
		}
	}
	if b.BrandID != nil {
		bid, err := hd.ParseRef(*b.BrandID, "brand_id")
		if err != nil {
			return Widget{}, err
		}
		if bid != brandID {
			if err := brands.Check(ctx, tx, wid, bid); err != nil {
				return Widget{}, err
			}
			if err := checkBrandRoom(ctx, tx, wid, bid); err != nil {
				return Widget{}, err
			}
			brandID = bid
			changed = append(changed, "brand_id")
		}
	}
	if b.TeamID.Set {
		var t *uuid.UUID
		if !b.TeamID.Null {
			if t, err = checkTeam(ctx, tx, wid, b.TeamID.Value); err != nil {
				return Widget{}, err
			}
		}
		if (t == nil) != (team == nil) || (t != nil && *t != *team) {
			team = t
			changed = append(changed, "team_id")
		}
	}
	if b.BotID.Set {
		var nb *uuid.UUID
		if !b.BotID.Null {
			if nb, err = bots.ParseRef(ctx, tx, wid, &b.BotID.Value); err != nil {
				return Widget{}, err
			}
		}
		if (nb == nil) != (bot == nil) || (nb != nil && *nb != *bot) {
			bot = nb
			changed = append(changed, "bot_id")
		}
	}
	if b.Position != nil && *b.Position != position {
		position = *b.Position
		changed = append(changed, "position")
	}
	var fields []string
	if b.Settings != nil {
		if settings, fields, err = applyPatch(ctx, tx, wid, settings, *b.Settings); err != nil {
			return Widget{}, err
		}
	}
	if len(changed) == 0 && len(fields) == 0 {
		return h.loadWidget(ctx, tx, wid, id)
	}
	doc, err := json.Marshal(settings)
	if err != nil {
		return Widget{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE widgets SET name = $3, brand_id = $4, team_id = $5, settings = $6, position = $7, bot_id = $8, updated_at = now()
		WHERE workspace_id = $1 AND id = $2`, wid, id, name, brandID, team, doc, position, bot); err != nil {
		return Widget{}, err
	}
	if err := brands.MirrorPrimaryWidget(ctx, tx, wid, primary); err != nil {
		return Widget{}, err
	}
	data := map[string]any{"changes": changed}
	if slices.Contains(changed, "bot_id") {
		data["bot_id"] = bot
	}
	if len(fields) > 0 {
		data["fields"] = fields
	}
	if err := audit.Log(ctx, tx, p, wid, action, "widget", &id, data); err != nil {
		return Widget{}, err
	}
	return h.publishWidget(ctx, tx, wid, id, "widget.updated")
}

func wop(id, method, path, summary string, status int) huma.Operation {
	return huma.Operation{OperationID: id, Method: method, Path: path, Summary: summary, Tags: []string{"Widget"}, DefaultStatus: status}
}

func (h *handler) registerWidgets(api huma.API) {
	images.SetHooks(images.Hooks{Widget: func(ctx context.Context, tx pgx.Tx, wid, id uuid.UUID) error {
		_, err := h.publishWidget(ctx, tx, wid, id, "widget.updated")
		return err
	}})

	// The inbox filters by widget: every member reads the names, nothing else.
	huma.Register(api, wop("list-widget-summaries", http.MethodGet, "/v1/widgets/summary", "List chat widget names", 0),
		func(ctx context.Context, _ *struct{}) (*struct{ Body httpx.Page[WidgetSummary] }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleViewer, auth.ScopeTicketsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			rows, err := h.d.DB.Pool.Query(ctx, `SELECT g.id, g.name, g.brand_id FROM widgets g JOIN brands b ON b.id = g.brand_id
				WHERE g.workspace_id = $1 ORDER BY b.is_default DESC, b.position, b.name, g.position, g.created_at, g.id`, p.WorkspaceID)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[WidgetSummary])
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if list == nil {
				list = []WidgetSummary{}
			}
			return &struct{ Body httpx.Page[WidgetSummary] }{httpx.Page[WidgetSummary]{Data: list}}, nil
		})

	huma.Register(api, wop("list-widgets", http.MethodGet, "/v1/widgets", "List chat widgets", 0),
		func(ctx context.Context, in *struct {
			BrandID string `query:"brand_id" doc:"Only this brand's widgets"`
		}) (*struct{ Body httpx.Page[Widget] }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			var brand *uuid.UUID
			if in.BrandID != "" {
				id, err := hd.ParseRef(in.BrandID, "brand_id")
				if err != nil {
					return nil, err
				}
				brand = &id
			}
			rows, err := h.d.DB.Pool.Query(ctx, widgetSelect+` JOIN brands b ON b.id = g.brand_id
				WHERE g.workspace_id = $1 AND ($2::uuid IS NULL OR g.brand_id = $2)
				ORDER BY b.is_default DESC, b.position, b.name, g.position, g.created_at, g.id`, p.WorkspaceID, brand)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Widget, error) { return scanWidget(r, h.d.Cfg) })
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if list == nil {
				list = []Widget{}
			}
			return &struct{ Body httpx.Page[Widget] }{httpx.Page[Widget]{Data: list}}, nil
		})

	huma.Register(api, wop("get-widget", http.MethodGet, "/v1/widgets/{id}", "Get a chat widget", 0),
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct{ Body Widget }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsRead)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			id, err := hd.ParseID(in.ID, "Widget")
			if err != nil {
				return nil, err
			}
			g, err := h.loadWidget(ctx, h.d.DB.Pool, p.WorkspaceID, id)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body Widget }{g}, nil
		})

	huma.Register(api, wop("create-widget", http.MethodPost, "/v1/widgets", "Create a chat widget", http.StatusCreated),
		func(ctx context.Context, in *struct{ Body widgetCreate }) (*struct {
			CacheControl string `header:"Cache-Control"`
			Body         widgetCreated
		}, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			b := in.Body
			brandID, err := hd.ParseRef(b.BrandID, "brand_id")
			if err != nil {
				return nil, err
			}
			name, err := cleanWidgetName(b.Name)
			if err != nil {
				return nil, err
			}
			secret, sealed, err := brands.NewWidgetSecret(h.d.Box)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			out := &struct {
				CacheControl string `header:"Cache-Control"`
				Body         widgetCreated
			}{CacheControl: "no-store"}
			err = h.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				// Serializes widget writes of the brand, so the limit holds under concurrency.
				if err := tx.QueryRow(ctx, `SELECT id FROM brands WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, p.WorkspaceID, brandID).
					Scan(&brandID); err != nil {
					if db.IsNotFound(err) {
						return httpx.Validation("brand not found in this workspace")
					}
					return err
				}
				if err := checkBrandRoom(ctx, tx, p.WorkspaceID, brandID); err != nil {
					return err
				}
				var team *uuid.UUID
				if b.TeamID != nil {
					if team, err = checkTeam(ctx, tx, p.WorkspaceID, *b.TeamID); err != nil {
						return err
					}
				}
				settings := hd.DefaultWidgetSettings()
				if b.Settings != nil {
					if settings, _, err = applyPatch(ctx, tx, p.WorkspaceID, settings, *b.Settings); err != nil {
						return err
					}
				}
				doc, err := json.Marshal(settings)
				if err != nil {
					return err
				}
				primary, err := brands.PrimaryWidget(ctx, tx, p.WorkspaceID)
				if err != nil {
					return err
				}
				id := ids.New()
				bot, err := bots.ParseRef(ctx, tx, p.WorkspaceID, in.Body.BotID)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO widgets (id, workspace_id, brand_id, name, key, secret_enc, settings, team_id, position, bot_id)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, (SELECT COALESCE(max(position), 0) + 1 FROM widgets WHERE workspace_id = $2 AND brand_id = $3), $9)`,
					id, p.WorkspaceID, brandID, name, brands.NewWidgetKey(), sealed, doc, team, bot); err != nil {
					return err
				}
				if err := brands.MirrorPrimaryWidget(ctx, tx, p.WorkspaceID, primary); err != nil {
					return err
				}
				if err := audit.Log(ctx, tx, p, p.WorkspaceID, "widget.created", "widget", &id, map[string]any{"name": name, "brand_id": brandID}); err != nil {
					return err
				}
				out.Body.Widget, err = h.publishWidget(ctx, tx, p.WorkspaceID, id, "widget.created")
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			// The secret is shown only past the same gate as rotating it.
			ok, err := auth.PassesStepUp(ctx, h.d.DB.Pool, p)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if ok {
				out.Body.Secret, out.Body.SecretAvailable = secret, true
			}
			return out, nil
		})

	huma.Register(api, wop("update-widget", http.MethodPatch, "/v1/widgets/{id}", "Update a chat widget", 0),
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body widgetPatch
		}) (*struct{ Body Widget }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			id, err := hd.ParseID(in.ID, "Widget")
			if err != nil {
				return nil, err
			}
			var g Widget
			err = h.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				g, err = h.update(ctx, tx, p, id, in.Body, "widget.updated")
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body Widget }{g}, nil
		})

	huma.Register(api, wop("delete-widget", http.MethodDelete, "/v1/widgets/{id}", "Delete a chat widget",
		http.StatusNoContent),
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*hd.Empty, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			id, err := hd.ParseID(in.ID, "Widget")
			if err != nil {
				return nil, err
			}
			err = h.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				primary, err := brands.PrimaryWidget(ctx, tx, p.WorkspaceID)
				if err != nil {
					return err
				}
				var name string
				err = tx.QueryRow(ctx, `DELETE FROM widgets WHERE workspace_id = $1 AND id = $2 RETURNING name`, p.WorkspaceID, id).Scan(&name)
				if db.IsNotFound(err) {
					return httpx.NotFound("Widget")
				}
				if err != nil {
					return err
				}
				if err := brands.MirrorPrimaryWidget(ctx, tx, p.WorkspaceID, primary); err != nil {
					return err
				}
				if err := audit.Log(ctx, tx, p, p.WorkspaceID, "widget.deleted", "widget", &id, map[string]any{"name": name}); err != nil {
					return err
				}
				_, err = events.Publish(ctx, tx, events.Event{WorkspaceID: p.WorkspaceID, Type: "widget.deleted", Data: map[string]any{"widget_id": id}})
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return nil, nil
		})

	huma.Register(api, images.UploadOperation(api, huma.Operation{OperationID: "upload-widget-image", Method: http.MethodPut,
		Path: "/v1/widgets/{id}/images/{slot}", Summary: "Upload a chat widget image", Tags: []string{"Widget"},
		Description: "slot: avatar (the header picture) or launcher (the launcher icon). PNG, JPEG, GIF or WebP, at most 5 MB."}),
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Slot string `path:"slot" enum:"avatar,launcher"`
			images.UploadInput
		}) (*struct{ Body Widget }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			id, err := hd.ParseID(in.ID, "Widget")
			if err != nil {
				return nil, err
			}
			if !h.d.Limiter.Allow("widgetimage:"+p.WorkspaceID.String(), 60, time.Hour) {
				return nil, httpx.RateLimited()
			}
			img, err := images.ReadUpload(&in.UploadInput, images.Contain)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			o := images.Owner{Type: images.OwnerWidget, ID: id, WorkspaceID: &p.WorkspaceID, Slot: in.Slot}
			err = images.Set(ctx, h.d, o, img, "upload", images.SetOptions{After: func(ctx context.Context, tx pgx.Tx) error {
				return audit.Log(ctx, tx, p, p.WorkspaceID, "widget.image_updated", "widget", &id, map[string]any{"slot": in.Slot})
			}})
			return h.imageResult(ctx, p.WorkspaceID, id, err)
		})

	huma.Register(api, wop("delete-widget-image", http.MethodDelete, "/v1/widgets/{id}/images/{slot}", "Remove a chat widget image", 0),
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Slot string `path:"slot" enum:"avatar,launcher"`
		}) (*struct{ Body Widget }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleAdmin, auth.ScopeSettingsWrite)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			id, err := hd.ParseID(in.ID, "Widget")
			if err != nil {
				return nil, err
			}
			o := images.Owner{Type: images.OwnerWidget, ID: id, WorkspaceID: &p.WorkspaceID, Slot: in.Slot}
			err = images.Clear(ctx, h.d, o, false, images.SetOptions{After: func(ctx context.Context, tx pgx.Tx) error {
				return audit.Log(ctx, tx, p, p.WorkspaceID, "widget.image_removed", "widget", &id, map[string]any{"slot": in.Slot})
			}})
			return h.imageResult(ctx, p.WorkspaceID, id, err)
		})
}

func (h *handler) imageResult(ctx context.Context, wid, id uuid.UUID, err error) (*struct{ Body Widget }, error) {
	if errors.Is(err, images.ErrOwnerGone) {
		return nil, httpx.NotFound("Widget")
	}
	if err != nil {
		return nil, httpx.Wrap(err)
	}
	g, err := h.loadWidget(ctx, h.d.DB.Pool, wid, id)
	if err != nil {
		return nil, httpx.Wrap(err)
	}
	return &struct{ Body Widget }{g}, nil
}
