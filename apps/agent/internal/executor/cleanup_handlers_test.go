package executor

import (
	"context"
	"testing"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestDispatchRegistry_ContainsOpCleanupDisk(t *testing.T) {
	r := buildRegistry(nil, nil, nil, "", nil, nil)
	if _, ok := r[protocol.OpCleanupDisk]; !ok {
		t.Fatalf("expected registry to contain %s", protocol.OpCleanupDisk)
	}
}

func TestCleanupDiskHandler_Dispatch(t *testing.T) {
	h := cleanupDiskHandler(nil, nil, nil)
	res, err := h.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("expected nil error on empty cleanup execution, got %v", err)
	}
	if res.Output["report"] == nil {
		t.Errorf("expected report in cleanup result")
	}
}
