package email

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/jhillyerd/enmime/v2"

	"github.com/useopendesk/opendesk/internal/jobs"
)

// sendQueued sends the queued ticket emails (auto-replies, CSAT) as the worker would and
// returns them as the recipient gets them, oldest first.
func (w *world) sendQueued(t *testing.T) []*enmime.Envelope {
	t.Helper()
	rows, err := w.d.DB.Pool.Query(w.ctx, `SELECT args FROM river_job WHERE kind = 'email.send_ticket_email' AND args->>'workspace_id' = $1 ORDER BY id`,
		w.wid.String())
	if err != nil {
		t.Fatal(err)
	}
	var list []jobs.SendTicketEmailArgs
	for rows.Next() {
		var raw []byte
		var a jobs.SendTicketEmailArgs
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal(raw, &a)
		list = append(list, a)
	}
	rows.Close()
	var sent [][]byte
	prev := w.s.rawSend
	w.s.rawSend = func(_ context.Context, _ string, _ []string, raw []byte) error { sent = append(sent, raw); return nil }
	defer func() { w.s.rawSend = prev }()
	for _, a := range list {
		if err := w.s.sendTicketEmail(w.ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	out := make([]*enmime.Envelope, 0, len(sent))
	for _, raw := range sent {
		env, err := enmime.ReadEnvelope(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, env)
	}
	return out
}
