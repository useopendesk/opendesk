DROP INDEX inbound_emails_ws_msgid_idx;
DELETE FROM inbound_emails a USING inbound_emails b
    WHERE a.mailbox_id = b.mailbox_id AND a.message_id_hdr = b.message_id_hdr AND (a.received_at, a.id) > (b.received_at, b.id);
ALTER TABLE inbound_emails DROP CONSTRAINT inbound_emails_mailbox_msgid_hash_key;
ALTER TABLE inbound_emails ADD CONSTRAINT inbound_emails_mailbox_id_message_id_hdr_key UNIQUE (mailbox_id, message_id_hdr);
ALTER TABLE inbound_emails DROP COLUMN envelope_from, DROP COLUMN content_hash;
ALTER TABLE messages DROP COLUMN no_auto_reply, DROP COLUMN auth_failed, DROP COLUMN reply_to;
ALTER TABLE kb_articles DROP COLUMN brand_id;
ALTER TABLE kb_collections DROP COLUMN brand_id;
ALTER TABLE webhooks DROP COLUMN private_team_ids;
ALTER TABLE api_keys DROP COLUMN private_team_ids;
ALTER TABLE events DROP COLUMN team_id;
ALTER TABLE views DROP COLUMN team_id;
ALTER TABLE saved_replies DROP COLUMN team_id;
ALTER TABLE sla_policies DROP COLUMN team_id;
DROP TABLE routing_rules;
ALTER TABLE messages DROP COLUMN mailbox_id;
ALTER TABLE tickets
    DROP COLUMN routed,
    DROP COLUMN page_url,
    DROP COLUMN received_at,
    DROP COLUMN origin_mailbox_id,
    DROP COLUMN widget_id,
    DROP COLUMN brand_id;
DELETE FROM email_auto_replies;
ALTER TABLE email_auto_replies DROP CONSTRAINT email_auto_replies_pkey;
ALTER TABLE email_auto_replies DROP COLUMN mailbox_id;
ALTER TABLE email_auto_replies ADD PRIMARY KEY (workspace_id, email);
ALTER TABLE mailboxes
    DROP COLUMN from_name_format,
    DROP COLUMN aliases,
    DROP COLUMN brand_id;
DROP INDEX team_members_user_idx;
ALTER TABLE team_members DROP COLUMN created_at, DROP COLUMN role;
ALTER TABLE teams
    DROP COLUMN updated_at,
    DROP COLUMN business_hours,
    DROP COLUMN signature_html,
    DROP COLUMN private,
    DROP COLUMN color,
    DROP COLUMN description;
DELETE FROM images WHERE owner_type IN ('brand', 'widget');
ALTER TABLE images DROP CONSTRAINT images_owner_type_check;
ALTER TABLE images ADD CONSTRAINT images_owner_type_check
    CHECK (owner_type IN ('user', 'contact', 'company', 'workspace'));
ALTER TABLE widget_sessions DROP COLUMN widget_id;
DROP TABLE widgets;
DROP TABLE brands;
