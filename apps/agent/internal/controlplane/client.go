package controlplane

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	goRuntime "runtime"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// HTTPStatusError represents an HTTP error response from the Control Plane.
type HTTPStatusError struct {
	StatusCode int
	Message    string
}

func (e *HTTPStatusError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("registration failed: status %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("registration failed: status %d", e.StatusCode)
}

// IsRetryable determines whether the HTTP status code represents a transient error.
func (e *HTTPStatusError) IsRetryable() bool {
	switch e.StatusCode {
	case http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusConflict:
		return false
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// Client provides connectivity to the Control Plane.
type Client struct {
	cfg        config.Config
	httpClient *http.Client
}

// NewClient creates a new Control Plane client.
func NewClient(cfg config.Config) *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   10 * time.Second,
		},
	}
}

// CheckConnectivity verifies if the Control Plane is reachable.
func (c *Client) CheckConnectivity(ctx context.Context) error {
	url := c.cfg.ControlPlaneURL.JoinPath("/health").String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("control plane unreachable: %w", err)
	}
	defer resp.Body.Close()

	// Read bounded amount to avoid slowloris/resource exhaustion
	_, _ = io.CopyN(io.Discard, resp.Body, 4096)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control plane returned unexpected status: %d", resp.StatusCode)
	}

	return nil
}

// Register calls the Control Plane to exchange a one-time token for durable credentials.
func (c *Client) Register(ctx context.Context, token, agentVersion string) (protocol.RegisterResult, error) {
	hostname, _ := os.Hostname()

	reqBody := protocol.RegisterRequest{
		RegistrationToken: token,
		AgentVersion:      agentVersion,
		ProtocolMajor:     1,
		ProtocolMinor:     0,
		Hostname:          hostname,
		OS:                goRuntime.GOOS,
		Architecture:      goRuntime.GOARCH,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return protocol.RegisterResult{}, fmt.Errorf("failed to marshal register request: %w", err)
	}

	urlStr := c.cfg.ControlPlaneURL.JoinPath("/api/v1/agents/register").String()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewBuffer(jsonData))
	if err != nil {
		return protocol.RegisterResult{}, fmt.Errorf("failed to create register request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return protocol.RegisterResult{}, fmt.Errorf("control plane unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// Read a bounded amount of body to display error without leaking credentials
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return protocol.RegisterResult{}, &HTTPStatusError{
			StatusCode: resp.StatusCode,
			Message:    string(bodyBytes),
		}
	}

	var wrap struct {
		Agent protocol.RegisterResult `json:"agent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrap); err != nil {
		return protocol.RegisterResult{}, fmt.Errorf("failed to decode register response: %w", err)
	}

	if err := wrap.Agent.Validate(); err != nil {
		return protocol.RegisterResult{}, fmt.Errorf("invalid register response: %w", err)
	}

	return wrap.Agent, nil
}
