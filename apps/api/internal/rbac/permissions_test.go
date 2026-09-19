package rbac_test

import (
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
)

func TestPermissionConstantsMatchPhase(t *testing.T) {
	t.Parallel()
	required := []string{
		rbac.ServerRead, rbac.ServerCreate, rbac.ServerUpdate, rbac.ServerDelete,
		rbac.ProjectRead, rbac.ProjectCreate, rbac.ProjectUpdate, rbac.ProjectDelete,
		rbac.ApplicationRead, rbac.ApplicationCreate, rbac.ApplicationUpdate,
		rbac.ApplicationDeploy, rbac.ApplicationRestart, rbac.ApplicationStop, rbac.ApplicationDelete,
		rbac.DeploymentRead, rbac.DeploymentCreate, rbac.DeploymentCancel, rbac.DeploymentRollback,
		rbac.SecretReadMetadata, rbac.SecretCreate, rbac.SecretUpdate, rbac.SecretDelete,
		rbac.DatabaseRead, rbac.DatabaseCreate, rbac.DatabaseUpdate, rbac.DatabaseBackup, rbac.DatabaseRestore,
		rbac.AuditRead,
	}
	for _, p := range required {
		if p == "" {
			t.Fatal("empty permission constant")
		}
	}
}
