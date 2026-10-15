package kb

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/httpx"
)

const (
	featuredLimit     = 6
	publicSearchLimit = 20
	publicPerMinute   = 300
	searchPerMinute   = 60
)

type helpCenterHome struct {
	Title       string         `json:"title"`
	Collections []KbCollection `json:"collections"`
	Featured    []KbArticle    `json:"featured"`
	Locale      string         `json:"locale" doc:"The brand's language, else the workspace's (BCP 47); empty when not set"`
	LogoURL     domain.Avatar  `json:"logo_url" doc:"The brand's help center logo, else its logo, else the workspace logo"`
	FaviconURL  domain.Avatar  `json:"favicon_url"`
}

type helpCenterCollection struct {
	Collection KbCollection `json:"collection"`
	Articles   []KbArticle  `json:"articles"`
}

// rateLimit throttles anonymous help center reads per client address; search has a
// tighter budget of its own.
func rateLimit(d *deps.Deps, key string, perMinute int) func(huma.Context, func(huma.Context)) {
	clientIP := httpx.ClientIP(d.Cfg.TrustProxy)
	return func(ctx huma.Context, next func(huma.Context)) {
		r, _ := humachi.Unwrap(ctx)
		ip := clientIP(r)
		if !d.Limiter.Allow(key+":"+ip, perMinute, time.Minute) {
			body, _ := json.Marshal(httpx.RateLimited())
			ctx.SetHeader("Content-Type", "application/problem+json")
			ctx.SetHeader("Retry-After", "60")
			ctx.SetStatus(http.StatusTooManyRequests)
			_, _ = ctx.BodyWriter().Write(body)
			return
		}
		next(ctx)
	}
}

// helpCenterBrand is the brand a public help center path names.
type helpCenterBrand struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	Title       string
	Locale      string
	Logo        domain.Avatar
	Favicon     domain.Avatar
}

// helpCenter resolves a brand slug whose help center is enabled; anything else is 404. The
// default brand's slug is the workspace slug, so workspace help center links keep working.
func helpCenter(ctx context.Context, d *deps.Deps, slug string) (helpCenterBrand, error) {
	var b helpCenterBrand
	var name, title, wsLocale string
	var enabled bool
	var imgs map[string]string
	var wsLogo domain.Avatar
	err := d.DB.Pool.QueryRow(ctx, `SELECT b.id, b.workspace_id, b.name, b.help_center_title, b.help_center_enabled, b.locale, b.images,
		COALESCE(w.settings ->> 'locale', ''), w.avatar_url
		FROM brands b JOIN workspaces w ON w.id = b.workspace_id WHERE b.slug = $1`, strings.ToLower(strings.TrimSpace(slug))).
		Scan(&b.ID, &b.WorkspaceID, &name, &title, &enabled, &b.Locale, &imgs, &wsLocale, &wsLogo)
	if db.IsNotFound(err) || (err == nil && !enabled) {
		return b, httpx.NotFound("Help center")
	}
	if err != nil {
		return b, httpx.Wrap(err)
	}
	b.Title = strings.TrimSpace(title)
	if b.Title == "" {
		b.Title = name
	}
	if b.Locale == "" {
		b.Locale = wsLocale
	}
	b.Logo, b.Favicon = domain.Avatar(imgs["help_center_logo"]), domain.Avatar(imgs["help_center_favicon"])
	if b.Logo == "" {
		b.Logo = domain.Avatar(imgs["logo"])
	}
	if b.Logo == "" {
		b.Logo = wsLogo
	}
	return b, nil
}

func registerPublic(d *deps.Deps, api huma.API) {
	limited := huma.Middlewares{rateLimit(d, "kb-public", publicPerMinute)}
	tags := []string{"Help center"}
	public1 := []map[string][]string{}

	huma.Register(api, huma.Operation{OperationID: "public-help-home", Method: http.MethodGet, Path: "/v1/public/help/{workspace_slug}",
		Summary: "Help center home", Tags: tags, Security: public1, Middlewares: limited},
		func(ctx context.Context, in *struct {
			Slug string `path:"workspace_slug"`
		}) (*struct{ Body helpCenterHome }, error) {
			hc, err := helpCenter(ctx, d, in.Slug)
			if err != nil {
				return nil, err
			}
			rows, err := d.DB.Pool.Query(ctx, publicCollectionSelect+` WHERE k.workspace_id = $2 AND COALESCE(k.brand_id, $1) = $1
				AND EXISTS (SELECT 1 FROM kb_articles a WHERE a.collection_id = k.id AND a.status = 'published' AND COALESCE(k.brand_id, a.brand_id, $1) = $1)
				ORDER BY k.position, k.name, k.id`, hc.ID, hc.WorkspaceID)
			cols, err := collectRows(rows, err, scanCollection)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			rows, err = d.DB.Pool.Query(ctx, articleSelect+` WHERE a.workspace_id = $1 AND a.status = 'published' AND `+forBrand(3)+`
				ORDER BY a.views DESC, a.published_at DESC NULLS LAST, a.id LIMIT $2`, hc.WorkspaceID, featuredLimit, hc.ID)
			featured, err := collectRows(rows, err, scanArticle)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			home := helpCenterHome{Title: hc.Title, Collections: cols, Featured: publicList(featured), Locale: hc.Locale,
				LogoURL: hc.Logo, FaviconURL: hc.Favicon}
			return &struct{ Body helpCenterHome }{home}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "public-help-collection", Method: http.MethodGet,
		Path: "/v1/public/help/{workspace_slug}/collections/{slug}", Summary: "Help center collection", Tags: tags,
		Security: public1, Middlewares: limited},
		func(ctx context.Context, in *struct {
			WSSlug string `path:"workspace_slug"`
			Slug   string `path:"slug"`
		}) (*struct{ Body helpCenterCollection }, error) {
			hc, err := helpCenter(ctx, d, in.WSSlug)
			if err != nil {
				return nil, err
			}
			c, err := scanCollection(d.DB.Pool.QueryRow(ctx, publicCollectionSelect+` WHERE k.workspace_id = $2 AND k.slug = $3
				AND COALESCE(k.brand_id, $1) = $1`, hc.ID, hc.WorkspaceID, in.Slug))
			if db.IsNotFound(err) {
				return nil, httpx.NotFound("Collection")
			}
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			rows, err := d.DB.Pool.Query(ctx, articleSelect+` WHERE a.workspace_id = $1 AND a.collection_id = $2 AND a.status = 'published'
				AND `+forBrand(3)+` ORDER BY a.position, a.title, a.id`, hc.WorkspaceID, c.ID, hc.ID)
			arts, err := collectRows(rows, err, scanArticle)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body helpCenterCollection }{helpCenterCollection{Collection: c, Articles: publicList(arts)}}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "public-help-article", Method: http.MethodGet,
		Path: "/v1/public/help/{workspace_slug}/articles/{slug}", Summary: "Help center article", Description: "Counts a view.",
		Tags: tags, Security: public1, Middlewares: limited},
		func(ctx context.Context, in *struct {
			WSSlug string `path:"workspace_slug"`
			Slug   string `path:"slug"`
		}) (*articleOut, error) {
			hc, err := helpCenter(ctx, d, in.WSSlug)
			if err != nil {
				return nil, err
			}
			var id uuid.UUID
			err = d.DB.Pool.QueryRow(ctx, `UPDATE kb_articles a SET views = views + 1
				WHERE a.workspace_id = $1 AND a.slug = $2 AND a.status = 'published' AND `+forBrand(3)+` RETURNING a.id`,
				hc.WorkspaceID, in.Slug, hc.ID).Scan(&id)
			if db.IsNotFound(err) {
				return nil, httpx.NotFound("Article")
			}
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			a, err := loadArticle(ctx, d.DB.Pool, hc.WorkspaceID, id)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &articleOut{public(a)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "public-help-search", Method: http.MethodGet,
		Path: "/v1/public/help/{workspace_slug}/search", Summary: "Search the help center", Tags: tags, Security: public1,
		Middlewares: huma.Middlewares{rateLimit(d, "kb-search", searchPerMinute)}},
		func(ctx context.Context, in *struct {
			WSSlug string `path:"workspace_slug"`
			Q      string `query:"q" maxLength:"200"`
		}) (*struct{ Body []KbArticle }, error) {
			hc, err := helpCenter(ctx, d, in.WSSlug)
			if err != nil {
				return nil, err
			}
			q := strings.TrimSpace(in.Q)
			pq := prefixQuery(q)
			if pq == "" {
				return &struct{ Body []KbArticle }{[]KbArticle{}}, nil
			}
			tsq := searchQuery(2)
			rows, err := d.DB.Pool.Query(ctx, articleSelect+` WHERE a.workspace_id = $1 AND a.status = 'published' AND a.search @@ `+tsq+`
				AND `+forBrand(5)+` ORDER BY ts_rank(a.search, `+tsq+`) DESC, a.views DESC, a.id LIMIT $4`, hc.WorkspaceID, q, pq, publicSearchLimit, hc.ID)
			arts, err := collectRows(rows, err, scanArticle)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body []KbArticle }{publicList(arts)}, nil
		})
}
