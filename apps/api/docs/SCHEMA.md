# DeployCore Control Plane Schema

Phase B2 foundation. Runtime migrations live in
`internal/platform/db/migrations/` (embedded). This document describes intent,
tenancy, and soft-delete policy.

## Conventions

| Concern | Rule |
| --- | --- |
| Primary keys | `UUID` via `gen_random_uuid()` |
| Timestamps | `TIMESTAMPTZ`, UTC; `created_at` / `updated_at` on mutable tables |
| Tenancy | Tenant-owned rows include `organization_id` and are queried through it |
| Secrets | Ciphertext + nonce + key id only — never plaintext tokens/passwords |
| Soft delete | Only where recovery / referential history justifies it (see below) |
| Append-only | `audit_logs`, `deployment_events` — no `deleted_at`, no in-place rewrite of meaning |

`updated_at` is maintained by trigger `set_updated_at()`.

## Soft delete policy

**Uses `deleted_at`:** `users`, `organizations`, `teams`, `projects`, `environments`,
`servers`, `applications`, `domains`, `certificates`, `secrets`, `git_connections`,
`registries`.

**Does not soft-delete:** membership/join tables, RBAC bindings, `sessions`
(revoke / hard delete), `server_heartbeats` (retention purge), `deployments`,
`deployment_events`, `revisions`, `application_configs`, `application_replicas`, `environment_variables`,
`audit_logs`, `jobs`, `application_health`, `health_probe_samples`.

## Entity map

```text
users ──< sessions
users ──< organization_members >── organizations
organizations ──< teams ──< team_members >── users
roles (system or org) ──< role_permissions >── permissions
organization_members ──< member_roles >── roles

organizations ──< projects ──< environments
organizations ──< servers ──< server_agents
                           └─< server_heartbeats

environments ──< applications ──< application_configs
                              ├─< application_replicas
                              ├─< deployments ──< deployment_events
                              ├─< revisions
                              └─< domains ──< certificates

organizations ──< environment_variables / secrets
organizations ──< git_connections ──< git_repositories
                               └─< git_webhook_deliveries
organizations ──< registries
organizations ──< audit_logs / jobs
```

## Table notes

### Identity

- **users** — email unique among non-deleted rows; Argon2id hash stored in
  `password_hash` (hashing implemented in B3).
- **sessions** — refresh token **hashes** only; `revoked_at` for logout/rotation;
  `family_id` groups rotated sessions so refresh-token reuse revokes the family
  (B31). `replaced_by_session_id` links rotation chain.

### Tenancy + RBAC

- **organizations** — slug unique among active orgs.
- **organization_members** — unique `(organization_id, user_id)`.
- **roles** — `organization_id NULL` + `is_system` for built-in roles; org-custom
  roles are org-scoped. Unique system `key` via partial index.
- **permissions** — global catalog (`server.read`, … seeded in B4).
- **member_roles** — assigns roles to membership rows (not bare users), so
  permissions stay org-bound.

### Projects / environments

- Project slug unique per organization.
- Environment slug unique per project; `kind` constrained
  (`production` / `staging` / `development` / `preview` / `custom`).

### Servers / agents

- Server `status`: `ONLINE` | `DEGRADED` | `OFFLINE` | `MAINTENANCE` | `DISABLED`.
- Capacity (B28): inventory totals (`cpu_cores`, `memory_bytes`, `disk_bytes`) plus
  denormalized reservations (`cpu_allocated_millis`, `memory_allocated_bytes`,
  `disk_allocated_bytes`) recomputed from latest `application_configs` of apps
  targeting the server. Placement is user-selected by default; optional
  `placement_policy.mode=scheduler` filters healthy → maintenance → labels →
  capacity, then scores (`least_loaded` / `most_free`). Impossible placement
  returns `INSUFFICIENT_RESOURCES`. APIs: `GET /servers/{id}/capacity`,
  `GET /servers/capacity?organizationId=`, `POST /placement/preview`.
- **server_agents** — one agent identity per server; registration token hash is
  one-time; durable credential stored as hash only.
- **server_heartbeats** — samples for recent telemetry; not a forever time series.

### Applications / deployments / revisions

- Application types match B9 (`WEB_SERVICE`, `API`, …).
- **application_configs** — mutable desired config, versioned per app;
  create writes v1; config PATCH inserts a new version. Soft-deleted apps
  free their environment slug (`000009`). `runtime_config.desiredReplicas`
  (B29, default 1, max 20) drives manual replica count; capacity multiplies
  per-replica limits by that value.
- **application_replicas** (B29) — observed slots (`replica_index`, status,
  healthy, routing_enabled). Scale via `/applications/{id}/replicas/scale`;
  deploy rolls replicas; Traefik LB shares service name (= app slug). See
  `docs/REPLICAS.md`.
- **deployments** — state machine statuses from B11; optional
  `idempotency_key` unique per application; links to active/target revisions.
  Create path persists `PENDING`→`QUEUED` events and a `DEPLOYMENT_EXECUTION`
  job row for B12 workers. Transitions use row locks; terminal states set
  `finished_at` and are immutable except admin reconcile.
  B13 orchestrator advances queued deployments through the happy path,
  creating revision candidates and activating only after health checks.
- **deployment_events** — every transition appends a row.
- **revisions** — immutable snapshots; unique `revision_number` per application.
  Read via `GET /applications/{id}/revisions` and `GET /revisions/{id}` (B14).
  States: `CREATED` | `READY` | `ACTIVE` | `INACTIVE` | `FAILED` | `ARCHIVED`.
  Effective config, variable metadata, and secret version refs are frozen at create time.
  Rollback (`POST /applications/{id}/rollback`, B15) reuses an existing `READY`/`INACTIVE`
  revision without creating a new snapshot; activation still requires health checks.

### Health checks (B19)

- **application_health** — one aggregated row per application (`UNKNOWN` /
  `STARTING` / `HEALTHY` / `DEGRADED` / `UNHEALTHY`) with consecutive success/failure
  counters. Updated during deployment health verification and probe reports.
- **health_probe_samples** — bounded recent probe history (retain last 20);
  avoids writing every raw probe tick.
- Policy lives in `application_configs.health_check` JSON (`HTTP`/`TCP`/`COMMAND`/
  `CONTAINER` plus timing/path/port fields). Orchestrator will not activate
  (route traffic) until the policy gate is satisfied.

### Logs (B20)

- Build and runtime container logs use a pluggable `logs.Store` (default:
  process-local ring buffer). They are **not** persisted unbounded in PostgreSQL.
- Deployment lifecycle events continue to live in append-only `deployment_events`
  and are streamed via the same log API (`kind=deployment_events`).
- Future backends (Loki / OpenSearch / ClickHouse) implement `Store` without
  changing the HTTP/SSE contract.

### Metrics (B21)

- **server_metric_snapshots** — one current/summary row per server (CPU, RAM,
  disk, load, uptime, network RX/TX, container count). Updated by
  `POST /agents/metrics` and mirrored from heartbeats.
- **container_metric_snapshots** — one current row per `(server_id, container_id)`
  with CPU/RAM/network/restart/status; optional `application_id` link.
- High-frequency time series are **not** stored indefinitely in transactional
  Postgres. `metrics.TimeSeriesStore` is the extension point for Prometheus
  (default: bounded in-memory ring for recent samples / tests).

### Managed databases (B22)

- **managed_databases** — PostgreSQL resources scoped to org/project/environment/server.
- Credentials encrypted (AES-256-GCM); `volume_protected` defaults true.
- Soft-delete sets `DELETED` but **never** removes `storage_volume_name` data —
  volumes outlive disposable database containers.
- Provisioning is agent-driven (`PROVISION_DATABASE`); completion updates status /
  `container_runtime_id` via the agent command completion hook.
- Backup policy JSON is stored for B24; no automatic backup execution here.

### Volumes (B23)

- **volumes** — managed Docker volumes per server (`local` driver default).
- States: `PENDING` → `CREATING` → `READY` / `ATTACHED` / `DELETING` / `FAILED` / `DELETED`.
- Attachment targets: `database` or `application`; delete blocked when attached or `protected`.
- Database provisioning registers a protected critical volume; never deleted as a
  side effect of database soft-delete or container recreate.
- Agent commands: `CREATE_VOLUME`, `REMOVE_VOLUME`, `ATTACH_VOLUME`, `DETACH_VOLUME`, `INSPECT_VOLUME`.

### Backups (B24)

- **backups** — PostgreSQL logical dump records (resource, checksum, size, destination,
  retention, job/command ids). Destination URIs are platform-allocated (`local://…`);
  S3 adapter reserved via `Destination` interface.
- **restore_operations** — explicit target + destructive `confirm=RESTORE`; status
  reaches `SUCCEEDED` only when agent reports `validationPassed=true`.
- Execution uses jobs `BACKUP`/`RESTORE` and agent ops `CREATE_BACKUP`/`RESTORE_BACKUP`.

### Notifications (B25)

- **notification_channels** — EMAIL/WEBHOOK enabled; other types reserved. Optional
  encrypted credential (bearer for webhooks). Soft-deletable.
- **notification_policies** — event type list + resource/environment JSON filters +
  channel id list; soft-deletable.
- **notification_deliveries** — delivery attempt log with status, job id, latency,
  response code. Jobs type `NOTIFICATION_DELIVERY`.
- API under `/integrations/notifications/...`; secrets never appear in webhook payloads.

### Outgoing webhooks (B26)

- **outgoing_webhooks** — URL + subscribed events + encrypted HMAC secret; consecutive
  failure counter and auto-disable at threshold.
- **outgoing_webhook_deliveries** — attempt log (status, latency, response code, job id).
- Jobs type `WEBHOOK_DELIVERY`; signature header `X-DeployCore-Signature: sha256=…`.
- API under `/integrations/webhooks`.

### Domains / certificates

- Active hostname unique per organization (prevents conflicting routing).
- At most one primary domain per application while active.
- DNS / TLS status enums align with B18.
- API under `/applications/{id}/domains` and `/domains/{id}` generates desired
  Traefik routing labels (`routing.labels`) for agents to apply; soft-delete
  frees hostnames for reassignment.

### Variables / secrets

- Scope: `ORGANIZATION` → `PROJECT` → `ENVIRONMENT` → `APPLICATION` with CHECK
  constraints on which FKs may be set.
- Resolved variables merge in that order (later scopes override earlier).
- **secrets** store `ciphertext` / `nonce` / `key_id` only (AES-256-GCM envelope;
  platform key wraps a per-record DEK). API returns metadata (name, scope, version,
  timestamps) — never plaintext. Rotation soft-deletes the prior row and inserts
  `version + 1`.

### Integrations

- **git_connections** — provider enums (`github`/`gitlab`/`bitbucket`/`generic`);
  access tokens and webhook signing secrets encrypted at rest (same envelope as
  secrets). Soft-deletable. API under `/integrations/git/connections` (B16).
- **git_repositories** — synced repository metadata per connection (full name,
  default branch, URLs); `last_sync_at` updated on sync.
- **git_webhook_deliveries** — append-ish delivery log with unique
  `(connection_id, delivery_id)` for duplicate webhook protection.
- **application_configs.auto_deploy_enabled** / **git_connection_id** — when
  enabled, matching GitHub push webhooks may enqueue `git_push` deployments.
- **registries** — provider enums; credentials encrypted at rest (B17).
  Enabled providers: `ghcr`, `dockerhub`, `oci`. Reserved (schema-ready):
  `gcp`, `ecr`, `acr`. Soft-delete frees `(organization_id, name)` via
  `000013`. API under `/integrations/registries`; agents resolve secrets
  via internal `ResolveCredentials` for pull/push.

### Audit / jobs

- **audit_logs** — actor, action, resource, org, request id, IP, UA, safe
  before/after metadata. Never store secret values. Append-only: UPDATE and
  DELETE are rejected by triggers (`000021` / `000025`); no HTTP mutate
  endpoints. Read via `/audit-logs` with `audit.read` (B27). Auth security
  events may omit `organization_id`. Test cleanup uses `TRUNCATE audit_logs`.
- **jobs** — PostgreSQL queue (B12): lease fields, attempts,
  `idempotency_key`, poll index on `(status, available_at)`, reclaim index on
  `leased_until`. Workers claim with `SKIP LOCKED`; expired leases return to
  `queued`. Job types include webhook/replicas reconcile and
  `DESIRED_STATE_RECONCILE` (B30): heartbeat expiry → offline, replica
  desired vs observed, restart-policy restarts with backoff. See
  `docs/RECONCILE.md`.

## Migrations

| Version | File | Purpose |
| --- | --- | --- |
| `000001_bootstrap` | extension + bootstrap marker | B1 |
| `000002_core_schema` | all core tables above | B2 |
| `000003_auth_password_reset` | `password_reset_tokens` (hashed) | B3 |
| `000004_rbac_seed` | invitations + permissions/roles seed | B4 |
| `000005_project_env_uniques` | active-only project/env slug uniques | B5 |
| `000006_server_name_unique` | active-only server name unique per org | B6 |
| `000007_agent_indexes` | registration/credential lookup indexes | B7 |
| `000008_agent_commands` | versionable agent command queue | B8 |
| `000009_application_slug_unique` | active-only app slug unique per environment | B9 |
| `000010_secrets_algorithm` | default secret algorithm `AES-256-GCM` | B10 |
| `000011_job_reclaim_index` | expired lease reclaim index | B12 |
| `000012_git_providers` | repos, webhook deliveries, auto-deploy, encrypted webhook secrets | B16 |
| `000013_registries` | soft-delete-safe registry names, username, credential algorithm | B17 |
| `000014_health_checks` | application_health + bounded health_probe_samples | B19 |
| `000025_security_hardening` | audit DELETE forbid; sessions.family_id rotation | B31 |

### Auth notes (B3 / B31)

- Access tokens are short-lived JWTs with `sub` (user id) + `sid` (session id) only — no permissions.
- Refresh tokens are opaque secrets stored as SHA-256 hashes in `sessions`.
- Refresh rotation is atomic; reuse of a rotated token revokes the session family.
- Password resets use hashed one-time tokens in `password_reset_tokens`.

Apply via API startup migrator (`internal/platform/db.Migrator`). Operator-readable
copies: `migrations/`.
