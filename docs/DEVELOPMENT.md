# DeployCore Development Guide

This guide explains how to set up, build, test, and run the DeployCore applications locally.

---

## 1. Prerequisites & Tooling

To work on DeployCore, ensure the following tools are installed:

| Tool | Recommended Version | Purpose |
| :--- | :--- | :--- |
| **Node.js** | `v20.x` or higher (`v20.19.4` active) | Frontend runtime |
| **pnpm** | `v9.15.x` or higher | Frontend package manager |
| **Go** | `v1.24.2` | Backend compiler & toolchain |
| **PostgreSQL** | `v16.x` | Control-plane database |
| **Python** | `3.10+` | OpenAPI and contract generation |

> **Note on Go Toolchain:**  
> A pre-compiled Go 1.24.2 distribution for macOS (`darwin/arm64`) is present in `apps/api/.tools/go/bin/go`.  
> To use it in your terminal, add it to your path:
> ```bash
> export PATH="$(pwd)/apps/api/.tools/go/bin:$PATH"
> ```

---

## 2. Environment Configuration

### Backend Configuration (`apps/api`)
The backend loads configuration from environment variables (and optionally an `.env` file via `godotenv`).

A sample file is available at `apps/api/.env.example`:
```bash
cp apps/api/.env.example apps/api/.env
```

#### Required Environment Variables:
- `DATABASE_URL`: PostgreSQL connection string (e.g. `postgres://deploycore:deploycore@localhost:5432/deploycore?sslmode=disable`).
- `AUTH_TOKEN_SECRET`: Minimum 32-character secret key for signing JWT access tokens (falls back to dev secret if `APP_ENV=development`).
- `SECRETS_PLATFORM_KEY`: 32 raw bytes or standard base64 string for AES-256-GCM secret encryption (falls back to dev key if `APP_ENV=development`).

#### Key Optional Tuning Variables:
- `APP_ENV`: `development` | `test` | `production` (default: `development`).
- `HTTP_ADDR`: Listener address (default: `:8080`).
- `LOG_LEVEL`: `debug` | `info` | `warn` | `error` (default: `info`).
- `CORS_ALLOWED_ORIGINS`: Comma-separated list of allowed origins (default: `http://localhost:3000`).
- `JOB_WORKER_ENABLED`: Enables background queue consumer (default: `true`).
- `ORCHESTRATOR_SIMULATE_AGENT`: Simulates agent command execution without requiring a live server agent (default: `true` in non-production).
- `RECONCILE_ENABLED`: Enables container desired-state reconciliation loop (default: `true`).
- `RECONCILE_INTERVAL`: Interval between reconciliation ticks (default: `30s`).

### Frontend Configuration (`apps/frontend`)
Currently, `apps/frontend` has no required external environment variables because data is loaded from mock fixtures. When connecting to the live API, a variable such as `NEXT_PUBLIC_API_URL` will be introduced.

---

## 3. Running Locally

### Starting PostgreSQL
Start a local PostgreSQL instance (ensure database `deploycore` exists):
```bash
createdb deploycore
```

### Starting the API Server
1. Navigate to the API directory:
   ```bash
   cd apps/api
   ```
2. Start the server (database migrations run automatically on startup):
   ```bash
   export DATABASE_URL='postgres://deploycore:deploycore@localhost:5432/deploycore?sslmode=disable'
   export HTTP_ADDR=':8080'
   go run ./cmd/api
   ```
3. Verify the server is responsive:
   ```bash
   curl http://localhost:8080/health
   curl http://localhost:8080/ready
   ```

### Starting the Next.js Frontend
1. Navigate to the frontend directory:
   ```bash
   cd apps/frontend
   ```
2. Install dependencies (if not already installed):
   ```bash
   pnpm install
   ```
3. Start the Next.js development server:
   ```bash
   pnpm run dev
   ```
4. Access the web interface at `http://localhost:3000`.

---

## 4. Testing & Verification Workflows

### Backend Tests (`apps/api`)
The Go test suite includes unit tests and PostgreSQL integration tests:

```bash
cd apps/api

# 1. Run unit tests only (fast, skips tests requiring PostgreSQL):
go test -short ./...

# 2. Run specific package tests:
go test -v ./internal/deployments/...
go test -v ./internal/auth/...
go test -v ./pkg/crypto/...

# 3. Run full integration suite with a dedicated test database:
export TEST_DATABASE_URL='postgres://deploycore:deploycore@localhost:5432/deploycore_test?sslmode=disable'
go test -v ./...

# 4. Run static analysis:
go vet ./...
```

### Frontend Checks (`apps/frontend`)
Ensure strict TypeScript compliance and lint rules pass:

```bash
cd apps/frontend

# 1. Typecheck:
pnpm run typecheck

# 2. ESLint:
pnpm run lint

# 3. Production Build:
pnpm run build
```

---

## 5. Synchronizing OpenAPI & Frontend Wire Contracts

When modifying Go HTTP handlers in `apps/api/internal/<domain>/handler.go`, update the OpenAPI schema and frontend contract types:

```bash
# From repository root:
python3 apps/api/scripts/gen-openapi.py
```

This script:
1. Inspects all Go handlers for `mux.Handle(...)` route registrations.
2. Regenerates `apps/api/docs/openapi.json`.
3. Updates `apps/frontend/lib/api/contract.ts` with updated TypeScript types.
