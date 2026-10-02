package orchestrator

import (
	"context"
	"encoding/json"
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

func TestWritableCandidateFailureRestartsPreviousContainer(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, organization_id, name, slug) VALUES ($1, $2, 'P', 'p')`, projectID, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO environments (id, organization_id, project_id, name, slug) VALUES ($1, $2, $3, 'E', 'e')`, envID, orgID, projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO servers (id, organization_id, name, provider, region, hostname, architecture, status) VALUES ($1, $2, 's1', 'hetzner', 'fsn1', 's1.local', 'amd64', 'OFFLINE')`, serverID, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, organization_id, project_id, environment_id, name, slug, type, target_server_id, status) VALUES ($1, $2, $3, $4, 'api', 'api', 'API', $5, 'ready')`, appID, orgID, projectID, envID, serverID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO application_configs (organization_id, application_id, version, source_type, image_reference, internal_port) VALUES ($1, $2, 1, 'image', 'ghcr.io/example/api:1', 8080)`, orgID, appID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO volumes (organization_id, server_id, name, mount_path, state, attached_resource_type, attached_resource_id, docker_name, labels) VALUES ($1, $2, 'redis-data', '/data', 'ATTACHED', 'application', $3, 'redis-data', '{}')`, orgID, serverID, appID); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	orch := New(pool, deployRepo, log, Config{SimulateAgent: true})
	first, err := deployRepo.CreateQueued(ctx, orgID, appID, envID, &serverID, deployments.TriggerManual, nil, nil, "cutover-1", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := orch.Execute(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	active, _ := deployRepo.Get(ctx, first.ID)
	if active.Status != deployments.StatusRunning {
		t.Fatalf("first status=%s", active.Status)
	}
	var volumeState string
	if err := pool.QueryRow(ctx, `SELECT state FROM volumes WHERE name = 'redis-data' AND organization_id = $1`, orgID).Scan(&volumeState); err != nil {
		t.Fatal(err)
	}
	if volumeState != "ATTACHED" {
		t.Fatalf("volume state=%s", volumeState)
	}

	replacement := New(pool, deployRepo, log, Config{SimulateAgent: true, ForceStartFail: true})
	second, err := deployRepo.CreateQueued(ctx, orgID, appID, envID, &serverID, deployments.TriggerManual, nil, nil, "cutover-2", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := replacement.Execute(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	failed, _ := deployRepo.Get(ctx, second.ID)
	if failed.Status != deployments.StatusStartFailed {
		t.Fatalf("status=%s", failed.Status)
	}

	oldName := replicas.ContainerName("api", 1, 0)
	var stoppedOld, startedOld, deployedBeforeStop bool
	deployIndex, stopIndex := -1, -1
	for i, cmd := range replacement.simulated {
		if cmd.deploymentID != second.ID {
			continue
		}
		name, _ := cmd.payload["containerName"].(string)
		switch cmd.op {
		case protocol.OpDeployRevision:
			if deployIndex < 0 {
				deployIndex = i
			}
		case protocol.OpStopContainer:
			if name == oldName && cmd.payload["reason"] == cutoverReason {
				stoppedOld = true
				if stopIndex < 0 {
					stopIndex = i
				}
			}
		case protocol.OpStartContainer:
			if name == oldName && cmd.payload["reason"] == "writable_volume_restore" {
				startedOld = true
			}
		}
	}
	if !stoppedOld || stopIndex < 0 || deployIndex < 0 || stopIndex > deployIndex {
		t.Fatalf("stop-before-create missing: stop=%d deploy=%d cmds=%d", stopIndex, deployIndex, len(replacement.simulated))
	}
	deployedBeforeStop = stopIndex < deployIndex
	if !deployedBeforeStop || !startedOld {
		t.Fatalf("stoppedOld=%v startedOld=%v", stoppedOld, startedOld)
	}
	if err := pool.QueryRow(ctx, `SELECT state FROM volumes WHERE name = 'redis-data' AND organization_id = $1`, orgID).Scan(&volumeState); err != nil {
		t.Fatal(err)
	}
	if volumeState != "ATTACHED" {
		t.Fatalf("volume deleted or detached, state=%s", volumeState)
	}
	if active.ActiveRevisionID == nil {
		t.Fatal("missing active revision")
	}
	var slotName string
	var slotRevision uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT container_name, revision_id FROM application_replicas
		WHERE application_id = $1 AND replica_index = 0`, appID).Scan(&slotName, &slotRevision); err != nil {
		t.Fatal(err)
	}
	if slotName != oldName || slotRevision != *active.ActiveRevisionID {
		t.Fatalf("slot after failure = %s %s, want %s %s", slotName, slotRevision, oldName, *active.ActiveRevisionID)
	}

	retry := New(pool, deployRepo, log, Config{SimulateAgent: true})
	third, err := deployRepo.CreateQueued(ctx, orgID, appID, envID, &serverID, deployments.TriggerManual, nil, nil, "cutover-3", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := retry.Execute(ctx, third.ID); err != nil {
		t.Fatal(err)
	}
	retryStop, retryDeploy := -1, -1
	for i, cmd := range retry.simulated {
		if cmd.deploymentID != third.ID {
			continue
		}
		name, _ := cmd.payload["containerName"].(string)
		switch cmd.op {
		case protocol.OpStopContainer:
			if name == oldName && cmd.payload["reason"] == cutoverReason && retryStop < 0 {
				retryStop = i
			}
		case protocol.OpDeployRevision:
			if retryDeploy < 0 {
				retryDeploy = i
			}
		}
	}
	if retryStop < 0 || retryDeploy < 0 || retryStop > retryDeploy {
		t.Fatalf("retry did not stop the restored container before create: stop=%d deploy=%d", retryStop, retryDeploy)
	}
}

// TestWritableCandidateFailureRestoresActiveRevision persists real agent
// commands. issueOrSimulate rewrites payload revisionId to the candidate, and
// the replica row must still keep the predecessor revision.
func TestWritableCandidateFailureRestoresActiveRevision(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b13_test?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
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
	if _, err := pool.Exec(ctx, `INSERT INTO servers (id, organization_id, name, provider, region, hostname, architecture, status) VALUES ($1, $2, 's1', 'hetzner', 'fsn1', 's1.local', 'amd64', 'ONLINE')`, serverID, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO applications (id, organization_id, project_id, environment_id, name, slug, type, target_server_id, status) VALUES ($1, $2, $3, $4, 'api', 'api', 'API', $5, 'ready')`, appID, orgID, projectID, envID, serverID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO application_configs (organization_id, application_id, version, source_type, image_reference, internal_port) VALUES ($1, $2, 1, 'image', 'ghcr.io/example/api:1', 8080)`, orgID, appID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO volumes (organization_id, server_id, name, mount_path, state, attached_resource_type, attached_resource_id, docker_name, labels) VALUES ($1, $2, 'redis-data', '/data', 'ATTACHED', 'application', $3, 'redis-data', '{}')`, orgID, serverID, appID); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	firstOrch := New(pool, deployRepo, log, Config{SimulateAgent: true})
	first, err := deployRepo.CreateQueued(ctx, orgID, appID, envID, &serverID, deployments.TriggerManual, nil, nil, "cutover-prod-1", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := firstOrch.Execute(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	active, _ := deployRepo.Get(ctx, first.ID)
	if active.Status != deployments.StatusRunning || active.ActiveRevisionID == nil {
		t.Fatalf("first status=%s active=%v", active.Status, active.ActiveRevisionID)
	}

	completeCtx, stopComplete := context.WithCancel(context.Background())
	defer stopComplete()
	go completePendingAgentCommands(completeCtx, pool, serverID)

	replacement := New(pool, deployRepo, log, Config{ForceStartFail: true})
	second, err := deployRepo.CreateQueued(ctx, orgID, appID, envID, &serverID, deployments.TriggerManual, nil, nil, "cutover-prod-2", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := replacement.Execute(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	failed, err := deployRepo.Get(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != deployments.StatusStartFailed || failed.TargetRevisionID == nil {
		t.Fatalf("status=%s target=%v", failed.Status, failed.TargetRevisionID)
	}

	var predecessorNumber, candidateNumber int
	if err := pool.QueryRow(ctx, `SELECT revision_number FROM revisions WHERE id = $1`, *active.ActiveRevisionID).Scan(&predecessorNumber); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT revision_number FROM revisions WHERE id = $1`, *failed.TargetRevisionID).Scan(&candidateNumber); err != nil {
		t.Fatal(err)
	}
	oldName := replicas.ContainerName("api", predecessorNumber, 0)
	candidateName := replicas.ContainerName("api", candidateNumber, 0)
	cmds := loadDeploymentCommands(t, ctx, pool, second.ID)
	cutoverStop, candidateStop, predecessorStart, deployAt := -1, -1, -1, -1
	for i, cmd := range cmds {
		name, _ := cmd.payload["containerName"].(string)
		reason, _ := cmd.payload["reason"].(string)
		switch {
		case cmd.op == protocol.OpDeployRevision && deployAt < 0:
			deployAt = i
		case cmd.op == protocol.OpStopContainer && name == oldName && reason == cutoverReason && cutoverStop < 0:
			cutoverStop = i
			if cmd.payload["revisionId"] != failed.TargetRevisionID.String() {
				t.Fatalf("production stop revisionId = %v, want overwritten candidate %s", cmd.payload["revisionId"], *failed.TargetRevisionID)
			}
			if cmd.payload["predecessorRevisionId"] != active.ActiveRevisionID.String() {
				t.Fatalf("predecessorRevisionId = %v, want %s", cmd.payload["predecessorRevisionId"], *active.ActiveRevisionID)
			}
		case cmd.op == protocol.OpStopContainer && name == candidateName && reason == "writable_volume_candidate_failed" && candidateStop < 0:
			candidateStop = i
		case cmd.op == protocol.OpStartContainer && name == oldName && reason == "writable_volume_restore" && predecessorStart < 0:
			predecessorStart = i
		}
	}
	if cutoverStop < 0 || deployAt < 0 || cutoverStop > deployAt {
		t.Fatalf("production cutover stop before deploy missing: stop=%d deploy=%d", cutoverStop, deployAt)
	}
	if candidateStop < 0 || predecessorStart < 0 || candidateStop > predecessorStart {
		t.Fatalf("candidate stop before predecessor start missing: stop=%d start=%d", candidateStop, predecessorStart)
	}

	var slotName, slotStatus string
	var slotRevision uuid.UUID
	var slotIndex int
	if err := pool.QueryRow(ctx, `
		SELECT container_name, revision_id, replica_index, status
		FROM application_replicas
		WHERE application_id = $1 AND replica_index = 0`, appID).Scan(&slotName, &slotRevision, &slotIndex, &slotStatus); err != nil {
		t.Fatal(err)
	}
	if slotName != oldName || slotIndex != 0 || slotRevision != *active.ActiveRevisionID || slotRevision == *failed.TargetRevisionID || slotStatus != replicas.StatusRunning {
		t.Fatalf("slot = name %s index %d revision %s status %s; want %s index 0 revision %s status %s",
			slotName, slotIndex, slotRevision, slotStatus, oldName, *active.ActiveRevisionID, replicas.StatusRunning)
	}

	retry := New(pool, deployRepo, log, Config{SimulateAgent: true})
	third, err := deployRepo.CreateQueued(ctx, orgID, appID, envID, &serverID, deployments.TriggerManual, nil, nil, "cutover-prod-3", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := retry.Execute(ctx, third.ID); err != nil {
		t.Fatal(err)
	}
	retryStop, retryDeploy := -1, -1
	for i, cmd := range retry.simulated {
		if cmd.deploymentID != third.ID {
			continue
		}
		name, _ := cmd.payload["containerName"].(string)
		switch cmd.op {
		case protocol.OpStopContainer:
			if name == oldName && cmd.payload["reason"] == cutoverReason && retryStop < 0 {
				retryStop = i
			}
		case protocol.OpDeployRevision:
			if retryDeploy < 0 {
				retryDeploy = i
			}
		}
	}
	if retryStop < 0 || retryDeploy < 0 || retryStop > retryDeploy {
		t.Fatalf("next deploy did not stop restored predecessor before create: stop=%d deploy=%d", retryStop, retryDeploy)
	}
}

func TestWritableFirstDeploySkipsPlaceholderAndAdoptsSlot(t *testing.T) {
	pool, orgID, envID, serverID, appID, userID := seedWritableApp(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO application_replicas (
			organization_id, application_id, replica_index, container_name, status
		) VALUES ($1, $2, 0, 'redis-r0-0', 'STARTING')`, orgID, appID); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	orch := New(pool, deployRepo, log, Config{SimulateAgent: true})
	first, err := deployRepo.CreateQueued(ctx, orgID, appID, envID, &serverID, deployments.TriggerManual, nil, nil, "placeholder-1", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := orch.Execute(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	got, err := deployRepo.Get(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != deployments.StatusRunning || got.TargetRevisionID == nil || got.ActiveRevisionID == nil {
		t.Fatalf("status=%s target=%v active=%v", got.Status, got.TargetRevisionID, got.ActiveRevisionID)
	}
	for _, cmd := range orch.simulated {
		if cmd.deploymentID != first.ID {
			continue
		}
		name, _ := cmd.payload["containerName"].(string)
		if cmd.op == protocol.OpStopContainer && name == "redis-r0-0" {
			t.Fatal("placeholder slot was stopped")
		}
	}
	var revNumber int
	if err := pool.QueryRow(ctx, `SELECT revision_number FROM revisions WHERE id = $1`, *got.TargetRevisionID).Scan(&revNumber); err != nil {
		t.Fatal(err)
	}
	wantName := replicas.ContainerName("api", revNumber, 0)
	var slotName, slotStatus string
	var slotRevision uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT container_name, revision_id, status
		FROM application_replicas
		WHERE application_id = $1 AND replica_index = 0`, appID).Scan(&slotName, &slotRevision, &slotStatus); err != nil {
		t.Fatal(err)
	}
	if slotName != wantName || slotRevision != *got.TargetRevisionID || slotName == "redis-r0-0" {
		t.Fatalf("slot name=%s revision=%s status=%s, want %s revision %s", slotName, slotRevision, slotStatus, wantName, *got.TargetRevisionID)
	}
	var deployed bool
	for _, cmd := range orch.simulated {
		if cmd.deploymentID == first.ID && cmd.op == protocol.OpDeployRevision && cmd.payload["containerName"] == wantName {
			deployed = true
		}
	}
	if !deployed {
		t.Fatalf("missing DEPLOY_REVISION for %s", wantName)
	}
}

func TestWritableRollbackStopsActiveHolder(t *testing.T) {
	pool, orgID, envID, serverID, appID, userID := seedWritableApp(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	firstOrch := New(pool, deployRepo, log, Config{SimulateAgent: true})
	first, err := deployRepo.CreateQueued(ctx, orgID, appID, envID, &serverID, deployments.TriggerManual, nil, nil, "wrb-1", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := firstOrch.Execute(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	active, err := deployRepo.Get(ctx, first.ID)
	if err != nil || active.Status != deployments.StatusRunning || active.ActiveRevisionID == nil {
		t.Fatalf("first status=%s err=%v", active.Status, err)
	}
	secondOrch := New(pool, deployRepo, log, Config{SimulateAgent: true})
	second, err := deployRepo.CreateQueued(ctx, orgID, appID, envID, &serverID, deployments.TriggerManual, nil, nil, "wrb-2", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := secondOrch.Execute(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	current, err := deployRepo.Get(ctx, second.ID)
	if err != nil || current.Status != deployments.StatusRunning || current.ActiveRevisionID == nil {
		t.Fatalf("second status=%s err=%v", current.Status, err)
	}
	holder := replicas.ContainerName("api", 2, 0)
	if holder != "dc-api-r2-1" || replicas.ContainerName("api", 1, 0) != "dc-api-r1-1" {
		t.Fatalf("platform names holder=%s previous=%s", holder, replicas.ContainerName("api", 1, 0))
	}
	stopAt, deployAt := -1, -1
	for i, cmd := range secondOrch.simulated {
		if cmd.deploymentID != second.ID {
			continue
		}
		name, _ := cmd.payload["containerName"].(string)
		if cmd.op == protocol.OpStopContainer && name == replicas.ContainerName("api", 1, 0) && cmd.payload["reason"] == cutoverReason && stopAt < 0 {
			stopAt = i
		}
		if cmd.op == protocol.OpDeployRevision && cmd.payload["phase"] == "create_container" && deployAt < 0 {
			deployAt = i
			if name != "dc-api-r2-1" || cmd.payload["revisionNumber"] != 2 {
				t.Fatalf("second deploy payload name=%s revisionNumber=%v", name, cmd.payload["revisionNumber"])
			}
		}
	}
	if stopAt < 0 || deployAt < 0 || stopAt > deployAt {
		t.Fatalf("second deploy did not stop the active holder before create: stop=%d deploy=%d", stopAt, deployAt)
	}

	rbOrch := New(pool, deployRepo, log, Config{SimulateAgent: true})
	rb, err := deployRepo.CreateQueued(ctx, orgID, appID, envID, &serverID, deployments.TriggerRollback, nil, nil, "wrb-3", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := deployRepo.SetTargetRevision(ctx, rb.ID, *active.ActiveRevisionID); err != nil {
		t.Fatal(err)
	}
	if err := rbOrch.Execute(ctx, rb.ID); err != nil {
		t.Fatal(err)
	}
	rolled, err := deployRepo.Get(ctx, rb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Status != deployments.StatusRunning || rolled.ActiveRevisionID == nil || *rolled.ActiveRevisionID != *active.ActiveRevisionID {
		t.Fatalf("rollback status=%s active=%v", rolled.Status, rolled.ActiveRevisionID)
	}
	rbStop, rbDeploy := -1, -1
	for i, cmd := range rbOrch.simulated {
		if cmd.deploymentID != rb.ID {
			continue
		}
		name, _ := cmd.payload["containerName"].(string)
		if cmd.op == protocol.OpStopContainer && name == holder && cmd.payload["reason"] == cutoverReason && rbStop < 0 {
			rbStop = i
		}
		if cmd.op == protocol.OpDeployRevision && cmd.payload["phase"] == "rollback" && rbDeploy < 0 {
			rbDeploy = i
			if name != "dc-api-r1-1" || cmd.payload["revisionNumber"] != 1 {
				t.Fatalf("rollback payload name=%s revisionNumber=%v", name, cmd.payload["revisionNumber"])
			}
		}
	}
	if rbStop < 0 || rbDeploy < 0 || rbStop > rbDeploy {
		t.Fatalf("rollback did not stop the active holder before create: stop=%d deploy=%d", rbStop, rbDeploy)
	}
	var slotName string
	var slotRevision uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT container_name, revision_id FROM application_replicas
		WHERE application_id = $1 AND replica_index = 0`, appID).Scan(&slotName, &slotRevision); err != nil {
		t.Fatal(err)
	}
	if slotName != replicas.ContainerName("api", 1, 0) || slotRevision != *active.ActiveRevisionID {
		t.Fatalf("slot after rollback = %s %s", slotName, slotRevision)
	}
}

func seedWritableApp(t *testing.T) (*pgxpool.Pool, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
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
	if _, err := pool.Exec(ctx, `INSERT INTO application_configs (organization_id, application_id, version, source_type, image_reference, internal_port, runtime_config) VALUES ($1, $2, 1, 'image', 'redis:7-alpine', 6379, '{"desiredReplicas":1}')`, orgID, appID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO volumes (organization_id, server_id, name, mount_path, state, attached_resource_type, attached_resource_id, docker_name, labels) VALUES ($1, $2, 'redis-data', '/data', 'ATTACHED', 'application', $3, 'redis-data', '{}')`, orgID, serverID, appID); err != nil {
		t.Fatal(err)
	}
	return pool, orgID, envID, serverID, appID, userID
}

type recordedCommand struct {
	op      string
	payload map[string]any
}

func loadDeploymentCommands(t *testing.T, ctx context.Context, pool *pgxpool.Pool, deploymentID uuid.UUID) []recordedCommand {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT operation, payload
		FROM agent_commands
		WHERE correlation_id = $1
		ORDER BY created_at ASC, id ASC`, deploymentID.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []recordedCommand
	for rows.Next() {
		var op string
		var raw []byte
		if err := rows.Scan(&op, &raw); err != nil {
			t.Fatal(err)
		}
		payload := map[string]any{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatal(err)
			}
		}
		out = append(out, recordedCommand{op: op, payload: payload})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func completePendingAgentCommands(ctx context.Context, pool *pgxpool.Pool, serverID uuid.UUID) {
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = pool.Exec(context.Background(), `
				UPDATE agent_commands
				SET status = 'completed', finished_at = NOW()
				WHERE server_id = $1 AND status = 'pending'`, serverID)
		}
	}
}
