package deployments

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/google/uuid"
)

type AuditMeta struct {
	IP        string
	UserAgent string
}

type Service struct {
	repo   Repository
	authz  *rbac.Authorizer
	audit  *audit.Writer
	log    *slog.Logger
	now    func() time.Time
	placer Placer
}

// Placer resolves a target server for an application at deploy time.
type Placer interface {
	EnsureApplicationPlacement(ctx context.Context, appID uuid.UUID) (uuid.UUID, error)
}

func NewService(repo Repository, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger) *Service {
	return &Service{repo: repo, authz: authz, audit: auditWriter, log: log, now: time.Now}
}

func (s *Service) WithPlacer(placer Placer) *Service {
	s.placer = placer
	return s
}

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, in CreateInput, meta AuditMeta) (Deployment, error) {
	app, err := s.repo.GetApplication(ctx, in.ApplicationID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Deployment{}, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return Deployment{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, app.OrganizationID, rbac.DeploymentCreate); err != nil {
		return Deployment{}, err
	}

	trigger := strings.TrimSpace(in.Trigger)
	if trigger == "" {
		trigger = TriggerManual
	}
	if !validTrigger(trigger) {
		return Deployment{}, apierror.Validation("invalid trigger", map[string]any{"trigger": "unsupported value"})
	}

	var idem *string
	if in.IdempotencyKey != nil {
		key := strings.TrimSpace(*in.IdempotencyKey)
		if key != "" {
			idem = &key
			existing, err := s.repo.GetByIdempotency(ctx, in.ApplicationID, key)
			if err == nil {
				events, _ := s.repo.ListEvents(ctx, existing.ID)
				existing.Events = events
				return existing, nil
			}
			if !errors.Is(err, ErrNotFound) {
				return Deployment{}, err
			}
		}
	}

	active, err := s.repo.CountActive(ctx, in.ApplicationID)
	if err != nil {
		return Deployment{}, err
	}
	if active > 0 {
		return Deployment{}, apierror.Conflict("application already has an in-progress deployment")
	}

	reqID := in.RequestID
	if reqID == "" {
		reqID = requestid.FromContext(ctx)
	}

	serverID := app.TargetServerID
	if s.placer != nil {
		placed, err := s.placer.EnsureApplicationPlacement(ctx, app.ID)
		if err != nil {
			return Deployment{}, err
		}
		serverID = &placed
	} else if serverID == nil {
		return Deployment{}, apierror.InsufficientResources("application has no target server", map[string]any{
			"applicationId": app.ID.String(),
		})
	}

	d, err := s.repo.CreateQueued(ctx, app.OrganizationID, app.ID, app.EnvironmentID, serverID,
		trigger, idem, in.CorrelationID, reqID, actorID, s.now().UTC())
	if err != nil {
		if errors.Is(err, ErrConflict) {
			if idem != nil {
				existing, gerr := s.repo.GetByIdempotency(ctx, in.ApplicationID, *idem)
				if gerr == nil {
					events, _ := s.repo.ListEvents(ctx, existing.ID)
					existing.Events = events
					return existing, nil
				}
			}
			return Deployment{}, apierror.Conflict("deployment already exists for idempotency key")
		}
		return Deployment{}, err
	}
	events, _ := s.repo.ListEvents(ctx, d.ID)
	d.Events = events

	s.writeAudit(ctx, &app.OrganizationID, &actorID, "deployment.create", "deployment", d.ID.String(), meta, nil, map[string]any{
		"applicationId": app.ID.String(), "status": d.Status, "trigger": d.Trigger,
	})
	return d, nil
}

func (s *Service) Rollback(ctx context.Context, actorID uuid.UUID, in RollbackInput, meta AuditMeta) (Deployment, error) {
	app, err := s.repo.GetApplication(ctx, in.ApplicationID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Deployment{}, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return Deployment{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, app.OrganizationID, rbac.DeploymentRollback); err != nil {
		return Deployment{}, err
	}

	status, revNumber, err := s.repo.GetRevisionForRollback(ctx, in.ApplicationID, in.TargetRevisionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Deployment{}, apierror.NotFoundCode(apierror.CodeRevisionNotFound, "target revision not found")
		}
		return Deployment{}, err
	}
	if err := EvaluateRollbackTarget(status); err != nil {
		return Deployment{}, err
	}

	active, err := s.repo.CountActive(ctx, in.ApplicationID)
	if err != nil {
		return Deployment{}, err
	}
	if active > 0 {
		return Deployment{}, apierror.Conflict("application already has an in-progress deployment")
	}

	reqID := in.RequestID
	if reqID == "" {
		reqID = requestid.FromContext(ctx)
	}

	serverID := app.TargetServerID
	if s.placer != nil {
		placed, err := s.placer.EnsureApplicationPlacement(ctx, app.ID)
		if err != nil {
			return Deployment{}, err
		}
		serverID = &placed
	} else if serverID == nil {
		return Deployment{}, apierror.InsufficientResources("application has no target server", map[string]any{
			"applicationId": app.ID.String(),
		})
	}

	d, err := s.repo.CreateQueued(ctx, app.OrganizationID, app.ID, app.EnvironmentID, serverID,
		TriggerRollback, nil, in.CorrelationID, reqID, actorID, s.now().UTC())
	if err != nil {
		return Deployment{}, err
	}
	if err := s.repo.SetTargetRevision(ctx, d.ID, in.TargetRevisionID); err != nil {
		return Deployment{}, err
	}
	d.TargetRevisionID = &in.TargetRevisionID

	events, _ := s.repo.ListEvents(ctx, d.ID)
	d.Events = events

	prevActive, _ := s.repo.GetActiveRevisionID(ctx, app.ID)
	after := map[string]any{
		"applicationId":    app.ID.String(),
		"status":           d.Status,
		"trigger":          d.Trigger,
		"targetRevisionId": in.TargetRevisionID.String(),
		"revisionNumber":   revNumber,
	}
	if prevActive != nil {
		after["previousActiveRevisionId"] = prevActive.String()
	}
	s.writeAudit(ctx, &app.OrganizationID, &actorID, "deployment.rollback", "deployment", d.ID.String(), meta, nil, after)
	return d, nil
}

func (s *Service) List(ctx context.Context, actorID, orgID uuid.UUID, applicationID *uuid.UUID, status *string, limit, offset int) ([]Deployment, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.DeploymentRead); err != nil {
		return nil, 0, err
	}
	if status != nil {
		st := strings.TrimSpace(*status)
		if st == "" {
			status = nil
		} else {
			status = &st
		}
	}
	return s.repo.List(ctx, orgID, applicationID, status, limit, offset)
}

func (s *Service) Get(ctx context.Context, actorID, id uuid.UUID) (Deployment, error) {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Deployment{}, apierror.NotFoundCode(apierror.CodeDeploymentNotFound, "deployment not found")
		}
		return Deployment{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, d.OrganizationID, rbac.DeploymentRead); err != nil {
		return Deployment{}, err
	}
	events, err := s.repo.ListEvents(ctx, id)
	if err != nil {
		return Deployment{}, err
	}
	d.Events = events
	return d, nil
}

func (s *Service) Cancel(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) (Deployment, error) {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Deployment{}, apierror.NotFoundCode(apierror.CodeDeploymentNotFound, "deployment not found")
		}
		return Deployment{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, d.OrganizationID, rbac.DeploymentCancel); err != nil {
		return Deployment{}, err
	}
	if IsTerminal(d.Status) {
		return Deployment{}, apierror.Conflict("deployment is already terminal")
	}
	if !CanTransition(d.Status, StatusCancelled) {
		return Deployment{}, apierror.Conflict("deployment cannot be cancelled from current status")
	}

	updated, err := s.repo.Transition(ctx, id, d.Status, TransitionInput{
		ToStatus:  StatusCancelled,
		Message:   "cancelled by user",
		RequestID: requestid.FromContext(ctx),
	}, s.now().UTC())
	if err != nil {
		return mapTransitionErr(err)
	}
	events, _ := s.repo.ListEvents(ctx, id)
	updated.Events = events
	s.writeAudit(ctx, &d.OrganizationID, &actorID, "deployment.cancel", "deployment", id.String(), meta,
		map[string]any{"status": d.Status}, map[string]any{"status": updated.Status},
	)
	return updated, nil
}

// Advance applies a validated state transition (used by orchestrator / workers in later phases).
func (s *Service) Advance(ctx context.Context, id uuid.UUID, from string, in TransitionInput) (Deployment, error) {
	updated, err := s.repo.Transition(ctx, id, from, in, s.now().UTC())
	if err != nil {
		return mapTransitionErr(err)
	}
	events, _ := s.repo.ListEvents(ctx, id)
	updated.Events = events
	return updated, nil
}

// AdminReconcile allows moving a terminal deployment for operational recovery.
func (s *Service) AdminReconcile(ctx context.Context, actorID, id uuid.UUID, to, message string, meta AuditMeta) (Deployment, error) {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Deployment{}, apierror.NotFoundCode(apierror.CodeDeploymentNotFound, "deployment not found")
		}
		return Deployment{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, d.OrganizationID, rbac.DeploymentCancel); err != nil {
		return Deployment{}, err
	}
	if !IsTerminal(d.Status) {
		return Deployment{}, apierror.Conflict("administrative reconciliation requires a terminal deployment")
	}
	updated, err := s.repo.AdminReconcile(ctx, id, d.Status, to, message, requestid.FromContext(ctx), s.now().UTC())
	if err != nil {
		return mapTransitionErr(err)
	}
	events, _ := s.repo.ListEvents(ctx, id)
	updated.Events = events
	s.writeAudit(ctx, &d.OrganizationID, &actorID, "deployment.reconcile", "deployment", id.String(), meta,
		map[string]any{"status": d.Status}, map[string]any{"status": updated.Status},
	)
	return updated, nil
}

func mapTransitionErr(err error) (Deployment, error) {
	switch {
	case errors.Is(err, ErrNotFound):
		return Deployment{}, apierror.NotFoundCode(apierror.CodeDeploymentNotFound, "deployment not found")
	case errors.Is(err, ErrTerminal):
		return Deployment{}, apierror.Conflict("deployment is terminal")
	case errors.Is(err, ErrInvalidTransition):
		return Deployment{}, apierror.Conflict("invalid deployment state transition")
	case errors.Is(err, ErrConflict):
		return Deployment{}, apierror.Conflict("concurrent deployment state change")
	default:
		return Deployment{}, err
	}
}

func validTrigger(t string) bool {
	switch t {
	case TriggerManual, TriggerGitPush, TriggerAPI, TriggerRollback, TriggerSchedule, TriggerSystem:
		return true
	default:
		return false
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
