package backups

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agentcmd"
	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
	"github.com/deploycore/deploy-core/apps/api/internal/notifications"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/webhooks"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/google/uuid"
)

type CommandStore interface {
	Create(ctx context.Context, cmd agentcmd.Command) (agentcmd.Command, error)
}

type JobEnqueuer interface {
	Enqueue(ctx context.Context, in jobs.EnqueueInput) (jobs.Job, error)
}

type NotificationEmitter interface {
	Emit(ctx context.Context, in notifications.EmitInput) (int, error)
}

type WebhookEmitter interface {
	Emit(ctx context.Context, in webhooks.EmitInput) (int, error)
}

type Service struct {
	repo     Repository
	commands CommandStore
	queue    JobEnqueuer
	authz    *rbac.Authorizer
	audit    *audit.Writer
	log      *slog.Logger
	now      func() time.Time
	notify   NotificationEmitter
	webhooks WebhookEmitter
}

func NewService(repo Repository, commands CommandStore, queue JobEnqueuer, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger) *Service {
	return &Service{repo: repo, commands: commands, queue: queue, authz: authz, audit: auditWriter, log: log, now: time.Now}
}

func (s *Service) WithNotifier(n NotificationEmitter) *Service {
	s.notify = n
	return s
}

func (s *Service) WithWebhooks(w WebhookEmitter) *Service {
	s.webhooks = w
	return s
}

func (s *Service) CreateBackup(ctx context.Context, actorID uuid.UUID, in CreateBackupInput, meta AuditMeta) (Backup, error) {
	db, err := s.repo.GetDatabase(ctx, in.DatabaseID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Backup{}, apierror.NotFoundCode(apierror.CodeDatabaseNotFound, "database not found")
		}
		return Backup{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, db.OrganizationID, rbac.DatabaseBackup); err != nil {
		return Backup{}, err
	}
	switch db.Status {
	case "RUNNING", "DEGRADED", "STOPPED":
	default:
		return Backup{}, apierror.Conflict("database must be running, degraded, or stopped to backup")
	}

	dest, err := LookupDestination(in.Destination)
	if err != nil {
		return Backup{}, err
	}

	retentionDays := 7
	if in.RetentionDays != nil && *in.RetentionDays > 0 {
		retentionDays = *in.RetentionDays
	} else if v, ok := asInt(db.BackupPolicy["retentionDays"]); ok && v > 0 {
		retentionDays = v
	}
	until := s.now().UTC().Add(time.Duration(retentionDays) * 24 * time.Hour)

	b, err := s.repo.CreateBackup(ctx, Backup{
		OrganizationID:  db.OrganizationID,
		ServerID:        db.ServerID,
		ResourceType:    ResourceDatabase,
		ResourceID:      db.ID,
		Type:            TypePGLogical,
		Status:          StatusPending,
		DestinationType: dest.Type(),
		RetentionUntil:  &until,
		Metadata:        map[string]any{"retentionDays": retentionDays},
		CreatedBy:       &actorID,
	})
	if err != nil {
		return Backup{}, err
	}

	uri, err := dest.Allocate(ctx, db.OrganizationID, b.ID)
	if err != nil {
		return Backup{}, err
	}
	b.DestinationURI = uri
	b.Status = StatusQueued
	b, err = s.repo.UpdateBackup(ctx, b)
	if err != nil {
		return Backup{}, err
	}

	reqID := requestid.FromContext(ctx)
	var reqPtr *string
	if reqID != "" {
		reqPtr = &reqID
	}
	job, err := s.queue.Enqueue(ctx, jobs.EnqueueInput{
		OrganizationID:      &db.OrganizationID,
		Type:                jobs.TypeBackup,
		Payload:             map[string]any{"backupId": b.ID.String()},
		IdempotencyKey:      in.IdempotencyKey,
		MaxAttempts:         5,
		RequestID:           reqPtr,
		RelatedResourceType: strPtr("backup"),
		RelatedResourceID:   &b.ID,
	})
	if err != nil {
		b.Status = StatusFailed
		b.LastError = err.Error()
		_, _ = s.repo.UpdateBackup(ctx, b)
		s.emitBackupFailed(ctx, b)
		return Backup{}, err
	}
	b.JobID = &job.ID

	// Issue agent command promptly; job worker monitors terminal status.
	now := s.now().UTC()
	b.Status = StatusRunning
	b.StartedAt = &now
	cmd, err := s.issue(ctx, b.OrganizationID, b.ServerID, agentcmd.OpCreateBackup, map[string]any{
		"backupId":        b.ID.String(),
		"databaseId":      b.ResourceID.String(),
		"type":            b.Type,
		"destinationType": b.DestinationType,
		"destinationUri":  b.DestinationURI,
	}, &actorID)
	if err != nil {
		b.Status = StatusFailed
		b.LastError = err.Error()
		_, _ = s.repo.UpdateBackup(ctx, b)
		s.emitBackupFailed(ctx, b)
		return Backup{}, err
	}
	b.CommandID = &cmd.ID
	b, err = s.repo.UpdateBackup(ctx, b)
	if err != nil {
		return Backup{}, err
	}

	s.writeAudit(ctx, &db.OrganizationID, &actorID, "backup.create", "backup", b.ID.String(), meta, nil, map[string]any{
		"databaseId": db.ID.String(), "destinationType": b.DestinationType, "jobId": job.ID.String(),
		"commandId": cmd.ID.String(),
	})
	return b, nil
}

func (s *Service) ListBackups(ctx context.Context, actorID, orgID uuid.UUID, databaseID *uuid.UUID, limit, offset int) ([]Backup, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.DatabaseRead); err != nil {
		return nil, 0, err
	}
	return s.repo.ListBackups(ctx, orgID, databaseID, limit, offset)
}

func (s *Service) GetBackup(ctx context.Context, actorID, id uuid.UUID) (Backup, error) {
	b, err := s.repo.GetBackup(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Backup{}, apierror.NotFound("backup not found")
		}
		return Backup{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, b.OrganizationID, rbac.DatabaseRead); err != nil {
		return Backup{}, err
	}
	return b, nil
}

func (s *Service) DeleteBackup(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) error {
	b, err := s.repo.GetBackup(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFound("backup not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, b.OrganizationID, rbac.DatabaseBackup); err != nil {
		return err
	}
	if err := s.repo.SoftDeleteBackup(ctx, id, s.now().UTC()); err != nil {
		return err
	}
	s.writeAudit(ctx, &b.OrganizationID, &actorID, "backup.delete", "backup", id.String(), meta,
		map[string]any{"status": b.Status}, map[string]any{"status": StatusDeleted},
	)
	return nil
}

func (s *Service) CreateRestore(ctx context.Context, actorID uuid.UUID, in CreateRestoreInput, meta AuditMeta) (Restore, error) {
	if strings.TrimSpace(in.Confirm) != RestoreConfirmPhrase {
		return Restore{}, apierror.Validation("destructive restore requires confirm field set to RESTORE", map[string]any{
			"field":   "confirm",
			"required": RestoreConfirmPhrase,
		})
	}
	b, err := s.repo.GetBackup(ctx, in.BackupID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Restore{}, apierror.NotFound("backup not found")
		}
		return Restore{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, b.OrganizationID, rbac.DatabaseRestore); err != nil {
		return Restore{}, err
	}
	if b.Status != StatusSucceeded || strings.TrimSpace(b.Checksum) == "" {
		return Restore{}, apierror.Conflict("backup must be succeeded with a checksum before restore")
	}

	target, err := s.repo.GetDatabase(ctx, in.TargetDatabaseID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Restore{}, apierror.NotFoundCode(apierror.CodeDatabaseNotFound, "target database not found")
		}
		return Restore{}, err
	}
	if target.OrganizationID != b.OrganizationID {
		return Restore{}, apierror.Forbidden("target database is outside backup organization")
	}

	rest, err := s.repo.CreateRestore(ctx, Restore{
		OrganizationID:     b.OrganizationID,
		BackupID:           b.ID,
		TargetResourceType: ResourceDatabase,
		TargetResourceID:   target.ID,
		ServerID:           target.ServerID,
		Status:             RestorePending,
		Metadata:           map[string]any{"confirm": RestoreConfirmPhrase},
		CreatedBy:          &actorID,
	})
	if err != nil {
		return Restore{}, err
	}
	rest.Status = RestoreQueued
	rest, err = s.repo.UpdateRestore(ctx, rest)
	if err != nil {
		return Restore{}, err
	}

	reqID := requestid.FromContext(ctx)
	var reqPtr *string
	if reqID != "" {
		reqPtr = &reqID
	}
	job, err := s.queue.Enqueue(ctx, jobs.EnqueueInput{
		OrganizationID:      &b.OrganizationID,
		Type:                jobs.TypeRestore,
		Payload:             map[string]any{"restoreId": rest.ID.String()},
		IdempotencyKey:      in.IdempotencyKey,
		MaxAttempts:         3,
		RequestID:           reqPtr,
		RelatedResourceType: strPtr("restore"),
		RelatedResourceID:   &rest.ID,
	})
	if err != nil {
		rest.Status = RestoreFailed
		rest.LastError = err.Error()
		_, _ = s.repo.UpdateRestore(ctx, rest)
		return Restore{}, err
	}
	rest.JobID = &job.ID

	now := s.now().UTC()
	rest.Status = RestoreRunning
	rest.StartedAt = &now
	cmd, err := s.issue(ctx, rest.OrganizationID, rest.ServerID, agentcmd.OpRestoreBackup, map[string]any{
		"restoreId":         rest.ID.String(),
		"backupId":          b.ID.String(),
		"targetDatabaseId":  target.ID.String(),
		"destinationType":   b.DestinationType,
		"destinationUri":    b.DestinationURI,
		"checksum":          b.Checksum,
		"requireValidation": true,
	}, &actorID)
	if err != nil {
		rest.Status = RestoreFailed
		rest.LastError = err.Error()
		_, _ = s.repo.UpdateRestore(ctx, rest)
		return Restore{}, err
	}
	rest.CommandID = &cmd.ID
	rest, err = s.repo.UpdateRestore(ctx, rest)
	if err != nil {
		return Restore{}, err
	}

	s.writeAudit(ctx, &b.OrganizationID, &actorID, "backup.restore", "restore", rest.ID.String(), meta, nil, map[string]any{
		"backupId": b.ID.String(), "targetDatabaseId": target.ID.String(), "jobId": job.ID.String(),
		"commandId": cmd.ID.String(), "destructiveConfirm": true,
	})
	return rest, nil
}

func (s *Service) GetRestore(ctx context.Context, actorID, id uuid.UUID) (Restore, error) {
	r, err := s.repo.GetRestore(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Restore{}, apierror.NotFound("restore not found")
		}
		return Restore{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, r.OrganizationID, rbac.DatabaseRead); err != nil {
		return Restore{}, err
	}
	return r, nil
}

func (s *Service) ListRestores(ctx context.Context, actorID, orgID uuid.UUID, backupID, targetID *uuid.UUID, limit, offset int) ([]Restore, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.DatabaseRead); err != nil {
		return nil, 0, err
	}
	return s.repo.ListRestores(ctx, orgID, backupID, targetID, limit, offset)
}

// ProcessBackupJob is the BACKUP job handler: issue agent command, wait for terminal status.
func (s *Service) ProcessBackupJob(ctx context.Context, job jobs.Job) error {
	raw, _ := job.Payload["backupId"].(string)
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return errors.New("backupId missing in job payload")
	}
	b, err := s.repo.GetBackup(ctx, id)
	if err != nil {
		return err
	}
	switch b.Status {
	case StatusSucceeded:
		return nil
	case StatusFailed, StatusDeleted, StatusExpired:
		return errors.New(b.LastError)
	}

	if b.CommandID == nil {
		now := s.now().UTC()
		b.Status = StatusRunning
		b.StartedAt = &now
		cmd, err := s.issue(ctx, b.OrganizationID, b.ServerID, agentcmd.OpCreateBackup, map[string]any{
			"backupId":        b.ID.String(),
			"databaseId":      b.ResourceID.String(),
			"type":            b.Type,
			"destinationType": b.DestinationType,
			"destinationUri":  b.DestinationURI,
		}, b.CreatedBy)
		if err != nil {
			b.Status = StatusFailed
			b.LastError = err.Error()
			_, _ = s.repo.UpdateBackup(ctx, b)
			s.emitBackupFailed(ctx, b)
			return err
		}
		b.CommandID = &cmd.ID
		if _, err := s.repo.UpdateBackup(ctx, b); err != nil {
			return err
		}
		return jobs.ErrRetryLater
	}

	// Waiting for agent completion hook to flip status.
	return jobs.ErrRetryLater
}

// ProcessRestoreJob is the RESTORE job handler.
func (s *Service) ProcessRestoreJob(ctx context.Context, job jobs.Job) error {
	raw, _ := job.Payload["restoreId"].(string)
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return errors.New("restoreId missing in job payload")
	}
	r, err := s.repo.GetRestore(ctx, id)
	if err != nil {
		return err
	}
	switch r.Status {
	case RestoreSucceeded:
		if r.ValidationPassed == nil || !*r.ValidationPassed {
			return errors.New("restore marked succeeded without validation")
		}
		return nil
	case RestoreFailed, RestoreCancelled:
		return errors.New(r.LastError)
	}

	if r.CommandID == nil {
		b, err := s.repo.GetBackup(ctx, r.BackupID)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		r.Status = RestoreRunning
		r.StartedAt = &now
		cmd, err := s.issue(ctx, r.OrganizationID, r.ServerID, agentcmd.OpRestoreBackup, map[string]any{
			"restoreId":          r.ID.String(),
			"backupId":           b.ID.String(),
			"targetDatabaseId":   r.TargetResourceID.String(),
			"destinationType":    b.DestinationType,
			"destinationUri":     b.DestinationURI,
			"checksum":           b.Checksum,
			"requireValidation":  true,
		}, r.CreatedBy)
		if err != nil {
			r.Status = RestoreFailed
			r.LastError = err.Error()
			_, _ = s.repo.UpdateRestore(ctx, r)
			return err
		}
		r.CommandID = &cmd.ID
		if _, err := s.repo.UpdateRestore(ctx, r); err != nil {
			return err
		}
		return jobs.ErrRetryLater
	}
	return jobs.ErrRetryLater
}

func (s *Service) HandleCommandCompletion(ctx context.Context, cmd agentcmd.Command) error {
	switch cmd.Operation {
	case agentcmd.OpCreateBackup:
		return s.onBackupCommand(ctx, cmd)
	case agentcmd.OpRestoreBackup:
		return s.onRestoreCommand(ctx, cmd)
	default:
		return nil
	}
}

func (s *Service) onBackupCommand(ctx context.Context, cmd agentcmd.Command) error {
	raw, _ := cmd.Payload["backupId"].(string)
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	b, err := s.repo.GetBackup(ctx, id)
	if err != nil {
		return nil
	}
	now := s.now().UTC()
	if cmd.Status == agentcmd.StatusCompleted {
		checksum := ""
		var size *int64
		if cmd.Result != nil {
			if c, ok := cmd.Result["checksum"].(string); ok {
				checksum = strings.TrimSpace(c)
			}
			if n, ok := asInt64(cmd.Result["sizeBytes"]); ok {
				size = &n
			}
			if u, ok := cmd.Result["destinationUri"].(string); ok && strings.TrimSpace(u) != "" {
				b.DestinationURI = strings.TrimSpace(u)
			}
		}
		if checksum == "" {
			b.Status = StatusFailed
			b.LastError = "backup completed without checksum"
			b.CompletedAt = &now
			_, err = s.repo.UpdateBackup(ctx, b)
			if err == nil {
				s.emitBackupFailed(ctx, b)
			}
			return err
		}
		b.Status = StatusSucceeded
		b.Checksum = checksum
		b.SizeBytes = size
		b.CompletedAt = &now
		b.LastError = ""
		if b.StartedAt != nil {
			ms := now.Sub(*b.StartedAt).Milliseconds()
			b.DurationMs = &ms
		}
		_, err = s.repo.UpdateBackup(ctx, b)
		if err == nil {
			s.emitBackupCompleted(ctx, b)
		}
		return err
	}
	if cmd.Status == agentcmd.StatusFailed {
		msg := "backup failed"
		if cmd.ErrorMessage != nil {
			msg = *cmd.ErrorMessage
		}
		b.Status = StatusFailed
		b.LastError = msg
		b.CompletedAt = &now
		_, err = s.repo.UpdateBackup(ctx, b)
		if err == nil {
			s.emitBackupFailed(ctx, b)
		}
		return err
	}
	return nil
}

func (s *Service) onRestoreCommand(ctx context.Context, cmd agentcmd.Command) error {
	raw, _ := cmd.Payload["restoreId"].(string)
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	r, err := s.repo.GetRestore(ctx, id)
	if err != nil {
		return nil
	}
	now := s.now().UTC()
	if cmd.Status == agentcmd.StatusCompleted {
		r.Status = RestoreValidating
		validated := false
		if cmd.Result != nil {
			if v, ok := cmd.Result["validationPassed"].(bool); ok {
				validated = v
			}
		}
		// Never report restore successful until validation completes successfully.
		if !validated {
			r.Status = RestoreFailed
			r.ValidationPassed = boolPtr(false)
			r.LastError = "restore completed without successful validation"
			r.CompletedAt = &now
			_, err = s.repo.UpdateRestore(ctx, r)
			return err
		}
		r.Status = RestoreSucceeded
		r.ValidationPassed = boolPtr(true)
		r.LastError = ""
		r.CompletedAt = &now
		if r.StartedAt != nil {
			ms := now.Sub(*r.StartedAt).Milliseconds()
			r.DurationMs = &ms
		}
		_, err = s.repo.UpdateRestore(ctx, r)
		return err
	}
	if cmd.Status == agentcmd.StatusFailed {
		msg := "restore failed"
		if cmd.ErrorMessage != nil {
			msg = *cmd.ErrorMessage
		}
		r.Status = RestoreFailed
		r.ValidationPassed = boolPtr(false)
		r.LastError = msg
		r.CompletedAt = &now
		_, err = s.repo.UpdateRestore(ctx, r)
		return err
	}
	return nil
}

func (s *Service) issue(ctx context.Context, orgID, serverID uuid.UUID, op string, payload map[string]any, issuedBy *uuid.UUID) (agentcmd.Command, error) {
	if s.commands == nil {
		return agentcmd.Command{}, apierror.Internal("command store not configured")
	}
	now := s.now().UTC()
	reqID := requestid.FromContext(ctx)
	var reqPtr *string
	if reqID != "" {
		reqPtr = &reqID
	}
	return s.commands.Create(ctx, agentcmd.Command{
		OrganizationID: orgID,
		ServerID:       serverID,
		Operation:      op,
		SchemaVersion:  agentcmd.SchemaVersion,
		Payload:        payload,
		IssuedAt:       now,
		ExpiresAt:      now.Add(60 * time.Minute),
		RequestID:      reqPtr,
		IssuedBy:       issuedBy,
	})
}

func (s *Service) writeAudit(ctx context.Context, orgID, actorID *uuid.UUID, action, resourceType, resourceID string, meta AuditMeta, before, after map[string]any) {
	if s.audit == nil {
		return
	}
	if err := s.audit.Write(ctx, audit.Entry{
		OrganizationID: orgID,
		ActorUserID:    actorID,
		ActorType:      "user",
		Action:         action,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		RequestID:      requestid.FromContext(ctx),
		IPAddress:      meta.IP,
		UserAgent:      meta.UserAgent,
		Before:         before,
		After:          after,
	}); err != nil {
		s.log.Error("audit write failed", slog.String("error", err.Error()), slog.String("action", action))
	}
}

func (s *Service) emitBackupFailed(ctx context.Context, b Backup) {
	if s.notify != nil {
		sid := b.ServerID
		rid := b.ID
		if _, err := s.notify.Emit(ctx, notifications.EmitInput{
			OrganizationID: b.OrganizationID,
			EventType:      notifications.EventBackupFailed,
			ServerID:       &sid,
			ResourceType:   "backup",
			ResourceID:     &rid,
			Payload: map[string]any{
				"backupId":     b.ID.String(),
				"resourceType": b.ResourceType,
				"resourceId":   b.ResourceID.String(),
				"lastError":    b.LastError,
			},
		}); err != nil && s.log != nil {
			s.log.Warn("backup failed notification emit failed", slog.String("error", err.Error()))
		}
	}
	s.emitWebhook(ctx, webhooks.EventBackupFailed, b, map[string]any{"lastError": b.LastError})
}

func (s *Service) emitBackupCompleted(ctx context.Context, b Backup) {
	s.emitWebhook(ctx, webhooks.EventBackupCompleted, b, map[string]any{
		"checksum":  b.Checksum,
		"sizeBytes": b.SizeBytes,
	})
}

func (s *Service) emitWebhook(ctx context.Context, eventType string, b Backup, extra map[string]any) {
	if s.webhooks == nil {
		return
	}
	sid := b.ServerID
	rid := b.ID
	payload := map[string]any{
		"backupId":     b.ID.String(),
		"resourceType": b.ResourceType,
		"resourceId":   b.ResourceID.String(),
	}
	for k, v := range extra {
		payload[k] = v
	}
	if _, err := s.webhooks.Emit(ctx, webhooks.EmitInput{
		OrganizationID: b.OrganizationID,
		EventType:      eventType,
		ServerID:       &sid,
		ResourceType:   "backup",
		ResourceID:     &rid,
		Payload:        payload,
	}); err != nil && s.log != nil {
		s.log.Warn("backup webhook emit failed", slog.String("error", err.Error()), slog.String("event", eventType))
	}
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
