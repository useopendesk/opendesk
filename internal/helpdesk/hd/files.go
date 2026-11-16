package hd

import (
	"html"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/useopendesk/opendesk/internal/domain"
)

// Files a body names: wiki pages (contracts/wiki.md "Files"), knowledge base articles and
// changelog entries (contracts/api.md "Knowledge base") keep one of our files as its plain
// path, /v1/attachments/<id>, and hand out a link to it as they are read.

const fileIDPattern = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`

var fileRef = regexp.MustCompile(`/v1/attachments/(` + fileIDPattern + `)`)

// fileAttr matches an attribute naming one of our files: its path, or a link to it at the
// API's address, signed or not. Group 1 is the attribute up to its value, 2 the path, 3 the id.
func fileAttr() *regexp.Regexp {
	host := ""
	if base := domain.APIURL(); base != "" {
		host = `(?:` + regexp.QuoteMeta(base) + `)?`
	}
	return regexp.MustCompile(`((?:src|data-src|href)=")` + host + `(/v1/attachments/(` + fileIDPattern + `))(?:\?[^"]*)?"`)
}

// FileIDs are the attachment ids a body names, each once.
func FileIDs(body string) []uuid.UUID {
	var out []uuid.UUID
	for _, m := range fileRef.FindAllStringSubmatch(body, -1) {
		if id, err := uuid.Parse(m[1]); err == nil && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// PlainFiles keeps every link to one of our files as its plain path.
func PlainFiles(body string) string {
	if !strings.Contains(body, "/v1/attachments/") {
		return body
	}
	return fileAttr().ReplaceAllString(body, `$1$2"`)
}

// LinkFiles points every link to one of our files at link(id), escaped for the attribute;
// a file for which link returns "" keeps its plain path.
func LinkFiles(body string, link func(id uuid.UUID) string) string {
	if !strings.Contains(body, "/v1/attachments/") {
		return body
	}
	re := fileAttr()
	return re.ReplaceAllStringFunc(body, func(m string) string {
		g := re.FindStringSubmatch(m)
		to := ""
		if id, err := uuid.Parse(g[3]); err == nil {
			to = link(id)
		}
		if to == "" {
			return g[1] + g[2] + `"`
		}
		return g[1] + html.EscapeString(to) + `"`
	})
}
