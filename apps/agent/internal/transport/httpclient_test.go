package transport_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/deploycore/deploy-core/apps/agent/internal/registration"
	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

func TestPollCommands_Success(t *testing.T) {
	expectedServerID := uuid.New().String()
	expectedCred := "test-live-agent-credential"

	var capturedHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/commands" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		capturedHeaders = r.Header.Clone()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"schemaVersion": 1,
			"commands": [
				{"id": "cmd-1", "operation": "START_CONTAINER"}
			]
		}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg := &config.Config{ControlPlaneURL: u}
	cred := registration.CredentialFile{
		ServerID:   expectedServerID,
		Credential: expectedCred,
	}

	client := transport.NewHTTPClient(cfg, cred)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmds, err := client.PollCommands(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(cmds) != 1 || cmds[0].ID != "cmd-1" {
		t.Errorf("unexpected cmds: %+v", cmds)
	}

	// Verify standard transport headers: Authoritative Bearer header only
	authHeader := capturedHeaders.Get("Authorization")
	if authHeader != "Bearer "+expectedCred {
		t.Errorf("expected Authorization Bearer %s, got %s", expectedCred, authHeader)
	}

	// Verify User-Agent is set for Control Plane audit metadata
	userAgent := capturedHeaders.Get("User-Agent")
	if !strings.HasPrefix(userAgent, "deploycore-agent/") {
		t.Errorf("expected User-Agent starting with deploycore-agent/, got %s", userAgent)
	}

	// Verify NO duplicate secret-bearing headers exist
	if duplicateToken := capturedHeaders.Get("X-DeployCore-Agent-Token"); duplicateToken != "" {
		t.Errorf("found disallowed duplicate credential header X-DeployCore-Agent-Token: %s", duplicateToken)
	}
	if serverIDHeader := capturedHeaders.Get("X-DeployCore-Server-Id"); serverIDHeader != "" {
		t.Errorf("found unneeded header X-DeployCore-Server-Id: %s", serverIDHeader)
	}

	// Verify connection state
	if client.CurrentState() != transport.StateConnected {
		t.Errorf("expected state CONNECTED, got %s", client.CurrentState())
	}
}

func TestPollCommands_NoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg := &config.Config{ControlPlaneURL: u}
	cred := registration.CredentialFile{Credential: "test-cred"}
	client := transport.NewHTTPClient(cfg, cred)

	cmds, err := client.PollCommands(context.Background())
	if err != nil {
		t.Fatalf("expected no error on 204 No Content, got %v", err)
	}
	if len(cmds) != 0 {
		t.Errorf("expected 0 commands, got %d", len(cmds))
	}
	if client.CurrentState() != transport.StateConnected {
		t.Errorf("expected state CONNECTED on 204, got %s", client.CurrentState())
	}
}

func TestPollCommands_TerminalAuthFailure(t *testing.T) {
	terminalCodes := []int{http.StatusUnauthorized, http.StatusForbidden}

	for _, code := range terminalCodes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var requestCount int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&requestCount, 1)
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`{"error":{"code":"terminal_auth_failure","message":"denied"}}`))
			}))
			defer srv.Close()

			u, _ := url.Parse(srv.URL)
			cfg := &config.Config{ControlPlaneURL: u}
			cred := registration.CredentialFile{Credential: "bad-cred"}
			client := transport.NewHTTPClient(cfg, cred)

			_, err := client.PollCommands(context.Background())
			if err == nil {
				t.Fatalf("expected error on status %d, got nil", code)
			}
			if count := atomic.LoadInt32(&requestCount); count != 1 {
				t.Errorf("expected exactly 1 attempt for terminal code %d, got %d", code, count)
			}
			if client.CurrentState() != transport.StateDisconnected {
				t.Errorf("expected state DISCONNECTED on %d, got %s", code, client.CurrentState())
			}
		})
	}
}

func TestTLSVerification_Enforced(t *testing.T) {
	// Untrusted TLS server (self-signed cert not in system CA pool)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg := &config.Config{ControlPlaneURL: u}
	cred := registration.CredentialFile{Credential: "tls-test-cred"}
	client := transport.NewHTTPClient(cfg, cred)

	// Attempting to ping the untrusted TLS server must fail certificate verification
	err := client.Ping(context.Background())
	if err == nil {
		t.Fatal("expected TLS certificate verification error for untrusted self-signed certificate, got nil")
	}
	if !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "tls") {
		t.Errorf("expected TLS certificate verification failure message, got: %v", err)
	}
}

func TestPollCommands_RetryOnFail(t *testing.T) {
	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&reqCount, 1)
		if count < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"schemaVersion": 1, "commands": []}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg := &config.Config{ControlPlaneURL: u}
	cred := registration.CredentialFile{Credential: "test-cred"}

	client := transport.NewHTTPClient(cfg, cred)
	client.SetBackoffConfig(transport.BackoffConfig{
		BaseDelay:  5 * time.Millisecond,
		MaxDelay:   20 * time.Millisecond,
		MaxRetries: 4,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmds, err := client.PollCommands(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(cmds) != 0 {
		t.Errorf("expected 0 cmds, got %d", len(cmds))
	}
	if count := atomic.LoadInt32(&reqCount); count != 3 {
		t.Errorf("expected 3 requests, got %d", count)
	}
	if client.CurrentState() != transport.StateConnected {
		t.Errorf("expected state CONNECTED, got %s", client.CurrentState())
	}
}

func TestSendCommandStatus(t *testing.T) {
	var capturedMethod string
	var capturedPath string
	var capturedBody protocol.CommandStatusRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path

		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &capturedBody)

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg := &config.Config{ControlPlaneURL: u}
	client := transport.NewHTTPClient(cfg, registration.CredentialFile{Credential: "cred-test"})

	cmdID := uuid.New().String()
	err := client.SendCommandStatus(context.Background(), cmdID, protocol.CommandStatusRequest{
		Status: "completed",
		Result: map[string]any{"exitCode": float64(0)},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Must be POST /api/v1/agents/commands/{id}/status conforming to apps/api
	if capturedMethod != http.MethodPost {
		t.Errorf("expected POST method, got %s", capturedMethod)
	}
	expectedPath := "/api/v1/agents/commands/" + cmdID + "/status"
	if capturedPath != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, capturedPath)
	}
	if capturedBody.Status != "completed" {
		t.Errorf("expected status completed, got %s", capturedBody.Status)
	}
}

func TestSendLogs(t *testing.T) {
	var capturedPath string
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg := &config.Config{ControlPlaneURL: u}
	client := transport.NewHTTPClient(cfg, registration.CredentialFile{Credential: "agent-secret"})

	err := client.SendLogs(context.Background(), protocol.LogIngestRequest{
		Kind:          "build",
		ApplicationID: "app-123",
		Entries: []protocol.LogIngestLine{
			{Stream: "system", Message: "pulling image layer 1/3"},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if capturedPath != "/api/v1/agents/logs" {
		t.Errorf("expected path /api/v1/agents/logs, got %s", capturedPath)
	}
	if capturedAuth != "Bearer agent-secret" {
		t.Errorf("expected bearer auth, got %s", capturedAuth)
	}
}

func TestSendHeartbeat(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg := &config.Config{ControlPlaneURL: u}
	client := transport.NewHTTPClient(cfg, registration.CredentialFile{Credential: "hb-cred"})

	err := client.SendHeartbeat(context.Background(), protocol.HeartbeatRequest{
		AgentVersion: "1.0.0",
		DockerStatus: "ONLINE",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if capturedPath != "/api/v1/agents/heartbeat" {
		t.Errorf("expected path /api/v1/agents/heartbeat, got %s", capturedPath)
	}
}

func TestPingAndDisconnect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg := &config.Config{ControlPlaneURL: u}
	client := transport.NewHTTPClient(cfg, registration.CredentialFile{Credential: "ping-cred"})

	// Initially disconnected
	if client.CurrentState() != transport.StateDisconnected {
		t.Errorf("expected initial state DISCONNECTED, got %s", client.CurrentState())
	}

	// Test Ping succeeds
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("expected ping to succeed, got %v", err)
	}
	if client.CurrentState() != transport.StateConnected {
		t.Errorf("expected state CONNECTED after ping, got %s", client.CurrentState())
	}

	// Test Disconnect transitions to DISCONNECTED
	if err := client.Disconnect(context.Background()); err != nil {
		t.Fatalf("expected disconnect to succeed, got %v", err)
	}
	if client.CurrentState() != transport.StateDisconnected {
		t.Errorf("expected state DISCONNECTED after disconnect, got %s", client.CurrentState())
	}
}
