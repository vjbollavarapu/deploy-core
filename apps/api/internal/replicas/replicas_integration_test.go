package replicas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

func testPool(t *testing.T) *pgxpool.Pool {
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
	if err := rbac.EnsureSeeded(ctx, pool); err != nil {
		t.Fatalf("rbac seed: %v", err)
	}
	_, _ = pool.Exec(ctx, `
		DELETE FROM application_replicas;
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

func TestReplicaScaleAndSummary(t *testing.T) {
	pool := testPool(t)
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b29-replicas!!"))
	cfg := config.Config{
		Env:                       "test",
		AuthTokenSecret:           "test-secret-0123456789abcdef01234567",
		AccessTokenTTL:            time.Minute,
		RefreshTokenTTL:           time.Hour,
		AuthRateLimitPerMin:       1000,
		CORSAllowedOrigins:        []string{"*"},
		AuthMinPasswordLength:     8,
		SecretsPlatformKey:        key,
		SecretsKeyID:              "platform:v1",
		OrchestratorSimulateAgent: true,
	}
	srv := server.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), pool)

	tok := register(t, srv, "owner-b29@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, tok, "B29 Org", "b29-org")
	projectID := createProject(t, srv, tok, orgID)
	envID := createEnv(t, srv, tok, projectID)
	serverID := createServer(t, srv, tok, orgID)
	appID := createApp(t, srv, tok, orgID, projectID, envID, serverID)

	sumRec := doJSON(t, srv, http.MethodGet, "/api/v1/applications/"+appID+"/replicas", nil, tok)
	if sumRec.Code != http.StatusOK {
		t.Fatalf("summary status=%d body=%s", sumRec.Code, sumRec.Body.String())
	}
	var sumOut struct {
		Summary struct {
			DesiredReplicas int `json:"desiredReplicas"`
		} `json:"summary"`
	}
	decode(t, sumRec, &sumOut)
	if sumOut.Summary.DesiredReplicas != 1 {
		t.Fatalf("desired=%d", sumOut.Summary.DesiredReplicas)
	}

	scaleBody, _ := json.Marshal(map[string]any{"desiredReplicas": 2})
	scaleRec := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/replicas/scale", scaleBody, tok)
	if scaleRec.Code != http.StatusOK {
		t.Fatalf("scale status=%d body=%s", scaleRec.Code, scaleRec.Body.String())
	}
	decode(t, scaleRec, &sumOut)
	if sumOut.Summary.DesiredReplicas != 2 {
		t.Fatalf("after scale desired=%d", sumOut.Summary.DesiredReplicas)
	}

	// Over-capacity scale should fail (server has 1 core = 1000m; 3*500=1500).
	overBody, _ := json.Marshal(map[string]any{"desiredReplicas": 3})
	overRec := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/replicas/scale", overBody, tok)
	if overRec.Code != http.StatusConflict {
		t.Fatalf("over scale status=%d body=%s", overRec.Code, overRec.Body.String())
	}
}

func register(t *testing.T, srv *server.Server, email, password, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password, "displayName": name})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/auth/register", body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Tokens struct {
			AccessToken string `json:"accessToken"`
		} `json:"tokens"`
	}
	decode(t, rec, &out)
	return out.Tokens.AccessToken
}

func createOrg(t *testing.T, srv *server.Server, token, name, slug string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": name, "slug": slug})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/organizations", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create org status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Organization struct {
			ID string `json:"id"`
		} `json:"organization"`
	}
	decode(t, rec, &out)
	return out.Organization.ID
}

func createProject(t *testing.T, srv *server.Server, token, orgID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"organizationId": orgID, "name": "B29", "slug": "b29"})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/projects", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("project status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	decode(t, rec, &out)
	return out.Project.ID
}

func createEnv(t *testing.T, srv *server.Server, token, projectID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": "prod", "slug": "prod", "kind": "production"})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/projects/"+projectID+"/environments", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("env status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Environment struct {
			ID string `json:"id"`
		} `json:"environment"`
	}
	decode(t, rec, &out)
	return out.Environment.ID
}

func createServer(t *testing.T, srv *server.Server, token, orgID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "name": "edge-b29", "provider": "hetzner",
		"region": "fsn1", "hostname": "edge-b29.local", "architecture": "amd64",
		"cpuCores": 1, "memoryBytes": 4 << 30, "diskBytes": 50 << 30,
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("server status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Server struct {
			ID string `json:"id"`
		} `json:"server"`
	}
	decode(t, rec, &out)
	return out.Server.ID
}

func createApp(t *testing.T, srv *server.Server, token, orgID, projectID, envID, serverID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"name": "api", "slug": "api", "type": "API", "targetServerId": serverID,
		"config": map[string]any{
			"sourceType": "image", "imageReference": "ghcr.io/example/api:1",
			"internalPort": 8080, "cpuLimitMillis": 500, "memoryLimitBytes": 256 << 20,
			"runtimeConfig": map[string]any{"desiredReplicas": 1},
		},
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/applications", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("app status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Application struct {
			ID string `json:"id"`
		} `json:"application"`
	}
	decode(t, rec, &out)
	return out.Application.ID
}

func doJSON(t *testing.T, srv *server.Server, method, path string, body []byte, access string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
	rec := httptest.NewRecorder()
	srv.HTTPHandler().ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
}
