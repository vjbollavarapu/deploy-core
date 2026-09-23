# DeployCore Control Plane API

Go modular monolith that owns authentication, tenancy, deployment orchestration,
and platform configuration. Docker runtime execution belongs to the Agent — not
this service.

## Phase B1 bootstrap

Implemented:

- configuration loading (`DATABASE_URL`, `HTTP_ADDR`, …)
- structured JSON logging (`log/slog`)
- HTTP server with graceful shutdown
- `GET /health` and `GET /ready`
- PostgreSQL pool + embedded SQL migrations
- request IDs (`X-Request-ID`)
- recover / CORS / secure header middleware
- `/api/v1` versioned mount
- standard API error envelope (`pkg/apierror`)
- validation + pagination primitives

## Phase B2 database foundation

- Core schema migration `000002_core_schema` (users through jobs)
- Organization scoping, FKs, unique constraints, selective soft deletes
- Schema documentation: [`docs/SCHEMA.md`](docs/SCHEMA.md)

## Phase B3 authentication

- `POST /api/v1/auth/register|login|refresh|logout|forgot-password|reset-password`
- `GET /api/v1/auth/me`
- Argon2id passwords, refresh-token rotation, session revocation
- Short-lived access JWTs (no embedded permissions) + hashed refresh tokens
- Auth endpoint rate limiting; MFA fields reserved (`mfaEnabled` / challenge stub)

## Phase B4 organizations + RBAC

- Org CRUD under `/api/v1/organizations`
- Membership: list, invite, update roles, remove; `POST /api/v1/invitations/accept`
- Seeded system roles (Owner → Viewer) and permission catalog
- Server-side `rbac.Authorizer` (active membership + role_permissions)
- Audit writes for org/membership mutations

## Phase B5 projects + environments

- `GET/POST /api/v1/projects` (org-scoped via `organizationId`)
- `GET/PATCH/DELETE /api/v1/projects/{projectId}`
- `GET/POST /api/v1/projects/{projectId}/environments`
- `GET/PATCH/DELETE /api/v1/environments/{environmentId}`
- Unique active slugs; delete blocked while applications exist; audited mutations

## Phase B6 server registry

- `GET/POST /api/v1/servers` (org-scoped via `organizationId`)
- `GET/PATCH/DELETE /api/v1/servers/{serverId}`
- `POST|DELETE /api/v1/servers/{serverId}/maintenance`
- Status is server-controlled (`OFFLINE` on create; maintenance/disable APIs; no client `status` writes)
- Delete blocked while applications target the server; audited mutations

## Phase B7 agent registration

- `POST|DELETE /api/v1/servers/{serverId}/registration-token`
- `POST /api/v1/agents/register` (one-time token → durable hashed credential)
- `POST /api/v1/agents/heartbeat` (agent bearer auth; updates server liveness/status)
- Heartbeat samples retained with prune (default last 50 per server)

## Phase B8 agent command model

- Issue/list: `POST|GET /api/v1/servers/{serverId}/commands`
- Get/cancel: `GET /api/v1/commands/{commandId}`, `POST .../cancel`
- Agent poll/report: `GET /api/v1/agents/commands`, `POST /api/v1/agents/commands/{id}/status`
- Versioned schema (`schemaVersion: 1`); structured ops only — no shell/exec payloads

## Phase B9 applications

- `GET/POST /api/v1/applications` (org-scoped list via `organizationId`; optional `projectId` / `environmentId`)
- `GET/PATCH/DELETE /api/v1/applications/{applicationId}`
- Types: `WEB_SERVICE`, `API`, `WORKER`, `SCHEDULED_JOB`, `STATIC_SITE`, `DOCKER_COMPOSE`, `DOCKER_IMAGE`
- Config versioned in `application_configs` (create = v1; config PATCH = new version)
- Source-type validation (`git` / `image` / `compose` / `upload`); target server or placement policy required
- Soft-delete with active slug uniqueness; delete blocked while deployments are in progress

## Phase B10 variables + encrypted secrets

- `GET/POST /api/v1/variables`, `GET/PATCH/DELETE /api/v1/variables/{variableId}`
- `GET /api/v1/variables/resolved` — inheritance order org → project → environment → application
- `GET/POST /api/v1/secrets`, `GET/PATCH/DELETE /api/v1/secrets/{secretId}` (metadata only; PATCH rotates to a new version)
- Secrets use AES-256-GCM envelope encryption (`SECRETS_PLATFORM_KEY`, `SECRETS_KEY_ID`); plaintext never returned, logged, or audited

## Phase B11 deployment domain + state machine

- `POST /api/v1/applications/{applicationId}/deployments` — creates `PENDING` → `QUEUED` promptly, enqueues `DEPLOYMENT_EXECUTION` job (no long work in-request)
- `GET /api/v1/deployments`, `GET /api/v1/deployments/{deploymentId}` (includes transition events)
- `POST /api/v1/deployments/{deploymentId}/cancel`
- Strict transition graph; every change is transactional (`FOR UPDATE`) and appends `deployment_events`
- Terminal states immutable (admin reconcile helper reserved); idempotency via body/`Idempotency-Key`

## Phase B12 job queue

- PostgreSQL-backed queue with `FOR UPDATE SKIP LOCKED` claim, lease TTL, heartbeat, reclaim
- Statuses: `queued` → `leased` → `running` → `succeeded` | retry `queued` | `dead` | `cancelled`
- Worker runs in-process (`JOB_WORKER_ENABLED`, `JOB_LEASE_TTL`, `JOB_POLL_INTERVAL`, …)
- Job types: `DEPLOYMENT_EXECUTION` (deferred to B13), `BACKUP`, `RESTORE`, `CERTIFICATE_OPERATION`, `NOTIFICATION_DELIVERY`
- Idempotency unique per `(type, idempotency_key)`; exponential retry backoff; soft `ErrRetryLater` release

## Phase B13 deployment orchestrator

- Worker-driven workflow (not long HTTP): validate server → effective config snapshot → revision candidate → source/build/image → container → health → activate → `RUNNING`
- Resumable from current deployment status; each step uses the B11 state machine + events
- Failures land on the exact stage (`*_FAILED` / `TIMEOUT`); previous `ACTIVE` revision and traffic are preserved
- Agent commands issued when server is `ONLINE`; otherwise requires agent connectivity (`ORCHESTRATOR_SIMULATE_AGENT`, default **false**; set `true` only for local/dev simulation)

## Phase B14 revisions

- `GET /api/v1/applications/{applicationId}/revisions` (optional `status` filter)
- `GET /api/v1/revisions/{revisionId}`
- Immutable snapshots: config/vars/secret refs/health/limits — no mutation APIs for deployed revision configuration

## Phase B15 rollback

- `POST /api/v1/applications/{applicationId}/rollback` with `{ "targetRevisionId": "<uuid>" }`
- Requires `deployment.rollback`; creates a `trigger=rollback` deployment that reuses the target revision (no rebuild)
- Eligible targets: `READY` or `INACTIVE` (not already `ACTIVE`)
- On health failure: previous `ACTIVE` revision stays live; target is not marked `FAILED`
- Audited as `deployment.rollback`

## Phase B16 git providers

- `POST/GET/PATCH/DELETE /api/v1/integrations/git/connections`
- `POST .../connections/{id}/sync` upserts repository metadata; `GET .../repositories`
- Access tokens and webhook secrets encrypted at rest (AES-256-GCM); plaintext webhook secret returned once on create/rotate
- Provider interface with GitHub implemented; GitLab/Bitbucket/generic reserved
- `POST /api/v1/webhooks/git/{connectionId}` — signature verification (`X-Hub-Signature-256`), delivery-id dedup
- Push events create `trigger=git_push` deployments only when latest app config has `autoDeployEnabled` and matching `repositoryUrl` + `gitBranch`
- Permissions: `git.connection.read`, `git.connection.manage`

## Phase B17 container registries

- `POST/GET/PATCH/DELETE /api/v1/integrations/registries`
- Providers enabled: `ghcr`, `dockerhub`, `oci` (defaults `ghcr.io` / `docker.io`); reserved: `gcp`, `ecr`, `acr`
- Credentials encrypted at rest; API returns `hasCredentials` only — never tokens/passwords
- `ResolveCredentials` is internal for agent pull/push (not exposed on HTTP)
- Soft-delete frees registry names; permissions `registry.read` / `registry.manage`

## Phase B18 domains + Traefik routing

- `POST/GET /api/v1/applications/{id}/domains`, `PATCH/DELETE /api/v1/domains/{id}`
- Hostname unique per organization (active); at most one primary domain per application
- DNS: `PENDING|VALID|INVALID`; TLS: `PENDING|ISSUING|ACTIVE|EXPIRING|FAILED`
- Responses include desired Traefik `routing.labels` (control plane); agents apply at runtime
- Conflict code `DOMAIN_ALREADY_ASSIGNED`; permissions `domain.read` / `domain.manage`

## Phase B19 health checks

- Policy types: `HTTP`, `TCP`, `COMMAND`, `CONTAINER` on `application_configs.health_check`
- Fields: `initialDelaySeconds`, `intervalSeconds`, `timeoutSeconds`, `retries`, `path`, `port`, `expectedStatus`, `command`
- Aggregated states: `UNKNOWN` | `STARTING` | `HEALTHY` | `DEGRADED` | `UNHEALTHY`
- `GET /applications/{id}/health`, `GET .../health/samples`, `POST .../health/probes`
- Probe samples retained (max 20); status upserted on meaningful transitions only
- Deployments activate only after health policy is satisfied (`HEALTHY` or disabled)

## Phase B20 log streaming contract

- Kinds: `build`, `runtime`, `deployment_events` (SSE or JSON snapshot)
- `GET /applications/{id}/logs?kind=runtime|build&follow=&since=&cursor=&limit=`
- `GET /deployments/{id}/logs?kind=...`, `GET /deployments/{id}/events/stream`
- Agent ingest: `POST /agents/logs` (agent bearer) — `stdout`/`stderr`/`system` lines
- Pluggable `logs.Store` (default in-memory ring buffer); Loki/OpenSearch/ClickHouse adapters later
- Bounded buffers, disconnect cleanup, drop-on-slow-subscriber backpressure
- Build/runtime logs are **not** written unbounded to PostgreSQL; deployment events still come from `deployment_events`
- Auth: `application.read` / `deployment.read` + org isolation

## Phase B21 metrics

- Server current/summary: CPU, RAM, disk, load, uptime, network RX/TX, container count
- Container current/summary: CPU, RAM, network RX/TX, restart count, status
- `GET /servers/{id}/metrics`, `GET .../metrics/containers`, `GET .../metrics/series?metric=`
- `GET /applications/{id}/metrics` (latest container snapshot linked to the app)
- Agent ingest: `POST /agents/metrics` (server + containers); heartbeats also warm server snapshots
- Snapshots in Postgres (`server_metric_snapshots`, `container_metric_snapshots`) — not indefinite high-frequency series
- Pluggable `metrics.TimeSeriesStore` (in-memory ring now; Prometheus-compatible later)

## Phase B22 database resources

- Managed PostgreSQL resources under `/api/v1/databases`
- Fields: org/project/environment/server, name, engine version, storage volume, CPU/memory, DB name/user, encrypted credential, container runtime id, status, backup policy
- `POST /databases` creates control-plane row + issues `PROVISION_DATABASE` agent command (creation is agent-driven)
- Password never stored in command payload; agent uses `GET /agents/databases/{id}/bootstrap` while provisioning
- `volumeProtected=true`; soft-delete never deletes the data volume
- Reveal: `POST /databases/{id}/credentials/reveal` (`database.update`); permissions `database.read|create|update`
- Agent ops: `PROVISION_DATABASE`, `START_DATABASE`, `STOP_DATABASE`

## Phase B23 volumes

- Lifecycle: create / attach / detach / inspect / delete under `/api/v1/volumes`
- Fields: server, name, driver, mountPath, attached resource, backup policy, state, protected
- Agent ops: `CREATE_VOLUME`, `REMOVE_VOLUME`, `ATTACH_VOLUME`, `DETACH_VOLUME`, `INSPECT_VOLUME`
- Delete blocked for attached or database-critical (`protected`) volumes (`VOLUME_IN_USE`)
- Database create registers a protected volume (`EnsureDatabaseVolume`); labels prove platform ownership
- Explicit `REMOVE_VOLUME` required; soft-delete only after agent completion

## Phase B24 backup + restore

- PostgreSQL logical backups: `POST /databases/{id}/backups`, `GET /backups/{id}`, `DELETE /backups/{id}`
- Fields: resource, type, started/completed, duration, size, checksum, destination, status, retention
- Destination abstraction: `local` now; `s3` reserved — platform-controlled URIs only (`local://org/...`)
- Jobs `BACKUP` / `RESTORE` + agent `CREATE_BACKUP` / `RESTORE_BACKUP`; completion requires checksum
- Restore: `POST /backups/{id}/restore` with explicit `targetDatabaseId` and `confirm: "RESTORE"`
- Restore never succeeds without `validationPassed: true` from the agent
- Permissions: `database.backup` / `database.restore`; audited

## Phase B25 notifications

- Channels: `EMAIL` and `WEBHOOK` enabled; `SLACK`/`TEAMS`/`DISCORD`/`TELEGRAM`/`WHATSAPP` reserved
- Policies: event types, resource/environment filters, channel list, enabled flag
- Events: `DEPLOYMENT_FAILED`, `DEPLOYMENT_SUCCEEDED`, `SERVER_OFFLINE`, `SERVER_DEGRADED`,
  `BACKUP_FAILED`, `CERTIFICATE_EXPIRING`, `DISK_LOW`
- Delivery is async via jobs `NOTIFICATION_DELIVERY` (email = log sink; webhook = HTTP POST)
- Webhook payloads never include password/secret/token/credential fields
- APIs under `/api/v1/integrations/notifications/{channels,policies,deliveries,emit,meta}`
- Permissions: `notification.read` / `notification.manage`; audited
- Emit hooks: backup failures, deployment success/fail, server degraded/offline transitions

## Phase B26 outgoing webhooks

- Endpoints under `/api/v1/integrations/webhooks` with encrypted HMAC signing secrets
- Events: `deployment.started`, `deployment.completed`, `deployment.failed`,
  `server.offline`, `backup.completed`, `backup.failed`
- Each delivery: status, attempt count, latency, response code; job `WEBHOOK_DELIVERY`
- Worker applies exponential retry; webhook auto-disables after `failureThreshold` exhausted deliveries
- Payloads never include password/secret/token/credential fields
- Permissions: `webhook.read` / `webhook.manage`; audited (including auto-disable)

## Phase B27 audit completeness

- Append-only `audit_logs`: UPDATE blocked by DB trigger; no mutate API
- Read API: `GET /api/v1/audit-logs?organizationId=` (+ filters) and `GET /audit-logs/{id}`
- Permission: `audit.read`
- Auth security events: register, login success/failure/denied, logout, password reset request/complete
- Role changes, secrets, deploys/rollbacks, server registration/maintenance/disable, domains, backup/restore already audited
- Never stores secret values in before/after metadata

## Phase B31 security hardening

- Refresh session families + reuse detection; agent credential revoke API
- SSRF checks on webhook/notification URLs; recursive agentcmd payload forbid; mountPath validation
- Request body limit, HTTP timeouts, production CORS guard, public/git webhook rate limits
- Audit `DELETE` forbidden; log field sanitization
- Details: [`docs/SECURITY.md`](docs/SECURITY.md)

## Phase B32 API documentation

- OpenAPI 3.1: [`docs/openapi.json`](docs/openapi.json) — also `GET /openapi.json`
- Conventions: [`docs/API.md`](docs/API.md) (errors, pagination, auth, permissions, enums)
- Frontend wire types (not UI view-models): `apps/frontend/lib/api/contract.ts`

## Phase B33 test strategy

- Behavior-focused unit, repository, HTTP, and deployment-path coverage
- Matrix and how to run: [`docs/TESTING.md`](docs/TESTING.md)

## Run locally

```bash
export DATABASE_URL='postgres://deploycore:deploycore@localhost:5432/deploycore?sslmode=disable'
export HTTP_ADDR=':8080'
go run ./cmd/api
```

```bash
curl -i http://localhost:8080/health
curl -i http://localhost:8080/ready
```

## Test

```bash
go test ./...
go vet ./...
```
