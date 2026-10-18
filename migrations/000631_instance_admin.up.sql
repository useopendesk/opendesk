-- Instance admin (contracts/admin.md): the operators of an instance, their own sessions
-- apart from the app's, their audit log, suspension of users and workspaces, and limit
-- overrides per workspace.

CREATE TABLE instance_admins (
    user_id    uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    role       text NOT NULL CHECK (role IN ('viewer', 'support', 'super')),
    granted_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Panel sessions: the refresh token is stored hashed and rotates on use; a session ends an
-- hour after its last refresh (expires_at) and 12 hours after sign-in.
CREATE TABLE admin_sessions (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_hash      bytea NOT NULL UNIQUE,
    prev_refresh_hash bytea,
    rotated_at        timestamptz,
    ip                text NOT NULL DEFAULT '',
    user_agent        text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now(),
    last_used_at      timestamptz NOT NULL DEFAULT now(),
    expires_at        timestamptz NOT NULL,
    revoked_at        timestamptz
);
CREATE INDEX admin_sessions_user_idx ON admin_sessions(user_id);
CREATE INDEX admin_sessions_prev_refresh_idx ON admin_sessions(prev_refresh_hash) WHERE prev_refresh_hash IS NOT NULL;

-- What admins did. The actor's email is kept after their account is gone; targets are ids only.
CREATE TABLE admin_audit_logs (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    actor_type    text NOT NULL CHECK (actor_type IN ('admin', 'cli')),
    actor_email   text NOT NULL DEFAULT '',
    action        text NOT NULL,
    target_type   text NOT NULL CHECK (target_type IN ('user', 'workspace', 'admin', 'job', 'instance')),
    target_id     text,
    data          jsonb NOT NULL DEFAULT '{}',
    ip            text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX admin_audit_logs_created_idx ON admin_audit_logs(created_at DESC, id DESC);
CREATE INDEX admin_audit_logs_target_idx ON admin_audit_logs(target_id, created_at DESC) WHERE target_id IS NOT NULL;
CREATE INDEX admin_audit_logs_actor_idx ON admin_audit_logs(actor_user_id, created_at DESC);

ALTER TABLE users
    ADD COLUMN suspended_at     timestamptz,
    ADD COLUMN suspended_reason text NOT NULL DEFAULT '';

ALTER TABLE workspaces
    ADD COLUMN suspended_at     timestamptz,
    ADD COLUMN suspended_reason text NOT NULL DEFAULT '';
CREATE INDEX workspaces_suspended_idx ON workspaces(suspended_at) WHERE suspended_at IS NOT NULL;
CREATE INDEX workspaces_created_idx ON workspaces(created_at DESC, id DESC);
CREATE INDEX users_created_idx ON users(created_at DESC, id DESC);
-- The overview sums AI usage across the instance by day.
CREATE INDEX ai_usage_created_idx ON ai_usage(created_at);

-- Limit overrides; NULL keeps the plan's. 0 mailboxes or members means unlimited.
ALTER TABLE subscriptions
    ADD COLUMN max_mailboxes_override  integer CHECK (max_mailboxes_override BETWEEN 0 AND 100000),
    ADD COLUMN max_members_override    integer CHECK (max_members_override BETWEEN 0 AND 100000),
    ADD COLUMN monthly_micro_usd_override bigint CHECK (monthly_micro_usd_override BETWEEN 0 AND 100000000000);
