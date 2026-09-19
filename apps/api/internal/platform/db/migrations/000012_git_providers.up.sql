-- Phase B16: git provider repositories, webhook deliveries, auto-deploy, encrypted webhook secrets.

ALTER TABLE git_connections
    ADD COLUMN IF NOT EXISTS webhook_secret_ciphertext BYTEA,
    ADD COLUMN IF NOT EXISTS webhook_secret_nonce BYTEA,
    ADD COLUMN IF NOT EXISTS webhook_secret_key_id TEXT,
    ADD COLUMN IF NOT EXISTS credential_algorithm TEXT NOT NULL DEFAULT 'AES-256-GCM';

ALTER TABLE application_configs
    ADD COLUMN IF NOT EXISTS auto_deploy_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS git_connection_id UUID REFERENCES git_connections (id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS application_configs_git_connection_id_idx
    ON application_configs (git_connection_id)
    WHERE git_connection_id IS NOT NULL;

CREATE TABLE git_repositories (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    connection_id    UUID NOT NULL REFERENCES git_connections (id) ON DELETE CASCADE,
    external_id      TEXT NOT NULL DEFAULT '',
    full_name        TEXT NOT NULL,
    default_branch   TEXT NOT NULL DEFAULT 'main',
    clone_url        TEXT NOT NULL DEFAULT '',
    html_url         TEXT NOT NULL DEFAULT '',
    metadata         JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_sync_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT git_repositories_connection_full_name_uidx UNIQUE (connection_id, full_name)
);

CREATE INDEX git_repositories_organization_id_idx ON git_repositories (organization_id);
CREATE INDEX git_repositories_connection_id_idx ON git_repositories (connection_id);

CREATE TRIGGER git_repositories_set_updated_at
    BEFORE UPDATE ON git_repositories
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE git_webhook_deliveries (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    connection_id    UUID NOT NULL REFERENCES git_connections (id) ON DELETE CASCADE,
    provider         TEXT NOT NULL,
    delivery_id      TEXT NOT NULL,
    event_type       TEXT NOT NULL,
    repository_full_name TEXT NOT NULL DEFAULT '',
    branch           TEXT NOT NULL DEFAULT '',
    commit_sha       TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'received'
                         CHECK (status IN ('received', 'processed', 'ignored', 'duplicate', 'failed')),
    deployment_ids   JSONB NOT NULL DEFAULT '[]'::jsonb,
    error_message    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT git_webhook_deliveries_connection_delivery_uidx UNIQUE (connection_id, delivery_id)
);

CREATE INDEX git_webhook_deliveries_organization_id_idx
    ON git_webhook_deliveries (organization_id, created_at DESC);
