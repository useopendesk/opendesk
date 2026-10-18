package core

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/audit"
	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/emails"
)

// Ops is what the instance admin area (internal/admin, contracts/admin.md) does through
// core: the same checks, removals and session endings the app's own routes use.
type Ops struct{ s *svc }

func NewOps(d *deps.Deps) *Ops { return &Ops{s: newSvc(d)} }

// CheckSecondFactor verifies a TOTP code (its step is consumed, so it cannot be replayed)
// or consumes a recovery code of uid. A wrong code is 401 invalid_code.
func (o *Ops) CheckSecondFactor(ctx context.Context, tx pgx.Tx, uid uuid.UUID, code string) (string, error) {
	return o.s.checkSecondFactor(ctx, tx, uid, code)
}

// DeleteWorkspace removes a workspace and everything it owns, like its owner's "Delete
// workspace". The caller commits tx, then calls Finish with the result.
func (o *Ops) DeleteWorkspace(ctx context.Context, tx pgx.Tx, wid uuid.UUID) (Removal, error) {
	members, keys, err := o.s.deleteWorkspace(ctx, tx, wid)
	return Removal{disconnect: members, files: keys}, err
}

// DeleteAccount removes a user like their own "Delete account": the workspaces they own
// alone go with them. 409 sole_owner while they own a workspace with other members. The
// workspaces they leave record member.account_deleted with actor (attribution).
func (o *Ops) DeleteAccount(ctx context.Context, tx pgx.Tx, actor *auth.Principal, uid uuid.UUID) (Removal, error) {
	disconnect, keys, err := o.s.deleteAccount(ctx, tx, actor, uid)
	return Removal{disconnect: disconnect, files: keys}, err
}

// Removal is what is left to do once a deletion committed.
type Removal struct {
	disconnect []uuid.UUID
	files      []string
}

// Finish closes the removed people's sockets and purges the stored files.
func (o *Ops) Finish(r Removal) {
	o.s.purgeFiles(r.files)
	for _, uid := range r.disconnect {
		o.s.disconnect(uid, uuid.Nil)
	}
}

// EndSessions ends every app session of uid (and their push devices). Call Disconnect
// after the commit.
func (o *Ops) EndSessions(ctx context.Context, tx pgx.Tx, uid uuid.UUID) error {
	return revokeOtherSessions(ctx, tx, uid, uuid.Nil)
}

// EndSession ends one app session of uid; false when it was not live.
func (o *Ops) EndSession(ctx context.Context, tx pgx.Tx, uid, sid uuid.UUID) (bool, error) {
	return endSession(ctx, tx, uid, sid)
}

// Disconnect asks the gateway to close the user's sockets: of one session, or all with
// uuid.Nil.
func (o *Ops) Disconnect(uid, sid uuid.UUID) { o.s.disconnect(uid, sid) }

// SendVerifyEmail sends a new email confirmation link.
func (o *Ops) SendVerifyEmail(ctx context.Context, tx pgx.Tx, uid uuid.UUID, email string) error {
	return o.s.sendVerifyEmail(ctx, tx, uid, email)
}

// SendTwoFactorDisabled tells the user an administrator turned off their two-factor
// authentication.
func (o *Ops) SendTwoFactorDisabled(ctx context.Context, tx pgx.Tx, email string) error {
	return o.s.enqueueMail(ctx, tx, email, emails.TwoFactorDisabled(o.s.site()))
}

// AuditUser records an operator's action on a user's account in each of their
// workspaces' audit logs, with no actor user and data.actor = "instance_admin".
func (o *Ops) AuditUser(ctx context.Context, tx pgx.Tx, actor *auth.Principal, uid uuid.UUID, action string, data map[string]any) error {
	_, err := auditUser(ctx, tx, actor, uid, action, InstanceAdminData(data))
	return err
}

// AuditWorkspace records an operator's action in the workspace's audit log.
func (o *Ops) AuditWorkspace(ctx context.Context, tx pgx.Tx, actor *auth.Principal, wid uuid.UUID, action string, data map[string]any) error {
	return audit.Log(ctx, tx, actor, wid, action, "workspace", &wid, InstanceAdminData(data))
}

// InstanceAdminData tags audit data as the instance operator's.
func InstanceAdminData(data map[string]any) map[string]any {
	out := make(map[string]any, len(data)+1)
	for k, v := range data {
		out[k] = v
	}
	out["actor"] = "instance_admin"
	return out
}
