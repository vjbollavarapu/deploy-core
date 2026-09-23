package appcontainer

import (
	"context"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type mockContainerReader struct {
	containers []docker.ContainerSummary
}

func (m *mockContainerReader) ListContainers(ctx context.Context, all bool) ([]docker.ContainerSummary, error) {
	return m.containers, nil
}

func TestDiscover_FilteringAndCollisions(t *testing.T) {
	orgID := "org-1"
	appID := "app-daya"
	envID := "env-prod"

	meta1 := Metadata{
		OrganizationID: orgID,
		ApplicationID:  appID,
		EnvironmentID:  envID,
		DeploymentID:   "dep-1",
		RevisionID:     "rev-49",
		Instance:       1,
		AppShortID:     "dayaapi",
	}

	meta2 := Metadata{
		OrganizationID: orgID,
		ApplicationID:  appID,
		EnvironmentID:  envID,
		DeploymentID:   "dep-1",
		RevisionID:     "rev-49",
		Instance:       2,
		AppShortID:     "dayaapi",
	}

	otherTenantMeta := Metadata{
		OrganizationID: "org-2", // different org!
		ApplicationID:  appID,
		EnvironmentID:  envID,
		DeploymentID:   "dep-99",
		RevisionID:     "rev-49",
		Instance:       1,
		AppShortID:     "dayaapi",
	}

	reader := &mockContainerReader{
		containers: []docker.ContainerSummary{
			{
				ID:      "cid-1",
				Names:   []string{"/dc-dayaapi-r49-1"},
				State:   "running",
				Created: time.Now().UTC(),
				Labels:  meta1.Labels(),
			},
			{
				ID:      "cid-2",
				Names:   []string{"/dc-dayaapi-r49-2"},
				State:   "running",
				Created: time.Now().UTC(),
				Labels:  meta2.Labels(),
			},
			{
				ID:      "cid-collision-1",
				Names:   []string{"/dc-dayaapi-r49-3"},
				State:   "running",
				Created: time.Now().UTC(),
				Labels:  map[string]string{}, // Untrusted! No labels!
			},
			{
				ID:      "cid-tenant-collision",
				Names:   []string{"/dc-dayaapi-r49-4"},
				State:   "running",
				Created: time.Now().UTC(),
				Labels:  otherTenantMeta.Labels(), // Cross-tenant!
			},
			{
				ID:      "cid-unmanaged",
				Names:   []string{"/traefik-proxy"},
				State:   "running",
				Created: time.Now().UTC(),
				Labels:  map[string]string{protocol.LabelManaged: "false"},
			},
		},
	}

	filter := DiscoveryFilter{
		OrganizationID: orgID,
		ApplicationID:  appID,
	}

	res, err := Discover(context.Background(), reader, filter)
	if err != nil {
		t.Fatalf("unexpected discovery error: %v", err)
	}

	if len(res.Containers) != 2 {
		t.Fatalf("expected 2 verified containers, got %d", len(res.Containers))
	}
	if res.Containers[0].ID != "cid-1" || res.Containers[1].ID != "cid-2" {
		t.Errorf("unexpected containers returned: %+v", res.Containers)
	}

	if len(res.Collisions) != 2 {
		t.Fatalf("expected 2 collisions (untrusted + tenant mismatch), got %d", len(res.Collisions))
	}

	foundUntrusted := false
	foundTenantMismatch := false
	for _, col := range res.Collisions {
		if col.OwnershipStatus == OwnershipUntrustedCollision && col.Name == "dc-dayaapi-r49-3" {
			foundUntrusted = true
		}
		if col.OwnershipStatus == OwnershipTenantMismatch && col.Name == "dc-dayaapi-r49-4" {
			foundTenantMismatch = true
		}
	}

	if !foundUntrusted {
		t.Errorf("expected untrusted collision for dc-dayaapi-r49-3")
	}
	if !foundTenantMismatch {
		t.Errorf("expected tenant mismatch collision for dc-dayaapi-r49-4")
	}
}
