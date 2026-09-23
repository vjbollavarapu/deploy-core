package executor

import (
	"context"
	"log/slog"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/reconciliation"
)

func reconcileHandler(cli *docker.Client, log *slog.Logger) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeInternalError, "docker client not initialized")
		}

		recon := reconciliation.NewReconciler(cli, log)

		var desired reconciliation.DesiredState
		if err := decodePayload(payload, &desired); err != nil {
			return ExecutionResult{}, err
		}

		report, err := recon.Reconcile(ctx, desired)
		if err != nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "reconciliation failed: %v", err)
		}

		return ExecutionResult{
			Output: map[string]any{
				"report": report,
			},
		}, nil
	})
}
