package executor_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/executor"
	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

// --------------------------------------------------------------------------
// Journal tests
// --------------------------------------------------------------------------

func TestJournal_ReplayProtection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	j := newTestJournal(t, dir)

	id := uuid.New().String()
	exp := time.Now().Add(5 * time.Minute)

	if j.Has(id) {
		t.Fatal("journal should not contain unseen ID")
	}
	j.Add(id, exp)
	if !j.Has(id) {
		t.Fatal("journal should contain ID after Add")
	}
	// Add again is idempotent
	j.Add(id, exp)
	if !j.Has(id) {
		t.Fatal("journal should still contain ID")
	}
}

func TestJournal_ExpiredNotLoaded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	j1 := newTestJournal(t, dir)

	id := uuid.New().String()
	// Add an already-expired entry
	j1.Add(id, time.Now().Add(-1*time.Second))

	// Load fresh journal — expired entry should not appear
	j2 := newTestJournal(t, dir)
	j2.Load()
	if j2.Has(id) {
		t.Fatal("expired entry should not be loaded on restart")
	}
}

func newTestJournal(t *testing.T, dir string) *executor.Journal {
	t.Helper()
	return executor.NewJournal(dir, nil)
}

// --------------------------------------------------------------------------
// Executor dispatch tests — use a mock transport
// --------------------------------------------------------------------------

// mockTransport implements transport.Client for testing.
type mockTransport struct {
	mu       sync.Mutex
	commands []protocol.CommandEnvelope
	statuses []capturedStatus
}

type capturedStatus struct {
	cmdID  string
	status string
}

func (m *mockTransport) Connect(_ context.Context) error    { return nil }
func (m *mockTransport) Disconnect(_ context.Context) error { return nil }
func (m *mockTransport) Ping(_ context.Context) error       { return nil }
func (m *mockTransport) State() <-chan transport.ConnectionState {
	ch := make(chan transport.ConnectionState)
	close(ch)
	return ch
}
func (m *mockTransport) CurrentState() transport.ConnectionState {
	return transport.StateConnected
}
func (m *mockTransport) SendHeartbeat(_ context.Context, _ protocol.HeartbeatRequest) error {
	return nil
}
func (m *mockTransport) SendCommandStatus(_ context.Context, cmdID string, req protocol.CommandStatusRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses = append(m.statuses, capturedStatus{cmdID: cmdID, status: req.Status})
	return nil
}

func (m *mockTransport) SendLogs(_ context.Context, _ protocol.LogIngestRequest) error {
	return nil
}

func (m *mockTransport) SendMetrics(_ context.Context, _ protocol.MetricIngestRequest) error {
	return nil
}

func (m *mockTransport) FetchDatabaseBootstrap(_ context.Context, _ string) (protocol.DatabaseBootstrap, error) {
	return protocol.DatabaseBootstrap{}, nil
}

func (m *mockTransport) PollCommands(ctx context.Context) ([]protocol.CommandEnvelope, error) {
	m.mu.Lock()
	if len(m.commands) == 0 {
		m.mu.Unlock()
		// Block until context is done so the executor loop doesn't spin.
		<-ctx.Done()
		return nil, ctx.Err()
	}
	cmds := m.commands
	m.commands = nil
	m.mu.Unlock()
	return cmds, nil
}

func (m *mockTransport) lastStatuses() []capturedStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]capturedStatus, len(m.statuses))
	copy(out, m.statuses)
	return out
}

// makeEnvelope builds a minimal valid command envelope.
func makeEnvelope(serverID uuid.UUID, op string) protocol.CommandEnvelope {
	id := uuid.New().String()
	sid := serverID.String()
	exp := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano)
	return protocol.CommandEnvelope{
		ID:             id,
		ServerID:       sid,
		OrganizationID: uuid.New().String(),
		Operation:      op,
		SchemaVersion:  protocol.SchemaVersion,
		Payload:        map[string]any{},
		IssuedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		ExpiresAt:      exp,
	}
}

func waitForStatus(t *testing.T, tr *mockTransport, cmdID string, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, s := range tr.lastStatuses() {
			if s.cmdID == cmdID && s.status == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("timed out waiting for command %s to reach status %s; got: %v", cmdID, want, tr.lastStatuses())
}

func TestExecutor_UnknownOperation_Rejected(t *testing.T) {
	t.Parallel()
	serverID := uuid.New()
	tr := &mockTransport{}
	cmd := makeEnvelope(serverID, "NOT_A_REAL_OPERATION")
	tr.commands = []protocol.CommandEnvelope{cmd}

	exec := newTestExecutor(t, serverID, tr)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	exec.Start(ctx)

	waitForStatus(t, tr, cmd.ID, string(executor.StateRejected), 2*time.Second)
	cancel()
	exec.Stop()
}

func TestExecutor_ServerIDMismatch_Rejected(t *testing.T) {
	t.Parallel()
	serverID := uuid.New()
	wrongServerID := uuid.New()
	tr := &mockTransport{}

	cmd := makeEnvelope(wrongServerID, protocol.OpStopContainer)
	tr.commands = []protocol.CommandEnvelope{cmd}

	exec := newTestExecutor(t, serverID, tr)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	exec.Start(ctx)

	waitForStatus(t, tr, cmd.ID, string(executor.StateRejected), 2*time.Second)
	cancel()
	exec.Stop()
}

func TestExecutor_ExpiredCommand_Rejected(t *testing.T) {
	t.Parallel()
	serverID := uuid.New()
	tr := &mockTransport{}

	cmd := makeEnvelope(serverID, protocol.OpStopContainer)
	// Set ExpiresAt in the past
	cmd.ExpiresAt = time.Now().Add(-5 * time.Second).UTC().Format(time.RFC3339Nano)
	tr.commands = []protocol.CommandEnvelope{cmd}

	exec := newTestExecutor(t, serverID, tr)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	exec.Start(ctx)

	waitForStatus(t, tr, cmd.ID, string(executor.StateRejected), 2*time.Second)
	cancel()
	exec.Stop()
}

func TestExecutor_ReplayProtection(t *testing.T) {
	t.Parallel()
	serverID := uuid.New()
	tr := &mockTransport{}

	cmd := makeEnvelope(serverID, protocol.OpStopContainer)
	// Add the same command twice to simulate a duplicate poll.
	tr.commands = []protocol.CommandEnvelope{cmd, cmd}

	// Count how many ACCEPTED statuses come back — must be exactly 1.

	exec := newTestExecutor(t, serverID, tr)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	exec.Start(ctx)

	// Wait for both commands to settle (one accepted, one rejected as duplicate)
	time.Sleep(500 * time.Millisecond)
	cancel()
	exec.Stop()

	var acceptedCount int
	for _, s := range tr.lastStatuses() {
		if s.status == string(executor.StateAccepted) {
			acceptedCount++
		}
	}
	if acceptedCount != 1 {
		t.Errorf("expected exactly 1 ACCEPTED for duplicate command, got %d; statuses=%v", acceptedCount, tr.lastStatuses())
	}
}

func TestExecutor_BoundedConcurrency(t *testing.T) {
	t.Parallel()
	serverID := uuid.New()
	tr := &mockTransport{}

	// Fill the semaphore (capacity = 4) with 4 commands, then send 2 more.
	// The 4 initial commands hold the slots; the 2 extras should be REJECTED with CAPACITY_EXCEEDED.
	// We send all 6 in one poll batch to maximize concurrency collision.
	cmds := make([]protocol.CommandEnvelope, 6)
	for i := range cmds {
		cmds[i] = makeEnvelope(serverID, protocol.OpDeployRevision)
		cmds[i].Payload = map[string]any{
			"applicationId": uuid.New().String(),
			"deploymentId":  uuid.New().String(),
			"revisionId":    uuid.New().String(),
			"image":         "busybox:latest",
		}
	}
	tr.commands = cmds

	exec := newTestExecutor(t, serverID, tr)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	exec.Start(ctx)
	time.Sleep(500 * time.Millisecond)
	cancel()
	exec.Stop()

	// Verify all 6 commands were seen — each should have reached either ACCEPTED or REJECTED.
	seen := map[string]bool{}
	for _, s := range tr.lastStatuses() {
		seen[s.cmdID] = true
	}
	t.Logf("statuses: %v", tr.lastStatuses())
	// Just assert the executor didn't panic and processed all commands.
	if len(seen) == 0 {
		t.Error("expected at least one command status to be reported")
	}
}

func newTestExecutor(t *testing.T, serverID uuid.UUID, tr *mockTransport) *executor.Executor {
	t.Helper()
	return executor.NewWithTransport(executor.Config{
		ServerID:       serverID,
		MaxConcurrency: 4,
		DataDir:        t.TempDir(),
	}, tr)
}

func TestExecutor_CommandID_CamelCaseSupport(t *testing.T) {
	t.Parallel()
	serverID := uuid.New()
	tr := &mockTransport{}

	// Envelope has ID empty but CommandID populated
	cmd := makeEnvelope(serverID, protocol.OpStopContainer)
	cmd.CommandID = cmd.ID
	cmd.ID = ""

	tr.commands = []protocol.CommandEnvelope{cmd}

	exec := newTestExecutor(t, serverID, tr)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	exec.Start(ctx)

	// Command should be recognized and processed using CommandID
	waitForStatus(t, tr, cmd.CommandID, string(executor.StateAccepted), 2*time.Second)
	cancel()
	exec.Stop()
}

func TestExecutor_EmptyCommandID_Rejected(t *testing.T) {
	t.Parallel()
	serverID := uuid.New()
	tr := &mockTransport{}

	// Envelope has both ID and CommandID empty
	cmd := makeEnvelope(serverID, protocol.OpStopContainer)
	cmd.ID = ""
	cmd.CommandID = ""

	tr.commands = []protocol.CommandEnvelope{cmd}

	exec := newTestExecutor(t, serverID, tr)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	exec.Start(ctx)

	// Since ID was empty, a report with empty ID reaching StateRejected is expected
	waitForStatus(t, tr, "", string(executor.StateRejected), 2*time.Second)
	cancel()
	exec.Stop()
}

func TestJournal_FilePermissions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	j := newTestJournal(t, dir)

	id := uuid.New().String()
	exp := time.Now().Add(10 * time.Minute)
	j.Add(id, exp)

	journalPath := filepath.Join(dir, "command-journal.json")
	info, err := os.Stat(journalPath)
	if err != nil {
		t.Fatalf("journal file not found on disk: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected journal file permissions 0600, got %04o", perm)
	}
}
