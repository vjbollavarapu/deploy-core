# `protocol-go`: Control Plane & Server Agent Wire Protocol

The `packages/protocol-go` package defines the versioned, strongly typed wire contracts exchanged between the DeployCore Control Plane (`apps/api`) and Server Agent (`apps/agent`).

> **Architectural Constraint:** This module contains **only** typed wire communication contracts, serialization logic, and boundary validation. It does **not** include business-domain persistence models, database schemas, or direct host execution logic.

---

## 1. Protocol Versioning & Compatibility Rules

The wire protocol uses strict semantic versioning semantics:

* **Protocol Major (`ProtocolMajor`)**:
  - Incremented on breaking wire changes (field removal, structural type mutations, changed lifecycle semantics).
  - Incompatible major versions are strictly rejected by both Control Plane and Agent with the stable reason code: `ErrUnsupportedProtocolVersion` (`"UNSUPPORTED_PROTOCOL_VERSION"`).
* **Protocol Minor (`ProtocolMinor`)**:
  - Incremented on backward-compatible additions (new optional fields, new operation constants, new telemetry attributes).
  - Components ignore unrecognized optional fields without failing.

### Compatibility Enforcement

```go
// CheckCompatibility returns ErrMajorVersionMismatch if the major version diverges.
err := protocol.CheckCompatibility(agentReport.ProtocolMajor, agentReport.ProtocolMinor)
if err != nil {
    // Reject connection with ERR_UNSUPPORTED_PROTOCOL_VERSION
}
```

---

## 2. Core Protocol Message Types

The protocol defines versioned DTOs for 13 essential communication categories:

1. **Agent Registration** ([`RegisterRequest`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/auth.go), [`RegisterResult`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/auth.go))
2. **Authentication Metadata** ([`AuthMetadata`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/auth.go), standard `X-DeployCore-*` transport headers)
3. **Heartbeats** ([`HeartbeatRequest`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/auth.go), [`HeartbeatResponse`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/auth.go))
4. **Host Inventory** ([`HostInventory`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/inventory.go))
5. **Command Envelope** ([`CommandEnvelope`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/commands.go))
6. **Command Acknowledgement** ([`CommandAckRequest`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/commands.go))
7. **Command Progress** ([`CommandProgressRequest`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/commands.go))
8. **Command Completion** ([`CommandCompletionRequest`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/commands.go))
9. **Structured Command Errors** ([`CommandError`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/commands.go))
10. **Runtime Events** ([`RuntimeEvent`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/events.go))
11. **Build Log Events** ([`BuildLogEvent`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/events.go))
12. **Container Log Events** ([`LogIngestRequest`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/telemetry.go))
13. **Metric Snapshots** ([`MetricIngestRequest`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/telemetry.go))

---

## 3. Command Lifecycle

Commands transition through explicit, observable states:

```
[Control Plane]                                                [Server Agent]
  │                                                               │
  │ ── 1. Issue Command (Status: "pending") ────────────────────> │
  │                                                               │
  │ <── 2. CommandAckRequest (Status: "accepted") ─────────────── │
  │                                                               │
  │       [State: "running"]                                      │
  │ <── 3. CommandProgressRequest (optional stages/percents) ──── │
  │                                                               │
  │ <── 4. CommandCompletionRequest ("completed" | "failed") ──── │
```

### Lifecycle Guarantees:
* **Idempotency**: All operations (start, stop, remove, create network/volume) verify local state and succeed if the target state is already achieved.
* **Bounded Expiry**: Every `CommandEnvelope` carries `IssuedAt` and `ExpiresAt`. Commands received past `ExpiresAt` are rejected with `ERR_COMMAND_EXPIRED`.
* **Correlated Errors**: Errors return stable enum codes with sensitive token redaction via `SanitizeMessage`.

---

## 4. Supported Command Catalog

| Operation | Typed Payload | Description |
| :--- | :--- | :--- |
| `BUILD_IMAGE` | [`BuildImagePayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Compiles Docker image from source workspace or context archive. |
| `PULL_IMAGE` | [`PullImagePayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Pulls container image from authenticated registry. |
| `CREATE_CONTAINER` | [`CreateContainerPayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Prepares container using hardened `ContainerSpec`. |
| `START_CONTAINER` | [`StartContainerPayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Starts container with bounded timeout. |
| `STOP_CONTAINER` | [`StopContainerPayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Gracefully stops container (SIGTERM → timeout → SIGKILL). |
| `RESTART_CONTAINER` | [`RestartContainerPayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Restarts container with timeout. |
| `REMOVE_CONTAINER` | [`RemoveContainerPayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Removes container and optionally associated non-persistent volumes. |
| `CREATE_NETWORK` | [`CreateNetworkPayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Creates isolated bridge network with project/env labels. |
| `REMOVE_NETWORK` | [`RemoveNetworkPayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Removes managed Docker network. |
| `CREATE_VOLUME` | [`CreateVolumePayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Creates managed named volume. |
| `REMOVE_VOLUME` | [`RemoveVolumePayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Removes managed volume if unreferenced. |
| `RUN_HEALTH_CHECK` | [`RunHealthCheckPayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Executes HTTP/TCP/exec health check probe against container. |
| `ACTIVATE_REVISION` | [`ActivateRevisionPayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Attaches candidate container to ingress router (Traefik). |
| `DEACTIVATE_REVISION`| [`DeactivateRevisionPayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Drains traffic from retiring container revision. |
| `CREATE_DATABASE` | [`CreateDatabasePayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Provisions stateful database container with private networking and persistent storage. |
| `BACKUP_DATABASE` | [`BackupDatabasePayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Executes logical backup (`pg_dump`) to staged storage. |
| `RESTORE_DATABASE` | [`RestoreDatabasePayload`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/payloads.go) | Validates checksum and restores database from logical dump. |

> **Security Mandate:** `EXEC_SHELL` or `RUN_ARBITRARY_COMMAND` operations are **strictly prohibited** by design and will fail validation.

---

## 5. Container Specification (`ContainerSpec`)

The [`ContainerSpec`](file:///Users/vijayababubollavarapu/dev/deploy-core/packages/protocol-go/containers.go) struct defines strict parameters for application workloads:

* `Name`, `Image`
* `Command`, `Entrypoint`
* `Environment` (key-value map)
* `Ports` (containerPort, hostPort, protocol)
* `CPULimit`, `MemoryLimitBytes`
* `RestartPolicy` (`"no"`, `"always"`, `"on-failure"`, `"unless-stopped"`)
* `ManagedNetworks`
* `ManagedVolumes`
* `HealthCheck`
* `Routing`

### Strict Security Constraints:
* Arbitrary host Docker configurations (`HostConfig`, `Privileged`, `HostNetwork`, `HostPID`, raw docker socket bindings) are **completely omitted** from the DTO.
* Sensitive mount destinations (`/`, `/etc`, `/proc`, `/sys`, `/dev`, `/var/run/docker.sock`) are rejected during schema validation.
