package healthchecks_test

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

	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/healthchecks"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b19_test?sslmode=disable"
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
		DELETE FROM health_probe_samples;
		DELETE FROM application_health;
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

func testServer(t *testing.T, pool *pgxpool.Pool) *server.Server {
	t.Helper()
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b19-health"))
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

func TestHealthPolicyAggregationAndAPI(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b19-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B19 Org", "b19-org-"+uuid.NewString()[:8])
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	appID := createApplicationWithHealth(t, srv, ownerTok, orgID, projectID, envID)

	get := doJSON(t, srv, http.MethodGet, "/api/v1/applications/"+appID+"/health", nil, ownerTok)
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}
	var healthBody struct {
		Health struct {
			State string `json:"state"`
		} `json:"health"`
		Policy struct {
			Type    string `json:"type"`
			Retries int    `json:"retries"`
			Path    string `json:"path"`
		} `json:"policy"`
	}
	decode(t, get, &healthBody)
	if healthBody.Health.State != healthchecks.StateUnknown {
		t.Fatalf("state=%s", healthBody.Health.State)
	}
	if healthBody.Policy.Type != healthchecks.TypeHTTP || healthBody.Policy.Retries != 2 || healthBody.Policy.Path != "/readyz" {
		t.Fatalf("policy=%#v", healthBody.Policy)
	}

	// Two successes → HEALTHY (retries=2).
	for i := 0; i < 2; i++ {
		rec := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/health/probes",
			mustJSON(map[string]any{"success": true, "message": "ok", "latencyMs": 12}), ownerTok)
		if rec.Code != http.StatusOK {
			t.Fatalf("probe status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	get2 := doJSON(t, srv, http.MethodGet, "/api/v1/applications/"+appID+"/health", nil, ownerTok)
	var after struct {
		Health struct {
			State                string `json:"state"`
			ConsecutiveSuccesses int    `json:"consecutiveSuccesses"`
		} `json:"health"`
	}
	decode(t, get2, &after)
	if after.Health.State != healthchecks.StateHealthy || after.Health.ConsecutiveSuccesses != 2 {
		t.Fatalf("after=%#v", after.Health)
	}

	samples := doJSON(t, srv, http.MethodGet, "/api/v1/applications/"+appID+"/health/samples", nil, ownerTok)
	if samples.Code != http.StatusOK {
		t.Fatalf("samples status=%d", samples.Code)
	}
	var sampleBody struct {
		Samples []any `json:"samples"`
	}
	decode(t, samples, &sampleBody)
	if len(sampleBody.Samples) == 0 {
		t.Fatal("expected retained samples")
	}

	// Failures flip to UNHEALTHY after threshold.
	for i := 0; i < 2; i++ {
		rec := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/health/probes",
			mustJSON(map[string]any{"success": false, "message": "boom"}), ownerTok)
		if rec.Code != http.StatusOK {
			t.Fatalf("fail probe=%d", rec.Code)
		}
	}
	get3 := doJSON(t, srv, http.MethodGet, "/api/v1/applications/"+appID+"/health", nil, ownerTok)
	var unhealthy struct {
		Health struct {
			State string `json:"state"`
		} `json:"health"`
	}
	decode(t, get3, &unhealthy)
	if unhealthy.Health.State != healthchecks.StateUnhealthy {
		t.Fatalf("state=%s", unhealthy.Health.State)
	}

	var sampleCount int
	_ = pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM health_probe_samples WHERE application_id = $1`, appID).Scan(&sampleCount)
	if sampleCount > healthchecks.MaxSamplesRetained {
		t.Fatalf("sampleCount=%d exceeds retention", sampleCount)
	}
}

func createApplicationWithHealth(t *testing.T, srv *server.Server, token, orgID, projectID, envID string) string {
	t.Helper()
	srvBody, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "name": "edge-b19", "provider": "hetzner",
		"region": "fsn1", "hostname": "edge-b19.local", "architecture": "amd64",
	})
	srec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", srvBody, token)
	if srec.Code != http.StatusCreated {
		t.Fatalf("server status=%d body=%s", srec.Code, srec.Body.String())
	}
	var sOut struct {
		Server struct {
			ID string `json:"id"`
		} `json:"server"`
	}
	decode(t, srec, &sOut)
	port := 8080
	body, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"name": "api", "slug": "api", "type": "API", "targetServerId": sOut.Server.ID,
		"config": map[string]any{
			"sourceType": "image", "imageReference": "ghcr.io/example/api:1", "internalPort": port,
			"healthCheck": map[string]any{
				"type": "HTTP", "path": "/readyz", "port": port, "retries": 2,
				"intervalSeconds": 5, "timeoutSeconds": 2, "expectedStatus": 200,
			},
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

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func register(t *testing.T, srv *server.Server, email, password, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password, "displayName": name})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/auth/register", body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s status=%d body=%s", email, rec.Code, rec.Body.String())
	}
	var out struct {
		Tokens auth.TokenPair `json:"tokens"`
	}
	decode(t, rec, &out)
	return out.Tokens.AccessToken
}

func createOrg(t *testing.T, srv *server.Server, token, name, slug string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name, "slug": slug})
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
	body, _ := json.Marshal(map[string]string{"organizationId": orgID, "name": "Platform", "slug": "platform"})
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

func createEnvironment(t *testing.T, srv *server.Server, token, projectID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": "Production", "slug": "production", "kind": "production"})
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

func doJSON(t *testing.T, srv *server.Server, method, path string, body []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
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
