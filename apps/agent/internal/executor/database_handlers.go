package executor

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/database"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
)

type dbActionPayload struct {
	DatabaseID string `json:"databaseId"`
	TimeoutSec int    `json:"timeoutSeconds,omitempty"`
}

func provisionDatabaseHandler(cli *docker.Client, tr transport.Client, log *slog.Logger) Handler {
	if log == nil {
		log = slog.Default()
	}
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		var req database.ProvisionRequest
		if err := decodePayload(payload, &req); err != nil {
			return ExecutionResult{}, err
		}
		if strings.TrimSpace(req.DatabaseID) == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "databaseId is required")
		}

		// Password is never embedded by the Control Plane — fetch via bootstrap.
		if strings.TrimSpace(req.Password) == "" {
			if tr == nil {
				return ExecutionResult{}, Errorf(ErrCodeInternalError, "transport required to bootstrap database credentials")
			}
			boot, err := tr.FetchDatabaseBootstrap(ctx, req.DatabaseID)
			if err != nil {
				return ExecutionResult{}, Errorf(ErrCodeInternalError, "database bootstrap failed: %v", err)
			}
			req.Password = boot.Password
			if req.DatabaseName == "" {
				req.DatabaseName = boot.DatabaseName
			}
			if req.Username == "" {
				req.Username = boot.Username
			}
			if req.StorageVolumeName == "" {
				req.StorageVolumeName = boot.StorageVolumeName
			}
			if req.Engine == "" {
				req.Engine = boot.Engine
			}
			if req.EngineVersion == "" {
				req.EngineVersion = boot.EngineVersion
			}
		}
		// Scrub password from payload map so it cannot be logged via residual maps.
		delete(payload, "password")

		mgr := database.NewManager(cli, log)
		state, err := mgr.Provision(ctx, req)
		if err != nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "database provision failed: %v", err)
		}

		// Flatten identifiers for Control Plane completion hooks.
		return ExecutionResult{
			Output: map[string]any{
				"databaseId":         state.DatabaseID,
				"containerId":        state.ContainerID,
				"containerRuntimeId": state.ContainerID,
				"containerName":      state.ContainerName,
				"status":             state.Status,
				"database":           state,
			},
		}, nil
	})
}

func startDatabaseHandler(cli *docker.Client, log *slog.Logger) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		var p dbActionPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}
		id := strings.TrimSpace(p.DatabaseID)
		if id == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "databaseId is required")
		}

		mgr := database.NewManager(cli, log)
		if err := mgr.Start(ctx, id); err != nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "start database failed: %v", err)
		}

		return ExecutionResult{
			Output: map[string]any{
				"status":     "started",
				"databaseId": id,
			},
		}, nil
	})
}

func stopDatabaseHandler(cli *docker.Client, log *slog.Logger) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		var p dbActionPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}
		id := strings.TrimSpace(p.DatabaseID)
		if id == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "databaseId is required")
		}

		timeout := 15 * time.Second
		if p.TimeoutSec > 0 {
			timeout = time.Duration(p.TimeoutSec) * time.Second
		}

		mgr := database.NewManager(cli, log)
		if err := mgr.Stop(ctx, id, timeout); err != nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "stop database failed: %v", err)
		}

		return ExecutionResult{
			Output: map[string]any{
				"status":     "stopped",
				"databaseId": id,
			},
		}, nil
	})
}
