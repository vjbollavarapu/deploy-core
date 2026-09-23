package registration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/config"
	"github.com/deploycore/deploy-core/apps/agent/internal/controlplane"
	"github.com/deploycore/deploy-core/apps/agent/pkg/version"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

var (
	// ErrCredentialAlreadyExists is returned when attempting registration if credentials already exist.
	ErrCredentialAlreadyExists = errors.New("credential file already exists: refusing to overwrite existing agent identity")
	// ErrNoRegistrationToken is returned when registration is attempted without a token.
	ErrNoRegistrationToken = errors.New("no registration token provided")
	// ErrInsecureFilePermissions is returned when credential file permissions are too permissive.
	ErrInsecureFilePermissions = errors.New("insecure credential file permissions")
)

// CredentialFile represents the structure of the saved identity file.
type CredentialFile struct {
	AgentID       string    `json:"agentId"`
	ServerID      string    `json:"serverId"`
	Credential    string    `json:"credential"`
	TokenType     string    `json:"tokenType,omitempty"`
	ProtocolMajor int       `json:"protocolMajor,omitempty"`
	CreatedAt     time.Time `json:"createdAt,omitempty"`
}

// LogValue implements slog.LogValuer to ensure secrets are never exposed in logs.
func (c CredentialFile) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("agentId", c.AgentID),
		slog.String("serverId", c.ServerID),
		slog.String("tokenType", c.TokenType),
		slog.Int("protocolMajor", c.ProtocolMajor),
		slog.String("credential", "[REDACTED]"),
		slog.Time("createdAt", c.CreatedAt),
	)
}

// String implements fmt.Stringer to ensure secrets are never printed in string representations.
func (c CredentialFile) String() string {
	return fmt.Sprintf("CredentialFile{AgentID:%s, ServerID:%s, TokenType:%s, Credential:[REDACTED]}",
		c.AgentID, c.ServerID, c.TokenType)
}

// BackoffConfig configures retry and exponential backoff behavior.
type BackoffConfig struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	MaxRetries      int
}

// DefaultBackoffConfig provides production defaults for registration retries.
var DefaultBackoffConfig = BackoffConfig{
	InitialInterval: 500 * time.Millisecond,
	MaxInterval:     5 * time.Second,
	Multiplier:      2.0,
	MaxRetries:      4,
}

// Manager handles the registration workflow.
type Manager struct {
	cfg     *config.Config
	cpCli   *controlplane.Client
	backoff BackoffConfig
}

// NewManager creates a new registration manager.
func NewManager(cfg *config.Config, cpCli *controlplane.Client) *Manager {
	return &Manager{
		cfg:     cfg,
		cpCli:   cpCli,
		backoff: DefaultBackoffConfig,
	}
}

// SetBackoffConfig overrides backoff parameters (primarily for fast unit testing).
func (m *Manager) SetBackoffConfig(b BackoffConfig) {
	m.backoff = b
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *controlplane.HTTPStatusError
	if errors.As(err, &httpErr) {
		return httpErr.IsRetryable()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

// PerformRegistration executes the one-time registration workflow.
// If successful, it writes the durable credential to disk with 0600 permissions
// using an atomic write sequence, sets the in-memory ServerID, and clears the token.
func (m *Manager) PerformRegistration(ctx context.Context) error {
	if m.cfg.RegistrationToken == "" {
		return ErrNoRegistrationToken
	}

	// Guard against silent overwrite: do not register if a credential file already exists
	if _, err := os.Stat(m.cfg.CredentialPath); err == nil {
		m.cfg.ClearRegistrationToken()
		return fmt.Errorf("%w at %s", ErrCredentialAlreadyExists, m.cfg.CredentialPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check existing credential path: %w", err)
	}

	agentVersion := version.Get().Version

	var res protocol.RegisterResult
	var lastErr error

	curInterval := m.backoff.InitialInterval
	if curInterval <= 0 {
		curInterval = 100 * time.Millisecond
	}

	for attempt := 0; attempt <= m.backoff.MaxRetries; attempt++ {
		res, lastErr = m.cpCli.Register(ctx, m.cfg.RegistrationToken, agentVersion)
		if lastErr == nil {
			break
		}

		if !isRetryableError(lastErr) || attempt == m.backoff.MaxRetries {
			// Terminal failure or retries exhausted: clear registration token from memory
			m.cfg.ClearRegistrationToken()
			return fmt.Errorf("registration request failed (attempt %d/%d): %w", attempt+1, m.backoff.MaxRetries+1, lastErr)
		}

		// Calculate backoff with jitter (+/- 20%)
		jitter := float64(curInterval) * (0.8 + 0.4*rand.Float64())
		sleepDur := time.Duration(jitter)

		select {
		case <-ctx.Done():
			m.cfg.ClearRegistrationToken()
			return fmt.Errorf("registration cancelled: %w", ctx.Err())
		case <-time.After(sleepDur):
		}

		curInterval = time.Duration(float64(curInterval) * m.backoff.Multiplier)
		if m.backoff.MaxInterval > 0 && curInterval > m.backoff.MaxInterval {
			curInterval = m.backoff.MaxInterval
		}
	}

	credFile := CredentialFile{
		AgentID:       res.AgentID,
		ServerID:      res.ServerID,
		Credential:    res.Credential,
		TokenType:     res.TokenType,
		ProtocolMajor: res.ProtocolMajor,
		CreatedAt:     time.Now().UTC(),
	}

	jsonData, err := json.MarshalIndent(credFile, "", "  ")
	if err != nil {
		m.cfg.ClearRegistrationToken()
		return fmt.Errorf("failed to marshal credential file: %w", err)
	}
	jsonData = append(jsonData, '\n')

	// Ensure the parent directory exists with restrictive 0700 permissions
	dir := filepath.Dir(m.cfg.CredentialPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		m.cfg.ClearRegistrationToken()
		return fmt.Errorf("failed to create credential directory: %w", err)
	}

	// Atomic write: write to temp file in the same directory, then rename
	tmpFile, err := os.CreateTemp(dir, ".credential-*.tmp")
	if err != nil {
		m.cfg.ClearRegistrationToken()
		return fmt.Errorf("failed to create temporary credential file: %w", err)
	}
	tmpName := tmpFile.Name()
	var writeSuccess bool
	defer func() {
		if !writeSuccess {
			_ = os.Remove(tmpName)
		}
	}()

	// Enforce 0600 permissions on temp file before writing data
	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		m.cfg.ClearRegistrationToken()
		return fmt.Errorf("failed to set restrictive permissions on credential temp file: %w", err)
	}

	if _, err := tmpFile.Write(jsonData); err != nil {
		_ = tmpFile.Close()
		m.cfg.ClearRegistrationToken()
		return fmt.Errorf("failed to write credential data: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		m.cfg.ClearRegistrationToken()
		return fmt.Errorf("failed to sync credential file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		m.cfg.ClearRegistrationToken()
		return fmt.Errorf("failed to close credential temp file: %w", err)
	}

	// Atomically move temp file to destination
	if err := os.Rename(tmpName, m.cfg.CredentialPath); err != nil {
		m.cfg.ClearRegistrationToken()
		return fmt.Errorf("failed to atomically rename credential file: %w", err)
	}
	writeSuccess = true

	// Clear the one-time token from memory immediately
	m.cfg.ClearRegistrationToken()

	// Remove systemd one-time registration env file if present (Linux installer path).
	// Best-effort: failure must not undo a successful credential write.
	scrubRegistrationEnvFile()

	// Transition memory state to registered
	if sID, err := uuid.Parse(res.ServerID); err == nil {
		m.cfg.ServerID = sID
	}

	return nil
}

// registrationEnvPath is the installer-managed one-time token file (A32/I9).
// Overridable in tests.
var registrationEnvPath = "/etc/deploycore-agent/registration.env"

func scrubRegistrationEnvFile() {
	if err := os.Remove(registrationEnvPath); err != nil && !os.IsNotExist(err) {
		// Non-fatal: credential already persisted; operator can delete manually.
		return
	}
}

// LoadCredential reads the durable identity file from disk and validates permissions.
func (m *Manager) LoadCredential() (CredentialFile, error) {
	var cred CredentialFile

	info, err := os.Stat(m.cfg.CredentialPath)
	if err != nil {
		return cred, fmt.Errorf("failed to stat credential file: %w", err)
	}

	if !info.Mode().IsRegular() {
		return cred, fmt.Errorf("credential file is not a regular file: %s", m.cfg.CredentialPath)
	}

	// Reject if group or world permissions exist
	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		return cred, fmt.Errorf("%w %04o: group or world access is forbidden, must be 0600 or more restrictive", ErrInsecureFilePermissions, perm)
	}

	data, err := os.ReadFile(m.cfg.CredentialPath)
	if err != nil {
		return cred, fmt.Errorf("failed to read credential file: %w", err)
	}

	if err := json.Unmarshal(data, &cred); err != nil {
		return cred, fmt.Errorf("failed to parse credential file: %w", err)
	}

	if cred.Credential == "" || cred.ServerID == "" {
		return cred, errors.New("credential file missing required identity fields")
	}

	return cred, nil
}
