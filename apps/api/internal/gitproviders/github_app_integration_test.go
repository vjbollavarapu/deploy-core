package gitproviders_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestGitConnectionAuthModeConstraints(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)
	ownerTok := register(t, srv, "owner-ghapp-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "GitHub App Org", "gha-"+uuid.NewString()[:8])
	const pat = "ghp_phase1_pat_must_stay_sealed"

	created := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/git/connections", mustAttachJSON(map[string]any{
		"organizationId": orgID,
		"provider":       "github",
		"accountLogin":   "acme",
		"displayName":    "Acme PAT",
		"accessToken":    pat,
	}), ownerTok)
	if created.Code != http.StatusCreated {
		t.Fatalf("create pat status=%d body=%s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), pat) || strings.Contains(created.Body.String(), "credentialCiphertext") {
		t.Fatal("PAT connection response exposed credential material")
	}
	var createdOut struct {
		Connection struct {
			ID       string `json:"id"`
			AuthMode string `json:"authMode"`
		} `json:"connection"`
	}
	decode(t, created, &createdOut)
	if createdOut.Connection.AuthMode != "pat" || createdOut.Connection.ID == "" {
		t.Fatalf("pat connection=%+v", createdOut.Connection)
	}

	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO git_connections (organization_id, provider, auth_mode, account_login, display_name)
		VALUES ($1, 'github', 'pat', 'missing', 'Missing')`, orgID)
	if !constraintFailed(err) {
		t.Fatalf("pat without credentials err=%v", err)
	}

	installationID := int64(900001)
	_, err = pool.Exec(ctx, `
		INSERT INTO git_connections (
			organization_id, provider, auth_mode, installation_id, account_id, account_type, repository_selection,
			account_login, display_name
		) VALUES ($1, 'github', 'github_app', NULL, '1', 'Organization', 'all', 'acme', 'Missing installation')`, orgID)
	if !constraintFailed(err) {
		t.Fatalf("github_app without installation err=%v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO git_connections (
			organization_id, provider, auth_mode, installation_id, account_login, display_name,
			credential_ciphertext, credential_nonce, credential_key_id
		) VALUES ($1, 'github', 'github_app', $2, 'acme', 'With PAT', decode('00','hex'), decode('00','hex'), 'platform:v1')`, orgID, installationID)
	if !constraintFailed(err) {
		t.Fatalf("github_app with credential material err=%v", err)
	}

	var appConnection string
	err = pool.QueryRow(ctx, `
		INSERT INTO git_connections (
			organization_id, provider, auth_mode, installation_id, account_id, account_type, repository_selection,
			account_login, display_name
		) VALUES ($1, 'github', 'github_app', $2, '99', 'Organization', 'selected', 'acme', 'Acme GitHub App')
		RETURNING id::text`, orgID, installationID).Scan(&appConnection)
	if err != nil {
		t.Fatal(err)
	}
	got := doJSON(t, srv, http.MethodGet, "/api/v1/integrations/git/connections/"+appConnection, nil, ownerTok)
	if got.Code != http.StatusOK {
		t.Fatalf("get github_app status=%d body=%s", got.Code, got.Body.String())
	}
	if strings.Contains(got.Body.String(), "credential") || strings.Contains(got.Body.String(), pat) {
		t.Fatal("github_app connection response exposed credential material")
	}
	var appOut struct {
		Connection struct {
			AuthMode            string `json:"authMode"`
			InstallationID      int64  `json:"installationId"`
			AccountID           string `json:"accountId"`
			AccountType         string `json:"accountType"`
			RepositorySelection string `json:"repositorySelection"`
		} `json:"connection"`
	}
	decode(t, got, &appOut)
	if appOut.Connection.AuthMode != "github_app" || appOut.Connection.InstallationID != installationID ||
		appOut.Connection.AccountID != "99" || appOut.Connection.AccountType != "Organization" ||
		appOut.Connection.RepositorySelection != "selected" {
		t.Fatalf("github_app connection=%+v", appOut.Connection)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO git_connections (
			organization_id, provider, auth_mode, installation_id, account_login, display_name
		) VALUES ($1, 'github', 'github_app', $2, 'other', 'Duplicate')`, orgID, installationID)
	if !constraintFailed(err) {
		t.Fatalf("duplicate live installation err=%v", err)
	}

	deleted := doJSON(t, srv, http.MethodDelete, "/api/v1/integrations/git/connections/"+appConnection, nil, ownerTok)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	var reused string
	err = pool.QueryRow(ctx, `
		INSERT INTO git_connections (
			organization_id, provider, auth_mode, installation_id, account_login, display_name
		) VALUES ($1, 'github', 'github_app', $2, 'acme', 'Reused')
		RETURNING id::text`, orgID, installationID).Scan(&reused)
	if err != nil || reused == "" {
		t.Fatalf("soft-deleted installation was not reusable: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO git_repositories (organization_id, connection_id, external_id, full_name, clone_url)
		VALUES ($1, $2, '', 'acme/empty-a', 'https://github.com/acme/empty-a.git')`, orgID, createdOut.Connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO git_repositories (organization_id, connection_id, external_id, full_name, clone_url)
		VALUES ($1, $2, '', 'acme/empty-b', 'https://github.com/acme/empty-b.git')`, orgID, createdOut.Connection.ID)
	if err != nil {
		t.Fatalf("empty external ids should remain compatible: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO git_repositories (organization_id, connection_id, external_id, full_name, clone_url)
		VALUES ($1, $2, '42', 'acme/api', 'https://github.com/acme/api.git')`, orgID, createdOut.Connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO git_repositories (organization_id, connection_id, external_id, full_name, clone_url)
		VALUES ($1, $2, '42', 'acme/api-renamed', 'https://github.com/acme/api-renamed.git')`, orgID, createdOut.Connection.ID)
	if !constraintFailed(err) {
		t.Fatalf("duplicate external id err=%v", err)
	}
}

func TestGitHubAppStatusKeepsSecretsOutOfTheResponse(t *testing.T) {
	pool := testPool(t)
	absent := testServer(t, pool)
	ownerTok := register(t, absent, "owner-status-"+uuid.NewString()+"@example.com", "password123", "Owner")
	unauth := doJSON(t, absent, http.MethodGet, "/api/v1/integrations/github/app", nil, "")
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", unauth.Code)
	}
	missing := doJSON(t, absent, http.MethodGet, "/api/v1/integrations/github/app", nil, ownerTok)
	if missing.Code != http.StatusOK || strings.TrimSpace(missing.Body.String()) != `{"configured":false}` {
		t.Fatalf("absent status=%d body=%s", missing.Code, missing.Body.String())
	}

	const webhook = "status-webhook-marker-9f3a"
	keyFile := writeGitHubAppKey(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_URL", "postgres://localhost/deploycore?sslmode=disable")
	t.Setenv("AUTH_TOKEN_SECRET", "test-secret-0123456789abcdef01234567")
	t.Setenv("SECRETS_PLATFORM_KEY", "")
	t.Setenv("GITHUB_APP_ID", "5150")
	t.Setenv("GITHUB_APP_SLUG", "deploycore")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_FILE", keyFile.path)
	t.Setenv("GITHUB_APP_WEBHOOK_SECRET", webhook)
	t.Setenv("GITHUB_APP_SETUP_BASE_URL", "https://deploycore.example.com")
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	platformKey, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b16-gitproviders"))
	present := server.New(config.Config{
		Env:                   "test",
		AuthTokenSecret:       "test-secret-0123456789abcdef01234567",
		AccessTokenTTL:        time.Minute,
		RefreshTokenTTL:       time.Hour,
		AuthRateLimitPerMin:   1000,
		CORSAllowedOrigins:    []string{"*"},
		AuthMinPasswordLength: 8,
		SecretsPlatformKey:    platformKey,
		SecretsKeyID:          "platform:v1",
		GitHubApp:             loaded.GitHubApp,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), pool)
	body := doJSON(t, present, http.MethodGet, "/api/v1/integrations/github/app", nil, ownerTok)
	if body.Code != http.StatusOK {
		t.Fatalf("configured status=%d body=%s", body.Code, body.Body.String())
	}
	var status struct {
		Configured bool   `json:"configured"`
		AppID      string `json:"appId"`
		Slug       string `json:"slug"`
	}
	decode(t, body, &status)
	if !status.Configured || status.AppID != "5150" || status.Slug != "deploycore" {
		t.Fatalf("status=%+v", status)
	}
	if strings.Contains(body.Body.String(), webhook) || strings.Contains(body.Body.String(), "PRIVATE KEY") || strings.Contains(body.Body.String(), keyFile.marker) {
		t.Fatal("GitHub App status response exposed secret material")
	}
}

func TestGitHubAppMigrationDownRefusesExistingRows(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)
	ownerTok := register(t, srv, "owner-down-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "Down Org", "gdn-"+uuid.NewString()[:8])
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO git_connections (
			organization_id, provider, auth_mode, installation_id, account_login, display_name
		) VALUES ($1, 'github', 'github_app', 77, 'acme', 'Acme')`, orgID); err != nil {
		t.Fatal(err)
	}
	down, err := fs.ReadFile(db.MigrationFS(), "migrations/000027_github_app.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, string(down)); err == nil || !strings.Contains(err.Error(), "github_app rows exist") {
		t.Fatalf("down migration err=%v", err)
	}
}

type gitHubAppKey struct {
	path   string
	marker string
}

func writeGitHubAppKey(t *testing.T) gitHubAppKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	body := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	path := filepath.Join(t.TempDir(), "github-app.pem")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	return gitHubAppKey{path: path, marker: string(body[40:80])}
}

func constraintFailed(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "23514" || pgErr.Code == "23505")
}
