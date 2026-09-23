package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/joho/godotenv"
)

// Config holds process configuration loaded from the environment.
type Config struct {
	Env                 string
	HTTPAddr            string
	DatabaseURL         string
	LogLevel            string
	ShutdownTimeout     time.Duration
	CORSAllowedOrigins  []string
	ReadHeaderTimeout   time.Duration
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	IdleTimeout         time.Duration
	MaxRequestBodyBytes int64

	AuthTokenSecret           string
	AccessTokenTTL            time.Duration
	RefreshTokenTTL           time.Duration
	PasswordResetTTL          time.Duration
	AuthRateLimitPerMin       int
	AuthMinPasswordLength     int
	AgentRegistrationTTL      time.Duration
	AgentHeartbeatRetain      int
	PublicRateLimitPerMin     int
	GitWebhookRateLimitPerMin int

	SecretsPlatformKey []byte
	SecretsKeyID       string

	JobWorkerEnabled  bool
	JobWorkerID       string
	JobLeaseTTL       time.Duration
	JobPollInterval   time.Duration
	JobRetryBaseDelay time.Duration
	JobDeferDelay     time.Duration

	OrchestratorSimulateAgent bool

	ReconcileEnabled            bool
	ReconcileInterval           time.Duration
	AgentHeartbeatTTL           time.Duration
	ReconcileMaxAppActions      int
	ReconcileMaxRestartActions  int
	ReconcileRestartMaxAttempts int
	ReconcileRestartBackoffBase time.Duration
}

// Load reads configuration from environment variables.
// Optional .env file is loaded when present (local development).
func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		Env:                       getenv("APP_ENV", "development"),
		HTTPAddr:                  getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:               os.Getenv("DATABASE_URL"),
		LogLevel:                  getenv("LOG_LEVEL", "info"),
		ShutdownTimeout:           durationEnv("SHUTDOWN_TIMEOUT", 15*time.Second),
		CORSAllowedOrigins:        splitCSV(getenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		ReadHeaderTimeout:         durationEnv("READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:               durationEnv("READ_TIMEOUT", 30*time.Second),
		WriteTimeout:              durationEnv("WRITE_TIMEOUT", 60*time.Second),
		IdleTimeout:               durationEnv("IDLE_TIMEOUT", 120*time.Second),
		MaxRequestBodyBytes:       int64Env("MAX_REQUEST_BODY_BYTES", 1<<20),
		AuthTokenSecret:           os.Getenv("AUTH_TOKEN_SECRET"),
		AccessTokenTTL:            durationEnv("AUTH_ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:           durationEnv("AUTH_REFRESH_TOKEN_TTL", 720*time.Hour),
		PasswordResetTTL:          durationEnv("AUTH_PASSWORD_RESET_TTL", time.Hour),
		AuthRateLimitPerMin:       intEnv("AUTH_RATE_LIMIT_PER_MINUTE", 30),
		AuthMinPasswordLength:     intEnv("AUTH_MIN_PASSWORD_LENGTH", 8),
		AgentRegistrationTTL:      durationEnv("AGENT_REGISTRATION_TOKEN_TTL", 15*time.Minute),
		AgentHeartbeatRetain:      intEnv("AGENT_HEARTBEAT_RETAIN_COUNT", 50),
		PublicRateLimitPerMin:     intEnv("PUBLIC_RATE_LIMIT_PER_MINUTE", 30),
		GitWebhookRateLimitPerMin: intEnv("GIT_WEBHOOK_RATE_LIMIT_PER_MINUTE", 120),
		SecretsKeyID:              getenv("SECRETS_KEY_ID", "platform:v1"),
		JobWorkerEnabled:          boolEnv("JOB_WORKER_ENABLED", true),
		JobWorkerID:               getenv("JOB_WORKER_ID", "api-1"),
		JobLeaseTTL:               durationEnv("JOB_LEASE_TTL", 30*time.Second),
		JobPollInterval:           durationEnv("JOB_POLL_INTERVAL", time.Second),
		JobRetryBaseDelay:         durationEnv("JOB_RETRY_BASE_DELAY", 5*time.Second),
		JobDeferDelay:             durationEnv("JOB_DEFER_DELAY", time.Minute),
		// Default OFF: simulation must be explicitly enabled (never accidental production success).
		OrchestratorSimulateAgent:   boolEnv("ORCHESTRATOR_SIMULATE_AGENT", false),
		ReconcileEnabled:            boolEnv("RECONCILE_ENABLED", true),
		ReconcileInterval:           durationEnv("RECONCILE_INTERVAL", 30*time.Second),
		AgentHeartbeatTTL:           durationEnv("AGENT_HEARTBEAT_TTL", 90*time.Second),
		ReconcileMaxAppActions:      intEnv("RECONCILE_MAX_APP_ACTIONS_PER_TICK", 50),
		ReconcileMaxRestartActions:  intEnv("RECONCILE_MAX_RESTART_ACTIONS_PER_TICK", 25),
		ReconcileRestartMaxAttempts: intEnv("RECONCILE_RESTART_MAX_ATTEMPTS", 5),
		ReconcileRestartBackoffBase: durationEnv("RECONCILE_RESTART_BACKOFF_BASE", 15*time.Second),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if strings.TrimSpace(cfg.AuthTokenSecret) == "" {
		if cfg.Env == "development" || cfg.Env == "test" {
			cfg.AuthTokenSecret = "dev-only-change-me-deploycore-auth-secret"
		} else {
			return Config{}, fmt.Errorf("AUTH_TOKEN_SECRET is required")
		}
	}
	if len(cfg.AuthTokenSecret) < 32 {
		return Config{}, fmt.Errorf("AUTH_TOKEN_SECRET must be at least 32 characters")
	}
	if cfg.Env == "production" {
		for _, o := range cfg.CORSAllowedOrigins {
			if o == "*" {
				return Config{}, fmt.Errorf("CORS_ALLOWED_ORIGINS must not be * in production")
			}
		}
		if len(cfg.CORSAllowedOrigins) == 0 {
			return Config{}, fmt.Errorf("CORS_ALLOWED_ORIGINS is required in production")
		}
	}

	platformKey, err := loadSecretsPlatformKey(cfg.Env)
	if err != nil {
		return Config{}, err
	}
	cfg.SecretsPlatformKey = platformKey
	if strings.TrimSpace(os.Getenv("ORCHESTRATOR_SIMULATE_AGENT")) == "" && cfg.Env == "production" {
		cfg.OrchestratorSimulateAgent = false
	}
	return cfg, nil
}

func loadSecretsPlatformKey(env string) ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("SECRETS_PLATFORM_KEY"))
	if raw == "" {
		if env == "development" || env == "test" {
			return crypto.NormalizePlatformKey([]byte("dev-only-change-me-deploycore-secrets-platform-key"))
		}
		return nil, fmt.Errorf("SECRETS_PLATFORM_KEY is required")
	}
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	if env == "development" || env == "test" {
		return crypto.NormalizePlatformKey([]byte(raw))
	}
	return nil, fmt.Errorf("SECRETS_PLATFORM_KEY must be 32 raw bytes or standard base64 of 32 bytes")
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func int64Env(key string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func boolEnv(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
