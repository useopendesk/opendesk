ALTER TABLE subscriptions
    DROP COLUMN IF EXISTS monthly_micro_usd_override,
    DROP COLUMN IF EXISTS max_members_override,
    DROP COLUMN IF EXISTS max_mailboxes_override;
DROP INDEX IF EXISTS ai_usage_created_idx;
DROP INDEX IF EXISTS users_created_idx;
DROP INDEX IF EXISTS workspaces_created_idx;
DROP INDEX IF EXISTS workspaces_suspended_idx;
ALTER TABLE workspaces DROP COLUMN IF EXISTS suspended_reason, DROP COLUMN IF EXISTS suspended_at;
ALTER TABLE users DROP COLUMN IF EXISTS suspended_reason, DROP COLUMN IF EXISTS suspended_at;
DROP TABLE IF EXISTS admin_audit_logs;
DROP TABLE IF EXISTS admin_sessions;
DROP TABLE IF EXISTS instance_admins;
