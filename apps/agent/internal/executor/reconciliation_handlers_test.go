package executor

import (
	"context"
	"testing"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestDispatchRegistry_ContainsOpReconcile(t *testing.T) {
	r := buildRegistry(nil, nil, nil, "", nil, nil)
	if _, ok := r[protocol.OpReconcile]; !ok {
		t.Fatalf("expected registry to contain %s", protocol.OpReconcile)
	}
}

func TestReconcileHandler_Dispatch(t *testing.T) {
	h := reconcileHandler(nil, nil)
	// Without docker client, expect error
	_, err := h.Execute(context.Background(), map[string]any{
		"containers": []any{},
	})
	if err == nil {
		t.Fatalf("expected error without docker client, got nil")
	}
}
