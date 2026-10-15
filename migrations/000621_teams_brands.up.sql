-- Teams, brands and routing (contracts/teams.md).
--
-- A brand is what customers see: a name, colours, a help center and any number of chat
-- widgets (one per website), plus the email addresses that belong to it. Every workspace
-- has one default brand, created here from the workspace's existing widget and help
-- center settings, so existing embeds, help center links and addresses keep working.
--
-- A team owns work. A private team's tickets are visible only to its members, its
-- observers, and API keys and webhooks one of its members granted it to.

-- Brands.
CREATE TABLE brands (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name                text NOT NULL,
    -- The help center lives at APP_URL/help/<slug>. The default brand keeps the workspace slug.
    slug                text NOT NULL UNIQUE,
    is_default          boolean NOT NULL DEFAULT false,
    color               text NOT NULL DEFAULT '',
    locale              text NOT NULL DEFAULT '',
    help_center_enabled boolean NOT NULL DEFAULT false,
    help_center_title   text NOT NULL DEFAULT '',
    -- logo, help_center_logo, help_center_favicon -> "img:<id>"
    images              jsonb NOT NULL DEFAULT '{}',
    position            integer NOT NULL DEFAULT 0,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name)
);
CREATE UNIQUE INDEX brands_default_uq ON brands(workspace_id) WHERE is_default;

INSERT INTO brands (workspace_id, name, slug, is_default, color, locale, help_center_enabled, help_center_title, images, created_at)
SELECT w.id, w.name, w.slug, true,
       COALESCE(w.settings ->> 'brand_color', ''),
       COALESCE(w.settings ->> 'locale', ''),
       COALESCE((w.settings ->> 'help_center_enabled')::boolean, false),
       COALESCE(w.settings ->> 'help_center_title', ''),
       jsonb_strip_nulls(jsonb_build_object(
           'help_center_logo', w.brand_images -> 'help_center_logo',
           'help_center_favicon', w.brand_images -> 'help_center_favicon')),
       w.created_at
FROM workspaces w;

-- Chat widgets, one per website. The default widget keeps the workspace's key and secret.
CREATE TABLE widgets (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    brand_id     uuid NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    name         text NOT NULL,
    key          text NOT NULL UNIQUE,
    secret_enc   bytea,
    -- WidgetSettings (contracts/api.md), plus topics and routing (contracts/teams.md).
    settings     jsonb NOT NULL DEFAULT '{}',
    -- New conversations go to this team unless a topic or a routing rule says otherwise.
    team_id      uuid REFERENCES teams(id) ON DELETE SET NULL,
    -- avatar, launcher -> "img:<id>"
    images       jsonb NOT NULL DEFAULT '{}',
    seen_at      timestamptz,
    position     integer NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX widgets_ws_idx ON widgets(workspace_id, brand_id);

INSERT INTO widgets (workspace_id, brand_id, name, key, secret_enc, settings, images, seen_at, created_at)
SELECT w.id, b.id, 'Website', w.widget_key, w.widget_secret_enc,
       COALESCE(w.settings -> 'widget', '{}'),
       jsonb_strip_nulls(jsonb_build_object(
           'avatar', w.brand_images -> 'widget_avatar',
           'launcher', w.brand_images -> 'widget_launcher')),
       w.widget_seen_at, w.created_at
FROM workspaces w JOIN brands b ON b.workspace_id = w.id AND b.is_default;

ALTER TABLE widget_sessions ADD COLUMN widget_id uuid REFERENCES widgets(id) ON DELETE CASCADE;
UPDATE widget_sessions s SET widget_id = g.id FROM widgets g WHERE g.workspace_id = s.workspace_id;
CREATE INDEX widget_sessions_widget_idx ON widget_sessions(widget_id);

ALTER TABLE images DROP CONSTRAINT images_owner_type_check;
ALTER TABLE images ADD CONSTRAINT images_owner_type_check
    CHECK (owner_type IN ('user', 'contact', 'company', 'workspace', 'brand', 'widget'));

-- Teams.
ALTER TABLE teams
    ADD COLUMN description    text NOT NULL DEFAULT '',
    ADD COLUMN color          text NOT NULL DEFAULT 'gray',
    ADD COLUMN private        boolean NOT NULL DEFAULT false,
    -- Replaces the address's signature on this team's replies when not empty.
    ADD COLUMN signature_html text NOT NULL DEFAULT '',
    -- BusinessHours (contracts/api.md); NULL follows the workspace's.
    ADD COLUMN business_hours jsonb,
    ADD COLUMN updated_at     timestamptz NOT NULL DEFAULT now();

-- lead: manages the team's members and settings. observer: sees the team's tickets but is
-- never auto-assigned.
ALTER TABLE team_members
    ADD COLUMN role       text NOT NULL DEFAULT 'member' CHECK (role IN ('lead', 'member', 'observer')),
    ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX team_members_user_idx ON team_members(user_id);

-- Addresses.
ALTER TABLE mailboxes
    ADD COLUMN brand_id         uuid REFERENCES brands(id) ON DELETE RESTRICT,
    -- Other addresses that deliver into this mailbox; replies can go out as the one written to.
    ADD COLUMN aliases          text[] NOT NULL DEFAULT '{}',
    -- The From name of replies: {agent}, {agent_first}, {team}, {brand}, {mailbox}.
    ADD COLUMN from_name_format text NOT NULL DEFAULT '{agent} at {mailbox}';
UPDATE mailboxes m SET brand_id = b.id FROM brands b WHERE b.workspace_id = m.workspace_id AND b.is_default;
ALTER TABLE mailboxes ALTER COLUMN brand_id SET NOT NULL;

-- Auto-replies are throttled per sender and address, so writing to two teams gets both.
DELETE FROM email_auto_replies;
ALTER TABLE email_auto_replies DROP CONSTRAINT email_auto_replies_pkey;
ALTER TABLE email_auto_replies ADD COLUMN mailbox_id uuid NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE;
ALTER TABLE email_auto_replies ADD PRIMARY KEY (workspace_id, mailbox_id, email);

-- Tickets.
ALTER TABLE tickets
    ADD COLUMN brand_id          uuid REFERENCES brands(id) ON DELETE RESTRICT,
    ADD COLUMN widget_id         uuid REFERENCES widgets(id) ON DELETE SET NULL,
    -- The mailbox and exact address the first message arrived at; mailbox_id is the
    -- address replies go out from and can change.
    ADD COLUMN origin_mailbox_id uuid REFERENCES mailboxes(id) ON DELETE SET NULL,
    ADD COLUMN received_at       text NOT NULL DEFAULT '',
    ADD COLUMN page_url          text NOT NULL DEFAULT '',
    -- Why the ticket is in its team: {via, rule_id?, rule_name?, by?}.
    ADD COLUMN routed            jsonb;
UPDATE tickets t SET brand_id = b.id FROM brands b WHERE b.workspace_id = t.workspace_id AND b.is_default;
UPDATE tickets t SET origin_mailbox_id = t.mailbox_id, received_at = COALESCE(m.email, '')
    FROM mailboxes m WHERE m.id = t.mailbox_id AND t.channel = 'email';
ALTER TABLE tickets ALTER COLUMN brand_id SET NOT NULL;
CREATE INDEX tickets_ws_brand_idx ON tickets(workspace_id, brand_id);
UPDATE tickets t SET widget_id = g.id FROM widgets g WHERE g.workspace_id = t.workspace_id AND t.channel = 'chat';

-- The address a message was sent from (outbound) or received at (inbound).
ALTER TABLE messages ADD COLUMN mailbox_id uuid REFERENCES mailboxes(id) ON DELETE SET NULL;
UPDATE messages ms SET mailbox_id = ie.mailbox_id FROM inbound_emails ie WHERE ie.id = ms.inbound_email_id;

-- Routing rules: the first enabled match, in position order, picks the team.
CREATE TABLE routing_rules (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         text NOT NULL,
    conditions   jsonb NOT NULL DEFAULT '[]',
    match        text NOT NULL DEFAULT 'all' CHECK (match IN ('all', 'any')),
    team_id      uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    enabled      boolean NOT NULL DEFAULT true,
    position     integer NOT NULL DEFAULT 0,
    run_count    bigint NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX routing_rules_ws_idx ON routing_rules(workspace_id, position);

-- Scoping to a team: SLA policies apply to its tickets, saved replies and views are offered
-- to its members.
ALTER TABLE sla_policies ADD COLUMN team_id uuid REFERENCES teams(id) ON DELETE CASCADE;
ALTER TABLE saved_replies ADD COLUMN team_id uuid REFERENCES teams(id) ON DELETE CASCADE;
ALTER TABLE views ADD COLUMN team_id uuid REFERENCES teams(id) ON DELETE CASCADE;

-- Events about a private team's tickets fan out only to that team (contracts/events.md).
ALTER TABLE events ADD COLUMN team_id uuid;

-- Private teams an API key or webhook may see, granted by a member of each.
ALTER TABLE api_keys ADD COLUMN private_team_ids uuid[] NOT NULL DEFAULT '{}';
ALTER TABLE webhooks ADD COLUMN private_team_ids uuid[] NOT NULL DEFAULT '{}';

-- Knowledge base per brand. NULL is shared by every brand.
ALTER TABLE kb_collections ADD COLUMN brand_id uuid REFERENCES brands(id) ON DELETE CASCADE;
ALTER TABLE kb_articles ADD COLUMN brand_id uuid REFERENCES brands(id) ON DELETE CASCADE;

-- Email intake (contracts/api.md "Inbound email").
ALTER TABLE messages
    -- The Reply-To an inbound message named, when it differs from From.
    ADD COLUMN reply_to      text NOT NULL DEFAULT '',
    -- Inbound mail whose sender failed DMARC at a trusted receiving server.
    ADD COLUMN auth_failed   boolean NOT NULL DEFAULT false,
    -- Inbound mail nothing automatic answers: automatic, bulk or list mail, mail from one of
    -- the workspace's own addresses, or mail that failed sender authentication.
    ADD COLUMN no_auto_reply boolean NOT NULL DEFAULT false;
-- A message is the same one again only when its Message-ID and its content match.
-- envelope_from is the SMTP envelope sender when the transport reports it ('' is the null
-- sender of a delivery report), NULL when it does not.
ALTER TABLE inbound_emails
    ADD COLUMN content_hash  text NOT NULL DEFAULT '',
    ADD COLUMN envelope_from text;
ALTER TABLE inbound_emails DROP CONSTRAINT inbound_emails_mailbox_id_message_id_hdr_key;
ALTER TABLE inbound_emails ADD CONSTRAINT inbound_emails_mailbox_msgid_hash_key UNIQUE (mailbox_id, message_id_hdr, content_hash);
CREATE INDEX inbound_emails_ws_msgid_idx ON inbound_emails(workspace_id, message_id_hdr);
