-- Puts back the fields bots replaced: a widget's bot_name, the workspace's agent name,
-- instructions and widget_bot picture (from the default bot). Bot looks, personalities
-- other than the instructions, and pictures of other bots are dropped.

UPDATE widgets g SET settings = g.settings || jsonb_build_object('bot_name', b.name)
FROM bots b
WHERE b.id = g.bot_id AND NOT b.is_default;

UPDATE workspaces w SET
    settings = jsonb_set(jsonb_set(w.settings, '{widget}', COALESCE(w.settings -> 'widget', '{}') || jsonb_build_object('bot_name',
        CASE WHEN b.name = 'Ollie' THEN '' ELSE b.name END)),
        '{ai}', COALESCE(w.settings -> 'ai', '{}') || jsonb_build_object('widget_instructions', COALESCE(b.personality ->> 'instructions', ''))),
    brand_images = CASE WHEN b.avatar LIKE 'img:%' THEN w.brand_images || jsonb_build_object('widget_bot', b.avatar) ELSE w.brand_images END
FROM bots b
WHERE b.workspace_id = w.id AND b.is_default;

UPDATE images i SET owner_type = 'workspace', owner_id = b.workspace_id, slot = 'widget_bot'
FROM bots b
WHERE i.owner_type = 'bot' AND i.owner_id = b.id AND b.is_default AND b.avatar = 'img:' || i.id::text;
-- Pictures of other bots: the rows go (the stored objects are left behind).
DELETE FROM images WHERE owner_type = 'bot';

ALTER TABLE images DROP CONSTRAINT images_owner_type_check;
ALTER TABLE images ADD CONSTRAINT images_owner_type_check
    CHECK (owner_type IN ('user', 'contact', 'company', 'workspace', 'brand', 'widget'));

ALTER TABLE messages DROP COLUMN bot_id;
ALTER TABLE mailboxes DROP COLUMN bot_id;
ALTER TABLE widgets DROP COLUMN bot_id;
DROP TABLE bots;
