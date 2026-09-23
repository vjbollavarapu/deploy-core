package executor

import (
	"context"
	"log/slog"
	"strings"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/stats"
)

type collectStatsPayload struct {
	ContainerID   string `json:"containerId,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
}

func collectStatsHandler(cli *docker.Client, log *slog.Logger) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		collector := stats.NewCollector(cli)

		var p collectStatsPayload
		_ = decodePayload(payload, &p)

		targetID := strings.TrimSpace(p.ContainerID)
		if targetID == "" {
			targetID = strings.TrimSpace(p.ContainerName)
		}

		if targetID != "" {
			stat, err := collector.GetContainerStats(ctx, targetID)
			if err != nil {
				return ExecutionResult{}, Errorf(ErrCodeDockerError, "failed to get stats for container %s: %v", targetID, err)
			}
			return ExecutionResult{
				Output: map[string]any{
					"container": stat,
				},
			}, nil
		}

		allStats, err := collector.CollectAllManaged(ctx)
		if err != nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "failed to collect managed container stats: %v", err)
		}

		return ExecutionResult{
			Output: map[string]any{
				"containers": allStats,
				"count":      len(allStats),
			},
		}, nil
	})
}
