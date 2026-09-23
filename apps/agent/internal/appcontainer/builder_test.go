package appcontainer

import (
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestBuildCreateRequest(t *testing.T) {
	spec := Spec{
		Metadata: Metadata{
			OrganizationID: "org-123",
			ApplicationID:  "app-456",
			EnvironmentID:  "env-789",
			DeploymentID:   "dep-101",
			RevisionID:     "49",
			Instance:       1,
			AppShortID:     "dayaapi",
		},
		Image:       "ghcr.io/deploycore/dayaapi:v49",
		Env:         []string{"PORT=8080", "NODE_ENV=production"},
		CPUMillis:   1000,
		MemoryBytes: 512 * 1024 * 1024,
		InternalPorts: []docker.PortMapping{
			{ContainerPort: 8080, Protocol: "tcp"},
		},
		Traefik: &docker.TraefikConfig{
			Enabled:     true,
			Host:        "daya.example.com",
			Port:        8080,
			ServiceName: "dayaapi",
		},
	}

	req, err := BuildCreateRequest(spec)
	if err != nil {
		t.Fatalf("unexpected builder error: %v", err)
	}

	// Verify standard naming
	expectedName := "dc-dayaapi-r49-1"
	if req.Name != expectedName {
		t.Errorf("expected container name %q, got %q", expectedName, req.Name)
	}

	// Verify trusted platform labels
	if req.PlatformLabels[protocol.LabelManaged] != "true" {
		t.Errorf("expected %s='true', got %q", protocol.LabelManaged, req.PlatformLabels[protocol.LabelManaged])
	}
	if req.PlatformLabels[protocol.LabelOrganizationID] != "org-123" {
		t.Errorf("expected org-123, got %q", req.PlatformLabels[protocol.LabelOrganizationID])
	}
	if req.PlatformLabels[protocol.LabelRevisionID] != "49" {
		t.Errorf("expected rev 49, got %q", req.PlatformLabels[protocol.LabelRevisionID])
	}

	// Verify request payload fields
	if req.Image != spec.Image {
		t.Errorf("image mismatch")
	}
	if req.CPUMillis != 1000 || req.MemoryBytes != 512*1024*1024 {
		t.Errorf("resource limits mismatch")
	}
	if req.Traefik == nil || req.Traefik.Host != "daya.example.com" {
		t.Errorf("traefik config mismatch")
	}
}

func TestBuildCreateRequest_InvalidMetadata(t *testing.T) {
	spec := Spec{
		Metadata: Metadata{
			// missing organization ID
			ApplicationID: "app-456",
			EnvironmentID: "env-789",
			DeploymentID:  "dep-101",
			RevisionID:    "49",
			Instance:      1,
			AppShortID:    "dayaapi",
		},
		Image: "ghcr.io/deploycore/dayaapi:v49",
	}

	_, err := BuildCreateRequest(spec)
	if err == nil {
		t.Errorf("expected error for invalid metadata, got nil")
	}
}
