package variables_test

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
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b10_test?sslmode=disable"
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
		DELETE FROM secrets;
		DELETE FROM environment_variables;
		DELETE FROM application_configs;
		DELETE FROM applications;
		DELETE FROM environments;
		DELETE FROM projects;
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
	key, err := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b10-secrets!!"))
	if err != nil {
		t.Fatal(err)
	}
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

func TestVariableInheritanceAndSecrets(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b10@example.com", "password123", "Owner")
	viewerTok := register(t, srv, "viewer-b10@example.com", "password123", "Viewer")
	orgID := createOrg(t, srv, ownerTok, "B10 Org", "b10-org")
	inviteViewer(t, srv, ownerTok, viewerTok, orgID, "viewer-b10@example.com")
	projectID := createProject(t, srv, ownerTok, orgID, "Platform", "platform")
	envID := createEnvironment(t, srv, ownerTok, projectID, "Production", "production")
	appID := createApplication(t, srv, ownerTok, orgID, projectID, envID)

	// Org / project / env / app variables with override.
	mustCreateVar(t, srv, ownerTok, map[string]any{
		"organizationId": orgID, "scope": "ORGANIZATION", "key": "LOG_LEVEL", "value": "info",
	})
	mustCreateVar(t, srv, ownerTok, map[string]any{
		"organizationId": orgID, "scope": "PROJECT", "projectId": projectID, "key": "LOG_LEVEL", "value": "debug",
	})
	mustCreateVar(t, srv, ownerTok, map[string]any{
		"organizationId": orgID, "scope": "ENVIRONMENT", "environmentId": envID, "key": "REGION", "value": "eu",
	})
	mustCreateVar(t, srv, ownerTok, map[string]any{
		"organizationId": orgID, "scope": "APPLICATION", "applicationId": appID, "key": "LOG_LEVEL", "value": "warn",
	})

	resolved := doJSON(t, srv, http.MethodGet,
		"/api/v1/variables/resolved?organizationId="+orgID+"&applicationId="+appID, nil, ownerTok)
	if resolved.Code != http.StatusOK {
		t.Fatalf("resolve status=%d body=%s", resolved.Code, resolved.Body.String())
	}
	var resBody struct {
		Items []struct {
			Key        string `json:"key"`
			Value      string `json:"value"`
			Scope      string `json:"scope"`
			Overridden bool   `json:"overridden"`
		} `json:"items"`
	}
	decode(t, resolved, &resBody)
	byKey := map[string]struct {
		Value      string
		Scope      string
		Overridden bool
	}{}
	for _, it := range resBody.Items {
		byKey[it.Key] = struct {
			Value      string
			Scope      string
			Overridden bool
		}{it.Value, it.Scope, it.Overridden}
	}
	if byKey["LOG_LEVEL"].Value != "warn" || byKey["LOG_LEVEL"].Scope != "APPLICATION" || !byKey["LOG_LEVEL"].Overridden {
		t.Fatalf("LOG_LEVEL=%#v", byKey["LOG_LEVEL"])
	}
	if byKey["REGION"].Value != "eu" || byKey["REGION"].Overridden {
		t.Fatalf("REGION=%#v", byKey["REGION"])
	}

	// Secrets: metadata only; rotate versions; never plaintext in response.
	createSec, _ := json.Marshal(map[string]any{
		"organizationId": orgID,
		"scope":          "APPLICATION",
		"applicationId":  appID,
		"name":           "DB_PASSWORD",
		"value":          "s3cret-one",
	})
	secRec := doJSON(t, srv, http.MethodPost, "/api/v1/secrets", createSec, ownerTok)
	if secRec.Code != http.StatusCreated {
		t.Fatalf("create secret status=%d body=%s", secRec.Code, secRec.Body.String())
	}
	body := secRec.Body.String()
	if strings.Contains(body, "s3cret-one") || strings.Contains(body, "ciphertext") {
		t.Fatalf("secret response leaked value/ciphertext: %s", body)
	}
	var created struct {
		Secret struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Version int    `json:"version"`
			Scope   string `json:"scope"`
		} `json:"secret"`
	}
	decode(t, secRec, &created)
	if created.Secret.Version != 1 || created.Secret.Name != "DB_PASSWORD" {
		t.Fatalf("created=%#v", created.Secret)
	}
	secretID := created.Secret.ID

	viewerCreate := doJSON(t, srv, http.MethodPost, "/api/v1/secrets", createSec, viewerTok)
	if viewerCreate.Code != http.StatusForbidden {
		t.Fatalf("viewer create status=%d", viewerCreate.Code)
	}
	viewerList := doJSON(t, srv, http.MethodGet, "/api/v1/secrets?organizationId="+orgID, nil, viewerTok)
	if viewerList.Code != http.StatusOK {
		t.Fatalf("viewer list status=%d", viewerList.Code)
	}

	rotate, _ := json.Marshal(map[string]string{"value": "s3cret-two"})
	rot := doJSON(t, srv, http.MethodPatch, "/api/v1/secrets/"+secretID, rotate, ownerTok)
	if rot.Code != http.StatusOK {
		t.Fatalf("rotate status=%d body=%s", rot.Code, rot.Body.String())
	}
	if strings.Contains(rot.Body.String(), "s3cret-two") {
		t.Fatalf("rotate leaked plaintext")
	}
	var rotated struct {
		Secret struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		} `json:"secret"`
	}
	decode(t, rot, &rotated)
	if rotated.Secret.Version != 2 {
		t.Fatalf("version=%d", rotated.Secret.Version)
	}
	if rotated.Secret.ID == secretID {
		t.Fatal("expected new secret row id after rotate")
	}
	gone := doJSON(t, srv, http.MethodGet, "/api/v1/secrets/"+secretID, nil, ownerTok)
	if gone.Code != http.StatusNotFound {
		t.Fatalf("old secret get status=%d", gone.Code)
	}

	// Ciphertext stored; plaintext decryptable with platform key.
	var ct, nonce []byte
	var keyID string
	err := pool.QueryRow(context.Background(), `
		SELECT ciphertext, nonce, key_id FROM secrets WHERE id = $1 AND deleted_at IS NULL`,
		rotated.Secret.ID).Scan(&ct, &nonce, &keyID)
	if err != nil {
		t.Fatalf("load ciphertext: %v", err)
	}
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b10-secrets!!"))
	plain, err := crypto.Open(key, crypto.Envelope{KeyID: keyID, Nonce: nonce, Ciphertext: ct})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(plain) != "s3cret-two" {
		t.Fatalf("plain=%q", plain)
	}
}

func mustCreateVar(t *testing.T, srv *server.Server, token string, body map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/variables", b, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create var status=%d body=%s", rec.Code, rec.Body.String())
	}
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
		t.Fatalf("invite status=%d body=%s", rec.Code, rec.Body.String())
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
		t.Fatalf("accept status=%d body=%s", acc.Code, acc.Body.String())
	}
}

func createProject(t *testing.T, srv *server.Server, token, orgID, name, slug string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"organizationId": orgID, "name": name, "slug": slug})
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

func createEnvironment(t *testing.T, srv *server.Server, token, projectID, name, slug string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name, "slug": slug, "kind": "production"})
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

func createApplication(t *testing.T, srv *server.Server, token, orgID, projectID, envID string) string {
	t.Helper()
	// Need a server for target.
	srvBody, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "name": "edge-b10", "provider": "hetzner",
		"region": "fsn1", "hostname": "edge-b10.local", "architecture": "amd64",
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
	body, _ := json.Marshal(map[string]any{
		"organizationId": orgID,
		"projectId":      projectID,
		"environmentId":  envID,
		"name":           "api",
		"slug":           "api",
		"type":           "API",
		"targetServerId": sOut.Server.ID,
		"config": map[string]any{
			"sourceType": "image", "imageReference": "ghcr.io/example/api:1", "internalPort": port,
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
