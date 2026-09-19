-- Phase B25: notification channels, policies, and delivery log.

CREATE TABLE notification_channels (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name                    TEXT NOT NULL,
    type                    TEXT NOT NULL
                                CHECK (type IN (
                                    'EMAIL', 'WEBHOOK',
                                    'SLACK', 'TEAMS', 'DISCORD', 'TELEGRAM', 'WHATSAPP'
                                )),
    config                  JSONB NOT NULL DEFAULT '{}'::jsonb,
    credential_ciphertext   BYTEA,
    credential_nonce        BYTEA,
    credential_key_id       TEXT,
    credential_algorithm    TEXT NOT NULL DEFAULT 'AES-256-GCM',
    enabled                 BOOLEAN NOT NULL DEFAULT TRUE,
    status                  TEXT NOT NULL DEFAULT 'ACTIVE'
                                CHECK (status IN ('ACTIVE', 'DISABLED', 'FAILED')),
    last_error              TEXT NOT NULL DEFAULT '',
    created_by              UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ
);

CREATE UNIQUE INDEX notification_channels_org_name_active_uidx
    ON notification_channels (organization_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX notification_channels_org_idx
    ON notification_channels (organization_id)
    WHERE deleted_at IS NULL;

CREATE TRIGGER notification_channels_set_updated_at
    BEFORE UPDATE ON notification_channels
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE notification_policies (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name                    TEXT NOT NULL,
    event_types             JSONB NOT NULL DEFAULT '[]'::jsonb,
    resource_filters        JSONB NOT NULL DEFAULT '{}'::jsonb,
    environment_filters     JSONB NOT NULL DEFAULT '{}'::jsonb,
    channel_ids             JSONB NOT NULL DEFAULT '[]'::jsonb,
    enabled                 BOOLEAN NOT NULL DEFAULT TRUE,
    created_by              UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ
);

CREATE UNIQUE INDEX notification_policies_org_name_active_uidx
    ON notification_policies (organization_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX notification_policies_org_idx
    ON notification_policies (organization_id)
    WHERE deleted_at IS NULL;

CREATE TRIGGER notification_policies_set_updated_at
    BEFORE UPDATE ON notification_policies
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE notification_deliveries (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    policy_id               UUID REFERENCES notification_policies (id) ON DELETE SET NULL,
    channel_id              UUID NOT NULL REFERENCES notification_channels (id) ON DELETE CASCADE,
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

CREATE INDEX notification_deliveries_org_created_idx
    ON notification_deliveries (organization_id, created_at DESC);
CREATE INDEX notification_deliveries_channel_idx
    ON notification_deliveries (channel_id, created_at DESC);
CREATE INDEX notification_deliveries_status_idx
    ON notification_deliveries (organization_id, status);

CREATE TRIGGER notification_deliveries_set_updated_at
    BEFORE UPDATE ON notification_deliveries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
