package registries_test

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
	"github.com/deploycore/deploy-core/apps/api/internal/registries"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b17_test?sslmode=disable"
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
		DELETE FROM registries;
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
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b17-registries"))
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

func TestRegistryCRUDEncryptedCredentials(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b17@example.com", "password123", "Owner")
	viewerTok := register(t, srv, "viewer-b17@example.com", "password123", "Viewer")
	orgID := createOrg(t, srv, ownerTok, "B17 Org", "b17-org")
	inviteViewer(t, srv, ownerTok, viewerTok, orgID, "viewer-b17@example.com")

	body, _ := json.Marshal(map[string]any{
		"organizationId": orgID,
		"name":           "ghcr-main",
		"provider":       "ghcr",
		"credentials": map[string]string{
			"username": "acme",
			"token":    "ghp_super_secret_token_value",
		},
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/registries", body, ownerTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Registry struct {
			ID             string `json:"id"`
			Provider       string `json:"provider"`
			RegistryURL    string `json:"registryUrl"`
			Username       string `json:"username"`
			HasCredentials bool   `json:"hasCredentials"`
		} `json:"registry"`
	}
	decode(t, rec, &created)
	if created.Registry.Provider != "ghcr" || created.Registry.RegistryURL != "ghcr.io" {
		t.Fatalf("registry=%#v", created.Registry)
	}
	if !created.Registry.HasCredentials || created.Registry.Username != "acme" {
		t.Fatalf("creds flags=%#v", created.Registry)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("ghp_super_secret")) {
		t.Fatal("token leaked in API response")
	}

	var ct []byte
	err := pool.QueryRow(context.Background(), `
		SELECT credential_ciphertext FROM registries WHERE id = $1`, created.Registry.ID).Scan(&ct)
	if err != nil || len(ct) == 0 {
		t.Fatalf("ciphertext: %v", err)
	}
	if bytes.Contains(ct, []byte("ghp_super_secret")) {
		t.Fatal("token stored plaintext")
	}

	viewerCreate := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/registries", body, viewerTok)
	if viewerCreate.Code != http.StatusForbidden {
		t.Fatalf("viewer create=%d", viewerCreate.Code)
	}

	list := doJSON(t, srv, http.MethodGet, "/api/v1/integrations/registries?organizationId="+orgID, nil, viewerTok)
	if list.Code != http.StatusOK {
		t.Fatalf("viewer list=%d", list.Code)
	}

	// Reserved providers rejected for create.
	gcpBody, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "name": "gcp-ar", "provider": "gcp",
		"registryUrl": "us-docker.pkg.dev/proj",
	})
	gcp := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/registries", gcpBody, ownerTok)
	if gcp.Code != http.StatusBadRequest {
		t.Fatalf("gcp status=%d body=%s", gcp.Code, gcp.Body.String())
	}

	// ResolveCredentials for agent use.
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b17-registries"))
	svc := registries.NewService(registries.NewPostgresRepository(pool), rbac.NewAuthorizer(pool), nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)), registries.ServiceConfig{PlatformKey: key, KeyID: "platform:v1"})
	_, creds, err := svc.ResolveCredentials(context.Background(), uuid.MustParse(created.Registry.ID))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if creds.Token != "ghp_super_secret_token_value" || creds.Username != "acme" {
		t.Fatalf("resolved=%#v", creds)
	}

	del := doJSON(t, srv, http.MethodDelete, "/api/v1/integrations/registries/"+created.Registry.ID, nil, ownerTok)
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", del.Code)
	}

	// Soft-delete frees the name.
	again := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/registries", body, ownerTok)
	if again.Code != http.StatusCreated {
		t.Fatalf("recreate status=%d body=%s", again.Code, again.Body.String())
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
