package config

import (
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"regexp"
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

	GitHubApp GitHubAppConfig
	// GitHubAPIBaseURL overrides the GitHub API host for tests. Empty uses api.github.com.
	// It is not loaded from the environment.
	GitHubAPIBaseURL string
}

// redactedBytes is PEM material that must not appear in logs, JSON, or fmt output.
type redactedBytes []byte

func (redactedBytes) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, "[redacted]") }
func (redactedBytes) MarshalJSON() ([]byte, error) { return []byte("null"), nil }
func (redactedBytes) LogValue() slog.Value         { return slog.StringValue("[redacted]") }

// redactedString is a secret that must not appear in logs, JSON, or fmt output.
type redactedString string

func (redactedString) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, "[redacted]") }
func (redactedString) MarshalJSON() ([]byte, error) { return []byte("null"), nil }
func (redactedString) LogValue() slog.Value         { return slog.StringValue("[redacted]") }

// GitHubAppConfig is the process-level GitHub App identity.
// The private key and webhook secret stay in memory and are never written to PostgreSQL.
type GitHubAppConfig struct {
	Configured    bool
	AppID         string
	Slug          string
	SetupBaseURL  string
	privateKey    redactedBytes
	webhookSecret redactedString
}

// GitHubAppPublicStatus is the only GitHub App view safe to return from the API.
type GitHubAppPublicStatus struct {
	Configured bool   `json:"configured"`
	AppID      string `json:"appId,omitempty"`
	Slug       string `json:"slug,omitempty"`
}

// PublicStatus reports whether the App is configured, without secrets.
func (g GitHubAppConfig) PublicStatus() GitHubAppPublicStatus {
	if !g.Configured {
		return GitHubAppPublicStatus{}
	}
	return GitHubAppPublicStatus{Configured: true, AppID: g.AppID, Slug: g.Slug}
}

func (g GitHubAppConfig) String() string {
	return fmt.Sprintf("GitHubAppConfig{configured:%t appId:%s slug:%s}", g.Configured, g.AppID, g.Slug)
}

func (g GitHubAppConfig) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, g.String())
}

func (g GitHubAppConfig) GoString() string { return g.String() }

func (g GitHubAppConfig) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Bool("configured", g.Configured),
		slog.String("appId", g.AppID),
		slog.String("slug", g.Slug),
	)
}

func (g GitHubAppConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(g.PublicStatus())
}

// PrivateKeyPEM returns the GitHub App private key loaded from disk.
// Callers must not log or persist it.
func (g GitHubAppConfig) PrivateKeyPEM() []byte {
	if len(g.privateKey) == 0 {
		return nil
	}
	out := make([]byte, len(g.privateKey))
	copy(out, g.privateKey)
	return out
}

// WebhookSecret returns the App webhook secret. Callers must not log or persist it.
func (g GitHubAppConfig) WebhookSecret() string { return string(g.webhookSecret) }

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
	githubApp, err := loadGitHubApp(cfg.Env)
	if err != nil {
		return Config{}, err
	}
	cfg.GitHubApp = githubApp
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

var githubAppSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func loadGitHubApp(env string) (GitHubAppConfig, error) {
	appID := strings.TrimSpace(os.Getenv("GITHUB_APP_ID"))
	slug := strings.TrimSpace(os.Getenv("GITHUB_APP_SLUG"))
	keyFile := strings.TrimSpace(os.Getenv("GITHUB_APP_PRIVATE_KEY_FILE"))
	webhook := strings.TrimSpace(os.Getenv("GITHUB_APP_WEBHOOK_SECRET"))
	setup := strings.TrimRight(strings.TrimSpace(os.Getenv("GITHUB_APP_SETUP_BASE_URL")), "/")
	if appID == "" && slug == "" && keyFile == "" && webhook == "" && setup == "" {
		return GitHubAppConfig{}, nil
	}
	var missing []string
	if appID == "" {
		missing = append(missing, "GITHUB_APP_ID")
	}
	if slug == "" {
		missing = append(missing, "GITHUB_APP_SLUG")
	}
	if keyFile == "" {
		missing = append(missing, "GITHUB_APP_PRIVATE_KEY_FILE")
	}
	if webhook == "" {
		missing = append(missing, "GITHUB_APP_WEBHOOK_SECRET")
	}
	if setup == "" {
		missing = append(missing, "GITHUB_APP_SETUP_BASE_URL")
	}
	if len(missing) > 0 {
		return GitHubAppConfig{}, fmt.Errorf("incomplete GitHub App configuration: %s", strings.Join(missing, ", "))
	}
	parsedID, err := strconv.ParseInt(appID, 10, 64)
	if err != nil || parsedID <= 0 {
		return GitHubAppConfig{}, fmt.Errorf("GITHUB_APP_ID must be a positive integer")
	}
	if !githubAppSlugPattern.MatchString(slug) {
		return GitHubAppConfig{}, fmt.Errorf("GITHUB_APP_SLUG must be a lowercase GitHub App slug")
	}
	if len(webhook) < 8 {
		return GitHubAppConfig{}, fmt.Errorf("GITHUB_APP_WEBHOOK_SECRET must be at least 8 characters")
	}
	parsedURL, err := url.Parse(setup)
	if err != nil || parsedURL.Host == "" || parsedURL.User != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") {
		return GitHubAppConfig{}, fmt.Errorf("GITHUB_APP_SETUP_BASE_URL must be an absolute http(s) URL")
	}
	if env == "production" && parsedURL.Scheme != "https" {
		return GitHubAppConfig{}, fmt.Errorf("GITHUB_APP_SETUP_BASE_URL must use https in production")
	}
	key, err := readGitHubAppPrivateKey(keyFile)
	if err != nil {
		return GitHubAppConfig{}, err
	}
	return GitHubAppConfig{
		Configured:    true,
		AppID:         appID,
		Slug:          slug,
		SetupBaseURL:  setup,
		privateKey:    key,
		webhookSecret: redactedString(webhook),
	}, nil
}

func readGitHubAppPrivateKey(path string) ([]byte, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("GITHUB_APP_PRIVATE_KEY_FILE could not be read")
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, fmt.Errorf("GITHUB_APP_PRIVATE_KEY_FILE is empty")
	}
	block, _ := pem.Decode(body)
	if block == nil || !strings.Contains(block.Type, "PRIVATE KEY") {
		return nil, fmt.Errorf("GITHUB_APP_PRIVATE_KEY_FILE is not a PEM private key")
	}
	return body, nil
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
