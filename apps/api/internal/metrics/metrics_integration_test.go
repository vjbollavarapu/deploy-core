package metrics_test

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
		url = "postgres://localhost/deploycore_b21_test?sslmode=disable"
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
		DELETE FROM container_metric_snapshots;
		DELETE FROM server_metric_snapshots;
		DELETE FROM application_configs;
		DELETE FROM applications;
		DELETE FROM environments;
		DELETE FROM projects;
		DELETE FROM server_heartbeats;
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
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b21-metrics"))
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
		AgentRegistrationTTL:  15 * time.Minute,
		AgentHeartbeatRetain:  50,
		JobWorkerEnabled:      false,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return server.New(cfg, log, pool)
}

func TestServerAndContainerMetrics(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b21-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B21 Org", "b21-org-"+uuid.NewString()[:8])
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	serverID, appID := createApp(t, srv, ownerTok, orgID, projectID, envID)
	agentCred := registerAgent(t, srv, ownerTok, serverID)

	cpu := 42.5
	mem := int64(1024 * 1024 * 512)
	rx := int64(9000)
	tx := int64(8000)
	restarts := 2
	load1 := 0.75
	uptime := int64(3600)
	count := 3
	memTotal := int64(1024 * 1024 * 1024 * 8)

	ingest := mustJSON(map[string]any{
		"server": map[string]any{
			"cpuPercent": cpu, "memoryUsedBytes": mem, "memoryTotalBytes": memTotal,
			"diskUsedBytes": int64(100), "diskTotalBytes": int64(1000),
			"load1": load1, "load5": 0.5, "load15": 0.4,
			"uptimeSeconds": uptime, "networkRxBytes": rx, "networkTxBytes": tx,
			"containerCount": count,
		},
		"containers": []map[string]any{{
			"containerId": "abc123", "containerName": "api", "applicationId": appID,
			"cpuPercent": 11.0, "memoryUsedBytes": int64(64 * 1024 * 1024),
			"networkRxBytes": int64(10), "networkTxBytes": int64(20),
			"restartCount": restarts, "status": "running",
		}},
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/agents/metrics", ingest, agentCred)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("ingest status=%d body=%s", rec.Code, rec.Body.String())
	}

	get := doJSON(t, srv, http.MethodGet, "/api/v1/servers/"+serverID+"/metrics", nil, ownerTok)
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}
	var serverBody struct {
		Metrics struct {
			CPUPercent     *float64 `json:"cpuPercent"`
			NetworkRxBytes *int64   `json:"networkRxBytes"`
			ContainerCount *int     `json:"containerCount"`
			Source         string   `json:"source"`
			UptimeSeconds  *int64   `json:"uptimeSeconds"`
		} `json:"metrics"`
	}
	decode(t, get, &serverBody)
	if serverBody.Metrics.Source != "agent" || serverBody.Metrics.CPUPercent == nil || *serverBody.Metrics.CPUPercent != cpu {
		t.Fatalf("server metrics=%#v", serverBody.Metrics)
	}
	if serverBody.Metrics.NetworkRxBytes == nil || *serverBody.Metrics.NetworkRxBytes != rx {
		t.Fatalf("network=%v", serverBody.Metrics.NetworkRxBytes)
	}

	list := doJSON(t, srv, http.MethodGet, "/api/v1/servers/"+serverID+"/metrics/containers", nil, ownerTok)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var listBody struct {
		Containers []struct {
			ContainerID  string `json:"containerId"`
			Status       string `json:"status"`
			RestartCount *int   `json:"restartCount"`
		} `json:"containers"`
	}
	decode(t, list, &listBody)
	if len(listBody.Containers) != 1 || listBody.Containers[0].Status != "running" {
		t.Fatalf("containers=%#v", listBody.Containers)
	}

	appMet := doJSON(t, srv, http.MethodGet, "/api/v1/applications/"+appID+"/metrics", nil, ownerTok)
	if appMet.Code != http.StatusOK {
		t.Fatalf("app metrics status=%d body=%s", appMet.Code, appMet.Body.String())
	}

	series := doJSON(t, srv, http.MethodGet, "/api/v1/servers/"+serverID+"/metrics/series?metric=server_cpu_percent", nil, ownerTok)
	if series.Code != http.StatusOK {
		t.Fatalf("series status=%d body=%s", series.Code, series.Body.String())
	}
	var seriesBody struct {
		Series []struct {
			Metric string `json:"metric"`
			Points []any  `json:"points"`
		} `json:"series"`
	}
	decode(t, series, &seriesBody)
	if len(seriesBody.Series) == 0 || len(seriesBody.Series[0].Points) == 0 {
		t.Fatalf("expected time series points: %s", series.Body.String())
	}

	stranger := register(t, srv, "stranger-b21-"+uuid.NewString()+"@example.com", "password123", "X")
	deny := doJSON(t, srv, http.MethodGet, "/api/v1/servers/"+serverID+"/metrics", nil, stranger)
	if deny.Code != http.StatusForbidden && deny.Code != http.StatusNotFound {
		t.Fatalf("expected forbid got %d", deny.Code)
	}
}

func TestHeartbeatWarmsServerSnapshot(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b21h-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B21H Org", "b21h-"+uuid.NewString()[:8])
	srec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", mustJSON(map[string]any{
		"organizationId": orgID, "name": "edge-b21h", "provider": "hetzner",
		"region": "fsn1", "hostname": "edge-b21h-" + uuid.NewString()[:8] + ".local", "architecture": "amd64",
	}), ownerTok)
	if srec.Code != http.StatusCreated {
		t.Fatalf("server=%d %s", srec.Code, srec.Body.String())
	}
	var sOut struct {
		Server struct {
			ID string `json:"id"`
		} `json:"server"`
	}
	decode(t, srec, &sOut)
	cred := registerAgent(t, srv, ownerTok, sOut.Server.ID)

	hb := mustJSON(map[string]any{
		"agentVersion": "1.0.0", "dockerStatus": "ok",
		"cpuPercent": 10.0, "memoryUsedBytes": 100, "diskUsedBytes": 200,
		"load1": 0.1, "containerCount": 1, "uptimeSeconds": 99,
	})
	hrec := doJSON(t, srv, http.MethodPost, "/api/v1/agents/heartbeat", hb, cred)
	if hrec.Code != http.StatusOK && hrec.Code != http.StatusNoContent {
		t.Fatalf("heartbeat=%d %s", hrec.Code, hrec.Body.String())
	}

	get := doJSON(t, srv, http.MethodGet, "/api/v1/servers/"+sOut.Server.ID+"/metrics", nil, ownerTok)
	if get.Code != http.StatusOK {
		t.Fatalf("metrics=%d %s", get.Code, get.Body.String())
	}
	var body struct {
		Metrics struct {
			Source     string   `json:"source"`
			CPUPercent *float64 `json:"cpuPercent"`
		} `json:"metrics"`
	}
	decode(t, get, &body)
	if body.Metrics.Source != "heartbeat" || body.Metrics.CPUPercent == nil || *body.Metrics.CPUPercent != 10.0 {
		t.Fatalf("metrics=%#v", body.Metrics)
	}
}

func createApp(t *testing.T, srv *server.Server, token, orgID, projectID, envID string) (serverID, appID string) {
	t.Helper()
	srec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", mustJSON(map[string]any{
		"organizationId": orgID, "name": "edge-b21", "provider": "hetzner",
		"region": "fsn1", "hostname": "edge-b21-" + uuid.NewString()[:8] + ".local", "architecture": "amd64",
	}), token)
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
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/applications", mustJSON(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"name": "api", "slug": "api-" + uuid.NewString()[:8], "type": "API", "targetServerId": sOut.Server.ID,
		"config": map[string]any{
			"sourceType": "image", "imageReference": "ghcr.io/example/api:1", "internalPort": port,
		},
	}), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("app status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Application struct {
			ID string `json:"id"`
		} `json:"application"`
	}
	decode(t, rec, &out)
	return sOut.Server.ID, out.Application.ID
}

func registerAgent(t *testing.T, srv *server.Server, ownerTok, serverID string) string {
	t.Helper()
	tokRec := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/registration-token", nil, ownerTok)
	if tokRec.Code != http.StatusCreated {
		t.Fatalf("reg token status=%d body=%s", tokRec.Code, tokRec.Body.String())
	}
	var tokOut struct {
		RegistrationToken struct {
			Token string `json:"token"`
		} `json:"registrationToken"`
	}
	decode(t, tokRec, &tokOut)
	body, _ := json.Marshal(map[string]any{"registrationToken": tokOut.RegistrationToken.Token, "agentVersion": "1.0.0"})
	reg := doJSON(t, srv, http.MethodPost, "/api/v1/agents/register", body, "")
	if reg.Code != http.StatusCreated {
		t.Fatalf("register agent status=%d body=%s", reg.Code, reg.Body.String())
	}
	var out struct {
		Agent struct {
			Credential string `json:"credential"`
		} `json:"agent"`
	}
	decode(t, reg, &out)
	return out.Agent.Credential
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
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/organizations", mustJSON(map[string]string{"name": name, "slug": slug}), token)
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
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/projects",
		mustJSON(map[string]string{"organizationId": orgID, "name": "Platform", "slug": "platform-" + uuid.NewString()[:8]}), token)
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
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/projects/"+projectID+"/environments",
		mustJSON(map[string]string{"name": "Production", "slug": "production", "kind": "production"}), token)
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
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
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
