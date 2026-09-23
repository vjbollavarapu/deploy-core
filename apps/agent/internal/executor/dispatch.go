package executor

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
	"github.com/deploycore/deploy-core/apps/agent/internal/workspace"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// Handler is the interface each operation handler must implement.
// It receives a typed context (cancelled if the command expires or is cancelled)
// and the raw payload map. Handlers are responsible for strict payload parsing —
// no raw map[string]any values may be passed directly to Docker calls.
type Handler interface {
	Execute(ctx context.Context, payload map[string]any) (ExecutionResult, error)
}

// HandlerFunc is a convenience adapter for function-based handlers.
type HandlerFunc func(ctx context.Context, payload map[string]any) (ExecutionResult, error)

func (f HandlerFunc) Execute(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
	return f(ctx, payload)
}

// registry maps operation strings to their Handler implementations.
// Only operations present in this map will be dispatched.
type registry map[string]Handler

// buildRegistry constructs the operation registry from all supported protocol operations.
// Any operation not listed here results in an immediate REJECTED response.
func buildRegistry(cli *docker.Client, tr transport.Client, wsMgr *workspace.Manager, backupsDir string, canceler CommandCanceler, log *slog.Logger) registry {
	r := make(registry)

	// Container operations
	r[protocol.OpStartContainer] = startContainerHandler(cli)
	r[protocol.OpStopContainer] = stopContainerHandler(cli, log)
	r[protocol.OpRestartContainer] = restartContainerHandler(cli)
	r[protocol.OpRemoveContainer] = removeContainerHandler(cli)

	// Image operations
	r[protocol.OpPullImage] = pullImageHandler(cli, tr, log)
	r[protocol.OpBuildImage] = buildImageHandler(cli, tr, wsMgr, log)

	// Network operations
	r[protocol.OpCreateNetwork] = createNetworkHandler(cli)
	r[protocol.OpRemoveNetwork] = removeNetworkHandler(cli)
	r[protocol.OpInspectNetwork] = inspectNetworkHandler(cli)

	// Volume operations
	r[protocol.OpCreateVolume] = createVolumeHandler(cli)
	r[protocol.OpRemoveVolume] = removeVolumeHandler(cli)
	r[protocol.OpAttachVolume] = attachVolumeHandler(cli)
	r[protocol.OpDetachVolume] = detachVolumeHandler(cli)
	r[protocol.OpInspectVolume] = inspectVolumeHandler(cli)

	// Log operations
	r[protocol.OpFetchLogs] = fetchLogsHandler(cli, log)
	r[protocol.OpStreamLogs] = streamLogsHandler(cli, tr, log)

	// Health check
	r[protocol.OpRunHealthCheck] = runHealthCheckHandler(cli)

	// Deployment — Phase A15 Candidate Revision Start
	r[protocol.OpDeployRevision] = deployRevisionHandler(cli, log)

	// Activation — Phase A17 Zero-Downtime Activation
	r[protocol.OpActivateRevision] = activateRevisionHandler(cli, log)

	// Rollback — Phase A19 Rollback Execution
	r[protocol.OpRollbackRevision] = rollbackRevisionHandler(cli, log)

	// Telemetry & Stats — Phase A22
	r[protocol.OpCollectStats] = collectStatsHandler(cli, log)

	// Reconciliation — Phase A24
	r[protocol.OpReconcile] = reconcileHandler(cli, log)

	// Safe Disk Cleanup — Phase A26
	r[protocol.OpCleanupDisk] = cleanupDiskHandler(cli, wsMgr, log)

	// Database Execution — Phase A27
	r[protocol.OpProvisionDatabase] = provisionDatabaseHandler(cli, tr, log)
	r[protocol.OpStartDatabase] = startDatabaseHandler(cli, log)
	r[protocol.OpStopDatabase] = stopDatabaseHandler(cli, log)

	// Database Backup & Restore — Phase A28 & A29
	r[protocol.OpCreateBackup] = createBackupHandler(cli, tr, backupsDir, log)
	r[protocol.OpRestoreBackup] = restoreBackupHandler(cli, tr, backupsDir, log)

	// Cancellation — Phase A36
	r[protocol.OpCancelCommand] = cancelCommandHandler(canceler, log)

	return r
}

// stubHandler returns a handler that immediately returns ACCEPTED with a "not_implemented" note.
// Used for operations whose full implementation is deferred to future phases.
func stubHandler(op string) Handler {
	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		return ExecutionResult{Output: map[string]any{
			"stub":      true,
			"operation": op,
			"note":      "implementation deferred to future phase",
		}}, nil
	})
}

// decodePayload is a helper that strictly deserialises a payload map into a typed struct.
// It returns an ExecutionError with ErrCodeInvalidPayload if decoding fails.
func decodePayload(payload map[string]any, dst any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return Errorf(ErrCodeInvalidPayload, "could not marshal payload: %v", err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return Errorf(ErrCodeInvalidPayload, "could not decode payload: %v", err)
	}
	return nil
}
