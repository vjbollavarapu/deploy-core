-- Phase B2: core control-plane schema foundation.
-- Tenant-owned rows are organization-scoped. Soft deletes only where justified.

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- Identity
-- ---------------------------------------------------------------------------

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           TEXT NOT NULL,
    email_verified_at TIMESTAMPTZ,
    password_hash   TEXT NOT NULL,
    display_name    TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'disabled', 'pending')),
    mfa_enabled     BOOLEAN NOT NULL DEFAULT FALSE,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX users_email_active_uidx
    ON users (LOWER(email))
    WHERE deleted_at IS NULL;

CREATE TRIGGER users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Auth sessions / refresh tokens (hashed). Hard-deleted or revoked; no soft delete.
CREATE TABLE sessions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    refresh_token_hash  TEXT NOT NULL,
    user_agent          TEXT,
    ip_address          INET,
    expires_at          TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT sessions_refresh_token_hash_uidx UNIQUE (refresh_token_hash)
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at)
    WHERE revoked_at IS NULL;

CREATE TRIGGER sessions_set_updated_at
    BEFORE UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Tenancy
-- ---------------------------------------------------------------------------

CREATE TABLE organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'suspended', 'pending_deletion')),
    created_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE UNIQUE INDEX organizations_slug_active_uidx
    ON organizations (LOWER(slug))
    WHERE deleted_at IS NULL;

CREATE TRIGGER organizations_set_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE organization_members (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id          UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status           TEXT NOT NULL DEFAULT 'active'
                         CHECK (status IN ('active', 'invited', 'disabled')),
    invited_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    joined_at        TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT organization_members_org_user_uidx UNIQUE (organization_id, user_id)
);

CREATE INDEX organization_members_user_id_idx ON organization_members (user_id);

CREATE TRIGGER organization_members_set_updated_at
    BEFORE UPDATE ON organization_members
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE teams (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    slug             TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ,
    CONSTRAINT teams_org_slug_uidx UNIQUE (organization_id, slug)
);

CREATE INDEX teams_organization_id_idx ON teams (organization_id);

CREATE TRIGGER teams_set_updated_at
    BEFORE UPDATE ON teams
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE team_members (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id    UUID NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT team_members_team_user_uidx UNIQUE (team_id, user_id)
);

CREATE INDEX team_members_user_id_idx ON team_members (user_id);

-- ---------------------------------------------------------------------------
-- RBAC (permissions are global; roles may be system-wide or org-scoped)
-- ---------------------------------------------------------------------------

CREATE TABLE roles (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID REFERENCES organizations (id) ON DELETE CASCADE,
    key              TEXT NOT NULL,
    name             TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    is_system        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT roles_org_key_uidx UNIQUE (organization_id, key)
);

-- Allow one set of global system roles (organization_id IS NULL).
CREATE UNIQUE INDEX roles_system_key_uidx
    ON roles (key)
    WHERE organization_id IS NULL;

CREATE TRIGGER roles_set_updated_at
    BEFORE UPDATE ON roles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key         TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT permissions_key_uidx UNIQUE (key)
);

CREATE TABLE role_permissions (
    role_id       UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE member_roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id   UUID NOT NULL REFERENCES organization_members (id) ON DELETE CASCADE,
    role_id     UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT member_roles_member_role_uidx UNIQUE (member_id, role_id)
);

CREATE INDEX member_roles_role_id_idx ON member_roles (role_id);

-- ---------------------------------------------------------------------------
-- Projects + environments
-- ---------------------------------------------------------------------------

CREATE TABLE projects (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    slug             TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ,
    CONSTRAINT projects_org_slug_uidx UNIQUE (organization_id, slug)
);

CREATE INDEX projects_organization_id_idx ON projects (organization_id);

CREATE TRIGGER projects_set_updated_at
    BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE environments (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    project_id       UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    slug             TEXT NOT NULL,
    kind             TEXT NOT NULL DEFAULT 'custom'
                         CHECK (kind IN ('production', 'staging', 'development', 'preview', 'custom')),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ,
    CONSTRAINT environments_project_slug_uidx UNIQUE (project_id, slug)
);

CREATE INDEX environments_organization_id_idx ON environments (organization_id);
CREATE INDEX environments_project_id_idx ON environments (project_id);

CREATE TRIGGER environments_set_updated_at
    BEFORE UPDATE ON environments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Servers + agents
-- ---------------------------------------------------------------------------

CREATE TABLE servers (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    provider            TEXT NOT NULL DEFAULT '',
    region              TEXT NOT NULL DEFAULT '',
    hostname            TEXT NOT NULL DEFAULT '',
    public_ip           INET,
    private_ip          INET,
    architecture        TEXT NOT NULL DEFAULT '',
    operating_system    TEXT NOT NULL DEFAULT '',
    cpu_cores           INTEGER CHECK (cpu_cores IS NULL OR cpu_cores > 0),
    memory_bytes        BIGINT CHECK (memory_bytes IS NULL OR memory_bytes > 0),
    disk_bytes          BIGINT CHECK (disk_bytes IS NULL OR disk_bytes > 0),
    docker_version      TEXT,
    status              TEXT NOT NULL DEFAULT 'OFFLINE'
                            CHECK (status IN ('ONLINE', 'DEGRADED', 'OFFLINE', 'MAINTENANCE', 'DISABLED')),
    maintenance_mode    BOOLEAN NOT NULL DEFAULT FALSE,
    last_heartbeat_at   TIMESTAMPTZ,
    labels              JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by          UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ,
    CONSTRAINT servers_org_name_uidx UNIQUE (organization_id, name)
);

CREATE INDEX servers_organization_id_idx ON servers (organization_id);
CREATE INDEX servers_status_idx ON servers (organization_id, status)
    WHERE deleted_at IS NULL;

CREATE TRIGGER servers_set_updated_at
    BEFORE UPDATE ON servers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE server_agents (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id         UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    server_id               UUID NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    agent_version           TEXT NOT NULL DEFAULT '',
    credential_hash         TEXT NOT NULL,
    registration_token_hash TEXT,
    registration_expires_at TIMESTAMPTZ,
    registration_used_at    TIMESTAMPTZ,
    registration_revoked_at TIMESTAMPTZ,
    status                  TEXT NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending', 'active', 'revoked', 'disabled')),
    last_seen_at            TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT server_agents_server_id_uidx UNIQUE (server_id)
);

CREATE INDEX server_agents_organization_id_idx ON server_agents (organization_id);

CREATE TRIGGER server_agents_set_updated_at
    BEFORE UPDATE ON server_agents
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Ephemeral telemetry samples; retain via jobs, not soft delete.
CREATE TABLE server_heartbeats (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    server_id        UUID NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    agent_id         UUID REFERENCES server_agents (id) ON DELETE SET NULL,
    recorded_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    agent_version    TEXT NOT NULL DEFAULT '',
    docker_status    TEXT NOT NULL DEFAULT '',
    cpu_percent      DOUBLE PRECISION,
    memory_used_bytes BIGINT,
    disk_used_bytes  BIGINT,
    load_1           DOUBLE PRECISION,
    container_count  INTEGER,
    uptime_seconds   BIGINT,
    payload          JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX server_heartbeats_server_recorded_idx
    ON server_heartbeats (server_id, recorded_at DESC);
CREATE INDEX server_heartbeats_org_recorded_idx
    ON server_heartbeats (organization_id, recorded_at DESC);

-- ---------------------------------------------------------------------------
-- Applications
-- ---------------------------------------------------------------------------

CREATE TABLE applications (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    project_id       UUID NOT NULL REFERENCES projects (id) ON DELETE RESTRICT,
    environment_id   UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    name             TEXT NOT NULL,
    slug             TEXT NOT NULL,
    type             TEXT NOT NULL
                         CHECK (type IN (
                             'WEB_SERVICE', 'API', 'WORKER', 'SCHEDULED_JOB',
                             'STATIC_SITE', 'DOCKER_COMPOSE', 'DOCKER_IMAGE'
                         )),
    status           TEXT NOT NULL DEFAULT 'draft'
                         CHECK (status IN ('draft', 'ready', 'deploying', 'running', 'stopped', 'failed', 'archived')),
    target_server_id UUID REFERENCES servers (id) ON DELETE SET NULL,
    placement_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ,
    CONSTRAINT applications_env_slug_uidx UNIQUE (environment_id, slug)
);

CREATE INDEX applications_organization_id_idx ON applications (organization_id);
CREATE INDEX applications_project_id_idx ON applications (project_id);
CREATE INDEX applications_environment_id_idx ON applications (environment_id);

CREATE TRIGGER applications_set_updated_at
    BEFORE UPDATE ON applications
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Mutable desired configuration for an application (revisions snapshot immutable copies).
CREATE TABLE application_configs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    application_id   UUID NOT NULL REFERENCES applications (id) ON DELETE CASCADE,
    version          INTEGER NOT NULL DEFAULT 1,
    source_type      TEXT NOT NULL DEFAULT 'git'
                         CHECK (source_type IN ('git', 'image', 'compose', 'upload')),
    repository_url   TEXT,
    git_branch       TEXT,
    dockerfile_path  TEXT,
    build_context    TEXT,
    image_reference  TEXT,
    internal_port    INTEGER CHECK (internal_port IS NULL OR (internal_port > 0 AND internal_port <= 65535)),
    command          TEXT,
    entrypoint       TEXT,
    cpu_limit_millis INTEGER CHECK (cpu_limit_millis IS NULL OR cpu_limit_millis > 0),
    memory_limit_bytes BIGINT CHECK (memory_limit_bytes IS NULL OR memory_limit_bytes > 0),
    restart_policy   TEXT NOT NULL DEFAULT 'unless-stopped',
    health_check     JSONB NOT NULL DEFAULT '{}'::jsonb,
    runtime_config   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT application_configs_app_version_uidx UNIQUE (application_id, version)
);

CREATE INDEX application_configs_organization_id_idx ON application_configs (organization_id);

CREATE TRIGGER application_configs_set_updated_at
    BEFORE UPDATE ON application_configs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Deployments + revisions
-- ---------------------------------------------------------------------------

CREATE TABLE deployments (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    application_id   UUID NOT NULL REFERENCES applications (id) ON DELETE RESTRICT,
    environment_id   UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    server_id        UUID REFERENCES servers (id) ON DELETE SET NULL,
    status           TEXT NOT NULL DEFAULT 'PENDING'
                         CHECK (status IN (
                             'PENDING', 'QUEUED', 'PREPARING', 'FETCHING_SOURCE', 'BUILDING',
                             'IMAGE_READY', 'CREATING_CONTAINER', 'STARTING', 'HEALTH_CHECKING',
                             'ACTIVATING', 'RUNNING',
                             'SOURCE_FAILED', 'BUILD_FAILED', 'IMAGE_FAILED', 'CONTAINER_FAILED',
                             'START_FAILED', 'HEALTH_CHECK_FAILED', 'ROUTING_FAILED',
                             'CANCELLED', 'TIMEOUT'
                         )),
    trigger          TEXT NOT NULL DEFAULT 'manual'
                         CHECK (trigger IN ('manual', 'git_push', 'api', 'rollback', 'schedule', 'system')),
    idempotency_key  TEXT,
    request_id       TEXT,
    correlation_id   TEXT,
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ,
    error_code       TEXT,
    error_message    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT deployments_app_idempotency_uidx UNIQUE (application_id, idempotency_key)
);

CREATE INDEX deployments_organization_id_idx ON deployments (organization_id);
CREATE INDEX deployments_application_id_idx ON deployments (application_id, created_at DESC);
CREATE INDEX deployments_status_idx ON deployments (organization_id, status);

CREATE TRIGGER deployments_set_updated_at
    BEFORE UPDATE ON deployments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE deployment_events (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    deployment_id    UUID NOT NULL REFERENCES deployments (id) ON DELETE CASCADE,
    from_status      TEXT,
    to_status        TEXT NOT NULL,
    message          TEXT NOT NULL DEFAULT '',
    metadata         JSONB NOT NULL DEFAULT '{}'::jsonb,
    request_id       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX deployment_events_deployment_id_idx
    ON deployment_events (deployment_id, created_at);

-- Immutable revision snapshots. No soft delete.
CREATE TABLE revisions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    application_id   UUID NOT NULL REFERENCES applications (id) ON DELETE RESTRICT,
    deployment_id    UUID REFERENCES deployments (id) ON DELETE SET NULL,
    revision_number  INTEGER NOT NULL CHECK (revision_number > 0),
    status           TEXT NOT NULL DEFAULT 'CREATED'
                         CHECK (status IN ('CREATED', 'READY', 'ACTIVE', 'INACTIVE', 'FAILED', 'ARCHIVED')),
    commit_sha       TEXT,
    image_digest     TEXT,
    image_tag        TEXT,
    effective_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    variable_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    secret_refs      JSONB NOT NULL DEFAULT '[]'::jsonb,
    health_check     JSONB NOT NULL DEFAULT '{}'::jsonb,
    resource_limits  JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT revisions_app_number_uidx UNIQUE (application_id, revision_number)
);

CREATE INDEX revisions_organization_id_idx ON revisions (organization_id);
CREATE INDEX revisions_application_status_idx ON revisions (application_id, status);

CREATE TRIGGER revisions_set_updated_at
    BEFORE UPDATE ON revisions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE deployments
    ADD COLUMN active_revision_id UUID REFERENCES revisions (id) ON DELETE SET NULL;

ALTER TABLE deployments
    ADD COLUMN target_revision_id UUID REFERENCES revisions (id) ON DELETE SET NULL;

-- ---------------------------------------------------------------------------
-- Domains + certificates
-- ---------------------------------------------------------------------------

CREATE TABLE domains (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    application_id   UUID NOT NULL REFERENCES applications (id) ON DELETE CASCADE,
    environment_id   UUID NOT NULL REFERENCES environments (id) ON DELETE RESTRICT,
    hostname         TEXT NOT NULL,
    internal_port    INTEGER NOT NULL CHECK (internal_port > 0 AND internal_port <= 65535),
    is_primary       BOOLEAN NOT NULL DEFAULT FALSE,
    force_https      BOOLEAN NOT NULL DEFAULT TRUE,
    dns_status       TEXT NOT NULL DEFAULT 'PENDING'
                         CHECK (dns_status IN ('PENDING', 'VALID', 'INVALID')),
    tls_status       TEXT NOT NULL DEFAULT 'PENDING'
                         CHECK (tls_status IN ('PENDING', 'ISSUING', 'ACTIVE', 'EXPIRING', 'FAILED')),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);

-- One active assignment of a hostname within an organization routing scope.
CREATE UNIQUE INDEX domains_org_hostname_active_uidx
    ON domains (organization_id, LOWER(hostname))
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX domains_app_primary_active_uidx
    ON domains (application_id)
    WHERE is_primary AND deleted_at IS NULL;

CREATE INDEX domains_application_id_idx ON domains (application_id);

CREATE TRIGGER domains_set_updated_at
    BEFORE UPDATE ON domains
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE certificates (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    domain_id        UUID NOT NULL REFERENCES domains (id) ON DELETE CASCADE,
    provider         TEXT NOT NULL DEFAULT 'letsencrypt',
    status           TEXT NOT NULL DEFAULT 'PENDING'
                         CHECK (status IN ('PENDING', 'ISSUING', 'ACTIVE', 'EXPIRING', 'FAILED', 'REVOKED')),
    not_before       TIMESTAMPTZ,
    not_after        TIMESTAMPTZ,
    fingerprint_sha256 TEXT,
    issuer           TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);

CREATE INDEX certificates_domain_id_idx ON certificates (domain_id);
CREATE INDEX certificates_organization_id_idx ON certificates (organization_id);

CREATE TRIGGER certificates_set_updated_at
    BEFORE UPDATE ON certificates
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Variables + secrets (secrets store ciphertext only)
-- ---------------------------------------------------------------------------

CREATE TABLE environment_variables (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    scope            TEXT NOT NULL
                         CHECK (scope IN ('ORGANIZATION', 'PROJECT', 'ENVIRONMENT', 'APPLICATION')),
    project_id       UUID REFERENCES projects (id) ON DELETE CASCADE,
    environment_id   UUID REFERENCES environments (id) ON DELETE CASCADE,
    application_id   UUID REFERENCES applications (id) ON DELETE CASCADE,
    key              TEXT NOT NULL,
    value            TEXT NOT NULL,
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    updated_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT environment_variables_scope_key_check CHECK (
        (scope = 'ORGANIZATION' AND project_id IS NULL AND environment_id IS NULL AND application_id IS NULL)
        OR (scope = 'PROJECT' AND project_id IS NOT NULL AND environment_id IS NULL AND application_id IS NULL)
        OR (scope = 'ENVIRONMENT' AND environment_id IS NOT NULL AND application_id IS NULL)
        OR (scope = 'APPLICATION' AND application_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX environment_variables_org_scope_key_uidx
    ON environment_variables (
        organization_id,
        scope,
        key,
        COALESCE(project_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(environment_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(application_id, '00000000-0000-0000-0000-000000000000'::uuid)
    );

CREATE TRIGGER environment_variables_set_updated_at
    BEFORE UPDATE ON environment_variables
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE secrets (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    scope            TEXT NOT NULL
                         CHECK (scope IN ('ORGANIZATION', 'PROJECT', 'ENVIRONMENT', 'APPLICATION')),
    project_id       UUID REFERENCES projects (id) ON DELETE CASCADE,
    environment_id   UUID REFERENCES environments (id) ON DELETE CASCADE,
    application_id   UUID REFERENCES applications (id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    version          INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    ciphertext       BYTEA NOT NULL,
    nonce            BYTEA NOT NULL,
    key_id           TEXT NOT NULL,
    algorithm        TEXT NOT NULL DEFAULT 'AEAD',
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    updated_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ,
    CONSTRAINT secrets_scope_refs_check CHECK (
        (scope = 'ORGANIZATION' AND project_id IS NULL AND environment_id IS NULL AND application_id IS NULL)
        OR (scope = 'PROJECT' AND project_id IS NOT NULL AND environment_id IS NULL AND application_id IS NULL)
        OR (scope = 'ENVIRONMENT' AND environment_id IS NOT NULL AND application_id IS NULL)
        OR (scope = 'APPLICATION' AND application_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX secrets_active_name_uidx
    ON secrets (
        organization_id,
        scope,
        name,
        COALESCE(project_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(environment_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(application_id, '00000000-0000-0000-0000-000000000000'::uuid)
    )
    WHERE deleted_at IS NULL;

CREATE INDEX secrets_organization_id_idx ON secrets (organization_id);

CREATE TRIGGER secrets_set_updated_at
    BEFORE UPDATE ON secrets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Integrations
-- ---------------------------------------------------------------------------

CREATE TABLE git_connections (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    provider         TEXT NOT NULL
                         CHECK (provider IN ('github', 'gitlab', 'bitbucket', 'generic')),
    account_login    TEXT NOT NULL DEFAULT '',
    display_name     TEXT NOT NULL DEFAULT '',
    credential_ciphertext BYTEA NOT NULL,
    credential_nonce BYTEA NOT NULL,
    credential_key_id TEXT NOT NULL,
    webhook_secret_hash TEXT,
    last_sync_at     TIMESTAMPTZ,
    status           TEXT NOT NULL DEFAULT 'active'
                         CHECK (status IN ('active', 'error', 'revoked', 'disabled')),
    metadata         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);

CREATE INDEX git_connections_organization_id_idx ON git_connections (organization_id);

CREATE TRIGGER git_connections_set_updated_at
    BEFORE UPDATE ON git_connections
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE registries (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    provider         TEXT NOT NULL
                         CHECK (provider IN ('ghcr', 'dockerhub', 'gcp', 'ecr', 'acr', 'oci')),
    registry_url     TEXT NOT NULL,
    credential_ciphertext BYTEA,
    credential_nonce BYTEA,
    credential_key_id TEXT,
    status           TEXT NOT NULL DEFAULT 'active'
                         CHECK (status IN ('active', 'error', 'disabled')),
    metadata         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ,
    CONSTRAINT registries_org_name_uidx UNIQUE (organization_id, name)
);

CREATE INDEX registries_organization_id_idx ON registries (organization_id);

CREATE TRIGGER registries_set_updated_at
    BEFORE UPDATE ON registries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------------
-- Audit (append-only) + jobs
-- ---------------------------------------------------------------------------

CREATE TABLE audit_logs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID REFERENCES organizations (id) ON DELETE SET NULL,
    actor_user_id    UUID REFERENCES users (id) ON DELETE SET NULL,
    actor_type       TEXT NOT NULL DEFAULT 'user'
                         CHECK (actor_type IN ('user', 'system', 'agent', 'api_token')),
    action           TEXT NOT NULL,
    resource_type    TEXT NOT NULL,
    resource_id      TEXT,
    request_id       TEXT,
    ip_address       INET,
    user_agent       TEXT,
    before_metadata  JSONB,
    after_metadata   JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_logs_organization_created_idx
    ON audit_logs (organization_id, created_at DESC);
CREATE INDEX audit_logs_actor_created_idx
    ON audit_logs (actor_user_id, created_at DESC);
CREATE INDEX audit_logs_resource_idx
    ON audit_logs (resource_type, resource_id);

CREATE TABLE jobs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID REFERENCES organizations (id) ON DELETE CASCADE,
    type             TEXT NOT NULL
                         CHECK (type IN (
                             'DEPLOYMENT_EXECUTION', 'BACKUP', 'RESTORE',
                             'CERTIFICATE_OPERATION', 'NOTIFICATION_DELIVERY'
                         )),
    status           TEXT NOT NULL DEFAULT 'queued'
                         CHECK (status IN (
                             'queued', 'leased', 'running', 'succeeded',
                             'failed', 'dead', 'cancelled'
                         )),
    payload          JSONB NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key  TEXT,
    attempt_count    INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts     INTEGER NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
    available_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    leased_until     TIMESTAMPTZ,
    lease_owner      TEXT,
    last_error       TEXT,
    request_id       TEXT,
    correlation_id   TEXT,
    related_resource_type TEXT,
    related_resource_id   UUID,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX jobs_type_idempotency_uidx
    ON jobs (type, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX jobs_poll_idx
    ON jobs (status, available_at)
    WHERE status IN ('queued', 'leased');

CREATE INDEX jobs_organization_id_idx ON jobs (organization_id);

CREATE TRIGGER jobs_set_updated_at
    BEFORE UPDATE ON jobs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
