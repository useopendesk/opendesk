-- Website contact forms (contracts/api.md "Inbound email"). A mailbox lists the addresses
-- (or "@domain" entries) its website forms send from; an authenticated message from one
-- of them is from the visitor its Reply-To names.
ALTER TABLE mailboxes ADD COLUMN trusted_form_senders text[] NOT NULL DEFAULT '{}';
-- The trusted form address an inbound message came through; '' for any other message.
ALTER TABLE messages ADD COLUMN form_sender text NOT NULL DEFAULT '';
