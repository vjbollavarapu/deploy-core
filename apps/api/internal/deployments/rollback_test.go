package deployments

import (
	"testing"

	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
)

func TestEvaluateRollbackTarget(t *testing.T) {
	t.Parallel()

	if err := EvaluateRollbackTarget("READY"); err != nil {
		t.Fatalf("READY: %v", err)
	}
	if err := EvaluateRollbackTarget("INACTIVE"); err != nil {
		t.Fatalf("INACTIVE: %v", err)
	}

	err := EvaluateRollbackTarget("ACTIVE")
	apiErr, ok := apierror.AsAPIError(err)
	if !ok || apiErr.Status != 409 || apiErr.Code != apierror.CodeConflict {
		t.Fatalf("ACTIVE got %#v", err)
	}

	for _, st := range []string{"CREATED", "FAILED", "ARCHIVED", ""} {
		err := EvaluateRollbackTarget(st)
		apiErr, ok := apierror.AsAPIError(err)
		if !ok || apiErr.Code != apierror.CodeRevisionNotReady {
			t.Fatalf("%s: got %#v", st, err)
		}
	}
}
