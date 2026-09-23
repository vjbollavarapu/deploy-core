package db_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
)

func TestEmbeddedCoreMigrationsPresent(t *testing.T) {
	t.Parallel()

	entries, err := fs.ReadDir(db.MigrationFS(), "migrations")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}

	want := map[string]bool{
		"000001_bootstrap.up.sql":                 false,
		"000001_bootstrap.down.sql":               false,
		"000002_core_schema.up.sql":               false,
		"000002_core_schema.down.sql":             false,
		"000003_auth_password_reset.up.sql":       false,
		"000003_auth_password_reset.down.sql":     false,
		"000004_rbac_seed.up.sql":                 false,
		"000004_rbac_seed.down.sql":               false,
		"000005_project_env_uniques.up.sql":       false,
		"000005_project_env_uniques.down.sql":     false,
		"000006_server_name_unique.up.sql":        false,
		"000006_server_name_unique.down.sql":      false,
		"000007_agent_indexes.up.sql":             false,
		"000007_agent_indexes.down.sql":           false,
		"000008_agent_commands.up.sql":            false,
		"000008_agent_commands.down.sql":          false,
		"000009_application_slug_unique.up.sql":   false,
		"000009_application_slug_unique.down.sql": false,
		"000010_secrets_algorithm.up.sql":         false,
		"000010_secrets_algorithm.down.sql":       false,
		"000011_job_reclaim_index.up.sql":         false,
		"000011_job_reclaim_index.down.sql":       false,
		"000012_git_providers.up.sql":             false,
		"000012_git_providers.down.sql":           false,
		"000013_registries.up.sql":                false,
		"000013_registries.down.sql":              false,
		"000014_health_checks.up.sql":             false,
		"000014_health_checks.down.sql":           false,
		"000015_metrics.up.sql":                   false,
		"000015_metrics.down.sql":                 false,
		"000016_managed_databases.up.sql":         false,
		"000016_managed_databases.down.sql":       false,
		"000017_volumes.up.sql":                   false,
		"000017_volumes.down.sql":                 false,
		"000018_backups.up.sql":                   false,
		"000018_backups.down.sql":                 false,
		"000019_notifications.up.sql":             false,
		"000019_notifications.down.sql":           false,
		"000020_outgoing_webhooks.up.sql":         false,
		"000020_outgoing_webhooks.down.sql":       false,
		"000021_audit_completeness.up.sql":        false,
		"000021_audit_completeness.down.sql":      false,
		"000022_capacity_placement.up.sql":        false,
		"000022_capacity_placement.down.sql":      false,
		"000023_replicas.up.sql":                  false,
		"000023_replicas.down.sql":                false,
		"000024_desired_state_reconcile.up.sql":   false,
		"000024_desired_state_reconcile.down.sql": false,
	}
	for _, e := range entries {
		if _, ok := want[e.Name()]; ok {
			want[e.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("missing embedded migration %s", name)
		}
	}

	up, err := fs.ReadFile(db.MigrationFS(), "migrations/000002_core_schema.up.sql")
	if err != nil {
		t.Fatalf("read core schema: %v", err)
	}
	body := string(up)
	for _, table := range []string{
		"users", "sessions", "organizations", "organization_members",
		"teams", "team_members", "roles", "permissions", "role_permissions", "member_roles",
		"projects", "environments", "servers", "server_agents", "server_heartbeats",
		"applications", "application_configs", "deployments", "deployment_events", "revisions",
		"domains", "certificates", "environment_variables", "secrets",
		"git_connections", "registries", "audit_logs", "jobs",
	} {
		if !strings.Contains(body, "CREATE TABLE "+table) && !strings.Contains(body, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("core schema missing table %s", table)
		}
	}
}
