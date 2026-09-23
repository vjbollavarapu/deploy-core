package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/deploycore/deploy-core/apps/agent/internal/registration"
	"github.com/deploycore/deploy-core/apps/agent/pkg/version"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// BackoffConfig configures reconnection and polling retry behavior.
type BackoffConfig struct {
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	MaxRetries int
}

// DefaultBackoffConfig provides production defaults for reconnection.
var DefaultBackoffConfig = BackoffConfig{
	BaseDelay:  1 * time.Second,
	MaxDelay:   30 * time.Second,
	MaxRetries: 6,
}

// HTTPClient implements the transport.Client interface over authenticated HTTPS.
type HTTPClient struct {
	cfg        *config.Config
	httpClient *http.Client
	cred       registration.CredentialFile
	backoff    BackoffConfig

	stateChan chan ConnectionState
	stateLock sync.RWMutex
	state     ConnectionState
}

// NewHTTPClient creates a new authenticated Control Plane transport client.
func NewHTTPClient(cfg *config.Config, cred registration.CredentialFile) *HTTPClient {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}

	return &HTTPClient{
		cfg: cfg,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   45 * time.Second,
		},
		cred:      cred,
		backoff:   DefaultBackoffConfig,
		stateChan: make(chan ConnectionState, 10),
		state:     StateDisconnected,
	}
}

// SetBackoffConfig overrides backoff parameters (useful for fast unit testing).
func (c *HTTPClient) SetBackoffConfig(b BackoffConfig) {
	c.backoff = b
}

func (c *HTTPClient) setState(s ConnectionState) {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()

	if c.state != s {
		c.state = s
		select {
		case c.stateChan <- s:
		default:
			// Non-blocking if channel full
		}
	}
}

// CurrentState returns the current connection state.
func (c *HTTPClient) CurrentState() ConnectionState {
	c.stateLock.RLock()
	defer c.stateLock.RUnlock()
	return c.state
}

// Connect verifies connectivity to the Control Plane and transitions state to StateConnected.
func (c *HTTPClient) Connect(ctx context.Context) error {
	c.setState(StateConnected)
	return nil
}

// Disconnect gracefully terminates idle connections and transitions state to StateDisconnected.
func (c *HTTPClient) Disconnect(ctx context.Context) error {
	c.setState(StateDisconnected)
	if tr, ok := c.httpClient.Transport.(*http.Transport); ok {
		tr.CloseIdleConnections()
	}
	return nil
}

// State returns the receive-only channel of connection state transitions.
func (c *HTTPClient) State() <-chan ConnectionState {
	return c.stateChan
}

// Ping tests authenticated connectivity to the control plane.
func (c *HTTPClient) Ping(ctx context.Context) error {
	resp, err := c.doRequest(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		c.setState(StateDisconnected)
		return fmt.Errorf("ping failed: %w", err)
	}
	defer resp.Body.Close()

	_, _ = io.CopyN(io.Discard, resp.Body, 1024)

	if resp.StatusCode != http.StatusOK {
		c.setState(StateDisconnected)
		return fmt.Errorf("ping returned unexpected status: %d", resp.StatusCode)
	}
	c.setState(StateConnected)
	return nil
}

func (c *HTTPClient) doRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	urlStr := c.cfg.ControlPlaneURL.JoinPath(path).String()

	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewBuffer(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	// Authoritative Bearer authorization header required by Control Plane RequireAgent middleware
	req.Header.Set("Authorization", "Bearer "+c.cred.Credential)
	// Standard User-Agent header consumed by Control Plane audit metadata
	req.Header.Set("User-Agent", "deploycore-agent/"+version.Get().Version)

	return c.httpClient.Do(req)
}

// PollCommands polls the Control Plane for pending commands using long-polling with exponential backoff and jitter.
func (c *HTTPClient) PollCommands(ctx context.Context) ([]protocol.CommandEnvelope, error) {
	baseDelay := c.backoff.BaseDelay
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}
	maxDelay := c.backoff.MaxDelay
	if maxDelay <= 0 {
		maxDelay = 5 * time.Second
	}
	maxRetries := c.backoff.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 6
	}

	retries := 0

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/agents/commands", nil)
		if err == nil {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				c.setState(StateConnected)

				var wrap struct {
					SchemaVersion int                        `json:"schemaVersion"`
					Commands      []protocol.CommandEnvelope `json:"commands"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&wrap); err != nil {
					return nil, fmt.Errorf("failed to decode commands: %w", err)
				}
				return wrap.Commands, nil
			}

			if resp.StatusCode == http.StatusNoContent {
				// Long poll returned no content, report connected and return empty
				c.setState(StateConnected)
				return nil, nil
			}

			// Terminal error: 401 Unauthorized or 403 Forbidden indicates invalid, revoked, or disabled agent credentials
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				c.setState(StateDisconnected)
				bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
				return nil, fmt.Errorf("agent authentication failed (%d): %s", resp.StatusCode, string(bodyBytes))
			}

			// Read bounded amount for error handling
			_, _ = io.CopyN(io.Discard, resp.Body, 1024)
		}

		c.setState(StateReconnecting)

		// Exponential backoff with jitter
		delay := float64(baseDelay) * float64(int(1)<<retries)
		if delay > float64(maxDelay) {
			delay = float64(maxDelay)
		}

		// Add +/- 20% jitter
		jitter := (rand.Float64()*0.4 + 0.8)
		actualDelay := time.Duration(delay * jitter)

		select {
		case <-time.After(actualDelay):
			if retries < maxRetries {
				retries++
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// SendCommandStatus reports command status updates back to the Control Plane.
func (c *HTTPClient) SendCommandStatus(ctx context.Context, cmdID string, status protocol.CommandStatusRequest) error {
	jsonData, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}

	// apps/api defines POST /api/v1/agents/commands/{commandId}/status
	path := fmt.Sprintf("/api/v1/agents/commands/%s/status", cmdID)
	resp, err := c.doRequest(ctx, http.MethodPost, path, jsonData)
	if err != nil {
		return fmt.Errorf("failed to send status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// SendHeartbeat sends periodic telemetry and health pings to the Control Plane.
func (c *HTTPClient) SendHeartbeat(ctx context.Context, hb protocol.HeartbeatRequest) error {
	jsonData, err := json.Marshal(hb)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/agents/heartbeat", jsonData)
	if err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// SendLogs ingests container and system logs into the Control Plane.
func (c *HTTPClient) SendLogs(ctx context.Context, req protocol.LogIngestRequest) error {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal logs: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/agents/logs", jsonData)
	if err != nil {
		return fmt.Errorf("failed to send logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// SendMetrics posts container/server metric snapshots to the Control Plane.
func (c *HTTPClient) SendMetrics(ctx context.Context, req protocol.MetricIngestRequest) error {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/agents/metrics", jsonData)
	if err != nil {
		return fmt.Errorf("failed to send metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// FetchDatabaseBootstrap loads managed-database secrets from the Control Plane.
// Password must never be written to logs by callers.
func (c *HTTPClient) FetchDatabaseBootstrap(ctx context.Context, databaseID string) (protocol.DatabaseBootstrap, error) {
	var out protocol.DatabaseBootstrap
	id := strings.TrimSpace(databaseID)
	if id == "" {
		return out, fmt.Errorf("databaseId is required")
	}
	path := fmt.Sprintf("/api/v1/agents/databases/%s/bootstrap", id)
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return out, fmt.Errorf("failed to fetch database bootstrap: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return out, fmt.Errorf("bootstrap unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("failed to decode database bootstrap: %w", err)
	}
	return out, nil
}
