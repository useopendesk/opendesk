-- Help center custom domains (contracts/teams.md "Help center addresses"): at most one per
-- brand. A hostname may be claimed by several brands while unverified; only one claim is
-- ever verified, and only a verified one is served.
CREATE TABLE help_domains (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    brand_id      uuid NOT NULL UNIQUE REFERENCES brands(id) ON DELETE CASCADE,
    hostname      text NOT NULL CHECK (hostname = lower(hostname) AND length(hostname) BETWEEN 4 AND 253),
    -- The value of the _opendesk-verify TXT record, new for every claim.
    token         text NOT NULL,
    status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'failed')),
    problem       text NOT NULL DEFAULT '',
    txt_ok        boolean NOT NULL DEFAULT false,
    cname_ok      boolean NOT NULL DEFAULT false,
    -- The Cloudflare custom hostname, when the instance uses Cloudflare for SaaS.
    provider_id   text,
    -- When the current run of checks began (the claim, or a manual check of a failed domain).
    pending_since timestamptz NOT NULL DEFAULT now(),
    next_check_at timestamptz NOT NULL DEFAULT now(),
    checked_at    timestamptz,
    verified_at   timestamptz,
    active_at     timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX help_domains_verified_uq ON help_domains (hostname) WHERE verified_at IS NOT NULL;
CREATE INDEX help_domains_hostname ON help_domains (hostname);
CREATE INDEX help_domains_workspace ON help_domains (workspace_id);
CREATE INDEX help_domains_due ON help_domains (next_check_at) WHERE status <> 'failed';
