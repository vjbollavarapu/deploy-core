# AGENTS.md — DeployCore Engineering & Operating Guidelines

> **Scope:** All AI coding assistants working within the `deploy-core` monorepo.  
> **Status:** MANDATORY — read before proposing or making any changes.

---

## 1. Monorepo Structure

```text
deploy-core/
├── apps/
│   ├── api/        # Go control-plane API & background worker (Go 1.24.2)
│   └── frontend/   # Next.js App Router frontend (Next.js 16.3.3, React 19, Tailwind v4)
├── docs/           # Monorepo persistent architecture and development documentation
├── PROMPTS.md      # Frontend prompt contracts and UI migration specifications
├── PROMPT_1.md     # Server agent architectural specification (future apps/agent)
└── AGENTS.md       # This file
```

### Architectural Roles:
1. **`apps/api` (Control Plane)**:
   - Owns authentication, tenancy, platform metadata, deployment state orchestration, and scheduling.
   - **Does NOT execute Docker commands directly on host servers**. Execution commands are structured and queued for server agents.
2. **`apps/frontend` (Presentation Plane)**:
   - Next.js App Router client interface.
   - Strictly decoupled from backend database structures. Uses generated wire contracts (`lib/api/contract.ts`) at API boundaries.
3. **`apps/agent` (Execution Plane — Planned)**:
   - Communicates with `apps/api` via authenticated agent protocol and controls host Docker engines.
   - Note: In development and test environments, `ORCHESTRATOR_SIMULATE_AGENT=true` simulates agent operations.

---

## 2. Non-Negotiable Operating Rules

1. **Review-Before-Change Discipline**:
   - Always view and read target files before modifying them. Never make edits based on assumptions.
   - Inspect existing patterns in the codebase before introducing new utilities or abstractions.
2. **Strict App Boundary Isolation**:
   - Never import from `apps/api` into `apps/frontend` or vice versa.
   - Root-level files (`docs/`, `AGENTS.md`, scripts) are repository-wide and must remain app-agnostic.
3. **No Mock Data in Production Paths**:
   - `apps/frontend/lib/mock-data.ts` is a legacy prototype fixture. Do not add more mock data to it.
   - When connecting real API routes, render proper loading states, error envelopes, and empty states.
4. **No Secrets or Credentials in Source**:
   - Never hardcode API keys, JWT secrets, passwords, or tenant IDs.
   - Secrets in the database must use AES-256-GCM envelope encryption (`pkg/crypto/aead.go`). Plaintext secrets must never appear in logs or audit records.
5. **No Shell Execution Payloads**:
   - Agent commands (`agent_commands`) and container configurations must use structured operational schemas (`schemaVersion: 1`), never raw shell strings or unvalidated command injection vectors.

---

## 3. Backend Conventions (`apps/api`)

- **Standard Library HTTP**: Use Go standard library `net/http.ServeMux` (Go 1.22+ method-and-path patterns). Do not add third-party HTTP routers (Gin, Echo, Chi).
- **Layered Architecture**:
  - `handler.go`: Request parsing, query/path extraction, error envelope writing (`pkg/apierror`).
  - `service.go`: Business logic, RBAC authorization (`rbac.Authorizer`), audit writes (`audit.Writer`).
  - `repository.go`: Parameterized SQL queries via `*pgxpool.Pool` (no ORMs).
  - `domain.go`: Pure domain models, validation logic, request/response DTOs.
- **Tenancy**: Every tenant-facing database query must be explicitly scoped by `organization_id`.
- **Database Migrations**:
  - Embedded migrations live in `internal/platform/db/migrations/` (`*.up.sql` and `*.down.sql`).
  - An operator mirror is kept in `migrations/`.
  - Use `TIMESTAMP WITH TIME ZONE` (UTC) and UUID primary keys (`gen_random_uuid()`).
- **Error Handling**: Return structured errors using `pkg/apierror.Error`. Never expose raw database errors or stack traces to HTTP responses.
- **Logging**: Use structured JSON logging with standard `log/slog`. Always pass `request_id`.

---

## 4. Frontend Conventions (`apps/frontend`)

- **Component Hierarchy**:
  - `components/ui/`: Generic design system primitives (shadcn/ui, Radix, Base-UI).
  - `components/platform/`: Reusable application shell, navigation, standard data tables, and state indicators.
  - `components/deploycore/`: Domain-specific business components (workloads, servers, databases, etc.).
  - *Rule:* Deprecated components in `components/deploycore/` that duplicate `components/platform/` must be consolidated into `@/components/platform`.
- **Forms & Validation**:
  - Always validate forms using `react-hook-form` and `zod` schemas placed in `lib/validations/`.
  - Provide inline validation messages and human-readable field errors.
- **Styling**:
  - Use Tailwind CSS v4 design tokens.
  - Support dark mode by default (`next-themes`).
  - Destructive actions (deletions, terminations, rollbacks) must always require user confirmation dialogs.
- **TypeScript**:
  - `strict: true`. Avoid `any`. If unavoidable, document why with a clear comment.
  - Keep wire API contracts (`lib/api/contract.ts`) separate from UI view-models (`lib/types.ts`).

---

## 5. Development & Verification Commands

### Tooling Paths:
- The bundled Go 1.24.2 compiler is located at: `apps/api/.tools/go/bin/go`.
- Package manager for frontend: `pnpm` (v9+).
- Node.js runtime: Node v20+.

### Backend Commands (`apps/api`):
```bash
# Add bundled Go to PATH for the current terminal session:
export PATH="$(pwd)/apps/api/.tools/go/bin:$PATH"

# Run unit tests (does not require PostgreSQL):
cd apps/api && go test -short ./...

# Run all tests with dedicated PostgreSQL test database:
export TEST_DATABASE_URL='postgres://deploycore:deploycore@localhost:5432/deploycore_test?sslmode=disable'
cd apps/api && go test ./...

# Run Go static analysis:
cd apps/api && go vet ./...

# Start API server locally:
export DATABASE_URL='postgres://deploycore:deploycore@localhost:5432/deploycore?sslmode=disable'
export HTTP_ADDR=':8080'
cd apps/api && go run ./cmd/api
```

### Frontend Commands (`apps/frontend`):
```bash
# Typecheck TypeScript (zero errors required):
cd apps/frontend && pnpm run typecheck

# Run ESLint:
cd apps/frontend && pnpm run lint

# Start Next.js development server:
cd apps/frontend && pnpm run dev
```

### Contract Synchronization:
```bash
# Regenerate OpenAPI specification and frontend TypeScript wire contracts:
python3 apps/api/scripts/gen-openapi.py
```

---

## 6. Pre-Commit Checklist for Agents

Before completing any task:
1. Did you view the relevant files before modifying them?
2. Does `pnpm run typecheck` pass in `apps/frontend`?
3. Does `pnpm run lint` pass in `apps/frontend`?
4. Do unit tests pass in `apps/api` (`go test -short ./...`)?
5. Did you avoid committing any `.env` files, credentials, or secrets?
6. Is your change scoped and commit-ready without half-finished implementations?
