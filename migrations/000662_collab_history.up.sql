-- Collab history imports, storage and search (contracts/collab.md "History", "Storage", "Search").

-- The latest history import of each account: its scope, its place at the provider and its counts.
CREATE TABLE collab_history (
    account_id         uuid PRIMARY KEY REFERENCES collab_accounts(id) ON DELETE CASCADE,
    workspace_id       uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    requested_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    status             text NOT NULL CHECK (status IN ('running', 'paused', 'limited', 'done', 'canceled', 'error')),
    -- Mail received from then on; NULL = all mail.
    since              timestamptz,
    include_spam_trash boolean NOT NULL DEFAULT false,
    started_at         timestamptz NOT NULL DEFAULT now(),
    finished_at        timestamptz,
    -- Where it goes on at the provider (Gmail page token, Microsoft delta pages per folder,
    -- IMAP UIDs per folder).
    cursor             jsonb NOT NULL DEFAULT '{}',
    reached            timestamptz,
    imported           integer NOT NULL DEFAULT 0,
    present            integer NOT NULL DEFAULT 0,
    too_large          integer NOT NULL DEFAULT 0,
    unreadable         integer NOT NULL DEFAULT 0,
    total              integer,
    last_error         text NOT NULL DEFAULT '',
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX collab_history_workspace_idx ON collab_history(workspace_id);
CREATE INDEX collab_history_active_idx ON collab_history(account_id) WHERE status IN ('running', 'limited');

-- What each message keeps in opendesk: its text, HTML and raw copy, in bytes.
ALTER TABLE collab_messages ADD COLUMN size bigint NOT NULL DEFAULT 0;
UPDATE collab_messages SET size = octet_length(body_html) + octet_length(body_text) + CASE WHEN raw_key <> '' THEN
    COALESCE((SELECT sum((e->>'size')::bigint) FROM jsonb_array_elements(attachments) e), 0) ELSE 0 END;
CREATE INDEX collab_messages_account_sent_idx ON collab_messages(account_id, sent_at) INCLUDE (size);

-- Every message is searched on its own: subject, sender and its first 100,000 characters.
ALTER TABLE collab_messages ADD COLUMN search tsvector GENERATED ALWAYS AS (to_tsvector('simple'::regconfig,
    left(subject, 1000) || ' ' || COALESCE(from_addr->>'name', '') || ' ' || COALESCE(from_addr->>'email', '') || ' ' ||
    left(body_text, 100000))) STORED;
CREATE INDEX collab_messages_search_idx ON collab_messages USING gin (search);

-- The sum of its messages' sizes, kept by the sync at the end of every pass.
ALTER TABLE collab_accounts ADD COLUMN stored_bytes bigint NOT NULL DEFAULT 0;
UPDATE collab_accounts a SET stored_bytes = COALESCE((SELECT sum(m.size) FROM collab_messages m WHERE m.account_id = a.id), 0);

-- An instance admin's Collab storage allowance for the workspace, in bytes (0 = unlimited).
ALTER TABLE subscriptions ADD COLUMN collab_storage_override bigint CHECK (collab_storage_override >= 0);
