-- The newest release of the running major version, beside the newest release overall, so a new
-- major version out does not hide the release automatic updates install (contracts/admin.md "Updates").
ALTER TABLE instance_updates
    ADD COLUMN in_major_version      text NOT NULL DEFAULT '',
    ADD COLUMN in_major_published_at timestamptz,
    ADD COLUMN notified_in_major     text NOT NULL DEFAULT '';
