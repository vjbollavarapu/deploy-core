package cleanup

import (
	"time"
)

// Policy defines parameters and safety constraints for a cleanup cycle.
type Policy struct {
	WorkspaceMaxAge          time.Duration `json:"workspaceMaxAge"`
	PruneStoppedContainers   bool          `json:"pruneStoppedContainers"`
	PruneUnusedManagedImages bool          `json:"pruneUnusedManagedImages"`
	ActiveRevisionIDs        []string      `json:"activeRevisionIds"`
	ActiveImageIDs           []string      `json:"activeImageIds"`
}

// DefaultPolicy returns conservative production safety settings.
func DefaultPolicy() Policy {
	return Policy{
		WorkspaceMaxAge:          24 * time.Hour,
		PruneStoppedContainers:   true,
		PruneUnusedManagedImages: true,
	}
}

// Report details the resources reclaimed during the platform cleanup cycle.
type Report struct {
	Timestamp         time.Time `json:"timestamp"`
	PrunedWorkspaces  int       `json:"prunedWorkspaces"`
	RemovedContainers []string  `json:"removedContainers"`
	PrunedImages      []string  `json:"prunedImages"`
	CleanedFiles      int       `json:"cleanedFiles"`
	ReclaimedBytes    int64     `json:"reclaimedBytes"`
	Errors            []string  `json:"errors,omitempty"`
}
