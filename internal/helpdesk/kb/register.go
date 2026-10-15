// Package kb is the knowledge base: collections and articles for agents, and the public
// help center of each brand (published articles of that brand or of every brand, when the
// brand's help center is on).
package kb

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/useopendesk/opendesk/internal/deps"
)

// Register adds this package's routes.
func Register(d *deps.Deps, api huma.API, r chi.Router) {
	registerCollections(d, api)
	registerArticles(d, api)
	registerPublic(d, api)
}
