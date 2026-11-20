package whatsapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/helpdesk/hd"
)

// templateSendMaxAttempts is how often Meta's rate limit or a server error may be retried
// before the message is failed: it matches the send job's MaxAttempts. The count is kept in
// the message's delivery_meta because the send helper is handed the message, not the job.
const templateSendMaxAttempts = 5

// sendTemplateMessage delivers one outbound reply as an approved Meta template. A template
// may go out at any time, so the customer service window is not checked here.
func (s *Service) sendTemplateMessage(ctx context.Context, c connection, m outboundMessage, waID string, tmpl *whatsappTemplate) error {
	row, reason, err := s.templateForSend(ctx, c, tmpl)
	if err != nil {
		return err
	}
	if reason != "" {
		return s.failMessage(ctx, c.WorkspaceID, m.ID, reason)
	}
	// What is sent is the row as re-read, never only what the message carried when it was
	// written: the template may have been paused or changed since.
	tmpl.Name, tmpl.Language = row.Name, row.Language
	tmpl.HeaderType = strings.ToUpper(strings.TrimSpace(row.HeaderType))
	if strings.TrimSpace(tmpl.Body) == "" {
		tmpl.Body = row.BodyContent
	}
	prepareTemplateHeader(tmpl, row)
	// prepareTemplateHeader drops every header that is not text. A media header keeps its
	// shape here so components() knows which file component to build; the file is uploaded
	// below and its Meta id fills the component.
	headerKind := templateMediaKind(row.HeaderType)
	if headerKind != "" {
		tmpl.HeaderType = strings.ToUpper(strings.TrimSpace(row.HeaderType))
	}
	acc, _, err := s.account(c)
	if err != nil {
		return s.failMessage(ctx, c.WorkspaceID, m.ID, "This WhatsApp connection's credentials could not be read.")
	}
	if headerKind != "" {
		reason, uerr := s.uploadTemplateHeaderMedia(ctx, c, acc, tmpl)
		if uerr != nil {
			// Meta refused the upload before accepting anything, so the same send may
			// succeed later: count the attempts here as the template send does.
			if attempts := templateSendAttempts(m.DeliveryMeta) + 1; attempts < templateSendMaxAttempts {
				if aerr := s.recordTemplateAttempt(ctx, c.WorkspaceID, m.ID, attempts); aerr != nil {
					return aerr
				}
				return uerr
			}
			return s.failMessage(ctx, c.WorkspaceID, m.ID, whatsappSendError(uerr))
		}
		if reason != "" {
			return s.failMessage(ctx, c.WorkspaceID, m.ID, reason)
		}
	}
	wamid, err := NewClient(s.http).SendTemplate(ctx, acc, waID, tmpl.Name, tmpl.Language, tmpl.components())
	if err != nil {
		if mErr, ok := err.(*MetaAPIError); ok && (mErr.StatusCode == http.StatusTooManyRequests || mErr.StatusCode >= http.StatusInternalServerError) {
			if attempts := templateSendAttempts(m.DeliveryMeta) + 1; attempts < templateSendMaxAttempts {
				// Meta refused it before accepting: safe to let River try again.
				if aerr := s.recordTemplateAttempt(ctx, c.WorkspaceID, m.ID, attempts); aerr != nil {
					return aerr
				}
				return err
			}
		}
		return s.failMessage(ctx, c.WorkspaceID, m.ID, whatsappSendError(err))
	}
	meta := map[string]any{"provider_status": "sent", "provider_sent_at": time.Now().UTC().Format(time.RFC3339)}
	raw, _ := json.Marshal(meta)
	return s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE messages SET delivery_status = 'sent', provider_message_id = $3,
				delivery_error = '', delivery_meta = (delivery_meta - 'template_send_attempts') || $4::jsonb
			WHERE workspace_id = $1 AND id = $2`, c.WorkspaceID, m.ID, wamid, raw); err != nil {
			return err
		}
		return domain.PublishMessageUpdated(ctx, tx, c.WorkspaceID, m.ID)
	})
}

// templateForSend re-reads the template an outbound message names and checks Meta still lets
// it go out: it exists, is approved, and belongs to the connection the message sends from. A
// reason is the safe text to record on the message; an error is a failure to retry.
func (s *Service) templateForSend(ctx context.Context, c connection, tmpl *whatsappTemplate) (whatsappTemplateRow, string, error) {
	var (
		row whatsappTemplateRow
		err error
	)
	if id, perr := uuid.Parse(strings.TrimSpace(tmpl.ID)); perr == nil && id != uuid.Nil {
		row, err = loadTemplate(ctx, s.d.DB.Pool, c.WorkspaceID, id)
	} else {
		row, err = loadTemplateByNameLanguage(ctx, s.d.DB.Pool, c.WorkspaceID, c.ID,
			strings.TrimSpace(tmpl.Name), strings.TrimSpace(tmpl.Language))
	}
	if db.IsNotFound(err) {
		return row, "This WhatsApp template is no longer available.", nil
	}
	if err != nil {
		return row, "", err
	}
	if row.ConnectionID != c.ID {
		return row, "This WhatsApp template belongs to another WhatsApp connection.", nil
	}
	if !strings.EqualFold(strings.TrimSpace(row.Status), templateApproved) {
		return row, "This WhatsApp template is no longer approved at Meta.", nil
	}
	return row, "", nil
}

// prepareTemplateHeader prepares a text header for components(). components() reads the
// header's variable out of HeaderText and its value out of Params, so the agent's header
// value (delivery_meta's header_text) travels under a placeholder the body's own variables
// do not use: Meta numbers a header's and a body's variables separately, and both may be
// {{1}}. A static header sends no component, so Meta uses the approved text as it is.
func prepareTemplateHeader(tmpl *whatsappTemplate, row whatsappTemplateRow) {
	if !strings.EqualFold(strings.TrimSpace(row.HeaderType), "TEXT") {
		tmpl.HeaderType, tmpl.HeaderText = "", ""
		return
	}
	keys := orderedPlaceholders(row.HeaderContent)
	if len(keys) == 0 || strings.TrimSpace(tmpl.HeaderText) == "" {
		tmpl.HeaderText = ""
		return
	}
	bodyKeys := make(map[string]bool)
	for _, key := range orderedPlaceholders(row.BodyContent) {
		bodyKeys[key] = true
	}
	carrier := keys[0]
	for i := 0; bodyKeys[carrier]; i++ {
		carrier = "header" + strconv.Itoa(i)
	}
	if tmpl.Params == nil {
		tmpl.Params = make(map[string]string)
	}
	tmpl.Params[carrier] = tmpl.HeaderText
	tmpl.HeaderText = "{{" + carrier + "}}"
}

// uploadTemplateHeaderMedia reads the file an image, video or document header names out of
// storage, checks it against Meta's caps from the stored row, and uploads it so the send can
// carry its media id. It returns the safe reason a message cannot be sent, or an error worth
// retrying (Meta's rate limit or a server error). The storage key never appears in the return
// value or a log line.
func (s *Service) uploadTemplateHeaderMedia(ctx context.Context, c connection, acc Account, tmpl *whatsappTemplate) (string, error) {
	kind := templateMediaKind(tmpl.HeaderType)
	if kind == "" {
		return "", nil
	}
	id, err := uuid.Parse(strings.TrimSpace(tmpl.HeaderAttachmentID))
	if err != nil || id == uuid.Nil {
		return "This template's file is missing.", nil
	}
	var (
		filename, contentType, storageKey string
		sizeBytes                         int64
	)
	err = s.d.DB.Pool.QueryRow(ctx, `SELECT filename, content_type, size_bytes, storage_key FROM attachments
		WHERE workspace_id = $1 AND id = $2`, c.WorkspaceID, id).Scan(&filename, &contentType, &sizeBytes, &storageKey)
	if db.IsNotFound(err) {
		return "This template's file is missing.", nil
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(storageKey) == "" {
		return "This template's file is missing.", nil
	}
	// The row as it is now decides: the file may have been replaced or re-uploaded since the
	// API checked it.
	if problem := templateMediaProblem(kind, contentType, sizeBytes); problem != "" {
		return problem, nil
	}
	limit := mediaSizeLimit(kind)
	body, err := s.d.Storage.Get(ctx, storageKey)
	if err != nil {
		return "This template's file could not be read.", nil
	}
	content, readErr := io.ReadAll(io.LimitReader(body, limit+1))
	_ = body.Close()
	if readErr != nil {
		return "This template's file could not be read.", nil
	}
	if int64(len(content)) > limit {
		return templateMediaProblem(kind, contentType, int64(len(content))), nil
	}
	if len(content) == 0 {
		return "This template's file is missing.", nil
	}
	filename = hd.CleanFilename(filename)
	mediaID, err := NewClient(s.http).UploadMedia(ctx, acc, content, contentType, filename)
	if err != nil {
		if metaRetryable(err) {
			return "", err
		}
		return whatsappSendError(err), nil
	}
	tmpl.HeaderMediaID = mediaID
	if kind == "document" {
		tmpl.HeaderMediaFilename = filename
	}
	return "", nil
}

// templateSendAttempts reads how often a template send was already refused for a transient
// Meta error out of the message's delivery_meta.
func templateSendAttempts(raw []byte) int {
	var env struct {
		Attempts int `json:"template_send_attempts"`
	}
	if len(raw) == 0 {
		return 0
	}
	_ = json.Unmarshal(raw, &env)
	return env.Attempts
}

// recordTemplateAttempt stores the attempt count a retry continues from.
func (s *Service) recordTemplateAttempt(ctx context.Context, wid, msgID uuid.UUID, attempts int) error {
	raw, _ := json.Marshal(map[string]any{"template_send_attempts": attempts})
	_, err := s.d.DB.Pool.Exec(ctx, `UPDATE messages SET delivery_meta = delivery_meta || $3::jsonb
		WHERE workspace_id = $1 AND id = $2`, wid, msgID, raw)
	return err
}
