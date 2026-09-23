package appcontainer

import (
	"strings"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// OwnershipStatus classifies whether a container on the host is owned by the platform and tenant.
type OwnershipStatus string

const (
	// OwnershipValid indicates the container has valid DeployCore platform labels and belongs
	// to the expected tenant.
	OwnershipValid OwnershipStatus = "VALID"

	// OwnershipTenantMismatch indicates the container has DeployCore labels, but belongs
	// to a different organization ID.
	OwnershipTenantMismatch OwnershipStatus = "TENANT_MISMATCH"

	// OwnershipUntrustedCollision indicates a container has a name conforming to the platform standard (dc-*),
	// but lacks valid trusted DeployCore labels. It CANNOT be claimed as owned!
	OwnershipUntrustedCollision OwnershipStatus = "UNTRUSTED_COLLISION"

	// OwnershipUnmanaged indicates a container does not use the platform naming standard and is not
	// managed by DeployCore.
	OwnershipUnmanaged OwnershipStatus = "UNMANAGED"
)

// VerifyOwnership inspects a container's name and labels to prove platform ownership.
// Rule: NEVER trust container names alone as ownership proof.
func VerifyOwnership(containerName string, labels map[string]string, expectedOrgID string) OwnershipStatus {
	cleanName := strings.TrimPrefix(strings.TrimSpace(containerName), "/")
	looksLikePlatform := IsPlatformName(cleanName) || strings.HasPrefix(cleanName, protocol.ContainerPrefix+"-")

	if labels == nil || labels[protocol.LabelManaged] != "true" {
		if looksLikePlatform {
			// Container name mimics platform convention, but lacks verified labels!
			return OwnershipUntrustedCollision
		}
		return OwnershipUnmanaged
	}

	// It has deploycore.managed=true. Now extract and validate all required metadata.
	meta, err := ExtractMetadata(labels)
	if err != nil {
		if looksLikePlatform {
			return OwnershipUntrustedCollision
		}
		return OwnershipUnmanaged
	}

	// Verify tenant isolation
	if expectedOrgID != "" && meta.OrganizationID != expectedOrgID {
		return OwnershipTenantMismatch
	}

	return OwnershipValid
}

// IsManagedContainer quickly checks whether a container claims deploycore management.
func IsManagedContainer(labels map[string]string) bool {
	return labels != nil && labels[protocol.LabelManaged] == "true"
}
