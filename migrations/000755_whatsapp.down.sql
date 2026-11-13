UPDATE messages SET delivery_status = 'sent' WHERE delivery_status IN ('delivered', 'read');
ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_delivery_status_check;
ALTER TABLE messages ADD CONSTRAINT messages_delivery_status_check
    CHECK (delivery_status IN ('none', 'pending', 'sent', 'failed', 'bounced'));

DELETE FROM tickets WHERE channel = 'whatsapp';
ALTER TABLE tickets DROP CONSTRAINT IF EXISTS tickets_channel_check;
ALTER TABLE tickets ADD CONSTRAINT tickets_channel_check CHECK (channel IN ('email', 'chat', 'api', 'portal', 'phone'));

DROP TABLE IF EXISTS whatsapp_messages;
DROP TABLE IF EXISTS whatsapp_inbound_events;
DROP TABLE IF EXISTS whatsapp_connections;
