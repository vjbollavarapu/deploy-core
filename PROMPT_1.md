Good. The next logical step is to define **`apps/agent`**, because that is the component that turns your control plane into an actual deployment platform.

Below is the Cursor prompt set I would use for the Go Agent.

# CURSOR MASTER INSTRUCTION — GO SERVER AGENT

You are building the DeployCore Server Agent.

Scope:
`apps/agent`

DeployCore architecture:

Next.js Frontend
↓
Go Control Plane API
↓
Secure Agent Protocol
↓
Go Server Agent
↓
Docker Engine
↓
Traefik + Application Containers + Stateful Resources

The Agent is the EXECUTION PLANE.

The Control Plane decides:

* what should happen
* who is authorized
* which revision/configuration is desired
* which server should execute the work

The Agent executes only authenticated, validated, structured commands.

The Agent must never become a second business-logic backend.

---

# CORE RESPONSIBILITIES

The Agent owns:

* server registration
* durable agent identity
* heartbeat
* server capability discovery
* Docker Engine connectivity
* container lifecycle
* image lifecycle
* image builds
* image pulls/pushes
* Docker networks
* Docker volumes
* runtime logs
* container statistics
* host statistics
* health-check execution
* deployment execution primitives
* Traefik integration
* backup execution primitives
* restore execution primitives
* agent self-status
* controlled upgrades later

The Agent does NOT own:

* user authentication
* RBAC
* organization permissions
* project/business rules
* deployment policy decisions
* billing
* audit policy
* application placement decisions
* revision selection
* approval workflows

Those belong to `apps/api`.

---

# NON-NEGOTIABLE SECURITY RULES

1. Do not expose a general remote shell API.
2. Do not accept arbitrary shell commands from the Control Plane.
3. Do not interpolate untrusted text into shell commands.
4. Prefer Docker Engine SDK/API rather than Docker CLI.
5. Validate all command payloads.
6. Authenticate every command.
7. Verify command target server/agent identity.
8. Reject expired commands.
9. Reject replayed commands.
10. Support idempotency.
11. Restrict file access to platform-owned directories.
12. Do not allow arbitrary host path mounts.
13. Do not mount Docker socket into customer application containers.
14. Never return host secrets.
15. Never log credentials or secret environment values.
16. Use least privilege where possible.
17. Treat the Agent as privileged infrastructure software.
18. Use structured logs.
19. Support graceful shutdown.
20. Bound all concurrency.

---

# RECOMMENDED STRUCTURE

apps/agent/
├── cmd/
│   └── agent/
│       └── main.go
│
├── internal/
│   ├── agent/
│   ├── config/
│   ├── registration/
│   ├── transport/
│   ├── commands/
│   ├── docker/
│   ├── containers/
│   ├── images/
│   ├── builds/
│   ├── networks/
│   ├── volumes/
│   ├── logs/
│   ├── metrics/
│   ├── health/
│   ├── proxy/
│   ├── backups/
│   ├── filesystem/
│   ├── security/
│   └── updater/
│
├── pkg/
│   ├── protocol/
│   ├── logger/
│   └── version/
│
└── tests/

Do not create unnecessary abstractions or interfaces.

---

# PHASE A1 — AGENT BOOTSTRAP

Inspect existing `apps/agent`.

If absent, create a clean Go service.

Implement:

* configuration loader
* structured logger
* version information
* startup validation
* graceful shutdown
* signal handling
* health status
* runtime directories
* Docker connectivity check
* Control Plane connectivity check

Configuration should support:

AGENT_SERVER_ID
AGENT_CONTROL_PLANE_URL
AGENT_CREDENTIAL_PATH
AGENT_DATA_DIR
AGENT_LOG_LEVEL
AGENT_DOCKER_HOST
AGENT_HEARTBEAT_INTERVAL

Default secure runtime paths under something like:

/var/lib/deploycore-agent
/etc/deploycore-agent
/var/log/deploycore-agent

Do not assume the process is run from a particular working directory.

Run:
go fmt ./...
go vet ./...
go test ./...

---

# PHASE A2 — REGISTRATION

Implement first-time agent registration.

Flow:

1. Agent starts without durable credentials.
2. User supplies one-time registration token.
3. Agent calls Control Plane registration endpoint.
4. Sends:

   * token
   * hostname
   * OS
   * architecture
   * agent version
   * machine identity information where safe
5. Control Plane validates and returns durable agent identity.
6. Agent securely stores credential locally.
7. Registration token is discarded.
8. Future startup uses durable credential.

Requirements:

* registration token never written to logs
* credential file permission restricted
* atomic credential writes
* no plaintext duplication across multiple files
* failed registration retries with bounded exponential backoff
* clear failure reason without leaking sensitive details

Prepare architecture for mTLS later even if initial implementation uses bearer-style agent credentials.

---

# PHASE A3 — AGENT AUTHENTICATION + TRANSPORT

Create secure transport to Control Plane.

Initial acceptable protocol:
HTTPS + authenticated long polling or WebSocket.

Preferred long-term:
gRPC + mTLS.

Requirements:

* agent identity attached to all requests
* TLS verification required
* reconnect automatically
* exponential backoff
* jitter
* bounded retry
* connection state reporting
* ping/keepalive
* graceful disconnect

Do not disable TLS verification outside explicitly controlled local development configuration.

Create transport abstraction so later conversion to gRPC does not require rewriting execution modules.

---

# PHASE A4 — HEARTBEAT + HOST INVENTORY

Send periodic heartbeat containing:

* agent version
* Docker status
* Docker version
* hostname
* OS
* architecture
* uptime
* CPU cores
* CPU usage
* total RAM
* used RAM
* total disk
* used disk
* load averages where supported
* container count
* running container count
* image count
* volume count
* network count

Ensure metric collection is efficient.

Do not run expensive full Docker scans every few seconds.

Heartbeat should include timestamp and monotonic sequence where useful.

Support state:
HEALTHY
DEGRADED
ERROR

---

# PHASE A5 — DOCKER CLIENT

Implement Docker Engine integration using official Go SDK.

Provide internal operations:

* Ping
* Info
* Version
* ListContainers
* InspectContainer
* StartContainer
* StopContainer
* RestartContainer
* RemoveContainer
* CreateContainer
* ListImages
* InspectImage
* PullImage
* RemoveImage
* ListNetworks
* CreateNetwork
* RemoveNetwork
* ListVolumes
* CreateVolume
* RemoveVolume
* StreamLogs
* ContainerStats
* DockerEvents

Use contexts with deadlines.

Map Docker errors into stable agent error codes.

Do not leak raw Docker daemon responses directly to API clients.

---

# PHASE A6 — COMMAND EXECUTION FRAMEWORK

Implement structured command execution.

Command envelope:

* command_id
* correlation_id
* request_id
* organization_id
* server_id
* operation
* issued_at
* expires_at
* payload_version
* payload

States:

RECEIVED
VALIDATING
ACCEPTED
RUNNING
COMPLETED
FAILED
EXPIRED
REJECTED
CANCELLED

Requirements:

* command ID uniqueness
* replay protection
* expiry checks
* server ID validation
* persistent recent-command journal if necessary
* idempotency
* bounded parallelism
* per-command context cancellation
* structured result
* stable error codes

Never deserialize arbitrary operations into generic shell execution.

---

# PHASE A7 — CONTAINER CREATE CONTRACT

Implement safe structured container creation.

Allowed properties:

* platform container name
* image
* command
* entrypoint
* environment values supplied securely
* internal ports
* CPU limit
* memory limit
* restart policy
* network attachments
* managed volume attachments
* Traefik labels generated by trusted logic
* health configuration
* read-only root filesystem option where compatible

Validate:

* names
* image references
* ports
* resource limits
* volume IDs
* network IDs
* label prefixes

Block by default:

* privileged mode
* host PID
* host IPC
* host network
* Docker socket mount
* arbitrary `/` host paths
* device passthrough
* dangerous capabilities

Create explicit privileged-policy hooks for possible future admin-authorized workloads, but keep them disabled by default.

---

# PHASE A8 — IMAGE PULL

Implement image pull operation.

Support:

* public registries
* authenticated registries

Credentials should be received through secure execution context and held only as long as needed.

Do not persist registry password unnecessarily.

Return:

* digest
* image ID
* size where available
* pull duration
* status

Stream progress events to Control Plane.

---

# PHASE A9 — IMAGE BUILD

Implement Docker image build.

Build request supports:

* build context identifier/path within approved workspace
* Dockerfile path
* build args
* target stage
* platform
* cache policy
* tags

Never allow arbitrary host filesystem as build context.

Create isolated deployment workspace:

/var/lib/deploycore-agent/workspaces/<deployment-id>

Lifecycle:

create workspace
→ fetch/materialize source
→ build
→ collect result
→ clean according to retention policy

Stream build output.

Support cancellation.

Apply build timeout.

Protect disk space.

---

# PHASE A10 — SOURCE WORKSPACE

Implement safe source workspace management.

The Agent may receive source archive or trusted retrieval instructions depending on final Control Plane design.

Workspace rules:

* unique per deployment
* no traversal outside workspace
* validate extracted paths
* reject `../`
* reject symlink escapes
* enforce max archive size
* enforce max extracted size
* clean failed workspaces
* retain temporarily only for debugging according to policy

Do not expose arbitrary filesystem browsing.

---

# PHASE A11 — APPLICATION CONTAINERS

Implement platform naming standard.

Example:

dc-<application-short-id>-r<revision>-<instance>

Example:

dc-dayaapi-r49-1

Add trusted labels:

deploycore.managed=true
deploycore.application_id=...
deploycore.revision_id=...
deploycore.deployment_id=...
deploycore.environment_id=...
deploycore.organization_id=...

Use labels for discovery and reconciliation.

Never trust container names alone as ownership proof.

---

# PHASE A12 — NETWORKS

Implement platform-managed Docker networks.

Naming example:

dc-<project>-<environment>-private

Proxy network:

deploycore-proxy

Requirements:

* inspect before create
* idempotent create
* ownership labels
* prevent accidental deletion when connected resources exist
* do not modify unmanaged networks without explicit import/ownership workflow

Support application isolation.

---

# PHASE A13 — VOLUMES

Implement managed volumes.

Requirements:

* labels proving platform ownership
* create
* inspect
* attach
* detach
* delete
* usage metadata where available

Never delete volume as side effect of normal application revision replacement.

Require explicit destructive volume delete command.

Block deletion if attached unless forced by separately authorized control-plane policy.

---

# PHASE A14 — TRAEFIK INTEGRATION

Integrate application routing using Docker labels where possible.

Do not manually rewrite static Traefik config per deployment unless necessary.

Generated trusted labels may include:

traefik.enable=true

router rule based on authorized domain

service port

TLS

middlewares

Requirements:

* labels generated from structured domain config
* escape/validate hostnames
* no arbitrary label injection from application users
* support HTTP→HTTPS
* WebSocket-compatible routing
* multiple domains
* primary domain
* redirect policies

Traefik network should be explicitly controlled.

---

# PHASE A15 — CANDIDATE REVISION START

Implement candidate deployment primitive.

Flow:

1. Ensure image exists.
2. Ensure required networks.
3. Ensure volumes.
4. Create candidate container.
5. Start.
6. Wait for runtime start.
7. Perform health checks.
8. Report candidate READY only after policy satisfied.

Do not expose candidate to production traffic prematurely.

Candidate container must carry revision/deployment labels.

---

# PHASE A16 — HEALTH EXECUTION

Support:

HTTP
TCP
COMMAND
CONTAINER_STATE

HTTP:

* scheme
* port
* path
* expected code/range
* timeout

TCP:

* port
* timeout

Command:
Only prevalidated command arrays associated with container health checks.
Do not execute arbitrary host command.

Health process:

initial delay
→ checks
→ successes/failures
→ threshold

Return structured observations.

---

# PHASE A17 — ZERO-DOWNTIME ACTIVATION

Implement activation primitives supporting:

current revision active
+
candidate revision ready

Flow:

candidate healthy
→ attach/enable routing
→ verify route if configured
→ report activation complete
→ old revision enters drain
→ graceful shutdown
→ stop old container according to retention policy

Ensure failure before switch leaves current revision untouched.

Where possible make activation idempotent.

---

# PHASE A18 — GRACEFUL STOP + DRAIN

Container stop should support:

* termination timeout
* graceful SIGTERM
* force kill only after timeout
* status result

For traffic services:

* remove/drain routing first where required
* wait configured drain period
* stop

Do not immediately kill production containers unless explicitly requested.

---

# PHASE A19 — ROLLBACK EXECUTION

Rollback receives an existing immutable revision configuration from Control Plane.

Agent must not rebuild.

Flow:

verify image exists or pull by immutable digest
→ create/start target revision
→ health check
→ activate
→ drain current
→ report complete

If target fails:

* do not switch traffic
* clean failed candidate
* report failure

---

# PHASE A20 — RUNTIME LOGS

Implement Docker log streaming.

Support:

* stdout
* stderr
* timestamps
* since
* tail
* follow
* cancellation

Requirements:

* bounded buffers
* respect slow consumers
* clean client disconnect
* no uncontrolled goroutine leak
* do not persist logs in Agent indefinitely

Redaction:
Do not attempt simplistic broad secret replacement that corrupts logs, but support configured sensitive-value redaction where the Control Plane safely supplies fingerprint/policy.

---

# PHASE A21 — BUILD LOGS

Stream build logs as structured events:

timestamp
stream
message
stage where identifiable

Control Plane should receive incrementally.

Allow:

* cancellation
* bounded buffering
* retention policy

---

# PHASE A22 — CONTAINER STATS

Collect:

CPU
memory
memory limit
network RX
network TX
block IO where useful
PID count where useful
restart count

Use efficient polling.

Do not open permanent Docker stats streams for every container if not required.

Provide current stats and optional sampling.

---

# PHASE A23 — DOCKER EVENT WATCHER

Subscribe to Docker events.

Watch managed containers only.

Relevant:

* start
* stop
* die
* restart
* destroy
* health_status

Translate into platform events.

Do not flood Control Plane with irrelevant Docker events.

Reconnect event stream automatically.

---

# PHASE A24 — RECONCILIATION SUPPORT

Agent should periodically inventory managed resources.

Compare observed local resources to commands/desired state supplied by Control Plane.

Report:

* missing expected container
* unexpected managed container
* exited container
* missing network
* missing volume
* unhealthy container

Agent should not independently make large policy decisions.

Control Plane decides desired-state reconciliation policy.

Simple locally safe restart policy may still be delegated where explicit.

---

# PHASE A25 — HOST RESOURCE SAFETY

Before expensive operation, validate:

* available disk
* available memory
* Docker health

Image build should fail gracefully when disk is below configured safety threshold.

Provide stable errors:
INSUFFICIENT_DISK
INSUFFICIENT_MEMORY
DOCKER_UNAVAILABLE

Never allow builds to consume disk until host becomes unusable.

---

# PHASE A26 — DISK CLEANUP

Implement safe platform-owned cleanup.

Eligible:

* expired workspaces
* stopped superseded managed containers
* expired unused managed images according to policy
* temporary build files

Never indiscriminately run destructive global Docker prune.

Do not remove:

* unmanaged resources
* active revision images
* backup-related volumes
* database volumes

Generate cleanup report.

---

# PHASE A27 — DATABASE CONTAINER EXECUTION

Support PostgreSQL runtime as a stateful resource.

Requirements:

* separate persistent Docker volume
* stable service identity
* private project/environment network
* resource limits
* restart policy
* health checks
* no public exposure by default

Creation input:

* Postgres version/image
* database name
* username
* password supplied securely
* CPU
* memory
* storage volume
* network

Never place database data inside disposable container filesystem.

---

# PHASE A28 — DATABASE BACKUP

Implement PostgreSQL logical backup primitive.

Preferred mechanism:
trusted fixed tool execution such as pg_dump with structured arguments.

Do not allow arbitrary host command strings.

Support:

* compressed dump
* checksum
* output size
* duration
* backup ID
* destination staging

Credentials must not be printed in process logs.

Cleanup temporary backup files according to policy.

---

# PHASE A29 — DATABASE RESTORE

Implement controlled PostgreSQL restore.

Requirements:

* explicit backup ID/path within managed backup area
* integrity/checksum verification
* controlled target DB
* progress
* timeout
* error handling
* post-restore validation

Do not allow arbitrary file path restore.

---

# PHASE A30 — BACKUP DESTINATION ABSTRACTION

Create internal interface for:

LocalManagedStorage

Prepare later:
S3Compatible

Do not overbuild cloud integrations yet.

Backup paths must be platform controlled.

---

# PHASE A31 — MANAGED FILESYSTEM

Centralize filesystem access.

Allowed roots:

data directory
workspace directory
backup staging
agent credential directory
managed certificate/config directory if applicable

Provide safe path helper:

* clean
* resolve
* verify under allowed root
* symlink checks where applicable

No user-supplied absolute path should bypass these guards.

---

# PHASE A32 — INSTALLER

Create installation scripts under:

deployments/install/

Target:
Ubuntu LTS.

Installer should:

* require/root escalate appropriately
* validate architecture
* create deploycore system user where useful
* install Agent binary
* create config directory
* create data directory
* create log/runtime directories
* set permissions
* optionally install Docker if explicitly chosen
* validate Docker
* install systemd unit
* register Agent
* start service
* report status

Do not curl arbitrary mutable code then run it without integrity strategy in mature implementation.

Prepare checksum/signature verification.

---

# PHASE A33 — SYSTEMD

Create hardened systemd service where compatible.

Consider:

Restart=always/on-failure
RestartSec
LimitNOFILE
NoNewPrivileges where possible
ProtectSystem where compatible
ProtectHome
PrivateTmp

Be careful: Docker access and managed filesystem needs may limit some hardening flags.

Test instead of blindly adding incompatible sandboxing.

---

# PHASE A34 — AGENT UPDATE DESIGN

Prepare safe self-update architecture.

Do not implement unsafe automatic binary replacement.

Required future flow:

Control Plane announces approved version
→ Agent downloads release
→ verify checksum/signature
→ stage
→ atomically replace
→ restart systemd service
→ report new version

Support rollback if startup fails.

Initially expose version mismatch only.

---

# PHASE A35 — CONCURRENCY CONTROL

Define worker pools.

Examples:

deployment/build concurrency
backup concurrency
container operations
log streams

Configuration:

MAX_CONCURRENT_BUILDS
MAX_CONCURRENT_DEPLOYMENTS
MAX_CONCURRENT_BACKUPS

Do not create unbounded goroutines per command.

Use semaphore/worker-pool pattern.

---

# PHASE A36 — CANCELLATION

Long-running commands must support cancellation:

image pull where possible
build
health wait
logs
backup
restore

Cancellation must:

* propagate context
* update command state
* clean partial safe resources
* avoid deleting previously healthy revision

---

# PHASE A37 — IDEMPOTENCY

Audit all Agent commands.

Examples:

CreateNetwork existing same managed network
→ success

StartContainer already running
→ success

StopContainer already stopped
→ success

CreateVolume already exists with same expected identity
→ success

Repeated deployment command
→ must not create duplicate candidate uncontrolled

Use labels and command journal.

---

# PHASE A38 — ERROR MODEL

Stable Agent error codes:

INVALID_COMMAND
COMMAND_EXPIRED
COMMAND_REPLAYED
UNAUTHORIZED_COMMAND

DOCKER_UNAVAILABLE

IMAGE_NOT_FOUND
IMAGE_PULL_FAILED
IMAGE_BUILD_FAILED

CONTAINER_NOT_FOUND
CONTAINER_CREATE_FAILED
CONTAINER_START_FAILED
CONTAINER_STOP_FAILED

NETWORK_NOT_FOUND
NETWORK_CONFLICT

VOLUME_NOT_FOUND
VOLUME_IN_USE

HEALTH_CHECK_FAILED

INSUFFICIENT_DISK
INSUFFICIENT_MEMORY

BACKUP_FAILED
RESTORE_FAILED

ROUTING_FAILED

WORKSPACE_INVALID
FILESYSTEM_ACCESS_DENIED

Return sanitized messages plus correlation IDs.

---

# PHASE A39 — SECURITY AUDIT

Review specifically for:

command injection
path traversal
symlink escape
Docker socket abuse
privileged containers
host mounts
unsafe capabilities
credential logging
registration replay
command replay
TLS bypass
unbounded payloads
archive bombs
zip/tar traversal
resource exhaustion
goroutine leaks
unbounded logs
unsafe image references
unvalidated Traefik labels

Fix confirmed issues.

---

# PHASE A40 — TEST SUITE

Unit tests:

* command validation
* expiry
* replay
* idempotency
* safe paths
* image reference validation
* resource limits
* network validation
* volume deletion protection
* health logic

Docker integration tests where environment permits:

* create managed network
* create/start container
* logs
* stop/remove
* volume persistence
* revision switch

Security tests:

* traversal
* malicious archive
* invalid host mount
* privileged request
* Docker socket mount
* replay
* wrong server identity

---

# PHASE A41 — FAILURE SIMULATION

Test:

Docker unavailable
Control Plane unavailable
network disconnect
build timeout
build cancelled
image pull failure
candidate crash
health failure
routing failure
old container stop timeout
server low disk
database backup failure
restore failure

Agent must recover cleanly.

---

# PHASE A42 — FINAL AGENT AUDIT

Inspect full `apps/agent`.

Find:

* arbitrary shell execution
* unsafe paths
* missing context timeout
* goroutine leaks
* missing cancellation
* direct secret logging
* Docker errors leaked raw
* resources without ownership labels
* missing idempotency
* dangerous cleanup
* unbounded memory/log buffers
* weak registration storage
* duplicated Docker logic
* huge command switch functions
* uncontrolled retries

Run:

go fmt ./...
go vet ./...
go test ./...

Run race detector where feasible:

go test -race ./...

Do not claim production readiness if privileged-execution risks remain unresolved.

There is also one architectural addition I would make now: create a **shared Go protocol package** so `apps/api` and `apps/agent` cannot drift independently.

```text
deploycore/
├── apps/
│   ├── frontend/
│   ├── api/
│   └── agent/
│
├── packages/
│   └── protocol-go/
│
└── ...
```

`protocol-go` should contain only things that genuinely cross the boundary:

```text
CommandEnvelope
CommandResult
AgentHeartbeat
AgentRegistration
DeploymentExecutionRequest
ContainerSpec
NetworkSpec
VolumeSpec
HealthCheckSpec
RuntimeEvent
LogEvent
MetricSnapshot
protocol version constants
```

It should **not** contain API business entities such as `Organization`, `User`, `Role`, or billing concepts.

## One more Cursor prompt I strongly recommend

Use this before B8/A6 so Cursor establishes the contract once.

Design and implement the versioned protocol shared between `apps/api` and `apps/agent`.

Create a small shared Go module such as:

`packages/protocol-go`

Its only responsibility is typed communication between Control Plane and Server Agent.

Do not put business-domain persistence models in this package.

Design versioned types for:

1. Agent registration
2. Agent authentication metadata
3. Heartbeats
4. Host inventory
5. Command envelope
6. Command acknowledgement
7. Command progress
8. Command completion
9. Structured command errors
10. Runtime events
11. Build log events
12. Container log events
13. Metric snapshots

Command types should include:

BUILD_IMAGE
PULL_IMAGE

CREATE_CONTAINER
START_CONTAINER
STOP_CONTAINER
RESTART_CONTAINER
REMOVE_CONTAINER

CREATE_NETWORK
REMOVE_NETWORK

CREATE_VOLUME
REMOVE_VOLUME

RUN_HEALTH_CHECK

ACTIVATE_REVISION
DEACTIVATE_REVISION

CREATE_DATABASE
BACKUP_DATABASE
RESTORE_DATABASE

Do not introduce an EXEC_SHELL or RUN_ARBITRARY_COMMAND command.

Command envelope must contain:

* protocolVersion
* commandId
* correlationId
* requestId
* organizationId
* serverId
* operation
* issuedAt
* expiresAt
* payload

Define strongly typed payloads instead of `map[string]any` for normal operations.

Container specification must include:

* name
* image reference/digest
* command array
* entrypoint array
* environment
* exposed/internal ports
* CPU limit
* memory limit
* restart policy
* managed networks
* managed volumes
* health check
* platform-owned routing configuration

Do not permit arbitrary host Docker configuration through this DTO.

Create protocol compatibility rules.

For example:

Protocol Major:
breaking compatibility

Protocol Minor:
backward-compatible additions

Agent should report:
protocol version
agent version

Control Plane should reject an unsupported major protocol version with a stable reason.

Add JSON serialization tests ensuring messages remain stable.

Add validation methods where appropriate.

Document every command and lifecycle expectation.

Run tests and ensure both `apps/api` and `apps/agent` compile against the shared protocol.

Once these three areas exist:

**Next.js frontend → Go control plane → Go agent**

you have the real skeleton of the platform. After that, the most valuable next step is to define the **complete repository structure, PostgreSQL schema/ERD, and build order from an empty repository through the first real deployment to Ubuntu**. That will prevent Cursor from making architectural choices ad hoc as you start implementing.
