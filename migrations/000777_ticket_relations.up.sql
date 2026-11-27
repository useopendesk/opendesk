-- Tickets that belong together without being merged: each keeps its own status, assignee,
-- team, SLA and timeline. One row per pair, read from either side.
CREATE TABLE ticket_relations (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id      uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    ticket_id         uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    related_ticket_id uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    created_by        uuid REFERENCES users(id) ON DELETE SET NULL,
    actor_type        text NOT NULL DEFAULT 'user' CHECK (actor_type IN ('user', 'ai', 'api')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    CHECK (ticket_id <> related_ticket_id)
);
CREATE UNIQUE INDEX ticket_relations_pair_uq ON ticket_relations
    (workspace_id, LEAST(ticket_id, related_ticket_id), GREATEST(ticket_id, related_ticket_id));
CREATE INDEX ticket_relations_ticket_idx ON ticket_relations (workspace_id, ticket_id);
CREATE INDEX ticket_relations_related_idx ON ticket_relations (workspace_id, related_ticket_id);
