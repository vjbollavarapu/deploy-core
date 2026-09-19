# Capacity + placement (B28)

DeployCore tracks per-server inventory and reservations, then places applications
onto servers without Kubernetes.

## Inventory vs allocated

| Field | Source |
| --- | --- |
| `cpu_cores` / `memory_bytes` / `disk_bytes` | Server registry inventory |
| `cpu_allocated_millis` | Sum of latest app `cpu_limit_millis` on this target |
| `memory_allocated_bytes` | Sum of latest app `memory_limit_bytes` |
| `disk_allocated_bytes` | Sum of latest app `runtime_config.diskBytes` |

Allocated counters are denormalized and refreshed when applications are created,
updated, deleted, or bound by the scheduler. Per-app reservations are multiplied
by `runtime_config.desiredReplicas` (default 1; see B29).

CPU placement units are **millicores** (`cores * 1000`).

## Placement modes

### Manual (default)

`applications.target_server_id` is set by the user. Deploy creation validates that
server (capacity; not maintenance/disabled). Offline hosts are allowed for
inventory targeting so agents can come online later.

### Scheduler (optional)

Set `placement_policy`:

```json
{
  "mode": "scheduler",
  "labels": { "tier": "prod" },
  "architecture": "amd64",
  "allowDegraded": false,
  "scoring": "least_loaded"
}
```

Pipeline:

1. Candidate servers in the org
2. Filter healthy (`ONLINE`, or `DEGRADED` if `allowDegraded`)
3. Filter maintenance / disabled
4. Filter label subset + architecture
5. Validate remaining CPU / memory / disk
6. Score (`least_loaded` default, or `most_free`) and select

On deploy, the selected server is written to `applications.target_server_id` and
`deployments.server_id`.

## API

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/api/v1/servers/{serverId}/capacity` | Totals, allocated, available |
| GET | `/api/v1/servers/capacity?organizationId=` | Org-wide capacity list |
| POST | `/api/v1/placement/preview` | Dry-run select (no bind) |

Preview body:

```json
{
  "organizationId": "...",
  "cpuMillis": 500,
  "memoryBytes": 1073741824,
  "diskBytes": 0,
  "forcedServerId": null,
  "policy": { "mode": "scheduler", "labels": { "tier": "prod" } }
}
```

## Errors

Impossible placement returns HTTP 409 with code `INSUFFICIENT_RESOURCES`.
