package reconcile_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agents"
	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/reconcile"
	"github.com/deploycore/deploy-core/apps/api/internal/replicas"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b30_test?sslmode=disable"
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
	if err := rbac.EnsureSeeded(ctx, pool); err != nil {
		t.Fatalf("rbac seed: %v", err)
	}
	_, _ = pool.Exec(ctx, `
		DELETE FROM application_replicas;
		DELETE FROM jobs;
		DELETE FROM application_configs;
		DELETE FROM applications;
		DELETE FROM environments;
		DELETE FROM projects;
		DELETE FROM server_agents;
		DELETE FROM server_heartbeats;
		DELETE FROM servers;
		DELETE FROM organization_invitation_roles;
		DELETE FROM organization_invitations;
		DELETE FROM member_roles;
		DELETE FROM organization_members;
		TRUNCATE audit_logs;
		DELETE FROM password_reset_tokens;
		DELETE FROM sessions;
		DELETE FROM organizations;
		DELETE FROM users;`)
	return pool
}

func TestSweepMarksExpiredHeartbeatOffline(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	orgID := uuid.New()
	userID := uuid.New()
	slug := "b30-" + orgID.String()[:8]
	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, display_name, status)
		VALUES ($1, $2, 'x', 'Owner', 'active')`, userID, slug+"@example.com")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO organizations (id, name, slug, created_by)
		VALUES ($1, 'B30', $2, $3)`, orgID, slug, userID)
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	serverID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO servers (
			id, organization_id, name, provider, region, hostname, architecture,
			operating_system, status, maintenance_mode, last_heartbeat_at, created_by
		) VALUES (
			$1, $2, 'edge', 'hetzner', 'fsn1', 'edge.local', 'amd64',
			'linux', 'ONLINE', FALSE, NOW() - INTERVAL '5 minutes', $3
		)`, serverID, orgID, userID)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	agentRepo := agents.NewPostgresRepository(pool)
	agentSvc := agents.NewService(agentRepo, nil, nil, log, agents.ServiceConfig{})
	replicaRepo := replicas.NewPostgresRepository(pool)
	replicaRec := replicas.NewReconciler(pool, replicaRepo, log, true)
	loop := reconcile.NewLoop(pool, agentSvc, replicaRepo, replicaRec, log, reconcile.Config{
		HeartbeatTTL: 90 * time.Second,
	})

	n, err := loop.SweepServers(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 offline, got %d", n)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM servers WHERE id = $1`, serverID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "OFFLINE" {
		t.Fatalf("status=%s", status)
	}

	n, err = loop.SweepServers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("second sweep got %d", n)
	}
}

func TestDesiredStateJobTypeAccepted(t *testing.T) {
	pool := testPool(t)
	q := jobs.NewQueue(jobs.NewPostgresRepository(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	key := reconcile.BucketKey(time.Now(), 30*time.Second) + "-" + uuid.NewString()[:8]
	job, err := q.Enqueue(context.Background(), jobs.EnqueueInput{
		Type:           jobs.TypeDesiredStateReconcile,
		IdempotencyKey: &key,
		MaxAttempts:    3,
		Payload:        map[string]any{"tick": key},
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if job.Type != jobs.TypeDesiredStateReconcile {
		t.Fatalf("type=%s", job.Type)
	}
}
