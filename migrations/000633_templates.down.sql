DROP TABLE IF EXISTS email_templates;
ALTER TABLE members
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS phone,
    DROP COLUMN IF EXISTS pronouns,
    DROP COLUMN IF EXISTS signature_html;
