package databases_test

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
	"github.com/deploycore/deploy-core/apps/api/internal/databases"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b22_test?sslmode=disable"
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
		DELETE FROM managed_databases;
		DELETE FROM agent_commands;
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
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b22-databases"))
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

func TestManagedDatabaseProvisionFlow(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b22-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B22 Org", "b22-org-"+uuid.NewString()[:8])
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	serverID := createServer(t, srv, ownerTok, orgID)
	agentCred := registerAgent(t, srv, ownerTok, serverID)

	cpu := 500
	mem := int64(512 * 1024 * 1024)
	create := doJSON(t, srv, http.MethodPost, "/api/v1/databases", mustJSON(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"serverId": serverID, "name": "Primary PG", "engine": "postgresql", "engineVersion": "16",
		"databaseName": "appdb", "username": "appuser", "password": "supersecret1",
		"cpuMillis": cpu, "memoryBytes": mem,
		"backupPolicy": map[string]any{"enabled": true, "retentionDays": 14},
	}), ownerTok)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Database struct {
			ID                 string  `json:"id"`
			Status             string  `json:"status"`
			StorageVolumeName  string  `json:"storageVolumeName"`
			VolumeProtected    bool    `json:"volumeProtected"`
			HasCredential      bool    `json:"hasCredential"`
			ProvisionCommandID *string `json:"provisionCommandId"`
		} `json:"database"`
	}
	decode(t, create, &created)
	if created.Database.Status != databases.StatusProvisioning {
		t.Fatalf("status=%s", created.Database.Status)
	}
	if !created.Database.VolumeProtected || !created.Database.HasCredential {
		t.Fatalf("db=%#v", created.Database)
	}
	if created.Database.ProvisionCommandID == nil {
		t.Fatal("expected provisionCommandId")
	}

	// Password must not appear in agent command payload.
	var payload []byte
	if err := pool.QueryRow(context.Background(), `
		SELECT payload FROM agent_commands WHERE id = $1`, *created.Database.ProvisionCommandID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("supersecret1")) {
		t.Fatalf("password leaked into command payload: %s", payload)
	}

	boot := doJSON(t, srv, http.MethodGet, "/api/v1/agents/databases/"+created.Database.ID+"/bootstrap", nil, agentCred)
	if boot.Code != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%s", boot.Code, boot.Body.String())
	}
	var bootBody struct {
		Password          string `json:"password"`
		StorageVolumeName string `json:"storageVolumeName"`
	}
	decode(t, boot, &bootBody)
	if bootBody.Password != "supersecret1" {
		t.Fatalf("password=%q", bootBody.Password)
	}

	// Complete provision via agent command status.
	statusBody := mustJSON(map[string]any{
		"status": "completed",
		"result": map[string]any{"containerRuntimeId": "ctr-pg-1"},
	})
	done := doJSON(t, srv, http.MethodPost, "/api/v1/agents/commands/"+*created.Database.ProvisionCommandID+"/status", statusBody, agentCred)
	if done.Code != http.StatusOK {
		t.Fatalf("command status=%d body=%s", done.Code, done.Body.String())
	}

	get := doJSON(t, srv, http.MethodGet, "/api/v1/databases/"+created.Database.ID, nil, ownerTok)
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}
	var got struct {
		Database struct {
			Status             string  `json:"status"`
			ContainerRuntimeID *string `json:"containerRuntimeId"`
		} `json:"database"`
	}
	decode(t, get, &got)
	if got.Database.Status != databases.StatusRunning {
		t.Fatalf("status=%s", got.Database.Status)
	}
	if got.Database.ContainerRuntimeID == nil || *got.Database.ContainerRuntimeID != "ctr-pg-1" {
		t.Fatalf("runtime=%v", got.Database.ContainerRuntimeID)
	}

	reveal := doJSON(t, srv, http.MethodPost, "/api/v1/databases/"+created.Database.ID+"/credentials/reveal", nil, ownerTok)
	if reveal.Code != http.StatusOK {
		t.Fatalf("reveal status=%d body=%s", reveal.Code, reveal.Body.String())
	}

	del := doJSON(t, srv, http.MethodDelete, "/api/v1/databases/"+created.Database.ID, nil, ownerTok)
	if del.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", del.Code, del.Body.String())
	}
	var delBody struct {
		RuntimeStopped bool   `json:"runtimeStopped"`
		VolumeDeleted  bool   `json:"volumeDeleted"`
		Message        string `json:"message"`
	}
	decode(t, del, &delBody)
	if delBody.RuntimeStopped || delBody.VolumeDeleted {
		t.Fatalf("delete must not claim runtime/volume destruction: %+v", delBody)
	}
	// Volume name row soft-deleted; protected volume must not be auto-removed (no volume table yet).
	var vol string
	var status string
	err := pool.QueryRow(context.Background(), `
		SELECT storage_volume_name, status FROM managed_databases WHERE id = $1`, created.Database.ID).
		Scan(&vol, &status)
	if err != nil {
		t.Fatal(err)
	}
	if status != databases.StatusDeleted || vol == "" {
		t.Fatalf("after delete status=%s vol=%s", status, vol)
	}

	_ = protocol.OpProvisionDatabase
}

func createServer(t *testing.T, srv *server.Server, token, orgID string) string {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", mustJSON(map[string]any{
		"organizationId": orgID, "name": "db-host", "provider": "hetzner",
		"region": "fsn1", "hostname": "db-host-" + uuid.NewString()[:8] + ".local", "architecture": "amd64",
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
