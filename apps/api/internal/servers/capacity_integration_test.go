package servers_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/jackc/pgx/v5/pgxpool"
)

func capacityPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b28_test?sslmode=disable"
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
		DELETE FROM deployment_events;
		DELETE FROM jobs;
		UPDATE deployments SET active_revision_id = NULL, target_revision_id = NULL;
		DELETE FROM revisions;
		DELETE FROM deployments;
		DELETE FROM application_configs;
		DELETE FROM applications;
		DELETE FROM environments;
		DELETE FROM projects;
		DELETE FROM server_agents;
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

func capacityHTTP(t *testing.T, pool *pgxpool.Pool) *server.Server {
	t.Helper()
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b28-capacity"))
	cfg := config.Config{
		Env:                   "test",
		AuthTokenSecret:       "test-secret-0123456789abcdef01234567",
		AccessTokenTTL:        time.Minute,
		RefreshTokenTTL:       time.Hour,
		AuthRateLimitPerMin:   1000,
		CORSAllowedOrigins:    []string{"*"},
		AuthMinPasswordLength: 8,
		SecretsPlatformKey:    key,
		SecretsKeyID:          "platform:v1",
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return server.New(cfg, log, pool)
}

func TestCapacityAllocationAndScheduler(t *testing.T) {
	pool := capacityPool(t)
	srv := capacityHTTP(t, pool)

	tok := register(t, srv, "owner-b28@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, tok, "B28 Org", "b28-org")
	projectID := createProjectB28(t, srv, tok, orgID)
	envID := createEnvB28(t, srv, tok, projectID)

	// Small server: 1 core / 1GiB
	smallID := createServerB28(t, srv, tok, orgID, "small", 1, 1<<30)
	// Large online server: 4 cores / 8GiB
	largeID := createServerB28(t, srv, tok, orgID, "large", 4, 8<<30)
	_, err := pool.Exec(context.Background(), `UPDATE servers SET status = 'ONLINE' WHERE id = $1`, largeID)
	if err != nil {
		t.Fatalf("mark online: %v", err)
	}

	// App targeting small with 500m / 512Mi — should succeed and allocate
	appID := createAppB28(t, srv, tok, orgID, projectID, envID, "web", smallID, 500, 512<<20)

	capRec := doJSON(t, srv, http.MethodGet, "/api/v1/servers/"+smallID+"/capacity", nil, tok)
	if capRec.Code != http.StatusOK {
		t.Fatalf("capacity status=%d body=%s", capRec.Code, capRec.Body.String())
	}
	var capOut struct {
		Capacity struct {
			CPUAllocatedMillis   int   `json:"cpuAllocatedMillis"`
			MemoryAllocatedBytes int64 `json:"memoryAllocatedBytes"`
			CPUAvailableMillis   int   `json:"cpuAvailableMillis"`
		} `json:"capacity"`
	}
	decode(t, capRec, &capOut)
	if capOut.Capacity.CPUAllocatedMillis != 500 {
		t.Fatalf("cpu allocated=%d want 500", capOut.Capacity.CPUAllocatedMillis)
	}
	if capOut.Capacity.MemoryAllocatedBytes != 512<<20 {
		t.Fatalf("mem allocated=%d", capOut.Capacity.MemoryAllocatedBytes)
	}
	if capOut.Capacity.CPUAvailableMillis != 500 {
		t.Fatalf("cpu available=%d want 500", capOut.Capacity.CPUAvailableMillis)
	}

	// Oversubscribe small → INSUFFICIENT_RESOURCES
	overBody, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"name": "too-big", "slug": "too-big", "type": "API", "targetServerId": smallID,
		"config": map[string]any{
			"sourceType": "image", "imageReference": "ghcr.io/example/x:1",
			"internalPort": 8080, "cpuLimitMillis": 600, "memoryLimitBytes": 100,
		},
	})
	overRec := doJSON(t, srv, http.MethodPost, "/api/v1/applications", overBody, tok)
	if overRec.Code != http.StatusConflict {
		t.Fatalf("oversubscribe status=%d body=%s", overRec.Code, overRec.Body.String())
	}
	var errBody struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decode(t, overRec, &errBody)
	if errBody.Error.Code != "INSUFFICIENT_RESOURCES" {
		t.Fatalf("code=%s", errBody.Error.Code)
	}

	// Scheduler preview picks large ONLINE host
	prevBody, _ := json.Marshal(map[string]any{
		"organizationId": orgID,
		"cpuMillis":      1000,
		"memoryBytes":    1 << 30,
		"policy":         map[string]any{"mode": "scheduler"},
	})
	prevRec := doJSON(t, srv, http.MethodPost, "/api/v1/placement/preview", prevBody, tok)
	if prevRec.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", prevRec.Code, prevRec.Body.String())
	}
	var prevOut struct {
		Placement struct {
			ServerID string `json:"serverId"`
			Reason   string `json:"reason"`
		} `json:"placement"`
	}
	decode(t, prevRec, &prevOut)
	if prevOut.Placement.ServerID != largeID {
		t.Fatalf("preview server=%s want %s", prevOut.Placement.ServerID, largeID)
	}

	// Scheduler app with no target → deploy binds large
	schedApp := createSchedulerAppB28(t, srv, tok, orgID, projectID, envID, "sched", 1000, 1<<30)
	depBody, _ := json.Marshal(map[string]any{"trigger": "manual"})
	depRec := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+schedApp+"/deployments", depBody, tok)
	if depRec.Code != http.StatusCreated {
		t.Fatalf("deploy status=%d body=%s", depRec.Code, depRec.Body.String())
	}
	var depOut struct {
		Deployment struct {
			ServerID *string `json:"serverId"`
		} `json:"deployment"`
	}
	decode(t, depRec, &depOut)
	if depOut.Deployment.ServerID == nil || *depOut.Deployment.ServerID != largeID {
		t.Fatalf("deploy server=%v want %s", depOut.Deployment.ServerID, largeID)
	}

	_ = appID
}

func createServerB28(t *testing.T, srv *server.Server, token, orgID, name string, cores int, mem int64) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "name": name, "provider": "hetzner",
		"region": "fsn1", "hostname": name + ".local", "architecture": "amd64",
		"cpuCores": cores, "memoryBytes": mem, "diskBytes": 50 << 30,
		"labels": map[string]string{"tier": "prod"},
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create server status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Server struct {
			ID string `json:"id"`
		} `json:"server"`
	}
	decode(t, rec, &out)
	return out.Server.ID
}

func createAppB28(t *testing.T, srv *server.Server, token, orgID, projectID, envID, slug, serverID string, cpu int, mem int64) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"name": slug, "slug": slug, "type": "API", "targetServerId": serverID,
		"config": map[string]any{
			"sourceType": "image", "imageReference": "ghcr.io/example/" + slug + ":1",
			"internalPort": 8080, "cpuLimitMillis": cpu, "memoryLimitBytes": mem,
		},
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/applications", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create app status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Application struct {
			ID string `json:"id"`
		} `json:"application"`
	}
	decode(t, rec, &out)
	return out.Application.ID
}

func createSchedulerAppB28(t *testing.T, srv *server.Server, token, orgID, projectID, envID, slug string, cpu int, mem int64) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"name": slug, "slug": slug, "type": "API",
		"placementPolicy": map[string]any{"mode": "scheduler"},
		"config": map[string]any{
			"sourceType": "image", "imageReference": "ghcr.io/example/" + slug + ":1",
			"internalPort": 8080, "cpuLimitMillis": cpu, "memoryLimitBytes": mem,
		},
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/applications", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create scheduler app status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Application struct {
			ID string `json:"id"`
		} `json:"application"`
	}
	decode(t, rec, &out)
	return out.Application.ID
}

func createProjectB28(t *testing.T, srv *server.Server, token, orgID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"organizationId": orgID, "name": "B28", "slug": "b28"})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/projects", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	decode(t, rec, &out)
	return out.Project.ID
}

func createEnvB28(t *testing.T, srv *server.Server, token, projectID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": "prod", "slug": "prod", "kind": "production"})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/projects/"+projectID+"/environments", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create env status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Environment struct {
			ID string `json:"id"`
		} `json:"environment"`
	}
	decode(t, rec, &out)
	return out.Environment.ID
}
