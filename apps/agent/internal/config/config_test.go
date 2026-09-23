package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/google/uuid"
)

func TestLoad_ValidUnregistered(t *testing.T) {
	os.Clearenv()
	os.Setenv("AGENT_CONTROL_PLANE_URL", "https://cp.example.com")
	os.Setenv("AGENT_LOG_LEVEL", "debug")
	os.Setenv("AGENT_REGISTRATION_TOKEN", "test-token")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.ControlPlaneURL.String() != "https://cp.example.com" {
		t.Errorf("expected url https://cp.example.com, got %s", cfg.ControlPlaneURL)
	}
	if cfg.IsRegistered() {
		t.Error("expected IsRegistered to be false")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.LogLevel)
	}
	if cfg.HeartbeatInterval != 30*time.Second {
		t.Errorf("expected default heartbeat 30s, got %v", cfg.HeartbeatInterval)
	}
	if cfg.RegistrationToken != "test-token" {
		t.Errorf("expected RegistrationToken test-token, got %s", cfg.RegistrationToken)
	}

	// Test clear token
	cfg.ClearRegistrationToken()
	if cfg.RegistrationToken != "" {
		t.Errorf("expected RegistrationToken to be cleared, got %s", cfg.RegistrationToken)
	}
}

func TestLoad_ValidRegistered(t *testing.T) {
	os.Clearenv()
	os.Setenv("AGENT_CONTROL_PLANE_URL", "https://cp.example.com")
	id := uuid.New().String()
	os.Setenv("AGENT_SERVER_ID", id)
	os.Setenv("AGENT_HEARTBEAT_INTERVAL", "15s")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !cfg.IsRegistered() {
		t.Error("expected IsRegistered to be true")
	}
	if cfg.ServerID.String() != id {
		t.Errorf("expected ID %s, got %s", id, cfg.ServerID.String())
	}
	if cfg.HeartbeatInterval != 15*time.Second {
		t.Errorf("expected heartbeat 15s, got %v", cfg.HeartbeatInterval)
	}
}

func TestLoad_InvalidURL(t *testing.T) {
	os.Clearenv()
	os.Setenv("AGENT_CONTROL_PLANE_URL", "://bad-url")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestLoad_MissingURL(t *testing.T) {
	os.Clearenv()

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing URL, got nil")
	}
}

func TestLoad_InvalidUUID(t *testing.T) {
	os.Clearenv()
	os.Setenv("AGENT_CONTROL_PLANE_URL", "https://cp.example.com")
	os.Setenv("AGENT_SERVER_ID", "not-a-uuid")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid UUID, got nil")
	}
}

func TestLoad_InvalidDuration(t *testing.T) {
	os.Clearenv()
	os.Setenv("AGENT_CONTROL_PLANE_URL", "https://cp.example.com")
	os.Setenv("AGENT_HEARTBEAT_INTERVAL", "bad")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}
