package kb

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
)

// Files (contracts/api.md "Knowledge base"): a body keeps one of our files as its plain path,
// /v1/attachments/<id>. As an article or changelog entry is saved, each upload its body names
// that the person or key saving it uploaded, and that nothing else holds (a message, a wiki
// page, another article or entry, an unsent reply draft), becomes its own; a file the body no
// longer names is let go, and the sweep of unattached uploads removes it later. Members read
// its files with signed links; a published one's help center serves its images at
// /_help/files/<id>.

// holder is the attachments column naming what holds a file.
type holder string

const (
	articleFiles holder = "kb_article_id"
	entryFiles   holder = "changelog_entry_id"
)

// holdFiles makes the files body names owner's own, as described above, and lets go of
// those it no longer names. Run in the transaction that saves the body.
func holdFiles(ctx context.Context, tx pgx.Tx, p *auth.Principal, col holder, owner uuid.UUID, body string) error {
	files := hd.FileIDs(body)
	if files == nil {
		files = []uuid.UUID{}
	}
	if _, err := tx.Exec(ctx, `UPDATE attachments SET `+string(col)+` = NULL
		WHERE workspace_id = $1 AND `+string(col)+` = $2 AND NOT (id = ANY($3))`, p.WorkspaceID, owner, files); err != nil {
		return err
	}
	user, key := p.ActorUserID(), p.ActorAPIKeyID()
	if len(files) == 0 || (user == nil && key == nil) {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE attachments a SET `+string(col)+` = $3
		WHERE a.workspace_id = $1 AND a.id = ANY($2) AND a.message_id IS NULL AND a.wiki_page_id IS NULL
		AND a.kb_article_id IS NULL AND a.changelog_entry_id IS NULL
		AND (a.uploaded_by = $4 OR a.uploaded_by_api_key_id = $5)
		AND NOT EXISTS (SELECT 1 FROM ticket_reply_drafts rd WHERE rd.workspace_id = a.workspace_id AND a.id = ANY(rd.attachment_ids))`,
		p.WorkspaceID, files, owner, user, key)
	return err
}

// signFiles gives a member's read signed links to the files of the bodies held by an article
// (or entry, per col) of the workspace, so images load without a token. Any other file keeps
// its plain path.
func signFiles(ctx context.Context, q db.Querier, wid uuid.UUID, col holder, bodies ...*string) error {
	var files []uuid.UUID
	for _, b := range bodies {
		files = append(files, hd.FileIDs(*b)...)
	}
	if len(files) == 0 {
		return nil
	}
	rows, err := q.Query(ctx, `SELECT id FROM attachments WHERE workspace_id = $1 AND id = ANY($2)
		AND message_id IS NULL AND wiki_page_id IS NULL AND `+string(col)+` IS NOT NULL`, wid, files)
	if err != nil {
		return err
	}
	held, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	ok := make(map[uuid.UUID]bool, len(held))
	for _, id := range held {
		ok[id] = true
	}
	for _, b := range bodies {
		*b = hd.LinkFiles(*b, func(id uuid.UUID) string {
			if !ok[id] {
				return ""
			}
			return domain.AttachmentURL(id)
		})
	}
	return nil
}

func signArticles(ctx context.Context, q db.Querier, wid uuid.UUID, arts []KbArticle) error {
	bodies := make([]*string, len(arts))
	for i := range arts {
		bodies[i] = &arts[i].BodyHTML
	}
	return signFiles(ctx, q, wid, articleFiles, bodies...)
}

func signEntries(ctx context.Context, q db.Querier, wid uuid.UUID, entries []ChangelogEntry) error {
	bodies := make([]*string, len(entries))
	for i := range entries {
		bodies[i] = &entries[i].BodyHTML
	}
	return signFiles(ctx, q, wid, entryFiles, bodies...)
}

// PublicBody points every link to one of our files in a public body at the help center whose
// address is base: base/_help/files/<id>, which serves the images of the help center's
// published articles and changelog entries for good.
func PublicBody(body, base string) string {
	return hd.LinkFiles(body, func(id uuid.UUID) string { return base + "/_help/files/" + id.String() })
}

// HelpFile is a file a help center serves.
type HelpFile struct {
	Filename    string
	ContentType string
	Size        int64
	Key         string
}

// PublicFile returns an image the help center shows: a raster image held by one of its
// published articles or by a changelog entry it shows. Anything else is a not-found error.
func PublicFile(ctx context.Context, q db.Querier, hc HelpCenter, id uuid.UUID) (HelpFile, error) {
	var f HelpFile
	err := q.QueryRow(ctx, `SELECT f.filename, f.content_type, f.size_bytes, f.storage_key FROM attachments f
		WHERE f.workspace_id = $1 AND f.id = $2 AND f.message_id IS NULL AND f.wiki_page_id IS NULL AND (
			EXISTS (SELECT 1 FROM kb_articles a WHERE a.workspace_id = $1 AND a.id = f.kb_article_id AND a.status = 'published' AND `+forBrand(3)+`)
			OR EXISTS (SELECT 1 FROM changelog_entries e WHERE e.id = f.changelog_entry_id AND `+changelogVisible(3)+`))`,
		hc.WorkspaceID, id, hc.BrandID).Scan(&f.Filename, &f.ContentType, &f.Size, &f.Key)
	if err == nil && !hd.InlineSafe(f.ContentType) {
		return HelpFile{}, pgx.ErrNoRows
	}
	return f, err
}
