# Migrations

SQL applied at runtime is **embedded** from:

`internal/platform/db/migrations/*.up.sql`

The migrator records versions in `schema_migrations` and applies pending files
in lexical order, one transaction per file.

## Versions

| Version | Description |
| --- | --- |
| `000001_bootstrap` | `pgcrypto` + bootstrap marker |
| `000002_core_schema` | Core control-plane tables (Phase B2) |
| `000003_auth_password_reset` | Password reset tokens (Phase B3) |
| `000004_rbac_seed` | Invitations + RBAC catalog seed (Phase B4) |
| `000005_project_env_uniques` | Soft-delete-safe project/env slug uniques (Phase B5) |
| `000006_server_name_unique` | Soft-delete-safe server names (Phase B6) |
| `000007_agent_indexes` | Agent registration/credential indexes (Phase B7) |
| `000008_agent_commands` | Agent command queue (Phase B8) |

## Operator mirror

Files in this directory are documentation mirrors for review. Prefer editing the
embedded path under `internal/platform/db/migrations/` and syncing here.

Schema intent: [`../docs/SCHEMA.md`](../docs/SCHEMA.md).

## Local apply (manual)

```bash
export DATABASE_URL='postgres://…/deploycore?sslmode=disable'
psql "$DATABASE_URL" -f ../internal/platform/db/migrations/000001_bootstrap.up.sql
psql "$DATABASE_URL" -f ../internal/platform/db/migrations/000002_core_schema.up.sql
```

Normal path: start `cmd/api`, which runs `Migrator.Up`.
