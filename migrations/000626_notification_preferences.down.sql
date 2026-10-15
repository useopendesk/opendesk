DROP TABLE IF EXISTS notification_unsubscribes;
ALTER TABLE members DROP COLUMN IF EXISTS active_at;
DROP TABLE IF EXISTS notification_email_log;
DROP TABLE IF EXISTS notification_email_state;
DROP INDEX IF EXISTS notifications_email_due_idx;
ALTER TABLE notifications DROP COLUMN IF EXISTS emailed_at, DROP COLUMN IF EXISTS email_due_at;
DROP TABLE IF EXISTS notification_preferences;
DROP TABLE IF EXISTS notification_settings;
