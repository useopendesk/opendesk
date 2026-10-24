package email

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/domain"
)

func TestMailboxWritesShareImportLockOrder(t *testing.T) {
	w := newWorld(t)
	m := w.mailbox(InboundForwarding, OutboundSystem, nil)
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()

	importTx, err := w.d.DB.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer importTx.Rollback(ctx)
	if _, err := importTx.Exec(ctx, `UPDATE workspaces SET ticket_seq = ticket_seq + 1 WHERE id = $1`, w.wid); err != nil {
		t.Fatal(err)
	}

	mailboxLocked := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- w.d.DB.Tx(ctx, func(tx pgx.Tx) error {
			if err := lockWorkspaceForMailboxWrite(ctx, tx, w.wid); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `SELECT id FROM mailboxes WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, w.wid, m.ID); err != nil {
				return err
			}
			close(mailboxLocked)
			return nil
		})
	}()

	select {
	case <-mailboxLocked:
		t.Fatal("mailbox write locked the mailbox before the workspace")
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := domain.CreateTicket(ctx, importTx, domain.NewTicket{WorkspaceID: w.wid, Subject: "Imported", Channel: domain.ChannelEmail,
		MailboxID: &m.ID, Imported: true, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := importTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
