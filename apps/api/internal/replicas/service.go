package replicas

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/google/uuid"
)

type JobEnqueuer interface {
	Enqueue(ctx context.Context, in jobs.EnqueueInput) (jobs.Job, error)
}

type CapacityRefresher interface {
	RefreshAllocated(ctx context.Context, serverIDs ...uuid.UUID) error
	ValidateTargetCapacity(ctx context.Context, orgID, serverID uuid.UUID, cpuMillis int, memBytes, diskBytes int64, excludeAppID *uuid.UUID) error
}

type Service struct {
	repo     Repository
	authz    *rbac.Authorizer
	audit    *audit.Writer
	queue    JobEnqueuer
	capacity CapacityRefresher
	log      *slog.Logger
	now      func() time.Time
}

func NewService(repo Repository, authz *rbac.Authorizer, auditWriter *audit.Writer, queue JobEnqueuer, log *slog.Logger) *Service {
	return &Service{repo: repo, authz: authz, audit: auditWriter, queue: queue, log: log, now: time.Now}
}

func (s *Service) WithCapacity(c CapacityRefresher) *Service {
	s.capacity = c
	return s
}

func (s *Service) GetSummary(ctx context.Context, actorID, appID uuid.UUID) (Summary, error) {
	meta, err := s.repo.GetApplicationMeta(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Summary{}, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return Summary{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, meta.OrganizationID, rbac.ApplicationRead); err != nil {
		return Summary{}, err
	}
	runtime, _, err := s.repo.GetLatestConfigRuntime(ctx, appID)
	if err != nil {
		return Summary{}, err
	}
	list, err := s.repo.ListByApplication(ctx, appID)
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{
		ApplicationID:   appID,
		DesiredReplicas: DesiredFromRuntime(runtime),
		Replicas:        list,
	}
	for _, r := range list {
		if r.Status != StatusStopped && r.Status != StatusFailed {
			sum.ObservedReplicas++
		}
		if r.Healthy && r.Status == StatusRunning {
			sum.HealthyReplicas++
		}
		if r.RoutingEnabled {
			sum.RoutingReplicas++
		}
	}
	return sum, nil
}

func (s *Service) Scale(ctx context.Context, actorID, appID uuid.UUID, desired int, meta AuditMeta) (Summary, error) {
	if err := ValidateDesired(desired); err != nil {
		return Summary{}, apierror.Validation(err.Error(), map[string]any{"desiredReplicas": err.Error()})
	}
	app, err := s.repo.GetApplicationMeta(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Summary{}, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return Summary{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, app.OrganizationID, rbac.ApplicationUpdate); err != nil {
		return Summary{}, err
	}

	runtime, cfg, err := s.repo.GetLatestConfigRuntime(ctx, appID)
	if err != nil {
		return Summary{}, err
	}
	prev := DesiredFromRuntime(runtime)
	if prev == desired {
		return s.GetSummary(ctx, actorID, appID)
	}

	if app.ServerID != nil && s.capacity != nil && desired > prev {
		cpu, mem, disk := 0, int64(0), int64(0)
		if cfg.CPULimitMillis != nil {
			cpu = *cfg.CPULimitMillis
		}
		if cfg.MemoryLimitBytes != nil {
			mem = *cfg.MemoryLimitBytes
		}
		if runtime != nil {
			switch v := runtime["diskBytes"].(type) {
			case float64:
				disk = int64(v)
			case int64:
				disk = v
			case int:
				disk = int64(v)
			}
		}
		totalCPU, totalMem, totalDisk := MultiplyLimits(cpu, mem, disk, desired)
		exclude := &appID
		if err := s.capacity.ValidateTargetCapacity(ctx, app.OrganizationID, *app.ServerID, totalCPU, totalMem, totalDisk, exclude); err != nil {
			return Summary{}, err
		}
	}

	nextRuntime := SetDesiredRuntime(runtime, desired)
	if err := s.repo.InsertConfigVersion(ctx, app.OrganizationID, appID, actorID, nextRuntime); err != nil {
		return Summary{}, err
	}
	if app.ServerID != nil && s.capacity != nil {
		_ = s.capacity.RefreshAllocated(ctx, *app.ServerID)
	}

	if s.queue != nil {
		org := app.OrganizationID
		key := "replicas-reconcile:" + appID.String()
		_, _ = s.queue.Enqueue(ctx, jobs.EnqueueInput{
			OrganizationID:      &org,
			Type:                jobs.TypeReplicasReconcile,
			Payload:             map[string]any{"applicationId": appID.String()},
			IdempotencyKey:      &key,
			RelatedResourceType: strPtr("application"),
			RelatedResourceID:   &appID,
			RequestID:           strPtr(requestid.FromContext(ctx)),
		})
	}

	s.writeAudit(ctx, &app.OrganizationID, &actorID, "application.scale", "application", appID.String(), meta,
		map[string]any{"desiredReplicas": prev},
		map[string]any{"desiredReplicas": desired},
	)
	return s.GetSummary(ctx, actorID, appID)
}

func (s *Service) Report(ctx context.Context, appID uuid.UUID, in ReportInput) (Replica, error) {
	rep, err := s.repo.GetByIndex(ctx, appID, in.ReplicaIndex)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Replica{}, apierror.NotFound("replica not found")
		}
		return Replica{}, err
	}
	status := strings.TrimSpace(strings.ToUpper(in.Status))
	if status == "" {
		status = rep.Status
	}
	return s.repo.UpdateStatus(ctx, rep.ID, status, in.Healthy, in.RoutingEnabled, in.ContainerID, in.LastError)
}

func (s *Service) MarkUnhealthy(ctx context.Context, actorID, appID uuid.UUID, index int, meta AuditMeta) (Replica, error) {
	app, err := s.repo.GetApplicationMeta(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Replica{}, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return Replica{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, app.OrganizationID, rbac.ApplicationUpdate); err != nil {
		return Replica{}, err
	}
	rep, err := s.repo.GetByIndex(ctx, appID, index)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Replica{}, apierror.NotFound("replica not found")
		}
		return Replica{}, err
	}
	healthy := false
	routing := false
	msg := "marked unhealthy"
	updated, err := s.repo.UpdateStatus(ctx, rep.ID, StatusUnhealthy, &healthy, &routing, nil, &msg)
	if err != nil {
		return Replica{}, err
	}
	if s.queue != nil {
		org := app.OrganizationID
		key := "replicas-reconcile:" + appID.String() + ":unhealthy"
		_, _ = s.queue.Enqueue(ctx, jobs.EnqueueInput{
			OrganizationID:      &org,
			Type:                jobs.TypeReplicasReconcile,
			Payload:             map[string]any{"applicationId": appID.String(), "replaceIndex": index},
			IdempotencyKey:      &key,
			RelatedResourceType: strPtr("application"),
			RelatedResourceID:   &appID,
		})
	}
	s.writeAudit(ctx, &app.OrganizationID, &actorID, "application.replica.unhealthy", "application_replica", updated.ID.String(), meta,
		nil, map[string]any{"replicaIndex": index, "status": StatusUnhealthy},
	)
	return updated, nil
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

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
