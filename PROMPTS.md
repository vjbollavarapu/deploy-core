# 2) Cursor — frontend/UI implementation

For Cursor, I recommend a very different instruction style: tell it to **inspect first, preserve functionality, establish reusable primitives, then systematically migrate every route**.

Do not send 30 separate “make this page better” prompts. Give it one master contract and then execute controlled phases.

# CURSOR MASTER INSTRUCTION — APPS/FRONTEND

You are working on an enterprise infrastructure management platform called DeployCore.

Scope for this work:
`apps/frontend`

Technology:

* Next.js App Router
* TypeScript
* Tailwind CSS
* shadcn/ui where appropriate
* Lucide icons
* React Hook Form
* Zod
* TanStack Table where appropriate
* TanStack Query if already present or when API data integration begins
* Recharts for operational charts

IMPORTANT RULES

1. First inspect the complete `apps/frontend` codebase.
2. Do not blindly rewrite working code.
3. Identify:

   * routes
   * layouts
   * existing components
   * duplicated UI
   * theme system
   * data fetching patterns
   * forms
   * tables
   * dialogs
   * API clients
   * state management
   * authentication hooks
4. Produce a short implementation assessment before making major structural changes.
5. Preserve existing functionality unless the functionality is explicitly being changed.
6. Do not create giant components.
7. Do not hardcode mock data into production components.
8. Separate:

   * UI primitives
   * domain components
   * page composition
   * API services
   * types
   * validation
9. Avoid `any`.
10. Do not duplicate backend business rules in the frontend.
11. All destructive actions require confirmation.
12. All API states require:

    * loading
    * success
    * empty
    * error
13. All forms require:

    * client validation
    * server error handling
    * disabled submitting state
    * usable field errors
14. Use accessible semantic HTML.
15. Preserve keyboard navigation.
16. Do not use excessive animations.
17. Avoid visual gimmicks, gradients, glassmorphism and oversized cards.
18. Prefer information density suitable for an infrastructure operations console.

DESIGN SYSTEM

Establish or consolidate:

`components/ui`
Generic primitives only.

`components/platform`
Reusable DeployCore-specific components.

Examples:

* AppShell
* Sidebar
* TopBar
* PageHeader
* ResourceHeader
* Breadcrumbs
* StatusBadge
* HealthIndicator
* EnvironmentBadge
* ProviderBadge
* Metric
* MetricCard
* ResourceUsageBar
* DataTable
* DataTableToolbar
* DataTablePagination
* EmptyState
* ErrorState
* LoadingState
* ConfirmDialog
* DestructiveConfirmDialog
* DetailList
* ActivityTimeline
* DeploymentPipeline
* LogViewer
* CodeBlock
* CopyButton
* SecretField
* FilterBar
* SearchInput
* SidePanel
* CommandPalette

STATUS TOKENS

Create one central status mapping.

Statuses:
HEALTHY
RUNNING
DEPLOYING
PENDING
QUEUED
STOPPED
DEGRADED
FAILED
OFFLINE
MAINTENANCE
UNKNOWN

Never manually assign colours separately on individual pages.

APPLICATION NAVIGATION

Main sidebar:

Overview

Projects

Applications
Deployments

Infrastructure

* Servers
* Databases
* Volumes
* Networks

Observability

* Logs
* Metrics
* Events

Integrations

* Git Providers
* Registries
* Notifications
* Webhooks

Security

* Secrets
* Access Control
* Audit Logs

Settings

Super Admin must be a separately authorised area.

ROUTES

Prepare route architecture equivalent to:

/dashboard

/projects
/projects/[projectId]
/projects/[projectId]/environments/[environmentId]

/applications
/applications/[applicationId]
/applications/[applicationId]/deployments
/applications/[applicationId]/revisions
/applications/[applicationId]/logs
/applications/[applicationId]/metrics
/applications/[applicationId]/domains
/applications/[applicationId]/environment
/applications/[applicationId]/secrets
/applications/[applicationId]/networking
/applications/[applicationId]/volumes
/applications/[applicationId]/settings

/deployments
/deployments/[deploymentId]

/servers
/servers/[serverId]

/databases
/databases/[databaseId]

/volumes
/networks

/logs
/metrics
/events

/integrations/git
/integrations/registries
/integrations/notifications
/integrations/webhooks

/security/secrets
/security/access
/security/audit

/settings

/admin/organizations
/admin/users
/admin/servers
/admin/agents
/admin/health
/admin/jobs
/admin/features
/admin/versions
/admin/audit

Do not create empty copy-paste pages simply to satisfy routes. Build shared layouts and reusable patterns.

PHASE F1 — FOUNDATION

Implement or refactor:

1. App shell
2. Navigation
3. Responsive sidebar
4. Header
5. Breadcrumbs
6. Organization switcher
7. Global command palette
8. Theme
9. Typography
10. spacing
11. common status system
12. page containers
13. standard table appearance
14. standard form appearance
15. dialog standards
16. loading skeletons
17. empty states
18. error states

Run:

* typecheck
* lint
* build

Fix all problems introduced by the refactor.

Do not proceed by suppressing TypeScript or ESLint errors.

---

PHASE F2 — DASHBOARD

Build the production-quality Dashboard.

Sections:

Infrastructure Status

* Servers Online
* Applications Running
* Active Deployments
* Databases Healthy
* Failures
* Agent Connections

Resource Utilisation

* CPU
* RAM
* Storage

Operational sections:

* Recent Deployments
* Applications Requiring Attention
* Server Capacity
* Backup Status
* Certificate Warnings
* Recent Incidents
* Activity Feed

Use reusable metric components.

Charts must be compact and useful.

Avoid decorative dashboard clutter.

---

PHASE F3 — PROJECTS AND ENVIRONMENTS

Implement:

Projects list
Project detail
Environment detail

Project list columns:

* project
* applications
* environments
* health
* owner
* latest deployment
* updated

Project detail:

* overview
* environments
* applications
* resources
* recent deployment activity

Environment:

* application inventory
* databases
* variables
* secrets metadata
* domains
* health

Create:

* new project form
* new environment form
* edit flows
* delete safeguards

---

PHASE F4 — APPLICATIONS

Build applications listing.

Columns:

* name
* project
* environment
* type
* server
* revision
* status
* domain
* last deployment

Create application detail shell with tabs:

Overview
Deployments
Revisions
Logs
Metrics
Domains
Environment
Secrets
Networking
Volumes
Settings

Overview must include:

* runtime status
* URL
* revision
* source
* branch
* commit
* deployment age
* uptime
* CPU
* RAM
* health
* server
* recent deployment
* recent logs
* domain/TLS
* activity

Use `ResourceHeader`.

---

PHASE F5 — CREATE APPLICATION WIZARD

Build a proper multi-step form.

Steps:

1 Source

* Git Repository
* Docker Image
* Docker Compose

2 Source Configuration

* repository
* branch
* Dockerfile
* build context

3 Runtime

* application type
* internal port
* command
* entrypoint
* CPU
* RAM
* restart policy

4 Configuration

* environment variables
* secrets

5 Networking

* domain
* health checks

6 Placement

* project
* environment
* target server

7 Review

8 Deploy

Persist wizard state safely while navigating between steps.

Use Zod schemas.

Do not submit until the final step.

---

PHASE F6 — DEPLOYMENTS

Implement deployment listing and detail.

Deployment filters:

* all
* queued
* running
* successful
* failed
* cancelled

Detail page must visualise state machine:

PENDING
QUEUED
PREPARING
FETCHING_SOURCE
BUILDING
IMAGE_READY
CREATING_CONTAINER
STARTING
HEALTH_CHECKING
ACTIVATING
RUNNING

Failure variants:
SOURCE_FAILED
BUILD_FAILED
IMAGE_FAILED
CONTAINER_FAILED
START_FAILED
HEALTH_CHECK_FAILED
ROUTING_FAILED
CANCELLED
TIMEOUT

Build:

* DeploymentPipeline
* DeploymentEventTimeline
* BuildLogViewer

Log viewer:

* virtualised if required
* streaming-ready
* search
* follow
* pause
* timestamps
* copy
* download
* fullscreen

---

PHASE F7 — REVISIONS + ROLLBACK

Implement revisions table and revision detail.

Fields:

* revision
* status
* traffic
* commit
* image digest
* creator
* created
* runtime

Actions:

* view
* redeploy
* rollback
* compare
* archive

Create rollback confirmation:

"Rollback production application daya-api from revision 49 to revision 47?"

Clearly explain:

* no rebuild
* configuration snapshot restored
* traffic switches only after health verification

Create revision comparison UI:

* commit
* image
* CPU
* RAM
* command
* environment variable metadata
* secret references
* volumes
* health check
* domains

Never reveal secret contents.

---

PHASE F8 — SERVERS

Servers table:

* server
* provider
* region
* IP
* CPU
* memory
* disk
* containers
* agent
* last heartbeat
* status

Server detail tabs:

Overview
Applications
Containers
Images
Volumes
Networks
Metrics
Logs
Agent
Settings

Build Add Server wizard:

1 Information
2 Provider
3 Registration
4 Install Agent
5 Verification
6 Complete

Registration command must use a copyable secure temporary token placeholder.

Create maintenance mode UI.

---

PHASE F9 — CONTAINERS / IMAGES / NETWORKS / VOLUMES

Create operational views.

Containers:

* application
* revision
* server
* image
* CPU
* memory
* restart count
* state

Actions:

* logs
* inspect
* restart
* stop
* terminal
* remove

Images:

* repository
* tag
* digest
* size
* application
* created

Networks:

* name
* driver
* scope
* environment
* connected services

Volumes:

* name
* server
* driver
* resource
* mount
* backup policy

All dangerous operations require explicit confirmation.

---

PHASE F10 — DOMAINS + TLS

Domains table:

* domain
* app
* environment
* port
* DNS
* HTTPS
* certificate
* expiry
* status

Domain detail:

* expected DNS
* observed DNS
* validation
* certificate issuance
* renewal
* primary domain
* force HTTPS
* redirect rules

Build state-aware UI for:
PENDING
VERIFYING
ISSUING
ACTIVE
EXPIRING
FAILED

---

PHASE F11 — VARIABLES + SECRETS

Build environment variable editor supporting hierarchy:

Organization
Project
Environment
Application

Show:

* inherited
* overridden
* application-specific

Support:

* add
* edit
* remove
* bulk paste
* .env import

Secrets:

* metadata only in tables
* masked inputs
* never repopulate existing secret values from server
* rotation workflow
* scoped access

---

PHASE F12 — DATABASES + BACKUPS

Database list and detail.

Initial database:
PostgreSQL

Database detail:
Overview
Connection
Metrics
Backups
Restore
Logs
Settings

Connection credentials should be revealable only when API policy allows and permission is granted.

Backups:

* status
* started
* completed
* duration
* size
* destination
* checksum

Restore flow must include strong destructive action warnings.

---

PHASE F13 — OBSERVABILITY

Implement:

* global logs
* metrics
* events
* application logs
* server logs
* database logs

Log viewer must support:

* application
* environment
* revision
* container
* search
* severity
* timestamps
* pause
* follow
* download

Metrics:

* CPU
* RAM
* storage
* network
* restart count
* uptime

Do not invent backend metrics not provided by API.

---

PHASE F14 — INTEGRATIONS

Create:
Git providers
Registries
Notification channels
Webhooks

Git:

* GitHub
* GitLab
* Bitbucket
* generic Git

Registries:

* GHCR
* Docker Hub
* GCP Artifact Registry
* ECR
* ACR
* generic OCI

Webhooks:

* endpoint
* secret
* events
* enabled
* deliveries
* retries

---

PHASE F15 — RBAC + AUDIT

Build:
Users
Teams
Roles
Permissions
Invitations

Default roles:
Owner
Administrator
DevOps
Developer
Support
Viewer

Permission matrix by capability.

Audit log:

* timestamp
* actor
* action
* resource
* project
* IP
* result

Audit detail drawer with before/after metadata.

Never display secrets.

---

PHASE F16 — FINAL UI/UX AUDIT

Inspect every route in `apps/frontend`.

Review all:

* pages
* tables
* forms
* cards
* drawers
* dialogs
* tabs
* filters
* toolbars
* navigation
* mobile states
* dark mode
* loading
* errors
* empty states

Find and eliminate:

* duplicated UI
* one-off styling
* inconsistent padding
* inconsistent headings
* unnecessary cards
* giant whitespace
* weak data hierarchy
* excessive badges
* duplicate status implementations
* hardcoded status colours
* giant files
* unsafe `any`
* inaccessible buttons
* forms without labels
* destructive actions without confirmation

Run:

* lint
* typecheck
* tests if present
* production build

Do not finish until the frontend passes the checks or clearly document any pre-existing failures unrelated to the work.

### How I would actually use that

Don't paste all F1–F16 into Cursor and say “do everything.”

Paste the **Master Instruction once**, then run:

> “Execute Phase F1 only.”

Then:

> “Review Phase F1 against the master instruction, fix all deviations, run build/typecheck, and stop.”

Then F2, F3, etc.

That will give you dramatically better results than a single giant Cursor run.

---

# 3) Cursor — Go backend `apps/api`

For the Go side, the important thing is to prevent Cursor from creating an uncontrolled collection of handlers and Docker shell commands.

I would make these prompts your **backend engineering contract**.

# CURSOR MASTER INSTRUCTION — GO CONTROL PLANE

You are building the backend control plane for DeployCore.

Scope:
`apps/api`

DeployCore is an enterprise self-hosted deployment platform.

Architecture:

Next.js frontend
↓
Go Control Plane API
↓
PostgreSQL + Job Queue
↓
Go Server Agent
↓
Docker Engine
↓
Traefik / workloads

The API is the CONTROL PLANE.

It owns:

* authentication
* authorization
* organizations
* projects
* environments
* server registry
* application configuration
* deployment orchestration
* deployment state machine
* revision management
* domains
* secrets metadata
* audit
* scheduling decisions
* jobs
* notifications

The API must NOT directly become the Docker runtime implementation.

Docker execution belongs to the Go Agent.

TECHNICAL PRINCIPLES

Use:

* Go
* PostgreSQL
* explicit migrations
* structured logging
* context.Context everywhere appropriate
* dependency injection by constructors
* domain/service/repository separation without overengineering
* REST initially
* OpenAPI generation/documentation
* UUID or similarly robust IDs
* UTC timestamps
* database transactions for state transitions
* optimistic/pessimistic locking where necessary
* idempotency for critical operations

Do NOT:

* create unnecessary microservices
* use global mutable dependencies
* use package-level database clients
* panic for expected runtime errors
* bury business logic in HTTP handlers
* execute arbitrary shell commands from request parameters
* let frontend determine authorization
* expose database entities directly as public API contracts
* store plaintext passwords/tokens/secrets
* return raw internal errors to clients
* silently ignore errors
* use Docker CLI execution as the architecture

ARCHITECTURE STYLE

Build a modular monolith.

Suggested:

apps/api/
cmd/
api/
internal/
auth/
organizations/
users/
teams/
rbac/
projects/
environments/
servers/
agents/
applications/
deployments/
revisions/
domains/
certificates/
variables/
secrets/
registries/
gitproviders/
databases/
volumes/
backups/
notifications/
webhooks/
audit/
events/
jobs/
health/
pkg/
apierror/
crypto/
logger/
pagination/
validation/
requestid/
migrations/

Within a module, prefer:

* domain types
* repository interface
* service
* transport/handler
* request/response DTOs

Do not introduce interfaces where there is no abstraction value.

STANDARD API ERROR MODEL

Create stable codes such as:

VALIDATION_ERROR
UNAUTHORIZED
FORBIDDEN
RESOURCE_NOT_FOUND
CONFLICT

ORGANIZATION_NOT_FOUND
PROJECT_NOT_FOUND
ENVIRONMENT_NOT_FOUND
SERVER_NOT_FOUND
SERVER_OFFLINE
AGENT_UNAVAILABLE
APPLICATION_NOT_FOUND
DEPLOYMENT_NOT_FOUND
REVISION_NOT_FOUND
REVISION_NOT_READY
DOMAIN_ALREADY_ASSIGNED
SECRET_NOT_FOUND
INSUFFICIENT_RESOURCES
BUILD_TIMEOUT
HEALTH_CHECK_FAILED

Response model:

{
"error": {
"code": "...",
"message": "...",
"requestId": "...",
"details": {}
}
}

Never expose:

* stack traces
* SQL errors
* secret contents
* internal credentials

REQUEST ID

Every incoming request gets a request ID.

Propagate it into:

* logs
* services
* database-related logs
* job records
* agent commands
* audit entries

TENANCY

Tenant-owned records must be scoped to organization_id.

Never trust organization IDs without checking membership/permission.

Design repositories so tenant boundary mistakes are difficult to make.

AUDIT

All important mutations must support auditing:

* actor
* action
* resource
* organization
* request ID
* IP
* user agent
* before metadata where safe
* after metadata where safe

Never log secret values.

---

PHASE B1 — BOOTSTRAP

Inspect the existing `apps/api`.

If absent, create a clean Go service.

Implement:

* configuration loading
* structured logger
* HTTP server
* graceful shutdown
* health endpoint
* readiness endpoint
* PostgreSQL connection
* migration mechanism
* request IDs
* recover middleware
* CORS configuration
* secure headers where appropriate
* API versioning `/api/v1`
* standard error response
* validation framework
* pagination primitives

Endpoints:
GET /health
GET /ready

Create meaningful tests.

Do not add business features yet.

Run:
go test ./...
go vet ./...

---

PHASE B2 — DATABASE FOUNDATION

Create initial migrations.

Core tables:

users
sessions

organizations
organization_members

teams
team_members

roles
permissions
role_permissions
member_roles

projects
environments

servers
server_agents
server_heartbeats

applications
application_configs

deployments
deployment_events
revisions

domains
certificates

environment_variables
secrets

git_connections
registries

audit_logs

jobs

Use:

* PKs
* FKs
* appropriate unique constraints
* organization scoping
* created_at
* updated_at
* deleted_at where justified

Do not indiscriminately use soft delete for every table.

Create schema documentation.

---

PHASE B3 — AUTHENTICATION

Implement:

POST /api/v1/auth/register
POST /api/v1/auth/login
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
POST /api/v1/auth/forgot-password
POST /api/v1/auth/reset-password
GET  /api/v1/auth/me

Security:

* Argon2id
* refresh token rotation
* hashed refresh tokens in database
* token/session revocation
* rate limiting for authentication
* account status checks

Prepare architecture for MFA without implementing full MFA unless requested.

Do not put permissions permanently inside long-lived tokens in a way that prevents prompt revocation.

---

PHASE B4 — ORGANIZATIONS + MEMBERSHIP + RBAC

Implement organizations.

Endpoints conceptually:

POST /organizations
GET /organizations
GET /organizations/{id}
PATCH /organizations/{id}

Membership:
GET /organizations/{id}/members
POST /organizations/{id}/invitations
PATCH member roles
DELETE member

RBAC permissions:

server.read
server.create
server.update
server.delete

project.read
project.create
project.update
project.delete

application.read
application.create
application.update
application.deploy
application.restart
application.stop
application.delete

deployment.read
deployment.create
deployment.cancel
deployment.rollback

secret.read_metadata
secret.create
secret.update
secret.delete

database.read
database.create
database.update
database.backup
database.restore

audit.read

Create default roles:
Owner
Administrator
DevOps
Developer
Support
Viewer

Authorization must be enforced server-side in reusable middleware/service checks.

Add comprehensive authorization tests.

---

PHASE B5 — PROJECTS + ENVIRONMENTS

Create:

projects
environments

Endpoints:

GET/POST /projects
GET/PATCH/DELETE /projects/{id}

GET/POST /projects/{id}/environments
GET/PATCH/DELETE /environments/{id}

Rules:

* unique project slug within organization
* environment uniqueness within project
* deletion protection when dependent resources exist unless explicit safe workflow exists
* audit mutations

---

PHASE B6 — SERVER REGISTRY

Implement server management.

Server fields:

* id
* organization_id
* name
* provider
* region
* hostname
* public_ip
* private_ip
* architecture
* operating_system
* cpu_cores
* memory_bytes
* disk_bytes
* docker_version
* status
* maintenance_mode
* last_heartbeat_at
* labels

Endpoints:

POST /servers
GET /servers
GET /servers/{id}
PATCH /servers/{id}
DELETE /servers/{id}

POST /servers/{id}/maintenance
DELETE /servers/{id}/maintenance

Server states:
ONLINE
DEGRADED
OFFLINE
MAINTENANCE
DISABLED

Do not assume server status solely from frontend.

---

PHASE B7 — AGENT REGISTRATION PROTOCOL

Implement server-agent bootstrap.

Flow:

1. User creates/registers server.
2. API generates one-time registration token.
3. Token is short-lived.
4. Agent sends registration request.
5. API validates token.
6. Token becomes permanently unusable.
7. Agent receives a durable identity/credential.
8. API stores only secure credential representation.
9. Future communication is authenticated.

Endpoints conceptually:

POST /servers/{id}/registration-token
POST /agents/register

Implement:

* expiration
* replay protection
* revocation
* agent identity
* agent version
* heartbeat

Heartbeat payload:

* timestamp
* agent version
* Docker status
* CPU
* RAM
* disk
* load
* container count
* uptime

Do not store every heartbeat forever in the main table. Maintain current server state and create a separate strategy for telemetry retention.

---

PHASE B8 — AGENT COMMAND MODEL

Create a transport-agnostic command model.

The API should be able to issue conceptual commands:

DEPLOY_REVISION
STOP_CONTAINER
START_CONTAINER
RESTART_CONTAINER
REMOVE_CONTAINER
FETCH_LOGS
STREAM_LOGS
BUILD_IMAGE
PULL_IMAGE
CREATE_NETWORK
CREATE_VOLUME
RUN_HEALTH_CHECK
CREATE_BACKUP
RESTORE_BACKUP

Each command requires:

* command ID
* server ID
* organization ID
* operation
* payload
* issued time
* expiry
* request ID
* correlation ID

Responses:

* accepted
* running
* completed
* failed

Do not implement business logic as raw arbitrary remote shell execution.

Keep the command schema versionable.

---

PHASE B9 — APPLICATIONS

Create application domain.

Types:

WEB_SERVICE
API
WORKER
SCHEDULED_JOB
STATIC_SITE
DOCKER_COMPOSE
DOCKER_IMAGE

Application fields:

* organization
* project
* environment
* name
* slug
* type
* target server or placement policy
* source type
* repository
* branch
* Dockerfile
* build context
* internal port
* command
* entrypoint
* CPU limit
* memory limit
* restart policy
* health check config

Endpoints:
GET /applications
POST /applications
GET /applications/{id}
PATCH /applications/{id}
DELETE /applications/{id}

Validate source-type-specific fields.

---

PHASE B10 — VARIABLES + ENCRYPTED SECRETS

Environment variable scopes:

ORGANIZATION
PROJECT
ENVIRONMENT
APPLICATION

Implement deterministic inheritance order:
organization
→ project
→ environment
→ application

Variables can be plaintext configuration.

Secrets must use authenticated encryption.

Implement envelope-style design if practical:

* platform key from secure external environment
* per-record or per-tenant data key strategy
* AEAD encryption

Never:

* log secret value
* return stored secret plaintext by default
* include secrets in audit
* include secrets in API errors

API for secrets should return metadata:

* name
* scope
* version
* updated_at
* updated_by

Updating a secret creates a new version or at minimum preserves rotation metadata.

---

PHASE B11 — DEPLOYMENT DOMAIN + STATE MACHINE

This is a critical phase.

Create state machine:

PENDING
QUEUED
PREPARING
FETCHING_SOURCE
BUILDING
IMAGE_READY
CREATING_CONTAINER
STARTING
HEALTH_CHECKING
ACTIVATING
RUNNING

Terminal failures:

SOURCE_FAILED
BUILD_FAILED
IMAGE_FAILED
CONTAINER_FAILED
START_FAILED
HEALTH_CHECK_FAILED
ROUTING_FAILED
CANCELLED
TIMEOUT

Rules:

* only valid state transitions
* every transition persisted
* every transition creates deployment_event
* terminal states immutable except explicit administrative reconciliation
* use transactions
* protect concurrent state transitions
* support idempotency

Endpoints:

POST /applications/{id}/deployments
GET /deployments
GET /deployments/{id}
POST /deployments/{id}/cancel

Do not perform long deployment execution inside the HTTP request.

Creating deployment should create/queue work and return promptly.

---

PHASE B12 — JOB QUEUE

Implement a reliable PostgreSQL-backed job queue initially.

Requirements:

* queued jobs
* workers
* leasing/locking
* visibility timeout
* retry count
* max attempts
* dead/failed status
* scheduled retry
* cancellation
* idempotency key
* worker heartbeat if useful

Jobs:
DEPLOYMENT_EXECUTION
BACKUP
RESTORE
CERTIFICATE_OPERATION
NOTIFICATION_DELIVERY

Use SKIP LOCKED or equivalent safe PostgreSQL job acquisition strategy.

Do not allow duplicate workers to execute the same lease concurrently.

---

PHASE B13 — DEPLOYMENT ORCHESTRATOR

Implement deployment workflow as orchestration, not giant handler logic.

Concept:

Deployment created
→ queue
→ worker acquires
→ validate server
→ create immutable effective configuration
→ generate revision candidate
→ issue build/source command to agent
→ process status callbacks/events
→ start candidate container
→ health check
→ activate routing
→ mark revision active
→ gracefully retire previous revision
→ mark deployment RUNNING/success

Failures must:

* persist exact stage
* preserve previous healthy revision
* avoid switching traffic
* issue cleanup where safe
* audit result

Design resumability where feasible.

---

PHASE B14 — REVISIONS

Revision is immutable.

Store:

* application
* revision number
* deployment
* commit SHA
* image digest
* image tag
* effective runtime config snapshot
* variable metadata snapshot
* secret version references
* health configuration
* resource limits
* created by
* state

States:
CREATED
READY
ACTIVE
INACTIVE
FAILED
ARCHIVED

Endpoints:
GET /applications/{id}/revisions
GET /revisions/{id}

Do not mutate deployed revision configuration.

---

PHASE B15 — ROLLBACK

Endpoint:

POST /applications/{id}/rollback

Body:
targetRevisionId

Rollback workflow:

* authorize
* validate target revision
* do NOT rebuild
* create rollback deployment record
* start/reuse target revision according to policy
* health check
* switch routing
* mark target active
* mark current inactive
* gracefully retire previous active container

If target revision fails health verification:

* keep current revision active
* rollback operation fails safely

Audit rollback.

---

PHASE B16 — GIT PROVIDERS

Create integration abstraction.

Initial provider:
GitHub

Support metadata:

* connection
* account
* repository
* branch
* webhook
* last sync

Never store provider access tokens plaintext.

Provide clean provider interface for later:
GitLab
Bitbucket
Generic Git

Support webhook verification.

Git push events may create deployments only when application auto-deploy rules match.

Protect against duplicate webhook delivery.

---

PHASE B17 — CONTAINER REGISTRIES

Registry abstraction.

Support initially:

* GHCR
* Docker Hub
* Generic OCI

Prepare for:

* GCP Artifact Registry
* ECR
* ACR

Credentials encrypted.

API should manage configuration, while agent performs pull/push operations.

---

PHASE B18 — DOMAINS + TRAEFIK ROUTING

Domain model:

* domain
* app
* environment
* internal port
* primary
* force_https
* dns_status
* tls_status

States:

DNS:
PENDING
VALID
INVALID

TLS:
PENDING
ISSUING
ACTIVE
EXPIRING
FAILED

Control plane should generate desired routing configuration/labels.

Agent executes runtime application.

Ensure one canonical domain cannot accidentally be assigned to conflicting active applications in the same routing scope.

Endpoints:
POST /applications/{id}/domains
GET /applications/{id}/domains
PATCH /domains/{id}
DELETE /domains/{id}

---

PHASE B19 — HEALTH CHECKS

Support:
HTTP
TCP
COMMAND
CONTAINER

Configuration:

* initial delay
* interval
* timeout
* retries
* expected HTTP status
* path
* port

Use health checks during deployment activation.

Never route production traffic to candidate revision before deployment health policy is satisfied.

Persist meaningful health state:
UNKNOWN
STARTING
HEALTHY
DEGRADED
UNHEALTHY

Do not create excessive database writes for every probe; design aggregation/retention appropriately.

---

PHASE B20 — LOG STREAMING CONTRACT

Design API/agent flow for:

* build logs
* deployment events
* runtime container logs

Requirements:

* authorization
* organization isolation
* bounded buffers
* client disconnect cleanup
* backpressure strategy
* optional cursor/since timestamp
* timestamps
* stdout/stderr identification where provided

Expose streaming via SSE or WebSocket where appropriate.

Do not permanently persist unlimited container logs in PostgreSQL.

Create an interface so Loki/OpenSearch/ClickHouse could be added later.

---

PHASE B21 — METRICS

Server metrics:

* CPU
* RAM
* disk
* load
* uptime
* network
* container count

Container metrics:

* CPU
* RAM
* network RX/TX
* restart count
* status

Do not store high-frequency time series indefinitely in transactional PostgreSQL.

Implement:

* current/summary metrics first
* time series abstraction for later Prometheus-compatible storage

---

PHASE B22 — DATABASE RESOURCES

Initial managed resource:
PostgreSQL.

Database entity:

* organization
* project
* environment
* server
* name
* PostgreSQL version
* storage volume
* CPU
* memory
* database name
* username
* encrypted credential
* container/runtime ID
* status
* backup policy

Creation must occur through agent commands.

Persistent storage must be independent of disposable application containers.

Never delete data volume automatically merely because application or database container is recreated.

---

PHASE B23 — VOLUMES

Volume lifecycle:

* create
* attach
* detach
* inspect
* delete

Fields:

* server
* name
* driver
* mount
* attached resource
* backup policy
* state

Protect deletion of attached or database-critical volumes.

---

PHASE B24 — BACKUP + RESTORE

Backup model:

* resource
* type
* started
* completed
* duration
* size
* checksum
* destination
* status
* retention

Initial PostgreSQL logical backup support.

Backup destinations should be abstracted:

* local
* S3-compatible later

Restore requires:

* authorization
* backup validation
* explicit target
* destructive confirmation semantics at API contract level
* job execution
* auditing

Never report restore successful until validation completes.

---

PHASE B25 — NOTIFICATIONS

Notification channels abstraction.

Initial:
EMAIL
WEBHOOK

Prepare:
SLACK
TEAMS
DISCORD
TELEGRAM
WHATSAPP

Notification policies:
event type
resource filters
environment filters
channels
enabled

Events:
DEPLOYMENT_FAILED
DEPLOYMENT_SUCCEEDED
SERVER_OFFLINE
SERVER_DEGRADED
BACKUP_FAILED
CERTIFICATE_EXPIRING
DISK_LOW

Delivery must occur asynchronously using jobs.

---

PHASE B26 — OUTGOING WEBHOOKS

Events:
deployment.started
deployment.completed
deployment.failed
server.offline
backup.completed
backup.failed

Requirements:

* HMAC signature
* delivery attempts
* status
* latency
* response code
* exponential retry
* disable/alert after repeated failures if policy requires

Never include secrets in webhook payloads.

---

PHASE B27 — AUDIT COMPLETENESS

Review every mutation endpoint.

Ensure important activity generates audit events.

Examples:

* login security events
* role change
* secret creation/update/delete
* deploy
* rollback
* server registration
* maintenance mode
* domain changes
* backup restore
* destructive actions

Audit must be append-oriented.

Protect audit records from normal user edits.

---

PHASE B28 — CAPACITY + PLACEMENT

Implement server capacity modelling.

Track:

* CPU total
* RAM total
* disk
* CPU allocated
* memory allocated
* maintenance status
* health
* labels

Initial placement:
user-selected server.

Then support optional scheduler:

candidate servers
→ filter healthy
→ filter maintenance
→ filter labels
→ validate capacity
→ score
→ select

Return `INSUFFICIENT_RESOURCES` cleanly when placement is impossible.

Do not implement Kubernetes.

---

PHASE B29 — REPLICAS

Add manual replicas.

Application runtime config:
desired_replicas

Agent/control plane reconciliation:
desired replicas vs observed replicas.

Traefik should distribute traffic between healthy replicas.

Handle:

* scale up
* scale down
* unhealthy replica
* rolling deployment

Do not add autoscaling yet.

---

PHASE B30 — RECONCILIATION LOOP

Introduce desired-state reconciliation.

Examples:

Application:
desired replicas = 3
observed healthy = 2
→ create one

Server:
expected agent online
heartbeat expired
→ mark offline

Container:
expected running
observed exited
→ reconcile according to restart policy

This is a major architecture milestone.

Keep reconciliation idempotent.

Avoid uncontrolled retry loops.

---

PHASE B31 — SECURITY HARDENING

Perform backend security review.

Review:

* authentication
* refresh tokens
* authorization
* organization isolation
* SQL query scoping
* agent credentials
* registration tokens
* secrets
* encryption
* API tokens
* rate limiting
* CORS
* request size
* file/path inputs
* webhook signatures
* SSRF risks
* command injection risks
* log injection
* audit integrity

Docker/runtime commands must use structured command contracts, never unsanitised arbitrary shell text from API requests.

Create security tests for critical paths.

---

PHASE B32 — API DOCUMENTATION

Produce OpenAPI documentation for public/control APIs.

Document:

* requests
* responses
* error codes
* permissions
* pagination
* filters
* state enums

Generate or maintain frontend-compatible API types where feasible without coupling frontend internals to backend DB types.

---

PHASE B33 — TEST STRATEGY

Create:

Unit tests:

* state machine
* RBAC
* secret encryption
* placement
* revision creation
* rollback rules

Repository integration tests:

* tenant isolation
* transactions
* constraints

HTTP tests:

* auth
* authorization
* validation
* errors

Deployment tests:

* successful state path
* build failure
* start failure
* health failure
* cancellation
* timeout
* concurrent transition attempt

Do not use tests that merely assert implementation internals.

---

PHASE B34 — PERFORMANCE + RELIABILITY

Review:

* N+1 queries
* indexes
* pagination
* long transactions
* job locking
* memory usage
* log streaming
* DB connection pool
* context cancellation
* timeouts

Create sensible timeouts for all external/agent operations.

No unbounded goroutines.

No unbounded queues.

No unbounded log buffers.

---

PHASE B35 — FINAL CONTROL PLANE AUDIT

Inspect the full `apps/api`.

Identify:

* duplicated business logic
* handlers doing service logic
* missing authorization
* missing organization filters
* unsafe SQL
* swallowed errors
* inconsistent error codes
* missing transactions
* secrets in logs
* uncontrolled goroutines
* non-idempotent job execution
* invalid state transition possibilities
* insufficient tests
* missing indexes
* migration problems

Run:

go fmt ./...
go vet ./...
go test ./...

Run staticcheck if installed.

Fix issues created by this work.

Document only genuine remaining risks.

Do not claim production readiness if unresolved security or consistency issues remain.

## One important addition: build `apps/agent` separately

I would **not include Docker implementation inside `apps/api`**. After B7/B8 establishes the protocol, create a separate:

```text
apps/agent
```

with Go.

That component would own:

```text
Docker Engine SDK
container lifecycle
image builds
image pulls
networks
volumes
logs
container metrics
health execution
Traefik runtime configuration
backup execution
```

The API says **what should happen**.

The Agent decides **how to execute the authorised command on that machine**.

That distinction will make the system substantially safer and easier to scale.

## Recommended Cursor workflow

For every phase, use the same pattern:

> Execute Phase B11 only. First inspect the existing implementation and dependencies related to this phase. Reuse correct existing architecture. Implement the phase completely, including migrations, validation, authorization, audit integration and tests. Do not start later phases. Run `go fmt`, `go vet` and `go test ./...`. At completion, report files changed, architectural decisions, tests added, API endpoints added, migrations added, and any unresolved risks.

Then follow it with:

> Audit the Phase B11 implementation against the master backend specification. Look specifically for authorization bypasses, tenant isolation issues, invalid state transitions, concurrency problems, missing transactions, non-idempotent behaviour, missing error codes and insufficient tests. Fix all confirmed issues and rerun the complete Go test suite.

That **implement → audit → proceed** rhythm is what I would use throughout this project.

The next piece I recommend preparing before you begin coding is the separate **`apps/agent` Cursor prompt set**, because the Go Agent is effectively the execution engine of the entire platform and deserves the same level of specification as `apps/api`.
