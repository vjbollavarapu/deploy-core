package volume

import (
	"errors"
	"fmt"
	"strings"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

var (
	// ErrMissingLabel indicates a required platform volume label is missing.
	ErrMissingLabel = errors.New("missing required platform volume label")
	// ErrNotManagedVolume indicates the volume does not have deploycore.managed=true.
	ErrNotManagedVolume = errors.New("volume is not managed by deploycore")
)

// OwnershipStatus classifies volume ownership and tenant isolation.
type OwnershipStatus string

const (
	OwnershipValid              OwnershipStatus = "VALID"
	OwnershipTenantMismatch     OwnershipStatus = "TENANT_MISMATCH"
	OwnershipUntrustedCollision OwnershipStatus = "UNTRUSTED_COLLISION"
	OwnershipUnmanaged          OwnershipStatus = "UNMANAGED"
)

// Metadata contains platform ownership and scoping metadata for a managed volume.
type Metadata struct {
	OrganizationID string
	VolumeID       string
	VolumeName     string
	ProjectID      string
	EnvironmentID  string
	ApplicationID  string
	Owner          string // default: "platform"
	Critical       string // optional: "database", "persistent", etc.
}

// Validate ensures required platform identification fields are populated.
func (m Metadata) Validate() error {
	var missing []string
	if strings.TrimSpace(m.OrganizationID) == "" {
		missing = append(missing, protocol.LabelOrganizationID)
	}

	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrMissingLabel, strings.Join(missing, ", "))
	}
	return nil
}

// Labels returns the canonical trusted platform labels proving DeployCore ownership.
func (m Metadata) Labels() map[string]string {
	owner := m.Owner
	if owner == "" {
		owner = protocol.OwnerPlatform
	}

	labels := map[string]string{
		protocol.LabelManaged: protocol.LabelManaged, // standard "true" handled below
		protocol.LabelOwner:   owner,
	}
	labels[protocol.LabelManaged] = "true"

	if strings.TrimSpace(m.OrganizationID) != "" {
		labels[protocol.LabelOrganizationID] = strings.TrimSpace(m.OrganizationID)
	}
	if strings.TrimSpace(m.VolumeID) != "" {
		labels[protocol.LabelVolumeID] = strings.TrimSpace(m.VolumeID)
	}
	if strings.TrimSpace(m.VolumeName) != "" {
		labels[protocol.LabelVolumeName] = strings.TrimSpace(m.VolumeName)
	}
	if strings.TrimSpace(m.ProjectID) != "" {
		labels[protocol.LabelProjectID] = strings.TrimSpace(m.ProjectID)
	}
	if strings.TrimSpace(m.EnvironmentID) != "" {
		labels[protocol.LabelEnvironmentID] = strings.TrimSpace(m.EnvironmentID)
	}
	if strings.TrimSpace(m.ApplicationID) != "" {
		labels[protocol.LabelApplicationID] = strings.TrimSpace(m.ApplicationID)
	}
	if strings.TrimSpace(m.Critical) != "" {
		labels[protocol.LabelCritical] = strings.TrimSpace(m.Critical)
	}

	return labels
}

// ExtractMetadata parses platform metadata from volume labels.
func ExtractMetadata(labels map[string]string) (Metadata, error) {
	if labels == nil || labels[protocol.LabelManaged] != "true" {
		return Metadata{}, ErrNotManagedVolume
	}

	m := Metadata{
		OrganizationID: labels[protocol.LabelOrganizationID],
		VolumeID:       labels[protocol.LabelVolumeID],
		VolumeName:     labels[protocol.LabelVolumeName],
		ProjectID:      labels[protocol.LabelProjectID],
		EnvironmentID:  labels[protocol.LabelEnvironmentID],
		ApplicationID:  labels[protocol.LabelApplicationID],
		Owner:          labels[protocol.LabelOwner],
		Critical:       labels[protocol.LabelCritical],
	}

	if err := m.Validate(); err != nil {
		return Metadata{}, err
	}

	return m, nil
}

// IsManagedVolume returns true if the volume has deploycore.managed=true.
func IsManagedVolume(labels map[string]string) bool {
	return labels != nil && labels[protocol.LabelManaged] == "true"
}

// VerifyOwnership validates volume ownership labels against expected tenant boundaries.
func VerifyOwnership(volumeName string, labels map[string]string, expectedOrgID string) OwnershipStatus {
	cleanName := strings.TrimSpace(volumeName)
	looksLikePlatform := IsPlatformVolumeName(cleanName)

	if labels == nil || labels[protocol.LabelManaged] != "true" {
		if looksLikePlatform {
			return OwnershipUntrustedCollision
		}
		return OwnershipUnmanaged
	}

	meta, err := ExtractMetadata(labels)
	if err != nil {
		if looksLikePlatform {
			return OwnershipUntrustedCollision
		}
		return OwnershipUnmanaged
	}

	if expectedOrgID != "" && meta.OrganizationID != "" && meta.OrganizationID != expectedOrgID {
		return OwnershipTenantMismatch
	}

	return OwnershipValid
}
