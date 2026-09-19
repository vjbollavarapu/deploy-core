# DeployCore Architecture

This document describes the actual architecture of the DeployCore platform as implemented in the `deploy-core` monorepo.

---

## 1. System Overview & Planes of Concern

DeployCore separates control plane orchestration from physical execution:

```text
┌────────────────────────────────────────────────────────────────────────┐
│                        PRESENTATION PLANE                              │
│                 apps/frontend (Next.js 16.3 App Router)                 │
│          UI Shell • Workload Dashboards • Multi-Step Wizards           │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ HTTP / JSON API (OpenAPI 3.1)
                                    │ Bearer JWT Authentication
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                          CONTROL PLANE                                 │
│                      apps/api (Go 1.24.2 Monolith)                     │
│  - Authentication & RBAC        - Deployment State Machine             │
│  - Multi-Tenancy Resolution     - Placement & Capacity Engine          │
│  - Envelope Secret Encryption   - PostgreSQL Job Worker & Queue        │
│  - Desired-State Reconciler     - Real-Time Log & Metric Streams (SSE) │
└──────────────────┬─────────────────────────────────┬───────────────────┘
                   │ SQL (pgx/v5 Pool)               │ Structured Agent Commands
                   ▼                                 ▼ (Agent Bearer Auth)
┌─────────────────────────────────────┐  ┌───────────────────────────────┐
│           DATABASE LAYER            │  │        EXECUTION PLANE        │
│           PostgreSQL 16+            │  │     apps/agent (Planned)      │
│  - 25 Embedded SQL Migrations       │  │  - Remote Docker Engine Host  │
│  - Transactional Jobs (SKIP LOCKED) │  │  - Traefik Reverse Proxy      │
│  - Append-Only Audits & Events      │  │  - Container & Volume Ops     │
└─────────────────────────────────────┘  └───────────────────────────────┘
```

---

## 2. Backend Architecture (`apps/api`)

The backend is structured as a modular monolith in Go with strict domain boundaries.

### 2.1 Package & Layer Organization
All domains reside under `apps/api/internal/` and follow a standard 4-tier layer:
1. **HTTP Handler (`handler.go`)**:
   - Mounts routes on `http.ServeMux` using Go 1.22+ pattern strings (`"GET /api/v1/projects/{projectId}"`).
   - Parses path parameters with `r.PathValue()` and decodes JSON bodies.
   - Enforces authentication using `authHandler.RequireAuth` or `agentHandler.RequireAgent`.
   - Returns responses using `pkg/apierror` envelopes and `pkg/pagination`.
2. **Domain Service (`service.go`)**:
   - Coordinates business logic and transactional flows.
   - Enforces organization-scoped role permissions via `rbac.Authorizer`.
   - Logs security and operational actions through `audit.Writer`.
3. **PostgreSQL Repository (`repository.go`)**:
   - Executes parameterized queries directly against `*pgxpool.Pool`.
   - Never leaks database transactions outside the repository/service boundary.
4. **Domain Definitions (`domain.go`)**:
   - Defines canonical Go structs, validation rules, enums, and request/response shapes.

### 2.2 Global Middleware Pipeline
Incoming HTTP requests pass through an ordered chain in `internal/server/middleware.go`:
1. `recoverMiddleware`: Traps panics, logs stack traces via `log/slog`, and returns standard HTTP 500 JSON envelopes.
2. `requestIDMiddleware`: Reads incoming `X-Request-ID` or generates a new UUID v4, attaching it to context and response headers.
3. `security.MaxBytesMiddleware`: Enforces a hard request body limit (default: 1 MB via `MAX_REQUEST_BODY_BYTES`).
4. `secureHeadersMiddleware`: Injects standard security headers (`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, `Cache-Control: no-store`).
5. `corsMiddleware`: Validates origins against `CORS_ALLOWED_ORIGINS` and handles preflight `OPTIONS` requests.
6. `loggingMiddleware`: Emits structured JSON log entries recording request ID, HTTP method, sanitized path, status code, duration, and remote IP.

### 2.3 Background Workers & Orchestration
- **PostgreSQL Job Queue (`internal/jobs`)**:
  - Implements durable background task processing backed by the `jobs` table.
  - Workers use `SELECT FOR UPDATE SKIP LOCKED` with leased locks (`JOB_LEASE_TTL`), heartbeat renewal, and exponential retry backoff.
  - Job types: `DEPLOYMENT_EXECUTION`, `BACKUP`, `RESTORE`, `NOTIFICATION_DELIVERY`, `WEBHOOK_DELIVERY`, `REPLICAS_RECONCILE`, `DESIRED_STATE_RECONCILE`.
- **Deployment State Machine (`internal/deployments`)**:
  - Governs application transitions across 21 discrete states:
    `PENDING` → `QUEUED` → `PREPARING` → `FETCHING_SOURCE` → `BUILDING` → `IMAGE_READY` → `CREATING_CONTAINER` → `STARTING` → `HEALTH_CHECKING` → `ACTIVATING` → `RUNNING`.
  - Failures transition to specific phase failures (`BUILD_FAILED`, `START_FAILED`, `HEALTH_CHECK_FAILED`) while preserving previous active revisions.
- **Desired-State Reconciler (`internal/reconcile`)**:
  - Periodically checks running container states against desired application configurations.
  - Detects offline agents, restarts unhealthy containers with exponential backoff, and cleans up dead replicas.
- **In-Memory Streaming Buffers**:
  - `internal/logs`: Bounded in-memory circular buffer for build and runtime logs, providing SSE streams via `/events/stream` and `/logs?follow=true`.
  - `internal/metrics`: Bounded 512-point circular buffer for real-time CPU/RAM time-series queries.

---

## 3. Frontend Architecture (`apps/frontend`)

The frontend is built with Next.js 16.3.3 utilizing App Router and React 19.

### 3.1 Layout & Navigation Hierarchy
- `app/layout.tsx`: Root layout with Geist font, `ThemeProvider` (dark mode default), `TooltipProvider`, and Vercel Analytics.
- `app/(dashboard)/layout.tsx`: Wraps pages in `components/platform/app-shell.tsx`, providing `AppSidebar` and `TopBar`.
- Sub-navigation layouts:
  - `app/(dashboard)/applications/[applicationId]/layout.tsx`: Contextual application sub-navigation (Overview, Deployments, Revisions, Networking, Logs, Metrics, Settings).
  - `app/(dashboard)/servers/[serverId]/layout.tsx`: Contextual server sub-navigation (Overview, Containers, Metrics, Maintenance, Settings).
  - `app/(dashboard)/databases/[databaseId]/layout.tsx`: Managed database sub-navigation (Overview, Backups, Volumes, Logs, Settings).
  - `app/(dashboard)/admin/layout.tsx`: Control plane administration sub-navigation.

### 3.2 Design System & Component Structure
- **Design Tokens**: Tailwind CSS v4 using CSS variable theme definitions in `app/globals.css`.
- **UI Primitives (`components/ui/`)**: 36 components based on Radix UI, Base-UI, and shadcn/ui patterns (`button`, `dialog`, `table`, `field`, `sidebar`, `sheet`, `sonner`).
- **Platform Primitives (`components/platform/`)**: Reusable high-level components (`data-table`, `filter-bar`, `search-input`, `metric-card`, `log-viewer`, `deployment-pipeline`).
- **Domain Components (`components/deploycore/`)**: Workload-specific components (`create-application-wizard`, `server-metrics-chart`, `revision-compare`, `database-overview`).
- **Status & Form Validations**: Zod validation schemas in `lib/validations/` paired with `react-hook-form`.

### 3.3 Current Data Flow State
The frontend currently functions as a high-fidelity visual prototype:
- All pages consume fixtures from `apps/frontend/lib/mock-data.ts`.
- Multi-step wizards and mutation dialogs simulate asynchronous delays (`setTimeout`) and trigger Sonner notifications.
- The wire contract exists in `apps/frontend/lib/api/contract.ts` (generated from backend OpenAPI), ready to be wired to a real API client.

---

## 4. Database Architecture

PostgreSQL is the single source of truth for all metadata and durable queues.

### 4.1 Schema Conventions
- **Identifiers**: UUID v4 primary keys (`gen_random_uuid()`).
- **Timestamps**: All timestamps are `TIMESTAMP WITH TIME ZONE` stored in UTC.
- **Soft Deletes**: Used selectively on top-level recoverable entities (`organizations`, `projects`, `environments`, `servers`, `applications`, `domains`, `secrets`, `git_connections`, `registries`).
- **Append-Only Immutability**:
  - `audit_logs`: A PostgreSQL trigger (`tg_audit_logs_no_update`) blocks all `UPDATE` queries.
  - `deployment_events`: Sequential timeline records documenting every state machine transition.

### 4.2 Entity Relational Model
```text
organizations (tenant root)
 ├── organization_members ── member_roles ── roles ── role_permissions ── permissions
 ├── teams ── team_members
 ├── projects
 │    └── environments
 │         └── applications
 │              ├── application_configs (versioned snapshots)
 │              ├── deployments ── deployment_events
 │              ├── revisions (immutable deploy targets)
 │              ├── application_replicas
 │              └── domains ── certificates
 ├── servers
 │    ├── server_agents (durable credential hash)
 │    ├── server_heartbeats (sampled telemetry)
 │    └── agent_commands (structured task queue)
 ├── managed_databases ── backups
 ├── volumes
 ├── environment_variables (scoped: org / project / env / app)
 ├── secrets ── secret_versions (AES-256-GCM encrypted ciphertext)
 ├── git_connections ── git_repositories
 ├── registries
 ├── notification_channels ── notification_policies ── notification_deliveries
 ├── outgoing_webhooks ── outgoing_webhook_deliveries
 └── jobs (PostgreSQL-backed worker queue)
```

---

## 5. Important Architectural & Security Decisions

1. **Strict Control vs. Execution Separation**:
   The API never executes remote shell commands or connects directly to Docker sockets. All operations are represented as structured domain jobs translated into signed, validated `agent_commands` for the host server agent.
2. **Envelope Encryption for Secrets**:
   Platform secrets (`variables`, database credentials, registry tokens, webhook signing keys) are encrypted using AES-256-GCM (`pkg/crypto/aead.go`). Plaintext secrets are never stored in the database, logged, or recorded in audit metadata.
3. **Session Family Tracking & Reuse Detection**:
   Refresh tokens are hashed (SHA-256) and tracked by `family_id`. If a previously-rotated refresh token is replayed, the entire session family is invalidated immediately to mitigate token compromise.
4. **Agent Simulation Mode (`ORCHESTRATOR_SIMULATE_AGENT`)**:
   Enables full local development and end-to-end testing of complex deployment pipelines and database provisioning without requiring active physical host servers.
5. **Deterministic Capacity Placement**:
   The scheduler filters candidate servers by health, maintenance status, and labels, calculating CPU millis and memory byte reservations before placement, rejecting overcommits with `INSUFFICIENT_RESOURCES`.
