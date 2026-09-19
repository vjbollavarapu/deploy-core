-- Phase B19: aggregated application health state + bounded probe samples.

CREATE TABLE application_health (
    application_id        UUID PRIMARY KEY REFERENCES applications (id) ON DELETE CASCADE,
    organization_id       UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    revision_id           UUID REFERENCES revisions (id) ON DELETE SET NULL,
    deployment_id         UUID REFERENCES deployments (id) ON DELETE SET NULL,
    state                 TEXT NOT NULL DEFAULT 'UNKNOWN'
                              CHECK (state IN ('UNKNOWN', 'STARTING', 'HEALTHY', 'DEGRADED', 'UNHEALTHY')),
    consecutive_successes INTEGER NOT NULL DEFAULT 0 CHECK (consecutive_successes >= 0),
    consecutive_failures  INTEGER NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
    last_probe_at         TIMESTAMPTZ,
    last_success_at       TIMESTAMPTZ,
    last_failure_at       TIMESTAMPTZ,
    last_message          TEXT NOT NULL DEFAULT '',
    probe_type            TEXT NOT NULL DEFAULT '',
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX application_health_organization_id_idx ON application_health (organization_id);
CREATE INDEX application_health_state_idx ON application_health (organization_id, state);

CREATE TRIGGER application_health_set_updated_at
    BEFORE UPDATE ON application_health
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE health_probe_samples (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    application_id   UUID NOT NULL REFERENCES applications (id) ON DELETE CASCADE,
    revision_id      UUID REFERENCES revisions (id) ON DELETE SET NULL,
    deployment_id    UUID REFERENCES deployments (id) ON DELETE SET NULL,
    success          BOOLEAN NOT NULL,
    probe_type       TEXT NOT NULL DEFAULT '',
    message          TEXT NOT NULL DEFAULT '',
    latency_ms       INTEGER CHECK (latency_ms IS NULL OR latency_ms >= 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX health_probe_samples_app_created_idx
    ON health_probe_samples (application_id, created_at DESC);
