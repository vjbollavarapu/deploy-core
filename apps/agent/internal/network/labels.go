package network

import (
	"errors"
	"fmt"
	"strings"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

var (
	// ErrMissingLabel indicates a required platform network label is missing.
	ErrMissingLabel = errors.New("missing required platform network label")
	// ErrNotManagedNetwork indicates the network does not have deploycore.managed=true.
	ErrNotManagedNetwork = errors.New("network is not managed by deploycore")
)

// OwnershipStatus classifies network ownership.
type OwnershipStatus string

const (
	OwnershipValid              OwnershipStatus = "VALID"
	OwnershipTenantMismatch     OwnershipStatus = "TENANT_MISMATCH"
	OwnershipUntrustedCollision OwnershipStatus = "UNTRUSTED_COLLISION"
	OwnershipUnmanaged          OwnershipStatus = "UNMANAGED"
)

// Metadata contains platform ownership and topology metadata for a network.
type Metadata struct {
	OrganizationID  string
	ProjectID       string
	ProjectSlug     string
	EnvironmentID   string
	EnvironmentSlug string
	NetworkType     string // "private", "proxy", "isolated"
}

// Validate ensures required fields are populated for project-scoped networks.
func (m Metadata) Validate() error {
	if m.NetworkType == protocol.NetworkTypeProxy {
		// Proxy network is platform-wide or org-wide
		return nil
	}

	var missing []string
	if strings.TrimSpace(m.OrganizationID) == "" {
		missing = append(missing, protocol.LabelOrganizationID)
	}
	if strings.TrimSpace(m.ProjectID) == "" && strings.TrimSpace(m.ProjectSlug) == "" {
		missing = append(missing, "project_id/slug")
	}
	if strings.TrimSpace(m.EnvironmentID) == "" && strings.TrimSpace(m.EnvironmentSlug) == "" {
		missing = append(missing, "environment_id/slug")
	}

	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrMissingLabel, strings.Join(missing, ", "))
	}
	return nil
}

// Labels returns the trusted platform labels for this network.
func (m Metadata) Labels() map[string]string {
	labels := map[string]string{
		protocol.LabelManaged: "true",
	}

	if m.NetworkType != "" {
		labels[protocol.LabelNetworkType] = m.NetworkType
	} else {
		labels[protocol.LabelNetworkType] = protocol.NetworkTypePrivate
	}

	if strings.TrimSpace(m.OrganizationID) != "" {
		labels[protocol.LabelOrganizationID] = strings.TrimSpace(m.OrganizationID)
	}
	if strings.TrimSpace(m.ProjectID) != "" {
		labels[protocol.LabelProjectID] = strings.TrimSpace(m.ProjectID)
	}
	if strings.TrimSpace(m.ProjectSlug) != "" {
		labels[protocol.LabelProjectSlug] = strings.TrimSpace(m.ProjectSlug)
	}
	if strings.TrimSpace(m.EnvironmentID) != "" {
		labels[protocol.LabelEnvironmentID] = strings.TrimSpace(m.EnvironmentID)
	}
	if strings.TrimSpace(m.EnvironmentSlug) != "" {
		labels[protocol.LabelEnvironmentSlug] = strings.TrimSpace(m.EnvironmentSlug)
	}

	return labels
}

// ExtractMetadata parses platform metadata from network labels.
func ExtractMetadata(labels map[string]string) (Metadata, error) {
	if labels == nil || labels[protocol.LabelManaged] != "true" {
		return Metadata{}, ErrNotManagedNetwork
	}

	netType := labels[protocol.LabelNetworkType]
	if netType == "" {
		netType = protocol.NetworkTypePrivate
	}

	m := Metadata{
		OrganizationID:  labels[protocol.LabelOrganizationID],
		ProjectID:       labels[protocol.LabelProjectID],
		ProjectSlug:     labels[protocol.LabelProjectSlug],
		EnvironmentID:   labels[protocol.LabelEnvironmentID],
		EnvironmentSlug: labels[protocol.LabelEnvironmentSlug],
		NetworkType:     netType,
	}

	if err := m.Validate(); err != nil {
		return Metadata{}, err
	}

	return m, nil
}

// IsManagedNetwork returns true if the network carries deploycore.managed=true.
func IsManagedNetwork(labels map[string]string) bool {
	return labels != nil && labels[protocol.LabelManaged] == "true"
}

// VerifyOwnership validates network ownership and isolation.
func VerifyOwnership(networkName string, labels map[string]string, expectedOrgID string) OwnershipStatus {
	cleanName := strings.TrimSpace(networkName)
	looksLikePlatform := IsPlatformNetworkName(cleanName)

	if labels == nil || labels[protocol.LabelManaged] != "true" {
		if looksLikePlatform {
			return OwnershipUntrustedCollision
		}
		return OwnershipUnmanaged
	}

	// For proxy network, it's shared across applications on the host
	if cleanName == protocol.ProxyNetworkName {
		return OwnershipValid
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
