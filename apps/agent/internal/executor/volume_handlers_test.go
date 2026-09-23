package executor

import (
	"context"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestVolumeHandlers_RegistrationAndValidation(t *testing.T) {
	cli := &docker.Client{}
	reg := buildRegistry(cli, nil, nil, "", nil, nil)

	ops := []string{
		protocol.OpCreateVolume,
		protocol.OpRemoveVolume,
		protocol.OpAttachVolume,
		protocol.OpDetachVolume,
		protocol.OpInspectVolume,
	}

	for _, op := range ops {
		h, ok := reg[op]
		if !ok {
			t.Errorf("expected handler registered for %q", op)
			continue
		}
		// Empty payload validation
		_, err := h.Execute(context.Background(), map[string]any{})
		if err == nil {
			t.Errorf("expected validation error for empty payload on %q, got nil", op)
		}
	}
}
