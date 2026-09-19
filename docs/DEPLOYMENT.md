# DeployCore Deployment & Infrastructure Reality

This document describes the actual state of containerization, continuous integration, and production deployment configuration in the repository.

---

## 1. Containerization State

### Application Dockerfiles
- **`apps/api`**: **No `Dockerfile` exists**. The Go API is compiled and run directly on the host using `go run ./cmd/api` or `go build ./cmd/api`.
- **`apps/frontend`**: **No `Dockerfile` exists**. The Next.js application is built and executed using `pnpm run build` and `next start`.
- **Root**: **No root `Dockerfile` exists**.

### Compose Configuration
- **`docker-compose.yml`**: **No compose file exists** in the repository root or sub-applications.
- **Local Overrides**: `.gitignore` explicitly ignores `docker-compose.override.yml` and `compose.override.yml`, indicating local developer overrides are anticipated but no baseline compose configuration is checked into source control.

---

## 2. CI/CD Pipelines

- **GitHub Actions**: **No `.github/` directory exists** in the repository.
- **Alternative CI Providers**: No `.gitlab-ci.yml`, `Jenkinsfile`, or cloud build configs are present.
- **Automated Validation**: Automated testing, linting, typechecking, and security scans are not currently executed in a remote CI pipeline. All verifications must be run manually prior to committing.

---

## 3. Runtime & Execution Architecture

DeployCore manages external infrastructure through an agent-driven model:

```text
Control Plane (apps/api)
       │
       │ HTTP / WebSockets / Agent Commands
       ▼
Server Agent (apps/agent — planned)
       │
       │ Docker Engine API
       ▼
Host Docker Containers & Traefik Routing
```

### Current Development Runtime
Because `apps/agent` is not yet implemented, the control plane includes an agent simulator:
- **`ORCHESTRATOR_SIMULATE_AGENT=true`**:
  - When enabled (default in non-production environments), the deployment orchestrator executes deployment state machine transitions locally without issuing commands to a physical Docker host.
  - Allows full end-to-end testing of deployment workflows, rollbacks, database resource records, and volume management.
- **Production Guard**:
  - If `APP_ENV=production`, `ORCHESTRATOR_SIMULATE_AGENT` automatically defaults to `false`. Without an active server agent reporting for the target server, deployments and provisioning commands will queue waiting for agent execution.

---

## 4. Production Readiness Gaps

To deploy DeployCore into staging or production, the following infrastructure assets must be created:
1. **API Dockerfile**: Multi-stage Go build resulting in a minimal scratch/distroless or alpine container running the compiled binary.
2. **Frontend Dockerfile**: Multi-stage Next.js standalone build (`output: 'standalone'`).
3. **Local Developer Compose Stack**: A `docker-compose.yml` defining PostgreSQL 16 with a healthcheck to bootstrap local developer onboarding.
4. **CI Workflow**: A GitHub Actions workflow running:
   - Go unit tests and `go vet`.
   - Frontend `pnpm run typecheck` and `pnpm run lint`.
