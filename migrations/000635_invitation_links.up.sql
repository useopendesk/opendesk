-- Invitation links an admin can copy and resend (contracts/api.md "Invitations"). The token
-- is kept sealed (AES-GCM, d.Box) beside its hash; link_copied_at marks a link that left
-- opendesk by another way than the invitation email, so signing up with it does not prove
-- the address.
ALTER TABLE invitations
    ADD COLUMN token_enc      bytea,
    ADD COLUMN link_copied_at timestamptz;
