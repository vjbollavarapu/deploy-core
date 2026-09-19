package gitproviders_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
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
		url = "postgres://localhost/deploycore_b16_test?sslmode=disable"
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
		DELETE FROM git_webhook_deliveries;
		DELETE FROM git_repositories;
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
		UPDATE git_connections SET deleted_at = NOW() WHERE deleted_at IS NULL;
		DELETE FROM git_connections;
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
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b16-gitproviders"))
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

func TestGitConnectionAndWebhookAutoDeploy(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b16@example.com", "password123", "Owner")
	viewerTok := register(t, srv, "viewer-b16@example.com", "password123", "Viewer")
	orgID := createOrg(t, srv, ownerTok, "B16 Org", "b16-org")
	inviteViewer(t, srv, ownerTok, viewerTok, orgID, "viewer-b16@example.com")
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)

	whSecret := "super-secret-webhook"
	body, _ := json.Marshal(map[string]any{
		"organizationId": orgID,
		"provider":       "github",
		"accountLogin":   "acme",
		"displayName":    "Acme GitHub",
		"accessToken":    "ghp_test_token_not_stored_plain",
		"webhookSecret":  whSecret,
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/git/connections", body, ownerTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create connection status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Connection struct {
			ID            string  `json:"id"`
			Provider      string  `json:"provider"`
			WebhookSecret *string `json:"webhookSecret"`
			HasWebhook    bool    `json:"hasWebhookSecret"`
		} `json:"connection"`
	}
	decode(t, rec, &created)
	if created.Connection.Provider != "github" || created.Connection.WebhookSecret == nil || *created.Connection.WebhookSecret != whSecret {
		t.Fatalf("connection=%#v", created.Connection)
	}
	connID := created.Connection.ID

	// Token must not be readable from DB plaintext.
	var ct []byte
	err := pool.QueryRow(context.Background(), `
		SELECT credential_ciphertext FROM git_connections WHERE id = $1`, connID).Scan(&ct)
	if err != nil || len(ct) == 0 {
		t.Fatalf("ciphertext missing: %v", err)
	}
	if bytes.Contains(ct, []byte("ghp_test_token")) {
		t.Fatal("access token stored in plaintext")
	}

	viewerCreate := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/git/connections", body, viewerTok)
	if viewerCreate.Code != http.StatusForbidden {
		t.Fatalf("viewer create status=%d", viewerCreate.Code)
	}

	syncBody, _ := json.Marshal(map[string]any{
		"repositories": []map[string]any{
			{"fullName": "acme/api", "defaultBranch": "main", "cloneUrl": "https://github.com/acme/api.git", "externalId": "1"},
		},
	})
	sync := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/git/connections/"+connID+"/sync", syncBody, ownerTok)
	if sync.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", sync.Code, sync.Body.String())
	}

	appID := createGitApp(t, srv, ownerTok, orgID, projectID, envID, connID)

	payload := []byte(`{
		"ref":"refs/heads/main",
		"after":"abcdef0123456789abcdef0123456789abcdef01",
		"deleted":false,
		"repository":{
			"full_name":"acme/api",
			"clone_url":"https://github.com/acme/api.git",
			"html_url":"https://github.com/acme/api"
		}
	}`)
	sig := signGitHub(whSecret, payload)
	wh := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/git/"+connID, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "delivery-001")
	srv.HTTPHandler().ServeHTTP(wh, req)
	if wh.Code != http.StatusOK {
		t.Fatalf("webhook status=%d body=%s", wh.Code, wh.Body.String())
	}
	var whOut struct {
		Status        string   `json:"status"`
		DeploymentIDs []string `json:"deploymentIds"`
	}
	if err := json.Unmarshal(wh.Body.Bytes(), &whOut); err != nil {
		t.Fatalf("decode webhook: %v", err)
	}
	if whOut.Status != "processed" || len(whOut.DeploymentIDs) != 1 {
		t.Fatalf("webhook out=%#v", whOut)
	}

	var trigger, status string
	err = pool.QueryRow(context.Background(), `
		SELECT trigger, status FROM deployments WHERE id = $1`, whOut.DeploymentIDs[0]).
		Scan(&trigger, &status)
	if err != nil {
		t.Fatalf("deployment: %v", err)
	}
	if trigger != deployments.TriggerGitPush || status != deployments.StatusQueued {
		t.Fatalf("deployment trigger=%s status=%s", trigger, status)
	}

	// Duplicate delivery is ignored safely.
	wh2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/git/"+connID, bytes.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Hub-Signature-256", sig)
	req2.Header.Set("X-GitHub-Event", "push")
	req2.Header.Set("X-GitHub-Delivery", "delivery-001")
	srv.HTTPHandler().ServeHTTP(wh2, req2)
	if wh2.Code != http.StatusOK {
		t.Fatalf("dup webhook status=%d", wh2.Code)
	}
	var dup struct {
		Duplicate bool   `json:"duplicate"`
		Status    string `json:"status"`
	}
	_ = json.Unmarshal(wh2.Body.Bytes(), &dup)
	if !dup.Duplicate {
		t.Fatalf("expected duplicate, got %#v", dup)
	}

	var deployCount int
	_ = pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM deployments WHERE application_id = $1`, appID).Scan(&deployCount)
	if deployCount != 1 {
		t.Fatalf("deployCount=%d", deployCount)
	}

	// Bad signature rejected.
	bad := httptest.NewRecorder()
	breq := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/git/"+connID, bytes.NewReader(payload))
	breq.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	breq.Header.Set("X-GitHub-Event", "push")
	breq.Header.Set("X-GitHub-Delivery", "delivery-002")
	srv.HTTPHandler().ServeHTTP(bad, breq)
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad sig status=%d", bad.Code)
	}
}

func signGitHub(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func createGitApp(t *testing.T, srv *server.Server, token, orgID, projectID, envID, connID string) string {
	t.Helper()
	srvBody, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "name": "edge-b16", "provider": "hetzner",
		"region": "fsn1", "hostname": "edge-b16.local", "architecture": "amd64",
	})
	srec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", srvBody, token)
	if srec.Code != http.StatusCreated {
		t.Fatalf("create server status=%d body=%s", srec.Code, srec.Body.String())
	}
	var sOut struct {
		Server struct {
			ID string `json:"id"`
		} `json:"server"`
	}
	decode(t, srec, &sOut)
	port := 8080
	auto := true
	body, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"name": "api", "slug": "api", "type": "API", "targetServerId": sOut.Server.ID,
		"config": map[string]any{
			"sourceType": "git", "repositoryUrl": "https://github.com/acme/api.git",
			"gitBranch": "main", "internalPort": port, "autoDeployEnabled": auto, "gitConnectionId": connID,
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

func inviteViewer(t *testing.T, srv *server.Server, ownerTok, viewerTok, orgID, email string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"email": email, "roleKeys": []string{"viewer"}})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/organizations/"+orgID+"/invitations", body, ownerTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite status=%d", rec.Code)
	}
	var inv struct {
		Invitation struct {
			Token string `json:"token"`
		} `json:"invitation"`
	}
	decode(t, rec, &inv)
	accept, _ := json.Marshal(map[string]string{"token": inv.Invitation.Token})
	acc := doJSON(t, srv, http.MethodPost, "/api/v1/invitations/accept", accept, viewerTok)
	if acc.Code != http.StatusOK {
		t.Fatalf("accept status=%d", acc.Code)
	}
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
