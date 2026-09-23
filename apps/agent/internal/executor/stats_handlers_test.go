package executor

import (
	"context"
	"testing"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestDispatchRegistry_ContainsOpCollectStats(t *testing.T) {
	r := buildRegistry(nil, nil, nil, "", nil, nil)
	if _, ok := r[protocol.OpCollectStats]; !ok {
		t.Fatalf("expected registry to contain %s", protocol.OpCollectStats)
	}
}

func TestCollectStatsHandler_Dispatch(t *testing.T) {
	h := collectStatsHandler(nil, nil)
	// Without docker client, expect error
	_, err := h.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatalf("expected error without docker client, got nil")
	}
}
