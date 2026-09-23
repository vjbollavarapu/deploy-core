package cleanup

import (
	"context"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type mockCleanupDockerClient struct {
	containers []docker.ContainerSummary
	images     []docker.ImageSummary

	removedContainers []string
	removedImages     []string
}

func (m *mockCleanupDockerClient) ListContainers(ctx context.Context, all bool) ([]docker.ContainerSummary, error) {
	return m.containers, nil
}

func (m *mockCleanupDockerClient) RemoveContainer(ctx context.Context, id string, force bool) error {
	m.removedContainers = append(m.removedContainers, id)
	return nil
}

func (m *mockCleanupDockerClient) ListImages(ctx context.Context, all bool) ([]docker.ImageSummary, error) {
	return m.images, nil
}

func (m *mockCleanupDockerClient) RemoveImage(ctx context.Context, id string, force bool) error {
	m.removedImages = append(m.removedImages, id)
	return nil
}

type mockWorkspacePruner struct {
	prunedCount int
}

func (m *mockWorkspacePruner) Prune(maxAge time.Duration) (int, error) {
	return m.prunedCount, nil
}

func TestCleaner_SafeCleanup(t *testing.T) {
	cli := &mockCleanupDockerClient{
		containers: []docker.ContainerSummary{
			// 1. Unmanaged stopped container -> MUST NOT BE REMOVED
			{
				ID:    "c-unmanaged",
				State: "exited",
				Names: []string{"/my-custom-postgres"},
				Labels: map[string]string{
					"some.vendor": "true",
				},
			},
			// 2. Protected database container -> MUST NOT BE REMOVED
			{
				ID:    "c-db",
				State: "exited",
				Names: []string{"/dc-db-123"},
				Labels: map[string]string{
					protocol.LabelManaged:     "true",
					protocol.LabelProtected:   "true",
					"deploycore.service_type": "database",
				},
			},
			// 3. Active revision container -> MUST NOT BE REMOVED
			{
				ID:    "c-active-rev",
				State: "exited",
				Names: []string{"/dc-app-active"},
				Labels: map[string]string{
					protocol.LabelManaged:    "true",
					protocol.LabelRevisionID: "rev-active",
				},
			},
			// 4. Running managed container -> MUST NOT BE REMOVED
			{
				ID:    "c-running",
				State: "running",
				Names: []string{"/dc-app-running"},
				Labels: map[string]string{
					protocol.LabelManaged: "true",
				},
				ImageID: "img-in-use",
			},
			// 5. Stopped superseded managed container -> MUST BE REMOVED
			{
				ID:    "c-superseded",
				State: "exited",
				Names: []string{"/dc-app-old"},
				Labels: map[string]string{
					protocol.LabelManaged:    "true",
					protocol.LabelRevisionID: "rev-superseded",
				},
				ImageID: "img-superseded",
			},
		},
		images: []docker.ImageSummary{
			// Unmanaged image -> MUST NOT BE REMOVED
			{
				ID:       "img-unmanaged",
				RepoTags: []string{"alpine:latest"},
				Labels:   map[string]string{},
				Size:     5000000,
			},
			// Active revision image -> MUST NOT BE REMOVED
			{
				ID:       "img-active",
				RepoTags: []string{"app:rev-active"},
				Labels:   map[string]string{protocol.LabelManaged: "true"},
				Size:     20000000,
			},
			// In-use image -> MUST NOT BE REMOVED
			{
				ID:       "img-in-use",
				RepoTags: []string{"app:rev-running"},
				Labels:   map[string]string{protocol.LabelManaged: "true"},
				Size:     20000000,
			},
			// Unused managed image -> MUST BE REMOVED
			{
				ID:       "img-unused-managed",
				RepoTags: []string{"app:rev-old-dead"},
				Labels:   map[string]string{protocol.LabelManaged: "true"},
				Size:     15000000,
			},
		},
	}

	wsPruner := &mockWorkspacePruner{prunedCount: 3}
	cleaner := NewCleaner(cli, wsPruner, nil)

	policy := Policy{
		WorkspaceMaxAge:          24 * time.Hour,
		PruneStoppedContainers:   true,
		PruneUnusedManagedImages: true,
		ActiveRevisionIDs:        []string{"rev-active"},
		ActiveImageIDs:           []string{"img-active"},
	}

	report, err := cleaner.Cleanup(context.Background(), policy)
	if err != nil {
		t.Fatalf("Cleanup returned error: %v", err)
	}

	if report.PrunedWorkspaces != 3 {
		t.Errorf("expected 3 pruned workspaces, got %d", report.PrunedWorkspaces)
	}

	// Verify containers removed
	if len(report.RemovedContainers) != 1 || report.RemovedContainers[0] != "c-superseded" {
		t.Errorf("expected only 'c-superseded' to be removed, got: %v", report.RemovedContainers)
	}

	// Verify images removed
	if len(report.PrunedImages) != 1 || report.PrunedImages[0] != "img-unused-managed" {
		t.Errorf("expected only 'img-unused-managed' to be removed, got: %v", report.PrunedImages)
	}

	if report.ReclaimedBytes != 15000000 {
		t.Errorf("expected 15000000 reclaimed bytes, got %d", report.ReclaimedBytes)
	}
}
