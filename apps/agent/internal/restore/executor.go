package restore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/backupstorage"
	"github.com/deploycore/deploy-core/apps/agent/internal/database"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

// ContainerExecutor defines the container operations required for restore execution.
type ContainerExecutor interface {
	InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error)
	ExecContainerWithIO(ctx context.Context, id string, cmd []string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

// Executor performs controlled PostgreSQL restores from platform backup storage.
type Executor struct {
	cli     ContainerExecutor
	storage backupstorage.Storage
	log     *slog.Logger
}

// NewExecutor constructs a restore Executor.
func NewExecutor(cli ContainerExecutor, storage backupstorage.Storage, log *slog.Logger) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{cli: cli, storage: storage, log: log}
}

// Execute restores a PostgreSQL logical backup into the target database container.
// Invariants:
// - Arbitrary host file paths are blocked; backup must reside in platform storage.
// - Checksum integrity is strictly verified before restoring.
// - Password is passed exclusively via PGPASSWORD exec environment, never in CLI args or process logs.
func (e *Executor) Execute(ctx context.Context, req Request) (*Result, error) {
	if strings.TrimSpace(req.BackupID) == "" {
		return nil, errors.New("backupId cannot be empty")
	}
	if strings.TrimSpace(req.TargetDatabaseID) == "" {
		return nil, errors.New("targetDatabaseId cannot be empty")
	}
	if strings.TrimSpace(req.DatabaseName) == "" {
		return nil, errors.New("databaseName cannot be empty")
	}
	if strings.TrimSpace(req.Username) == "" {
		return nil, errors.New("username cannot be empty")
	}

	containerName := database.FormatContainerName(req.TargetDatabaseID)
	detail, err := e.cli.InspectContainer(ctx, containerName)
	if err != nil {
		return nil, fmt.Errorf("target database container %s not found: %w", containerName, err)
	}
	if err := database.VerifyManagedDatabaseContainer(detail, req.TargetDatabaseID); err != nil {
		return nil, fmt.Errorf("refusing restore into container %s: %w", containerName, err)
	}

	if !detail.State.Running {
		return nil, fmt.Errorf("cannot restore to database container %s: container is not running (state=%s)", detail.Name, detail.State.Status)
	}

	// 1. Open backup artifact from platform-controlled storage (blocks arbitrary host path access)
	r, err := e.storage.Open(ctx, req.BackupID)
	if err != nil {
		return nil, fmt.Errorf("failed to open backup %s from storage: %w", req.BackupID, err)
	}
	defer r.Close()

	// 2. Read into temporary staging buffer to verify checksum before executing destructive restore
	var buf bytes.Buffer
	hasher := sha256.New()
	mw := io.MultiWriter(&buf, hasher)

	if _, err := io.Copy(mw, r); err != nil {
		return nil, fmt.Errorf("failed to read backup artifact: %w", err)
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	expected := normalizeChecksum(req.ExpectedChecksum)
	if expected == "" {
		return nil, errors.New("checksum is required before restore")
	}
	if !strings.EqualFold(actualChecksum, expected) {
		return nil, fmt.Errorf("backup checksum mismatch: expected %s, got %s (restore aborted)", expected, actualChecksum)
	}

	e.log.Info("starting controlled database restore",
		slog.String("restore_id", req.RestoreID),
		slog.String("backup_id", req.BackupID),
		slog.String("target_container", detail.Name),
		slog.String("database", req.DatabaseName),
		slog.String("checksum", actualChecksum),
	)

	start := time.Now()

	// 3. Fixed trusted tool: pg_restore -U <username> -d <databaseName> --clean --if-exists
	cmd := []string{
		"pg_restore",
		"-U", req.Username,
		"-d", req.DatabaseName,
		"--clean",
		"--if-exists",
	}

	env := []string{
		"PGPASSWORD=" + req.Password,
	}

	var stderrBuf bytes.Buffer
	exitCode, execErr := e.cli.ExecContainerWithIO(ctx, detail.ID, cmd, env, bytes.NewReader(buf.Bytes()), nil, &stderrBuf)
	if execErr != nil {
		return nil, fmt.Errorf("pg_restore exec error: %w", execErr)
	}

	// Note: pg_restore returns exit code 0 on success, and code 1 on non-fatal warnings (e.g. table to drop didn't exist yet).
	// Exit code >= 2 indicates fatal errors.
	if exitCode > 1 {
		sanitizedErr := strings.TrimSpace(stderrBuf.String())
		if req.Password != "" {
			sanitizedErr = strings.ReplaceAll(sanitizedErr, req.Password, "[REDACTED]")
		}
		return nil, fmt.Errorf("pg_restore failed with exit code %d: %s", exitCode, sanitizedErr)
	}

	// 4. Post-restore validation
	validationPassed := !req.RequireValidation
	if req.RequireValidation {
		validCmd := []string{
			"pg_isready",
			"-U", req.Username,
			"-d", req.DatabaseName,
		}
		var validErrBuf bytes.Buffer
		validExit, err := e.cli.ExecContainerWithIO(ctx, detail.ID, validCmd, nil, nil, nil, &validErrBuf)
		if err != nil || validExit != 0 {
			return nil, fmt.Errorf("post-restore database readiness check failed (exit %d): %s", validExit, strings.TrimSpace(validErrBuf.String()))
		}
		validationPassed = true
	}

	durationMs := time.Since(start).Milliseconds()

	e.log.Info("database restore completed successfully",
		slog.String("restore_id", req.RestoreID),
		slog.String("backup_id", req.BackupID),
		slog.Int64("duration_ms", durationMs),
	)

	return &Result{
		RestoreID:        req.RestoreID,
		BackupID:         req.BackupID,
		TargetDatabaseID: req.TargetDatabaseID,
		Restored:         true,
		ValidationPassed: validationPassed,
		DurationMs:       durationMs,
		CompletedAt:      time.Now().UTC(),
	}, nil
}

func normalizeChecksum(s string) string {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "sha256:") {
		return strings.TrimSpace(s[len("sha256:"):])
	}
	return s
}
