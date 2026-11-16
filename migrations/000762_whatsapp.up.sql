-- WhatsApp as a customer channel (contracts/whatsapp.md): a connection to a business
-- solution provider (Twilio first), whose inbound messages land on the customer's ticket
-- and whose replies go out through it.

CREATE TABLE whatsapp_connections (
    id                uuid PRIMARY KEY,
    workspace_id      uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    provider          text NOT NULL,
    name              text NOT NULL,
    -- The WhatsApp number customers write to, in E.164 ("+14155238886").
    sender            text NOT NULL CHECK (sender ~ '^\+[0-9]{6,15}$'),
    -- The provider account's name, refreshed on every test.
    account_name      text NOT NULL DEFAULT '',
    -- The API credentials, sealed with d.Box.
    credentials_enc   bytea NOT NULL,
    status            text NOT NULL DEFAULT 'connected' CHECK (status IN ('connected', 'error')),
    last_error        text NOT NULL DEFAULT '',
    health_status     text NOT NULL DEFAULT 'unknown' CHECK (health_status IN ('unknown', 'healthy', 'action_required', 'error')),
    health_detail     text NOT NULL DEFAULT '',
    health_checked_at timestamptz,
    -- The brand whose tickets this number's conversations are (routing sees it).
    brand_id          uuid REFERENCES brands(id) ON DELETE SET NULL,
    -- A conversation goes on the customer's open ticket, else on their latest one solved
    -- within reuse_days (0: never), else on a new ticket.
    reuse_days        integer NOT NULL DEFAULT 7 CHECK (reuse_days BETWEEN 0 AND 90),
    last_event_at     timestamptz,
    created_by        uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, sender)
);
CREATE INDEX whatsapp_connections_created_by_idx ON whatsapp_connections (created_by);
CREATE INDEX whatsapp_connections_brand_idx ON whatsapp_connections (brand_id);

-- Verified provider events, normalized (never the raw payload), waiting for or done by
-- the worker. The id deduplicates the provider's retries; rows go after a week.
CREATE TABLE whatsapp_inbound_events (
    connection_id     uuid NOT NULL REFERENCES whatsapp_connections(id) ON DELETE CASCADE,
    external_event_id text NOT NULL,
    workspace_id      uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    payload           jsonb NOT NULL,
    received_at       timestamptz NOT NULL DEFAULT now(),
    processed_at      timestamptz,
    PRIMARY KEY (connection_id, external_event_id)
);
CREATE INDEX whatsapp_inbound_events_workspace_idx ON whatsapp_inbound_events (workspace_id);
CREATE INDEX whatsapp_inbound_events_received_idx ON whatsapp_inbound_events (received_at);

-- A ticket message and the provider's id for it: what a delivery status is matched by, and
-- the connection a reply goes out through (the one the customer last wrote to).
CREATE TABLE whatsapp_messages (
    message_id    uuid PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE,
    workspace_id  uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    ticket_id     uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    connection_id uuid REFERENCES whatsapp_connections(id) ON DELETE SET NULL,
    direction     text NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    -- The provider's id (Twilio: the message SID); '' until an outbound message is sent.
    external_id   text NOT NULL DEFAULT '',
    -- The number of the customer, E.164.
    number        text NOT NULL,
    -- An approved template an outbound message sends (the provider's id of it) and the
    -- values of its variables, for a reply outside the customer-service window.
    template_id   text NOT NULL DEFAULT '',
    variables     jsonb NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX whatsapp_messages_external_idx ON whatsapp_messages (connection_id, external_id) WHERE external_id <> '';
CREATE INDEX whatsapp_messages_ticket_idx ON whatsapp_messages (ticket_id, created_at DESC);
CREATE INDEX whatsapp_messages_workspace_idx ON whatsapp_messages (workspace_id);
CREATE INDEX whatsapp_messages_connection_idx ON whatsapp_messages (connection_id);

-- Tickets a customer's WhatsApp message opened are on the whatsapp channel.
ALTER TABLE tickets DROP CONSTRAINT IF EXISTS tickets_channel_check;
ALTER TABLE tickets ADD CONSTRAINT tickets_channel_check CHECK (channel IN ('email', 'chat', 'api', 'portal', 'phone', 'whatsapp'));

-- A WhatsApp message can be delivered to the phone and read there.
ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_delivery_status_check;
ALTER TABLE messages ADD CONSTRAINT messages_delivery_status_check
    CHECK (delivery_status IN ('none', 'pending', 'sent', 'delivered', 'read', 'failed', 'bounced'));
