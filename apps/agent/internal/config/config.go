package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/google/uuid"
)

// Config represents the typed agent configuration.
type Config struct {
	ServerID          uuid.UUID
	ControlPlaneURL   *url.URL
	CredentialPath    string
	DataDir           string
	LogLevel          string
	DockerHost        string
	HeartbeatInterval time.Duration
	RegistrationToken string
}

// Load reads configuration from the environment and validates it.
func Load() (Config, error) {
	cfg := Config{
		CredentialPath:    os.Getenv("AGENT_CREDENTIAL_PATH"),
		DataDir:           os.Getenv("AGENT_DATA_DIR"),
		LogLevel:          os.Getenv("AGENT_LOG_LEVEL"),
		DockerHost:        os.Getenv("AGENT_DOCKER_HOST"),
		HeartbeatInterval: 30 * time.Second,
		RegistrationToken: os.Getenv("AGENT_REGISTRATION_TOKEN"),
	}

	// AGENT_SERVER_ID is an optional expected-server hint (installer may set it
	// before first registration). It alone does not mean registration completed;
	// durable credentials.json is authoritative (see IsRegistered).
	if sID := os.Getenv("AGENT_SERVER_ID"); sID != "" {
		parsed, err := uuid.Parse(sID)
		if err != nil {
			return cfg, fmt.Errorf("invalid AGENT_SERVER_ID: %w", err)
		}
		cfg.ServerID = parsed
	}

	// Control Plane URL is required.
	cpURL := os.Getenv("AGENT_CONTROL_PLANE_URL")
	if cpURL == "" {
		return cfg, errors.New("AGENT_CONTROL_PLANE_URL is required")
	}
	parsedURL, err := url.ParseRequestURI(cpURL)
	if err != nil {
		return cfg, fmt.Errorf("invalid AGENT_CONTROL_PLANE_URL: %w", err)
	}
	cfg.ControlPlaneURL = parsedURL

	if hb := os.Getenv("AGENT_HEARTBEAT_INTERVAL"); hb != "" {
		d, err := time.ParseDuration(hb)
		if err != nil {
			return cfg, fmt.Errorf("invalid AGENT_HEARTBEAT_INTERVAL: %w", err)
		}
		cfg.HeartbeatInterval = d
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	return cfg, nil
}

// IsRegistered returns true when durable agent credentials exist on disk.
// A non-nil ServerID from AGENT_SERVER_ID is not sufficient: the installer may
// write the expected server UUID before PerformRegistration completes.
func (c *Config) IsRegistered() bool {
	if c.CredentialPath == "" {
		return false
	}
	info, err := os.Stat(c.CredentialPath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// ClearRegistrationToken removes the token from memory.
func (c *Config) ClearRegistrationToken() {
	c.RegistrationToken = ""
}
