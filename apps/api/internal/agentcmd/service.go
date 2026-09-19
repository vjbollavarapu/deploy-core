package agentcmd

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agents"
	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/deploycore/deploy-core/apps/api/pkg/validation"
	"github.com/google/uuid"
)

type AuditMeta struct {
	IP        string
	UserAgent string
}

type ServiceConfig struct {
	DefaultTTL time.Duration
}

type Service struct {
	repo  Repository
	authz *rbac.Authorizer
	audit *audit.Writer
	log   *slog.Logger
	cfg   ServiceConfig
	now   func() time.Time
	onComplete CompletionHook
}

// CompletionHook runs after a terminal agent command status update.
type CompletionHook func(ctx context.Context, cmd Command) error

func NewService(repo Repository, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger, cfg ServiceConfig) *Service {
	if cfg.DefaultTTL <= 0 {
		cfg.DefaultTTL = 10 * time.Minute
	}
	return &Service{repo: repo, authz: authz, audit: auditWriter, log: log, cfg: cfg, now: time.Now}
}

func (s *Service) WithCompletionHook(h CompletionHook) *Service {
	if h == nil {
		return s
	}
	prev := s.onComplete
	if prev == nil {
		s.onComplete = h
		return s
	}
	s.onComplete = func(ctx context.Context, cmd Command) error {
		if err := prev(ctx, cmd); err != nil {
			return err
		}
		return h(ctx, cmd)
	}
	return s
}

func (s *Service) Issue(ctx context.Context, actorID uuid.UUID, in IssueInput, meta AuditMeta) (Command, error) {
	orgID, status, err := s.repo.GetServerOrg(ctx, in.ServerID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Command{}, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return Command{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ServerUpdate); err != nil {
		return Command{}, err
	}
	if status == "DISABLED" {
		return Command{}, apierror.Conflict("cannot issue commands to a disabled server")
	}

	op := strings.TrimSpace(in.Operation)
	if !IsAllowedOperation(op) {
		return Command{}, apierror.Validation("unsupported operation", map[string]any{
			"fields": validation.Errors{{Field: "operation", Message: "must be a known agent command operation"}},
		})
	}
	if err := validatePayload(in.Payload); err != nil {
		return Command{}, err
	}

	ttl := in.TTL
	if ttl <= 0 {
		ttl = s.cfg.DefaultTTL
	}
	now := s.now().UTC()
	reqID := requestid.FromContext(ctx)
	var reqPtr, corrPtr *string
	if reqID != "" {
		reqPtr = &reqID
	}
	if c := strings.TrimSpace(in.CorrelationID); c != "" {
		corrPtr = &c
	}

	cmd, err := s.repo.Create(ctx, Command{
		OrganizationID: orgID,
		ServerID:       in.ServerID,
		Operation:      op,
		SchemaVersion:  SchemaVersion,
		Payload:        in.Payload,
		IssuedAt:       now,
		ExpiresAt:      now.Add(ttl),
		RequestID:      reqPtr,
		CorrelationID:  corrPtr,
		IssuedBy:       &actorID,
	})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Command{}, apierror.Validation("payload must be structured and must not include shell execution fields", nil)
		}
		return Command{}, err
	}
	s.writeAudit(ctx, &orgID, &actorID, "agent.command.issue", "agent_command", cmd.ID.String(), meta, nil, map[string]any{
		"operation": op, "serverId": in.ServerID.String(), "schemaVersion": SchemaVersion,
	})
	return cmd, nil
}

func (s *Service) Get(ctx context.Context, actorID, commandID uuid.UUID) (Command, error) {
	cmd, err := s.repo.Get(ctx, commandID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Command{}, apierror.NotFound("command not found")
		}
		return Command{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, cmd.OrganizationID, rbac.ServerRead); err != nil {
		return Command{}, err
	}
	return cmd, nil
}

func (s *Service) ListForServer(ctx context.Context, actorID, serverID uuid.UUID, limit, offset int) ([]Command, int64, error) {
	orgID, _, err := s.repo.GetServerOrg(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, 0, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return nil, 0, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ServerRead); err != nil {
		return nil, 0, err
	}
	return s.repo.ListForServer(ctx, orgID, serverID, limit, offset)
}

func (s *Service) Cancel(ctx context.Context, actorID, commandID uuid.UUID, meta AuditMeta) (Command, error) {
	cmd, err := s.repo.Get(ctx, commandID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Command{}, apierror.NotFound("command not found")
		}
		return Command{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, cmd.OrganizationID, rbac.ServerUpdate); err != nil {
		return Command{}, err
	}
	updated, err := s.repo.Cancel(ctx, commandID, cmd.OrganizationID, s.now().UTC())
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Command{}, apierror.Conflict("command cannot be cancelled in its current state")
		}
		return Command{}, err
	}
	s.writeAudit(ctx, &cmd.OrganizationID, &actorID, "agent.command.cancel", "agent_command", commandID.String(), meta,
		map[string]any{"status": cmd.Status}, map[string]any{"status": updated.Status},
	)
	return updated, nil
}

func (s *Service) PollPending(ctx context.Context, agent agents.Agent, limit int) ([]Command, error) {
	now := s.now().UTC()
	_ = s.repo.ExpirePending(ctx, agent.ServerID, now)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return s.repo.ListPendingForServer(ctx, agent.ServerID, limit, now)
}

func (s *Service) ReportStatus(ctx context.Context, agent agents.Agent, commandID uuid.UUID, status string, result map[string]any, errCode, errMsg *string) (Command, error) {
	status = strings.TrimSpace(strings.ToLower(status))
	var from []string
	switch status {
	case StatusAccepted:
		from = []string{StatusPending}
	case StatusRunning:
		from = []string{StatusPending, StatusAccepted}
	case StatusCompleted, StatusFailed:
		from = []string{StatusPending, StatusAccepted, StatusRunning}
	default:
		return Command{}, apierror.Validation("invalid status", map[string]any{
			"fields": validation.Errors{{Field: "status", Message: "must be accepted, running, completed, or failed"}},
		})
	}
	if err := validatePayload(result); err != nil {
		return Command{}, err
	}

	cmd, err := s.repo.Get(ctx, commandID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Command{}, apierror.NotFound("command not found")
		}
		return Command{}, err
	}
	if cmd.ServerID != agent.ServerID {
		return Command{}, apierror.Forbidden("command does not belong to this agent server")
	}
	if cmd.ExpiresAt.Before(s.now().UTC()) && cmd.Status == StatusPending {
		_ = s.repo.ExpirePending(ctx, agent.ServerID, s.now().UTC())
		return Command{}, apierror.Conflict("command expired")
	}

	updated, err := s.repo.UpdateStatus(ctx, commandID, agent.ServerID, from, status, result, errCode, errMsg, s.now().UTC())
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Command{}, apierror.Conflict("invalid command status transition")
		}
		return Command{}, err
	}
	if s.onComplete != nil && (updated.Status == StatusCompleted || updated.Status == StatusFailed) {
		if hookErr := s.onComplete(ctx, updated); hookErr != nil && s.log != nil {
			s.log.Warn("command completion hook failed",
				slog.String("error", hookErr.Error()),
				slog.String("commandId", updated.ID.String()),
				slog.String("operation", updated.Operation),
			)
		}
	}
	return updated, nil
}

func validatePayload(payload map[string]any) error {
	if payload == nil {
		return nil
	}
	var errs validation.Errors
	walkForbiddenKeys("", payload, &errs)
	if !errs.Empty() {
		return apierror.Validation("invalid command payload", errs.Details())
	}
	return nil
}

func walkForbiddenKeys(prefix string, v any, errs *validation.Errors) {
	forbidden := map[string]struct{}{
		"shell": {}, "command": {}, "script": {}, "exec": {},
		"bash": {}, "sh": {}, "powershell": {}, "cmd": {},
	}
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			lk := strings.ToLower(strings.TrimSpace(k))
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			if _, bad := forbidden[lk]; bad {
				errs.Add("payload."+path, "arbitrary execution fields are not allowed")
			}
			walkForbiddenKeys(path, child, errs)
		}
	case []any:
		for i, child := range t {
			path := prefix + "[" + strconv.Itoa(i) + "]"
			walkForbiddenKeys(path, child, errs)
		}
	}
}

func (s *Service) writeAudit(ctx context.Context, orgID, actorID *uuid.UUID, action, resourceType, resourceID string, meta AuditMeta, before, after map[string]any) {
	if s.audit == nil {
		return
	}
	if err := s.audit.Write(ctx, audit.Entry{
		OrganizationID: orgID,
		ActorUserID:    actorID,
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
