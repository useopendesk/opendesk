-- Aliases with their own team (contracts/teams.md "Routing"): new tickets written to the alias
-- go to its team instead of the mailbox's. {"<alias>": "<team id>"}; a key is one of `aliases`.
ALTER TABLE mailboxes ADD COLUMN alias_teams jsonb NOT NULL DEFAULT '{}';

-- The address of ours each inbound email was written to: the mailbox's address or an alias.
ALTER TABLE messages ADD COLUMN received_at text NOT NULL DEFAULT '';

-- Ticket lists filter on the address a ticket was written to, compared lowercased.
UPDATE tickets SET received_at = lower(received_at) WHERE received_at <> lower(received_at);
CREATE INDEX tickets_ws_received_at_idx ON tickets(workspace_id, received_at) WHERE received_at <> '' AND deleted_at IS NULL;
