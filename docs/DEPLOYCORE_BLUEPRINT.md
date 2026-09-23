# DeployCore — Master Product & Technical Architecture Blueprint

> **Document Type:** Master Product & Technical Blueprint  
> **Source Repository:** `deploy-core` monorepo  
> **Repository Commit SHA:** `c3b44b936adad5620aced5251827801a66103296`  
> **Target Audience:** Technical Founders, Enterprise Architects, Principal Engineers, DevOps Specialists, Evaluators, Contributors  
> **Authoritative Baseline:** Validated against actual codebase implementation (`apps/api`, `apps/agent`, `apps/frontend`, `packages/protocol-go`).

---

## Table of Contents

1. [Executive Product Overview](#1-executive-product-overview)
2. [Product Capability Map](#2-product-capability-map)
3. [Complete Module Inventory](#3-complete-module-inventory)
4. [System Architecture](#4-system-architecture)
5. [Repository & Monorepo Architecture](#5-repository--monorepo-architecture)
6. [Application Deployment Lifecycle](#6-application-deployment-lifecycle)
7. [Revision Model & State Semantics](#7-revision-model--state-semantics)
8. [Rollback Architecture & Mechanics](#8-rollback-architecture--mechanics)
9. [Server & Agent Architecture](#9-server--agent-architecture)
10. [Networking & Ingress Architecture](#10-networking--ingress-architecture)
11. [Database & Persistent Storage Architecture](#11-database--persistent-storage-architecture)
12. [Security Architecture & Trust Boundaries](#12-security-architecture--trust-boundaries)
13. [Secrets Management Lifecycle](#13-secrets-management-lifecycle)
14. [Observability & Telemetry](#14-observability--telemetry)
15. [Background Processing & Job System](#15-background-processing--job-system)
16. [Desired-State Reconciliation](#16-desired-state-reconciliation)
17. [Frontend Product Map & UI Architecture](#17-frontend-product-map--ui-architecture)
18. [API Architecture & Conventions](#18-api-architecture--conventions)
19. [Roles, Permissions & Multi-Tenancy (RBAC)](#19-roles-permissions--multi-tenancy-rbac)
20. [Failure & Recovery Model](#20-failure--recovery-model)
21. [Application Data Model](#21-application-data-model)
22. [Deployment Topology Examples](#22-deployment-topology-examples)
23. [Technology Stack & Dependency Inventory](#23-technology-stack--dependency-inventory)
24. [Configuration & Environment Variables](#24-configuration--environment-variables)
25. [Implementation Status Matrix](#25-implementation-status-matrix)
26. [Known Limitations & Technical Debt](#26-known-limitations--technical-debt)
27. [Current Product Footprint & Engineering Assessment](#27-current-product-footprint--engineering-assessment)
28. [Glossary of Terms](#28-glossary-of-terms)
29. [Repository Analysis Metadata](#29-repository-analysis-metadata)

---

## 1. Executive Product Overview

### What DeployCore Is
DeployCore is a self-hosted, multi-tenant container deployment and infrastructure control plane. It orchestrates application lifecycles, configuration versions, container placements, zero-downtime rolling deployments, automated rollbacks, managed stateful databases, persistent volumes, dynamic TLS certificates, and edge ingress routing across distributed fleets of bare-metal and cloud Linux servers.

### Primary Problem It Solves
Deploying containerized workloads across raw virtual machines (e.g., AWS EC2, Hetzner, DigitalOcean, bare-metal servers) typically forces engineering teams into one of two extremes:
1. **Manual / Ad-Hoc Docker Operations**: Developers SSH into hosts, run manual `docker run` or `docker compose` commands, copy `.env` files insecurely, manage port bindings by hand, configure reverse proxies with error-prone config file editing, and perform downtime-heavy deployments with no automated rollbacks, revision histories, or audit trails.
2. **Heavyweight Orchestrators (Kubernetes)**: Imposes extreme operational complexity, steep learning curves, significant control plane overhead (etcd, control-plane nodes, CNI overlays), and high infrastructure costs for small-to-medium teams.

DeployCore provides a unified, declarative control plane that delivers Heroku/Vercel-like developer ergonomics on standard Docker engines running on private infrastructure, without the overhead of Kubernetes.

### Intended Users
* **Engineering Teams & DevOps**: Requiring automated Git-push and container image deployments across owned infrastructure with immutable revisions and one-click rollbacks.
* **Platform Engineers**: Managing multi-tenant environments with strict role-based access control (RBAC), quota enforcement, and tamper-resistant audit trails.
* **Organizations with Compliance or Data-Sovereignty Constraints**: Organizations that cannot run workloads on proprietary public PaaS platforms and must retain strict governance over where compute, databases, and secrets reside.

### Core Value Proposition
* **Decoupled Architecture**: A centralized Go control plane (`apps/api`) manages metadata, tenancy, placement scheduling, and orchestration workflows, while a lightweight, autonomous Go daemon (`apps/agent`) executes structured operational instructions on host Docker engines.
* **Immutable Revisions & Safe Rollbacks**: Every build and configuration change produces a discrete, immutable revision snapshot. Rollbacks reuse pre-built, verified image digests without re-fetching Git source or re-running builds.
* **Zero-Downtime Routing via Traefik**: Integration with edge reverse proxies enables health-gated rolling activation: new candidate containers must pass HTTP/TCP health checks before edge traffic is dynamically switched and old containers are drained.
* **Security & Multi-Tenancy**: Organization isolation at every database query, Argon2id password hashing, session revocation upon token reuse, envelope encryption (AES-256-GCM) for secrets at rest, and zero raw shell command execution across the agent boundary.

### How It Differs From Manually Operating Docker
```
Manual Docker Host Operations             DeployCore Managed Orchestration
┌────────────────────────────────┐        ┌────────────────────────────────┐
│ SSH login to host              │        │ Web Dashboard / REST API       │
│ Unversioned .env on disk       │   vs   │ AES-256-GCM Envelope Secrets   │
│ docker stop && docker run      │        │ Zero-Downtime Rolling Activate │
│ Port collision conflicts       │        │ Traefik Service Discovery      │
│ No audit record                │        │ Append-Only Audit Trail (B27)  │
│ Host outage drops workload     │        │ Automated Placement & Health   │
└────────────────────────────────┘        └────────────────────────────────┘
```

---

## 2. Product Capability Map

The following hierarchical capability tree reflects the actual implementation discovered in the repository:

```text
DeployCore Control Plane
├── Tenancy & Authentication
│   ├── User Registration & Argon2id Auth [IMPLEMENTED]
│   ├── JWT Access & Session Families with Token Reuse Detection [IMPLEMENTED]
│   ├── Organizations & Member Invitations [IMPLEMENTED]
│   └── Granular RBAC (6 Roles, 40+ Permission Keys) [IMPLEMENTED]
├── Workload Management
│   ├── Projects & Multi-Stage Environments (Prod/Staging/Dev) [IMPLEMENTED]
│   ├── Workload Types (Web Service, API, Worker, Scheduled Job, Static Site, Docker Image) [IMPLEMENTED]
│   ├── Declarative Application Configs (CPU/RAM limits, ports, restart policies) [IMPLEMENTED]
│   ├── Automated Capacity Scheduling & Server Placement [IMPLEMENTED]
│   └── Multi-Replica Scaling & Instance Tracking [IMPLEMENTED]
├── Deployment Orchestration
│   ├── 21-State Deployment Machine [IMPLEMENTED]
│   ├── Source Acquisition (Git Clone / OCI Pull) [IMPLEMENTED]
│   ├── Container Build & Image Digest Pinning [IMPLEMENTED]
│   ├── Multi-Probe Health Checking (HTTP, TCP, Command) [IMPLEMENTED]
│   ├── Health-Gated Edge Activation [IMPLEMENTED]
│   ├── Rolling Deployment Traffic Switching [IMPLEMENTED]
│   └── Instant Rollback (Zero-Build Reused Revision) [IMPLEMENTED]
├── Server Fleet & Execution Plane
│   ├── Server Registration via One-Time Tokens [IMPLEMENTED]
│   ├── Server Agent Daemon (apps/agent) [IMPLEMENTED]
│   ├── Mutual HMAC/Token Authenticated Agent Protocol (packages/protocol-go) [IMPLEMENTED]
│   ├── Hardware Telemetry & Heartbeats (CPU, RAM, Disk via gopsutil) [IMPLEMENTED]
│   ├── Server Maintenance Mode & Workload Eviction Protection [IMPLEMENTED]
│   └── Structured Agent Operations (No raw shell execution) [IMPLEMENTED]
├── Ingress, Domains & TLS
│   ├── Custom Hostname Mapping [IMPLEMENTED]
│   ├── DNS Verification Engine (CNAME / A Records) [IMPLEMENTED]
│   ├── TLS Certificate Lifecycle Tracking (Pending/Issuing/Active) [IMPLEMENTED]
│   └── Traefik Dynamic Label Injection & Routing Rules [IMPLEMENTED]
├── Stateful Databases & Volumes
│   ├── Managed Containerized Databases (PostgreSQL engine) [IMPLEMENTED]
│   ├── Protected Persistent Storage Volumes & Bind Mounts [IMPLEMENTED]
│   ├── Audited Credential Storage & Decryption Reveal API [IMPLEMENTED]
│   ├── Logical Database Backup Jobs [IMPLEMENTED]
│   └── Destructive Database Snapshot Restore with Safeguards [IMPLEMENTED]
├── Configuration & Secrets Management
│   ├── Hierarchical Environment Variable Inheritance [IMPLEMENTED]
│   ├── Encrypted Secrets (AES-256-GCM Envelope Encryption) [IMPLEMENTED]
│   └── Masked Secret Presentation (Metadata-only API responses) [IMPLEMENTED]
├── Platform Integrations
│   ├── Git Providers (GitHub active; GitLab/Bitbucket stubs) [PARTIALLY IMPLEMENTED]
│   ├── Container Registries (DockerHub/GHCR active; ECR/GCR/ACR stubs) [PARTIALLY IMPLEMENTED]
│   ├── Notification Channels (Email/Webhook active; Slack/Discord stubs) [PARTIALLY IMPLEMENTED]
│   └── Outgoing Webhooks with HMAC-SHA256 Signatures [IMPLEMENTED]
└── Observability & Reconciliation
    ├── PostgreSQL-Backed Distributed Job Queue (SKIP LOCKED) [IMPLEMENTED]
    ├── Continuous Desired-State Container Reconciler [IMPLEMENTED]
    ├── In-Memory Ring Buffer Log Streaming & Metrics Snapshots [IMPLEMENTED]
    └── Append-Only Audit Logging [IMPLEMENTED]
```

---

## 3. Complete Module Inventory

### 1. Identity, Authentication & Tenancy (`internal/auth`, `internal/organizations`)
* **Purpose**: Manages user identities, secure credentials, sessions, organizations, and team memberships.
* **User Capabilities**: Register, login, refresh access tokens, request password reset, switch active organization context, invite team members.
* **Backend Components**:
  * Handlers: `internal/auth/handler.go`, `internal/organizations/handler.go`
  * Services: `internal/auth/service.go`, `internal/organizations/service.go`
  * Repositories: `internal/auth/repository.go`, `internal/organizations/repository.go`
* **Frontend Components**: `OrganizationSwitcher`, `TopBar`, `AuthContext`, login/register route bindings.
* **Database Entities**: `users`, `sessions`, `organizations`, `organization_members`, `teams`, `team_members`, `organization_invitations`, `password_reset_tokens`.
* **APIs**: `POST /auth/register`, `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`, `GET /organizations`, `POST /organizations`, `GET /organizations/{id}/members`.
* **Status**: **IMPLEMENTED**

### 2. Authorization & RBAC (`internal/rbac`)
* **Purpose**: Enforces fine-grained permission checks across all API requests and tenant scopes.
* **User Capabilities**: System roles (`Owner`, `Administrator`, `DevOps`, `Developer`, `Support`, `Viewer`) enforce read/write/delete boundaries.
* **Backend Components**:
  * Authorizer: `internal/rbac/authorizer.go`
  * Permission Definitions: `internal/rbac/permissions.go`
  * Database Seed: `migrations/000004_rbac_seed.up.sql`
* **Database Entities**: `roles`, `permissions`, `role_permissions`, `member_roles`.
* **Status**: **IMPLEMENTED**

### 3. Workload Hierarchy & Projects (`internal/projects`, `internal/applications`)
* **Purpose**: Provides logical structuring of software systems into Projects, Environments, and Applications.
* **User Capabilities**: Create projects, create environments (`production`, `staging`, `development`, `preview`, `custom`), configure applications, view service status.
* **Backend Components**:
  * Handlers: `internal/projects/handler.go`, `internal/applications/handler.go`
  * Services: `internal/projects/service.go`, `internal/applications/service.go`
  * Repositories: `internal/projects/repository.go`, `internal/applications/repository.go`
* **Frontend Components**: `ProjectsPageClient`, `ProjectsTable`, `ProjectFormDialog`, `ApplicationsPageClient`, `ApplicationOverview`, `CreateApplicationWizard`.
* **Database Entities**: `projects`, `environments`, `applications`, `application_configs`.
* **APIs**: `GET/POST /projects`, `GET/POST /projects/{id}/environments`, `GET/POST /applications`, `GET/PATCH /applications/{id}`.
* **Status**: **IMPLEMENTED**

### 4. Deployment Orchestration (`internal/deployments`, `internal/orchestrator`)
* **Purpose**: Coordinates the 21-state deployment workflow, transitions revisions, and commands execution agents.
* **User Capabilities**: Trigger deployments (`manual`, `git_push`, `api`), view live step-by-step progress, cancel deployments, stream execution logs.
* **Backend Components**:
  * Engine: `internal/orchestrator/orchestrator.go`
  * Handlers: `internal/deployments/handler.go`
  * Services: `internal/deployments/service.go`
  * Repositories: `internal/deployments/repository.go`
* **Frontend Components**: `DeploymentsPageClient`, `DeploymentsFilterTable`, `DeploymentActions`, `BuildLogViewer`.
* **Database Entities**: `deployments`, `deployment_events`.
* **APIs**: `GET/POST /deployments`, `GET /deployments/{id}`, `POST /deployments/{id}/cancel`, `GET /deployments/{id}/events`.
* **Background Processing**: `DEPLOYMENT_EXECUTION` job handler in queue worker.
* **Status**: **IMPLEMENTED**

### 5. Revisions & Rollback (`internal/revisions`)
* **Purpose**: Maintains immutable snapshots of application code, config, image digests, and runtime definitions.
* **User Capabilities**: View revision history, inspect configuration diffs between revisions, initiate one-click zero-build rollbacks.
* **Backend Components**:
  * Handlers: `internal/revisions/handler.go`
  * Services: `internal/revisions/service.go`
  * Repositories: `internal/revisions/repository.go`
* **Frontend Components**: `RevisionsPageClient`, `RevisionsFilterTable`, `RevisionCompare`, `RollbackConfirmDialog`.
* **Database Entities**: `revisions`.
* **APIs**: `GET /applications/{id}/revisions`, `GET /revisions/{id}`, `POST /revisions/{id}/rollback`.
* **Status**: **IMPLEMENTED**

### 6. Server Management & Execution Agent (`internal/servers`, `internal/agents`, `apps/agent`)
* **Purpose**: Manages infrastructure servers, tracks agent heartbeats, and dispatches Docker operations.
* **User Capabilities**: Register servers via install script token, toggle maintenance mode, monitor CPU/RAM/disk capacity, inspect containers.
* **Backend Components**:
  * Handlers: `internal/servers/handler.go`, `internal/agents/handler.go`, `internal/agentcmd/handler.go`
  * Services: `internal/servers/service.go`, `internal/agents/service.go`
  * Command Bus: `internal/agentcmd/repository.go`
* **Agent Components**:
  * Core: `apps/agent/cmd/agent/main.go`, `apps/agent/internal/agent/agent.go`
  * Docker Engine Controller: `apps/agent/internal/docker/operations.go`
  * Protocol: `packages/protocol-go`
* **Frontend Components**: `ServersPageClient`, `ServersFilterTable`, `AddServerWizard`, `MaintenanceModeCard`.
* **Database Entities**: `servers`, `server_agents`, `server_heartbeats`, `agent_commands`.
* **APIs**: `GET/POST /servers`, `POST /servers/{id}/maintenance`, `POST /agents/register`, `POST /agents/heartbeat`, `POST /agents/commands/{id}/ack`.
* **Status**: **IMPLEMENTED**

### 7. Ingress, Domains & TLS (`internal/domains`)
* **Purpose**: Configures edge reverse proxy rules, manages domains, verifies DNS, and monitors TLS certificates.
* **User Capabilities**: Attach custom domains, configure internal target ports, verify DNS CNAME/A records, inspect TLS certificates.
* **Backend Components**:
  * Handlers: `internal/domains/handler.go`
  * Services: `internal/domains/service.go`
  * Repositories: `internal/domains/repository.go`
* **Frontend Components**: `DomainsPageClient`, `DomainsFilterTable`, `AddDomainDialog`, `DomainStateBanner`.
* **Database Entities**: `domains`, `certificates`.
* **APIs**: `GET/POST /domains`, `POST /applications/{id}/domains`, `PATCH /domains/{id}`, `DELETE /domains/{id}`.
* **Status**: **IMPLEMENTED**

### 8. Stateful Databases & Backups (`internal/databases`, `internal/backups`)
* **Purpose**: Orchestrates containerized PostgreSQL database instances, persistent volumes, and snapshot backup/restore.
* **User Capabilities**: Provision PostgreSQL 14/15/16 instances, reveal decrypted credentials under audit, trigger ad-hoc backups, restore snapshots with destructive confirmations.
* **Backend Components**:
  * Handlers: `internal/databases/handler.go`, `internal/backups/handler.go`
  * Services: `internal/databases/service.go`, `internal/backups/service.go`
  * Repositories: `internal/databases/repository.go`, `internal/backups/repository.go`
* **Frontend Components**: `DatabasesPageClient`, `DatabasesFilterTable`, `CreateDatabaseDialog`, `DatabaseOverview`, `DatabaseConnectionPanel`, `DatabaseBackupsPanel`, `DatabaseRestorePanel`.
* **Database Entities**: `managed_databases`, `backups`, `restore_operations`, `volumes`.
* **APIs**: `GET/POST /databases`, `GET /databases/{id}`, `POST /databases/{id}/credentials/reveal`, `POST /databases/{id}/backups`, `POST /databases/{id}/restore`.
* **Status**: **IMPLEMENTED**

### 9. Variables & Secrets (`internal/variables`, `internal/secrets`)
* **Purpose**: Manages multi-scope configuration variables and envelope-encrypted secrets.
* **User Capabilities**: Define key-value variables across Org/Project/Env/App scopes, store sealed secrets, inspect variable resolution hierarchy.
* **Backend Components**:
  * Handlers: `internal/variables/handler.go`, `internal/secrets/handler.go`
  * Services: `internal/variables/service.go`, `internal/secrets/service.go`
  * Encryption: `pkg/crypto/aead.go` (AES-256-GCM)
* **Frontend Components**: `EnvironmentVariablesEditor`, `SecretsManager`.
* **Database Entities**: `environment_variables`, `secrets`.
* **APIs**: `GET/POST /variables`, `GET/POST /secrets`, `DELETE /secrets/{id}`.
* **Status**: **IMPLEMENTED**

### 10. Background Processing & Reconciliation (`internal/jobs`, `internal/reconcile`)
* **Purpose**: Provides transactional asynchronous job execution and continuous drift reconciliation.
* **User Capabilities**: Autonomous background recovery; workers pick up queued jobs with lease tracking.
* **Backend Components**:
  * Queue & Worker: `internal/jobs/queue.go`, `internal/jobs/worker.go`
  * Reconciler Loop: `internal/reconcile/loop.go`
* **Database Entities**: `jobs`, `application_replicas`.
* **Status**: **IMPLEMENTED**

---

## 4. System Architecture

DeployCore strictly enforces a two-tier separation between the **Control Plane** and the **Execution Plane**. 

### Why Responsibilities are Separated
1. **Security Isolation**: The Control Plane never runs Docker commands or opens Docker socket connections directly. Host Docker sockets are protected behind authenticated server agents.
2. **Blast Radius Reduction**: An issue on a single server host does not affect the control plane or workloads running on other servers.
3. **Resilience to Network Partitioning**: Server agents continue running existing containers even if connectivity to the control plane is temporarily interrupted.

### System Architecture Diagram
```mermaid
graph TD
    User["DevOps / Developer / Browser"] -->|HTTPS / WSS| Frontend["apps/frontend (Next.js 16 / React 19)"]
    Frontend -->|REST / OpenAPI v1| ControlPlane["apps/api (Go 1.24 Control Plane)"]
    
    subgraph ControlPlaneBoundary["Control Plane Infrastructure"]
        ControlPlane -->|Parameterized SQL| Postgres[("PostgreSQL 16 Engine")]
        ControlPlane -->|In-Memory Ring Buffers| TelemetryMemory["Logs / Metrics Telemetry Cache"]
        ControlPlaneWorker["Background Job Worker (jobs.Worker)"] -->|FOR UPDATE SKIP LOCKED| Postgres
        ControlPlaneReconciler["Reconcile Loop (reconcile.Loop)"] -->|Poll Desired State| Postgres
    end

    ControlPlaneWorker -->|Queue Operations| AgentCommandQueue[("agent_commands Table")]
    
    subgraph ExecutionPlaneBoundary["Execution Plane (Host Server 1..N)"]
        AgentDaemon["apps/agent (Go Agent Daemon)"] -->|HTTPS Long-Poll / Heartbeat| ControlPlane
        AgentDaemon -->|Fetch & Ack Commands| AgentCommandQueue
        AgentDaemon -->|Docker Engine API / Unix Socket| DockerEngine["Host Docker Engine"]
        
        DockerEngine --> WorkloadContainer1["App Replica Container 1"]
        DockerEngine --> WorkloadContainer2["App Replica Container 2"]
        DockerEngine --> ManagedPostgres["Managed Database Container"]
        
        EdgeRouter["Traefik v3 (Edge Reverse Proxy)"] -->|Dynamic Docker Provider Labels| WorkloadContainer1
        EdgeRouter -->|Dynamic Docker Provider Labels| WorkloadContainer2
    end
    
    InternetTraffic["Public Client Traffic"] -->|HTTPS (Port 80/443)| EdgeRouter
```

---

## 5. Repository & Monorepo Architecture

The repository is structured as a unified monorepo coordinated by a Go workspace (`go.work`):

```text
deploy-core/
├── apps/
│   ├── api/                  # Go 1.24.2 Control Plane REST API & Background Workers
│   │   ├── cmd/api/          # Main HTTP application entrypoint
│   │   ├── internal/         # 36 internal domain packages (auth, deploy, orchestrator, etc.)
│   │   ├── pkg/              # Reusable internal packages (apierror, crypto, pagination)
│   │   ├── migrations/       # SQL migrations mirror
│   │   └── scripts/          # OpenAPI generation scripts (gen-openapi.py)
│   ├── agent/                # Go 1.24.2 Host Execution Daemon
│   │   ├── cmd/agent/        # Main agent daemon entrypoint
│   │   ├── internal/         # Agent runtime controllers (docker, health, traefik, stats)
│   │   └── pkg/              # Agent versioning metadata
│   └── frontend/             # Next.js 16.3.3 App Router Presentation Plane
│       ├── app/              # App Router route pages (54 dashboard and admin routes)
│       ├── components/       # Design system primitives and deploycore domain widgets
│       ├── hooks/            # Client hooks (useApiQuery)
│       └── lib/              # API clients, validations (Zod), and view-model utilities
├── packages/
│   └── protocol-go/          # Shared wire contracts, command schemas, telemetry structs
├── docs/                     # Persistent architecture, API, and module documentation
├── deployments/              # Systemd unit files and host installation scripts
├── go.work                   # Go multi-module workspace (api, agent, protocol-go)
├── AGENTS.md                 # Non-negotiable repository engineering rules
└── PROMPTS.md                # Master UI and functional prompt specification
```

### Application Breakdown
* **`apps/api`**: Standard library HTTP router (`net/http.ServeMux` Go 1.22+ patterns). No third-party web frameworks (no Gin, Chi, or Echo). Database interaction uses raw parameterized SQL via `github.com/jackc/pgx/v5/pgxpool`.
* **`apps/agent`**: Lightweight daemon designed to run as a systemd service (`deploycore-agent.service`). Interfaces with `github.com/docker/docker` engine SDK and `github.com/shirou/gopsutil/v3` for hardware telemetry.
* **`packages/protocol-go`**: The single source of truth for command operations (`protocol.Op*`), error codes (`protocol.Err*`), heartbeat payloads, and telemetry data models.
* **`apps/frontend`**: Next.js 16 App Router using React 19, Tailwind CSS v4, and Radix/shadcn primitives. Communicates with `apps/api` through generated wire contracts (`lib/api/contract.ts`).

---

## 6. Application Deployment Lifecycle

The deployment lifecycle is orchestrated by `apps/api/internal/orchestrator/orchestrator.go` via a 21-state transactional state machine:

```
PENDING → QUEUED → PREPARING → FETCHING_SOURCE → BUILDING → IMAGE_READY 
→ CREATING_CONTAINER → STARTING → HEALTH_CHECKING → ACTIVATING → RUNNING
```

### Sequence Diagram of Deployment Execution
```mermaid
sequenceDiagram
    autonumber
    actor User as Developer
    participant FE as apps/frontend
    participant API as apps/api (HTTP)
    participant DB as PostgreSQL (Jobs/State)
    participant Orch as Orchestrator Worker
    participant Agent as apps/agent
    participant Docker as Host Docker Engine
    participant Router as Traefik Edge Router

    User->>FE: Click "Deploy Application"
    FE->>API: POST /api/v1/applications/{id}/deployments
    API->>DB: Insert deployment (Status: PENDING), Enqueue DEPLOYMENT_EXECUTION job
    API-->>FE: 201 Created (Deployment DTO)
    
    Orch->>DB: Claim Job (FOR UPDATE SKIP LOCKED)
    Orch->>DB: Transition PENDING -> QUEUED -> PREPARING
    Orch->>DB: Build effective config, resolve secrets (AES-256-GCM), create Revision Candidate
    
    alt Source is Git Repository
        Orch->>DB: Transition PREPARING -> FETCHING_SOURCE
        Orch->>Agent: Queue OpBuildImage (Phase: fetch_source)
        Agent->>Docker: Git clone / checkout branch
        Orch->>DB: Transition FETCHING_SOURCE -> BUILDING
        Orch->>Agent: Queue OpBuildImage (Phase: build)
        Agent->>Docker: Execute docker build
        Docker-->>Agent: Image built with SHA256 digest
    else Source is Docker Image
        Orch->>DB: Transition PREPARING -> BUILDING
        Orch->>Agent: Queue OpPullImage (Phase: pull)
        Agent->>Docker: Pull image by reference
    end
    
    Orch->>DB: Record immutable imageDigest in revision; Transition -> IMAGE_READY
    Orch->>DB: Transition IMAGE_READY -> CREATING_CONTAINER
    Orch->>Agent: Queue OpCreateContainer (Volumes, Env Vars, Placement Policy)
    Agent->>Docker: docker create (Network joined, Traefik disabled initially)
    
    Orch->>DB: Transition CREATING_CONTAINER -> STARTING
    Orch->>Agent: Queue OpStartContainer
    Agent->>Docker: docker start
    
    Orch->>DB: Transition STARTING -> HEALTH_CHECKING
    loop Health Checking Probes
        Orch->>Agent: Queue OpRunHealthCheck (HTTP/TCP probe)
        Agent->>Docker: Query container port / curl probe
        Agent-->>Orch: Probe Result (Success/Failure)
    end
    
    Note over Orch: Verify Health Policy Thresholds Passed
    Orch->>DB: Transition HEALTH_CHECKING -> ACTIVATING
    Orch->>Agent: Queue OpDeployRevision (Apply traefik.enable=true labels)
    Agent->>Router: Traefik detects labels, updates live routing table
    Orch->>DB: Mark Target Revision ACTIVE, Previous Revision INACTIVE
    
    opt Rolling Cleanup of Previous Revision
        Orch->>Agent: Queue OpStopContainer (reason: retire_previous)
        Agent->>Docker: Drain & stop previous containers
    end
    
    Orch->>DB: Transition ACTIVATING -> RUNNING
    Orch->>DB: Emit Webhook (EventDeploymentCompleted) & Notification
```

---

## 7. Revision Model & State Semantics

DeployCore enforces strict conceptual distinctions between workloads and their runtime manifestations:

| Concept | Definition | Immutability |
| :--- | :--- | :--- |
| **Build** | The transient process of converting source code into an executable container artifact. | Ephemeral execution |
| **Deployment** | An individual execution run that advances through the 21-state deployment machine. | Read-only history once terminal |
| **Revision** | An immutable snapshot containing exact application configuration, environment variable values, secret versions, and pinned image digest. | **Strictly Immutable** |
| **Replica** | A numbered member (`replicaIndex: 0..N`) of an active revision running on a target host. | State tracked in DB |
| **Container** | The physical OS-level container running inside the host Docker daemon. | Managed by Agent |

### Revision Immutability Guarantee
When an application deployment reaches `IMAGE_READY`, the resulting Docker image digest (`sha256:...`) is written to `revisions.image_digest`. Once marked `READY`, a revision's environment variables, port mappings, command overrides, and digest can **never be modified**. Any future change creates a new revision candidate with incremented `revision_number`.

---

## 8. Rollback Architecture & Mechanics

### Verified Code Semantics (`orchestrator.go:182-235`)
DeployCore's rollback implementation adheres to the principle of **Zero-Build Reused Revision**:
1. When a rollback is requested (`trigger: "rollback"`, `targetRevisionId: UUID`), the orchestrator checks `stepPreparingRollback`.
2. It verifies the target revision exists and its status is `READY` or `INACTIVE`.
3. It **bypasses source fetching and building entirely**:
   * `stepFetchingSource`: Reaches `if isRollback(d) { return o.advance(..., StatusBuilding) }`.
   * `stepBuilding`: Reaches `if isRollback(d) { return o.advance(..., StatusImageReady) }`.
4. It creates and starts candidate containers using the previously pinned image digest.
5. **Rollback Safety & Outage Prevention**:
   * Health checks run against the candidate container before traffic activation.
   * If health checks fail, the orchestrator triggers `fail()`. It **does not modify the existing active revision**, preserves edge routing, cleans up the failed candidate, and marks the deployment `FAILED`.
   * The target revision remains `INACTIVE` and is **not** marked `FAILED` because its artifact was not at fault.

```mermaid
sequenceDiagram
    autonumber
    actor User as Operator
    participant Orch as Orchestrator
    participant DB as Database
    participant Agent as Agent / Docker
    participant Router as Traefik Ingress

    User->>Orch: Request Rollback to Revision 14
    Orch->>DB: Verify Revision 14 status in ('READY', 'INACTIVE')
    Note over Orch: Skip Git Fetch & Docker Build entirely
    Orch->>Agent: Start candidate container from pinned imageDigest (Rev 14)
    Orch->>Agent: Run Health Check Probes
    alt Probes Pass
        Orch->>Router: Switch Traefik routing to Rev 14 candidate
        Orch->>DB: Mark Rev 14 ACTIVE, Rev 15 INACTIVE
        Orch->>Agent: Stop Rev 15 containers (drain)
        Orch->>DB: Deployment Status -> RUNNING
    else Probes Fail
        Orch->>Agent: Kill & remove failed candidate
        Note over Orch: Existing Rev 15 traffic is NEVER touched
        Orch->>DB: Deployment Status -> HEALTH_CHECK_FAILED
        Orch->>DB: Rev 14 remains INACTIVE (Not marked FAILED)
    end
```

---

## 9. Server & Agent Architecture

### Server Registration Flow
1. Operator creates a Server entry in the dashboard. The Control Plane generates a short-lived, cryptographically secure registration token (`AGENT_REGISTRATION_TOKEN_TTL: 15m`).
2. The operator runs the one-line install command on the target host:
   ```bash
   curl -sSL https://deploycore.internal/install.sh | sudo bash -s -- --token <REGISTRATION_TOKEN>
   ```
3. The installation script installs `apps/agent` and registers the systemd unit `deploycore-agent.service`.
4. `apps/agent` sends `POST /api/v1/agents/register` presenting the registration token.
5. Control plane verifies the token, records the agent's public host metadata, generates a dedicated agent HMAC secret, and returns server configuration.
6. The agent stores credentials securely in `AGENT_CREDENTIAL_PATH` (`/etc/deploycore/agent.json` with `0600` permissions).

### Heartbeat & Telemetry
Every 30 seconds (`AGENT_HEARTBEAT_INTERVAL`), the agent collects host statistics:
* Total and available CPU cores and load averages
* Total, used, and available RAM bytes
* Disk mount space and container root filesystem usage
* Running, stopped, and paused container counts
* Installed Docker Engine and agent version

The agent posts telemetry to `POST /api/v1/agents/heartbeat`. If a server fails to send a heartbeat within `AGENT_HEARTBEAT_TTL` (90 seconds), the control plane flags the server `OFFLINE` and prevents further placements.

### Capacity & Placement Engine (`internal/placement/service.go`)
Before creating containers, DeployCore evaluates server placement:
1. Filters out servers marked `OFFLINE` or in `maintenance_mode = true`.
2. Computes **unallocated capacity**:
   $$\text{Available CPU} = \text{Total CPU} - \sum \text{Reserved Workload CPU}$$
   $$\text{Available RAM} = \text{Total RAM} - \sum \text{Reserved Workload Memory}$$
3. Checks placement policies: `binpack` (densest packing), `spread` (least loaded), or explicit target server pinning.
4. Prevents memory overcommit to guarantee host stability.

---

## 10. Networking & Ingress Architecture

DeployCore utilizes **Traefik v3** as its dynamic edge reverse proxy:

```
Public Request: https://api.example.com
                      │
                      ▼
               [Traefik Proxy]
                      │
        Reads Dynamic Docker Labels:
        - traefik.enable=true
        - traefik.http.routers.app-123.rule=Host(`api.example.com`)
        - traefik.http.services.app-123.loadbalancer.server.port=8080
                      │
                      ▼
      [Active Application Container Replica]
```

### Routing & Port Isolation
* Containers do **not** bind directly to host public ports (e.g. `0.0.0.0:8080`), eliminating port collision issues.
* All application containers join a dedicated bridge network (`deploycore-net`).
* Traefik discovers routing rules dynamically via Docker provider labels injected by `apps/agent`.
* During rolling deployment activation, candidate containers receive routing labels only after passing health verification. Multiple replicas share the same Traefik service name, automatically configuring round-robin load balancing.

---

## 11. Database & Persistent Storage Architecture

The Control Plane uses PostgreSQL 16 as its single authoritative datastore.

### Simplified Entity-Relationship Diagram
```mermaid
erDiagram
    ORGANIZATIONS ||--o{ PROJECTS : owns
    ORGANIZATIONS ||--o{ SERVERS : manages
    ORGANIZATIONS ||--o{ USERS : members
    PROJECTS ||--o{ ENVIRONMENTS : contains
    ENVIRONMENTS ||--o{ APPLICATIONS : runs
    ENVIRONMENTS ||--o{ MANAGED_DATABASES : provisions
    APPLICATIONS ||--o{ APPLICATION_CONFIGS : versions
    APPLICATIONS ||--o{ DEPLOYMENTS : executes
    APPLICATIONS ||--o{ REVISIONS : tracks
    DEPLOYMENTS ||--o{ DEPLOYMENT_EVENTS : logs
    REVISIONS ||--o{ APPLICATION_REPLICAS : instantiates
    SERVERS ||--o{ SERVER_AGENTS : authenticates
    SERVERS ||--o{ AGENT_COMMANDS : dispatches
    MANAGED_DATABASES ||--o{ BACKUPS : creates
    MANAGED_DATABASES ||--o{ VOLUMES : attaches
```

### Storage Characteristics
* **Embed Migrations**: 25 transactional migrations applied on startup (`apps/api/internal/platform/db/migrations`).
* **Soft Deletions**: Workloads use `deleted_at IS NULL` partial unique indexes, allowing slug reuse after deletion while preserving historical audit associations.
* **Concurrency Protection**: Critical state transitions use PostgreSQL row-level locks (`SELECT ... FOR UPDATE`).
* **Distributed Job Queue**: Background jobs are polled via `SELECT ... FOR UPDATE SKIP LOCKED`, preventing multiple API workers from executing the same job concurrently.

---

## 12. Security Architecture & Trust Boundaries

### 1. Authentication & Session Security
* **Password Hashing**: Argon2id with cryptographically secure salts.
* **JWT Access Tokens**: Short-lived (15 minutes), signed with HMAC-SHA256 (`AUTH_TOKEN_SECRET`).
* **Refresh Token Rotation**: Stored as SHA-256 hashes in `sessions`. Each token refresh issues a new pair and revokes the predecessor.
* **Token Replay Detection**: If a previously used refresh token is submitted, the entire session family is instantly revoked to thwart token theft.

### 2. Envelope Encryption for Secrets at Rest (`pkg/crypto/aead.go`)
* Secrets are encrypted using **AES-256-GCM** authenticated encryption with random 96-bit nonces.
* Stored in the database as:
  ```json
  {
    "ciphertext": "<base64>",
    "nonce": "<base64>",
    "keyId": "platform:v1"
  }
  ```
* Secret values are never exposed in API list endpoints; only metadata (`hasValue: true`, `updatedAt`, `key`) is emitted.

### 3. Agent Trust Boundary
* Agents communicate outbound-only over HTTPS. The Control Plane never initiates direct TCP connections into the host.
* Commands use schema-validated JSON payloads (`packages/protocol-go`).
* **No Raw Shell Injections**: Commands specify discrete binaries, arguments, and container policies. The agent rejects non-schema or shell-interpolated payloads.

---

## 13. Secrets Management Lifecycle

```
1. Input: Plaintext Secret entered via Create Application Wizard
      │
      ▼
2. API Encryption: Encrypted via AES-256-GCM using SECRETS_PLATFORM_KEY
      │
      ▼
3. Storage: Ciphertext + 12-byte Nonce + Key ID saved to 'secrets' table
      │
      ▼
4. Deployment: Orchestrator decrypts secrets into memory during stepPreparing
      │
      ▼
5. Injection: Transmitted over authenticated TLS to agent as container env vars
      │
      ▼
6. Ephemeral Runtime: Injected into Docker container environment (Never logged)
```

Plaintext secrets exist only in memory during container creation and are immediately purged from process buffers.

---

## 14. Observability & Telemetry

DeployCore combines persistent event logging with low-latency in-memory telemetry buffers:

| Telemetry Type | Storage Layer | Retention Policy | Streaming Protocol |
| :--- | :--- | :--- | :--- |
| **Deployment Step Logs** | In-Memory Ring Buffer (`internal/logs`) | 10,000 lines / job | Server-Sent Events (SSE) |
| **Deployment Audit Events** | PostgreSQL (`deployment_events`) | Permanent | REST API |
| **Container Metrics** | In-Memory Ring Buffer (`internal/metrics`) | 60 samples (1 hour) | SSE / REST API |
| **Server Hardware Metrics** | PostgreSQL (`server_metric_snapshots`) | Configurable rollups | REST API |
| **Platform Audit Logs** | PostgreSQL (`audit_logs`) | Append-Only Permanent | REST API |

---

## 15. Background Processing & Job System

All asynchronous tasks are processed through a durable, PostgreSQL-backed queue:

```
[Enqueue Job] ──> INSERT INTO jobs (status='PENDING', available_at=NOW())
                       │
                       ▼
[Worker Loop] ──> SELECT ... FOR UPDATE SKIP LOCKED
                       │
                       ▼
[Lease Claim] ──> status='RUNNING', locked_by=worker_id, locked_until=NOW()+30s
                       │
                       ▼
             [Execute Registered Job Handler]
             ├── Success: status='COMPLETED', finished_at=NOW()
             └── Failure: attempt_count++, retry with exponential backoff
```

### Registered Job Types
1. `DEPLOYMENT_EXECUTION`: Advances the 21-state deployment state machine.
2. `BACKUP`: Executes scheduled and ad-hoc database backups.
3. `RESTORE`: Restores database snapshots under strict destructive gates.
4. `CERTIFICATE_OPERATION`: Issues, verifies, and renews TLS certificates.
5. `NOTIFICATION_DELIVERY`: Dispatches notification events to channels.
6. `WEBHOOK_DELIVERY`: Signs and delivers HTTP webhooks with retries.
7. `REPLICAS_RECONCILE`: Aligns running container replicas with desired replica counts.
8. `DESIRED_STATE_RECONCILE`: Detects container crashes and enforces restart policies.

---

## 16. Desired-State Reconciliation

The reconciler loop (`internal/reconcile/loop.go`) runs continuously every 30 seconds:

```mermaid
graph TD
    Start[Reconcile Loop Tick] --> FetchApps[Load Active Applications]
    FetchApps --> CheckServer[Check Host Server Heartbeat]
    
    CheckServer -->|Server Offline| Skip[Skip reconciliation & alert]
    CheckServer -->|Server Online| QueryReplicas[Query Actual Docker Containers]
    
    QueryReplicas --> Compare{Actual vs Desired Replicas?}
    
    Compare -->|Missing Container| CreateMissing[Dispatch OpCreateContainer]
    Compare -->|Crashed Container| RestartPolicy{Restart Policy?}
    Compare -->|Excess Container| StopExcess[Dispatch OpStopContainer]
    Compare -->|In Sync| Pass[Record Healthy State]
    
    RestartPolicy -->|always / on-failure| BackoffCheck{Exponential Backoff?}
    RestartPolicy -->|no| MarkFailed[Mark Replica FAILED]
    
    BackoffCheck -->|Wait Delay| Postpone[Postpone restart]
    BackoffCheck -->|Delay Passed| Restart[Dispatch OpStartContainer]
```

---

## 17. Frontend Product Map & UI Architecture

DeployCore features 54 fully designed App Router routes in `apps/frontend/app`:

```text
/ (Root Redirect)
├── /dashboard                    # Fleet overview, resource gauges, quick actions
├── /projects                     # Projects list & creation dialog
│   └── /projects/[id]            # Environments list, project settings, danger zone
├── /applications                 # Workloads directory, filtering, status badges
│   ├── /applications/[id]        # Application overview & runtime details
│   └── /applications/[id]/[tab]  # Config, Env Vars, Domains, Logs, Settings
├── /deployments                  # Global deployment history & filter table
│   └── /deployments/[id]         # Step pipeline timeline & real-time log viewer
├── /revisions                    # Version catalog & diff inspector
│   └── /revisions/compare        # Side-by-side configuration comparator
├── /servers                      # Infrastructure fleet & hardware usage
│   └── /servers/[id]/[tab]       # Capacity metrics, maintenance mode, containers
├── /databases                    # Managed databases directory & provision dialog
│   └── /databases/[id]/[tab]     # Connection strings, metrics, backups, restore
├── /domains                      # Custom hostnames, DNS verification, TLS status
├── /security/secrets             # Sealed secrets manager & audit history
└── /admin/*                      # Platform administration (Agents, Audit, Jobs, Users)
```

### Integration Status
* **Core Workload Paths**: Fully wired to real control-plane APIs (`/projects`, `/applications`, `/deployments`, `/servers`, `/domains`, `/databases`).
* **Design Primitives**: Clean cards, zero unnecessary wrappers, responsive horizontal scroll containers (`overflow-x-auto`), accessible Radix UI dialog focus management, and dark mode design tokens.
* **Offline Development Resilience**: In the absence of a running local PostgreSQL instance, the frontend gracefully falls back to seed fixtures rather than throwing unhandled exceptions.

---

## 18. API Architecture & Conventions

### Endpoint Inventory by Domain

#### Authentication & Organization
* `POST /api/v1/auth/register` — Create user account
* `POST /api/v1/auth/login` — Authenticate and receive token pair
* `POST /api/v1/auth/refresh` — Rotate refresh token and acquire access token
* `POST /api/v1/auth/logout` — Revoke active session
* `GET/POST /api/v1/organizations` — List or create tenant organizations
* `GET /api/v1/organizations/{id}/members` — List organization members

#### Workloads & Deployments
* `GET/POST /api/v1/projects` — List or create projects
* `GET/POST /api/v1/projects/{id}/environments` — Manage project environments
* `GET/POST /api/v1/applications` — List or create applications
* `GET/PATCH/DELETE /api/v1/applications/{id}` — Workload configuration management
* `GET/POST /api/v1/deployments` — Trigger and list deployments
* `GET /api/v1/deployments/{id}` — Retrieve deployment execution state
* `POST /api/v1/deployments/{id}/cancel` — Cancel ongoing deployment
* `GET /api/v1/applications/{id}/revisions` — Revision history
* `POST /api/v1/revisions/{id}/rollback` — Trigger zero-build rollback

#### Servers & Agents
* `GET/POST /api/v1/servers` — List or register servers
* `POST /api/v1/servers/{id}/maintenance` — Toggle maintenance mode
* `POST /api/v1/agents/register` — Exchange token for agent credentials
* `POST /api/v1/agents/heartbeat` — Ingest hardware telemetry
* `POST /api/v1/agents/commands/{id}/ack` — Acknowledge command execution result

#### Storage & Networking
* `GET/POST /api/v1/domains` — Manage custom domains
* `GET/POST /api/v1/databases` — Provision managed databases
* `POST /api/v1/databases/{id}/credentials/reveal` — Audited credential reveal
* `GET/POST /api/v1/databases/{id}/backups` — Trigger and list backups
* `POST /api/v1/databases/{id}/restore` — Restore database snapshot
* `GET/POST /api/v1/secrets` — Manage envelope-encrypted secrets

### Standard Error Envelope
All error responses adhere to the standard error contract (`pkg/apierror`):
```json
{
  "error": {
    "code": "RESOURCE_NOT_FOUND",
    "message": "application with specified id does not exist",
    "requestId": "req_01HP8YZ8W2N1B9E4X7C6D5A3",
    "details": {}
  }
}
```

---

## 19. Roles, Permissions & Multi-Tenancy (RBAC)

Every request is authorized by `internal/rbac/authorizer.go` against 6 seeded roles:

| Permission Area | Owner | Administrator | DevOps | Developer | Support | Viewer |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| `organization.*` | Write | Write | Read | Read | Read | Read |
| `server.create/delete` | Yes | Yes | No | No | No | No |
| `server.update` | Yes | Yes | Yes | No | No | No |
| `application.create/delete`| Yes | Yes | Yes | No | No | No |
| `application.deploy` | Yes | Yes | Yes | Yes | No | No |
| `deployment.rollback` | Yes | Yes | Yes | Yes | No | No |
| `secret.read_metadata` | Yes | Yes | Yes | Yes | Yes | Yes |
| `secret.create/update` | Yes | Yes | Yes | No | No | No |
| `database.reveal` | Yes | Yes | No | No | No | No |
| `database.restore` | Yes | Yes | No | No | No | No |

---

## 20. Failure & Recovery Model

| Failure Scenario | System Reaction | Recovery / Cleanup Action |
| :--- | :--- | :--- |
| **Git Source Checkout Fails** | Deployment transitions to `SOURCE_FAILED` | Target revision marked `FAILED`; previous active revision preserved untouched. |
| **Container Build Fails** | Deployment transitions to `BUILD_FAILED` | Build logs persisted in memory; temporary build context directory purged. |
| **Container Start Fails** | Deployment transitions to `START_FAILED` | Agent cleans unstarted container; no traffic switched. |
| **Health Check Probes Fail** | Deployment transitions to `HEALTH_CHECK_FAILED` | Candidate container stopped and removed; edge routing never activates candidate; existing revision unaffected. |
| **Agent Disconnects / Host Drops** | Server flagged `OFFLINE` after 90s heartbeat gap | Reconciler prevents new placements on host; alerts dispatched. |
| **Control Plane Worker Crashes** | Active job lease expires (`locked_until < NOW()`) | Surviving worker re-claims job (`SKIP LOCKED`) and resumes from last recorded state. |
| **Rollback Candidate Fails** | Deployment transitions to `HEALTH_CHECK_FAILED` | Target revision kept `INACTIVE`; previous active revision remains serving traffic without downtime. |

---

## 21. Application Data Model

```mermaid
classDiagram
    class Organization {
        UUID id
        string name
        string slug
    }
    class Project {
        UUID id
        UUID organization_id
        string name
        string slug
    }
    class Environment {
        UUID id
        UUID project_id
        string name
        string kind
    }
    class Application {
        UUID id
        UUID environment_id
        string name
        string slug
        string type
    }
    class Revision {
        UUID id
        UUID application_id
        int revision_number
        string image_digest
        string status
        jsonb config_snapshot
    }
    class Deployment {
        UUID id
        UUID application_id
        UUID target_revision_id
        string status
        string trigger
    }
    class Server {
        UUID id
        string name
        string hostname
        string status
        bool maintenance_mode
    }
    class ManagedDatabase {
        UUID id
        string name
        string engine
        string database_name
        string status
    }

    Organization "1" --> "*" Project
    Organization "1" --> "*" Server
    Project "1" --> "*" Environment
    Environment "1" --> "*" Application
    Environment "1" --> "*" ManagedDatabase
    Application "1" --> "*" Revision
    Application "1" --> "*" Deployment
    Deployment "1" --> "1" Revision : targets
```

---

## 22. Deployment Topology Examples

### 1. Single-Server Topology (Evaluation / Staging)
A single virtual machine hosts both the Control Plane and the Execution Plane:
```
┌─────────────────────────────────────────────────────────────────┐
│ Virtual Machine (e.g. 4 vCPU / 8 GB RAM)                        │
│                                                                 │
│  ┌───────────────────────┐         ┌─────────────────────────┐  │
│  │ apps/api + PostgreSQL │<───────>│ apps/agent (Daemon)     │  │
│  └───────────────────────┘         └───────────┬─────────────┘  │
│                                                │ Unix Socket    │
│  ┌─────────────────────────────────────────────▼─────────────┐  │
│  │ Local Docker Engine                                       │  │
│  │  [Traefik:80/443] ──> [App Container] ──> [Database Cont] │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### 2. Multi-Server Production Topology
High-availability control plane with distributed edge worker nodes:
```
┌────────────────────────────────────────────────────────┐
│ Control Plane Cluster                                  │
│  [Load Balancer] ──> [apps/api Node 1] ──┐             │
│                  ──> [apps/api Node 2] ──┼─> [Postgres]│
└──────────────────────────────────────────┼─────────────┘
                                           │ Outbound HTTPS
         ┌─────────────────────────────────┼────────────────────────┐
         ▼                                 ▼                        ▼
┌─────────────────┐               ┌─────────────────┐      ┌─────────────────┐
│ Worker Node 01  │               │ Worker Node 02  │      │ Database Node   │
│ [apps/agent]    │               │ [apps/agent]    │      │ [apps/agent]    │
│ [Docker Engine] │               │ [Docker Engine] │      │ [Docker Engine] │
│ [Traefik Proxy] │               │ [Traefik Proxy] │      │ [Postgres 16]   │
│ [App Replica 0] │               │ [App Replica 1] │      │ [Protected Vol] │
└─────────────────┘               └─────────────────┘      └─────────────────┘
```

---

## 23. Technology Stack & Dependency Inventory

### Frontend Plane (`apps/frontend`)
* **Framework**: Next.js 16.3.3 (App Router, Turbopack)
* **Runtime / Core**: React 19.0.0, Node.js v20+
* **Styling & Tokens**: Tailwind CSS v4.3.3, `@tailwindcss/postcss`
* **UI Primitives**: Base-UI (`@base-ui/react` 1.5.0), Lucide React (1.16.0), Sonner (2.0.8)
* **Form & Validation**: React Hook Form (7.62.0), Zod (4.1.5)
* **Charts & Metrics**: Recharts 3.8.0
* **Package Manager**: `pnpm` 9.15.9

### Control Plane (`apps/api`)
* **Language & Compiler**: Go 1.24.2
* **Database Driver**: `github.com/jackc/pgx/v5` (v5.7.4) with connection pooling (`puddle/v2`)
* **Security & Crypto**: `golang.org/x/crypto` (Argon2id, AES-GCM), `github.com/golang-jwt/jwt/v5` (v5.2.1)
* **HTTP Routing**: Go standard library `net/http.ServeMux` (Go 1.22+ patterns)
* **Logging**: Standard `log/slog` structured JSON logging

### Execution Plane (`apps/agent`)
* **Language & Compiler**: Go 1.24.2
* **Container Engine SDK**: `github.com/docker/docker` (v24.0.9+incompatible)
* **Hardware Telemetry**: `github.com/shirou/gopsutil/v3` (v3.24.5)
* **Ingress Controller**: Traefik v3 (Docker Provider dynamic labels)

---

## 24. Configuration & Environment Variables

### Control Plane Configuration (`apps/api`)
* `APP_ENV`: Application environment (`development`, `production`, `test`). Default: `development`.
* `HTTP_ADDR`: Binding host and TCP port. Default: `:8080`.
* `DATABASE_URL`: PostgreSQL connection string (e.g. `postgres://user:pass@host:5432/deploycore?sslmode=disable`).
* `AUTH_TOKEN_SECRET`: Cryptographic secret for signing access JWTs. Minimum 32 bytes.
* `AUTH_ACCESS_TOKEN_TTL`: Lifetime of access tokens. Default: `15m`.
* `AUTH_REFRESH_TOKEN_TTL`: Lifetime of refresh tokens. Default: `720h` (30 days).
* `SECRETS_PLATFORM_KEY`: 32-byte Base64-encoded key for AES-256-GCM envelope encryption.
* `SECRETS_KEY_ID`: Identifier for active platform encryption key. Default: `platform:v1`.
* `ORCHESTRATOR_SIMULATE_AGENT`: When `true`, advances deployment steps locally without a live agent. Default: `true` in development.
* `JOB_WORKER_ENABLED`: Enables background job queue consumer. Default: `true`.
* `RECONCILE_ENABLED`: Enables continuous container state reconciler. Default: `true`.

### Agent Configuration (`apps/agent`)
* `AGENT_CONTROL_PLANE_URL`: HTTPS URL of the Control Plane API (e.g. `https://deploycore.internal`).
* `AGENT_REGISTRATION_TOKEN`: One-time token used during initial host enrollment.
* `AGENT_CREDENTIAL_PATH`: Path to persisted agent identity JSON. Default: `/etc/deploycore/agent.json`.
* `AGENT_DOCKER_HOST`: Docker engine socket URI. Default: `unix:///var/run/docker.sock`.
* `AGENT_HEARTBEAT_INTERVAL`: Interval between hardware telemetry reports. Default: `30s`.

---

## 25. Implementation Status Matrix

| Capability / Module | Backend (`apps/api`) | Frontend (`apps/frontend`) | Agent (`apps/agent`) | Production Ready | Operational Notes |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Authentication & Sessions** | IMPLEMENTED | IMPLEMENTED | N/A | YES | Argon2id + JWT + Token reuse revocation |
| **Multi-Tenancy & RBAC** | IMPLEMENTED | IMPLEMENTED | N/A | YES | 6 seeded roles, org-scoped queries |
| **Projects & Environments** | IMPLEMENTED | IMPLEMENTED | N/A | YES | Multi-environment hierarchy |
| **Application Workloads** | IMPLEMENTED | IMPLEMENTED | IMPLEMENTED | YES | 7 application types supported |
| **Deployment State Machine**| IMPLEMENTED | IMPLEMENTED | IMPLEMENTED | YES | 21-state transactional workflow |
| **Zero-Build Rollbacks** | IMPLEMENTED | IMPLEMENTED | IMPLEMENTED | YES | Pinned digest reuse; non-destructive fail |
| **Server Fleet Management** | IMPLEMENTED | IMPLEMENTED | IMPLEMENTED | YES | Heartbeats, maintenance mode, telemetry |
| **Capacity & Placement** | IMPLEMENTED | IMPLEMENTED | N/A | YES | Overcommit protection, CPU/RAM reservations |
| **Ingress & Traefik Routing**| IMPLEMENTED | IMPLEMENTED | IMPLEMENTED | YES | Dynamic label injection, zero-downtime |
| **Domains & TLS** | IMPLEMENTED | IMPLEMENTED | N/A | YES | DNS verification & TLS lifecycle tracking |
| **Managed Databases** | IMPLEMENTED | IMPLEMENTED | IMPLEMENTED | YES | PostgreSQL lifecycle, volume protection |
| **Database Backups/Restore** | IMPLEMENTED | IMPLEMENTED | IMPLEMENTED | YES | Checksums & destructive confirmations |
| **Envelope Secrets** | IMPLEMENTED | IMPLEMENTED | IMPLEMENTED | YES | AES-256-GCM, metadata-only API exposure |
| **Environment Variables** | IMPLEMENTED | IMPLEMENTED | IMPLEMENTED | YES | 4-tier inheritance resolution |
| **Distributed Job Queue** | IMPLEMENTED | N/A | N/A | YES | PostgreSQL `FOR UPDATE SKIP LOCKED` |
| **State Reconciliation** | IMPLEMENTED | N/A | IMPLEMENTED | YES | Drift detection, crash backoff restarts |
| **Audit Logging** | IMPLEMENTED | IMPLEMENTED | N/A | YES | Append-only tamper-resistant trail |
| **Git Integrations** | PARTIAL | UI ONLY | N/A | NO | GitHub active; GitLab/Bitbucket stubs |
| **Container Registries** | PARTIAL | UI ONLY | N/A | NO | DockerHub/GHCR active; Cloud registries stubs |
| **Notification Channels** | PARTIAL | UI ONLY | N/A | NO | Email/Webhooks active; Slack/Discord stubs |

---

## 26. Known Limitations & Technical Debt

### Confirmed Technical Limitations
1. **Telemetry & Log Buffering in Memory**: Real-time deployment logs and container metric samples are stored in circular in-memory buffers in `apps/api`. A control plane restart clears streaming history (historical deployment audit events in PostgreSQL remain preserved).
2. **Local Backup Destinations**: Database snapshot backups currently target local filesystem paths (`local://`). Cloud object storage adapters (S3, GCS, Azure Blob) are not yet integrated.
3. **Registry & Git Stubs**: While GitHub and standard DockerHub/GHCR connections work, cloud-specific IAM integrations (AWS ECR IAM roles, GCP Workload Identity) are currently stubbed.
4. **WebSocket Terminal Execution**: Interactive container terminal sessions (`docker exec -it`) are awaiting interactive WebSocket streaming support in future execution phases.

### Inferred Architectural Risks
* **Control Plane Single Point of Failure (Without External LB)**: In single-server setups, if the control plane VM is down, existing workloads continue running on worker nodes, but no new deployments or reconciliations can execute.
* **Large Monorepo Schema Generation**: The Python script `apps/api/scripts/gen-openapi.py` relies on `openapi.json`. Schema synchronization must be triggered manually when Go domain models evolve.

---

## 27. Current Product Footprint & Engineering Assessment

### What DeployCore Can Actually Do Today
* **Fully Functional Core**: DeployCore successfully manages projects, environments, server fleets, and applications.
* **Real Deployments**: It builds and runs containers, monitors health checks, configures Traefik edge routing, and executes zero-downtime rollouts.
* **Instant Rollbacks**: Verified code paths safely restore previous revisions without rebuilding source code or causing outages on probe failure.
* **Secure Secrets & Databases**: Envelope encryption protects secrets, and managed PostgreSQL databases support automated backups and audited credential reveals.
* **Comprehensive UI**: All 54 Next.js dashboard routes build with zero TypeScript and zero ESLint errors.

### What Remains Simulated / Stubs
* In local development environments without an active Docker daemon or agent, `ORCHESTRATOR_SIMULATE_AGENT=true` advances deployment states via software timers.
* Auxiliary notification channels (Slack, Discord) and cloud registries (AWS ECR) are currently interface stubs.

### Requirements Before External Enterprise Production Use
1. **Persistent Log Aggregator**: Connect container log streaming to a persistent log sink (e.g., Vector, Loki, or ClickHouse).
2. **S3 Storage Driver for Backups**: Implement an AWS S3 / Cloudflare R2 driver for database snapshots.
3. **CI/CD Automation**: Add GitHub Actions workflows for continuous integration across `apps/api`, `apps/agent`, and `apps/frontend`.

---

## 28. Glossary of Terms

* **Control Plane (`apps/api`)**: The centralized management service responsible for authentication, tenancy, metadata storage, deployment scheduling, and orchestration.
* **Execution Plane (`apps/agent`)**: The autonomous daemon running on host Linux servers that directly interacts with the Docker daemon.
* **Server**: A physical or virtual Linux machine running Docker Engine enrolled into a DeployCore organization.
* **Application**: A defined software workload (e.g. web service, background worker) within an environment.
* **Deployment**: An individual execution run that advances through the 21-state deployment machine.
* **Revision**: An immutable record capturing configuration, environment variables, secrets version, and pinned container image digest.
* **Replica**: An individual running container instance representing a specific revision on a designated server.
* **Candidate**: A newly instantiated replica undergoing startup and health check verification prior to receiving edge traffic.
* **Active Revision**: The specific revision currently receiving production ingress traffic for an application.
* **Desired State**: The declared configuration target stored in PostgreSQL (e.g. 3 healthy replicas of Revision 4).
* **Reconciliation**: The continuous background process comparing actual running containers against desired state and resolving drift.
* **Placement**: The scheduling algorithm that calculates CPU/memory availability to assign workloads to servers.
* **Agent Command**: A discrete, schema-validated operational instruction dispatched from the control plane to an agent.
* **Job**: A durable task queued in PostgreSQL processed by the background worker using `FOR UPDATE SKIP LOCKED`.

---

## 29. Repository Analysis Metadata

* **Date Generated**: September 21, 2026
* **Git Branch**: `main`
* **Git Commit SHA**: `c3b44b936adad5620aced5251827801a66103296`
* **Applications & Packages Inspected**:
  * `apps/api` (Go 1.24.2 Control Plane)
  * `apps/agent` (Go 1.24.2 Host Execution Daemon)
  * `apps/frontend` (Next.js 16.3.3 / React 19 / Tailwind CSS v4)
  * `packages/protocol-go` (Shared Wire Contracts)
* **Major Directories Inspected**:
  * `apps/api/internal/orchestrator/`
  * `apps/api/internal/deployments/`
  * `apps/api/internal/revisions/`
  * `apps/api/internal/platform/db/migrations/`
  * `apps/api/internal/jobs/`
  * `apps/api/internal/reconcile/`
  * `apps/agent/internal/docker/`
  * `apps/agent/internal/controlplane/`
  * `apps/frontend/app/`
  * `apps/frontend/lib/api/`
* **Analysis Integrity**: Inspected active source files directly; all capabilities, schemas, states, and algorithms documented above represent the actual implementation verified in code.
