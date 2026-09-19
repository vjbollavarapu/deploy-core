-- Phase B22: managed database resources (initial engine: PostgreSQL).
-- Persistent data volumes are independent of disposable containers; never
-- auto-delete volumes when a database container is recreated or the row is soft-deleted.

ALTER TABLE agent_commands DROP CONSTRAINT IF EXISTS agent_commands_operation_check;
ALTER TABLE agent_commands ADD CONSTRAINT agent_commands_operation_check CHECK (
    operation IN (
        'DEPLOY_REVISION',
        'STOP_CONTAINER',
        'START_CONTAINER',
        'RESTART_CONTAINER',
        'REMOVE_CONTAINER',
        'FETCH_LOGS',
        'STREAM_LOGS',
        'BUILD_IMAGE',
        'PULL_IMAGE',
        'CREATE_NETWORK',
        'CREATE_VOLUME',
        'RUN_HEALTH_CHECK',
        'CREATE_BACKUP',
        'RESTORE_BACKUP',
        'PROVISION_DATABASE',
        'START_DATABASE',
        'STOP_DATABASE'
    )
);

CREATE TABLE managed_databases (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    project_id              UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    environment_id          UUID NOT NULL REFERENCES environments (id) ON DELETE CASCADE,
    server_id               UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT,
    name                    TEXT NOT NULL,
    engine                  TEXT NOT NULL DEFAULT 'postgresql'
                                CHECK (engine IN ('postgresql')),
    engine_version          TEXT NOT NULL DEFAULT '16',
    database_name           TEXT NOT NULL,
    username                TEXT NOT NULL,
    credential_ciphertext   BYTEA NOT NULL,
    credential_nonce        BYTEA NOT NULL,
    credential_key_id       TEXT NOT NULL,
    credential_algorithm    TEXT NOT NULL DEFAULT 'AES-256-GCM',
    storage_volume_name     TEXT NOT NULL,
    volume_protected        BOOLEAN NOT NULL DEFAULT TRUE,
    cpu_millis              INTEGER CHECK (cpu_millis IS NULL OR cpu_millis > 0),
    memory_bytes            BIGINT CHECK (memory_bytes IS NULL OR memory_bytes > 0),
    container_runtime_id    TEXT,
    status                  TEXT NOT NULL DEFAULT 'PENDING'
                                CHECK (status IN (
                                    'PENDING', 'PROVISIONING', 'RUNNING', 'STOPPED',
                                    'DEGRADED', 'FAILED', 'DELETING', 'DELETED'
                                )),
    backup_policy           JSONB NOT NULL DEFAULT '{}'::jsonb,
    provision_command_id    UUID REFERENCES agent_commands (id) ON DELETE SET NULL,
    last_error              TEXT NOT NULL DEFAULT '',
    created_by              UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ
);

CREATE UNIQUE INDEX managed_databases_org_name_active_uidx
    ON managed_databases (organization_id, name)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX managed_databases_server_volume_active_uidx
    ON managed_databases (server_id, storage_volume_name)
    WHERE deleted_at IS NULL;

CREATE INDEX managed_databases_org_idx ON managed_databases (organization_id)
    WHERE deleted_at IS NULL;
CREATE INDEX managed_databases_project_env_idx
    ON managed_databases (project_id, environment_id)
    WHERE deleted_at IS NULL;
CREATE INDEX managed_databases_server_idx ON managed_databases (server_id)
    WHERE deleted_at IS NULL;

CREATE TRIGGER managed_databases_set_updated_at
    BEFORE UPDATE ON managed_databases
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
