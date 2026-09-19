# DeployCore Module Inventory

This document maps all functional modules across the Go Control Plane (`apps/api`) and the Next.js Frontend (`apps/frontend`).

---

## Module Matrix

| Module | Backend Package (`apps/api/internal/`) | Frontend Route (`apps/frontend/app/`) | Backend Status | Frontend Status |
| :--- | :--- | :--- | :--- | :--- |
| **Authentication & Identity** | `auth/` | — (No auth routes) | **Complete** (Argon2id, JWT, sessions) | **Missing** |
| **Organizations & Tenancy** | `organizations/` | `(dashboard)/admin/organizations/` | **Complete** (CRUD, invitations, members) | **Mocked** |
| **Role-Based Access Control** | `rbac/` | `(dashboard)/security/access/` | **Complete** (Seeded roles, permissions) | **Mocked** |
| **Projects & Environments** | `projects/` | `(dashboard)/projects/` | **Complete** (Hierarchy, slugs, deletion guards) | **Mocked** |
| **Servers & Fleet Management** | `servers/`, `placement/` | `(dashboard)/servers/` | **Complete** (Capacity engine, status) | **Mocked** |
| **Agent Management & Commands** | `agents/`, `agentcmd/` | `(dashboard)/admin/agents/` | **Complete** (Registration, heartbeats, tasks) | **Mocked** |
| **Applications & Workloads** | `applications/` | `(dashboard)/applications/` | **Complete** (7 workload types, versioned config)| **Mocked** |
| **Deployments & Rollbacks** | `deployments/`, `orchestrator/` | `(dashboard)/deployments/` | **Complete** (State machine, transactional jobs) | **Mocked** |
| **Revisions** | `revisions/` | `(dashboard)/revisions/` | **Complete** (Immutable configuration snapshots) | **Mocked** |
| **Replicas & Auto-Reconcile** | `replicas/`, `reconcile/` | `(dashboard)/containers/` | **Complete** (Desired-state loop, scheduler) | **Mocked** |
| **Variables & Encrypted Secrets**| `variables/`, `secrets/` | `(dashboard)/security/secrets/` | **Complete** (AES-256-GCM envelope encryption) | **Mocked** |
| **Ingress, Domains & TLS** | `domains/` | `(dashboard)/domains/` | **Complete** (Traefik label generator, DNS/TLS) | **Mocked** |
| **Managed Databases** | `databases/` | `(dashboard)/databases/` | **Complete** (PostgreSQL provisioning, credentials) | **Mocked** |
| **Volumes & Storage** | `volumes/` | `(dashboard)/volumes/` | **Complete** (Docker volumes, protected DB mounts) | **Mocked** |
| **Backup & Restore** | `backups/` | `(dashboard)/backups/` | **Complete** (Logical dumps, restore validation) | **Mocked** |
| **Git Integrations** | `gitproviders/` | `(dashboard)/integrations/git/` | **Partial** (GitHub active; GitLab/Bitbucket stubs) | **Mocked** |
| **Container Registries** | `registries/` | `(dashboard)/integrations/registries/` | **Partial** (GHCR/DockerHub active; GCP/ECR stubs) | **Mocked** |
| **Notifications** | `notifications/` | `(dashboard)/integrations/notifications/`| **Partial** (Email/Webhook active; chat stubs) | **Mocked** |
| **Outgoing Webhooks** | `webhooks/` | `(dashboard)/integrations/webhooks/` | **Complete** (HMAC-SHA256 signing, retry worker) | **Mocked** |
| **Audit Logs** | `audit/` | `(dashboard)/security/audit/` | **Complete** (Append-only database triggers) | **Mocked** |
| **Observability & Health** | `logs/`, `metrics/`, `healthchecks/` | `(dashboard)/logs/`, `/metrics/` | **Complete** (In-memory ring buffers, SSE) | **Mocked** |
| **Super Admin Platform Ops** | `openapi/`, `jobs/` | `(dashboard)/admin/` | **Complete** (Background job worker, health) | **Mocked** |

---

## Detailed Module Descriptions

### 1. Authentication & Identity
- **Backend (`apps/api/internal/auth`)**:
  - Implements user registration, password login, refresh token rotation, logout, password reset flow, and current user retrieval (`/api/v1/auth/*`).
  - Passwords hashed using Argon2id (`pkg/crypto/password.go`).
  - Session tokens tracked in `sessions` with rotation family trees to detect token reuse attacks.
- **Frontend**:
  - Currently has no UI routes or forms for login, registration, or session tracking. All routes in `app/(dashboard)/` are publicly accessible without authentication.

### 2. Organizations & RBAC
- **Backend (`apps/api/internal/organizations`, `apps/api/internal/rbac`)**:
  - Organizations act as the top-level tenancy boundary.
  - Organization invitations with email notifications and acceptance endpoints (`/api/v1/invitations/accept`).
  - RBAC engine evaluates 6 built-in roles (`Owner`, `Administrator`, `DevOps`, `Developer`, `Support`, `Viewer`) against granular permission keys (e.g. `application.deploy`, `database.create`).
- **Frontend**:
  - Organization switcher (`components/platform/organization-switcher.tsx`) and team access management pages (`app/(dashboard)/security/access/page.tsx`) populate from mock data.

### 3. Projects & Environments
- **Backend (`apps/api/internal/projects`)**:
  - Two-level grouping for applications: Projects contain one or more Environments (`production`, `staging`, `development`, `preview`, `custom`).
  - Strict deletion guards: projects cannot be deleted while containing active environments; environments cannot be deleted while applications exist.
- **Frontend**:
  - Detailed overview boards, environment cards, resource panels, and creation dialogs in `components/deploycore/projects/`.

### 4. Servers, Agents & Capacity Placement
- **Backend (`apps/api/internal/servers`, `apps/api/internal/agents`, `apps/api/internal/placement`)**:
  - Servers represent physical or virtual Docker host nodes.
  - Agents authenticate via bearer credentials derived from one-time registration tokens.
  - Capacity engine tracks CPU millicores, RAM bytes, and disk allocations from deployed applications.
  - Placement scheduler scores candidate nodes based on resource availability (`least_loaded`, `most_free`) or respects explicit server pinning.
- **Frontend**:
  - Fleet management table, server hardware resource usage bars, maintenance mode toggle cards, and add-server wizard in `components/deploycore/servers/`.

### 5. Applications, Deployments & Revisions
- **Backend (`apps/api/internal/applications`, `apps/api/internal/deployments`, `apps/api/internal/revisions`, `apps/api/internal/orchestrator`)**:
  - Workload types: `WEB_SERVICE`, `API`, `WORKER`, `SCHEDULED_JOB`, `STATIC_SITE`, `DOCKER_COMPOSE`, `DOCKER_IMAGE`.
  - Application configuration is versioned immutably in `application_configs`.
  - Deployment state machine handles 21 distinct status codes.
  - Revisions provide immutable runtime snapshots enabling zero-rebuild rollbacks.
- **Frontend**:
  - Comprehensive 7-step application creation wizard (`create-application-wizard.tsx`), deployment pipeline visualization (`deployment-pipeline.tsx`), revision comparison view, and rollback confirmation dialogs.

### 6. Variables & Encrypted Secrets
- **Backend (`apps/api/internal/variables`, `apps/api/internal/secrets`)**:
  - 4-tier hierarchical variable resolution (`ORGANIZATION` → `PROJECT` → `ENVIRONMENT` → `APPLICATION`).
  - Envelope encryption via AES-256-GCM. Secret values are never returned over HTTP once set (only metadata, key ID, and version).
- **Frontend**:
  - Environment variable matrix editor (`environment-variables-editor.tsx`) and secrets manager (`secrets-manager.tsx`) with secret masking components (`secret-field.tsx`).

### 7. Ingress, Domains & Traefik Routing
- **Backend (`apps/api/internal/domains`)**:
  - Manages custom domain assignments with DNS verification status (`PENDING`, `VALID`, `INVALID`) and TLS certificate issuance states.
  - Dynamically synthesizes Traefik routing labels (`traefik.http.routers.*`) returned in the API contract for server agents to apply.
- **Frontend**:
  - Domain records table and domain detail page (`app/(dashboard)/domains/[domainId]/page.tsx`).

### 8. Managed Databases, Volumes & Backups
- **Backend (`apps/api/internal/databases`, `apps/api/internal/volumes`, `apps/api/internal/backups`)**:
  - Control-plane management of containerized PostgreSQL database instances.
  - Volume tracking with protection flags to prevent accidental database data loss.
  - Backup creation and restore validation with checksum verification.
- **Frontend**:
  - Database overview cards, volume tables, and backup run timelines in `components/deploycore/databases/`, `volumes/`, and `backups/`.

### 9. Integrations (Git, Registries, Notifications, Webhooks)
- **Backend (`apps/api/internal/gitproviders`, `apps/api/internal/registries`, `apps/api/internal/notifications`, `apps/api/internal/webhooks`)**:
  - GitHub integration supporting webhook parsing and push-triggered deployments.
  - Container registries: DockerHub, GHCR, and OCI registries.
  - Asynchronous event notifications (deployment success/failure, server offline, disk low) via email log sink and outgoing webhooks with HMAC signatures.
- **Frontend**:
  - Dedicated pages under `app/(dashboard)/integrations/` for git providers, registries, notification policies, and webhook endpoints.

### 10. Observability & Telemetry
- **Backend (`apps/api/internal/logs`, `apps/api/internal/metrics`, `apps/api/internal/healthchecks`)**:
  - In-memory circular buffers for build/runtime logs and metric samples.
  - Server-Sent Events (SSE) streaming for real-time deployment and runtime logs.
  - Health check policies (`HTTP`, `TCP`, `COMMAND`, `CONTAINER`) with probe history.
- **Frontend**:
  - Live log explorer with ANSI syntax highlighting (`log-viewer.tsx`), metric dashboards using Recharts (`metrics-dashboard.tsx`), and health check status badges.
