// Package widget is the chat widget API (visitor sessions, conversations, uploads), the
// admin-side widgets and their settings, and the attachment routes shared by agents and
// visitors. A workspace has any number of widgets, one per website, each in a brand
// (contracts/teams.md).
package widget

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/useopendesk/opendesk/internal/deps"
)

// Register adds this package's routes.
func Register(d *deps.Deps, api huma.API, r chi.Router) {
	h := &handler{d: d, api: api}
	h.registerAttachments(api, r)
	h.registerPublic(api)
	h.registerConversations(api)
	h.registerAgentRoutes(api)
	h.registerSettings(api)
	h.registerWidgets(api)
}

type handler struct {
	d   *deps.Deps
	api huma.API
}
