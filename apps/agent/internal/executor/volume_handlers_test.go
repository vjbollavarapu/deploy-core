package executor

import (
	"context"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestCreateVolumePayloadLabelContract(t *testing.T) {
	boolLabels := map[string]any{
		"name":           "data",
		"organizationId": "org-1",
		"volumeId":       "vol-1",
		"labels": map[string]any{
			"readOnly":           false,
			"deploycore.managed": "true",
		},
	}
	if err := decodePayload(boolLabels, &createVolumePayload{}); err == nil {
		t.Fatal("expected boolean label to fail map[string]string decoding")
	}

	var decoded createVolumePayload
	if err := decodePayload(map[string]any{
		"name":           "data",
		"organizationId": "org-1",
		"volumeId":       "vol-1",
		"labels": map[string]string{
			"deploycore.managed":         "true",
			"deploycore.owner":           "platform",
			"deploycore.organization_id": "org-1",
		},
	}, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OrganizationID != "org-1" || decoded.VolumeID != "vol-1" || decoded.Name != "data" {
		t.Fatalf("structured metadata = %#v", decoded)
	}
	if decoded.Labels["readOnly"] != "" || decoded.Labels["deploycore.managed"] != "true" {
		t.Fatalf("labels = %#v", decoded.Labels)
	}
}

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
