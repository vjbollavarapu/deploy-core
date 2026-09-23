package executor

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/runtime"
	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
	"github.com/deploycore/deploy-core/apps/agent/internal/workspace"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

const (
	// DefaultConcurrency is the default number of commands that may run in parallel.
	DefaultConcurrency = 4
	// pruneInterval controls how often expired journal entries are cleaned up.
	pruneInterval = 10 * time.Minute
)

// Config holds tunable executor parameters.
type Config struct {
	// ServerID is the expected server ID from agent configuration.
	// Commands with a different server_id are immediately rejected.
	ServerID uuid.UUID
	// MaxConcurrency limits parallel command execution. Default: DefaultConcurrency.
	MaxConcurrency int
	// DataDir is used for the persistent command journal.
	DataDir string
}

// Executor is the command execution engine. It:
//  1. Polls the Control Plane for pending commands via transport.Client.
//  2. Validates each command (expiry, server ID, schema, uniqueness).
//  3. Dispatches to the appropriate typed Handler.
//  4. Reports status transitions back via transport.Client.
//
// It is safe for concurrent use. Start and Stop must be called once each.
type Executor struct {
	cfg           Config
	transport     transport.Client
	handlers      registry
	journal       *journal
	sem           chan struct{} // bounded parallelism semaphore
	activeMu      sync.Mutex
	activeCancels map[string]context.CancelFunc
	log           *slog.Logger
	wg            sync.WaitGroup
}

// New creates a new Executor. Call Start to begin processing.
func New(cfg Config, t transport.Client, dockerCli *docker.Client, paths runtime.Paths, log *slog.Logger) *Executor {
	concurrency := cfg.MaxConcurrency
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = paths.DataDir
	}
	j := newJournal(dataDir, log)
	j.Load()

	workspacesDir := paths.WorkspacesDir
	if workspacesDir == "" {
		workspacesDir = filepath.Join(dataDir, "workspaces")
	}
	wsMgr := workspace.NewManager(workspacesDir)

	backupsDir := paths.BackupsDir
	if backupsDir == "" {
		backupsDir = filepath.Join(dataDir, "backups")
	}

	ex := &Executor{
		cfg:           cfg,
		transport:     t,
		journal:       j,
		sem:           make(chan struct{}, concurrency),
		activeCancels: make(map[string]context.CancelFunc),
		log:           log,
	}
	ex.handlers = buildRegistry(dockerCli, t, wsMgr, backupsDir, ex, log)
	return ex
}

// CancelCommand cancels the context of a currently running command.
// Returns true if the command was found and cancelled, false otherwise.
func (e *Executor) CancelCommand(commandID string) bool {
	e.activeMu.Lock()
	cancel, ok := e.activeCancels[commandID]
	e.activeMu.Unlock()

	if ok && cancel != nil {
		cancel()
		return true
	}
	return false
}

// Start begins the poll → dispatch loop. It runs until ctx is cancelled.
// Start is non-blocking; the loop runs in a managed goroutine.
func (e *Executor) Start(ctx context.Context) {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.runLoop(ctx)
	}()

	// Periodic journal pruning
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(pruneInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.journal.Prune()
			}
		}
	}()
}

// Stop waits for all in-flight commands to complete.
func (e *Executor) Stop() {
	e.wg.Wait()
}

// runLoop continuously polls for commands and dispatches each one.
func (e *Executor) runLoop(ctx context.Context) {
	pollBackoff := time.Duration(0)
	for {
		if ctx.Err() != nil {
			return
		}

		cmds, err := e.transport.PollCommands(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			e.log.Error("poll error", slog.String("error", err.Error()))
			// Back off on poll failures (including terminal auth) to avoid hot-loop (I8).
			if pollBackoff == 0 {
				pollBackoff = time.Second
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(pollBackoff):
			}
			if pollBackoff < 30*time.Second {
				pollBackoff *= 2
			}
			continue
		}
		pollBackoff = 0

		for _, cmd := range cmds {
			select {
			case <-ctx.Done():
				return
			default:
				e.dispatchCommand(ctx, cmd)
			}
		}
	}
}

// dispatchCommand performs all validation then runs the handler in a goroutine.
func (e *Executor) dispatchCommand(ctx context.Context, cmd protocol.CommandEnvelope) {
	cmdID := cmd.GetCommandID()
	logAttrs := []any{
		slog.String("command_id", cmdID),
		slog.String("operation", cmd.Operation),
	}
	if cmd.CorrelationID != nil && *cmd.CorrelationID != "" {
		logAttrs = append(logAttrs, slog.String("correlation_id", *cmd.CorrelationID))
	}
	if cmd.RequestID != nil && *cmd.RequestID != "" {
		logAttrs = append(logAttrs, slog.String("request_id", *cmd.RequestID))
	}
	log := e.log.With(logAttrs...)

	// ── RECEIVED ─────────────────────────────────────────────────────────────
	log.Info("command received")

	// ── VALIDATING ───────────────────────────────────────────────────────────
	if errCode, msg := e.validate(cmd); errCode != "" {
		e.report(ctx, cmdID, StateRejected, nil, (*string)(&errCode), &msg, log)
		return
	}

	// Acquire semaphore (non-blocking check first to give CAPACITY_EXCEEDED quickly)
	select {
	case e.sem <- struct{}{}:
	default:
		msg := "executor at capacity"
		code := string(ErrCodeCapacityExceeded)
		e.report(ctx, cmdID, StateRejected, nil, &code, &msg, log)
		return
	}

	// Journal BEFORE accepted so a crash after accept cannot lose replay protection (I8).
	expiresAt := parseTime(cmd.ExpiresAt)
	if expiresAt.IsZero() {
		// Defensive TTL so journal entries survive restart even if envelope omitted expiresAt.
		expiresAt = time.Now().UTC().Add(24 * time.Hour)
	}
	e.journal.Add(cmdID, expiresAt)

	// ── ACCEPTED ─────────────────────────────────────────────────────────────
	e.report(ctx, cmdID, StateAccepted, nil, nil, nil, log)

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer func() { <-e.sem }()
		e.executeCommand(ctx, cmd, expiresAt, log)
	}()
}

// validate performs all pre-dispatch checks. Returns an ErrCode and message if
// the command must be rejected, or empty strings if it is valid.
func (e *Executor) validate(cmd protocol.CommandEnvelope) (ErrCode, string) {
	cmdID := cmd.GetCommandID()
	if cmdID == "" {
		return ErrCodeInvalidPayload, "command ID is required"
	}

	// Schema version
	if cmd.SchemaVersion != protocol.SchemaVersion {
		return ErrCodeSchemaVersion, "unsupported schema version"
	}

	// Server ID must match our own
	if e.cfg.ServerID != uuid.Nil && cmd.ServerID != e.cfg.ServerID.String() {
		return ErrCodeServerMismatch, "command addressed to different server"
	}

	// Expiry check
	expiresAt := parseTime(cmd.ExpiresAt)
	if !expiresAt.IsZero() && time.Now().After(expiresAt) {
		return ErrCodeExpired, "command has expired"
	}

	// Replay protection — check before any acceptance
	if e.journal.Has(cmdID) {
		return ErrCodeDuplicate, "command already processed"
	}

	// Operation whitelist
	if _, ok := e.handlers[cmd.Operation]; !ok {
		return ErrCodeUnknownOperation, "unsupported operation: " + cmd.Operation
	}

	return "", ""
}

// executeCommand runs the handler for an already-accepted command.
func (e *Executor) executeCommand(ctx context.Context, cmd protocol.CommandEnvelope, expiresAt time.Time, log *slog.Logger) {
	cmdID := cmd.GetCommandID()
	// Build a per-command context with its own deadline.
	cmdCtx := ctx
	var cmdCancel context.CancelFunc

	if !expiresAt.IsZero() {
		cmdCtx, cmdCancel = context.WithDeadline(ctx, expiresAt)
	} else {
		cmdCtx, cmdCancel = context.WithCancel(ctx)
	}
	defer cmdCancel()

	e.activeMu.Lock()
	if e.activeCancels == nil {
		e.activeCancels = make(map[string]context.CancelFunc)
	}
	e.activeCancels[cmdID] = cmdCancel
	e.activeMu.Unlock()
	defer func() {
		e.activeMu.Lock()
		delete(e.activeCancels, cmdID)
		e.activeMu.Unlock()
	}()

	// ── RUNNING ──────────────────────────────────────────────────────────────
	e.report(ctx, cmdID, StateRunning, nil, nil, nil, log)
	log.Info("command running")

	handler := e.handlers[cmd.Operation]
	result, err := handler.Execute(cmdCtx, cmd.Payload)

	// ── COMPLETED / FAILED / EXPIRED / CANCELLED ──────────────────────────────
	switch {
	case cmdCtx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded):
		msg := "command deadline exceeded"
		code := string(ErrCodeExpired)
		e.report(ctx, cmdID, StateExpired, nil, &code, &msg, log)

	case cmdCtx.Err() == context.Canceled || errors.Is(err, context.Canceled):
		msg := "command was cancelled"
		code := string(ErrCodeCancelled)
		e.report(ctx, cmdID, StateCancelled, nil, &code, &msg, log)

	case err != nil:
		var execErr *ExecutionError
		if errors.As(err, &execErr) {
			code := string(execErr.Code)
			msg := execErr.Message
			e.report(ctx, cmdID, StateFailed, nil, &code, &msg, log)
		} else {
			code := string(ErrCodeInternalError)
			msg := err.Error()
			e.report(ctx, cmdID, StateFailed, nil, &code, &msg, log)
		}

	default:
		e.report(ctx, cmdID, StateCompleted, result.Output, nil, nil, log)
		log.Info("command completed")
	}
}

// report sends a status update to the Control Plane.
func (e *Executor) report(
	ctx context.Context,
	cmdID string,
	state CommandState,
	output map[string]any,
	errCode *string,
	errMsg *string,
	log *slog.Logger,
) {
	status := protocol.CommandStatusRequest{
		Status:       string(state),
		Result:       output,
		ErrorCode:    errCode,
		ErrorMessage: errMsg,
	}
	if err := e.transport.SendCommandStatus(ctx, cmdID, status); err != nil {
		log.Error("failed to report command status",
			slog.String("state", string(state)),
			slog.String("error", err.Error()),
		)
	}
}

// parseTime parses an RFC3339 time string, returning zero time on failure.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}
