DROP INDEX IF EXISTS tickets_ws_received_at_idx;
ALTER TABLE messages DROP COLUMN IF EXISTS received_at;
ALTER TABLE mailboxes DROP COLUMN IF EXISTS alias_teams;
