# DeployCore API Specification & Integration Guide

The DeployCore Control Plane exposes a RESTful JSON API generated from OpenAPI 3.1.

---

## 1. General Conventions

- **Base Path**: All control plane endpoints are mounted under `/api/v1/`.
- **Content Type**: `application/json; charset=utf-8` for all requests and responses.
- **Request Tracing**: Every response includes an `X-Request-ID` header. If a client sends `X-Request-ID`, it is preserved and bound to all logging contexts.
- **Health Checks**:
  - `GET /health`: Liveness probe (returns HTTP 200 `{ "status": "ok" }`).
  - `GET /ready`: Readiness probe (validates PostgreSQL database pool connectivity).
- **Documentation**:
  - `GET /openapi.json`: OpenAPI 3.1 schema definition.

---

## 2. Standard Envelopes

### Success Envelopes
Single-entity endpoints return the target object directly:
```json
{
  "id": "c7a8b412-8e12-4cf3-90d2-7b561c28f092",
  "name": "production-api",
  "slug": "prod-api",
  "status": "RUNNING",
  "createdAt": "2026-09-19T08:00:00Z",
  "updatedAt": "2026-09-19T08:15:00Z"
}
```

Paginated collections use standard pagination envelopes (`pkg/pagination`):
```json
{
  "items": [ ... ],
  "nextCursor": "eyJjcmVhdGVkX2F0IjoiMjAyNi0wOS0xOVQ...}",
  "hasMore": true
}
```

### Error Envelopes (`pkg/apierror`)
All HTTP errors return a standardized JSON payload:
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "application name is required",
    "requestId": "a0e1c28f-7b56-4cf3-90d2-8e12c7a8b412",
    "details": {
      "field": "name",
      "reason": "must not be empty"
    }
  }
}
```

#### Standard Error Codes:
- `VALIDATION_ERROR` (400)
- `UNAUTHORIZED` (401)
- `FORBIDDEN` (403)
- `RESOURCE_NOT_FOUND` (404)
- `CONFLICT` (409)
- `RATE_LIMITED` (429)
- `INSUFFICIENT_RESOURCES` (422)
- `INTERNAL_ERROR` (500)
- `SERVICE_UNAVAILABLE` (503)

---

## 3. Authentication & Authorization Model

### User Authentication
Endpoints requiring user context expect an HTTP `Authorization` header with a Bearer JWT:
```http
Authorization: Bearer <access_token>
```
- **Access Tokens**: Short-lived (default: 15 minutes) HMAC-SHA256 JWTs containing `sub` (User ID) and `sid` (Session ID). They do not embed static permissions.
- **Refresh Flow**: `POST /api/v1/auth/refresh` accepts a refresh token and returns a new access/refresh pair, rotating the database session record.
- **Session Revocation**: `POST /api/v1/auth/logout` marks the active session revoked in PostgreSQL.

### Agent Authentication
Agent-facing endpoints (`/api/v1/agents/*`, `/api/v1/agents/commands/*`) use agent bearer tokens:
```http
Authorization: Bearer <agent_secret_token>
```
The token is verified against the hashed credential stored in `server_agents`.

### Authorization & Tenancy
1. All resources are strictly scoped by `organization_id`.
2. The user's active membership in the organization is loaded on each request.
3. Permissions are evaluated server-side via `rbac.Authorizer` against the assigned role catalog (`Owner`, `Administrator`, `DevOps`, `Developer`, `Support`, `Viewer`).

---

## 4. Route Organization (102 Endpoints)

| Tag | Route Prefix | Primary Endpoints |
| :--- | :--- | :--- |
| **Auth** | `/api/v1/auth` | `/register`, `/login`, `/refresh`, `/logout`, `/me`, `/forgot-password`, `/reset-password` |
| **Organizations** | `/api/v1/organizations` | `GET/POST /organizations`, `GET/PATCH/DELETE /organizations/{id}`, `GET/POST /organizations/{id}/members`, `POST /invitations/accept` |
| **Projects** | `/api/v1/projects` | `GET/POST /projects`, `GET/PATCH/DELETE /projects/{id}`, `GET/POST /projects/{id}/environments`, `GET/PATCH/DELETE /environments/{id}` |
| **Servers** | `/api/v1/servers` | `GET/POST /servers`, `GET/PATCH/DELETE /servers/{id}`, `POST/DELETE /servers/{id}/maintenance`, `GET /servers/capacity`, `POST /placement/preview` |
| **Agents** | `/api/v1/agents` | `POST /servers/{id}/registration-token`, `POST /agents/register`, `POST /agents/heartbeat`, `GET /agents/commands`, `POST /agents/commands/{id}/status`, `POST /agents/logs`, `POST /agents/metrics` |
| **Applications** | `/api/v1/applications` | `GET/POST /applications`, `GET/PATCH/DELETE /applications/{id}`, `GET/POST /applications/{id}/deployments`, `GET /applications/{id}/revisions`, `POST /applications/{id}/rollback` |
| **Deployments** | `/api/v1/deployments` | `GET /deployments`, `GET /deployments/{id}`, `POST /deployments/{id}/cancel`, `GET /deployments/{id}/logs`, `GET /deployments/{id}/events/stream` |
| **Revisions** | `/api/v1/revisions` | `GET /revisions/{id}` |
| **Replicas** | `/api/v1/applications` | `GET /applications/{id}/replicas`, `POST /applications/{id}/replicas/scale`, `POST /applications/{id}/replicas/{index}/unhealthy` |
| **Variables & Secrets** | `/api/v1/variables`, `/secrets` | `GET/POST /variables`, `GET/PATCH/DELETE /variables/{id}`, `GET /variables/resolved`, `GET/POST /secrets`, `GET/PATCH/DELETE /secrets/{id}` |
| **Domains & TLS** | `/api/v1/domains` | `GET/POST /applications/{id}/domains`, `PATCH/DELETE /domains/{id}` |
| **Databases** | `/api/v1/databases` | `GET/POST /databases`, `GET/PATCH/DELETE /databases/{id}`, `POST /databases/{id}/credentials/reveal`, `POST /databases/{id}/backups` |
| **Volumes** | `/api/v1/volumes` | `GET/POST /volumes`, `GET/PATCH/DELETE /volumes/{id}`, `POST /volumes/{id}/attach`, `POST /volumes/{id}/detach`, `GET /volumes/{id}/inspect` |
| **Backups** | `/api/v1/backups` | `GET/DELETE /backups/{id}`, `POST /backups/{id}/restore` |
| **Git Integrations** | `/api/v1/integrations/git` | `GET/POST /integrations/git/connections`, `POST .../sync`, `GET .../repositories`, `POST /webhooks/git/{connectionId}` |
| **Registries** | `/api/v1/integrations/registries` | `GET/POST /integrations/registries`, `GET/PATCH/DELETE /integrations/registries/{id}` |
| **Webhooks** | `/api/v1/integrations/webhooks` | `GET/POST /integrations/webhooks`, `GET/PATCH/DELETE /integrations/webhooks/{id}`, `GET /integrations/webhooks/{id}/deliveries`, `POST /integrations/webhooks/emit` |
| **Notifications** | `/api/v1/integrations/notifications` | `GET/POST .../channels`, `GET/POST .../policies`, `GET .../deliveries`, `POST .../emit` |
| **Audit Logs** | `/api/v1/audit-logs` | `GET /audit-logs`, `GET /audit-logs/{id}` |

---

## 5. Frontend Integration Architecture

### Generated Wire Contracts
The TypeScript file `apps/frontend/lib/api/contract.ts` defines wire models matching the Go API responses:
```ts
import { API_BASE } from '@/lib/api'
import type { Application, Deployment, Server } from '@/lib/api'
```

### Current Status
- The Next.js frontend has not yet integrated an HTTP client (no `fetch` calls).
- All pages currently render data from `apps/frontend/lib/mock-data.ts`.

### Integration Path
1. **API Client**: Introduce a lightweight HTTP fetch utility (`lib/api/client.ts`) that appends the `Authorization: Bearer` header, parses `pkg/apierror` JSON responses, and refreshes tokens on `401 Unauthorized`.
2. **Authentication Flow**: Implement login and session state management to capture access tokens.
3. **Progressive Adoption**: Migrate UI components from `apps/frontend/lib/mock-data.ts` to API client calls.
