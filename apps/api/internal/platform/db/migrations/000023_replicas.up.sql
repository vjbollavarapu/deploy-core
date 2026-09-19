-- Phase B29: manual replicas — inventory + observed state.

CREATE TABLE application_replicas (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    application_id   UUID NOT NULL REFERENCES applications (id) ON DELETE CASCADE,
    revision_id      UUID REFERENCES revisions (id) ON DELETE SET NULL,
    server_id        UUID REFERENCES servers (id) ON DELETE SET NULL,
    replica_index    INTEGER NOT NULL CHECK (replica_index >= 0),
    container_name   TEXT NOT NULL DEFAULT '',
    container_id     TEXT,
    status           TEXT NOT NULL DEFAULT 'PENDING'
                         CHECK (status IN (
                             'PENDING', 'STARTING', 'RUNNING', 'UNHEALTHY',
                             'STOPPING', 'STOPPED', 'FAILED'
                         )),
    healthy          BOOLEAN NOT NULL DEFAULT FALSE,
    routing_enabled  BOOLEAN NOT NULL DEFAULT FALSE,
    last_probe_at    TIMESTAMPTZ,
    last_error       TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (application_id, replica_index)
);

CREATE INDEX application_replicas_org_app_idx
    ON application_replicas (organization_id, application_id);
CREATE INDEX application_replicas_app_status_idx
    ON application_replicas (application_id, status);
CREATE INDEX application_replicas_revision_idx
    ON application_replicas (revision_id)
    WHERE revision_id IS NOT NULL;

CREATE TRIGGER application_replicas_set_updated_at
    BEFORE UPDATE ON application_replicas
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE application_replicas IS
    'Observed replica slots for an application; desired count lives in application_configs.runtime_config.desiredReplicas';
COMMENT ON COLUMN application_replicas.replica_index IS
    'Zero-based slot index in 0..desiredReplicas-1';
COMMENT ON COLUMN application_replicas.routing_enabled IS
    'When true, Traefik labels should route traffic to this replica';
