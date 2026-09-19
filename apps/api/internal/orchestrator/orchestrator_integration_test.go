package orchestrator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
	"github.com/deploycore/deploy-core/apps/api/internal/orchestrator"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
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
	_, _ = pool.Exec(ctx, `
		DELETE FROM deployment_events;
		DELETE FROM jobs;
		DELETE FROM application_replicas;
		DELETE FROM health_probe_samples;
		DELETE FROM application_health;
		UPDATE deployments SET active_revision_id = NULL, target_revision_id = NULL;
		DELETE FROM revisions;
		DELETE FROM deployments;
		DELETE FROM agent_commands;
		DELETE FROM application_configs;
		DELETE FROM applications;
		DELETE FROM environments;
		DELETE FROM projects;
		DELETE FROM servers;
		DELETE FROM environment_variables;
		DELETE FROM secrets;
		DELETE FROM organization_members;
		DELETE FROM organizations;
		DELETE FROM users;`)
	return pool
}

func seedApp(t *testing.T, pool *pgxpool.Pool) (orgID, appID, envID, serverID, userID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	userID = uuid.New()
	orgID = uuid.New()
	projectID := uuid.New()
	envID = uuid.New()
	serverID = uuid.New()
	appID = uuid.New()

	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, display_name) VALUES ($1, $2, 'x', 'Orch')`,
		userID, userID.String()+"@example.com")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO organizations (id, name, slug) VALUES ($1, 'O', 'o-b13')`, orgID)
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO projects (id, organization_id, name, slug) VALUES ($1, $2, 'P', 'p')`, projectID, orgID)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO environments (id, organization_id, project_id, name, slug) VALUES ($1, $2, $3, 'E', 'e')`,
		envID, orgID, projectID)
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO servers (id, organization_id, name, provider, region, hostname, architecture, status)
		VALUES ($1, $2, 's1', 'hetzner', 'fsn1', 's1.local', 'amd64', 'OFFLINE')`, serverID, orgID)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO applications (id, organization_id, project_id, environment_id, name, slug, type, target_server_id, status)
		VALUES ($1, $2, $3, $4, 'api', 'api', 'API', $5, 'ready')`,
		appID, orgID, projectID, envID, serverID)
	if err != nil {
		t.Fatalf("app: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO application_configs (
			organization_id, application_id, version, source_type, image_reference, internal_port
		) VALUES ($1, $2, 1, 'image', 'ghcr.io/example/api:1', 8080)`, orgID, appID)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO environment_variables (organization_id, scope, key, value)
		VALUES ($1, 'ORGANIZATION', 'LOG_LEVEL', 'info')`, orgID)
	if err != nil {
		t.Fatalf("var: %v", err)
	}
	return orgID, appID, envID, serverID, userID
}

func TestOrchestratorHappyPathSimulated(t *testing.T) {
	pool := testPool(t)
	orgID, appID, envID, serverID, userID := seedApp(t, pool)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	orch := orchestrator.New(pool, deployRepo, log, orchestrator.Config{SimulateAgent: true})

	d, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerManual, nil, nil, "req-1", userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if d.Status != deployments.StatusQueued {
		t.Fatalf("status=%s", d.Status)
	}

	if err := orch.Execute(context.Background(), d.ID); err != nil {
		t.Fatalf("execute: %v", err)
	}

	got, err := deployRepo.Get(context.Background(), d.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != deployments.StatusRunning {
		t.Fatalf("status=%s want RUNNING", got.Status)
	}
	if got.ActiveRevisionID == nil || got.TargetRevisionID == nil {
		t.Fatalf("revisions active=%v target=%v", got.ActiveRevisionID, got.TargetRevisionID)
	}
	if *got.ActiveRevisionID != *got.TargetRevisionID {
		t.Fatal("active should equal target after success")
	}

	var revStatus string
	var revNum int
	err = pool.QueryRow(context.Background(), `
		SELECT status, revision_number FROM revisions WHERE id = $1`, *got.ActiveRevisionID).
		Scan(&revStatus, &revNum)
	if err != nil {
		t.Fatalf("revision: %v", err)
	}
	if revStatus != "ACTIVE" || revNum != 1 {
		t.Fatalf("revision status=%s num=%d", revStatus, revNum)
	}

	events, err := deployRepo.ListEvents(context.Background(), d.ID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) < 10 {
		t.Fatalf("expected full transition trail, got %d events", len(events))
	}

	var appStatus string
	_ = pool.QueryRow(context.Background(), `SELECT status FROM applications WHERE id = $1`, appID).Scan(&appStatus)
	if appStatus != "running" {
		t.Fatalf("app status=%s", appStatus)
	}
}

func TestOrchestratorPreservesPreviousRevisionOnFailure(t *testing.T) {
	pool := testPool(t)
	orgID, appID, envID, serverID, userID := seedApp(t, pool)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	orch := orchestrator.New(pool, deployRepo, log, orchestrator.Config{SimulateAgent: true})

	// First successful deploy.
	d1, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerManual, nil, nil, "req-a", userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("create1: %v", err)
	}
	if err := orch.Execute(context.Background(), d1.ID); err != nil {
		t.Fatalf("exec1: %v", err)
	}
	ok1, _ := deployRepo.Get(context.Background(), d1.ID)
	prevRev := *ok1.ActiveRevisionID

	// Second deploy: put server in maintenance so validation fails at QUEUED.
	_, _ = pool.Exec(context.Background(), `
		UPDATE servers SET maintenance_mode = TRUE WHERE id = $1`, serverID)

	d2, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerManual, nil, nil, "req-b", userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("create2: %v", err)
	}
	if err := orch.Execute(context.Background(), d2.ID); err != nil {
		t.Fatalf("exec2: %v", err)
	}
	failed, _ := deployRepo.Get(context.Background(), d2.ID)
	if failed.Status != deployments.StatusTimeout {
		t.Fatalf("failed status=%s", failed.Status)
	}

	var activeCount int
	_ = pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM revisions WHERE application_id = $1 AND status = 'ACTIVE'`, appID).Scan(&activeCount)
	if activeCount != 1 {
		t.Fatalf("activeCount=%d", activeCount)
	}
	var stillActive uuid.UUID
	_ = pool.QueryRow(context.Background(), `
		SELECT id FROM revisions WHERE application_id = $1 AND status = 'ACTIVE'`, appID).Scan(&stillActive)
	if stillActive != prevRev {
		t.Fatalf("previous active revision was replaced on failure")
	}
}

func TestOrchestratorViaJobWorker(t *testing.T) {
	pool := testPool(t)
	orgID, appID, envID, serverID, userID := seedApp(t, pool)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	orch := orchestrator.New(pool, deployRepo, log, orchestrator.Config{SimulateAgent: true})

	d, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerAPI, nil, nil, "req-w", userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	q := jobs.NewQueue(jobs.NewPostgresRepository(pool), log)
	reg := jobs.Registry{}
	reg.Register(jobs.TypeDeploymentExecution, orch.JobHandler())
	w := jobs.NewWorker(q, reg, jobs.WorkerConfig{
		WorkerID:     "orch-test",
		LeaseTTL:     30 * time.Second,
		PollInterval: 20 * time.Millisecond,
	}, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, err := deployRepo.Get(context.Background(), d.ID)
		if err == nil && got.Status == deployments.StatusRunning {
			cancel()
			// Job should be succeeded.
			var st string
			_ = pool.QueryRow(context.Background(), `
				SELECT status FROM jobs
				WHERE related_resource_type = 'deployment' AND related_resource_id = $1
				ORDER BY created_at DESC LIMIT 1`, d.ID).Scan(&st)
			if st != jobs.StatusSucceeded {
				t.Fatalf("job status=%s", st)
			}
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	body, _ := json.Marshal(d)
	t.Fatalf("timeout waiting for RUNNING: %s", body)
}

func TestOrchestratorRollbackReusesRevision(t *testing.T) {
	pool := testPool(t)
	orgID, appID, envID, serverID, userID := seedApp(t, pool)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	orch := orchestrator.New(pool, deployRepo, log, orchestrator.Config{SimulateAgent: true})

	d1, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerManual, nil, nil, "rb-1", userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("create1: %v", err)
	}
	if err := orch.Execute(context.Background(), d1.ID); err != nil {
		t.Fatalf("exec1: %v", err)
	}
	ok1, _ := deployRepo.Get(context.Background(), d1.ID)
	rev1 := *ok1.ActiveRevisionID

	d2, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerManual, nil, nil, "rb-2", userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("create2: %v", err)
	}
	if err := orch.Execute(context.Background(), d2.ID); err != nil {
		t.Fatalf("exec2: %v", err)
	}
	ok2, _ := deployRepo.Get(context.Background(), d2.ID)
	rev2 := *ok2.ActiveRevisionID
	if rev1 == rev2 {
		t.Fatal("expected distinct revisions")
	}

	var rev1Status string
	_ = pool.QueryRow(context.Background(), `SELECT status FROM revisions WHERE id = $1`, rev1).Scan(&rev1Status)
	if rev1Status != "INACTIVE" {
		t.Fatalf("rev1 status=%s want INACTIVE", rev1Status)
	}

	rb, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerRollback, nil, nil, "rb-3", userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("create rollback: %v", err)
	}
	if err := deployRepo.SetTargetRevision(context.Background(), rb.ID, rev1); err != nil {
		t.Fatalf("set target: %v", err)
	}
	if err := orch.Execute(context.Background(), rb.ID); err != nil {
		t.Fatalf("exec rollback: %v", err)
	}

	got, err := deployRepo.Get(context.Background(), rb.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != deployments.StatusRunning {
		t.Fatalf("status=%s", got.Status)
	}
	if got.ActiveRevisionID == nil || *got.ActiveRevisionID != rev1 {
		t.Fatalf("active=%v want %s", got.ActiveRevisionID, rev1)
	}

	var activeStatus, prevStatus string
	_ = pool.QueryRow(context.Background(), `SELECT status FROM revisions WHERE id = $1`, rev1).Scan(&activeStatus)
	_ = pool.QueryRow(context.Background(), `SELECT status FROM revisions WHERE id = $1`, rev2).Scan(&prevStatus)
	if activeStatus != "ACTIVE" {
		t.Fatalf("rev1=%s", activeStatus)
	}
	if prevStatus != "INACTIVE" {
		t.Fatalf("rev2=%s", prevStatus)
	}

	var revCount int
	_ = pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM revisions WHERE application_id = $1`, appID).Scan(&revCount)
	if revCount != 2 {
		t.Fatalf("rollback must not create a new revision, count=%d", revCount)
	}

	events, _ := deployRepo.ListEvents(context.Background(), rb.ID)
	foundSkip := false
	for _, e := range events {
		if e.Message == "rollback: skip build/pull" || e.Message == "rollback: skip source fetch" {
			foundSkip = true
			break
		}
	}
	if !foundSkip {
		t.Fatal("expected skip rebuild events")
	}
}

func TestOrchestratorRollbackHealthFailPreservesActive(t *testing.T) {
	pool := testPool(t)
	orgID, appID, envID, serverID, userID := seedApp(t, pool)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	orch := orchestrator.New(pool, deployRepo, log, orchestrator.Config{SimulateAgent: true})

	d1, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerManual, nil, nil, "hf-1", userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("create1: %v", err)
	}
	if err := orch.Execute(context.Background(), d1.ID); err != nil {
		t.Fatalf("exec1: %v", err)
	}
	ok1, _ := deployRepo.Get(context.Background(), d1.ID)
	active := *ok1.ActiveRevisionID

	d2, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerManual, nil, nil, "hf-2", userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("create2: %v", err)
	}
	if err := orch.Execute(context.Background(), d2.ID); err != nil {
		t.Fatalf("exec2: %v", err)
	}
	ok2, _ := deployRepo.Get(context.Background(), d2.ID)
	current := *ok2.ActiveRevisionID

	failOrch := orchestrator.New(pool, deployRepo, log, orchestrator.Config{
		SimulateAgent:   true,
		ForceHealthFail: true,
	})
	rb, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerRollback, nil, nil, "hf-3", userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("create rollback: %v", err)
	}
	if err := deployRepo.SetTargetRevision(context.Background(), rb.ID, active); err != nil {
		t.Fatalf("set target: %v", err)
	}
	if err := failOrch.Execute(context.Background(), rb.ID); err != nil {
		t.Fatalf("exec rollback: %v", err)
	}

	failed, _ := deployRepo.Get(context.Background(), rb.ID)
	if failed.Status != deployments.StatusHealthCheckFailed {
		t.Fatalf("status=%s", failed.Status)
	}

	var stillActive, targetStatus string
	_ = pool.QueryRow(context.Background(), `SELECT status FROM revisions WHERE id = $1`, current).Scan(&stillActive)
	_ = pool.QueryRow(context.Background(), `SELECT status FROM revisions WHERE id = $1`, active).Scan(&targetStatus)
	if stillActive != "ACTIVE" {
		t.Fatalf("current active was changed: %s", stillActive)
	}
	if targetStatus == "FAILED" {
		t.Fatal("rollback must not mark target revision FAILED")
	}
	if targetStatus != "INACTIVE" && targetStatus != "READY" {
		t.Fatalf("target status=%s", targetStatus)
	}
}

func TestOrchestratorBuildFailure(t *testing.T) {
	pool := testPool(t)
	orgID, appID, envID, serverID, userID := seedApp(t, pool)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	orch := orchestrator.New(pool, deployRepo, log, orchestrator.Config{
		SimulateAgent:  true,
		ForceBuildFail: true,
	})
	d, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerManual, nil, nil, "build-fail", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := orch.Execute(context.Background(), d.ID); err != nil {
		t.Fatalf("exec: %v", err)
	}
	got, _ := deployRepo.Get(context.Background(), d.ID)
	if got.Status != deployments.StatusBuildFailed {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestOrchestratorStartFailure(t *testing.T) {
	pool := testPool(t)
	orgID, appID, envID, serverID, userID := seedApp(t, pool)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	orch := orchestrator.New(pool, deployRepo, log, orchestrator.Config{
		SimulateAgent:  true,
		ForceStartFail: true,
	})
	d, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerManual, nil, nil, "start-fail", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := orch.Execute(context.Background(), d.ID); err != nil {
		t.Fatalf("exec: %v", err)
	}
	got, _ := deployRepo.Get(context.Background(), d.ID)
	if got.Status != deployments.StatusStartFailed {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestOrchestratorForwardHealthFailurePreservesPreviousActive(t *testing.T) {
	pool := testPool(t)
	orgID, appID, envID, serverID, userID := seedApp(t, pool)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	okOrch := orchestrator.New(pool, deployRepo, log, orchestrator.Config{SimulateAgent: true})

	d1, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerManual, nil, nil, "fwd-hf-1", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := okOrch.Execute(context.Background(), d1.ID); err != nil {
		t.Fatalf("exec1: %v", err)
	}
	ok1, _ := deployRepo.Get(context.Background(), d1.ID)
	if ok1.ActiveRevisionID == nil {
		t.Fatal("expected active revision")
	}
	active := *ok1.ActiveRevisionID

	failOrch := orchestrator.New(pool, deployRepo, log, orchestrator.Config{
		SimulateAgent:   true,
		ForceHealthFail: true,
	})
	d2, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
		deployments.TriggerManual, nil, nil, "fwd-hf-2", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := failOrch.Execute(context.Background(), d2.ID); err != nil {
		t.Fatalf("exec2: %v", err)
	}
	failed, _ := deployRepo.Get(context.Background(), d2.ID)
	if failed.Status != deployments.StatusHealthCheckFailed {
		t.Fatalf("status=%s", failed.Status)
	}
	var still string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM revisions WHERE id = $1`, active).Scan(&still); err != nil {
		t.Fatal(err)
	}
	if still != "ACTIVE" {
		t.Fatalf("previous active became %s", still)
	}
}

func TestOrchestratorRevisionNumbersIncrement(t *testing.T) {
	pool := testPool(t)
	orgID, appID, envID, serverID, userID := seedApp(t, pool)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	orch := orchestrator.New(pool, deployRepo, log, orchestrator.Config{SimulateAgent: true})

	var numbers []int
	for i := 0; i < 2; i++ {
		d, err := deployRepo.CreateQueued(context.Background(), orgID, appID, envID, &serverID,
			deployments.TriggerManual, nil, nil, fmt.Sprintf("rev-%d", i), userID, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		if err := orch.Execute(context.Background(), d.ID); err != nil {
			t.Fatalf("exec %d: %v", i, err)
		}
		got, _ := deployRepo.Get(context.Background(), d.ID)
		if got.ActiveRevisionID == nil {
			t.Fatal("missing active revision")
		}
		var n int
		if err := pool.QueryRow(context.Background(),
			`SELECT revision_number FROM revisions WHERE id = $1`, *got.ActiveRevisionID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		numbers = append(numbers, n)
	}
	if numbers[0] != 1 || numbers[1] != 2 {
		t.Fatalf("revision numbers=%v", numbers)
	}
}
