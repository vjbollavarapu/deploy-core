# Manual replicas (B29)

DeployCore supports manual replica counts per application. There is no
autoscaling (that stays out of scope).

## Desired state

`application_configs.runtime_config.desiredReplicas` (integer, default **1**,
max **20**).

Example:

```json
{
  "desiredReplicas": 3,
  "diskBytes": 1073741824
}
```

Capacity allocation (B28) multiplies per-replica CPU / memory / disk by
`desiredReplicas`.

## Observed state

Table `application_replicas` tracks slots:

| Field | Meaning |
| --- | --- |
| `replica_index` | `0 .. desired-1` |
| `status` | `PENDING` / `STARTING` / `RUNNING` / `UNHEALTHY` / `STOPPING` / `STOPPED` / `FAILED` |
| `healthy` | Last probe outcome |
| `routing_enabled` | Traefik should send traffic here |
| `container_name` | Deterministic `{slug}-r{revision}-{index}` |

## APIs

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/v1/applications/{id}/replicas` | Desired vs observed summary |
| POST | `/api/v1/applications/{id}/replicas/scale` | `{ "desiredReplicas": N }` |
| POST | `/api/v1/applications/{id}/replicas/{index}/unhealthy` | Drain + enqueue replace |

Scale updates config, refreshes capacity, and enqueues `REPLICAS_RECONCILE`.

## Deploy / rolling

Orchestrator create → start → health → activate loops over replica indexes.
Activation enables Traefik labels on healthy candidates, then retires previous
revision replicas (`reason=rolling_replace`).

## Traefik

Domain routing uses **service name = application slug**. Every healthy replica
container carries the same Traefik service labels plus:

- `deploycore.replica.index`
- `deploycore.replica.role=member`
- `deploycore.routing.mode=replicas`

Docker Traefik provider load-balances across containers sharing that service.
Scale-down / unhealthy paths set `drainRouting` before `STOP_CONTAINER`.

## Agent payloads

Deploy/start/stop/health commands include `replicaIndex` and `containerName`.
Routing phases may include `routingLabels` and `routingEnabled`.

## Out of scope

- Autoscaling / HPA
- Kubernetes

Full control-plane reconciliation loop: see `docs/RECONCILE.md` (B30).
