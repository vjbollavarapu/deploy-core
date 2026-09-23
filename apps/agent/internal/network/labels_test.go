package network

import (
	"testing"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestNetworkMetadata_Labels_RoundTrip(t *testing.T) {
	meta := Metadata{
		OrganizationID:  "org-1",
		ProjectID:       "proj-123",
		ProjectSlug:     "daya",
		EnvironmentID:   "env-456",
		EnvironmentSlug: "prod",
		NetworkType:     protocol.NetworkTypePrivate,
	}

	labels := meta.Labels()

	if labels[protocol.LabelManaged] != "true" {
		t.Errorf("expected deploycore.managed='true', got %q", labels[protocol.LabelManaged])
	}
	if labels[protocol.LabelNetworkType] != protocol.NetworkTypePrivate {
		t.Errorf("expected network type private, got %q", labels[protocol.LabelNetworkType])
	}

	extracted, err := ExtractMetadata(labels)
	if err != nil {
		t.Fatalf("unexpected extract error: %v", err)
	}

	if extracted != meta {
		t.Errorf("expected %+v, got %+v", meta, extracted)
	}
}

func TestVerifyNetworkOwnership(t *testing.T) {
	validMeta := Metadata{
		OrganizationID:  "org-alpha",
		ProjectSlug:     "daya",
		EnvironmentSlug: "prod",
		NetworkType:     protocol.NetworkTypePrivate,
	}

	tests := []struct {
		name           string
		networkName    string
		labels         map[string]string
		expectedOrgID  string
		expectedStatus OwnershipStatus
	}{
		{
			name:           "valid platform private network",
			networkName:    "dc-daya-prod-private",
			labels:         validMeta.Labels(),
			expectedOrgID:  "org-alpha",
			expectedStatus: OwnershipValid,
		},
		{
			name:           "valid deploycore-proxy network",
			networkName:    "deploycore-proxy",
			labels:         map[string]string{protocol.LabelManaged: "true", protocol.LabelNetworkType: protocol.NetworkTypeProxy},
			expectedOrgID:  "org-alpha",
			expectedStatus: OwnershipValid,
		},
		{
			name:           "tenant mismatch cross-tenant network",
			networkName:    "dc-daya-prod-private",
			labels:         validMeta.Labels(),
			expectedOrgID:  "org-beta",
			expectedStatus: OwnershipTenantMismatch,
		},
		{
			name:           "untrusted name collision dc-* without labels",
			networkName:    "dc-daya-prod-private",
			labels:         map[string]string{},
			expectedOrgID:  "org-alpha",
			expectedStatus: OwnershipUntrustedCollision,
		},
		{
			name:           "unmanaged third-party network",
			networkName:    "my-custom-bridge",
			labels:         map[string]string{},
			expectedOrgID:  "org-alpha",
			expectedStatus: OwnershipUnmanaged,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := VerifyOwnership(tc.networkName, tc.labels, tc.expectedOrgID)
			if status != tc.expectedStatus {
				t.Errorf("expected %s, got %s", tc.expectedStatus, status)
			}
		})
	}
}
