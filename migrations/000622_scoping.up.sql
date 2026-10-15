-- Uploads record the API key that made them, so only that key attaches them.
ALTER TABLE attachments ADD COLUMN uploaded_by_api_key_id uuid REFERENCES api_keys(id) ON DELETE SET NULL;
