-- Native apps (contracts/native.md): the client a session belongs to, App Attest keys and
-- challenges (the iPhone stand-in for Turnstile), and push devices bound to a session.

ALTER TABLE sessions ADD COLUMN client text NOT NULL DEFAULT 'web'
    CHECK (client IN ('web', 'desktop', 'ios'));

-- An App Attest key attested by Apple once per install. counter only moves forward.
CREATE TABLE app_attest_keys (
    key_id       text PRIMARY KEY,
    app_id       text NOT NULL,
    public_key   bytea NOT NULL,
    counter      bigint NOT NULL DEFAULT 0,
    environment  text NOT NULL CHECK (environment IN ('production', 'development')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);

-- Single-use App Attest challenges, stored hashed.
CREATE TABLE app_attest_challenges (
    challenge_hash bytea PRIMARY KEY,
    expires_at     timestamptz NOT NULL
);
CREATE INDEX app_attest_challenges_expires_idx ON app_attest_challenges(expires_at);

-- A phone that receives pushes. It lives exactly as long as the session that registered
-- it. The APNs token (direct) or relay token (relay) is hashed for lookup and sealed for
-- sending.
CREATE TABLE push_devices (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id   uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    platform     text NOT NULL DEFAULT 'ios' CHECK (platform IN ('ios')),
    mode         text NOT NULL CHECK (mode IN ('direct', 'relay')),
    token_hash   bytea NOT NULL UNIQUE,
    token_sealed bytea NOT NULL,
    environment  text NOT NULL CHECK (environment IN ('production', 'sandbox')),
    name         text NOT NULL DEFAULT '',
    app_version  text NOT NULL DEFAULT '',
    muted_types  text[] NOT NULL DEFAULT '{}',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    last_push_at timestamptz
);
CREATE INDEX push_devices_user_idx ON push_devices(user_id, created_at DESC);
CREATE INDEX push_devices_session_idx ON push_devices(session_id);
