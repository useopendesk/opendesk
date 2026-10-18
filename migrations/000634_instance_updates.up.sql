-- Releases and automatic updates (contracts/admin.md "Updates"): one row for the instance.
-- The update worker records the running version and the newest release; opendesk-ctl
-- reports each automatic update run through `opendesk updates report`.
CREATE TABLE instance_updates (
    id                       boolean PRIMARY KEY DEFAULT true CHECK (id),
    running_version          text NOT NULL DEFAULT '',
    running_since            timestamptz,
    previous_version         text NOT NULL DEFAULT '',
    latest_version           text NOT NULL DEFAULT '',
    latest_published_at      timestamptz,
    checked_at               timestamptz,
    check_error              text NOT NULL DEFAULT '',
    notified_version         text NOT NULL DEFAULT '',
    auto_enabled             boolean,
    auto_ran_at              timestamptz,
    auto_result              text NOT NULL DEFAULT 'none' CHECK (auto_result IN ('none', 'current', 'updated', 'skipped', 'failed')),
    auto_detail              text NOT NULL DEFAULT '',
    auto_failure_notified_at timestamptz
);
