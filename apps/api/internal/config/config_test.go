package config_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/config"
)

func clearGitHubAppEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_APP_ID", "")
	t.Setenv("GITHUB_APP_SLUG", "")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_FILE", "")
	t.Setenv("GITHUB_APP_WEBHOOK_SECRET", "")
	t.Setenv("GITHUB_APP_SETUP_BASE_URL", "")
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	clearGitHubAppEnv(t)
	t.Setenv("DATABASE_URL", "")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL missing")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearGitHubAppEnv(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://localhost/deploycore?sslmode=disable")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("AUTH_TOKEN_SECRET", "")
	t.Setenv("SECRETS_PLATFORM_KEY", "")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000, http://127.0.0.1:3000")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %s", cfg.HTTPAddr)
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("CORS = %#v", cfg.CORSAllowedOrigins)
	}
	if len(cfg.AuthTokenSecret) < 32 {
		t.Fatalf("expected development auth secret")
	}
	if len(cfg.SecretsPlatformKey) != 32 {
		t.Fatalf("expected development secrets platform key")
	}
	if cfg.SecretsKeyID != "platform:v1" {
		t.Fatalf("SecretsKeyID=%s", cfg.SecretsKeyID)
	}
	if cfg.GitHubApp.Configured {
		t.Fatal("GitHub App should be unconfigured when its environment is absent")
	}
	_ = os.Unsetenv("HTTP_ADDR")
}

func TestLoadRequiresAuthSecretInProduction(t *testing.T) {
	clearGitHubAppEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://localhost/deploycore?sslmode=disable")
	t.Setenv("AUTH_TOKEN_SECRET", "")
	t.Setenv("SECRETS_PLATFORM_KEY", "")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected AUTH_TOKEN_SECRET required in production")
	}
}

func TestLoadRequiresSecretsKeyInProduction(t *testing.T) {
	clearGitHubAppEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://localhost/deploycore?sslmode=disable")
	t.Setenv("AUTH_TOKEN_SECRET", "production-auth-secret-at-least-32-chars")
	t.Setenv("SECRETS_PLATFORM_KEY", "")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected SECRETS_PLATFORM_KEY required in production")
	}
}

func TestGitHubAppConfigAbsentAndPresent(t *testing.T) {
	clearGitHubAppEnv(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_URL", "postgres://localhost/deploycore?sslmode=disable")
	t.Setenv("AUTH_TOKEN_SECRET", "test-secret-0123456789abcdef01234567")
	absent, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if absent.GitHubApp.Configured || absent.GitHubApp.PrivateKeyPEM() != nil || absent.GitHubApp.WebhookSecret() != "" {
		t.Fatal("absent GitHub App configuration loaded credential material")
	}

	const webhook = "github-app-webhook-marker-9f3a"
	keyPEM := writeTestPrivateKey(t)
	t.Setenv("GITHUB_APP_ID", "4242")
	t.Setenv("GITHUB_APP_SLUG", "deploycore")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_FILE", keyPEM.path)
	t.Setenv("GITHUB_APP_WEBHOOK_SECRET", webhook)
	t.Setenv("GITHUB_APP_SETUP_BASE_URL", "https://deploycore.example.com/")
	present, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !present.GitHubApp.Configured || present.GitHubApp.AppID != "4242" || present.GitHubApp.Slug != "deploycore" {
		t.Fatalf("app=%s", present.GitHubApp)
	}
	if present.GitHubApp.SetupBaseURL != "https://deploycore.example.com" {
		t.Fatalf("setup=%s", present.GitHubApp.SetupBaseURL)
	}
	if !strings.Contains(string(present.GitHubApp.PrivateKeyPEM()), "PRIVATE KEY") || present.GitHubApp.WebhookSecret() != webhook {
		t.Fatal("configured GitHub App did not retain its in-memory credentials")
	}
	status, err := json.Marshal(present.GitHubApp.PublicStatus())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(present.GitHubApp)
	if err != nil {
		t.Fatal(err)
	}
	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	logger.Info("github app", slog.Any("githubApp", present.GitHubApp))
	printed := fmt.Sprintf("%v %+v %#v %s %s", present.GitHubApp, present.GitHubApp, present.GitHubApp, status, encoded)
	printed += logs.String()
	if strings.Contains(printed, "BEGIN") || strings.Contains(printed, webhook) || strings.Contains(printed, keyPEM.marker) {
		t.Fatal("GitHub App configuration exposed private key or webhook secret")
	}
}

func TestGitHubAppConfigRejectsPartialAndInvalidKey(t *testing.T) {
	clearGitHubAppEnv(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_URL", "postgres://localhost/deploycore?sslmode=disable")
	t.Setenv("AUTH_TOKEN_SECRET", "test-secret-0123456789abcdef01234567")
	const webhook = "partial-webhook-marker-9f3a"
	t.Setenv("GITHUB_APP_ID", "7")
	t.Setenv("GITHUB_APP_WEBHOOK_SECRET", webhook)
	_, err := config.Load()
	if err == nil || strings.Contains(err.Error(), webhook) {
		t.Fatalf("partial config error=%v", err)
	}

	path := filepath.Join(t.TempDir(), "not-a-key.pem")
	const marker = "not-a-private-key-marker-9f3a"
	if err := os.WriteFile(path, []byte(marker), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_APP_SLUG", "deploycore")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_FILE", path)
	t.Setenv("GITHUB_APP_SETUP_BASE_URL", "https://deploycore.example.com")
	_, err = config.Load()
	if err == nil || strings.Contains(err.Error(), marker) || strings.Contains(err.Error(), webhook) {
		t.Fatalf("invalid key error leaked material: %v", err)
	}
}

type testPrivateKey struct {
	path   string
	marker string
}

func writeTestPrivateKey(t *testing.T) testPrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	body := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	marker := string(body[40:80])
	path := filepath.Join(t.TempDir(), "github-app.pem")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	return testPrivateKey{path: path, marker: marker}
}
