-- Phase B23: managed Docker volumes with attach/detach and delete protection.

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
        'REMOVE_VOLUME',
        'ATTACH_VOLUME',
        'DETACH_VOLUME',
        'INSPECT_VOLUME',
        'RUN_HEALTH_CHECK',
        'CREATE_BACKUP',
        'RESTORE_BACKUP',
        'PROVISION_DATABASE',
        'START_DATABASE',
        'STOP_DATABASE'
    )
);

CREATE TABLE volumes (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id          UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    server_id                UUID NOT NULL REFERENCES servers (id) ON DELETE RESTRICT,
    name                     TEXT NOT NULL,
    driver                   TEXT NOT NULL DEFAULT 'local',
    mount_path               TEXT NOT NULL DEFAULT '',
    state                    TEXT NOT NULL DEFAULT 'PENDING'
                                 CHECK (state IN (
                                     'PENDING', 'CREATING', 'READY', 'ATTACHED',
                                     'DETACHING', 'DELETING', 'FAILED', 'DELETED'
                                 )),
    attached_resource_type   TEXT
                                 CHECK (attached_resource_type IS NULL OR attached_resource_type IN (
                                     'database', 'application'
                                 )),
    attached_resource_id     UUID,
    backup_policy            JSONB NOT NULL DEFAULT '{}'::jsonb,
    protected                BOOLEAN NOT NULL DEFAULT FALSE,
    docker_name              TEXT,
    usage_bytes              BIGINT,
    labels                   JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_command_id          UUID REFERENCES agent_commands (id) ON DELETE SET NULL,
    last_error               TEXT NOT NULL DEFAULT '',
    created_by               UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at               TIMESTAMPTZ,
    CONSTRAINT volumes_attached_pair_chk CHECK (
        (attached_resource_type IS NULL AND attached_resource_id IS NULL)
        OR (attached_resource_type IS NOT NULL AND attached_resource_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX volumes_server_name_active_uidx
    ON volumes (server_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX volumes_org_idx ON volumes (organization_id) WHERE deleted_at IS NULL;
CREATE INDEX volumes_server_idx ON volumes (server_id) WHERE deleted_at IS NULL;
CREATE INDEX volumes_attached_idx
    ON volumes (attached_resource_type, attached_resource_id)
    WHERE deleted_at IS NULL AND attached_resource_id IS NOT NULL;

CREATE TRIGGER volumes_set_updated_at
    BEFORE UPDATE ON volumes
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
