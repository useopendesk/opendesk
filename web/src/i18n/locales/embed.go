// Package locales embeds the customer-facing catalogs for the help center the API renders
// (internal/helpdesk/helpsite), so the web app and the rendered pages read one set of words.
package locales

import "embed"

// Public holds <locale>/public.json for every SUPPORTED locale.
//
//go:embed */public.json
var Public embed.FS
