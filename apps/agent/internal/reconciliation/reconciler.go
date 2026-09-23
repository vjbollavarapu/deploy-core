package reconciliation

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// DockerClient defines the Docker operations needed by the Reconciler.
type DockerClient interface {
	ListContainers(ctx context.Context, all bool) ([]docker.ContainerSummary, error)
	InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error)
	RestartContainer(ctx context.Context, id string, timeout time.Duration) error
	ListNetworks(ctx context.Context) ([]docker.NetworkSummary, error)
	ListVolumes(ctx context.Context) ([]docker.VolumeSummary, error)
}

// Reconciler inventories managed local resources and reconciles them against Control Plane desired state.
type Reconciler struct {
	cli DockerClient
	log *slog.Logger
}

// NewReconciler constructs a Reconciler.
func NewReconciler(cli DockerClient, log *slog.Logger) *Reconciler {
	if log == nil {
		log = slog.Default()
	}
	return &Reconciler{cli: cli, log: log}
}

// Inventory collects all managed resources currently on the host.
func (r *Reconciler) Inventory(ctx context.Context) (*LocalInventory, error) {
	containers, err := r.cli.ListContainers(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var managedContainers []docker.ContainerSummary
	for _, c := range containers {
		if isManaged(c.Labels) {
			managedContainers = append(managedContainers, c)
		}
	}

	networks, err := r.cli.ListNetworks(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}
	var managedNetworks []docker.NetworkSummary
	for _, n := range networks {
		if isManaged(n.Labels) {
			managedNetworks = append(managedNetworks, n)
		}
	}

	volumes, err := r.cli.ListVolumes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes: %w", err)
	}
	var managedVolumes []docker.VolumeSummary
	for _, v := range volumes {
		if isManaged(v.Labels) {
			managedVolumes = append(managedVolumes, v)
		}
	}

	return &LocalInventory{
		Containers: managedContainers,
		Networks:   managedNetworks,
		Volumes:    managedVolumes,
		Timestamp:  time.Now().UTC(),
	}, nil
}

// Reconcile compares local observed state to the Control Plane's desired state.
// Agent does NOT perform arbitrary deletion or deployment decisions.
// Simple safe local restarts of exited containers are executed only if explicitly permitted.
func (r *Reconciler) Reconcile(ctx context.Context, desired DesiredState) (*ReconciliationReport, error) {
	inv, err := r.Inventory(ctx)
	if err != nil {
		return nil, err
	}

	var discrepancies []Discrepancy

	// Index observed managed containers by clean name
	observedByName := make(map[string]docker.ContainerSummary, len(inv.Containers))
	matchedObserved := make(map[string]bool)

	for _, c := range inv.Containers {
		for _, name := range c.Names {
			clean := strings.TrimPrefix(name, "/")
			observedByName[clean] = c
		}
	}

	// 1. Check Desired Containers against Observed Containers
	for _, desiredCnt := range desired.Containers {
		desiredName := strings.TrimPrefix(desiredCnt.ContainerName, "/")
		obs, found := observedByName[desiredName]

		if !found {
			discrepancies = append(discrepancies, Discrepancy{
				Type:          DiscrepancyMissingContainer,
				ResourceName:  desiredName,
				ApplicationID: desiredCnt.ApplicationID,
				RevisionID:    desiredCnt.RevisionID,
				ObservedState: "missing",
				DesiredState:  desiredCnt.DesiredStatus,
				ActionTaken:   "reported",
				Details:       "expected container was not found on host",
			})
			continue
		}

		matchedObserved[obs.ID] = true

		// Inspect detailed state for running/exited/health
		detail, err := r.cli.InspectContainer(ctx, obs.ID)
		if err != nil {
			r.log.Warn("failed to inspect observed container", slog.String("id", obs.ID), slog.String("error", err.Error()))
			continue
		}

		expectedRunning := desiredCnt.DesiredStatus == "running" || desiredCnt.DesiredStatus == ""
		isExited := !detail.State.Running

		if expectedRunning && isExited {
			action := "reported"
			details := fmt.Sprintf("container exited with code %d (status: %s)", detail.State.ExitCode, detail.State.Status)

			// Simple locally safe restart policy if explicitly authorized
			if desired.AllowSafeRestartExited || desiredCnt.AutoRestartExited {
				r.log.Info("attempting authorized safe local restart of exited container", slog.String("id", obs.ID), slog.String("name", desiredName))
				if restartErr := r.cli.RestartContainer(ctx, obs.ID, 10*time.Second); restartErr != nil {
					action = "failed_restart"
					details += fmt.Sprintf("; restart attempt failed: %s", restartErr.Error())
				} else {
					action = "restarted"
					details += "; successfully restarted locally"
				}
			}

			discrepancies = append(discrepancies, Discrepancy{
				Type:          DiscrepancyExitedContainer,
				ResourceID:    obs.ID,
				ResourceName:  desiredName,
				ApplicationID: desiredCnt.ApplicationID,
				RevisionID:    desiredCnt.RevisionID,
				ObservedState: detail.State.Status,
				DesiredState:  desiredCnt.DesiredStatus,
				ActionTaken:   action,
				Details:       details,
			})
		} else if detail.State.Health != nil && detail.State.Health.Status == "unhealthy" {
			discrepancies = append(discrepancies, Discrepancy{
				Type:          DiscrepancyUnhealthyContainer,
				ResourceID:    obs.ID,
				ResourceName:  desiredName,
				ApplicationID: desiredCnt.ApplicationID,
				RevisionID:    desiredCnt.RevisionID,
				ObservedState: "unhealthy",
				DesiredState:  "healthy",
				ActionTaken:   "reported",
				Details:       fmt.Sprintf("health check failing streak: %d", detail.State.Health.FailingStreak),
			})
		}
	}

	// 2. Check Observed Managed Containers for Unexpected Managed Containers
	for _, obs := range inv.Containers {
		if !matchedObserved[obs.ID] {
			var name string
			if len(obs.Names) > 0 {
				name = strings.TrimPrefix(obs.Names[0], "/")
			} else {
				name = obs.ID
			}

			appID := ""
			revID := ""
			if obs.Labels != nil {
				appID = obs.Labels[protocol.LabelApplicationID]
				revID = obs.Labels[protocol.LabelRevisionID]
			}

			discrepancies = append(discrepancies, Discrepancy{
				Type:          DiscrepancyUnexpectedContainer,
				ResourceID:    obs.ID,
				ResourceName:  name,
				ApplicationID: appID,
				RevisionID:    revID,
				ObservedState: obs.State,
				DesiredState:  "none",
				ActionTaken:   "reported", // Agent never destroys unexpected containers without CP command!
				Details:       "managed container exists on host but is not in desired state",
			})
		}
	}

	// 3. Check Networks
	observedNets := make(map[string]bool, len(inv.Networks))
	for _, n := range inv.Networks {
		observedNets[n.Name] = true
	}
	for _, desiredNet := range desired.Networks {
		if !observedNets[desiredNet] {
			discrepancies = append(discrepancies, Discrepancy{
				Type:          DiscrepancyMissingNetwork,
				ResourceName:  desiredNet,
				ObservedState: "missing",
				DesiredState:  "present",
				ActionTaken:   "reported",
				Details:       "expected network does not exist on host",
			})
		}
	}

	// 4. Check Volumes
	observedVols := make(map[string]bool, len(inv.Volumes))
	for _, v := range inv.Volumes {
		observedVols[v.Name] = true
	}
	for _, desiredVol := range desired.Volumes {
		if !observedVols[desiredVol] {
			discrepancies = append(discrepancies, Discrepancy{
				Type:          DiscrepancyMissingVolume,
				ResourceName:  desiredVol,
				ObservedState: "missing",
				DesiredState:  "present",
				ActionTaken:   "reported",
				Details:       "expected volume does not exist on host",
			})
		}
	}

	report := &ReconciliationReport{
		Timestamp:     time.Now().UTC(),
		Healthy:       len(discrepancies) == 0,
		Discrepancies: discrepancies,
		InventorySummary: map[string]int{
			"managedContainers": len(inv.Containers),
			"managedNetworks":   len(inv.Networks),
			"managedVolumes":    len(inv.Volumes),
		},
	}

	return report, nil
}

func isManaged(labels map[string]string) bool {
	return labels != nil && labels[protocol.LabelManaged] == "true"
}
