-- Phase B26: outgoing webhook endpoints and delivery attempts.

CREATE TABLE outgoing_webhooks (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name                    TEXT NOT NULL,
    url                     TEXT NOT NULL,
    events                  JSONB NOT NULL DEFAULT '[]'::jsonb,
    secret_ciphertext       BYTEA NOT NULL,
    secret_nonce            BYTEA NOT NULL,
    secret_key_id           TEXT NOT NULL,
    secret_algorithm        TEXT NOT NULL DEFAULT 'AES-256-GCM',
    enabled                 BOOLEAN NOT NULL DEFAULT TRUE,
    status                  TEXT NOT NULL DEFAULT 'ACTIVE'
                                CHECK (status IN ('ACTIVE', 'DISABLED', 'FAILING')),
    consecutive_failures    INTEGER NOT NULL DEFAULT 0,
    failure_threshold       INTEGER NOT NULL DEFAULT 5
                                CHECK (failure_threshold >= 1 AND failure_threshold <= 100),
    last_error              TEXT NOT NULL DEFAULT '',
    disabled_at             TIMESTAMPTZ,
    created_by              UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ
);

CREATE UNIQUE INDEX outgoing_webhooks_org_name_active_uidx
    ON outgoing_webhooks (organization_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX outgoing_webhooks_org_idx
    ON outgoing_webhooks (organization_id)
    WHERE deleted_at IS NULL;

CREATE TRIGGER outgoing_webhooks_set_updated_at
    BEFORE UPDATE ON outgoing_webhooks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE outgoing_webhook_deliveries (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    webhook_id              UUID NOT NULL REFERENCES outgoing_webhooks (id) ON DELETE CASCADE,
    event_type              TEXT NOT NULL,
    payload                 JSONB NOT NULL DEFAULT '{}'::jsonb,
    status                  TEXT NOT NULL DEFAULT 'PENDING'
                                CHECK (status IN (
                                    'PENDING', 'QUEUED', 'DELIVERING', 'DELIVERED', 'FAILED', 'SKIPPED'
                                )),
    attempt_count           INTEGER NOT NULL DEFAULT 0,
    job_id                  UUID REFERENCES jobs (id) ON DELETE SET NULL,
    response_code           INTEGER,
    latency_ms              INTEGER,
    last_error              TEXT NOT NULL DEFAULT '',
    delivered_at            TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX outgoing_webhook_deliveries_org_created_idx
    ON outgoing_webhook_deliveries (organization_id, created_at DESC);
CREATE INDEX outgoing_webhook_deliveries_webhook_idx
    ON outgoing_webhook_deliveries (webhook_id, created_at DESC);
CREATE INDEX outgoing_webhook_deliveries_status_idx
    ON outgoing_webhook_deliveries (organization_id, status);

CREATE TRIGGER outgoing_webhook_deliveries_set_updated_at
    BEFORE UPDATE ON outgoing_webhook_deliveries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
