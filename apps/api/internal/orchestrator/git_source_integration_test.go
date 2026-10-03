package orchestrator_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
	"github.com/deploycore/deploy-core/apps/api/internal/orchestrator"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func seedGitApp(t *testing.T, pool *pgxpool.Pool, dockerfile, contextPath *string) (orgID, appID, envID, serverID, userID uuid.UUID) {
	t.Helper()
	orgID, appID, envID, serverID, userID = seedApp(t, pool)
	ctx := context.Background()
	tag, err := pool.Exec(ctx, `
		UPDATE application_configs
		SET source_type = 'git',
		    repository_url = 'https://github.com/vjbollavarapu/modulyn',
		    git_branch = 'main',
		    image_reference = NULL,
		    dockerfile_path = $2,
		    build_context = $3
		WHERE application_id = $1`, appID, dockerfile, contextPath)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated %d configs", tag.RowsAffected())
	}
	return orgID, appID, envID, serverID, userID
}

func runSimulated(t *testing.T, pool *pgxpool.Pool, orgID, appID, envID, serverID, userID uuid.UUID, requestID string) (*orchestrator.Orchestrator, deployments.Deployment) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := deployments.NewPostgresRepository(pool)
	orch := orchestrator.New(pool, repo, log, orchestrator.Config{SimulateAgent: true})
	created, err := repo.CreateQueued(context.Background(), orgID, appID, envID, &serverID, deployments.TriggerManual, nil, nil, requestID, userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := orch.Execute(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	return orch, got
}

func payloadFor(t *testing.T, orch *orchestrator.Orchestrator, operation, phase string) map[string]any {
	t.Helper()
	for _, cmd := range orch.SimulatedCommands() {
		if cmd.Operation != operation {
			continue
		}
		if phase == "" || cmd.Payload["phase"] == phase {
			return cmd.Payload
		}
	}
	t.Fatalf("missing %s phase %q", operation, phase)
	return nil
}

func TestGitRevisionSnapshotAndExactBuildPayloads(t *testing.T) {
	pool := testPool(t)
	dockerfile := "Dockerfile.prod"
	contextPath := "apps/backend"
	orgID, appID, envID, serverID, userID := seedGitApp(t, pool, &dockerfile, &contextPath)
	orch, got := runSimulated(t, pool, orgID, appID, envID, serverID, userID, "git-modulyn")
	if got.Status != deployments.StatusRunning {
		t.Fatalf("status=%s", got.Status)
	}
	if got.TargetRevisionID == nil {
		t.Fatal("missing revision")
	}

	var snap []byte
	var number int
	if err := pool.QueryRow(context.Background(), `
		SELECT revision_number, effective_config FROM revisions WHERE id = $1`, *got.TargetRevisionID).
		Scan(&number, &snap); err != nil {
		t.Fatal(err)
	}
	if number != 1 {
		t.Fatalf("revision number=%d", number)
	}
	var cfg map[string]any
	if err := json.Unmarshal(snap, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["sourceType"] != "git" || cfg["dockerfilePath"] != "Dockerfile.prod" || cfg["buildContext"] != "apps/backend" {
		t.Fatalf("snapshot=%s", snap)
	}
	if cfg["repositoryUrl"] != "https://github.com/vjbollavarapu/modulyn" || cfg["gitBranch"] != "main" {
		t.Fatalf("snapshot=%s", snap)
	}

	fetch := payloadFor(t, orch, protocol.OpBuildImage, protocol.BuildPhaseFetchSource)
	assertExactKeys(t, fetch, []string{"phase", "deploymentId", "applicationId", "revisionId", "repositoryUrl", "gitBranch"})
	if fetch["phase"] != "fetch_source" ||
		fetch["deploymentId"] != got.ID.String() ||
		fetch["applicationId"] != appID.String() ||
		fetch["revisionId"] != got.TargetRevisionID.String() ||
		fetch["repositoryUrl"] != "https://github.com/vjbollavarapu/modulyn" ||
		fetch["gitBranch"] != "main" {
		t.Fatalf("fetch=%v", fetch)
	}

	build := payloadFor(t, orch, protocol.OpBuildImage, protocol.BuildPhaseBuild)
	assertExactKeys(t, build, []string{
		"phase", "deploymentId", "applicationId", "revisionId", "repositoryUrl", "gitBranch", "dockerfilePath", "contextPath",
	})
	if build["dockerfilePath"] != "Dockerfile.prod" || build["contextPath"] != "apps/backend" || build["revisionId"] != got.TargetRevisionID.String() {
		t.Fatalf("build=%v", build)
	}
}

func TestGitBlankDockerfileAndContextUseDefaults(t *testing.T) {
	pool := testPool(t)
	orgID, appID, envID, serverID, userID := seedGitApp(t, pool, nil, nil)
	orch, got := runSimulated(t, pool, orgID, appID, envID, serverID, userID, "git-defaults")
	var snap []byte
	if err := pool.QueryRow(context.Background(), `SELECT effective_config FROM revisions WHERE id = $1`, *got.TargetRevisionID).Scan(&snap); err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(snap, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["dockerfilePath"] != "Dockerfile" || cfg["buildContext"] != "." {
		t.Fatalf("snapshot=%s", snap)
	}
	build := payloadFor(t, orch, protocol.OpBuildImage, protocol.BuildPhaseBuild)
	if build["dockerfilePath"] != "Dockerfile" || build["contextPath"] != "." {
		t.Fatalf("build=%v", build)
	}
}

func TestGitTraversalIsRejectedBeforeAgentCommand(t *testing.T) {
	pool := testPool(t)
	dockerfile := "../Dockerfile"
	contextPath := "/etc"
	orgID, appID, envID, serverID, userID := seedGitApp(t, pool, &dockerfile, &contextPath)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := deployments.NewPostgresRepository(pool)
	orch := orchestrator.New(pool, repo, log, orchestrator.Config{SimulateAgent: true})
	created, err := repo.CreateQueued(context.Background(), orgID, appID, envID, &serverID, deployments.TriggerManual, nil, nil, "git-traverse", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := orch.Execute(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != deployments.StatusSourceFailed {
		t.Fatalf("status=%s", got.Status)
	}
	if got.ErrorCode == nil || *got.ErrorCode != protocol.ErrInvalidSourcePath {
		t.Fatalf("error=%v", got.ErrorCode)
	}
	for _, cmd := range orch.SimulatedCommands() {
		if cmd.Operation == protocol.OpBuildImage {
			t.Fatalf("unsafe source was sent to the agent: %#v", cmd.Payload)
		}
	}
}

func TestImageSourceStillPullsAndSkipsFetch(t *testing.T) {
	pool := testPool(t)
	orgID, appID, envID, serverID, userID := seedApp(t, pool)
	orch, got := runSimulated(t, pool, orgID, appID, envID, serverID, userID, "image-regression")
	if got.Status != deployments.StatusRunning {
		t.Fatalf("status=%s", got.Status)
	}
	for _, cmd := range orch.SimulatedCommands() {
		if cmd.Operation == protocol.OpBuildImage {
			t.Fatalf("image source issued a build: %#v", cmd.Payload)
		}
	}
	pull := payloadFor(t, orch, protocol.OpPullImage, "pull")
	if pull["imageReference"] != "ghcr.io/example/api:1" || pull["deploymentId"] != got.ID.String() {
		t.Fatalf("pull=%v", pull)
	}
	var snap []byte
	if err := pool.QueryRow(context.Background(), `SELECT effective_config FROM revisions WHERE id = $1`, *got.TargetRevisionID).Scan(&snap); err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(snap, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["sourceType"] != "image" || cfg["dockerfilePath"] != "Dockerfile" || cfg["buildContext"] != "." {
		t.Fatalf("snapshot=%s", snap)
	}
}

func TestSecondDeploymentLeavesFailedRevisionUntouched(t *testing.T) {
	pool := testPool(t)
	dockerfile := "Dockerfile.prod"
	contextPath := "apps/backend"
	orgID, appID, envID, serverID, userID := seedGitApp(t, pool, &dockerfile, &contextPath)
	ctx := context.Background()
	original := []byte(`{"gitBranch":"main","healthCheck":{},"imageReference":null,"internalPort":8080,"repositoryUrl":"https://github.com/vjbollavarapu/modulyn","restartPolicy":"","runtimeConfig":{},"sourceType":"git"}`)
	var revisionID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO revisions (
			organization_id, application_id, revision_number, status, effective_config
		) VALUES ($1, $2, 1, 'FAILED', $3)
		RETURNING id`, orgID, appID, original).Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	var before []byte
	if err := pool.QueryRow(ctx, `SELECT effective_config FROM revisions WHERE id = $1`, revisionID).Scan(&before); err != nil {
		t.Fatal(err)
	}

	_, got := runSimulated(t, pool, orgID, appID, envID, serverID, userID, "git-r2")
	if got.Status != deployments.StatusRunning || got.TargetRevisionID == nil || *got.TargetRevisionID == revisionID {
		t.Fatalf("deployment=%s revision=%v", got.Status, got.TargetRevisionID)
	}
	var after []byte
	var status string
	var number int
	if err := pool.QueryRow(ctx, `SELECT status, revision_number, effective_config FROM revisions WHERE id = $1`, revisionID).Scan(&status, &number, &after); err != nil {
		t.Fatal(err)
	}
	if status != "FAILED" || number != 1 || string(after) != string(before) {
		t.Fatalf("r1 changed status=%s number=%d before=%s after=%s", status, number, before, after)
	}
	var createdNumber int
	var created []byte
	if err := pool.QueryRow(ctx, `SELECT revision_number, effective_config FROM revisions WHERE id = $1`, *got.TargetRevisionID).Scan(&createdNumber, &created); err != nil {
		t.Fatal(err)
	}
	if createdNumber != 2 {
		t.Fatalf("new revision number=%d", createdNumber)
	}
	var cfg map[string]any
	if err := json.Unmarshal(created, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["dockerfilePath"] != "Dockerfile.prod" || cfg["buildContext"] != "apps/backend" {
		t.Fatalf("r2 snapshot=%s", created)
	}
}

func assertExactKeys(t *testing.T, payload map[string]any, keys []string) {
	t.Helper()
	if len(payload) != len(keys) {
		t.Fatalf("payload keys=%v want %v", payload, keys)
	}
	for _, key := range keys {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing %s in %#v", key, payload)
		}
	}
}
