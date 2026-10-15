package domain

import (
	"context"
	"fmt"
	"hash/fnv"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/events"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/ids"
)

// Labels are the API's tags (contracts/api.md "Labels and custom fields").

// LabelColors is the fixed palette labels, groups and select options use.
var LabelColors = []string{"gray", "red", "orange", "amber", "yellow", "lime", "green", "teal", "cyan", "blue",
	"indigo", "violet", "purple", "pink", "rose"}

// Label scopes: the kinds of records a label applies to.
const (
	ScopeTickets   = "tickets"
	ScopeContacts  = "contacts"
	ScopeCompanies = "companies"
)

var LabelScopes = []string{ScopeTickets, ScopeContacts, ScopeCompanies}

// MaxLabels bounds the labels of one workspace.
const MaxLabels = 1000

// MaxLabelsPerRecord bounds the labels on one ticket, contact or company.
const MaxLabelsPerRecord = 50

// TagRef is a label as embedded in tickets, contacts and companies.
type TagRef struct {
	ID      uuid.UUID  `json:"id"`
	Name    string     `json:"name"`
	Color   string     `json:"color" enum:"gray,red,orange,amber,yellow,lime,green,teal,cyan,blue,indigo,violet,purple,pink,rose"`
	GroupID *uuid.UUID `json:"group_id"`
}

// TagRefsSQL aggregates a record's labels as TagRef JSON; join is the link table
// condition on alias lt (e.g. "lt.ticket_id = t.id"), table the link table.
func TagRefsSQL(table, join string) string {
	return `COALESCE((SELECT json_agg(json_build_object('id', g.id, 'name', g.name, 'color', g.color, 'group_id', g.group_id)
		ORDER BY tg.position NULLS LAST, g.position, lower(g.name))
		FROM ` + table + ` lt JOIN tags g ON g.id = lt.tag_id LEFT JOIN tag_groups tg ON tg.id = g.group_id WHERE ` + join + `), '[]')`
}

// DefaultLabelColor picks a palette colour from a name, so new labels are told apart.
func DefaultLabelColor(name string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(name)))
	return LabelColors[h.Sum32()%uint32(len(LabelColors))]
}

var tagChars = regexp.MustCompile(`^[\p{L}\p{N} _\-./:&+#]+$`)

// CleanTagName normalizes a label name: 1..50 characters, no markup.
func CleanTagName(s string) (string, error) {
	s = strings.Join(strings.Fields(norm.NFC.String(s)), " ")
	if s == "" {
		return "", httpx.Validation("label name is required")
	}
	if len([]rune(s)) > 50 {
		return "", httpx.Validation("label names must be 50 characters or less")
	}
	if !tagChars.MatchString(s) {
		return "", httpx.Validation("label names may contain letters, numbers, spaces and - _ . / : & + #")
	}
	return s, nil
}

// LabelKind names the record kinds that carry labels, with their link tables.
type LabelKind struct {
	Scope string // tickets | contacts | companies
	Table string // link table
	Col   string // link column
}

var (
	TicketLabels  = LabelKind{ScopeTickets, "ticket_tags", "ticket_id"}
	ContactLabels = LabelKind{ScopeContacts, "contact_tags", "contact_id"}
	CompanyLabels = LabelKind{ScopeCompanies, "company_tags", "company_id"}
)

// TagOps is a change to one record's labels, by name.
type TagOps struct {
	Replace *[]string // replaces the set
	Add     []string
	Remove  []string
	// Create makes missing labels; without it an unknown name is 422 unknown_label.
	Create bool
	// Lenient skips unknown and not applicable labels instead of refusing (automations, AI).
	Lenient bool
}

func (o TagOps) Empty() bool { return o.Replace == nil && len(o.Add) == 0 && len(o.Remove) == 0 }

type labelRow struct {
	id        uuid.UUID
	name      string
	group     *uuid.UUID
	exclusive bool
	appliesTo []string
}

const labelRowSelect = `SELECT g.id, g.name, g.group_id, COALESCE(tg.exclusive, false), g.applies_to
	FROM tags g LEFT JOIN tag_groups tg ON tg.id = g.group_id AND tg.workspace_id = g.workspace_id`

func scanLabelRow(r pgx.Row) (labelRow, error) {
	var l labelRow
	err := r.Scan(&l.id, &l.name, &l.group, &l.exclusive, &l.appliesTo)
	return l, err
}

// lookupLabel finds a label by name (ignoring case) in the workspace.
func lookupLabel(ctx context.Context, q db.Querier, wid uuid.UUID, name string) (labelRow, bool, error) {
	l, err := scanLabelRow(q.QueryRow(ctx, labelRowSelect+` WHERE g.workspace_id = $1 AND lower(g.name) = lower($2)
		ORDER BY g.name = $2 DESC LIMIT 1`, wid, name))
	if db.IsNotFound(err) {
		return l, false, nil
	}
	return l, err == nil, err
}

// CreateLabel inserts a label with defaults (palette colour, last position) and publishes
// tag.created. The caller has checked the name is free.
func CreateLabel(ctx context.Context, tx pgx.Tx, wid uuid.UUID, name string) (uuid.UUID, error) {
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tags WHERE workspace_id = $1`, wid).Scan(&n); err != nil {
		return uuid.Nil, err
	}
	if n >= MaxLabels {
		return uuid.Nil, httpx.Validation(fmt.Sprintf("a workspace can have at most %d labels", MaxLabels))
	}
	id := ids.New()
	var created bool
	err := tx.QueryRow(ctx, `INSERT INTO tags (id, workspace_id, name, color, position)
		VALUES ($1, $2, $3, $4, (SELECT COALESCE(max(position) + 1, 0) FROM tags WHERE workspace_id = $2))
		ON CONFLICT (workspace_id, name) DO UPDATE SET name = EXCLUDED.name RETURNING id, (xmax = 0)`,
		id, wid, name, DefaultLabelColor(name)).Scan(&id, &created)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create label: %w", err)
	}
	if created {
		t, err := LoadTag(ctx, tx, wid, id)
		if err != nil {
			return uuid.Nil, err
		}
		if _, err := events.Publish(ctx, tx, events.Event{WorkspaceID: wid, Type: "tag.created", Data: map[string]any{"tag": t}}); err != nil {
			return uuid.Nil, err
		}
	}
	return id, nil
}

// resolveLabels maps names to labels, creating missing ones when allowed. With lenient,
// unknown names (and not applicable labels) are skipped.
func resolveLabels(ctx context.Context, tx pgx.Tx, wid uuid.UUID, kind LabelKind, names []string, create, lenient bool) ([]labelRow, error) {
	out := []labelRow{}
	seen := map[uuid.UUID]bool{}
	for _, raw := range names {
		name, err := CleanTagName(raw)
		if err != nil {
			if lenient {
				continue
			}
			return nil, err
		}
		l, ok, err := lookupLabel(ctx, tx, wid, name)
		if err != nil {
			return nil, err
		}
		if !ok {
			if !create {
				if lenient {
					continue
				}
				e := httpx.E(422, "unknown_label", fmt.Sprintf("There is no label named %q; ask an admin to create it.", name))
				return nil, e
			}
			if _, err := CreateLabel(ctx, tx, wid, name); err != nil {
				return nil, err
			}
			if l, _, err = lookupLabel(ctx, tx, wid, name); err != nil {
				return nil, err
			}
		}
		if !slices.Contains(l.appliesTo, kind.Scope) {
			if lenient {
				continue
			}
			return nil, httpx.Validation(fmt.Sprintf("the label %q does not apply to %s", l.name, kind.Scope))
		}
		if seen[l.id] {
			// Keep the latest mention: it decides exclusive groups.
			out = slices.DeleteFunc(out, func(x labelRow) bool { return x.id == l.id })
		}
		seen[l.id] = true
		out = append(out, l)
	}
	return out, nil
}

// ApplyTags changes a record's labels and returns the names added and removed (sorted).
// A record holds at most one label of an exclusive group: the one named last wins, and
// labels applied now win over the ones it already had. The record (id) must already be
// checked to belong to the workspace; every label read here is scoped to it.
func ApplyTags(ctx context.Context, tx pgx.Tx, wid uuid.UUID, kind LabelKind, id uuid.UUID, ops TagOps) (added, removed []string, err error) {
	if ops.Empty() {
		return nil, nil, nil
	}
	rows, err := tx.Query(ctx, labelRowSelect+` JOIN `+kind.Table+` lt ON lt.tag_id = g.id
		WHERE lt.`+kind.Col+` = $1 AND g.workspace_id = $2 ORDER BY g.position, g.name`, id, wid)
	if err != nil {
		return nil, nil, err
	}
	current, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (labelRow, error) { return scanLabelRow(r) })
	if err != nil {
		return nil, nil, err
	}

	// seq is the wanted set in precedence order: earlier entries lose exclusive groups.
	var seq []labelRow
	if ops.Replace != nil {
		if len(*ops.Replace) > MaxLabelsPerRecord {
			return nil, nil, httpx.Validation(fmt.Sprintf("a record can have at most %d labels", MaxLabelsPerRecord))
		}
		ls, err := resolveLabels(ctx, tx, wid, kind, *ops.Replace, ops.Create, ops.Lenient)
		if err != nil {
			return nil, nil, err
		}
		seq = ls
	} else {
		seq = slices.Clone(current)
	}
	if len(ops.Add) > 0 {
		ls, err := resolveLabels(ctx, tx, wid, kind, ops.Add, ops.Create, ops.Lenient)
		if err != nil {
			return nil, nil, err
		}
		for _, l := range ls {
			seq = slices.DeleteFunc(seq, func(x labelRow) bool { return x.id == l.id })
			seq = append(seq, l)
		}
	}
	for _, raw := range ops.Remove {
		name := strings.TrimSpace(raw)
		seq = slices.DeleteFunc(seq, func(x labelRow) bool { return strings.EqualFold(x.name, name) })
	}
	// Exclusive groups: walk from the end, keep the first label seen per group.
	keep := map[uuid.UUID]bool{}
	taken := map[uuid.UUID]bool{}
	for i := len(seq) - 1; i >= 0; i-- {
		l := seq[i]
		if l.exclusive && l.group != nil {
			if taken[*l.group] {
				continue
			}
			taken[*l.group] = true
		}
		keep[l.id] = true
	}
	if len(keep) > MaxLabelsPerRecord {
		return nil, nil, httpx.Validation(fmt.Sprintf("a record can have at most %d labels", MaxLabelsPerRecord))
	}
	have := map[uuid.UUID]bool{}
	for _, l := range current {
		have[l.id] = true
		if keep[l.id] {
			continue
		}
		if _, err := tx.Exec(ctx, `DELETE FROM `+kind.Table+` WHERE `+kind.Col+` = $1 AND tag_id = $2`, id, l.id); err != nil {
			return nil, nil, err
		}
		removed = append(removed, l.name)
	}
	for _, l := range seq {
		if !keep[l.id] || have[l.id] {
			continue
		}
		have[l.id] = true
		if _, err := tx.Exec(ctx, `INSERT INTO `+kind.Table+` (`+kind.Col+`, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, l.id); err != nil {
			return nil, nil, err
		}
		added = append(added, l.name)
	}
	slices.Sort(added)
	slices.Sort(removed)
	return added, removed, nil
}

// EnsureTags returns label ids for names, creating missing labels (and publishing tag.created).
func EnsureTags(ctx context.Context, tx pgx.Tx, wid uuid.UUID, names []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(names))
	seen := map[string]bool{}
	for _, raw := range names {
		name, err := CleanTagName(raw)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		l, ok, err := lookupLabel(ctx, tx, wid, name)
		if err != nil {
			return nil, err
		}
		id := l.id
		if !ok {
			if id, err = CreateLabel(ctx, tx, wid, name); err != nil {
				return nil, err
			}
		}
		out = append(out, id)
	}
	return out, nil
}

// LabelCreationOpen reports whether the workspace lets anyone create labels by applying
// a new name (settings.label_creation is not "admins").
func LabelCreationOpen(ctx context.Context, q db.Querier, wid uuid.UUID) (bool, error) {
	var v *string
	if err := q.QueryRow(ctx, `SELECT settings->>'label_creation' FROM workspaces WHERE id = $1`, wid).Scan(&v); err != nil {
		return false, err
	}
	return v == nil || *v != "admins", nil
}
