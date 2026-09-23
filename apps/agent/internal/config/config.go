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

	// Server ID is optional for unregistered state.
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

// IsRegistered returns true if the agent has a valid ServerID.
func (c *Config) IsRegistered() bool {
	return c.ServerID != uuid.Nil
}

// ClearRegistrationToken removes the token from memory.
func (c *Config) ClearRegistrationToken() {
	c.RegistrationToken = ""
}
