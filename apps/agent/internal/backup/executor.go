package backup

import (
	"bytes"
	"context"
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

// ContainerExecutor defines the container operations required for backup execution.
type ContainerExecutor interface {
	InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error)
	ExecContainerWithIO(ctx context.Context, id string, cmd []string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

// Executor performs PostgreSQL logical backups by executing pg_dump inside the database container.
type Executor struct {
	cli     ContainerExecutor
	storage backupstorage.Storage
	log     *slog.Logger
}

// NewExecutor constructs a backup Executor.
func NewExecutor(cli ContainerExecutor, storage backupstorage.Storage, log *slog.Logger) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{cli: cli, storage: storage, log: log}
}

// Execute triggers a logical PostgreSQL backup using pg_dump with structured arguments.
// Invariant: Arbitrary host shell strings are prohibited.
// Invariant: Database password is passed solely via PGPASSWORD exec environment, never in CLI args or process logs.
func (e *Executor) Execute(ctx context.Context, req Request) (*Result, error) {
	if strings.TrimSpace(req.BackupID) == "" {
		return nil, errors.New("backupId cannot be empty")
	}
	if strings.TrimSpace(req.DatabaseID) == "" {
		return nil, errors.New("databaseId cannot be empty")
	}
	if strings.TrimSpace(req.DatabaseName) == "" {
		return nil, errors.New("databaseName cannot be empty")
	}
	if strings.TrimSpace(req.Username) == "" {
		return nil, errors.New("username cannot be empty")
	}

	containerName := database.FormatContainerName(req.DatabaseID)
	detail, err := e.cli.InspectContainer(ctx, containerName)
	if err != nil {
		return nil, fmt.Errorf("target database container %s not found: %w", containerName, err)
	}
	if err := database.VerifyManagedDatabaseContainer(detail, req.DatabaseID); err != nil {
		return nil, fmt.Errorf("refusing backup of container %s: %w", containerName, err)
	}

	if !detail.State.Running {
		return nil, fmt.Errorf("cannot backup database container %s: container is not running (state=%s)", detail.Name, detail.State.Status)
	}

	e.log.Info("starting logical database backup",
		slog.String("backup_id", req.BackupID),
		slog.String("container", detail.Name),
		slog.String("database", req.DatabaseName),
		slog.String("user", req.Username),
	)

	start := time.Now()

	// Fixed trusted command array: pg_dump -U <username> -d <databaseName> -F c
	// Custom format (-F c) is compressed by default and supports pg_restore.
	cmd := []string{
		"pg_dump",
		"-U", req.Username,
		"-d", req.DatabaseName,
		"-F", "c",
	}

	// Password passed securely via exec environment variable
	env := []string{
		"PGPASSWORD=" + req.Password,
	}

	pr, pw := io.Pipe()
	var stderrBuf bytes.Buffer
	var exitCode int
	var execErr error

	execDone := make(chan struct{})
	go func() {
		defer close(execDone)
		exitCode, execErr = e.cli.ExecContainerWithIO(ctx, detail.ID, cmd, env, nil, pw, &stderrBuf)
		if execErr != nil {
			_ = pw.CloseWithError(execErr)
		} else {
			_ = pw.Close()
		}
	}()

	// Stream directly to platform backup storage
	meta, saveErr := e.storage.Save(ctx, req.BackupID, pr)
	<-execDone

	if execErr != nil {
		_ = e.storage.Delete(ctx, req.BackupID)
		return nil, fmt.Errorf("pg_dump exec error: %w", execErr)
	}

	if exitCode != 0 {
		_ = e.storage.Delete(ctx, req.BackupID)
		sanitizedErr := strings.TrimSpace(stderrBuf.String())
		if req.Password != "" {
			sanitizedErr = strings.ReplaceAll(sanitizedErr, req.Password, "[REDACTED]")
		}
		return nil, fmt.Errorf("pg_dump failed with exit code %d: %s", exitCode, sanitizedErr)
	}

	if saveErr != nil {
		_ = e.storage.Delete(ctx, req.BackupID)
		return nil, fmt.Errorf("failed to save backup: %w", saveErr)
	}

	durationMs := time.Since(start).Milliseconds()

	e.log.Info("database backup completed successfully",
		slog.String("backup_id", req.BackupID),
		slog.Int64("size_bytes", meta.SizeBytes),
		slog.String("checksum", meta.Checksum),
		slog.Int64("duration_ms", durationMs),
	)

	return &Result{
		BackupID:       req.BackupID,
		Checksum:       meta.Checksum,
		SizeBytes:      meta.SizeBytes,
		DestinationURI: meta.URI,
		DurationMs:     durationMs,
		CompletedAt:    time.Now().UTC(),
	}, nil
}
