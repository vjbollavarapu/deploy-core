package reconciliation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type mockReconDockerClient struct {
	containers []docker.ContainerSummary
	details    map[string]docker.ContainerDetail
	networks   []docker.NetworkSummary
	volumes    []docker.VolumeSummary
	restarted  []string
}

func (m *mockReconDockerClient) ListContainers(ctx context.Context, all bool) ([]docker.ContainerSummary, error) {
	return m.containers, nil
}

func (m *mockReconDockerClient) InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error) {
	d, ok := m.details[id]
	if !ok {
		return docker.ContainerDetail{}, fmt.Errorf("container not found: %s", id)
	}
	return d, nil
}

func (m *mockReconDockerClient) RestartContainer(ctx context.Context, id string, timeout time.Duration) error {
	m.restarted = append(m.restarted, id)
	return nil
}

func (m *mockReconDockerClient) ListNetworks(ctx context.Context) ([]docker.NetworkSummary, error) {
	return m.networks, nil
}

func (m *mockReconDockerClient) ListVolumes(ctx context.Context) ([]docker.VolumeSummary, error) {
	return m.volumes, nil
}

func TestReconciler_AllDiscrepancies(t *testing.T) {
	cli := &mockReconDockerClient{
		containers: []docker.ContainerSummary{
			{
				ID:    "cnt-exited",
				Names: []string{"/dc-app-worker"},
				State: "exited",
				Labels: map[string]string{
					protocol.LabelManaged: "true",
				},
			},
			{
				ID:    "cnt-unhealthy",
				Names: []string{"/dc-app-web"},
				State: "running",
				Labels: map[string]string{
					protocol.LabelManaged: "true",
				},
			},
			{
				ID:    "cnt-unexpected",
				Names: []string{"/dc-old-orphan"},
				State: "running",
				Labels: map[string]string{
					protocol.LabelManaged: "true",
				},
			},
		},
		details: map[string]docker.ContainerDetail{
			"cnt-exited": {
				ID:   "cnt-exited",
				Name: "/dc-app-worker",
				State: docker.ContainerState{
					Running:  false,
					Status:   "exited",
					ExitCode: 1,
				},
			},
			"cnt-unhealthy": {
				ID:   "cnt-unhealthy",
				Name: "/dc-app-web",
				State: docker.ContainerState{
					Running: true,
					Status:  "running",
					Health: &docker.ContainerHealth{
						Status:        "unhealthy",
						FailingStreak: 3,
					},
				},
			},
			"cnt-unexpected": {
				ID:   "cnt-unexpected",
				Name: "/dc-old-orphan",
				State: docker.ContainerState{
					Running: true,
					Status:  "running",
				},
			},
		},
		networks: []docker.NetworkSummary{
			{
				Name:   "dc-net-prod",
				Labels: map[string]string{protocol.LabelManaged: "true"},
			},
		},
		volumes: []docker.VolumeSummary{
			{
				Name:   "dc-vol-data",
				Labels: map[string]string{protocol.LabelManaged: "true"},
			},
		},
	}

	reconciler := NewReconciler(cli, nil)

	desired := DesiredState{
		Containers: []DesiredContainer{
			{
				ContainerName: "dc-app-worker",
				DesiredStatus: "running",
			},
			{
				ContainerName: "dc-app-web",
				DesiredStatus: "running",
			},
			{
				ContainerName: "dc-app-missing",
				DesiredStatus: "running",
			},
		},
		Networks:               []string{"dc-net-prod", "dc-net-missing"},
		Volumes:                []string{"dc-vol-data", "dc-vol-missing"},
		AllowSafeRestartExited: true, // Should trigger safe restart for worker!
	}

	report, err := reconciler.Reconcile(context.Background(), desired)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Healthy {
		t.Errorf("expected unhealthy report due to discrepancies")
	}

	foundMissingCnt := false
	foundExited := false
	foundUnhealthy := false
	foundUnexpected := false
	foundMissingNet := false
	foundMissingVol := false

	for _, d := range report.Discrepancies {
		switch d.Type {
		case DiscrepancyMissingContainer:
			if d.ResourceName == "dc-app-missing" {
				foundMissingCnt = true
			}
		case DiscrepancyExitedContainer:
			if d.ResourceName == "dc-app-worker" {
				foundExited = true
				if d.ActionTaken != "restarted" {
					t.Errorf("expected exited container to be restarted, got %s", d.ActionTaken)
				}
			}
		case DiscrepancyUnhealthyContainer:
			if d.ResourceName == "dc-app-web" {
				foundUnhealthy = true
			}
		case DiscrepancyUnexpectedContainer:
			if d.ResourceName == "dc-old-orphan" {
				foundUnexpected = true
				if d.ActionTaken != "reported" {
					t.Errorf("expected unexpected container action to be reported only, got %s", d.ActionTaken)
				}
			}
		case DiscrepancyMissingNetwork:
			if d.ResourceName == "dc-net-missing" {
				foundMissingNet = true
			}
		case DiscrepancyMissingVolume:
			if d.ResourceName == "dc-vol-missing" {
				foundMissingVol = true
			}
		}
	}

	if !foundMissingCnt {
		t.Errorf("missing expected container not reported")
	}
	if !foundExited {
		t.Errorf("exited container not reported")
	}
	if !foundUnhealthy {
		t.Errorf("unhealthy container not reported")
	}
	if !foundUnexpected {
		t.Errorf("unexpected container not reported")
	}
	if !foundMissingNet {
		t.Errorf("missing network not reported")
	}
	if !foundMissingVol {
		t.Errorf("missing volume not reported")
	}

	if len(cli.restarted) != 1 || cli.restarted[0] != "cnt-exited" {
		t.Errorf("expected cnt-exited to be restarted, got %v", cli.restarted)
	}
}

func TestReconciler_Healthy(t *testing.T) {
	cli := &mockReconDockerClient{
		containers: []docker.ContainerSummary{
			{
				ID:    "c1",
				Names: []string{"/dc-app-prod"},
				State: "running",
				Labels: map[string]string{
					protocol.LabelManaged: "true",
				},
			},
		},
		details: map[string]docker.ContainerDetail{
			"c1": {
				ID:   "c1",
				Name: "/dc-app-prod",
				State: docker.ContainerState{
					Running: true,
					Status:  "running",
				},
			},
		},
		networks: []docker.NetworkSummary{
			{Name: "dc-net", Labels: map[string]string{protocol.LabelManaged: "true"}},
		},
		volumes: []docker.VolumeSummary{
			{Name: "dc-vol", Labels: map[string]string{protocol.LabelManaged: "true"}},
		},
	}

	reconciler := NewReconciler(cli, nil)
	desired := DesiredState{
		Containers: []DesiredContainer{
			{ContainerName: "dc-app-prod", DesiredStatus: "running"},
		},
		Networks: []string{"dc-net"},
		Volumes:  []string{"dc-vol"},
	}

	report, err := reconciler.Reconcile(context.Background(), desired)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !report.Healthy {
		t.Errorf("expected report to be healthy, got discrepancies: %+v", report.Discrepancies)
	}
	if len(report.Discrepancies) != 0 {
		t.Errorf("expected 0 discrepancies, got %d", len(report.Discrepancies))
	}
}
