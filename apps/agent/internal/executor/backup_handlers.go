package executor

import (
	"context"
	"log/slog"
	"strings"

	"github.com/deploycore/deploy-core/apps/agent/internal/backup"
	"github.com/deploycore/deploy-core/apps/agent/internal/backupstorage"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/restore"
	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
)

func createBackupHandler(cli *docker.Client, tr transport.Client, backupsDir string, log *slog.Logger) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		var req backup.Request
		if err := decodePayload(payload, &req); err != nil {
			return ExecutionResult{}, err
		}
		if strings.TrimSpace(req.DatabaseID) == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "databaseId is required")
		}

		if strings.TrimSpace(req.Password) == "" || strings.TrimSpace(req.DatabaseName) == "" || strings.TrimSpace(req.Username) == "" {
			if tr == nil {
				return ExecutionResult{}, Errorf(ErrCodeInternalError, "transport required to bootstrap database credentials")
			}
			boot, err := tr.FetchDatabaseBootstrap(ctx, req.DatabaseID)
			if err != nil {
				return ExecutionResult{}, Errorf(ErrCodeInternalError, "database bootstrap failed: %v", err)
			}
			if req.Password == "" {
				req.Password = boot.Password
			}
			if req.DatabaseName == "" {
				req.DatabaseName = boot.DatabaseName
			}
			if req.Username == "" {
				req.Username = boot.Username
			}
		}
		delete(payload, "password")

		storage, err := backupstorage.NewLocalStorage(backupsDir)
		if err != nil {
			return ExecutionResult{}, Errorf(ErrCodeInternalError, "failed to initialize backup storage: %v", err)
		}

		exec := backup.NewExecutor(cli, storage, log)
		res, err := exec.Execute(ctx, req)
		if err != nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "backup failed: %v", err)
		}

		return ExecutionResult{
			Output: map[string]any{
				"backupId":       res.BackupID,
				"checksum":       res.Checksum,
				"sizeBytes":      res.SizeBytes,
				"destinationUri": res.DestinationURI,
				"durationMs":     res.DurationMs,
			},
		}, nil
	})
}

func restoreBackupHandler(cli *docker.Client, tr transport.Client, backupsDir string, log *slog.Logger) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		if cli == nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "docker client unavailable")
		}

		var req restore.Request
		if err := decodePayload(payload, &req); err != nil {
			return ExecutionResult{}, err
		}
		if strings.TrimSpace(req.TargetDatabaseID) == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "targetDatabaseId is required")
		}
		if strings.TrimSpace(req.BackupID) == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "backupId is required")
		}

		if strings.TrimSpace(req.Password) == "" || strings.TrimSpace(req.DatabaseName) == "" || strings.TrimSpace(req.Username) == "" {
			if tr == nil {
				return ExecutionResult{}, Errorf(ErrCodeInternalError, "transport required to bootstrap database credentials")
			}
			boot, err := tr.FetchDatabaseBootstrap(ctx, req.TargetDatabaseID)
			if err != nil {
				return ExecutionResult{}, Errorf(ErrCodeInternalError, "database bootstrap failed: %v", err)
			}
			if req.Password == "" {
				req.Password = boot.Password
			}
			if req.DatabaseName == "" {
				req.DatabaseName = boot.DatabaseName
			}
			if req.Username == "" {
				req.Username = boot.Username
			}
		}
		delete(payload, "password")

		storage, err := backupstorage.NewLocalStorage(backupsDir)
		if err != nil {
			return ExecutionResult{}, Errorf(ErrCodeInternalError, "failed to initialize backup storage: %v", err)
		}

		exec := restore.NewExecutor(cli, storage, log)
		res, err := exec.Execute(ctx, req)
		if err != nil {
			return ExecutionResult{}, Errorf(ErrCodeDockerError, "restore failed: %v", err)
		}

		return ExecutionResult{
			Output: map[string]any{
				"restoreId":        res.RestoreID,
				"backupId":         res.BackupID,
				"targetDatabaseId": res.TargetDatabaseID,
				"restored":         res.Restored,
				"validationPassed": res.ValidationPassed,
				"durationMs":       res.DurationMs,
			},
		}, nil
	})
}
