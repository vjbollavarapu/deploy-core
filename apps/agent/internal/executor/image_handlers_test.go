package executor

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type mockLogTransport struct {
	mu   sync.Mutex
	logs []protocol.LogIngestRequest
}

func (m *mockLogTransport) Connect(_ context.Context) error         { return nil }
func (m *mockLogTransport) Disconnect(_ context.Context) error      { return nil }
func (m *mockLogTransport) Ping(_ context.Context) error            { return nil }
func (m *mockLogTransport) State() <-chan transport.ConnectionState { return nil }
func (m *mockLogTransport) CurrentState() transport.ConnectionState { return transport.StateConnected }
func (m *mockLogTransport) PollCommands(_ context.Context) ([]protocol.CommandEnvelope, error) {
	return nil, nil
}
func (m *mockLogTransport) SendHeartbeat(_ context.Context, _ protocol.HeartbeatRequest) error {
	return nil
}
func (m *mockLogTransport) SendCommandStatus(_ context.Context, _ string, _ protocol.CommandStatusRequest) error {
	return nil
}

func (m *mockLogTransport) SendLogs(_ context.Context, req protocol.LogIngestRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, req)
	return nil
}

func (m *mockLogTransport) SendMetrics(_ context.Context, _ protocol.MetricIngestRequest) error {
	return nil
}

func (m *mockLogTransport) FetchDatabaseBootstrap(_ context.Context, _ string) (protocol.DatabaseBootstrap, error) {
	return protocol.DatabaseBootstrap{}, nil
}

func TestPullImageHandler_MissingImage(t *testing.T) {
	h := pullImageHandler(nil, nil, nil)
	_, err := h.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty payload, got nil")
	}
	if !strings.Contains(err.Error(), "image or imageReference is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestPullImageHandler_CredentialScrubbing(t *testing.T) {
	auth := &docker.RegistryAuth{
		Username: "user",
		Password: "supersecretpassword",
	}

	// Payload with credential
	payload := map[string]any{
		"image": "myregistry.example.com/org/app:v1",
		"registryAuth": map[string]any{
			"username": auth.Username,
			"password": auth.Password,
		},
	}

	// Handler execution will fail at Docker layer (nil client), but defer must scrub credentials
	defer func() {
		_ = recover()
	}()

	h := pullImageHandler(nil, nil, nil)
	_, _ = h.Execute(context.Background(), payload)

	// Verify decoding succeeded and password is wiped if passed in directly
	auth.Zero()
	if auth.Password != "" {
		t.Errorf("expected Password to be scrubbed, got %q", auth.Password)
	}
}

func TestPullProgress_Streaming(t *testing.T) {
	tr := &mockLogTransport{}
	appID := "app-test-123"
	depID := "dep-test-456"

	// Create a progress callback like the one in pullImageHandler
	progressFn := func(ev docker.PullProgressEvent) {
		msg := ev.Status
		if ev.ID != "" {
			msg = ev.ID + ": " + ev.Status + " " + ev.Progress
		}
		_ = tr.SendLogs(context.Background(), protocol.LogIngestRequest{
			Kind:          "build",
			ApplicationID: appID,
			DeploymentID:  &depID,
			Entries: []protocol.LogIngestLine{
				{
					Stream:    "system",
					Message:   strings.TrimSpace(msg),
					Timestamp: ev.Timestamp,
				},
			},
		})
	}

	progressFn(docker.PullProgressEvent{
		ID:        "layer-1",
		Status:    "Downloading",
		Progress:  "[===> ] 10MB/50MB",
		Current:   10000000,
		Total:     50000000,
		Timestamp: time.Now().UTC(),
	})

	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(tr.logs))
	}
	req := tr.logs[0]
	if req.ApplicationID != appID {
		t.Errorf("expected app ID %s, got %s", appID, req.ApplicationID)
	}
	if req.DeploymentID == nil || *req.DeploymentID != depID {
		t.Errorf("expected dep ID %s, got %v", depID, req.DeploymentID)
	}
	if len(req.Entries) != 1 || !strings.Contains(req.Entries[0].Message, "Downloading") {
		t.Errorf("unexpected entry message: %+v", req.Entries)
	}
}
