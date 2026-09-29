package orchestrator

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/replicas"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStatelessRetirementStopsExactPredecessorAfterActivation(t *testing.T) {
	pool, orgID, envID, serverID, appID, userID := seedRollingApp(t, nil)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	orch := New(pool, deployRepo, log, Config{SimulateAgent: true})

	first := runDeploy(t, ctx, deployRepo, orch, orgID, appID, envID, serverID, userID, "roll-1", deployments.TriggerManual, nil)
	if first.Status != deployments.StatusRunning {
		t.Fatalf("first status=%s", first.Status)
	}
	oldName := replicas.ContainerName("api", 1, 0)

	secondOrch := New(pool, deployRepo, log, Config{SimulateAgent: true})
	second := runDeploy(t, ctx, deployRepo, secondOrch, orgID, appID, envID, serverID, userID, "roll-2", deployments.TriggerManual, nil)
	if second.Status != deployments.StatusRunning {
		t.Fatalf("second status=%s", second.Status)
	}
	assertRetiredAfterActivation(t, commandsFor(secondOrch, second.ID), []string{oldName})
}

func TestStatelessHealthFailureDoesNotStopPredecessor(t *testing.T) {
	pool, orgID, envID, serverID, appID, userID := seedRollingApp(t, nil)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	_ = runDeploy(t, ctx, deployRepo, New(pool, deployRepo, log, Config{SimulateAgent: true}), orgID, appID, envID, serverID, userID, "health-1", deployments.TriggerManual, nil)

	oldName := replicas.ContainerName("api", 1, 0)
	failOrch := New(pool, deployRepo, log, Config{SimulateAgent: true, ForceHealthFail: true})
	failed := runDeploy(t, ctx, deployRepo, failOrch, orgID, appID, envID, serverID, userID, "health-2", deployments.TriggerManual, nil)
	if failed.Status != deployments.StatusHealthCheckFailed {
		t.Fatalf("status=%s", failed.Status)
	}
	assertPredecessorNotStopped(t, commandsFor(failOrch, failed.ID), oldName)
}

func TestStatelessActivationFailureDoesNotStopPredecessor(t *testing.T) {
	pool, orgID, envID, serverID, appID, userID := seedRollingApp(t, nil)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	_ = runDeploy(t, ctx, deployRepo, New(pool, deployRepo, log, Config{SimulateAgent: true}), orgID, appID, envID, serverID, userID, "route-1", deployments.TriggerManual, nil)

	oldName := replicas.ContainerName("api", 1, 0)
	failOrch := New(pool, deployRepo, log, Config{SimulateAgent: true, ForceRoutingFail: true})
	failed := runDeploy(t, ctx, deployRepo, failOrch, orgID, appID, envID, serverID, userID, "route-2", deployments.TriggerManual, nil)
	if failed.Status != deployments.StatusRoutingFailed {
		t.Fatalf("status=%s", failed.Status)
	}
	assertPredecessorNotStopped(t, commandsFor(failOrch, failed.ID), oldName)
}

func TestReadOnlyVolumeRetiresPredecessorAfterActivation(t *testing.T) {
	pool, orgID, envID, serverID, appID, userID := seedRollingApp(t, nil)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO volumes (
			organization_id, server_id, name, mount_path, state, attached_resource_type,
			attached_resource_id, docker_name, labels
		) VALUES ($1, $2, 'config', '/config', 'ATTACHED', 'application', $3, 'config', '{"readOnly":true}')`,
		orgID, serverID, appID); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	_ = runDeploy(t, ctx, deployRepo, New(pool, deployRepo, log, Config{SimulateAgent: true}), orgID, appID, envID, serverID, userID, "ro-1", deployments.TriggerManual, nil)
	oldName := replicas.ContainerName("api", 1, 0)
	secondOrch := New(pool, deployRepo, log, Config{SimulateAgent: true})
	second := runDeploy(t, ctx, deployRepo, secondOrch, orgID, appID, envID, serverID, userID, "ro-2", deployments.TriggerManual, nil)
	if second.Status != deployments.StatusRunning {
		t.Fatalf("status=%s", second.Status)
	}
	assertRetiredAfterActivation(t, commandsFor(secondOrch, second.ID), []string{oldName})
	for _, cmd := range commandsFor(secondOrch, second.ID) {
		if cmd.op == protocol.OpStopContainer && cmd.payload["reason"] == cutoverReason {
			t.Fatal("read-only deploy used writable stop-first")
		}
	}
}

func TestWritableDeployStillStopsBeforeCreate(t *testing.T) {
	pool, orgID, envID, serverID, appID, userID := seedRollingApp(t, nil)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO volumes (
			organization_id, server_id, name, mount_path, state, attached_resource_type,
			attached_resource_id, docker_name, labels
		) VALUES ($1, $2, 'data', '/data', 'ATTACHED', 'application', $3, 'data', '{}')`,
		orgID, serverID, appID); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	_ = runDeploy(t, ctx, deployRepo, New(pool, deployRepo, log, Config{SimulateAgent: true}), orgID, appID, envID, serverID, userID, "wr-1", deployments.TriggerManual, nil)
	oldName := replicas.ContainerName("api", 1, 0)
	secondOrch := New(pool, deployRepo, log, Config{SimulateAgent: true})
	second := runDeploy(t, ctx, deployRepo, secondOrch, orgID, appID, envID, serverID, userID, "wr-2", deployments.TriggerManual, nil)
	if second.Status != deployments.StatusRunning {
		t.Fatalf("status=%s", second.Status)
	}
	cmds := commandsFor(secondOrch, second.ID)
	stopAt, deployAt := -1, -1
	for i, cmd := range cmds {
		name, _ := cmd.payload["containerName"].(string)
		if cmd.op == protocol.OpStopContainer && name == oldName && cmd.payload["reason"] == cutoverReason && stopAt < 0 {
			stopAt = i
		}
		if cmd.op == protocol.OpDeployRevision && deployAt < 0 {
			deployAt = i
		}
		if cmd.payload["reason"] == "retire_previous" {
			t.Fatal("writable deploy issued retire_previous")
		}
	}
	if stopAt < 0 || deployAt < 0 || stopAt > deployAt {
		t.Fatalf("writable stop-before-create missing: stop=%d deploy=%d", stopAt, deployAt)
	}
}

func TestReplicaSlotsRetireOnlyTheirPredecessor(t *testing.T) {
	pool, orgID, envID, serverID, appID, userID := seedRollingApp(t, []byte(`{"desiredReplicas":2}`))
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	_ = runDeploy(t, ctx, deployRepo, New(pool, deployRepo, log, Config{SimulateAgent: true}), orgID, appID, envID, serverID, userID, "multi-1", deployments.TriggerManual, nil)
	old := []string{replicas.ContainerName("api", 1, 0), replicas.ContainerName("api", 1, 1)}
	secondOrch := New(pool, deployRepo, log, Config{SimulateAgent: true})
	second := runDeploy(t, ctx, deployRepo, secondOrch, orgID, appID, envID, serverID, userID, "multi-2", deployments.TriggerManual, nil)
	if second.Status != deployments.StatusRunning {
		t.Fatalf("status=%s", second.Status)
	}
	cmds := commandsFor(secondOrch, second.ID)
	assertRetiredAfterActivation(t, cmds, old)
	for _, cmd := range cmds {
		if cmd.op != protocol.OpStopContainer || cmd.payload["reason"] != "retire_previous" {
			continue
		}
		name, _ := cmd.payload["containerName"].(string)
		index, _ := cmd.payload["replicaIndex"].(int)
		if name != replicas.ContainerName("api", 1, index) {
			t.Fatalf("slot %d retired %s", index, name)
		}
	}
}

func TestRollbackRetiresTheReplacedContainerAfterActivation(t *testing.T) {
	pool, orgID, envID, serverID, appID, userID := seedRollingApp(t, nil)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	orch := New(pool, deployRepo, log, Config{SimulateAgent: true})
	first := runDeploy(t, ctx, deployRepo, orch, orgID, appID, envID, serverID, userID, "rb-1", deployments.TriggerManual, nil)
	second := runDeploy(t, ctx, deployRepo, orch, orgID, appID, envID, serverID, userID, "rb-2", deployments.TriggerManual, nil)
	var predecessor string
	if err := pool.QueryRow(ctx, `
		SELECT container_name FROM application_replicas
		WHERE application_id = $1 AND replica_index = 0`, appID).Scan(&predecessor); err != nil {
		t.Fatal(err)
	}
	if predecessor != replicas.ContainerName("api", 2, 0) {
		t.Fatalf("predecessor before rollback = %s", predecessor)
	}
	rbOrch := New(pool, deployRepo, log, Config{SimulateAgent: true})
	rb := runDeploy(t, ctx, deployRepo, rbOrch, orgID, appID, envID, serverID, userID, "rb-3", deployments.TriggerRollback, first.ActiveRevisionID)
	if rb.Status != deployments.StatusRunning || rb.ActiveRevisionID == nil || *rb.ActiveRevisionID != *first.ActiveRevisionID {
		t.Fatalf("rollback status=%s active=%v", rb.Status, rb.ActiveRevisionID)
	}
	if second.ActiveRevisionID == nil || *rb.ActiveRevisionID == *second.ActiveRevisionID {
		t.Fatal("rollback did not leave the replaced revision")
	}
	assertRetiredAfterActivation(t, commandsFor(rbOrch, rb.ID), []string{predecessor})
}

func seedRollingApp(t *testing.T, runtime []byte) (*pgxpool.Pool, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b13_test?sslmode=disable"
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
	userID, orgID, envID, serverID, appID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	projectID := uuid.New()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, display_name) VALUES ($1, $2, 'x', 'Orch')`, userID, userID.String()+"@example.com"); err != nil {
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
	if _, err := pool.Exec(ctx, `INSERT INTO servers (id, organization_id, name, provider, region, hostname, architecture, status) VALUES ($1, $2, 's1', 'hetzner', 'fsn1', 's1.local', 'amd64', 'OFFLINE')`, serverID, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, organization_id, project_id, environment_id, name, slug, type, target_server_id, status) VALUES ($1, $2, $3, $4, 'api', 'api', 'API', $5, 'ready')`, appID, orgID, projectID, envID, serverID); err != nil {
		t.Fatal(err)
	}
	if runtime == nil {
		runtime = []byte(`{}`)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO application_configs (
			organization_id, application_id, version, source_type, image_reference, internal_port, runtime_config
		) VALUES ($1, $2, 1, 'image', 'ghcr.io/example/api:1', 8080, $3)`, orgID, appID, runtime); err != nil {
		t.Fatal(err)
	}
	return pool, orgID, envID, serverID, appID, userID
}

func runDeploy(t *testing.T, ctx context.Context, repo deployments.Repository, orch *Orchestrator, orgID, appID, envID, serverID, userID uuid.UUID, key, trigger string, target *uuid.UUID) deployments.Deployment {
	t.Helper()
	d, err := repo.CreateQueued(ctx, orgID, appID, envID, &serverID, trigger, nil, nil, key, userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if target != nil {
		if err := repo.SetTargetRevision(ctx, d.ID, *target); err != nil {
			t.Fatal(err)
		}
	}
	if err := orch.Execute(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func commandsFor(orch *Orchestrator, deploymentID uuid.UUID) []simulatedCommand {
	var out []simulatedCommand
	for _, cmd := range orch.simulated {
		if cmd.deploymentID == deploymentID {
			out = append(out, cmd)
		}
	}
	return out
}

func assertRetiredAfterActivation(t *testing.T, cmds []simulatedCommand, oldNames []string) {
	t.Helper()
	createAt, startAt, healthAt, routingAt := -1, -1, -1, -1
	stops := map[string]int{}
	for i, cmd := range cmds {
		name, _ := cmd.payload["containerName"].(string)
		phase, _ := cmd.payload["phase"].(string)
		switch cmd.op {
		case protocol.OpDeployRevision:
			if (phase == "create_container" || phase == "rollback") && createAt < 0 {
				createAt = i
			}
			if phase == "enable_routing" {
				routingAt = i
			}
		case protocol.OpStartContainer:
			if startAt < 0 {
				startAt = i
			}
		case protocol.OpRunHealthCheck:
			healthAt = i
		case protocol.OpStopContainer:
			if cmd.payload["reason"] != "retire_previous" {
				continue
			}
			if name == "" {
				t.Fatal("retire_previous is missing containerName")
			}
			stops[name] = i
		}
	}
	if createAt < 0 || startAt < 0 || routingAt < 0 || !(createAt < startAt && startAt < routingAt) {
		t.Fatalf("rolling order create=%d start=%d health=%d routing=%d", createAt, startAt, healthAt, routingAt)
	}
	if healthAt >= 0 && !(startAt < healthAt && healthAt < routingAt) {
		t.Fatalf("health check out of order start=%d health=%d routing=%d", startAt, healthAt, routingAt)
	}
	if len(stops) != len(oldNames) {
		t.Fatalf("retired %#v, want %v", stops, oldNames)
	}
	for _, name := range oldNames {
		at, ok := stops[name]
		if !ok || at < routingAt {
			t.Fatalf("predecessor %s retired at %d, routing at %d", name, at, routingAt)
		}
	}
}

func assertPredecessorNotStopped(t *testing.T, cmds []simulatedCommand, oldName string) {
	t.Helper()
	for _, cmd := range cmds {
		name, _ := cmd.payload["containerName"].(string)
		if cmd.op == protocol.OpStopContainer && (name == oldName || cmd.payload["reason"] == "retire_previous") {
			t.Fatalf("predecessor stopped: %#v", cmd.payload)
		}
	}
}
