-- A custom MCP server's icon (contracts/integrations.md): "img:<image id>" (images,
-- owner_type 'integration'), or ''. Catalog integrations are drawn by the app.
ALTER TABLE integrations ADD COLUMN icon text NOT NULL DEFAULT '';

ALTER TABLE images DROP CONSTRAINT images_owner_type_check;
ALTER TABLE images ADD CONSTRAINT images_owner_type_check
    CHECK (owner_type IN ('user', 'contact', 'company', 'workspace', 'brand', 'widget', 'bot', 'integration'));
