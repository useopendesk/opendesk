-- Who granted an API key or webhook each of its private teams (contracts/teams.md
-- "Visibility"). A grant lasts while its granter is a lead, member or observer of the team:
-- the foreign key to team_members holds at commit, so removing someone from a team also
-- removes their grants (and the team from the key's or webhook's private_team_ids) in the
-- same transaction. private_team_ids stays the effective list the read paths use.
CREATE TABLE private_team_grants (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    team_id      uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    api_key_id   uuid REFERENCES api_keys(id) ON DELETE CASCADE,
    webhook_id   uuid REFERENCES webhooks(id) ON DELETE CASCADE,
    granted_by   uuid NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT private_team_grants_one_owner CHECK (num_nonnulls(api_key_id, webhook_id) = 1),
    CONSTRAINT private_team_grants_granter_on_team FOREIGN KEY (team_id, granted_by)
        REFERENCES team_members (team_id, user_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE UNIQUE INDEX private_team_grants_key_team_idx ON private_team_grants (api_key_id, team_id) WHERE api_key_id IS NOT NULL;
CREATE UNIQUE INDEX private_team_grants_webhook_team_idx ON private_team_grants (webhook_id, team_id) WHERE webhook_id IS NOT NULL;
CREATE INDEX private_team_grants_granter_idx ON private_team_grants (team_id, granted_by);
CREATE INDEX private_team_grants_ws_idx ON private_team_grants (workspace_id);

-- Existing grants were made by the key's or webhook's creator. Each one is kept while that
-- creator is on the team; every other team leaves private_team_ids.
INSERT INTO private_team_grants (workspace_id, team_id, api_key_id, granted_by, created_at)
SELECT DISTINCT k.workspace_id, t.id, k.id, k.created_by, k.created_at
FROM api_keys k
CROSS JOIN LATERAL unnest(k.private_team_ids) AS g(team_id)
JOIN teams t ON t.id = g.team_id AND t.workspace_id = k.workspace_id AND t.private
JOIN team_members tm ON tm.team_id = t.id AND tm.user_id = k.created_by
ON CONFLICT (api_key_id, team_id) WHERE api_key_id IS NOT NULL DO NOTHING;

INSERT INTO private_team_grants (workspace_id, team_id, webhook_id, granted_by, created_at)
SELECT DISTINCT w.workspace_id, t.id, w.id, w.created_by, w.created_at
FROM webhooks w
CROSS JOIN LATERAL unnest(w.private_team_ids) AS g(team_id)
JOIN teams t ON t.id = g.team_id AND t.workspace_id = w.workspace_id AND t.private
JOIN team_members tm ON tm.team_id = t.id AND tm.user_id = w.created_by
ON CONFLICT (webhook_id, team_id) WHERE webhook_id IS NOT NULL DO NOTHING;

UPDATE api_keys k SET private_team_ids = ARRAY(
    SELECT g.team_id FROM private_team_grants g WHERE g.api_key_id = k.id ORDER BY g.team_id)
WHERE k.private_team_ids <> '{}';

UPDATE webhooks w SET private_team_ids = ARRAY(
    SELECT g.team_id FROM private_team_grants g WHERE g.webhook_id = w.id ORDER BY g.team_id)
WHERE w.private_team_ids <> '{}';
