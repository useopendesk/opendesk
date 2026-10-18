-- Integration icons go: the rows go (the stored objects are left behind).
DELETE FROM images WHERE owner_type = 'integration';

ALTER TABLE images DROP CONSTRAINT images_owner_type_check;
ALTER TABLE images ADD CONSTRAINT images_owner_type_check
    CHECK (owner_type IN ('user', 'contact', 'company', 'workspace', 'brand', 'widget', 'bot'));

ALTER TABLE integrations DROP COLUMN IF EXISTS icon;
