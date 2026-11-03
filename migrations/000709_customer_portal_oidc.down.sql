DROP TABLE customer_oidc_relink_authorizations;
DROP TABLE customer_oidc_identities;
ALTER TABLE contacts DROP COLUMN customer_oidc_relink_required;
ALTER TABLE brands DROP CONSTRAINT brands_portal_oidc_config,
    DROP COLUMN portal_oidc_client_secret_enc, DROP COLUMN portal_oidc_client_id,
    DROP COLUMN portal_oidc_issuer, DROP COLUMN portal_auth_mode,
    DROP CONSTRAINT brands_workspace_id_id_key;
