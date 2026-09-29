package orchestrator

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSecretSnapshotScopePrecedence(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `INSERT INTO application_configs (organization_id, application_id, version, source_type, image_reference, internal_port) VALUES ($1, $2, 1, 'image', 'ghcr.io/example/api:1', 8080)`, orgID, appID); err != nil {
		t.Fatal(err)
	}

	insertVar := func(scope, key, value string, project, environment, application *uuid.UUID) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO environment_variables (
				organization_id, scope, project_id, environment_id, application_id, key, value
			) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			orgID, scope, project, environment, application, key, value); err != nil {
			t.Fatal(err)
		}
	}
	insertVar("ORGANIZATION", "LOG_LEVEL", "info", nil, nil, nil)
	insertVar("APPLICATION", "LOG_LEVEL", "warn", &projectID, &envID, &appID)
	insertVar("ENVIRONMENT", "REGION", "eu", &projectID, &envID, nil)

	insertSecret := func(scope, name string, version int, project, environment, application *uuid.UUID) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO secrets (
				organization_id, scope, project_id, environment_id, application_id,
				name, version, ciphertext, nonce, key_id, algorithm
			) VALUES ($1, $2, $3, $4, $5, $6, $7, '\x01', '\x02', 'test-key', 'AES-256-GCM')`,
			orgID, scope, project, environment, application, name, version); err != nil {
			t.Fatal(err)
		}
	}
	insertSecret("ORGANIZATION", "ONLY_ORG", 11, nil, nil, nil)
	insertSecret("ORGANIZATION", "SHARED_OP", 1, nil, nil, nil)
	insertSecret("PROJECT", "SHARED_OP", 2, &projectID, nil, nil)
	insertSecret("PROJECT", "SHARED_PE", 3, &projectID, nil, nil)
	insertSecret("ENVIRONMENT", "SHARED_PE", 4, &projectID, &envID, nil)
	insertSecret("ENVIRONMENT", "SHARED_EA", 5, &projectID, &envID, nil)
	insertSecret("APPLICATION", "SHARED_EA", 6, &projectID, &envID, &appID)
	insertSecret("ORGANIZATION", "SHARED_ALL", 7, nil, nil, nil)
	insertSecret("PROJECT", "SHARED_ALL", 8, &projectID, nil, nil)
	insertSecret("ENVIRONMENT", "SHARED_ALL", 9, &projectID, &envID, nil)
	insertSecret("APPLICATION", "SHARED_ALL", 10, &projectID, &envID, &appID)
	insertSecret("ORGANIZATION", "OTHER_ORG", 1, nil, nil, nil)
	insertSecret("APPLICATION", "OTHER_APP", 2, &projectID, &envID, &appID)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deployRepo := deployments.NewPostgresRepository(pool)
	orch := New(pool, deployRepo, log, Config{SimulateAgent: true})
	first, err := deployRepo.CreateQueued(ctx, orgID, appID, envID, &serverID, deployments.TriggerManual, nil, nil, "secret-scope-1", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := orch.Execute(ctx, first.ID); err != nil {
		t.Fatal(err)
	}

	rawRefs, rawVars := revisionSnapshots(t, ctx, pool, appID, 1)
	if strings.Contains(string(rawRefs), "ciphertext") || strings.Contains(string(rawVars), "ciphertext") {
		t.Fatal("revision snapshot contains ciphertext")
	}
	refs := parseSecretRefs(t, rawRefs)
	want := map[string]secretPick{
		"ONLY_ORG":   {scope: "ORGANIZATION", version: 11},
		"SHARED_OP":  {scope: "PROJECT", version: 2},
		"SHARED_PE":  {scope: "ENVIRONMENT", version: 4},
		"SHARED_EA":  {scope: "APPLICATION", version: 6},
		"SHARED_ALL": {scope: "APPLICATION", version: 10},
		"OTHER_ORG":  {scope: "ORGANIZATION", version: 1},
		"OTHER_APP":  {scope: "APPLICATION", version: 2},
	}
	if len(refs) != len(want) {
		t.Fatalf("secret refs = %#v", refs)
	}
	for name, pick := range want {
		got, ok := refs[name]
		if !ok || got != pick {
			t.Fatalf("secret %s = %#v, want %#v", name, got, pick)
		}
	}

	vars := map[string]map[string]any{}
	if err := json.Unmarshal(rawVars, &vars); err != nil {
		t.Fatal(err)
	}
	if vars["LOG_LEVEL"]["value"] != "warn" || vars["LOG_LEVEL"]["scope"] != "APPLICATION" {
		t.Fatalf("LOG_LEVEL = %#v", vars["LOG_LEVEL"])
	}
	if vars["REGION"]["value"] != "eu" || vars["REGION"]["scope"] != "ENVIRONMENT" {
		t.Fatalf("REGION = %#v", vars["REGION"])
	}

	if _, err := pool.Exec(ctx, `
		UPDATE secrets SET deleted_at = NOW()
		WHERE organization_id = $1 AND name = 'SHARED_ALL' AND scope = 'APPLICATION' AND deleted_at IS NULL`, orgID); err != nil {
		t.Fatal(err)
	}
	insertSecret("APPLICATION", "SHARED_ALL", 11, &projectID, &envID, &appID)

	rawAfter, _ := revisionSnapshots(t, ctx, pool, appID, 1)
	after := parseSecretRefs(t, rawAfter)
	if after["SHARED_ALL"] != want["SHARED_ALL"] {
		t.Fatalf("rotated revision ref = %#v", after["SHARED_ALL"])
	}

	second, err := deployRepo.CreateQueued(ctx, orgID, appID, envID, &serverID, deployments.TriggerManual, nil, nil, "secret-scope-2", userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := orch.Execute(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	rawNext, _ := revisionSnapshots(t, ctx, pool, appID, 2)
	next := parseSecretRefs(t, rawNext)
	if next["SHARED_ALL"] != (secretPick{scope: "APPLICATION", version: 11}) {
		t.Fatalf("new revision SHARED_ALL = %#v", next["SHARED_ALL"])
	}
	rawStill, _ := revisionSnapshots(t, ctx, pool, appID, 1)
	still := parseSecretRefs(t, rawStill)
	if still["SHARED_ALL"] != want["SHARED_ALL"] {
		t.Fatalf("original revision changed to %#v", still["SHARED_ALL"])
	}
}

type secretPick struct {
	scope   string
	version int
}

func revisionSnapshots(t *testing.T, ctx context.Context, pool *pgxpool.Pool, appID uuid.UUID, number int) ([]byte, []byte) {
	t.Helper()
	var refs, vars []byte
	if err := pool.QueryRow(ctx, `
		SELECT secret_refs, variable_snapshot FROM revisions
		WHERE application_id = $1 AND revision_number = $2`, appID, number).Scan(&refs, &vars); err != nil {
		t.Fatal(err)
	}
	return refs, vars
}

func parseSecretRefs(t *testing.T, raw []byte) map[string]secretPick {
	t.Helper()
	var items []struct {
		Name string `json:"name"`
		Ref  struct {
			Version int    `json:"version"`
			Scope   string `json:"scope"`
		} `json:"ref"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatal(err)
	}
	out := make(map[string]secretPick, len(items))
	for _, item := range items {
		out[item.Name] = secretPick{scope: item.Ref.Scope, version: item.Ref.Version}
	}
	return out
}
