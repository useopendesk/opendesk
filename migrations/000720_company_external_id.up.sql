-- The id a host app (the chat widget's identify) gives its own account, so a signed-in user's
-- company is found again instead of created twice.
ALTER TABLE companies ADD COLUMN external_id text;
CREATE UNIQUE INDEX companies_ws_external_uq ON companies(workspace_id, external_id) WHERE external_id IS NOT NULL;
