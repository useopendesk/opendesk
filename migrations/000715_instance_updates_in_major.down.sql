ALTER TABLE instance_updates
    DROP COLUMN notified_in_major,
    DROP COLUMN in_major_published_at,
    DROP COLUMN in_major_version;
