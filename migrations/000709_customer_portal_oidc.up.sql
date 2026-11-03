ALTER TABLE brands ADD CONSTRAINT brands_workspace_id_id_key UNIQUE (workspace_id, id);
ALTER TABLE brands
    ADD COLUMN portal_auth_mode text NOT NULL DEFAULT 'local' CHECK (portal_auth_mode IN ('local', 'oidc', 'local_and_oidc')),
    ADD COLUMN portal_oidc_issuer text NOT NULL DEFAULT '',
    ADD COLUMN portal_oidc_client_id text NOT NULL DEFAULT '',
    ADD COLUMN portal_oidc_client_secret_enc bytea,
    ADD CONSTRAINT brands_portal_oidc_config CHECK (portal_auth_mode = 'local' OR
        (portal_oidc_issuer <> '' AND portal_oidc_client_id <> '' AND portal_oidc_client_secret_enc IS NOT NULL));
ALTER TABLE contacts ADD COLUMN customer_oidc_relink_required boolean NOT NULL DEFAULT false;
CREATE TABLE customer_oidc_identities (
    id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    -- Origin is optional metadata; ownership and tombstones survive brand deletion.
    brand_id uuid,
    contact_id uuid NOT NULL,
    issuer text NOT NULL CHECK (issuer <> ''),
    subject text NOT NULL CHECK (subject <> ''),
    email citext NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    UNIQUE (workspace_id, issuer, subject),
    FOREIGN KEY (workspace_id, brand_id) REFERENCES brands(workspace_id, id) ON DELETE SET NULL (brand_id),
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts(workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX customer_oidc_identities_contact_idx ON customer_oidc_identities(workspace_id, contact_id);
CREATE TABLE customer_oidc_relink_authorizations (
    id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    brand_id uuid NOT NULL,
    contact_id uuid NOT NULL,
    actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL CHECK (expires_at > created_at),
    used_at timestamptz,
    revoked_at timestamptz,
    FOREIGN KEY (workspace_id, brand_id) REFERENCES brands(workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, contact_id) REFERENCES contacts(workspace_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX customer_oidc_relink_authorizations_live_idx ON customer_oidc_relink_authorizations(workspace_id, brand_id, contact_id)
    WHERE used_at IS NULL AND revoked_at IS NULL;
CREATE INDEX customer_oidc_relink_authorizations_expiry_idx ON customer_oidc_relink_authorizations(expires_at);
