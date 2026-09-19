-- Phase B21: current/summary metric snapshots (not high-frequency time series).

CREATE TABLE server_metric_snapshots (
    server_id            UUID PRIMARY KEY REFERENCES servers (id) ON DELETE CASCADE,
    organization_id      UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    recorded_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cpu_percent          DOUBLE PRECISION,
    memory_used_bytes    BIGINT,
    memory_total_bytes   BIGINT,
    disk_used_bytes      BIGINT,
    disk_total_bytes     BIGINT,
    load_1               DOUBLE PRECISION,
    load_5               DOUBLE PRECISION,
    load_15              DOUBLE PRECISION,
    uptime_seconds       BIGINT,
    network_rx_bytes     BIGINT,
    network_tx_bytes     BIGINT,
    container_count      INTEGER,
    source               TEXT NOT NULL DEFAULT 'agent'
                             CHECK (source IN ('agent', 'heartbeat')),
    payload              JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX server_metric_snapshots_org_idx
    ON server_metric_snapshots (organization_id);

CREATE TRIGGER server_metric_snapshots_set_updated_at
    BEFORE UPDATE ON server_metric_snapshots
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE container_metric_snapshots (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id      UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    server_id            UUID NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    application_id       UUID REFERENCES applications (id) ON DELETE SET NULL,
    container_id         TEXT NOT NULL,
    container_name       TEXT NOT NULL DEFAULT '',
    recorded_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cpu_percent          DOUBLE PRECISION,
    memory_used_bytes    BIGINT,
    memory_limit_bytes   BIGINT,
    network_rx_bytes     BIGINT,
    network_tx_bytes     BIGINT,
    restart_count        INTEGER,
    status               TEXT NOT NULL DEFAULT 'unknown',
    payload              JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT container_metric_snapshots_server_container_uidx UNIQUE (server_id, container_id)
);

CREATE INDEX container_metric_snapshots_org_idx
    ON container_metric_snapshots (organization_id);
CREATE INDEX container_metric_snapshots_app_idx
    ON container_metric_snapshots (application_id)
    WHERE application_id IS NOT NULL;
CREATE INDEX container_metric_snapshots_server_idx
    ON container_metric_snapshots (server_id, recorded_at DESC);

CREATE TRIGGER container_metric_snapshots_set_updated_at
    BEFORE UPDATE ON container_metric_snapshots
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
