package reconciliation

import (
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

// DiscrepancyType classifies the difference between desired and observed state.
type DiscrepancyType string

const (
	DiscrepancyMissingContainer    DiscrepancyType = "MISSING_EXPECTED_CONTAINER"
	DiscrepancyUnexpectedContainer DiscrepancyType = "UNEXPECTED_MANAGED_CONTAINER"
	DiscrepancyExitedContainer     DiscrepancyType = "EXITED_CONTAINER"
	DiscrepancyUnhealthyContainer  DiscrepancyType = "UNHEALTHY_CONTAINER"
	DiscrepancyMissingNetwork      DiscrepancyType = "MISSING_NETWORK"
	DiscrepancyMissingVolume       DiscrepancyType = "MISSING_VOLUME"
)

// Discrepancy represents a single divergence between desired and observed state.
type Discrepancy struct {
	Type          DiscrepancyType `json:"type"`
	ResourceID    string          `json:"resourceId,omitempty"`
	ResourceName  string          `json:"resourceName"`
	ApplicationID string          `json:"applicationId,omitempty"`
	RevisionID    string          `json:"revisionId,omitempty"`
	ObservedState string          `json:"observedState"`
	DesiredState  string          `json:"desiredState"`
	ActionTaken   string          `json:"actionTaken"` // "reported", "restarted", "failed_restart"
	Details       string          `json:"details,omitempty"`
}

// DesiredContainer specifies the expected container state supplied by Control Plane.
type DesiredContainer struct {
	ContainerName     string `json:"containerName"`
	ApplicationID     string `json:"applicationId,omitempty"`
	RevisionID        string `json:"revisionId,omitempty"`
	DesiredStatus     string `json:"desiredStatus"` // e.g. "running"
	AutoRestartExited bool   `json:"autoRestartExited,omitempty"`
}

// DesiredState represents the cluster/server target state supplied by Control Plane.
type DesiredState struct {
	Containers             []DesiredContainer `json:"containers"`
	Networks               []string           `json:"networks,omitempty"`
	Volumes                []string           `json:"volumes,omitempty"`
	AllowSafeRestartExited bool               `json:"allowSafeRestartExited,omitempty"`
}

// LocalInventory is a point-in-time snapshot of managed resources on the host.
type LocalInventory struct {
	Containers []docker.ContainerSummary `json:"containers"`
	Networks   []docker.NetworkSummary   `json:"networks"`
	Volumes    []docker.VolumeSummary    `json:"volumes"`
	Timestamp  time.Time                 `json:"timestamp"`
}

// ReconciliationReport summarizes the results of the reconciliation comparison.
type ReconciliationReport struct {
	Timestamp        time.Time      `json:"timestamp"`
	Healthy          bool           `json:"healthy"`
	Discrepancies    []Discrepancy  `json:"discrepancies"`
	InventorySummary map[string]int `json:"inventorySummary"`
}
