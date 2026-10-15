-- Bots: the AI agent's customer-facing identities (contracts/bots.md). Each workspace has
-- exactly one default bot; widgets and mailboxes answer as their bot_id, NULL meaning the
-- default. The default bot is built from what the workspace had (bot_name, the widget_bot
-- picture, the agent instructions and tone), so existing workspaces look and answer the
-- same; those old fields are then removed.

CREATE TABLE bots (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         text NOT NULL,
    role         text NOT NULL DEFAULT '',
    -- BCP 47 tag the bot always writes in; '' = the customer's language.
    language     text NOT NULL DEFAULT '',
    -- BotLook (contracts/bots.md), validated against an allow-list before it is stored.
    look         jsonb NOT NULL,
    -- The uploaded picture, "img:<image id>" (images, owner_type 'bot'), or ''.
    avatar       text NOT NULL DEFAULT '',
    -- BotPersonality (contracts/bots.md).
    personality  jsonb NOT NULL DEFAULT '{}',
    is_default   boolean NOT NULL DEFAULT false,
    position     integer NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX bots_ws_idx ON bots (workspace_id, position);
CREATE UNIQUE INDEX bots_default_idx ON bots (workspace_id) WHERE is_default;

ALTER TABLE widgets ADD COLUMN bot_id uuid REFERENCES bots(id) ON DELETE SET NULL;
ALTER TABLE mailboxes ADD COLUMN bot_id uuid REFERENCES bots(id) ON DELETE SET NULL;
-- The bot an AI reply was sent as; the reply keeps it when the widget or mailbox changes bot.
ALTER TABLE messages ADD COLUMN bot_id uuid REFERENCES bots(id) ON DELETE SET NULL;
CREATE INDEX messages_bot_idx ON messages (bot_id) WHERE bot_id IS NOT NULL;
CREATE INDEX widgets_bot_idx ON widgets (bot_id) WHERE bot_id IS NOT NULL;
CREATE INDEX mailboxes_bot_idx ON mailboxes (bot_id) WHERE bot_id IS NOT NULL;

ALTER TABLE images DROP CONSTRAINT images_owner_type_check;
ALTER TABLE images ADD CONSTRAINT images_owner_type_check
    CHECK (owner_type IN ('user', 'contact', 'company', 'workspace', 'brand', 'widget', 'bot'));

-- The default bot of every workspace.
INSERT INTO bots (workspace_id, name, look, avatar, personality, is_default, position)
SELECT w.id,
    COALESCE(NULLIF(left(btrim(w.settings -> 'widget' ->> 'bot_name'), 40), ''), 'Ollie'),
    jsonb_build_object('kind', CASE WHEN COALESCE(w.brand_images ->> 'widget_bot', '') LIKE 'img:%' THEN 'image' ELSE 'built' END,
        'shape', 'blob', 'fill', jsonb_build_object('type', 'gradient', 'color', '#c9bcff', 'to', '#4a2bc4'),
        'eyes', 'pills', 'mouth', 'none', 'accessory', 'none', 'accent', '#f59e0b', 'blush', false,
        'background', 'none', 'emoji', ''),
    CASE WHEN COALESCE(w.brand_images ->> 'widget_bot', '') LIKE 'img:%' THEN w.brand_images ->> 'widget_bot' ELSE '' END,
    jsonb_build_object(
        'tone', CASE WHEN COALESCE(btrim(w.settings -> 'ai' ->> 'tone'), '') = '' THEN 'friendly'
            WHEN lower(btrim(w.settings -> 'ai' ->> 'tone')) IN ('friendly', 'professional', 'concise') THEN lower(btrim(w.settings -> 'ai' ->> 'tone'))
            ELSE 'custom' END,
        'custom_tone', CASE WHEN lower(COALESCE(btrim(w.settings -> 'ai' ->> 'tone'), '')) IN ('', 'friendly', 'professional', 'concise') THEN ''
            ELSE left(btrim(w.settings -> 'ai' ->> 'tone'), 200) END,
        'instructions', left(COALESCE(w.settings -> 'ai' ->> 'widget_instructions', ''), 4000),
        'reply_length', 'short', 'emoji', 'none', 'greeting', '', 'handoff_message', '',
        'blocked_topics', '[]'::jsonb, 'sign_off', ''),
    true, 0
FROM workspaces w;

-- The widget_bot picture now belongs to the default bot.
UPDATE images i SET owner_type = 'bot', owner_id = b.id, slot = 'avatar'
FROM bots b
WHERE b.is_default AND b.avatar LIKE 'img:%' AND i.id::text = substring(b.avatar FROM 5)
    AND i.workspace_id = b.workspace_id;

-- A widget named its agent differently: one bot per distinct name, a copy of the default.
INSERT INTO bots (workspace_id, name, look, personality, position)
SELECT n.workspace_id, n.name, jsonb_set(d.look, '{kind}', '"built"'), d.personality, n.rank
FROM (
    SELECT g.workspace_id, left(btrim(g.settings ->> 'bot_name'), 40) AS name,
        row_number() OVER (PARTITION BY g.workspace_id ORDER BY min(g.created_at), left(btrim(g.settings ->> 'bot_name'), 40))::int AS rank
    FROM widgets g
    WHERE COALESCE(btrim(g.settings ->> 'bot_name'), '') <> ''
    GROUP BY g.workspace_id, left(btrim(g.settings ->> 'bot_name'), 40)
) n
JOIN bots d ON d.workspace_id = n.workspace_id AND d.is_default
WHERE n.name <> d.name;

UPDATE widgets g SET bot_id = b.id
FROM bots b
WHERE b.workspace_id = g.workspace_id AND NOT b.is_default
    AND b.name = left(btrim(g.settings ->> 'bot_name'), 40);

-- Replies the agent already sent were sent as these bots.
UPDATE messages m SET bot_id = COALESCE(g.bot_id, d.id)
FROM tickets t
JOIN bots d ON d.workspace_id = t.workspace_id AND d.is_default
LEFT JOIN widgets g ON g.id = t.widget_id
WHERE m.ai AND t.workspace_id = m.workspace_id AND t.id = m.ticket_id;

-- The old fields are gone: nothing reads them any more.
UPDATE widgets SET settings = settings - 'bot_name' WHERE settings ? 'bot_name';
UPDATE workspaces SET
    settings = (settings #- '{widget,bot_name}') #- '{ai,widget_instructions}',
    brand_images = brand_images - 'widget_bot'
WHERE settings -> 'widget' ? 'bot_name' OR settings -> 'ai' ? 'widget_instructions' OR brand_images ? 'widget_bot';
