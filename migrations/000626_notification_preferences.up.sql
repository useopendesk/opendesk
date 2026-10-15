-- Notification preferences and notification emails (contracts/api.md "Notification
-- preferences and emails").

-- The account's email schedule and pause, across workspaces.
CREATE TABLE notification_settings (
    user_id       uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    email_cadence text NOT NULL DEFAULT 'instant' CHECK (email_cadence IN ('instant', 'hourly', 'daily', 'off')),
    digest_hour   smallint NOT NULL DEFAULT 9 CHECK (digest_hour BETWEEN 0 AND 23),
    timezone      text NOT NULL DEFAULT 'UTC',
    paused_until  timestamptz,
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Channels a member chose for one notification type in one workspace. NULL follows the
-- default. Rows go with the membership.
CREATE TABLE notification_preferences (
    workspace_id uuid NOT NULL,
    user_id      uuid NOT NULL,
    type         text NOT NULL,
    email        boolean,
    push         boolean,
    desktop      boolean,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id, type),
    FOREIGN KEY (workspace_id, user_id) REFERENCES members(workspace_id, user_id) ON DELETE CASCADE
);

-- email_due_at: when an unread notification is emailed (NULL: never). emailed_at: when it
-- was queued for email; set once.
ALTER TABLE notifications
    ADD COLUMN email_due_at timestamptz,
    ADD COLUMN emailed_at   timestamptz;

CREATE INDEX notifications_email_due_idx ON notifications(email_due_at)
    WHERE email_due_at IS NOT NULL AND emailed_at IS NULL AND read_at IS NULL;

-- Email pacing per member (one email "conversation" per user and workspace): the cooldown
-- step reached while activity continues, when the next email may go, and when the sweep
-- should look again.
CREATE TABLE notification_email_state (
    workspace_id    uuid NOT NULL,
    user_id         uuid NOT NULL,
    step            smallint NOT NULL DEFAULT 0,
    last_emailed_at timestamptz,
    next_allowed_at timestamptz,
    last_urgent_at  timestamptz,
    next_check_at   timestamptz,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id),
    FOREIGN KEY (workspace_id, user_id) REFERENCES members(workspace_id, user_id) ON DELETE CASCADE
);

-- Notification emails sent: the per-person hourly and daily caps and the instance send
-- rate count these. Kept two days.
CREATE TABLE notification_email_log (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id  uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    sent_at       timestamptz NOT NULL DEFAULT now(),
    notifications integer NOT NULL,
    urgent        boolean NOT NULL DEFAULT false
);
CREATE INDEX notification_email_log_user_idx ON notification_email_log(user_id, sent_at DESC);
CREATE INDEX notification_email_log_sent_idx ON notification_email_log(sent_at);

-- When the member was last active in the app in this workspace (the web heartbeat).
-- Notification emails wait while it is recent.
ALTER TABLE members ADD COLUMN active_at timestamptz;

-- An email's unsubscribe link turned off notification email from a workspace: when, and
-- each type's email choice before it, so "turn email back on" restores them.
CREATE TABLE notification_unsubscribes (
    workspace_id    uuid NOT NULL,
    user_id         uuid NOT NULL,
    unsubscribed_at timestamptz NOT NULL DEFAULT now(),
    previous        jsonb NOT NULL DEFAULT '{}',
    PRIMARY KEY (workspace_id, user_id),
    FOREIGN KEY (workspace_id, user_id) REFERENCES members(workspace_id, user_id) ON DELETE CASCADE
);
