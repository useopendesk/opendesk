-- Templates (contracts/templates.md): each member's profile feeds the agent variables,
-- and each brand can design its emails.
ALTER TABLE members
    ADD COLUMN title text NOT NULL DEFAULT '',
    ADD COLUMN phone text NOT NULL DEFAULT '',
    ADD COLUMN pronouns text NOT NULL DEFAULT '',
    ADD COLUMN signature_html text NOT NULL DEFAULT '';

CREATE TABLE email_templates (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    brand_id     uuid NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    kind         text NOT NULL CHECK (kind IN ('layout', 'csat')),
    subject      text NOT NULL DEFAULT '',
    body_html    text NOT NULL DEFAULT '',
    options      jsonb NOT NULL DEFAULT '{}',
    updated_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, brand_id, kind)
);
CREATE INDEX email_templates_brand_idx ON email_templates(brand_id);
