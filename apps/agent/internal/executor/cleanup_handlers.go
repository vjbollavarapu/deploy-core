package executor

import (
	"context"
	"log/slog"

	"github.com/deploycore/deploy-core/apps/agent/internal/cleanup"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/workspace"
)

func cleanupDiskHandler(cli *docker.Client, wsMgr *workspace.Manager, log *slog.Logger) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		policy := cleanup.DefaultPolicy()
		_ = decodePayload(payload, &policy)

		var dCli cleanup.DockerClient
		if cli != nil {
			dCli = cli
		}
		var pruner cleanup.WorkspacePruner
		if wsMgr != nil {
			pruner = wsMgr
		}
		cleaner := cleanup.NewCleaner(dCli, pruner, log)
		report, err := cleaner.Cleanup(ctx, policy)
		if err != nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "cleanup failed: %v", err)
		}

		return ExecutionResult{
			Output: map[string]any{
				"report": report,
			},
		}, nil
	})
}
