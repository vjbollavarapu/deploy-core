package deployments

import "github.com/deploycore/deploy-core/apps/api/pkg/apierror"

// EvaluateRollbackTarget reports whether a revision status may be used as a
// rollback target (reuse without rebuild). Pure rule used by Rollback.
func EvaluateRollbackTarget(status string) error {
	switch status {
	case "READY", "INACTIVE":
		return nil
	case "ACTIVE":
		return apierror.Conflict("target revision is already active")
	default:
		return apierror.New(409, apierror.CodeRevisionNotReady, "target revision is not eligible for rollback")
	}
}
