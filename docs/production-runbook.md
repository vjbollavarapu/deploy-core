# DeployCore Production Runbook (v1)

**Audience:** operators deploying and operating DeployCore for the first OCI Ubuntu validation and subsequent production use.  
**Authority:** repository state after I1–I10.  
**Status:** operational documentation. Docker-dependent procedures are marked **LIVE VALIDATION**.

This runbook does **not** claim that live Ubuntu/Docker validation has already passed.

---

## 1. Supported v1 architecture

```text
┌─────────────────────────────────────────────────────────────┐
│ CONTROL PLANE (operators / tenants)                         │
│  • Frontend (Next.js)                                       │
│  • API (Go)                                                 │
│  • Control Plane PostgreSQL  ← platform metadata ONLY       │
└───────────────────────────┬─────────────────────────────────┘
                            │ HTTPS Agent protocol
                            │ (register, heartbeat, poll, results,
                            │  logs, metrics)
┌───────────────────────────▼─────────────────────────────────┐
│ MANAGED SERVER (Ubuntu 22.04/24.04)                         │
│  systemd: deploycore-agent                                  │
│  Docker:                                                    │
│    • Traefik (deploycore-traefik) — edge HTTP/HTTPS         │
│    • deploycore-proxy network                               │
│    • application containers                                 │
│    • managed database containers (e.g. PostgreSQL)          │
│    • volumes / networks owned by DeployCore labels          │
└─────────────────────────────────────────────────────────────┘
```

### Control Plane PostgreSQL vs managed application PostgreSQL

| | Control Plane PostgreSQL | Managed application PostgreSQL |
|---|---|---|
| Purpose | Tenancy, auth, deployments, jobs, agent commands | Customer/application data |
| Owner | API / operators | Agent on a managed server |
| Provenance | Provisioned for `apps/api` (`DATABASE_URL`) | Created via Databases UI/API → Agent `PROVISION_DATABASE` |
| Backup/restore | **Out of scope of managed backup UI** | Local single-host backup (v1) |
| **Never** | Select as managed restore target | Treat as CP database |

---

## 2. First deployment topology (OCI validation)

**Recommended first topology:** one Ubuntu server for **managed workloads** (Agent + Docker + Traefik), with the **Control Plane on a separate host or the same host only with strict boundaries**.

### What runs where

| Component | Runtime | Owner |
|---|---|---|
| Control Plane PostgreSQL | Process or Docker (`docker-compose.yml` postgres service for **dev/bootstrap only**) | Operator |
| API (`deploycore-api` / `go run ./cmd/api`) | Host process (no API Dockerfile in repo) | Operator / systemd (operator-managed) |
| Frontend (`next start`) | Host Node process (no frontend Dockerfile in repo) | Operator |
| Agent | **systemd** `deploycore-agent.service` | Installer |
| Docker Engine | systemd (`docker.service`) | Installer (`--install-docker`) or operator |
| Traefik | **Docker** container `deploycore-traefik` | **Installer** (I9) |
| `deploycore-proxy` | Docker network | **Installer** (create) + Agent (ensure via commands) |
| Apps / managed DBs / volumes | Docker | Agent |

### Same-host Control Plane + managed workloads

**Supported with care** by current architecture (no hard prohibition), but:

1. Do **not** register the Control Plane PostgreSQL container as a managed DeployCore database.
2. Do **not** restore managed backups into CP PostgreSQL.
3. Prefer separate disks/volumes for CP data vs Agent `/var/lib/deploycore-agent`.
4. Expose 80/443 for Traefik; keep CP API on a distinct port/hostname (or reverse-proxy path) so Traefik and the API do not fight for the same listeners.
5. Resource contention (CPU/RAM/disk) is an operator concern; capacity helpers exist in the API but hard sizing numbers are **recommendations**, not hard requirements (see §3).

There is **no packaged Control Plane production installer** in this repository. CP deploy is **manual** (build + env + process manager). Managed-server install uses `deployments/install/install-agent.sh`.

---

## 3. Prerequisites

### Hard requirements (installer / config)

| Requirement | Detail |
|---|---|
| OS (Agent host) | Ubuntu **22.04** or **24.04** only |
| Architecture | `amd64` (`x86_64`) or `arm64` (`aarch64`) |
| Privileges | root or sudo |
| Docker | Required on Agent host (installer can install via official Docker Ubuntu repo) |
| systemd | Required for Agent unit |
| Outbound network | Control Plane URL (Agent); Docker Hub / Traefik image; Let’s Encrypt if ACME used |
| ACME email | Required for Traefik HTTPS unless `--skip-traefik` |
| DNS (apps) | Public A/AAAA (or equivalent) to the managed server for domains you attach |

### Recommendations (not hard-coded in product)

| Resource | Recommendation for first OCI host |
|---|---|
| CPU | ≥ 2 vCPU |
| RAM | ≥ 4 GiB (8 GiB preferred if CP + Agent colocated) |
| Disk | ≥ 40 GiB free for images, volumes, Traefik ACME, backups |
| CP PostgreSQL | PostgreSQL 15+ compatible with migrations (compose ships `postgres:15-alpine` for local bootstrap) |

### Ports (managed server)

| Port | Use |
|---|---|
| 80 / 443 | Traefik (`web` / `websecure`) |
| Docker socket | local `unix:///var/run/docker.sock` only — **never** expose remotely |
| CP API | typically `:8080` or behind TLS reverse proxy (operator choice) |
| Frontend | typically `:3000` or behind reverse proxy |

### TLS expectations

- Application HTTPS uses Traefik Docker-provider labels + `certresolver=letsencrypt` (Agent-generated).
- HTTP→HTTPS uses Docker-provider `redirectscheme` middleware (not `redirect-https@file`).
- Control Plane TLS is operator-provided (reverse proxy / load balancer); API itself serves HTTP on `HTTP_ADDR`.

---

## 4. Production environment variables

Secrets are shown as `<REDACTED>`. Never commit real values.

### 4.1 API (`apps/api`)

| Variable | Required? | Purpose | Example format | Production note |
|---|---|---|---|---|
| `APP_ENV` | Yes | Environment mode | `production` | Enables production secret/CORS rules |
| `DATABASE_URL` | Yes | CP PostgreSQL DSN | `postgres://user:<REDACTED>@host:5432/deploycore?sslmode=require` | **CP DB only** |
| `HTTP_ADDR` | No (default `:8080`) | Listen address | `:8080` | Bind behind reverse proxy as needed |
| `AUTH_TOKEN_SECRET` | Yes (prod) | JWT HMAC secret | `<REDACTED>` ≥ 32 chars | No default in production |
| `SECRETS_PLATFORM_KEY` | Yes (prod) | AEAD key for stored secrets | 32 raw bytes or base64 of 32 bytes `<REDACTED>` | Required in production |
| `CORS_ALLOWED_ORIGINS` | Yes (prod) | Browser origins | `https://app.example.com` | Must not be `*` in production |
| `ORCHESTRATOR_SIMULATE_AGENT` | Yes (explicit) | Skip real Agent execution | `false` | **Must be `false` in production** (default is already `false`) |
| `LOG_LEVEL` | No | Log verbosity | `info` | Avoid `debug` with secret-bearing payloads |
| `AGENT_HEARTBEAT_TTL` | No | Offline marking | `90s` | ONLINE/DEGRADED → OFFLINE when heartbeat stale |
| `JOB_WORKER_ENABLED` | No | Background jobs | `true` | Keep enabled for deploys/backups |
| `RECONCILE_ENABLED` | No | Desired-state sweep | `true` | Full reconcile still limited (R1) |
| `SECRETS_KEY_ID` | No | Envelope key id | `platform:v1` | Rotate via documented key process |
| `AUTH_ACCESS_TOKEN_TTL` | No | Access JWT TTL | `15m` | |
| `AUTH_REFRESH_TOKEN_TTL` | No | Refresh TTL | `720h` | |
| `MAX_REQUEST_BODY_BYTES` | No | Body limit | `1048576` | |
| `READ_TIMEOUT` / `WRITE_TIMEOUT` / `IDLE_TIMEOUT` | No | HTTP timeouts | see `.env.example` | |

Reference template (development only): `apps/api/.env.example`.

### 4.2 Frontend (`apps/frontend`)

| Variable | Required? | Purpose | Example | Production note |
|---|---|---|---|---|
| `NEXT_PUBLIC_DEMO_MODE` | Must be unset/`false` | Demo fixtures | _(omit)_ | **Must not be `true`** |
| `NEXT_PUBLIC_ENABLE_MOCK_FALLBACK` | Must be unset/`false` | Mock API fallback | _(omit)_ | **Must not be `true`** |

Wire API base is relative `API_BASE = '/api/v1'` (`lib/api/contract.ts`). Production must reverse-proxy `/api/v1` to the Control Plane API (or otherwise ensure the browser reaches the real API). There is no separate production frontend packaging installer in-repo.

### 4.3 Agent (via `/etc/deploycore-agent/agent.env`)

| Variable | Required? | Purpose | Example | Production note |
|---|---|---|---|---|
| `AGENT_CONTROL_PLANE_URL` | Yes | CP base URL | `https://cp.example.com` | Reachable from Agent host |
| `AGENT_DATA_DIR` | Yes (installer sets) | Data root | `/var/lib/deploycore-agent` | 0700 |
| `AGENT_CREDENTIAL_PATH` | Yes (installer sets) | Durable credential | `/var/lib/deploycore-agent/credentials.json` | 0600 after register |
| `AGENT_SERVER_ID` | Recommended | Expected server UUID | `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx` | Matches CP server record |
| `AGENT_LOG_LEVEL` | No | Logging | `info` | |
| `AGENT_HEARTBEAT_INTERVAL` | No | Heartbeat period | `30s` | |
| `AGENT_DOCKER_HOST` | No | Docker endpoint | _(empty = default socket)_ | |
| `AGENT_REGISTRATION_TOKEN` | First boot only | One-time register | in `registration.env` | **Not** left in `agent.env`; file deleted after success |

### 4.4 Installer flags (`deployments/install/install-agent.sh`)

| Flag | Required? | Purpose | Example | Production note |
|---|---|---|---|---|
| `--server-url` | First install | CP URL | `https://cp.example.com` | Written to `agent.env` |
| `--server-id` | Recommended | Server UUID | UUID | |
| `--token` | First register | One-time token | `<REGISTRATION_TOKEN>` | → `registration.env` only |
| `--binary` | OCI path | Local Agent binary | `./deploycore-agent-linux-amd64` | Preferred until public releases (R20) |
| `--checksum` | With binary/remote | SHA256SUMS path/URL | `./SHA256SUMS` | Mismatch aborts |
| `--version` | Remote download | Release tag | `v0.1.0` | Not `latest` for remote |
| `--install-docker` | If no Docker | Official Docker CE install | | Skips if Docker healthy |
| `--acme-email` | If Traefik | Let’s Encrypt email | `ops@example.com` | Required unless `--skip-traefik` |
| `--skip-traefik` | Optional | Skip edge proxy setup | | No public HTTPS routing |
| `--release-url` | Optional | Artifact base URL | HTTPS base | |

---

## 5. Control Plane deployment (manual)

**There is no packaged CP production installer.** Supported method: provision PostgreSQL, build/run API and frontend as host processes (or your own process manager).

### 5.1 PostgreSQL (Control Plane)

Option A — existing managed Postgres (recommended for production):

1. Create empty database and role.
2. Set `DATABASE_URL` with `sslmode=require` (or your org standard).

Option B — local bootstrap compose (**development / first lab only**):

```bash
# repo root — starts postgres:15-alpine only
docker compose up -d postgres
```

File: `docker-compose.yml` (Postgres only; **not** a full CP stack).

### 5.2 Migrations

Embedded migrations run automatically on API startup:

```bash
# apps/api/cmd/api/main.go → db.NewMigrator(pool).Up(ctx)
```

No separate migrate CLI is required for normal startup.

### 5.3 API

```bash
export PATH="$(pwd)/apps/api/.tools/go/bin:$PATH"   # if using bundled Go
cd apps/api
export APP_ENV=production
export DATABASE_URL='postgres://…'                 # <REDACTED>
export AUTH_TOKEN_SECRET='<REDACTED>'              # ≥ 32 chars
export SECRETS_PLATFORM_KEY='<REDACTED>'           # 32-byte key
export CORS_ALLOWED_ORIGINS='https://app.example.com'
export ORCHESTRATOR_SIMULATE_AGENT=false
export HTTP_ADDR=':8080'
go build -o /usr/local/bin/deploycore-api ./cmd/api
# run under systemd/supervisor of your choice
deploycore-api
```

Health:

```bash
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
```

### 5.4 Frontend

```bash
cd apps/frontend
# Ensure demo/mock are OFF (unset both):
#   NEXT_PUBLIC_DEMO_MODE
#   NEXT_PUBLIC_ENABLE_MOCK_FALLBACK
pnpm install
pnpm run build
pnpm run start    # default :3000
```

Put a reverse proxy in front so browser calls to `/api/v1/*` reach the API.

### 5.5 Startup verification

1. `/health` and `/ready` → 200  
2. Register/login via UI or `POST /api/v1/auth/register|login`  
3. Create organization  
4. Confirm `ORCHESTRATOR_SIMULATE_AGENT=false` (no simulated deploy success)

---

## 6. First server creation (operator flow)

UI terminology matches current product: **Servers**, registration token, Agent.

1. Sign in to DeployCore (Frontend).  
2. Create or select an **organization**.  
3. Open **Servers** → add server (`POST /api/v1/servers`) with name/capacity inventory.  
4. Issue one-time registration token:  
   - UI server detail / agent panel, or  
   - `POST /api/v1/servers/{serverId}/registration-token`  
   - Copy token once; treat as `<REGISTRATION_TOKEN>` — **do not log it**.  
5. Prepare Ubuntu 22.04/24.04 host (SSH, outbound HTTPS).  
6. Install Agent (§7).  
7. `systemctl enable --now deploycore-agent`.  
8. Verify server → **ONLINE** or **DEGRADED** (§9).

Revoke unused tokens: `DELETE /api/v1/servers/{serverId}/registration-token`.

---

## 7. Agent installation (I9 installer)

Artifacts:

- `deployments/install/install-agent.sh`  
- `deployments/install/deploycore-agent.service`  
- Agent binary + `SHA256SUMS` (build locally until public releases exist — **R20**)

### 7.1 First OCI validation command shape

```bash
# On the Ubuntu host, as a user with sudo:
sudo ./install-agent.sh \
  --install-docker \
  --server-url 'https://cp.example.com' \
  --server-id '<SERVER_UUID>' \
  --token '<REGISTRATION_TOKEN>' \
  --binary ./deploycore-agent-linux-amd64 \
  --checksum ./SHA256SUMS \
  --acme-email 'ops@example.com'
```

Then:

```bash
sudo systemctl enable --now deploycore-agent
```

### 7.2 Registration lifecycle

1. Installer writes durable `/etc/deploycore-agent/agent.env` (no long-lived token).  
2. Installer writes one-time `/etc/deploycore-agent/registration.env` containing `AGENT_REGISTRATION_TOKEN`.  
3. systemd loads both `EnvironmentFile=` entries.  
4. First Agent start → `POST /api/v1/agents/register`.  
5. Durable `/var/lib/deploycore-agent/credentials.json` (0600).  
6. Agent deletes `registration.env`.  
7. Restarts use **credentials only** — do not reuse registration tokens.

### 7.3 Paths created

| Path | Role |
|---|---|
| `/usr/local/bin/deploycore-agent` | Binary |
| `/etc/deploycore-agent/agent.env` | Durable env |
| `/etc/deploycore-agent/registration.env` | One-time (removed) |
| `/var/lib/deploycore-agent/` | Data, workspaces, backups, updates, traefik |
| `/var/lib/deploycore-agent/credentials.json` | Agent identity |
| `/var/lib/deploycore-agent/command-journal.json` | Replay journal |
| `/var/log/deploycore-agent` | Log directory (primary logs via journald) |
| `/etc/systemd/system/deploycore-agent.service` | Unit |

User/group: `deploycore` (+ `docker` supplementary group).

---

## 8. systemd operations

```bash
sudo systemctl status deploycore-agent
sudo systemctl start deploycore-agent
sudo systemctl stop deploycore-agent
sudo systemctl restart deploycore-agent
sudo systemctl enable deploycore-agent
sudo journalctl -u deploycore-agent -f
```

### Healthy indicators (logs)

- `agent starting in REGISTERED state`  
- `transport connected successfully` / `control plane connectivity OK`  
- `command executor started`  
- `docker connectivity OK` → Agent may report **HEALTHY**  
- Docker down → `docker connectivity degraded`, Agent **DEGRADED** (still heartbeating)

**Do not** paste credential file contents into tickets. Redact tokens in any shared logs.

`KillSignal=SIGTERM` + `TimeoutStopSec=45` exercise Agent graceful `Stop()`.

---

## 9. Server health verification

| Check | How |
|---|---|
| Agent registered | Server detail shows agent; `credentials.json` exists on host |
| Heartbeat | CP `servers.last_heartbeat_at` advancing; UI status not stuck OFFLINE |
| Docker | `docker info` on host; Agent log without persistent Docker errors |
| Polling | API access logs or Agent activity polling `/api/v1/agents/commands` |
| Traefik | `docker ps` shows `deploycore-traefik` |
| Proxy network | `docker network inspect deploycore-proxy` |

### Status meanings (current implementation)

| Status | Meaning |
|---|---|
| **ONLINE** | Recent heartbeat and Docker reported healthy/ok |
| **DEGRADED** | Heartbeat present but Docker (or related) unhealthy — e.g. Docker daemon down |
| **OFFLINE** | Heartbeat older than `AGENT_HEARTBEAT_TTL` (~90s) while previously ONLINE/DEGRADED |
| **MAINTENANCE** / **DISABLED** | Operator/admin states — not used for normal Agent health |

---

## 10. First application deployment

**TO BE EXECUTED DURING OCI LIVE VALIDATION.**

Safe shape using a public test image (example only):

1. Create **project** + **environment**.  
2. Create **application** (type image/web) targeting the registered server (`target_server_id`).  
3. Attach **domain** (DNS to Traefik host).  
4. `POST /api/v1/applications/{id}/deployments` with `{"trigger":"manual"}` (UI Deploy).  
5. Observe: deployment → revision → Agent commands (`PULL_IMAGE` / deploy path) → container → health → activation.  
6. Verify Traefik routes host on 80/443; HTTPS via Let’s Encrypt when DNS public.  
7. Open **Logs** / **Metrics** for the application/server.

Until OCI validation: treat IMAGE_FAILED / DOCKER_UNAVAILABLE as expected if Docker is absent.

**Remember R4:** job `succeeded` ≠ deployment `RUNNING`.

---

## 11. Zero-downtime / rollback

**LIVE VALIDATION** for real traffic switching.

Operator procedure (Control Plane + Agent design):

1. Deploy new revision (candidate container; proxy network withheld until activation).  
2. Health gate (`OpRunHealthCheck` / health checks).  
3. Activation enables routing (`deploycore-proxy` + Traefik labels).  
4. Previous revision drained/stopped.  
5. Rollback: deploy/activate a prior **READY/INACTIVE** revision via application rollback API/UI — **no rebuild required** when image already present.

Evidence to check: revision statuses (`ACTIVE` / `INACTIVE` / `FAILED`), deployment terminal vs `RUNNING`, replica/container health, domain still resolving.

---

## 12. Managed database

1. **Databases** → create PostgreSQL (engine/version, server, sizing).  
2. CP enqueues provision → Agent `PROVISION_DATABASE`.  
3. Status → `RUNNING` / `FAILED` (do not PATCH status to fake readiness — rejected).  
4. Credential reveal: explicit UI/API reveal with audit (`POST .../credentials/reveal`).  
5. Persistent volume: protected; soft-delete of DB record does **not** stop container or delete volume (API reports `runtimeStopped: false`).  

### Restore

- Confirm phrase must be exactly **`RESTORE`** (not the database name).  
- `POST /api/v1/backups/{backupId}/restore` with `targetDatabaseId` + `confirm: "RESTORE"`.  
- Same-database restore only — **no cross-database restore** as a supported v1 feature.  
- **Never** use Control Plane PostgreSQL as target.

---

## 13. Backup / restore

| Step | Action |
|---|---|
| Create | `POST /api/v1/databases/{databaseId}/backups` |
| Status | List backups; wait for terminal success/failure |
| Checksum | Inspect backup metadata/checksum fields |
| Restore | §12 confirm `RESTORE` |
| Verify | App connectivity / DB status after Agent completes |

**v1 limitation (R17):** local single-host backup destination.  
**LIVE VALIDATION:** real `pg_dump` / `pg_restore` on OCI.

---

## 14. Logs / metrics / events

| Signal | Where |
|---|---|
| Runtime / build logs | Application logs UI; Agent → `POST /api/v1/agents/logs` (server-scoped) |
| Metrics | `/metrics` dashboard; `GET /servers/{id}/metrics`; Agent → `POST /api/v1/agents/metrics` |
| Events | Deployment events; Docker event watcher → log ingest |

Continuous metrics transport and frontend wiring: **I10**.  
Real Docker telemetry under load: **LIVE VALIDATION**.

---

## 15. Failure / recovery operations (from I8)

| Situation | Operator response |
|---|---|
| Agent OFFLINE | Check process/systemd, network to CP, credentials; restart unit; wait for heartbeat TTL recovery |
| Agent DEGRADED | Usually Docker; fix Docker; Agent should stay up and resume |
| Docker unavailable | Structured command failures; do not mark deploy success manually |
| CP restart | Leave PostgreSQL intact; Agent reconnects; durable state in DB |
| Agent restart | Uses `credentials.json`; no new registration token |
| Failed deployment | Remains terminal; create **new** deployment; do not rewrite history |
| Failed DB provision | Remains FAILED; fix root cause; reprovision/new resource as designed |
| Failed backup/restore | Remains failed; inspect Agent command error; retry new operation |
| Expired/stuck command | CP expires stale in-flight by `expires_at`; inspect `agent_commands` |

**Do not** manually `UPDATE` deployment/database status rows in CP PostgreSQL.

---

## 16. Safe restart order

No hard coded global orchestrator dependency beyond systemd `After=`/`Requires=docker.service` for the Agent.

**Suggested operator order on a colocated host:**

1. Control Plane PostgreSQL  
2. API  
3. Frontend  
4. Docker  
5. Traefik (Docker restart policy `unless-stopped`; comes back with Docker)  
6. Agent (`systemctl restart deploycore-agent`)

On managed-only hosts: Docker → Traefik (auto) → Agent.

---

## 17. Troubleshooting

| Symptom | Likely component | Safe checks | Safe corrective action | Do-not-do |
|---|---|---|---|---|
| Server OFFLINE | Agent / network / CP | `systemctl status`, journalctl, CP reachability, heartbeat age | Restart Agent; fix network/DNS to CP | Delete server row; forge heartbeat |
| Server DEGRADED | Docker | `docker info`, Agent Docker logs | Start Docker; fix socket perms for `deploycore` | `docker system prune -a` |
| Docker unavailable | Docker Engine | socket, service | `systemctl start docker` | Expose socket on TCP |
| Agent unauthorized | Credential / token | 401 on poll; credential file presence | Re-issue registration **only** if identity lost; else restore credential file from backup | Paste secrets in chat |
| IMAGE_FAILED | Pull/registry/Docker | Command result error_code | Fix registry auth/image name; redeploy | Mark deployment RUNNING in SQL |
| Health check failed | App / health config | Candidate logs, probe settings | Fix health path/port; redeploy | Disable health to force activate |
| HTTPS/cert failure | Traefik / DNS / ACME | Traefik logs, DNS A record, `--acme-email` | Fix DNS; restart Traefik; verify ACME file perms | Disable TLS verify |
| DB provision failed | Agent / Docker / image | Command + DB `lastError` | Fix resources; retry provision path | PATCH status to RUNNING |
| Backup failed | Agent / disk | Backup status + command | Free disk; retry | Mark SUCCEEDED in SQL |
| Restore failed | Confirm / checksum / target | Confirm was `RESTORE`; checksum | Retry with correct backup/target | Cross-restore to CP DB |
| Metrics empty | Agent ingest / demo mode | DEMO flags off; Agent connected; Docker stats | Wait for heartbeat/metrics loop | Invent fixture data in prod |

---

## 18. Data safety — DO NOT

- DO NOT delete Control Plane PostgreSQL.  
- DO NOT restore managed backups into CP PostgreSQL.  
- DO NOT manually delete protected DB volumes.  
- DO NOT `docker system prune` indiscriminately.  
- DO NOT expose Docker socket remotely.  
- DO NOT disable TLS verification.  
- DO NOT reuse registration tokens.  
- DO NOT manually mark failed resources successful.  
- DO NOT place production secrets in reports/logs (`<REDACTED>`).  

---

## 19. V1 limitations (I10 dispositions)

| ID | Limitation |
|---|---|
| R1 | Full desired-state reconciliation incomplete |
| R2 | IMAGE/BUILD error-code cosmetic mismatch |
| R3 | Broader Docker failure surface needs live validation |
| R4 | Job success ≠ deployment success |
| R7 | Static environment display deferred |
| R13 | PostgreSQL major-version restore compatibility limited / live |
| R17 | Local single-host backup destination only |
| R18 | Automatic Agent update deferred (library exists, not E2E) |
| R19 | Uninstall tooling deferred |
| R20 | Public release artifacts + SHA256SUMS required before GA remote install |

Deferred features are **not** implemented. Use `--binary` + `--checksum` for OCI until R20 is closed.

---

## 20. Related documents

- [OCI live validation checklist](./oci-live-validation-checklist.md)  
- [Deployment infrastructure notes](./DEPLOYMENT.md) (pointer / historical)  
- Installer: `deployments/install/`  
- API env example: `apps/api/.env.example`  
- Capacity/placement: `apps/api/docs/CAPACITY.md`
