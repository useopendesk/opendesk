package whatsapp

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/domain"
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
	tmpl.HeaderType = row.HeaderType
	if strings.TrimSpace(tmpl.Body) == "" {
		tmpl.Body = row.BodyContent
	}
	prepareTemplateHeader(tmpl, row)
	acc, _, err := s.account(c)
	if err != nil {
		return s.failMessage(ctx, c.WorkspaceID, m.ID, "This WhatsApp connection's credentials could not be read.")
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
