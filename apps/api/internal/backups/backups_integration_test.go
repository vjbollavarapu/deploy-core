package backups_test

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
	"github.com/deploycore/deploy-core/apps/api/internal/backups"
	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/databases"
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
		url = "postgres://localhost/deploycore_b24_test?sslmode=disable"
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
		DELETE FROM restore_operations;
		DELETE FROM backups;
		DELETE FROM volumes;
		DELETE FROM managed_databases;
		DELETE FROM agent_commands;
		DELETE FROM jobs;
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
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b24-backups"))
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

func TestBackupAndRestoreFlow(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b24-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B24 Org", "b24-org-"+uuid.NewString()[:8])
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	serverID := createServer(t, srv, ownerTok, orgID)
	agentCred := registerAgent(t, srv, ownerTok, serverID)

	dbRec := doJSON(t, srv, http.MethodPost, "/api/v1/databases", mustJSON(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"serverId": serverID, "name": "Primary", "databaseName": "appdb", "username": "appuser",
		"password": "supersecret1",
	}), ownerTok)
	if dbRec.Code != http.StatusCreated {
		t.Fatalf("db create=%d %s", dbRec.Code, dbRec.Body.String())
	}
	var dbOut struct {
		Database struct {
			ID string `json:"id"`
		} `json:"database"`
	}
	decode(t, dbRec, &dbOut)
	// Mark running so backup is allowed.
	if _, err := pool.Exec(context.Background(), `
		UPDATE managed_databases SET status = $2 WHERE id = $1`, dbOut.Database.ID, databases.StatusRunning); err != nil {
		t.Fatal(err)
	}

	bak := doJSON(t, srv, http.MethodPost, "/api/v1/databases/"+dbOut.Database.ID+"/backups",
		mustJSON(map[string]any{"destination": "local", "retentionDays": 3}), ownerTok)
	if bak.Code != http.StatusAccepted {
		t.Fatalf("backup create=%d %s", bak.Code, bak.Body.String())
	}
	var bakOut struct {
		Backup struct {
			ID             string  `json:"id"`
			Status         string  `json:"status"`
			DestinationURI string  `json:"destinationUri"`
			CommandID      *string `json:"commandId"`
			JobID          *string `json:"jobId"`
		} `json:"backup"`
	}
	decode(t, bak, &bakOut)
	if bakOut.Backup.Status != backups.StatusRunning || bakOut.Backup.CommandID == nil || bakOut.Backup.JobID == nil {
		t.Fatalf("backup=%#v", bakOut.Backup)
	}
	if bakOut.Backup.DestinationURI == "" || bakOut.Backup.DestinationURI[0] == '/' {
		t.Fatalf("destinationUri=%s", bakOut.Backup.DestinationURI)
	}

	// Complete without checksum → failed.
	bad := doJSON(t, srv, http.MethodPost, "/api/v1/agents/commands/"+*bakOut.Backup.CommandID+"/status",
		mustJSON(map[string]any{"status": "completed", "result": map[string]any{"sizeBytes": 10}}), agentCred)
	if bad.Code != http.StatusOK {
		t.Fatalf("bad complete=%d %s", bad.Code, bad.Body.String())
	}
	getBad := doJSON(t, srv, http.MethodGet, "/api/v1/backups/"+bakOut.Backup.ID, nil, ownerTok)
	var failed struct {
		Backup struct {
			Status string `json:"status"`
		} `json:"backup"`
	}
	decode(t, getBad, &failed)
	if failed.Backup.Status != backups.StatusFailed {
		t.Fatalf("expected failed without checksum, got %s", failed.Backup.Status)
	}

	// Fresh successful backup.
	bak2 := doJSON(t, srv, http.MethodPost, "/api/v1/databases/"+dbOut.Database.ID+"/backups",
		mustJSON(map[string]any{"destination": "local"}), ownerTok)
	var bak2Out struct {
		Backup struct {
			ID        string  `json:"id"`
			CommandID *string `json:"commandId"`
		} `json:"backup"`
	}
	decode(t, bak2, &bak2Out)
	ok := doJSON(t, srv, http.MethodPost, "/api/v1/agents/commands/"+*bak2Out.Backup.CommandID+"/status",
		mustJSON(map[string]any{"status": "completed", "result": map[string]any{
			"checksum": "sha256:abc", "sizeBytes": 99,
		}}), agentCred)
	if ok.Code != http.StatusOK {
		t.Fatalf("ok complete=%d %s", ok.Code, ok.Body.String())
	}
	getOK := doJSON(t, srv, http.MethodGet, "/api/v1/backups/"+bak2Out.Backup.ID, nil, ownerTok)
	var succeeded struct {
		Backup struct {
			Status   string `json:"status"`
			Checksum string `json:"checksum"`
		} `json:"backup"`
	}
	decode(t, getOK, &succeeded)
	if succeeded.Backup.Status != backups.StatusSucceeded || succeeded.Backup.Checksum != "sha256:abc" {
		t.Fatalf("succeeded=%#v", succeeded.Backup)
	}

	// Restore requires confirm phrase.
	noConfirm := doJSON(t, srv, http.MethodPost, "/api/v1/backups/"+bak2Out.Backup.ID+"/restore",
		mustJSON(map[string]any{"targetDatabaseId": dbOut.Database.ID, "confirm": "yes"}), ownerTok)
	if noConfirm.Code != http.StatusBadRequest {
		t.Fatalf("expected validation, got %d %s", noConfirm.Code, noConfirm.Body.String())
	}

	rest := doJSON(t, srv, http.MethodPost, "/api/v1/backups/"+bak2Out.Backup.ID+"/restore",
		mustJSON(map[string]any{"targetDatabaseId": dbOut.Database.ID, "confirm": backups.RestoreConfirmPhrase}), ownerTok)
	if rest.Code != http.StatusAccepted {
		t.Fatalf("restore=%d %s", rest.Code, rest.Body.String())
	}
	var restOut struct {
		Restore struct {
			ID        string  `json:"id"`
			Status    string  `json:"status"`
			CommandID *string `json:"commandId"`
		} `json:"restore"`
	}
	decode(t, rest, &restOut)
	if restOut.Restore.CommandID == nil {
		t.Fatal("expected restore command")
	}

	// Complete without validationPassed → failed (never success).
	noval := doJSON(t, srv, http.MethodPost, "/api/v1/agents/commands/"+*restOut.Restore.CommandID+"/status",
		mustJSON(map[string]any{"status": "completed", "result": map[string]any{}}), agentCred)
	if noval.Code != http.StatusOK {
		t.Fatalf("noval=%d", noval.Code)
	}
	getRest := doJSON(t, srv, http.MethodGet, "/api/v1/restores/"+restOut.Restore.ID, nil, ownerTok)
	var restFailed struct {
		Restore struct {
			Status           string `json:"status"`
			ValidationPassed *bool  `json:"validationPassed"`
		} `json:"restore"`
	}
	decode(t, getRest, &restFailed)
	if restFailed.Restore.Status != backups.RestoreFailed {
		t.Fatalf("expected failed restore without validation, got %#v", restFailed.Restore)
	}

	// Successful restore with validation.
	rest2 := doJSON(t, srv, http.MethodPost, "/api/v1/backups/"+bak2Out.Backup.ID+"/restore",
		mustJSON(map[string]any{"targetDatabaseId": dbOut.Database.ID, "confirm": "RESTORE"}), ownerTok)
	var rest2Out struct {
		Restore struct {
			ID        string  `json:"id"`
			CommandID *string `json:"commandId"`
		} `json:"restore"`
	}
	decode(t, rest2, &rest2Out)
	_ = doJSON(t, srv, http.MethodPost, "/api/v1/agents/commands/"+*rest2Out.Restore.CommandID+"/status",
		mustJSON(map[string]any{"status": "completed", "result": map[string]any{"validationPassed": true}}), agentCred)
	getRest2 := doJSON(t, srv, http.MethodGet, "/api/v1/restores/"+rest2Out.Restore.ID, nil, ownerTok)
	var restOK struct {
		Restore struct {
			Status           string `json:"status"`
			ValidationPassed *bool  `json:"validationPassed"`
		} `json:"restore"`
	}
	decode(t, getRest2, &restOK)
	if restOK.Restore.Status != backups.RestoreSucceeded || restOK.Restore.ValidationPassed == nil || !*restOK.Restore.ValidationPassed {
		t.Fatalf("restore ok=%#v", restOK.Restore)
	}
}

func createServer(t *testing.T, srv *server.Server, token, orgID string) string {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", mustJSON(map[string]any{
		"organizationId": orgID, "name": "db-host", "provider": "hetzner",
		"region": "fsn1", "hostname": "b24-" + uuid.NewString()[:8] + ".local", "architecture": "amd64",
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
