package config_test

import (
	"os"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/config"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL missing")
	}
}

func TestLoadDefaults(t *testing.T) {
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
	_ = os.Unsetenv("HTTP_ADDR")
}

func TestLoadRequiresAuthSecretInProduction(t *testing.T) {
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
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://localhost/deploycore?sslmode=disable")
	t.Setenv("AUTH_TOKEN_SECRET", "production-auth-secret-at-least-32-chars")
	t.Setenv("SECRETS_PLATFORM_KEY", "")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected SECRETS_PLATFORM_KEY required in production")
	}
}
