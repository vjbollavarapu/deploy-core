# DeployCore Current Status & Roadmap

This document outlines the current state of completion, known gaps, technical debt, and recommended next development steps for the `deploy-core` monorepo.

---

## 1. Completed Functionality

### Control Plane Backend (`apps/api`)
- **Core HTTP & Security Foundation**:
  - Structured request logging (`log/slog`), request ID propagation (`X-Request-ID`), panic recovery, body size limiting, strict CORS, and security headers.
  - Rate limiting on public, authentication, and webhook ingress routes.
- **Database Migrations**:
  - 25 embedded PostgreSQL migrations (`000001_bootstrap` through `000025_security_hardening`) with transactional application on startup.
  - Soft-delete semantics with active slug uniqueness constraints.
- **Identity, Auth & RBAC**:
  - User registration, Argon2id password hashing, login, logout, password reset tokens.
  - HMAC-SHA256 JWT access tokens paired with hashed refresh tokens in `sessions`.
  - Session family tracking with automatic revocation upon token replay detection.
  - Full RBAC authorizer with 6 seeded roles (`Owner`, `Administrator`, `DevOps`, `Developer`, `Support`, `Viewer`) evaluating granular permission keys.
- **Workloads & Deployment Orchestration**:
  - Support for 7 application types (`WEB_SERVICE`, `API`, `WORKER`, `SCHEDULED_JOB`, `STATIC_SITE`, `DOCKER_COMPOSE`, `DOCKER_IMAGE`).
  - Immutable versioned configs (`application_configs`) and revision snapshots (`revisions`).
  - 21-state deployment state machine with transactional transitions and append-only event logging.
  - Safe rollbacks to previous revisions without rebuilding images.
- **Capacity & Placement Engine**:
  - Deterministic capacity scheduling calculating CPU millicores, RAM bytes, and disk reservations.
- **Stateful Resource Lifecycle**:
  - Managed PostgreSQL database lifecycle with volume protection guards.
  - Volume creation, attachment, and deletion tracking.
  - Database logical backup jobs with checksum validation and restore gates.
- **Configuration & Secrets**:
  - AES-256-GCM envelope encryption for secrets at rest.
  - 4-tier variable inheritance (`ORGANIZATION` → `PROJECT` → `ENVIRONMENT` → `APPLICATION`).
- **Telemetry & Background Engine**:
  - PostgreSQL job queue (`FOR UPDATE SKIP LOCKED`) with lease heartbeat and retry workers.
  - Desired-state container reconciliation loop.
  - In-memory circular buffers for real-time build/runtime logs and metric snapshots with SSE streaming.
  - OpenAPI 3.1 schema generation and contract exports.

### Frontend Presentation (`apps/frontend`)
- **Design System & Shell**:
  - Full dark mode theme configured with Tailwind CSS v4 design tokens.
  - Complete application shell with collapsible navigation sidebar, breadcrumbs, search command palette, and organization switcher.
- **Route Hierarchy**:
  - 54 complete page routes across workloads, servers, databases, deployments, revisions, containers, storage, networks, domains, security, and administrative features.
- **Complex UI Workflows**:
  - 7-step Application creation wizard with inline Zod validation and React Hook Form.
  - Server metrics charts and resource gauges powered by Recharts.
  - Deployment pipeline timeline, build log viewer with ANSI styling, and revision comparison diff views.

---

## 2. Partially Completed Functionality

1. **Git Providers (`internal/gitproviders`)**:
   - GitHub is implemented with webhook signature verification and push triggers.
   - GitLab, Bitbucket, and generic Git providers are currently `stubProvider` implementations.
2. **Container Registries (`internal/registries`)**:
   - GHCR, DockerHub, and generic OCI registries are functional.
   - AWS ECR, Google Artifact Registry (GCP), and Azure ACR are currently `stubProvider` implementations.
3. **Notification Channels (`internal/notifications`)**:
   - Email (log sink) and Webhooks are active.
   - Slack, Discord, Microsoft Teams, Telegram, and WhatsApp channels are reserved stubs.
4. **Storage Adapters**:
   - Log streaming is strictly in-memory (no persistent Loki, OpenSearch, or ClickHouse adapter).
   - Metrics are strictly in-memory ring buffers (no Prometheus exporter).
   - Backup destinations support only `local://` platform URIs (no S3 / blob storage adapter).

---

## 3. Gaps & Disconnects (Current Gaps)

1. **Frontend-to-Backend Disconnection**:
   - The frontend does not execute network requests against `apps/api`.
   - All pages and components consume static mock fixtures from `apps/frontend/lib/mock-data.ts` (66 KB).
   - Mutation actions (create app, add server, create project) simulate execution via `setTimeout` and Sonner toasts.
2. **Missing Frontend Authentication**:
   - No login, registration, password reset, or invitation acceptance pages exist in `apps/frontend`.
   - No token storage, session context, or route guard middleware is configured.
3. **Missing Server Agent (`apps/agent`)**:
   - The execution plane does not exist.
   - The control plane relies on `ORCHESTRATOR_SIMULATE_AGENT=true` to simulate deployments and database provisioning.
4. **Duplicated Frontend Components**:
   - 9 components exist duplicated in both `components/deploycore/` and `components/platform/` (`app-sidebar.tsx`, `status-badge.tsx`, `resource-usage-bar.tsx`, `health-indicator.tsx`, `command-palette.tsx`, `empty-state.tsx`, `metric.tsx`, `page-header.tsx`, `build-log-viewer.tsx`).

---

## 4. Known Technical Debt & Issues

- **Isolated Go Toolchain**:
  - Go compiler is located at `apps/api/.tools/go/bin/go` and is not on the default system PATH.
- **Lack of Monorepo Automation**:
  - No root `package.json`, `pnpm-workspace.yaml`, or `Makefile` exists to run the monorepo apps together.
- **Missing Containerization**:
  - No `Dockerfile` or `docker-compose.yml` exists for either application.
- **Missing CI/CD**:
  - No GitHub Actions workflows exist to validate builds, linting, or tests.
- **React Compiler Warning**:
  - `components/deploycore/servers/add-server-wizard.tsx` triggers a React Compiler bailout warning on React Hook Form's `watch()`.

---

## 5. Recommended Next Tasks (Prioritized)

### Phase 1: Local Developer Experience (DX)
1. Provide a root `docker-compose.yml` to spin up local PostgreSQL 16.
2. Provide a root `Makefile` to start the database, run migrations, run the API, and launch the frontend.
3. Provide PATH wrapper or instructions for the bundled Go compiler.

### Phase 2: Frontend Auth & API Client Integration
1. Build an authenticated API fetch client in `apps/frontend/lib/api/client.ts`.
2. Implement auth routes (`/login`, `/register`, `/forgot-password`, `/reset-password`) and an `AuthProvider` with token storage.
3. Add Next.js middleware to protect `(dashboard)` routes.

### Phase 3: Wire Frontend to Live API
1. Wire **Organizations & Projects** pages to live API endpoints.
2. Wire **Servers & Fleet** management pages.
3. Wire **Applications & Deployments** pages, streaming live build/runtime logs via Server-Sent Events (SSE).
4. Deprecate and remove `apps/frontend/lib/mock-data.ts`.

### Phase 4: Component Hygiene & Modernization
1. Delete duplicate components in `components/deploycore/` and standardize on `@/components/platform/`.
2. Fix the `watch()` compiler warning in `add-server-wizard.tsx`.

### Phase 5: Containerization & CI/CD
1. Add production multi-stage Dockerfiles for `apps/api` and `apps/frontend`.
2. Add GitHub Actions CI workflows for automated testing, typechecking, and linting.

### Phase 6: Execution Plane (`apps/agent`)
1. Scaffold `apps/agent` based on specifications in `PROMPT_1.md` to communicate with the control plane and execute container operations on host Docker engines.
