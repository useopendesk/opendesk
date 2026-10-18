// Package adminui serves the built instance admin panel (admin/, built with base /admin/)
// from the binary at /admin/ (contracts/admin.md).
package adminui

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Base is where the binary serves the panel.
const Base = "/admin/"

// RuntimeConfig is exposed to the panel as window.__OPENDESK_ADMIN__ via config.js.
type RuntimeConfig struct {
	APIURL           string `json:"apiUrl"`
	AppURL           string `json:"appUrl"`
	TurnstileSiteKey string `json:"turnstileSiteKey,omitempty"`
	EnvLabel         string `json:"envLabel,omitempty"`
}

// Handler serves /admin/: assets, config.js, and index.html for the panel's routes.
func Handler(rc RuntimeConfig, csp string) http.Handler {
	sub, _ := fs.Sub(dist, "dist")
	files := http.StripPrefix(strings.TrimSuffix(Base, "/"), http.FileServer(http.FS(sub)))
	cfg, _ := json.Marshal(rc)
	configJS := []byte("window.__OPENDESK_ADMIN__=" + string(cfg) + ";")
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		index = []byte(`<!doctype html><meta charset="utf-8"><title>opendesk admin</title><p>The admin panel is not built into this binary. Run <code>make admin-embed</code>.</p>`)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Robots-Tag", "noindex, nofollow")
		if r.URL.Path == strings.TrimSuffix(Base, "/") {
			http.Redirect(w, r, Base, http.StatusMovedPermanently)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), strings.TrimSuffix(Base, "/"))
		p = strings.TrimPrefix(p, "/")
		if p == "config.js" {
			h.Set("Content-Type", "text/javascript; charset=utf-8")
			h.Set("Cache-Control", "no-store")
			_, _ = w.Write(configJS)
			return
		}
		if p != "" && p != "index.html" {
			if st, err := fs.Stat(sub, p); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					h.Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
			// A missing bundle is a 404, never the panel's page under a script's name.
			if strings.HasPrefix(p, "assets/") {
				h.Set("Cache-Control", "no-store")
				http.NotFound(w, r)
				return
			}
		}
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}
