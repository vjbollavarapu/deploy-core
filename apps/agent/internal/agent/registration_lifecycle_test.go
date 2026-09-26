package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/deploycore/deploy-core/apps/agent/internal/controlplane"
	"github.com/deploycore/deploy-core/apps/agent/internal/registration"
	"github.com/deploycore/deploy-core/apps/agent/internal/runtime"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

// TestRun_FirstInstall_ServerIDWithoutCredentials_PerformsRegistration reproduces the
// OCI LV-A defect: installer wrote AGENT_SERVER_ID + registration token, but
// credentials.json does not exist yet. Run must still PerformRegistration.
func TestRun_FirstInstall_ServerIDWithoutCredentials_PerformsRegistration(t *testing.T) {
	expectedServerID := uuid.New()
	expectedAgentID := uuid.New().String()
	expectedCredential := "sec_live_first_install_cred"

	var registerCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents/register":
			registerCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"agent": protocol.RegisterResult{
					AgentID:       expectedAgentID,
					ServerID:      expectedServerID.String(),
					Credential:    expectedCredential,
					TokenType:     "Bearer",
					ProtocolMajor: 1,
					ProtocolMinor: 0,
				},
			})
		case r.URL.Path == "/api/v1/agents/commands":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaVersion": 1,
				"commands":      []any{},
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	dataDir := t.TempDir()
	credPath := filepath.Join(dataDir, "credentials.json")
	cpURL, _ := url.Parse(srv.URL)

	cfg := config.Config{
		ServerID:          expectedServerID, // installer hint — must NOT skip registration
		ControlPlaneURL:   cpURL,
		CredentialPath:    credPath,
		DataDir:           dataDir,
		HeartbeatInterval: time.Hour,
		RegistrationToken: "one-time-token",
		LogLevel:          "error",
	}
	if cfg.IsRegistered() {
		t.Fatal("precondition: IsRegistered must be false without credentials.json")
	}

	paths := runtime.Paths{DataDir: dataDir, CredentialPath: credPath}
	cpCli := controlplane.NewClient(cfg)
	a := &Agent{
		log:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		cfg:    cfg,
		paths:  paths,
		cpCli:  cpCli,
		docCli: nil, // no Docker — registration gate only
		state:  StateStarting,
	}
	a.regMgr = registration.NewManager(&a.cfg, cpCli)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	deadline := time.After(5 * time.Second)
	for registerCalls.Load() == 0 {
		select {
		case <-deadline:
			cancel()
			t.Fatal("timed out waiting for PerformRegistration")
		case err := <-done:
			t.Fatalf("Run exited early: %v", err)
		case <-time.After(10 * time.Millisecond):
		}
	}

	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run error after cancel: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("agent did not shut down")
	}

	if registerCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 register call, got %d", registerCalls.Load())
	}
	raw, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatalf("expected credentials.json after registration: %v", err)
	}
	var cred registration.CredentialFile
	if err := json.Unmarshal(raw, &cred); err != nil {
		t.Fatalf("parse credentials: %v", err)
	}
	if cred.Credential != expectedCredential || cred.ServerID != expectedServerID.String() || cred.AgentID != expectedAgentID {
		t.Fatalf("credential mismatch: %+v", cred)
	}
	info, err := os.Stat(credPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected credentials 0600, got %04o", info.Mode().Perm())
	}
	if !a.cfg.IsRegistered() {
		t.Fatal("expected IsRegistered true after durable credentials written")
	}
}

// TestRun_AlreadyRegistered_UsesDurableCredentials ensures a restart with
// credentials.json present skips registration and loads durable identity.
func TestRun_AlreadyRegistered_UsesDurableCredentials(t *testing.T) {
	var registerCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agents/register" {
			registerCalls.Add(1)
			http.Error(w, "register must not be called", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		if r.URL.Path == "/api/v1/agents/commands" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaVersion": 1,
				"commands":      []any{},
			})
		}
	}))
	defer srv.Close()

	dataDir := t.TempDir()
	credPath := filepath.Join(dataDir, "credentials.json")
	serverID := uuid.New()
	agentID := uuid.New().String()
	cred := registration.CredentialFile{
		AgentID:    agentID,
		ServerID:   serverID.String(),
		Credential: "sec_live_existing",
		TokenType:  "Bearer",
	}
	raw, _ := json.MarshalIndent(cred, "", "  ")
	if err := os.WriteFile(credPath, append(raw, '\n'), 0600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	cpURL, _ := url.Parse(srv.URL)
	cfg := config.Config{
		ServerID:          serverID,
		ControlPlaneURL:   cpURL,
		CredentialPath:    credPath,
		DataDir:           dataDir,
		HeartbeatInterval: time.Hour,
		LogLevel:          "error",
	}
	if !cfg.IsRegistered() {
		t.Fatal("precondition: IsRegistered must be true with credentials.json")
	}

	paths := runtime.Paths{DataDir: dataDir, CredentialPath: credPath}
	cpCli := controlplane.NewClient(cfg)
	a := &Agent{
		log:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		cfg:    cfg,
		paths:  paths,
		cpCli:  cpCli,
		docCli: nil,
		state:  StateStarting,
	}
	a.regMgr = registration.NewManager(&a.cfg, cpCli)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("agent did not shut down")
	}

	if registerCalls.Load() != 0 {
		t.Fatalf("already-registered agent must not call register, got %d calls", registerCalls.Load())
	}
	info, err := os.Stat(credPath)
	if err != nil {
		t.Fatalf("credentials missing after run: %v", err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("credentials permissions weakened: %04o", info.Mode().Perm())
	}
}
