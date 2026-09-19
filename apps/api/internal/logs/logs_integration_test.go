package logs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
		url = "postgres://localhost/deploycore_b20_test?sslmode=disable"
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
		DELETE FROM deployments;
		DELETE FROM revisions;
		DELETE FROM application_configs;
		DELETE FROM applications;
		DELETE FROM environments;
		DELETE FROM projects;
		DELETE FROM agent_commands;
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
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b20-logs"))
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

func TestLogIngestAndSnapshot(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b20-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B20 Org", "b20-org-"+uuid.NewString()[:8])
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	serverID, appID := createApp(t, srv, ownerTok, orgID, projectID, envID)
	agentCred := registerAgent(t, srv, ownerTok, serverID)

	ingestBody, _ := json.Marshal(map[string]any{
		"kind": "runtime", "applicationId": appID,
		"entries": []map[string]any{
			{"stream": "stdout", "message": "hello from container"},
			{"stream": "stderr", "message": "warn line"},
		},
	})
	ing := doJSON(t, srv, http.MethodPost, "/api/v1/agents/logs", ingestBody, agentCred)
	if ing.Code != http.StatusAccepted {
		t.Fatalf("ingest status=%d body=%s", ing.Code, ing.Body.String())
	}

	snap := doJSON(t, srv, http.MethodGet, "/api/v1/applications/"+appID+"/logs?kind=runtime&follow=false", nil, ownerTok)
	if snap.Code != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", snap.Code, snap.Body.String())
	}
	var body struct {
		Entries []struct {
			Stream  string `json:"stream"`
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"entries"`
	}
	decode(t, snap, &body)
	if len(body.Entries) != 2 {
		t.Fatalf("entries=%d body=%s", len(body.Entries), snap.Body.String())
	}
	if body.Entries[0].Message != "hello from container" || body.Entries[1].Stream != "stderr" {
		t.Fatalf("entries=%#v", body.Entries)
	}

	// Org isolation: stranger cannot read.
	stranger := register(t, srv, "stranger-b20-"+uuid.NewString()+"@example.com", "password123", "X")
	deny := doJSON(t, srv, http.MethodGet, "/api/v1/applications/"+appID+"/logs?kind=runtime&follow=false", nil, stranger)
	if deny.Code != http.StatusForbidden && deny.Code != http.StatusNotFound {
		t.Fatalf("expected forbid, got %d %s", deny.Code, deny.Body.String())
	}

	// SSE follow receives ready + live line.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/applications/"+appID+"/logs?kind=runtime&follow=true&cursor=", nil)
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.HTTPHandler().ServeHTTP(w, req)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(w.Body.String(), "event: ready") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(w.Body.String(), "event: ready") {
		cancel()
		<-done
		t.Fatalf("missing ready event: %s", w.Body.String())
	}

	live, _ := json.Marshal(map[string]any{
		"kind": "runtime", "applicationId": appID,
		"entries": []map[string]any{{"stream": "stdout", "message": "live-line"}},
	})
	liveRec := doJSON(t, srv, http.MethodPost, "/api/v1/agents/logs", live, agentCred)
	if liveRec.Code != http.StatusAccepted {
		cancel()
		<-done
		t.Fatalf("live ingest=%d", liveRec.Code)
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(w.Body.String(), "live-line") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	if !strings.Contains(w.Body.String(), "live-line") {
		t.Fatalf("SSE missing live line: %s", w.Body.String())
	}
}

func TestDeploymentEventsStreamSnapshot(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b20e-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B20E Org", "b20e-"+uuid.NewString()[:8])
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	_, appID := createApp(t, srv, ownerTok, orgID, projectID, envID)

	depRec := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/deployments",
		mustJSON(map[string]any{"trigger": "manual"}), ownerTok)
	if depRec.Code != http.StatusCreated && depRec.Code != http.StatusOK {
		t.Fatalf("deploy status=%d body=%s", depRec.Code, depRec.Body.String())
	}
	var depOut struct {
		Deployment struct {
			ID string `json:"id"`
		} `json:"deployment"`
	}
	decode(t, depRec, &depOut)

	snap := doJSON(t, srv, http.MethodGet, "/api/v1/deployments/"+depOut.Deployment.ID+"/events/stream?follow=false", nil, ownerTok)
	if snap.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", snap.Code, snap.Body.String())
	}
	var body struct {
		Entries []struct {
			Kind    string `json:"kind"`
			Stream  string `json:"stream"`
			Message string `json:"message"`
		} `json:"entries"`
		Kind string `json:"kind"`
	}
	decode(t, snap, &body)
	if body.Kind != "deployment_events" || len(body.Entries) == 0 {
		t.Fatalf("body=%s", snap.Body.String())
	}
}

func createApp(t *testing.T, srv *server.Server, token, orgID, projectID, envID string) (serverID, appID string) {
	t.Helper()
	srec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", mustJSON(map[string]any{
		"organizationId": orgID, "name": "edge-b20", "provider": "hetzner",
		"region": "fsn1", "hostname": "edge-b20-" + uuid.NewString()[:8] + ".local", "architecture": "amd64",
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
