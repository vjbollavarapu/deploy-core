package appcontainer

import (
	"testing"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestVerifyOwnership_NeverTrustNameAlone(t *testing.T) {
	validMeta := Metadata{
		OrganizationID: "org-tenant-a",
		ApplicationID:  "app-1",
		EnvironmentID:  "env-1",
		DeploymentID:   "dep-1",
		RevisionID:     "rev-49",
		Instance:       1,
		AppShortID:     "dayaapi",
	}

	tests := []struct {
		name           string
		containerName  string
		labels         map[string]string
		expectedOrgID  string
		expectedStatus OwnershipStatus
	}{
		{
			name:           "valid owned container",
			containerName:  "dc-dayaapi-r49-1",
			labels:         validMeta.Labels(),
			expectedOrgID:  "org-tenant-a",
			expectedStatus: OwnershipValid,
		},
		{
			name:           "valid container with docker inspect leading slash",
			containerName:  "/dc-dayaapi-r49-1",
			labels:         validMeta.Labels(),
			expectedOrgID:  "org-tenant-a",
			expectedStatus: OwnershipValid,
		},
		{
			name:           "tenant mismatch — cross-tenant isolation",
			containerName:  "dc-dayaapi-r49-1",
			labels:         validMeta.Labels(),
			expectedOrgID:  "org-tenant-b", // different tenant expected!
			expectedStatus: OwnershipTenantMismatch,
		},
		{
			name:           "untrusted name collision — dc name without labels",
			containerName:  "dc-dayaapi-r49-1",
			labels:         map[string]string{}, // no labels!
			expectedOrgID:  "org-tenant-a",
			expectedStatus: OwnershipUntrustedCollision,
		},
		{
			name:           "untrusted name collision — dc name with nil labels",
			containerName:  "dc-dayaapi-r49-1",
			labels:         nil,
			expectedOrgID:  "org-tenant-a",
			expectedStatus: OwnershipUntrustedCollision,
		},
		{
			name:          "untrusted name collision — deploycore.managed is false",
			containerName: "dc-dayaapi-r49-1",
			labels: map[string]string{
				protocol.LabelManaged: "false",
			},
			expectedOrgID:  "org-tenant-a",
			expectedStatus: OwnershipUntrustedCollision,
		},
		{
			name:          "untrusted name collision — managed true but incomplete metadata",
			containerName: "dc-dayaapi-r49-1",
			labels: map[string]string{
				protocol.LabelManaged:        "true",
				protocol.LabelOrganizationID: "org-tenant-a",
				// missing ApplicationID, RevisionID, etc.
			},
			expectedOrgID:  "org-tenant-a",
			expectedStatus: OwnershipUntrustedCollision,
		},
		{
			name:           "unmanaged third-party container",
			containerName:  "redis-cache",
			labels:         map[string]string{},
			expectedOrgID:  "org-tenant-a",
			expectedStatus: OwnershipUnmanaged,
		},
		{
			name:           "unmanaged third-party container with random labels",
			containerName:  "my-custom-app",
			labels:         map[string]string{"foo": "bar"},
			expectedOrgID:  "org-tenant-a",
			expectedStatus: OwnershipUnmanaged,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := VerifyOwnership(tc.containerName, tc.labels, tc.expectedOrgID)
			if status != tc.expectedStatus {
				t.Errorf("expected status %s, got %s", tc.expectedStatus, status)
			}
		})
	}
}
