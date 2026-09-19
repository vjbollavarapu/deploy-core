# Control plane API (B32)

## Spec

- **OpenAPI 3.1:** [`openapi.json`](./openapi.json) (also embedded and served as `GET /openapi.json`)
- Base URL for versioned resources: `/api/v1/...`
- Process probes: `GET /health`, `GET /ready`

## Conventions

| Concern | Contract |
| --- | --- |
| Errors | `{ "error": { "code", "message", "requestId?", "details?" } }` — see `ErrorCode` |
| Pagination | Query `limit` (default 20, max 100) + `offset`; response `items` / `limit` / `offset` / `totalCount?` |
| Auth (user) | `Authorization: Bearer <accessToken>` |
| Auth (agent) | `Authorization: Bearer <agentCredential>` from `POST /agents/register` |
| Git webhooks | Provider signature headers on `POST /webhooks/git/{connectionId}` |
| Permissions | Org RBAC keys in `Permission` schema; `x-permission` on many operations |
| IDs / time | UUID strings; RFC3339 UTC timestamps |
| Request size | `MAX_REQUEST_BODY_BYTES` (default 1 MiB) |

## Filters (common query params)

List endpoints typically accept `organizationId` (required for most org-scoped lists).
Additional filters vary by resource, for example:

- Deployments: `applicationId`, `status`
- Audit logs: `action`, `actorId`, `resourceType`, time range
- Revisions: `status`
- Metrics series: `metric`, window params
- Logs: `kind`, `follow`, `since`, `cursor`, `limit`

See operation parameters in `openapi.json` for the full set.

## State enums

Documented as OpenAPI schemas (and TypeScript unions in the frontend contract):

| Schema | Examples |
| --- | --- |
| `ServerStatus` | `ONLINE`, `DEGRADED`, `OFFLINE`, `MAINTENANCE`, `DISABLED` |
| `DeploymentStatus` | happy path `PENDING`→`RUNNING` plus terminal failure/cancel/timeout |
| `RevisionStatus` | `CREATED`, `READY`, `ACTIVE`, `INACTIVE`, `FAILED`, `ARCHIVED` |
| `ReplicaStatus` | `PENDING`…`FAILED` |
| `VolumeState` / `DatabaseStatus` | provisioning lifecycle |
| `DNSStatus` / `TLSStatus` / `HealthStatus` | domain + probe aggregation |
| `RestartPolicy` | `always`, `unless-stopped`, `on-failure`, `no` |

## Frontend types

Wire-format TypeScript (not UI view-models):

`apps/frontend/lib/api/contract.ts`

Keep `apps/frontend/lib/types.ts` as presentation models. Map API → UI at the client
boundary so the UI is not coupled to DB columns or Go internal structs.

## Regenerating

```bash
python3 apps/api/scripts/gen-openapi.py
```

This refreshes `docs/openapi.json`, the embed copy under `internal/openapi/`, and
`apps/frontend/lib/api/contract.ts` (preserving curated `components.schemas`).
