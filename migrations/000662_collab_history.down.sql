ALTER TABLE subscriptions DROP COLUMN collab_storage_override;
ALTER TABLE collab_accounts DROP COLUMN stored_bytes;
DROP INDEX collab_messages_search_idx;
ALTER TABLE collab_messages DROP COLUMN search;
DROP INDEX collab_messages_account_sent_idx;
ALTER TABLE collab_messages DROP COLUMN size;
DROP TABLE collab_history;
