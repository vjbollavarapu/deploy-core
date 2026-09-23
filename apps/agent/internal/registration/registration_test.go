package registration_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/deploycore/deploy-core/apps/agent/internal/controlplane"
	"github.com/deploycore/deploy-core/apps/agent/internal/registration"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

func TestPerformRegistration_Success(t *testing.T) {
	expectedServerID := uuid.New().String()
	expectedAgentID := uuid.New().String()
	expectedCredential := "sec_live_abcdef1234567890"

	var receivedReq protocol.RegisterRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/agents/register" {
			t.Errorf("expected /api/v1/agents/register path, got %s", r.URL.Path)
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		if err := json.Unmarshal(bodyBytes, &receivedReq); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"agent": protocol.RegisterResult{
				AgentID:       expectedAgentID,
				ServerID:      expectedServerID,
				Credential:    expectedCredential,
				TokenType:     "Bearer",
				ProtocolMajor: 1,
				ProtocolMinor: 0,
			},
		})
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "sub", "credentials.json")

	cfg := &config.Config{
		ControlPlaneURL:   u,
		RegistrationToken: "one-time-token-xyz",
		CredentialPath:    credPath,
	}

	cpCli := controlplane.NewClient(*cfg)
	mgr := registration.NewManager(cfg, cpCli)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.PerformRegistration(ctx); err != nil {
		t.Fatalf("expected registration to succeed, got %v", err)
	}

	// 1. Wire request contents validation
	if receivedReq.RegistrationToken != "one-time-token-xyz" {
		t.Errorf("expected token one-time-token-xyz, got %s", receivedReq.RegistrationToken)
	}
	if receivedReq.ProtocolMajor != 1 {
		t.Errorf("expected ProtocolMajor 1, got %d", receivedReq.ProtocolMajor)
	}
	if receivedReq.OS == "" {
		t.Error("expected OS to be populated")
	}
	if receivedReq.Architecture == "" {
		t.Error("expected Architecture to be populated")
	}

	// 2. Token memory clearance
	if cfg.RegistrationToken != "" {
		t.Errorf("expected registration token to be cleared, got %q", cfg.RegistrationToken)
	}

	// 3. In-memory ServerID update
	if cfg.ServerID.String() != expectedServerID {
		t.Errorf("expected in-memory ServerID %s, got %s", expectedServerID, cfg.ServerID.String())
	}

	// 4. File permissions verification (0600)
	stat, err := os.Stat(credPath)
	if err != nil {
		t.Fatalf("failed to stat credential file: %v", err)
	}
	if stat.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %v", stat.Mode().Perm())
	}

	// 5. Parent directory permissions (0700)
	dirStat, err := os.Stat(filepath.Dir(credPath))
	if err != nil {
		t.Fatalf("failed to stat credential directory: %v", err)
	}
	if dirStat.Mode().Perm() != 0700 {
		t.Errorf("expected directory permissions 0700, got %v", dirStat.Mode().Perm())
	}

	// 6. Durable credential loading
	cred, err := mgr.LoadCredential()
	if err != nil {
		t.Fatalf("failed to load saved credential: %v", err)
	}
	if cred.AgentID != expectedAgentID || cred.ServerID != expectedServerID || cred.Credential != expectedCredential {
		t.Errorf("loaded credential mismatch: got %+v", cred)
	}
	if cred.TokenType != "Bearer" {
		t.Errorf("expected TokenType Bearer, got %s", cred.TokenType)
	}
	if cred.CreatedAt.IsZero() {
		t.Error("expected CreatedAt timestamp to be set")
	}
}

func TestPerformRegistration_OverwriteRefusal(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "existing_credentials.json")

	originalContent := []byte(`{"agentId":"old-agent","serverId":"old-server","credential":"old-secret"}`)
	if err := os.WriteFile(credPath, originalContent, 0600); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}

	u, _ := url.Parse("http://127.0.0.1:9999")
	cfg := &config.Config{
		ControlPlaneURL:   u,
		RegistrationToken: "attempt-token",
		CredentialPath:    credPath,
	}

	cpCli := controlplane.NewClient(*cfg)
	mgr := registration.NewManager(cfg, cpCli)

	err := mgr.PerformRegistration(context.Background())
	if err == nil {
		t.Fatal("expected overwrite error, got nil")
	}
	if !errors.Is(err, registration.ErrCredentialAlreadyExists) {
		t.Errorf("expected ErrCredentialAlreadyExists, got %v", err)
	}

	// Token must still be cleared to prevent leak
	if cfg.RegistrationToken != "" {
		t.Errorf("expected token to be cleared on overwrite refusal, got %s", cfg.RegistrationToken)
	}

	// Existing file must not be modified
	content, _ := os.ReadFile(credPath)
	if string(content) != string(originalContent) {
		t.Errorf("existing credential file was unexpectedly modified")
	}
}

func TestPerformRegistration_TerminalErrors(t *testing.T) {
	terminalCodes := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusConflict,
	}

	for _, code := range terminalCodes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var requestCount int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&requestCount, 1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`{"error":{"code":"terminal_error","message":"do not retry"}}`))
			}))
			defer srv.Close()

			u, _ := url.Parse(srv.URL)
			tmpDir := t.TempDir()
			credPath := filepath.Join(tmpDir, "creds.json")

			cfg := &config.Config{
				ControlPlaneURL:   u,
				RegistrationToken: "token-terminal",
				CredentialPath:    credPath,
			}

			cpCli := controlplane.NewClient(*cfg)
			mgr := registration.NewManager(cfg, cpCli)
			mgr.SetBackoffConfig(registration.BackoffConfig{
				InitialInterval: 5 * time.Millisecond,
				MaxInterval:     20 * time.Millisecond,
				Multiplier:      2.0,
				MaxRetries:      3,
			})

			err := mgr.PerformRegistration(context.Background())
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", code)
			}

			// Must NOT retry terminal status codes: count must be exactly 1
			if count := atomic.LoadInt32(&requestCount); count != 1 {
				t.Errorf("expected exactly 1 request for terminal status %d, got %d", code, count)
			}

			// Token must be cleared
			if cfg.RegistrationToken != "" {
				t.Errorf("expected token to be cleared, got %s", cfg.RegistrationToken)
			}

			// No file written
			if _, err := os.Stat(credPath); !os.IsNotExist(err) {
				t.Errorf("credential file should not have been written")
			}
		})
	}
}

func TestPerformRegistration_RetryOnTransientFailure(t *testing.T) {
	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")

		if count < 3 {
			// Transient error on first two attempts
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":"temporarily_unavailable","message":"try again"}}`))
			return
		}

		// Succeeds on 3rd attempt
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"agent": protocol.RegisterResult{
				AgentID:    "agent-recovered",
				ServerID:   uuid.New().String(),
				Credential: "recovered-credential",
				TokenType:  "Bearer",
			},
		})
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "creds.json")

	cfg := &config.Config{
		ControlPlaneURL:   u,
		RegistrationToken: "retry-token",
		CredentialPath:    credPath,
	}

	cpCli := controlplane.NewClient(*cfg)
	mgr := registration.NewManager(cfg, cpCli)
	mgr.SetBackoffConfig(registration.BackoffConfig{
		InitialInterval: 5 * time.Millisecond,
		MaxInterval:     20 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      4,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.PerformRegistration(ctx); err != nil {
		t.Fatalf("expected registration to succeed after retries, got %v", err)
	}

	if count := atomic.LoadInt32(&requestCount); count != 3 {
		t.Errorf("expected 3 requests before success, got %d", count)
	}

	cred, err := mgr.LoadCredential()
	if err != nil {
		t.Fatalf("failed to load credential: %v", err)
	}
	if cred.Credential != "recovered-credential" {
		t.Errorf("unexpected credential content: %s", cred.Credential)
	}
}

func TestPerformRegistration_RetryExhausted(t *testing.T) {
	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal","message":"persistent failure"}}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "creds.json")

	cfg := &config.Config{
		ControlPlaneURL:   u,
		RegistrationToken: "exhaust-token",
		CredentialPath:    credPath,
	}

	cpCli := controlplane.NewClient(*cfg)
	mgr := registration.NewManager(cfg, cpCli)
	mgr.SetBackoffConfig(registration.BackoffConfig{
		InitialInterval: 2 * time.Millisecond,
		MaxInterval:     10 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      2,
	})

	err := mgr.PerformRegistration(context.Background())
	if err == nil {
		t.Fatal("expected error when retries are exhausted, got nil")
	}

	// 1 initial + 2 retries = 3 attempts total
	if count := atomic.LoadInt32(&requestCount); count != 3 {
		t.Errorf("expected 3 requests before exhaustion, got %d", count)
	}

	if cfg.RegistrationToken != "" {
		t.Errorf("expected token to be cleared after failure, got %s", cfg.RegistrationToken)
	}
}

func TestPerformRegistration_NoToken(t *testing.T) {
	cfg := &config.Config{
		RegistrationToken: "",
	}
	mgr := registration.NewManager(cfg, nil)

	err := mgr.PerformRegistration(context.Background())
	if !errors.Is(err, registration.ErrNoRegistrationToken) {
		t.Fatalf("expected ErrNoRegistrationToken, got %v", err)
	}
}

func TestLoadCredential_InsecurePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "insecure_creds.json")

	sID := uuid.New().String()
	data := []byte(`{"agentId":"a-1","serverId":"` + sID + `","credential":"c-1"}`)
	// Permissive 0644 (world readable)
	if err := os.WriteFile(credPath, data, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg := &config.Config{CredentialPath: credPath}
	mgr := registration.NewManager(cfg, nil)

	_, err := mgr.LoadCredential()
	if err == nil {
		t.Fatal("expected error for insecure file permissions, got nil")
	}
	if !errors.Is(err, registration.ErrInsecureFilePermissions) {
		t.Errorf("expected ErrInsecureFilePermissions, got %v", err)
	}
}

func TestCredentialFile_Redaction(t *testing.T) {
	cred := registration.CredentialFile{
		AgentID:       "agent-1",
		ServerID:      "server-1",
		Credential:    "super-secret-token",
		TokenType:     "Bearer",
		ProtocolMajor: 1,
		CreatedAt:     time.Now().UTC(),
	}

	// Test fmt.Stringer
	str := cred.String()
	if strings.Contains(str, "super-secret-token") {
		t.Errorf("String() leaked secret: %s", str)
	}
	if !strings.Contains(str, "[REDACTED]") {
		t.Errorf("String() missing [REDACTED]: %s", str)
	}

	// Test slog.LogValuer
	val := cred.LogValue()
	if val.Kind() != slog.KindGroup {
		t.Fatalf("expected LogValue to be a Group, got %v", val.Kind())
	}
	attrs := val.Group()
	var foundRedacted bool
	for _, attr := range attrs {
		if attr.Key == "credential" {
			foundRedacted = true
			if attr.Value.String() != "[REDACTED]" {
				t.Errorf("expected slog credential to be [REDACTED], got %s", attr.Value.String())
			}
		}
	}
	if !foundRedacted {
		t.Error("credential attribute not found in slog GroupValue")
	}
}
