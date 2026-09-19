-- Phase B24: PostgreSQL logical backups + restore operations.

CREATE TABLE backups (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id      UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    server_id            UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT,
    resource_type        TEXT NOT NULL DEFAULT 'database'
                             CHECK (resource_type IN ('database')),
    resource_id          UUID NOT NULL,
    type                 TEXT NOT NULL DEFAULT 'postgresql_logical'
                             CHECK (type IN ('postgresql_logical')),
    status               TEXT NOT NULL DEFAULT 'PENDING'
                             CHECK (status IN (
                                 'PENDING', 'QUEUED', 'RUNNING', 'SUCCEEDED',
                                 'FAILED', 'EXPIRED', 'DELETED'
                             )),
    started_at           TIMESTAMPTZ,
    completed_at         TIMESTAMPTZ,
    duration_ms          BIGINT,
    size_bytes           BIGINT,
    checksum             TEXT NOT NULL DEFAULT '',
    destination_type     TEXT NOT NULL DEFAULT 'local'
                             CHECK (destination_type IN ('local', 's3')),
    destination_uri      TEXT NOT NULL DEFAULT '',
    retention_until      TIMESTAMPTZ,
    job_id               UUID REFERENCES jobs (id) ON DELETE SET NULL,
    command_id           UUID REFERENCES agent_commands (id) ON DELETE SET NULL,
    last_error           TEXT NOT NULL DEFAULT '',
    metadata             JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by           UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ
);

CREATE INDEX backups_org_created_idx ON backups (organization_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX backups_resource_idx ON backups (resource_type, resource_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX backups_server_idx ON backups (server_id)
    WHERE deleted_at IS NULL;
CREATE INDEX backups_status_idx ON backups (organization_id, status)
    WHERE deleted_at IS NULL;

CREATE TRIGGER backups_set_updated_at
    BEFORE UPDATE ON backups
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE restore_operations (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id        UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    backup_id              UUID NOT NULL REFERENCES backups (id) ON DELETE RESTRICT,
    target_resource_type   TEXT NOT NULL DEFAULT 'database'
                               CHECK (target_resource_type IN ('database')),
    target_resource_id     UUID NOT NULL,
    server_id              UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT,
    status                 TEXT NOT NULL DEFAULT 'PENDING'
                               CHECK (status IN (
                                   'PENDING', 'QUEUED', 'RUNNING', 'VALIDATING',
                                   'SUCCEEDED', 'FAILED', 'CANCELLED'
                               )),
    started_at             TIMESTAMPTZ,
    completed_at           TIMESTAMPTZ,
    duration_ms            BIGINT,
    job_id                 UUID REFERENCES jobs (id) ON DELETE SET NULL,
    command_id             UUID REFERENCES agent_commands (id) ON DELETE SET NULL,
    validation_passed      BOOLEAN,
    last_error             TEXT NOT NULL DEFAULT '',
    metadata               JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by             UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX restore_operations_org_created_idx
    ON restore_operations (organization_id, created_at DESC);
CREATE INDEX restore_operations_backup_idx ON restore_operations (backup_id);
CREATE INDEX restore_operations_target_idx
    ON restore_operations (target_resource_type, target_resource_id);

CREATE TRIGGER restore_operations_set_updated_at
    BEFORE UPDATE ON restore_operations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
