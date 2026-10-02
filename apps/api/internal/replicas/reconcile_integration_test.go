package replicas_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/replicas"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReconcileWithoutActiveRevisionDoesNotMaterialize(t *testing.T) {
	pool := replicaPool(t)
	ctx := context.Background()
	appID, serverID, orgID := seedReplicaApp(t, ctx, pool, "redis")
	slotID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO application_replicas (
			id, organization_id, application_id, replica_index, container_name, status
		) VALUES ($1, $2, $3, 0, 'redis-r0-0', 'STARTING')`, slotID, orgID, appID); err != nil {
		t.Fatal(err)
	}
	var before time.Time
	if err := pool.QueryRow(ctx, `SELECT updated_at FROM application_replicas WHERE id = $1`, slotID).Scan(&before); err != nil {
		t.Fatal(err)
	}

	rec := replicas.NewReconciler(pool, replicas.NewPostgresRepository(pool), slog.New(slog.NewTextHandler(io.Discard, nil)), false)
	if err := rec.Reconcile(ctx, appID, -1); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM application_replicas WHERE application_id = $1`, appID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("replica rows=%d, want the original placeholder only", count)
	}
	var name, status string
	var revisionID *uuid.UUID
	var updated time.Time
	if err := pool.QueryRow(ctx, `
		SELECT container_name, status, revision_id, updated_at
		FROM application_replicas WHERE id = $1`, slotID).Scan(&name, &status, &revisionID, &updated); err != nil {
		t.Fatal(err)
	}
	if name != "redis-r0-0" || status != replicas.StatusStarting || revisionID != nil || !updated.Equal(before) {
		t.Fatalf("placeholder mutated: name=%s status=%s revision=%v updated=%s before=%s", name, status, revisionID, updated, before)
	}
	var commands int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM agent_commands
		WHERE server_id = $1
		  AND operation IN ($2, $3)`, serverID, protocol.OpDeployRevision, protocol.OpStartContainer).Scan(&commands); err != nil {
		t.Fatal(err)
	}
	if commands != 0 {
		t.Fatalf("lifecycle commands=%d, want 0", commands)
	}
	var named int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM agent_commands
		WHERE server_id = $1 AND payload->>'containerName' LIKE '%-r0-%'`, serverID).Scan(&named); err != nil {
		t.Fatal(err)
	}
	if named != 0 {
		t.Fatalf("r0 lifecycle commands=%d", named)
	}
}

func TestReconcileAfterActiveRevisionMaterializesSlot(t *testing.T) {
	pool := replicaPool(t)
	ctx := context.Background()
	appID, serverID, orgID := seedReplicaApp(t, ctx, pool, "redis")
	revID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO revisions (
			id, organization_id, application_id, revision_number, status, image_tag
		) VALUES ($1, $2, $3, 3, 'ACTIVE', 'redis:7-alpine')`, revID, orgID, appID); err != nil {
		t.Fatal(err)
	}

	rec := replicas.NewReconciler(pool, replicas.NewPostgresRepository(pool), slog.New(slog.NewTextHandler(io.Discard, nil)), false)
	if err := rec.Reconcile(ctx, appID, -1); err != nil {
		t.Fatal(err)
	}

	var name, status string
	var slotRevision uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT container_name, status, revision_id
		FROM application_replicas
		WHERE application_id = $1 AND replica_index = 0`, appID).Scan(&name, &status, &slotRevision); err != nil {
		t.Fatal(err)
	}
	if name != replicas.ContainerName("redis", 3, 0) || slotRevision != revID || status != replicas.StatusStarting {
		t.Fatalf("slot name=%s revision=%s status=%s", name, slotRevision, status)
	}
	assertLifecycleCommands(t, ctx, pool, serverID, name, 1)

	if err := rec.Reconcile(ctx, appID, -1); err != nil {
		t.Fatal(err)
	}
	assertLifecycleCommands(t, ctx, pool, serverID, name, 1)
	var rows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM application_replicas WHERE application_id = $1`, appID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("replica rows=%d", rows)
	}
}

func TestReconcilePropagatesActiveRevisionQueryError(t *testing.T) {
	pool := replicaPool(t)
	ctx := context.Background()
	appID, _, _ := seedReplicaApp(t, ctx, pool, "redis")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	rec := replicas.NewReconciler(pool, replicas.NewPostgresRepository(pool), slog.New(slog.NewTextHandler(io.Discard, nil)), false)
	if err := rec.Reconcile(cancelled, appID, -1); err == nil {
		t.Fatal("expected query error")
	}
}

func assertLifecycleCommands(t *testing.T, ctx context.Context, pool *pgxpool.Pool, serverID uuid.UUID, containerName string, wantEach int) {
	t.Helper()
	for _, op := range []string{protocol.OpDeployRevision, protocol.OpStartContainer} {
		var n int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM agent_commands
			WHERE server_id = $1 AND operation = $2 AND payload->>'containerName' = $3`,
			serverID, op, containerName).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != wantEach {
			t.Fatalf("%s commands for %s = %d, want %d", op, containerName, n, wantEach)
		}
	}
	var r0 int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM agent_commands
		WHERE server_id = $1 AND payload->>'containerName' LIKE '%-r0-%'`, serverID).Scan(&r0); err != nil {
		t.Fatal(err)
	}
	if r0 != 0 {
		t.Fatalf("r0 commands=%d", r0)
	}
}

func replicaPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b29_test?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, url)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.NewMigrator(pool).Up(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func seedReplicaApp(t *testing.T, ctx context.Context, pool *pgxpool.Pool, slug string) (appID, serverID, orgID uuid.UUID) {
	t.Helper()
	userID, orgID, envID, serverID, appID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	projectID := uuid.New()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, display_name) VALUES ($1, $2, 'x', 'Replicas')`, userID, userID.String()+"@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'O', $2)`, orgID, "o-"+orgID.String()[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, organization_id, name, slug) VALUES ($1, $2, 'P', $3)`, projectID, orgID, "p-"+projectID.String()[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO environments (id, organization_id, project_id, name, slug) VALUES ($1, $2, $3, 'E', $4)`, envID, orgID, projectID, "e-"+envID.String()[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO servers (id, organization_id, name, provider, region, hostname, architecture, status) VALUES ($1, $2, 's1', 'hetzner', 'fsn1', 's1.local', 'amd64', 'ONLINE')`, serverID, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, organization_id, project_id, environment_id, name, slug, type, target_server_id, status) VALUES ($1, $2, $3, $4, $5, $5, 'API', $6, 'ready')`, appID, orgID, projectID, envID, slug, serverID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO application_configs (
			organization_id, application_id, version, source_type, image_reference, internal_port, runtime_config
		) VALUES ($1, $2, 1, 'image', 'redis:7-alpine', 6379, '{"desiredReplicas":1}')`, orgID, appID); err != nil {
		t.Fatal(err)
	}
	return appID, serverID, orgID
}
