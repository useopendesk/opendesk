-- One shared reply draft per ticket: a pending reply body staged by a teammate or an API
-- client (an AI agent over MCP), shown in the composer of everyone who opens the ticket.
CREATE TABLE ticket_drafts (
    ticket_id    uuid PRIMARY KEY REFERENCES tickets(id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    body_html    text NOT NULL,
    body_text    text NOT NULL,
    author_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    api_key_id   uuid REFERENCES api_keys(id) ON DELETE SET NULL,
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ticket_drafts_workspace_idx ON ticket_drafts (workspace_id);
