package appcontainer

import (
	"testing"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestMetadata_Labels_Generation(t *testing.T) {
	meta := Metadata{
		OrganizationID: "org-123",
		ApplicationID:  "app-456",
		EnvironmentID:  "env-789",
		DeploymentID:   "dep-101",
		RevisionID:     "rev-202",
		Instance:       1,
		AppShortID:     "dayaapi",
		IsCandidate:    true,
	}

	labels := meta.Labels()

	expectedLabels := map[string]string{
		protocol.LabelManaged:         "true",
		protocol.LabelOrganizationID:  "org-123",
		protocol.LabelApplicationID:   "app-456",
		protocol.LabelEnvironmentID:   "env-789",
		protocol.LabelDeploymentID:    "dep-101",
		protocol.LabelRevisionID:      "rev-202",
		protocol.LabelInstance:        "1",
		protocol.LabelApplicationSlug: "dayaapi",
		protocol.LabelCandidate:       "true",
	}

	for k, v := range expectedLabels {
		if got, ok := labels[k]; !ok || got != v {
			t.Errorf("label %q: expected %q, got %q (present=%v)", k, v, got, ok)
		}
	}
}

func TestMetadata_RoundTrip(t *testing.T) {
	original := Metadata{
		OrganizationID: "org-xyz",
		ApplicationID:  "app-abc",
		EnvironmentID:  "env-prod",
		DeploymentID:   "dep-99",
		RevisionID:     "rev-49",
		Instance:       2,
		AppShortID:     "dayaapi",
		IsCandidate:    false,
	}

	labels := original.Labels()

	extracted, err := ExtractMetadata(labels)
	if err != nil {
		t.Fatalf("unexpected error extracting metadata: %v", err)
	}

	if extracted != original {
		t.Errorf("expected %+v, got %+v", original, extracted)
	}
}

func TestMetadata_Validation(t *testing.T) {
	tests := []struct {
		name      string
		meta      Metadata
		shouldErr bool
	}{
		{
			name: "valid metadata",
			meta: Metadata{
				OrganizationID: "org-1",
				ApplicationID:  "app-1",
				EnvironmentID:  "env-1",
				DeploymentID:   "dep-1",
				RevisionID:     "rev-1",
				Instance:       1,
			},
			shouldErr: false,
		},
		{
			name: "missing organization id",
			meta: Metadata{
				ApplicationID: "app-1",
				EnvironmentID: "env-1",
				DeploymentID:  "dep-1",
				RevisionID:    "rev-1",
				Instance:      1,
			},
			shouldErr: true,
		},
		{
			name: "missing revision id",
			meta: Metadata{
				OrganizationID: "org-1",
				ApplicationID:  "app-1",
				EnvironmentID:  "env-1",
				DeploymentID:   "dep-1",
				Instance:       1,
			},
			shouldErr: true,
		},
		{
			name: "zero instance",
			meta: Metadata{
				OrganizationID: "org-1",
				ApplicationID:  "app-1",
				EnvironmentID:  "env-1",
				DeploymentID:   "dep-1",
				RevisionID:     "rev-1",
				Instance:       0,
			},
			shouldErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.meta.Validate()
			if tc.shouldErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}
