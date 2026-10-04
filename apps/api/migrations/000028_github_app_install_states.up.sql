-- Single-use GitHub App installation state. The raw state is not stored; only its hash.

CREATE TABLE github_app_install_states (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    state_hash       TEXT NOT NULL,
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id          UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at       TIMESTAMPTZ NOT NULL,
    consumed_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT github_app_install_states_hash_uidx UNIQUE (state_hash)
);

CREATE INDEX github_app_install_states_pending_expires_idx
    ON github_app_install_states (expires_at)
    WHERE consumed_at IS NULL;
