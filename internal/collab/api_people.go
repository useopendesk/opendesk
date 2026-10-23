package collab

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// A person the caller writes with (contracts/collab.md "People"): what the mail the caller
// sees holds about one address, beside the conversation that is open.

const (
	personThreads    = 8  // conversations a person card lists by default
	maxPersonThreads = 20 // and at most
	personFiles      = 6  // files from them it lists
)

// MailPerson is one address across the conversations the caller sees.
type MailPerson struct {
	Email string `json:"email"`
	// Name is the newest name the address has on those conversations ("" none).
	Name string `json:"name"`
	// Conversations counts them, trash and spam left out, the open one included.
	Conversations int `json:"conversations"`
	// Others counts them without the one asked to leave out.
	Others int `json:"others"`
	// FirstAt is their oldest message: the oldest in the mail opendesk has, not the first ever.
	FirstAt *time.Time `json:"first_at"`
	LastAt  *time.Time `json:"last_at"`
	// Threads are the newest of them, without the one asked to leave out.
	Threads []MailThread `json:"threads"`
	// Files are attachments of messages they sent, newest first.
	Files []MailPersonFile `json:"files"`
}

// MailPersonFile is a file from a person, with where it came.
type MailPersonFile struct {
	ThreadID   uuid.UUID      `json:"thread_id"`
	MessageID  uuid.UUID      `json:"message_id"`
	SentAt     time.Time      `json:"sent_at"`
	Attachment MailAttachment `json:"attachment"`
}

// personThreadsSQL is the predicate of the conversations with the address ($3) the caller
// ($2) sees in the workspace ($1), on collab_threads alias t.
func personThreadsSQL() string {
	return `t.workspace_id = $1 AND t.addresses @> ARRAY[$3::text] AND ` + threadVisibleSQL("t", "$2") +
		` AND NOT (t.folder_ids && ` + roleSQL(RoleTrash, RoleSpam) + `)`
}

func (s *service) registerPeopleRoutes(api huma.API) {
	collabOp(api, huma.Operation{OperationID: "get-collab-person", Method: http.MethodGet, Path: "/v1/collab/person",
		Summary: "A person across the conversations you see"},
		func(ctx context.Context, in *struct {
			Email   string `query:"email" required:"true" maxLength:"254"`
			Exclude string `query:"exclude" doc:"A conversation to leave out of threads (the open one)"`
			Limit   int    `query:"limit" minimum:"0" maximum:"20" doc:"Conversations to list (8)"`
		}) (*struct{ Body MailPerson }, error) {
			p, err := s.require(ctx, auth.RoleViewer)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			addr := cleanAddress(in.Email)
			if addr == "" {
				return nil, httpx.Validation("Enter an email address.")
			}
			var exclude uuid.UUID
			if in.Exclude != "" {
				exclude, _ = uuid.Parse(in.Exclude)
			}
			limit := in.Limit
			if limit <= 0 || limit > maxPersonThreads {
				limit = personThreads
			}
			out, err := s.person(ctx, p, addr, exclude, limit)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body MailPerson }{out}, nil
		})
}

func (s *service) person(ctx context.Context, p *auth.Principal, addr string, exclude uuid.UUID, limit int) (MailPerson, error) {
	out := MailPerson{Email: addr, Threads: []MailThread{}, Files: []MailPersonFile{}}
	pool := s.d.DB.Pool
	where := personThreadsSQL()
	var last *time.Time
	if err := pool.QueryRow(ctx, `SELECT count(*), max(t.last_message_at) FROM collab_threads t WHERE `+where,
		p.WorkspaceID, p.UserID, addr).Scan(&out.Conversations, &last); err != nil {
		return out, err
	}
	if out.Conversations == 0 {
		return out, nil
	}
	out.LastAt = last
	out.Others = out.Conversations
	if exclude != uuid.Nil {
		var in bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM collab_threads t WHERE `+where+` AND t.id = $4)`,
			p.WorkspaceID, p.UserID, addr, exclude).Scan(&in); err != nil {
			return out, err
		}
		if in {
			out.Others--
		}
	}
	if err := pool.QueryRow(ctx, `SELECT min(m.sent_at) FROM collab_messages m
		WHERE NOT m.draft AND m.thread_id IN (SELECT t.id FROM collab_threads t WHERE `+where+`)`,
		p.WorkspaceID, p.UserID, addr).Scan(&out.FirstAt); err != nil {
		return out, err
	}

	// One more than listed, in case the open one is among them.
	rows, err := pool.Query(ctx, `SELECT `+threadColumns+` FROM collab_threads t WHERE `+where+`
		ORDER BY t.last_message_at DESC, t.id DESC LIMIT $4`, p.WorkspaceID, p.UserID, addr, limit+1)
	if err != nil {
		return out, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (*threadRow, error) { return scanThread(r) })
	if err != nil {
		return out, err
	}
	kept := make([]*threadRow, 0, len(list))
	for _, t := range list {
		if out.Name == "" {
			out.Name = nameOf(t.Participants, addr)
		}
		if t.ID != exclude && len(kept) < limit {
			kept = append(kept, t)
		}
	}
	if out.Name == "" {
		// Their name may be on an older conversation only.
		var name *string
		if err := pool.QueryRow(ctx, `SELECT p->>'name' FROM collab_threads t, jsonb_array_elements(t.participants) p
			WHERE `+where+` AND lower(p->>'email') = $3 AND p->>'name' <> '' ORDER BY t.last_message_at DESC LIMIT 1`,
			p.WorkspaceID, p.UserID, addr).Scan(&name); err != nil && err != pgx.ErrNoRows {
			return out, err
		}
		if name != nil {
			out.Name = cleanName(*name)
		}
	}
	if out.Threads, err = threadsOut(ctx, pool, kept); err != nil {
		return out, err
	}

	rows, err = pool.Query(ctx, `SELECT m.id, m.thread_id, m.sent_at, x."index", x.filename, x.content_type, x.size, x.content_id
		FROM collab_messages m
		CROSS JOIN LATERAL jsonb_to_recordset(m.attachments) AS x("index" int, filename text, content_type text, size bigint, inline boolean, content_id text)
		WHERE m.workspace_id = $1 AND NOT m.draft AND lower(m.from_addr->>'email') = $3 AND NOT coalesce(x.inline, false)
			AND m.thread_id IN (SELECT t.id FROM collab_threads t WHERE `+where+`)
		ORDER BY m.sent_at DESC, m.id, x."index" LIMIT $4`, p.WorkspaceID, p.UserID, addr, personFiles)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var f MailPersonFile
		var index int
		var filename, contentType, contentID *string
		var size *int64
		if err := rows.Scan(&f.MessageID, &f.ThreadID, &f.SentAt, &index, &filename, &contentType, &size, &contentID); err != nil {
			return out, err
		}
		f.Attachment = MailAttachment{ID: strconv.Itoa(index), URL: s.attachmentURL(f.MessageID, index)}
		if filename != nil {
			f.Attachment.Filename = *filename
		}
		if contentType != nil {
			f.Attachment.ContentType = *contentType
		}
		if contentID != nil {
			f.Attachment.ContentID = *contentID
		}
		if size != nil {
			f.Attachment.Size = *size
		}
		out.Files = append(out.Files, f)
	}
	return out, rows.Err()
}

// nameOf is the name the address has among the participants ("" none).
func nameOf(people []Address, addr string) string {
	for _, x := range people {
		if strings.EqualFold(x.Email, addr) && strings.TrimSpace(x.Name) != "" {
			return x.Name
		}
	}
	return ""
}
