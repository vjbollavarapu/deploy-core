package volumes_test

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
	"github.com/deploycore/deploy-core/apps/api/internal/volumes"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b23_test?sslmode=disable"
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
		DELETE FROM volumes;
		DELETE FROM managed_databases;
		DELETE FROM agent_commands;
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
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b23-volumes"))
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

func TestVolumeLifecycleAndDeleteProtection(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b23-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B23 Org", "b23-org-"+uuid.NewString()[:8])
	serverID := createServer(t, srv, ownerTok, orgID)
	agentCred := registerAgent(t, srv, ownerTok, serverID)

	create := doJSON(t, srv, http.MethodPost, "/api/v1/volumes", mustJSON(map[string]any{
		"organizationId": orgID, "serverId": serverID, "name": "app-data", "driver": "local",
		"mountPath": "/data",
	}), ownerTok)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Volume struct {
			ID        string `json:"id"`
			State     string `json:"state"`
			Protected bool   `json:"protected"`
			Labels    map[string]any `json:"labels"`
			LastCommandID *string `json:"lastCommandId"`
		} `json:"volume"`
	}
	decode(t, create, &created)
	if created.Volume.State != volumes.StateCreating {
		t.Fatalf("state=%s", created.Volume.State)
	}
	if created.Volume.Labels["deploycore.managed"] != true {
		t.Fatalf("labels=%v", created.Volume.Labels)
	}
	if created.Volume.LastCommandID == nil {
		t.Fatal("expected lastCommandId")
	}

	done := doJSON(t, srv, http.MethodPost, "/api/v1/agents/commands/"+*created.Volume.LastCommandID+"/status",
		mustJSON(map[string]any{"status": "completed", "result": map[string]any{"dockerName": "app-data", "usageBytes": 42}}), agentCred)
	if done.Code != http.StatusOK {
		t.Fatalf("complete status=%d body=%s", done.Code, done.Body.String())
	}
	get := doJSON(t, srv, http.MethodGet, "/api/v1/volumes/"+created.Volume.ID, nil, ownerTok)
	var ready struct {
		Volume struct {
			State      string `json:"state"`
			DockerName *string `json:"dockerName"`
			UsageBytes *int64  `json:"usageBytes"`
		} `json:"volume"`
	}
	decode(t, get, &ready)
	if ready.Volume.State != volumes.StateReady || ready.Volume.DockerName == nil || *ready.Volume.DockerName != "app-data" {
		t.Fatalf("ready=%#v", ready.Volume)
	}

	// Create app to attach to.
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	appID := createApp(t, srv, ownerTok, orgID, projectID, envID, serverID)

	attach := doJSON(t, srv, http.MethodPost, "/api/v1/volumes/"+created.Volume.ID+"/attach", mustJSON(map[string]any{
		"resourceType": "application", "resourceId": appID, "mountPath": "/var/lib/app",
	}), ownerTok)
	if attach.Code != http.StatusOK {
		t.Fatalf("attach status=%d body=%s", attach.Code, attach.Body.String())
	}

	delAttached := doJSON(t, srv, http.MethodDelete, "/api/v1/volumes/"+created.Volume.ID, nil, ownerTok)
	if delAttached.Code != http.StatusConflict {
		t.Fatalf("expected conflict deleting attached, got %d %s", delAttached.Code, delAttached.Body.String())
	}

	detach := doJSON(t, srv, http.MethodPost, "/api/v1/volumes/"+created.Volume.ID+"/detach", nil, ownerTok)
	if detach.Code != http.StatusOK {
		t.Fatalf("detach status=%d body=%s", detach.Code, detach.Body.String())
	}

	del := doJSON(t, srv, http.MethodDelete, "/api/v1/volumes/"+created.Volume.ID, nil, ownerTok)
	if del.Code != http.StatusAccepted {
		t.Fatalf("delete status=%d body=%s", del.Code, del.Body.String())
	}
	var deleting struct {
		Volume struct {
			State         string  `json:"state"`
			LastCommandID *string `json:"lastCommandId"`
		} `json:"volume"`
	}
	getDel := doJSON(t, srv, http.MethodGet, "/api/v1/volumes/"+created.Volume.ID, nil, ownerTok)
	decode(t, getDel, &deleting)
	if deleting.Volume.State != volumes.StateDeleting || deleting.Volume.LastCommandID == nil {
		t.Fatalf("deleting=%#v", deleting.Volume)
	}
	_ = doJSON(t, srv, http.MethodPost, "/api/v1/agents/commands/"+*deleting.Volume.LastCommandID+"/status",
		mustJSON(map[string]any{"status": "completed", "result": map[string]any{}}), agentCred)
	gone := doJSON(t, srv, http.MethodGet, "/api/v1/volumes/"+created.Volume.ID, nil, ownerTok)
	if gone.Code != http.StatusNotFound {
		t.Fatalf("expected not found after remove, got %d", gone.Code)
	}
}

func TestDatabaseCriticalVolumeProtected(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b23d-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B23D Org", "b23d-"+uuid.NewString()[:8])
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	serverID := createServer(t, srv, ownerTok, orgID)

	createDB := doJSON(t, srv, http.MethodPost, "/api/v1/databases", mustJSON(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"serverId": serverID, "name": "Primary", "databaseName": "appdb", "username": "appuser",
		"password": "supersecret1", "storageVolume": "db-primary-data",
	}), ownerTok)
	if createDB.Code != http.StatusCreated {
		t.Fatalf("db create=%d %s", createDB.Code, createDB.Body.String())
	}

	list := doJSON(t, srv, http.MethodGet, "/api/v1/volumes?organizationId="+orgID+"&serverId="+serverID, nil, ownerTok)
	if list.Code != http.StatusOK {
		t.Fatalf("list=%d %s", list.Code, list.Body.String())
	}
	var body struct {
		Items []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Protected bool   `json:"protected"`
			State     string `json:"state"`
		} `json:"items"`
	}
	decode(t, list, &body)
	if len(body.Items) != 1 || body.Items[0].Name != "db-primary-data" || !body.Items[0].Protected {
		t.Fatalf("items=%#v", body.Items)
	}

	del := doJSON(t, srv, http.MethodDelete, "/api/v1/volumes/"+body.Items[0].ID, nil, ownerTok)
	if del.Code != http.StatusConflict {
		t.Fatalf("expected protected conflict, got %d %s", del.Code, del.Body.String())
	}
}

func createServer(t *testing.T, srv *server.Server, token, orgID string) string {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", mustJSON(map[string]any{
		"organizationId": orgID, "name": "vol-host", "provider": "hetzner",
		"region": "fsn1", "hostname": "vol-" + uuid.NewString()[:8] + ".local", "architecture": "amd64",
	}), token)
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
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/applications", mustJSON(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"name": "api", "slug": "api-" + uuid.NewString()[:8], "type": "API", "targetServerId": serverID,
		"config": map[string]any{"sourceType": "image", "imageReference": "ghcr.io/example/api:1", "internalPort": 8080},
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
	return out.Application.ID
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
