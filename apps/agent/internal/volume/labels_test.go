package volume

import (
	"testing"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestVolumeMetadata_Labels_RoundTrip(t *testing.T) {
	meta := Metadata{
		OrganizationID: "org-1",
		VolumeID:       "vol-123",
		VolumeName:     "dc-app-data",
		ProjectID:      "proj-456",
		EnvironmentID:  "env-789",
		ApplicationID:  "app-101",
		Owner:          protocol.OwnerPlatform,
		Critical:       "database",
	}

	labels := meta.Labels()

	if labels[protocol.LabelManaged] != "true" {
		t.Errorf("expected deploycore.managed='true', got %q", labels[protocol.LabelManaged])
	}
	if labels[protocol.LabelOwner] != protocol.OwnerPlatform {
		t.Errorf("expected deploycore.owner='platform', got %q", labels[protocol.LabelOwner])
	}

	extracted, err := ExtractMetadata(labels)
	if err != nil {
		t.Fatalf("unexpected extract error: %v", err)
	}

	if extracted != meta {
		t.Errorf("expected %+v, got %+v", meta, extracted)
	}
}

func TestVerifyVolumeOwnership(t *testing.T) {
	meta := Metadata{
		OrganizationID: "org-alpha",
		VolumeID:       "vol-1",
	}

	tests := []struct {
		name           string
		volumeName     string
		labels         map[string]string
		expectedOrgID  string
		expectedStatus OwnershipStatus
	}{
		{
			name:           "valid platform volume",
			volumeName:     "dc-app-data",
			labels:         meta.Labels(),
			expectedOrgID:  "org-alpha",
			expectedStatus: OwnershipValid,
		},
		{
			name:           "tenant mismatch cross-tenant volume",
			volumeName:     "dc-app-data",
			labels:         meta.Labels(),
			expectedOrgID:  "org-beta",
			expectedStatus: OwnershipTenantMismatch,
		},
		{
			name:           "untrusted collision dc-* without labels",
			volumeName:     "dc-app-data",
			labels:         map[string]string{},
			expectedOrgID:  "org-alpha",
			expectedStatus: OwnershipUntrustedCollision,
		},
		{
			name:           "unmanaged third-party volume",
			volumeName:     "redis-data",
			labels:         map[string]string{},
			expectedOrgID:  "org-alpha",
			expectedStatus: OwnershipUnmanaged,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := VerifyOwnership(tc.volumeName, tc.labels, tc.expectedOrgID)
			if status != tc.expectedStatus {
				t.Errorf("expected %s, got %s", tc.expectedStatus, status)
			}
		})
	}
}
