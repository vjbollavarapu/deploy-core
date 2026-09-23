package heartbeat_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/heartbeat"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type mockSystemCollector struct {
	metrics heartbeat.SystemMetrics
	err     error
}

func (m *mockSystemCollector) CollectSystemMetrics(ctx context.Context) (heartbeat.SystemMetrics, error) {
	return m.metrics, m.err
}

type mockDockerProvider struct {
	metrics docker.DockerMetrics
	err     error
	calls   int
}

func (m *mockDockerProvider) GetDockerMetrics(ctx context.Context) (docker.DockerMetrics, error) {
	m.calls++
	return m.metrics, m.err
}

func TestSampler_Sample_Healthy(t *testing.T) {
	sys := &mockSystemCollector{
		metrics: heartbeat.SystemMetrics{
			Hostname:         "srv-prod-01",
			OS:               "linux",
			Architecture:     "x86_64",
			UptimeSeconds:    3600,
			CPUCores:         8,
			CPUPercent:       14.5,
			MemoryTotalBytes: 16 * 1024 * 1024 * 1024,
			MemoryUsedBytes:  4 * 1024 * 1024 * 1024,
			DiskTotalBytes:   100 * 1024 * 1024 * 1024,
			DiskUsedBytes:    25 * 1024 * 1024 * 1024,
			Load1:            1.25,
		},
	}

	docVer := "24.0.7"
	doc := &mockDockerProvider{
		metrics: docker.DockerMetrics{
			Version:           docVer,
			ContainerCount:    10,
			RunningContainers: 8,
			ImageCount:        25,
			VolumeCount:       6,
			NetworkCount:      4,
		},
	}

	sampler := heartbeat.NewSampler(sys, doc)
	ctx := context.Background()

	hb := sampler.Sample(ctx, true)
	var _ protocol.HeartbeatRequest = hb

	// Validate against protocol-go wire validation rules
	if err := hb.Validate(); err != nil {
		t.Fatalf("heartbeat validation failed: %v", err)
	}

	// Verify all inventory fields required by Phase A4
	if hb.Hostname != "srv-prod-01" {
		t.Errorf("unexpected Hostname: %s", hb.Hostname)
	}
	if hb.OS != "linux" {
		t.Errorf("unexpected OS: %s", hb.OS)
	}
	if hb.Architecture != "x86_64" {
		t.Errorf("unexpected Architecture: %s", hb.Architecture)
	}
	if hb.CPUCores != 8 {
		t.Errorf("unexpected CPUCores: %d", hb.CPUCores)
	}
	if hb.CPUPercent == nil || *hb.CPUPercent != 14.5 {
		t.Errorf("unexpected CPUPercent: %v", hb.CPUPercent)
	}
	if hb.MemoryTotalBytes != 16*1024*1024*1024 {
		t.Errorf("unexpected MemoryTotalBytes: %d", hb.MemoryTotalBytes)
	}
	if hb.MemoryUsedBytes == nil || *hb.MemoryUsedBytes != 4*1024*1024*1024 {
		t.Errorf("unexpected MemoryUsedBytes: %v", hb.MemoryUsedBytes)
	}
	if hb.DiskTotalBytes != 100*1024*1024*1024 {
		t.Errorf("unexpected DiskTotalBytes: %d", hb.DiskTotalBytes)
	}
	if hb.DiskUsedBytes == nil || *hb.DiskUsedBytes != 25*1024*1024*1024 {
		t.Errorf("unexpected DiskUsedBytes: %v", hb.DiskUsedBytes)
	}
	if hb.Load1 == nil || *hb.Load1 != 1.25 {
		t.Errorf("unexpected Load1: %v", hb.Load1)
	}
	if hb.UptimeSeconds == nil || *hb.UptimeSeconds != 3600 {
		t.Errorf("unexpected UptimeSeconds: %v", hb.UptimeSeconds)
	}

	// Docker inventory
	if hb.DockerStatus != "ONLINE" {
		t.Errorf("unexpected DockerStatus: %s", hb.DockerStatus)
	}
	if hb.DockerVersion == nil || *hb.DockerVersion != docVer {
		t.Errorf("unexpected DockerVersion: %v", hb.DockerVersion)
	}
	if hb.ContainerCount == nil || *hb.ContainerCount != 10 {
		t.Errorf("unexpected ContainerCount: %v", hb.ContainerCount)
	}
	if hb.RunningContainers != 8 {
		t.Errorf("unexpected RunningContainers: %d", hb.RunningContainers)
	}
	if hb.ImageCount != 25 {
		t.Errorf("unexpected ImageCount: %d", hb.ImageCount)
	}
	if hb.VolumeCount != 6 {
		t.Errorf("unexpected VolumeCount: %d", hb.VolumeCount)
	}
	if hb.NetworkCount != 4 {
		t.Errorf("unexpected NetworkCount: %d", hb.NetworkCount)
	}

	// State
	if hb.AgentState != "HEALTHY" {
		t.Errorf("expected state HEALTHY, got %s", hb.AgentState)
	}
}

func TestSampler_StateTransitions(t *testing.T) {
	sys := &mockSystemCollector{
		metrics: heartbeat.SystemMetrics{Hostname: "host-1", OS: "linux"},
	}

	tests := []struct {
		name               string
		dockerErr          error
		docNil             bool
		transportConnected bool
		expectedState      string
		expectedDocker     string
	}{
		{
			name:               "both healthy",
			dockerErr:          nil,
			transportConnected: true,
			expectedState:      "HEALTHY",
			expectedDocker:     "ONLINE",
		},
		{
			name:               "docker failing, transport ok -> DEGRADED",
			dockerErr:          errors.New("daemon connection refused"),
			transportConnected: true,
			expectedState:      "DEGRADED",
			expectedDocker:     "OFFLINE",
		},
		{
			name:               "docker nil, transport ok -> DEGRADED",
			docNil:             true,
			transportConnected: true,
			expectedState:      "DEGRADED",
			expectedDocker:     "OFFLINE",
		},
		{
			name:               "docker ok, transport disconnected -> DEGRADED",
			dockerErr:          nil,
			transportConnected: false,
			expectedState:      "DEGRADED",
			expectedDocker:     "ONLINE",
		},
		{
			name:               "both docker and transport failing -> ERROR",
			dockerErr:          errors.New("daemon offline"),
			transportConnected: false,
			expectedState:      "ERROR",
			expectedDocker:     "OFFLINE",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var doc heartbeat.DockerProvider
			if !tc.docNil {
				doc = &mockDockerProvider{
					metrics: docker.DockerMetrics{Version: "24.0"},
					err:     tc.dockerErr,
				}
			}

			sampler := heartbeat.NewSampler(sys, doc)
			hb := sampler.Sample(context.Background(), tc.transportConnected)

			if hb.AgentState != tc.expectedState {
				t.Errorf("expected state %s, got %s", tc.expectedState, hb.AgentState)
			}
			if hb.DockerStatus != tc.expectedDocker {
				t.Errorf("expected docker status %s, got %s", tc.expectedDocker, hb.DockerStatus)
			}
			if err := hb.Validate(); err != nil {
				t.Fatalf("heartbeat validation failed: %v", err)
			}
		})
	}
}

func TestSampler_MonotonicSequence(t *testing.T) {
	sampler := heartbeat.NewSampler(nil, nil)
	s1 := sampler.NextSequence()
	s2 := sampler.NextSequence()
	s3 := sampler.NextSequence()

	if s1 != 1 || s2 != 2 || s3 != 3 {
		t.Errorf("unexpected sequence values: %d, %d, %d", s1, s2, s3)
	}

	if sampler.Sequence() != 3 {
		t.Errorf("expected sequence 3, got %d", sampler.Sequence())
	}

	// Calling Sample also advances sequence monotonically
	sampler.Sample(context.Background(), true)
	if sampler.Sequence() != 4 {
		t.Errorf("expected sequence 4 after Sample, got %d", sampler.Sequence())
	}
}

func TestSampler_DefaultSystemCollector(t *testing.T) {
	collector := &heartbeat.DefaultSystemCollector{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	m, err := collector.CollectSystemMetrics(ctx)
	if err != nil {
		t.Fatalf("failed to collect system metrics: %v", err)
	}

	if m.Hostname == "" {
		t.Error("expected non-empty hostname")
	}
	if m.OS == "" {
		t.Error("expected non-empty OS")
	}
	if m.Architecture == "" {
		t.Error("expected non-empty architecture")
	}
	if m.CPUCores <= 0 {
		t.Errorf("expected CPU cores > 0, got %d", m.CPUCores)
	}
}
