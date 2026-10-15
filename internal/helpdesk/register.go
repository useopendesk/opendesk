// Package helpdesk is the helpdesk area. Register wires its HTTP routes, RegisterWorkers its
// River workers, Periodic its scheduled jobs and Background its long-running loops.
//
// The work is split by resource: tickets (tickets, messages, triggers, SLA, automations
// engine, CSAT, search, reports), contacts (contacts, companies), resources (tags, saved
// replies, views, SLA policies, automations), kb (knowledge base and help center) and
// widget (chat widget API and attachments), brands, helpdomains (help center custom
// domains) and helpsite (the rendered help centers). Shared helpers live in hd.
package helpdesk

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/riverqueue/river"

	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/helpdesk/brands"
	"github.com/useopendesk/opendesk/internal/helpdesk/contacts"
	"github.com/useopendesk/opendesk/internal/helpdesk/helpdomains"
	"github.com/useopendesk/opendesk/internal/helpdesk/helpsite"
	"github.com/useopendesk/opendesk/internal/helpdesk/kb"
	"github.com/useopendesk/opendesk/internal/helpdesk/resources"
	"github.com/useopendesk/opendesk/internal/helpdesk/routing"
	"github.com/useopendesk/opendesk/internal/helpdesk/tickets"
	"github.com/useopendesk/opendesk/internal/helpdesk/widget"
	"github.com/useopendesk/opendesk/internal/images"
)

// Register adds this area's routes. Prefer huma operations; use r only for raw HTTP
// (redirects, raw bodies, streaming).
func Register(d *deps.Deps, api huma.API, r chi.Router) {
	tickets.Register(d, api, r)
	contacts.Register(d, api, r)
	resources.Register(d, api, r)
	kb.Register(d, api, r)
	widget.Register(d, api, r)
	brands.Register(d, api, r)
	helpdomains.Register(d, api)
	helpsite.Register(d, r)
	routing.Register(d, api)
	images.Register(d, r)
}

// RegisterWorkers adds this area's River workers.
func RegisterWorkers(d *deps.Deps, w *river.Workers) {
	tickets.RegisterWorkers(d, w)
	images.RegisterWorkers(d, w)
	helpdomains.RegisterWorkers(d, w)
	contacts.InstallImageHooks()
}

// Periodic returns this area's scheduled jobs.
func Periodic(d *deps.Deps) []*river.PeriodicJob {
	return append(append(tickets.Periodic(d), images.Periodic()...), helpdomains.Periodic(d)...)
}

// Background runs until ctx is done, in the worker role only. The helpdesk has no
// long-running loops; its scheduled work runs as River periodic jobs.
func Background(ctx context.Context, d *deps.Deps) {}
