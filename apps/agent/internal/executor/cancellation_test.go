package executor

import (
	"context"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type mockCanceler struct {
	cancelledID string
}

func (m *mockCanceler) CancelCommand(id string) bool {
	m.cancelledID = id
	return true
}

func TestCancelCommandHandler_Validation(t *testing.T) {
	h := cancelCommandHandler(nil, nil)
	_, err := h.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error on empty payload")
	}
}

func TestCancelCommandHandler_Success(t *testing.T) {
	mc := &mockCanceler{}
	h := cancelCommandHandler(mc, nil)

	res, err := h.Execute(context.Background(), map[string]any{
		"targetCommandId": "cmd-1234",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mc.cancelledID != "cmd-1234" {
		t.Errorf("expected cancelled id cmd-1234, got %s", mc.cancelledID)
	}

	if res.Output["cancelled"] != true {
		t.Errorf("expected cancelled=true in output")
	}
}

func TestExecutor_CancelRunningCommand(t *testing.T) {
	ex := &Executor{
		activeCancels: make(map[string]context.CancelFunc),
	}

	ctx, cancel := context.WithCancel(context.Background())
	ex.activeCancels["active-1"] = cancel

	ok := ex.CancelCommand("active-1")
	if !ok {
		t.Fatalf("expected CancelCommand to return true")
	}

	select {
	case <-ctx.Done():
		// Success! Context was cancelled
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("expected context to be cancelled")
	}

	// Second cancel on non-existent command returns false
	if ex.CancelCommand("not-found") {
		t.Errorf("expected CancelCommand to return false for unknown command")
	}
}

func TestDispatchRegistry_ContainsOpCancelCommand(t *testing.T) {
	r := buildRegistry(nil, nil, nil, "", nil, nil)
	if _, ok := r[protocol.OpCancelCommand]; !ok {
		t.Fatalf("expected registry to contain %s", protocol.OpCancelCommand)
	}
}
