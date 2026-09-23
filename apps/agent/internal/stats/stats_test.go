package stats

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type mockDockerClient struct {
	containers map[string]docker.ContainerDetail
	stats      map[string]docker.ContainerStatsSnapshot
	summaries  []docker.ContainerSummary
}

func (m *mockDockerClient) InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error) {
	d, ok := m.containers[id]
	if !ok {
		return docker.ContainerDetail{}, fmt.Errorf("container not found: %s", id)
	}
	return d, nil
}

func (m *mockDockerClient) ContainerStats(ctx context.Context, id string) (docker.ContainerStatsSnapshot, error) {
	s, ok := m.stats[id]
	if !ok {
		return docker.ContainerStatsSnapshot{}, fmt.Errorf("stats not found: %s", id)
	}
	return s, nil
}

func (m *mockDockerClient) ListContainers(ctx context.Context, all bool) ([]docker.ContainerSummary, error) {
	return m.summaries, nil
}

func TestCollector_GetContainerStats(t *testing.T) {
	cli := &mockDockerClient{
		containers: map[string]docker.ContainerDetail{
			"c-1": {
				ID:           "c-1",
				Name:         "/dc-app-rev1",
				RestartCount: 2,
				State: docker.ContainerState{
					Status:  "running",
					Running: true,
				},
				Labels: map[string]string{
					protocol.LabelManaged:       "true",
					protocol.LabelApplicationID: "app-123",
					protocol.LabelRevisionID:    "rev-456",
				},
			},
		},
		stats: map[string]docker.ContainerStatsSnapshot{
			"c-1": {
				ID:          "c-1",
				CPUPercent:  12.5,
				MemoryUsage: 256 * 1024 * 1024,
				MemoryLimit: 1024 * 1024 * 1024,
				NetworkRx:   5000,
				NetworkTx:   8000,
				BlockRead:   1024,
				BlockWrite:  2048,
				PidsCurrent: 18,
			},
		},
	}

	collector := NewCollector(cli)
	stat, err := collector.GetContainerStats(context.Background(), "c-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stat.ContainerID != "c-1" {
		t.Errorf("expected container ID 'c-1', got %s", stat.ContainerID)
	}
	if stat.ApplicationID != "app-123" {
		t.Errorf("expected app ID 'app-123', got %s", stat.ApplicationID)
	}
	if stat.RevisionID != "rev-456" {
		t.Errorf("expected rev ID 'rev-456', got %s", stat.RevisionID)
	}
	if stat.CPUPercent != 12.5 {
		t.Errorf("expected CPU 12.5, got %f", stat.CPUPercent)
	}
	if stat.MemoryUsage != 256*1024*1024 {
		t.Errorf("expected memory usage 256MB, got %d", stat.MemoryUsage)
	}
	if stat.RestartCount != 2 {
		t.Errorf("expected restart count 2, got %d", stat.RestartCount)
	}
	if stat.PidsCurrent != 18 {
		t.Errorf("expected 18 pids, got %d", stat.PidsCurrent)
	}
	if stat.BlockRead != 1024 || stat.BlockWrite != 2048 {
		t.Errorf("unexpected block IO: read=%d, write=%d", stat.BlockRead, stat.BlockWrite)
	}
}

func TestCollector_CollectAllManaged(t *testing.T) {
	cli := &mockDockerClient{
		summaries: []docker.ContainerSummary{
			{
				ID:    "c-managed",
				Names: []string{"/dc-managed"},
				Labels: map[string]string{
					protocol.LabelManaged: "true",
				},
			},
			{
				ID:    "c-unmanaged",
				Names: []string{"/my-redis"},
				Labels: map[string]string{
					"some.label": "foo",
				},
			},
		},
		containers: map[string]docker.ContainerDetail{
			"c-managed": {
				ID:           "c-managed",
				Name:         "/dc-managed",
				State:        docker.ContainerState{Running: true, Status: "running"},
				RestartCount: 0,
				Labels: map[string]string{
					protocol.LabelManaged: "true",
				},
			},
		},
		stats: map[string]docker.ContainerStatsSnapshot{
			"c-managed": {
				ID:          "c-managed",
				CPUPercent:  5.0,
				MemoryUsage: 100 * 1024 * 1024,
			},
		},
	}

	collector := NewCollector(cli)
	results, err := collector.CollectAllManaged(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 managed container result, got %d", len(results))
	}
	if results[0].ContainerID != "c-managed" {
		t.Errorf("expected 'c-managed', got %s", results[0].ContainerID)
	}
}

func TestSampler_SlidingWindow(t *testing.T) {
	cli := &mockDockerClient{
		summaries: []docker.ContainerSummary{
			{
				ID:     "c-1",
				Labels: map[string]string{protocol.LabelManaged: "true"},
			},
		},
		containers: map[string]docker.ContainerDetail{
			"c-1": {
				ID:     "c-1",
				Name:   "/dc-1",
				State:  docker.ContainerState{Running: true, Status: "running"},
				Labels: map[string]string{protocol.LabelManaged: "true"},
			},
		},
		stats: map[string]docker.ContainerStatsSnapshot{
			"c-1": {
				ID:         "c-1",
				CPUPercent: 1.0,
			},
		},
	}

	collector := NewCollector(cli)
	cfg := SamplerConfig{
		Interval:           20 * time.Millisecond,
		MaxSamplesPerEntry: 3,
	}
	sampler := NewSampler(collector, cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sampler.Start(ctx)

	// Wait enough intervals for more than 3 samples
	time.Sleep(100 * time.Millisecond)
	sampler.Stop()

	latest, ok := sampler.GetLatest("c-1")
	if !ok || latest == nil {
		t.Fatalf("expected latest sample for c-1")
	}

	samples := sampler.GetSamples("c-1")
	if len(samples) > 3 {
		t.Errorf("expected max 3 samples retained, got %d", len(samples))
	}
	if len(samples) == 0 {
		t.Errorf("expected at least 1 sample, got 0")
	}
}
