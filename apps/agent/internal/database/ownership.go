package database

import (
	"fmt"
	"strings"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

const labelServiceType = "deploycore.service_type"
const labelDatabaseID = "deploycore.database_id"

// VerifyManagedDatabaseLabels ensures a container or volume belongs to the
// DeployCore-managed database identified by databaseID.
func VerifyManagedDatabaseLabels(labels map[string]string, databaseID string) error {
	id := strings.TrimSpace(databaseID)
	if id == "" {
		return fmt.Errorf("databaseId cannot be empty")
	}
	if labels == nil {
		return fmt.Errorf("resource has no DeployCore ownership labels")
	}
	if labels[protocol.LabelManaged] != "true" {
		return fmt.Errorf("resource is not deploycore-managed")
	}
	if labels[labelServiceType] != "database" {
		return fmt.Errorf("resource is not a managed database")
	}
	got := strings.TrimSpace(labels[labelDatabaseID])
	if got == "" {
		return fmt.Errorf("resource missing deploycore.database_id label")
	}
	// Accept both raw UUID and dc-db-{id} / db-{id} forms.
	want := strings.TrimPrefix(id, "db-")
	gotNorm := strings.TrimPrefix(got, "db-")
	if !strings.EqualFold(gotNorm, want) && !strings.EqualFold(got, id) {
		return fmt.Errorf("resource database_id %q does not match requested %q", got, id)
	}
	return nil
}

// VerifyManagedDatabaseContainer inspects labels on a running/inspected container.
func VerifyManagedDatabaseContainer(detail docker.ContainerDetail, databaseID string) error {
	return VerifyManagedDatabaseLabels(detail.Labels, databaseID)
}
