# Security hardening (B31)

Control-plane security posture for `apps/api`.

## Authentication

- Passwords: Argon2id; never logged.
- Access tokens: short-lived JWTs (`sub` + `sid` only).
- Refresh tokens: opaque, SHA-256 hashed in `sessions`.
- Refresh rotation is atomic (`RevokeSessionAtomic`). Concurrent refresh races
  and reuse of a rotated refresh token revoke the entire **session family**
  (`family_id`).

## Authorization / tenancy

- RBAC permissions are org-scoped via membership → roles → permissions.
- Repository queries include organization / resource scoping; cross-tenant
  access returns not-found or forbidden.

## Agents

- Registration tokens are one-time, TTL-bound, hashed at rest.
- Agent credentials are hashed; revoke via
  `DELETE /servers/{serverId}/agent-credential` (sets `status=revoked`).
- Agent commands use a structured op allowlist; payloads recursively forbid
  shell/exec keys. Runtime actions never accept raw shell text from the API.
- Volume `mountPath` must be an absolute container path without `..`.

## Secrets / encryption

- Platform secrets and git/registry credentials use AES-256-GCM with a
  platform key (`SECRETS_PLATFORM_KEY`).
- Webhook signing secrets are encrypted; outbound deliveries verify HMAC.

## Outbound HTTP (SSRF)

Webhook and notification channel URLs are validated with
`security.ValidateOutboundURL`: http(s) only; localhost, private, link-local,
CGNAT, and metadata addresses rejected (including DNS answers).

## Request surface

| Control | Default |
| --- | --- |
| Max body | `MAX_REQUEST_BODY_BYTES` (1 MiB) via `MaxBytesReader` |
| Auth rate limit | `AUTH_RATE_LIMIT_PER_MINUTE` |
| Agent register rate limit | `PUBLIC_RATE_LIMIT_PER_MINUTE` |
| Git webhook rate limit | `GIT_WEBHOOK_RATE_LIMIT_PER_MINUTE` |
| CORS | explicit origins; `*` forbidden when `APP_ENV=production` |
| Timeouts | `READ_HEADER_TIMEOUT`, `READ_TIMEOUT`, `WRITE_TIMEOUT`, `IDLE_TIMEOUT` |
| Headers | nosniff, DENY frame, no-store, no-referrer |

## Audit integrity

`audit_events` cannot be `DELETE`d (DB trigger). Soft immutability for
forensic trails; test cleanup uses `UPDATE`/`TRUNCATE` alternatives or
dedicated test helpers where needed.

## Logging

Request path / user-agent / remote addr pass through `SanitizeLogField`
(control chars stripped, length capped) to reduce log injection risk.

## Tests

Critical-path coverage lives in:

- `internal/security/*_test.go` — SSRF, mount paths, log sanitize
- `internal/agentcmd` — recursive forbid of execution payload keys
- `internal/auth` — refresh reuse / family revoke (integration)
- Agents / webhooks / notifications integration suites exercise revoke and
  URL validation on the HTTP surface
