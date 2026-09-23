package appcontainer

import (
	"fmt"
	"strings"
)

// DesiredWorkload describes the target state of an application deployment on this server.
type DesiredWorkload struct {
	Metadata        Metadata
	DesiredReplicas int
	Image           string
}

// ReconciliationPlan describes the actions needed to align actual container state with desired state.
type ReconciliationPlan struct {
	// Active contains verified running containers that match the desired revision.
	Active []DiscoveredContainer

	// Candidates contains candidate containers deployed for this revision awaiting health check or traffic promotion.
	Candidates []DiscoveredContainer

	// Outdated contains verified containers running an older revision of this application.
	Outdated []DiscoveredContainer

	// MissingInstances lists instance indices (1-indexed, e.g. [1, 2]) that are not currently running
	// and need to be created.
	MissingInstances []int

	// UntrustedCollisions lists host containers whose names mimic the platform standard
	// but lack valid DeployCore labels.
	UntrustedCollisions []DiscoveredCollision

	// BlockedInstances lists missing instance numbers that CANNOT be started because an untrusted container
	// already occupies the standard name.
	BlockedInstances map[int]string
}

// PlanReconciliation compares discovered host containers against desired workload state.
// Labels are strictly used to determine ownership, revision matching, and instance slots.
func PlanReconciliation(desired DesiredWorkload, discovered DiscoveryResult) (ReconciliationPlan, error) {
	if err := desired.Metadata.Validate(); err != nil {
		return ReconciliationPlan{}, fmt.Errorf("invalid desired workload metadata: %w", err)
	}
	if desired.DesiredReplicas < 0 {
		return ReconciliationPlan{}, fmt.Errorf("desired replicas cannot be negative: %d", desired.DesiredReplicas)
	}

	plan := ReconciliationPlan{
		Active:              make([]DiscoveredContainer, 0),
		Candidates:          make([]DiscoveredContainer, 0),
		Outdated:            make([]DiscoveredContainer, 0),
		MissingInstances:    make([]int, 0),
		UntrustedCollisions: discovered.Collisions,
		BlockedInstances:    make(map[int]string),
	}

	// Map of running active instances for the target revision: instanceIndex -> container
	activeMap := make(map[int]DiscoveredContainer)

	for _, c := range discovered.Containers {
		// Strict isolation: must match organization and environment
		if c.Metadata.OrganizationID != desired.Metadata.OrganizationID ||
			c.Metadata.EnvironmentID != desired.Metadata.EnvironmentID {
			continue
		}

		// Application check
		if c.Metadata.ApplicationID != desired.Metadata.ApplicationID {
			continue
		}

		if c.Metadata.RevisionID == desired.Metadata.RevisionID {
			if c.Metadata.IsCandidate {
				plan.Candidates = append(plan.Candidates, c)
			} else {
				plan.Active = append(plan.Active, c)
				activeMap[c.Metadata.Instance] = c
			}
		} else {
			// Older or different revision for this application
			plan.Outdated = append(plan.Outdated, c)
		}
	}

	// Build map of untrusted collision names for quick lookup
	collisionNames := make(map[string]DiscoveredCollision)
	for _, col := range discovered.Collisions {
		clean := strings.TrimPrefix(strings.TrimSpace(col.Name), "/")
		collisionNames[clean] = col
	}

	// Find missing instances within 1..DesiredReplicas
	for inst := 1; inst <= desired.DesiredReplicas; inst++ {
		if _, exists := activeMap[inst]; !exists {
			plan.MissingInstances = append(plan.MissingInstances, inst)

			// Check if an untrusted container on the host is squatting on the expected name
			expectedName, err := FormatName(desired.Metadata.AppShortID, desired.Metadata.RevisionID, inst)
			if err == nil {
				if col, ok := collisionNames[expectedName]; ok {
					plan.BlockedInstances[inst] = fmt.Sprintf("untrusted container %q (ID %s) collides with expected name", col.Name, col.ID)
				}
			}
		}
	}

	return plan, nil
}
