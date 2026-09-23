package controlplane_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/deploycore/deploy-core/apps/agent/internal/controlplane"
)

func TestCheckConnectivity_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("expected path /health, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg := config.Config{
		ControlPlaneURL: u,
	}

	client := controlplane.NewClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.CheckConnectivity(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckConnectivity_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg := config.Config{
		ControlPlaneURL: u,
	}

	client := controlplane.NewClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.CheckConnectivity(ctx)
	if err == nil {
		t.Fatal("expected error for 500 status, got nil")
	}
}

func TestCheckConnectivity_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg := config.Config{
		ControlPlaneURL: u,
	}

	client := controlplane.NewClient(cfg)

	// Intentionally short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := client.CheckConnectivity(ctx)
	if err == nil {
		t.Fatal("expected error due to timeout, got nil")
	}
}

func TestRegister_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/register" {
			t.Errorf("expected path /api/v1/agents/register, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Return mocked response
		_, _ = w.Write([]byte(`{
			"agent": {
				"id": "agent-123",
				"serverId": "server-456",
				"credential": "secure-credential",
				"tokenType": "Bearer"
			}
		}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg := config.Config{
		ControlPlaneURL: u,
	}

	client := controlplane.NewClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := client.Register(ctx, "test-token", "v1.0.0")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if res.AgentID != "agent-123" {
		t.Errorf("expected agent-123, got %s", res.AgentID)
	}
	if res.Credential != "secure-credential" {
		t.Errorf("expected secure-credential, got %s", res.Credential)
	}
}

func TestRegister_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "invalid token"}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	cfg := config.Config{
		ControlPlaneURL: u,
	}

	client := controlplane.NewClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.Register(ctx, "bad-token", "v1.0.0")
	if err == nil {
		t.Fatal("expected error for 401 status, got nil")
	}
}
