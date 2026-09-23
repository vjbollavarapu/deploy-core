package appcontainer

import (
	"testing"
	"time"
)

func TestPlanReconciliation_Scenarios(t *testing.T) {
	orgID := "org-1"
	appID := "app-1"
	envID := "env-1"

	desired := DesiredWorkload{
		Metadata: Metadata{
			OrganizationID: orgID,
			ApplicationID:  appID,
			EnvironmentID:  envID,
			DeploymentID:   "dep-50",
			RevisionID:     "rev-50",
			Instance:       1,
			AppShortID:     "dayaapi",
		},
		DesiredReplicas: 2,
		Image:           "myreg.com/dayaapi:v50",
	}

	// 1. Existing container for previous revision (rev-49 instance 1)
	outdatedContainer := DiscoveredContainer{
		ID:   "cid-old-1",
		Name: "dc-dayaapi-r49-1",
		Metadata: Metadata{
			OrganizationID: orgID,
			ApplicationID:  appID,
			EnvironmentID:  envID,
			DeploymentID:   "dep-49",
			RevisionID:     "rev-49",
			Instance:       1,
			AppShortID:     "dayaapi",
		},
		State:   "running",
		Created: time.Now().Add(-2 * time.Hour),
	}

	// 2. Candidate container for current revision (rev-50 instance 1)
	candidateContainer := DiscoveredContainer{
		ID:   "cid-cand-1",
		Name: "dc-dayaapi-r50-1",
		Metadata: Metadata{
			OrganizationID: orgID,
			ApplicationID:  appID,
			EnvironmentID:  envID,
			DeploymentID:   "dep-50",
			RevisionID:     "rev-50",
			Instance:       1,
			AppShortID:     "dayaapi",
			IsCandidate:    true,
		},
		State:   "running",
		Created: time.Now().Add(-5 * time.Minute),
	}

	// 3. Collision on host squatting on expected instance 2 name
	collision := DiscoveredCollision{
		ID:              "cid-squatter",
		Name:            "dc-dayaapi-r50-2",
		OwnershipStatus: OwnershipUntrustedCollision,
	}

	discovered := DiscoveryResult{
		Containers: []DiscoveredContainer{outdatedContainer, candidateContainer},
		Collisions: []DiscoveredCollision{collision},
	}

	plan, err := PlanReconciliation(desired, discovered)
	if err != nil {
		t.Fatalf("unexpected plan error: %v", err)
	}

	// Candidates
	if len(plan.Candidates) != 1 || plan.Candidates[0].ID != "cid-cand-1" {
		t.Errorf("expected 1 candidate (cid-cand-1), got: %+v", plan.Candidates)
	}

	// Outdated
	if len(plan.Outdated) != 1 || plan.Outdated[0].ID != "cid-old-1" {
		t.Errorf("expected 1 outdated container (cid-old-1), got: %+v", plan.Outdated)
	}

	// Active is empty because candidate is not promoted yet
	if len(plan.Active) != 0 {
		t.Errorf("expected 0 active promoted containers, got %d", len(plan.Active))
	}

	// Missing instances should be [1, 2]
	if len(plan.MissingInstances) != 2 || plan.MissingInstances[0] != 1 || plan.MissingInstances[1] != 2 {
		t.Errorf("expected missing instances [1, 2], got %+v", plan.MissingInstances)
	}

	// Instance 2 should be blocked due to untrusted collision
	if msg, ok := plan.BlockedInstances[2]; !ok || msg == "" {
		t.Errorf("expected instance 2 to be marked blocked by collision, got map: %+v", plan.BlockedInstances)
	}
}
