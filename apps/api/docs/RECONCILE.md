# Desired-state reconciliation (B30)

The control plane continuously converges observed state toward desired state.

## Loop

A ticker (`RECONCILE_INTERVAL`, default 30s) enqueues one
`DESIRED_STATE_RECONCILE` job per time bucket (idempotent). The job worker runs:

1. **Servers** — `ONLINE`/`DEGRADED` with `last_heartbeat_at` older than
   `AGENT_HEARTBEAT_TTL` (default 90s) → mark `OFFLINE` (+ notify/webhook)
2. **Applications** — compare `desiredReplicas` vs observed slots; call
   `replicas.Reconciler` when missing / unhealthy / excess
3. **Containers** — for failed/unhealthy/stopped slots still in desired range,
   apply `restart_policy` with exponential backoff

## Restart policy

| Policy | Behavior |
| --- | --- |
| `always` | Restart failed/unhealthy/(non-intentional) stopped |
| `unless-stopped` | Restart failed/unhealthy/stopped except scale-down/retire |
| `on-failure` | Restart failed/unhealthy or non-zero `observed_exit_code` |
| `no` | Never auto-restart |

Backoff: `RECONCILE_RESTART_BACKOFF_BASE * 2^attempt`, capped at 5m.
Stops after `RECONCILE_RESTART_MAX_ATTEMPTS` (default 5).
Healthy `RUNNING` resets attempt counters.

## Safety

- Time-bucketed job idempotency keys (`desired-state-reconcile:{bucket}`)
- Job lease + max attempts (no uncontrolled retry storms)
- Per-tick caps: `RECONCILE_MAX_APP_ACTIONS_PER_TICK`,
  `RECONCILE_MAX_RESTART_ACTIONS_PER_TICK`
- Skip apps whose target server is `OFFLINE` / `DISABLED` / `MAINTENANCE`
- Scale-down sets `last_error=scale_down` so `unless-stopped` does not fight it

## Config

| Env | Default |
| --- | --- |
| `RECONCILE_ENABLED` | `true` |
| `RECONCILE_INTERVAL` | `30s` |
| `AGENT_HEARTBEAT_TTL` | `90s` |
| `RECONCILE_MAX_APP_ACTIONS_PER_TICK` | `50` |
| `RECONCILE_MAX_RESTART_ACTIONS_PER_TICK` | `25` |
| `RECONCILE_RESTART_MAX_ATTEMPTS` | `5` |
| `RECONCILE_RESTART_BACKOFF_BASE` | `15s` |

## Migration

`000024_desired_state_reconcile` expands `jobs.type` and adds replica restart
backoff columns.
