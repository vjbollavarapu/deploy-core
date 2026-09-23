# DeployCore Deployment & Infrastructure Notes

> **Authoritative production operator guide:**  
> **[production-runbook.md](./production-runbook.md)**  
> **OCI validation checklist:**  
> **[oci-live-validation-checklist.md](./oci-live-validation-checklist.md)**

This file is a short index of what exists in the repository for packaging and local bootstrap. Prefer the production runbook for end-to-end procedures.

---

## What exists today

| Asset | Location | Notes |
|---|---|---|
| Agent installer | `deployments/install/install-agent.sh` | Ubuntu 22.04/24.04; checksum; Docker optional install; Traefik + `deploycore-proxy` |
| systemd unit | `deployments/install/deploycore-agent.service` | `User=deploycore`, env files, `Requires=docker.service` |
| Installer static checks | `deployments/install/validate-install-static.sh` | Documentation/CI helper |
| Local Postgres compose | `docker-compose.yml` | **Postgres only** (`postgres:15-alpine`) for lab/bootstrap — not a full CP stack |
| API | `apps/api` | No Dockerfile; run `go build` / `go run ./cmd/api`; embedded migrations on startup |
| Frontend | `apps/frontend` | No Dockerfile; `pnpm run build` / `pnpm start`; demo/mock opt-in only |
| Agent | `apps/agent` | Execution plane; systemd on managed servers |

---

## What does **not** exist (v1)

- Packaged Control Plane production installer / Helm / full compose stack for API+frontend  
- Public GA release artifact pipeline for Agent binaries + SHA256SUMS (**R20** — use `--binary` for OCI)  
- Automatic Agent update E2E (**R18** deferred)  
- Uninstall tooling (**R19** deferred)  
- CI workflows under `.github/` (not present)

---

## Production simulation / mock (must stay off)

| Setting | Production value |
|---|---|
| `ORCHESTRATOR_SIMULATE_AGENT` | `false` (API default is already `false`) |
| `NEXT_PUBLIC_DEMO_MODE` | unset / not `true` |
| `NEXT_PUBLIC_ENABLE_MOCK_FALLBACK` | unset / not `true` |

---

## Related

- Architecture overview: [ARCHITECTURE.md](./ARCHITECTURE.md)  
- Capacity / placement: [apps/api/docs/CAPACITY.md](../apps/api/docs/CAPACITY.md)  
- API README: [apps/api/README.md](../apps/api/README.md)
