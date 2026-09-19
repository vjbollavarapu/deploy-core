-- Phase B8: transport-agnostic, versionable agent command queue.

CREATE TABLE agent_commands (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    server_id        UUID NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    operation        TEXT NOT NULL
                         CHECK (operation IN (
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
                             'RESTORE_BACKUP'
                         )),
    schema_version   INTEGER NOT NULL DEFAULT 1 CHECK (schema_version > 0),
    payload          JSONB NOT NULL DEFAULT '{}'::jsonb,
    status           TEXT NOT NULL DEFAULT 'pending'
                         CHECK (status IN (
                             'pending', 'accepted', 'running', 'completed', 'failed', 'expired', 'cancelled'
                         )),
    issued_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at       TIMESTAMPTZ NOT NULL,
    request_id       TEXT,
    correlation_id   TEXT,
    issued_by        UUID REFERENCES users (id) ON DELETE SET NULL,
    result           JSONB,
    error_code       TEXT,
    error_message    TEXT,
    accepted_at      TIMESTAMPTZ,
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT agent_commands_no_shell_payload CHECK (
        NOT (payload ? 'shell')
        AND NOT (payload ? 'command')
        AND NOT (payload ? 'script')
        AND NOT (payload ? 'exec')
    )
);

CREATE INDEX agent_commands_server_pending_idx
    ON agent_commands (server_id, issued_at)
    WHERE status = 'pending';

CREATE INDEX agent_commands_org_issued_idx
    ON agent_commands (organization_id, issued_at DESC);

CREATE INDEX agent_commands_correlation_idx
    ON agent_commands (correlation_id)
    WHERE correlation_id IS NOT NULL;

CREATE TRIGGER agent_commands_set_updated_at
    BEFORE UPDATE ON agent_commands
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
