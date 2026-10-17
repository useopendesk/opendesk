-- Imports from other helpdesks and CSV files (contracts/imports.md). One runs per workspace
-- at a time; it checkpoints in `cursor` after every page and resumes from there.
CREATE TABLE imports (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    source          text NOT NULL CHECK (source IN ('zendesk', 'helpscout', 'freshdesk', 'intercom', 'csv')),
    -- The source account (subdomain, app id, the CSV file's hash); records are keyed by it.
    account_key     text NOT NULL,
    account_name    text NOT NULL DEFAULT '',
    account_url     text NOT NULL DEFAULT '',
    status          text NOT NULL DEFAULT 'running'
                    CHECK (status IN ('running', 'paused', 'completed', 'failed', 'canceled', 'undoing', 'undone')),
    phase           text NOT NULL DEFAULT 'articles' CHECK (phase IN ('articles', 'tickets', 'done')),
    run             integer NOT NULL DEFAULT 1,
    -- Bumped by pause, resume, sync and undo: a job of an older generation stops.
    generation      integer NOT NULL DEFAULT 1,
    options         jsonb NOT NULL DEFAULT '{}',
    mapping         jsonb NOT NULL DEFAULT '{}',
    -- Where each phase resumes: {"articles": "<cursor>", "tickets": "<cursor>"}.
    cursor          jsonb NOT NULL DEFAULT '{}',
    -- AES-GCM (d.Box): the source credentials. Removed 30 days after the last run, on undo,
    -- or when the user removes them.
    credentials_enc bytea,
    -- The uploaded CSV file, removed when the import completes or is canceled, on undo, or
    -- with the credentials.
    storage_key     text,
    totals          jsonb NOT NULL DEFAULT '{}',
    counts          jsonb NOT NULL DEFAULT '{}',
    -- The latest problems, newest first: [{at, item, message}].
    problems        jsonb NOT NULL DEFAULT '[]',
    last_error      text NOT NULL DEFAULT '',
    needs_credentials boolean NOT NULL DEFAULT false,
    -- Failures in a row of the source itself; five fail the import.
    failures        integer NOT NULL DEFAULT 0,
    waiting_until   timestamptz,
    -- A sync brings over tickets updated since this time (the start of the previous run).
    updated_since   timestamptz,
    started_by      uuid REFERENCES users(id) ON DELETE SET NULL,
    started_at      timestamptz NOT NULL DEFAULT now(),
    finished_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX imports_ws_idx ON imports(workspace_id, created_at DESC);
CREATE UNIQUE INDEX imports_one_active_uq ON imports(workspace_id) WHERE status IN ('running', 'paused', 'undoing');
CREATE INDEX imports_credentials_idx ON imports(finished_at) WHERE credentials_enc IS NOT NULL;

-- What an import wrote, by source id: a second run of the same account finds it instead of
-- writing it again, and undo removes what the import created (created = true).
CREATE TABLE import_records (
    workspace_id      uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    source            text NOT NULL,
    account_key       text NOT NULL,
    kind              text NOT NULL CHECK (kind IN ('ticket', 'message', 'contact', 'company', 'tag', 'article', 'collection')),
    external_id       text NOT NULL,
    local_id          uuid NOT NULL,
    import_id         uuid NOT NULL REFERENCES imports(id) ON DELETE CASCADE,
    created           boolean NOT NULL DEFAULT true,
    -- The source's updated time when it was written; a sync skips tickets that have not moved.
    source_updated_at timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, source, account_key, kind, external_id)
);
CREATE INDEX import_records_import_idx ON import_records(import_id, kind);

-- A CSV file uploaded for an import that has not started; removed after 24 hours unused.
CREATE TABLE import_uploads (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    storage_key  text NOT NULL,
    filename     text NOT NULL,
    sha256       text NOT NULL,
    size_bytes   bigint NOT NULL,
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX import_uploads_created_idx ON import_uploads(created_at);
