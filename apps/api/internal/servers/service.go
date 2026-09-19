package servers

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/placement"
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

type Service struct {
	repo  Repository
	authz *rbac.Authorizer
	audit *audit.Writer
	log   *slog.Logger
	now   func() time.Time
}

func NewService(repo Repository, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger) *Service {
	return &Service{repo: repo, authz: authz, audit: auditWriter, log: log, now: time.Now}
}

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, in CreateInput, meta AuditMeta) (Server, error) {
	if err := s.authz.RequirePermission(ctx, actorID, in.OrganizationID, rbac.ServerCreate); err != nil {
		return Server{}, err
	}
	if err := validateCreate(in); err != nil {
		return Server{}, err
	}
	srv, err := s.repo.Create(ctx, in, actorID)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Server{}, apierror.Conflict("server name already exists in organization")
		}
		return Server{}, err
	}
	s.writeAudit(ctx, &in.OrganizationID, &actorID, "server.create", "server", srv.ID.String(), meta, nil, map[string]any{
		"name": srv.Name, "status": srv.Status,
	})
	return srv, nil
}

func (s *Service) List(ctx context.Context, actorID, orgID uuid.UUID, limit, offset int) ([]Server, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ServerRead); err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, orgID, limit, offset)
}

func (s *Service) Get(ctx context.Context, actorID, serverID uuid.UUID) (Server, error) {
	srv, err := s.repo.Get(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Server{}, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return Server{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, srv.OrganizationID, rbac.ServerRead); err != nil {
		return Server{}, err
	}
	return srv, nil
}

func (s *Service) Update(ctx context.Context, actorID, serverID uuid.UUID, in UpdateInput, meta AuditMeta) (Server, error) {
	before, err := s.repo.Get(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Server{}, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return Server{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.ServerUpdate); err != nil {
		return Server{}, err
	}
	if err := validateUpdate(in); err != nil {
		return Server{}, err
	}

	var status *string
	var maintenance *bool
	if in.Disabled != nil {
		if *in.Disabled {
			st := StatusDisabled
			status = &st
			f := false
			maintenance = &f
		} else if before.Status == StatusDisabled {
			// Re-enable: leave offline until agent heartbeat (B7) updates liveness.
			st := StatusOffline
			status = &st
		}
	}

	srv, err := s.repo.Update(ctx, serverID, in, status, maintenance)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Server{}, apierror.Conflict("server name already exists in organization")
		}
		if errors.Is(err, ErrNotFound) {
			return Server{}, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return Server{}, err
	}
	action := "server.update"
	if in.Disabled != nil {
		if *in.Disabled {
			action = "server.disable"
		} else if before.Status == StatusDisabled {
			action = "server.enable"
		}
	}
	s.writeAudit(ctx, &before.OrganizationID, &actorID, action, "server", serverID.String(), meta,
		map[string]any{"name": before.Name, "status": before.Status},
		map[string]any{"name": srv.Name, "status": srv.Status},
	)
	return srv, nil
}

func (s *Service) Delete(ctx context.Context, actorID, serverID uuid.UUID, meta AuditMeta) error {
	srv, err := s.repo.Get(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, srv.OrganizationID, rbac.ServerDelete); err != nil {
		return err
	}
	n, err := s.repo.CountActiveApplications(ctx, serverID)
	if err != nil {
		return err
	}
	if n > 0 {
		return apierror.Conflict("server has active applications targeting it; reassign or delete them first")
	}
	if err := s.repo.SoftDelete(ctx, serverID, s.now().UTC()); err != nil {
		return err
	}
	s.writeAudit(ctx, &srv.OrganizationID, &actorID, "server.delete", "server", serverID.String(), meta,
		map[string]any{"name": srv.Name, "status": srv.Status}, nil,
	)
	return nil
}

func (s *Service) EnterMaintenance(ctx context.Context, actorID, serverID uuid.UUID, meta AuditMeta) (Server, error) {
	before, err := s.repo.Get(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Server{}, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return Server{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.ServerUpdate); err != nil {
		return Server{}, err
	}
	if before.Status == StatusDisabled {
		return Server{}, apierror.Conflict("disabled servers cannot enter maintenance")
	}
	st := StatusMaintenance
	maint := true
	srv, err := s.repo.Update(ctx, serverID, UpdateInput{}, &st, &maint)
	if err != nil {
		return Server{}, err
	}
	s.writeAudit(ctx, &before.OrganizationID, &actorID, "server.maintenance.enter", "server", serverID.String(), meta,
		map[string]any{"status": before.Status, "maintenanceMode": before.MaintenanceMode},
		map[string]any{"status": srv.Status, "maintenanceMode": srv.MaintenanceMode},
	)
	return srv, nil
}

func (s *Service) ExitMaintenance(ctx context.Context, actorID, serverID uuid.UUID, meta AuditMeta) (Server, error) {
	before, err := s.repo.Get(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Server{}, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return Server{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.ServerUpdate); err != nil {
		return Server{}, err
	}
	if before.Status == StatusDisabled {
		return Server{}, apierror.Conflict("disabled servers cannot leave maintenance")
	}
	if !before.MaintenanceMode && before.Status != StatusMaintenance {
		return before, nil
	}
	// Do not invent ONLINE — wait for agent heartbeat (B7).
	st := StatusOffline
	maint := false
	srv, err := s.repo.Update(ctx, serverID, UpdateInput{}, &st, &maint)
	if err != nil {
		return Server{}, err
	}
	s.writeAudit(ctx, &before.OrganizationID, &actorID, "server.maintenance.exit", "server", serverID.String(), meta,
		map[string]any{"status": before.Status, "maintenanceMode": before.MaintenanceMode},
		map[string]any{"status": srv.Status, "maintenanceMode": srv.MaintenanceMode},
	)
	return srv, nil
}

func validateCreate(in CreateInput) error {
	var errs validation.Errors
	validation.RequiredString(&errs, "name", strings.TrimSpace(in.Name))
	validation.MaxLen(&errs, "name", in.Name, 120)
	validation.MaxLen(&errs, "provider", in.Provider, 120)
	validation.MaxLen(&errs, "region", in.Region, 120)
	validation.MaxLen(&errs, "hostname", in.Hostname, 253)
	validation.MaxLen(&errs, "architecture", in.Architecture, 64)
	validation.MaxLen(&errs, "operatingSystem", in.OperatingSystem, 120)
	if in.PublicIP != nil && strings.TrimSpace(*in.PublicIP) != "" {
		if _, err := netip.ParseAddr(strings.TrimSpace(*in.PublicIP)); err != nil {
			errs.Add("publicIp", "must be a valid IP address")
		}
	}
	if in.PrivateIP != nil && strings.TrimSpace(*in.PrivateIP) != "" {
		if _, err := netip.ParseAddr(strings.TrimSpace(*in.PrivateIP)); err != nil {
			errs.Add("privateIp", "must be a valid IP address")
		}
	}
	if in.CPUCores != nil && *in.CPUCores <= 0 {
		errs.Add("cpuCores", "must be greater than 0")
	}
	if in.MemoryBytes != nil && *in.MemoryBytes <= 0 {
		errs.Add("memoryBytes", "must be greater than 0")
	}
	if in.DiskBytes != nil && *in.DiskBytes <= 0 {
		errs.Add("diskBytes", "must be greater than 0")
	}
	if !errs.Empty() {
		return apierror.Validation("invalid server", errs.Details())
	}
	return nil
}

func validateUpdate(in UpdateInput) error {
	var errs validation.Errors
	if in.Name != nil {
		validation.RequiredString(&errs, "name", strings.TrimSpace(*in.Name))
		validation.MaxLen(&errs, "name", *in.Name, 120)
	}
	if in.Provider != nil {
		validation.MaxLen(&errs, "provider", *in.Provider, 120)
	}
	if in.Region != nil {
		validation.MaxLen(&errs, "region", *in.Region, 120)
	}
	if in.Hostname != nil {
		validation.MaxLen(&errs, "hostname", *in.Hostname, 253)
	}
	if in.PublicIP != nil && strings.TrimSpace(*in.PublicIP) != "" {
		if _, err := netip.ParseAddr(strings.TrimSpace(*in.PublicIP)); err != nil {
			errs.Add("publicIp", "must be a valid IP address")
		}
	}
	if in.PrivateIP != nil && strings.TrimSpace(*in.PrivateIP) != "" {
		if _, err := netip.ParseAddr(strings.TrimSpace(*in.PrivateIP)); err != nil {
			errs.Add("privateIp", "must be a valid IP address")
		}
	}
	if in.CPUCores != nil && *in.CPUCores <= 0 {
		errs.Add("cpuCores", "must be greater than 0")
	}
	if in.MemoryBytes != nil && *in.MemoryBytes <= 0 {
		errs.Add("memoryBytes", "must be greater than 0")
	}
	if in.DiskBytes != nil && *in.DiskBytes <= 0 {
		errs.Add("diskBytes", "must be greater than 0")
	}
	if !errs.Empty() {
		return apierror.Validation("invalid server", errs.Details())
	}
	return nil
}

func (s *Service) GetCapacity(ctx context.Context, actorID, serverID uuid.UUID) (CapacityView, error) {
	srv, err := s.repo.Get(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return CapacityView{}, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return CapacityView{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, srv.OrganizationID, rbac.ServerRead); err != nil {
		return CapacityView{}, err
	}
	refreshed, err := s.repo.RecomputeAllocated(ctx, serverID)
	if err != nil {
		return CapacityView{}, err
	}
	return toCapacityView(refreshed), nil
}

func (s *Service) ListCapacity(ctx context.Context, actorID, orgID uuid.UUID) ([]CapacityView, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ServerRead); err != nil {
		return nil, err
	}
	list, err := s.repo.ListActive(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]CapacityView, 0, len(list))
	for _, srv := range list {
		refreshed, err := s.repo.RecomputeAllocated(ctx, srv.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, toCapacityView(refreshed))
	}
	return out, nil
}

// RefreshAllocated recomputes denormalized counters for the given servers.
func (s *Service) RefreshAllocated(ctx context.Context, serverIDs ...uuid.UUID) error {
	seen := map[uuid.UUID]struct{}{}
	for _, id := range serverIDs {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if _, err := s.repo.RecomputeAllocated(ctx, id); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

// ValidateTargetCapacity checks whether a server can accept the given reservation.
// excludeAppID, when set, subtracts that application's current reservation from the server's allocated totals.
// Offline servers are allowed for inventory targeting; maintenance/disabled are not.
func (s *Service) ValidateTargetCapacity(ctx context.Context, orgID, serverID uuid.UUID, cpuMillis int, memBytes, diskBytes int64, excludeAppID *uuid.UUID) error {
	srv, err := s.repo.Get(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return err
	}
	if srv.OrganizationID != orgID {
		return apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found in organization")
	}
	if srv.MaintenanceMode || srv.Status == StatusMaintenance || srv.Status == StatusDisabled {
		return apierror.InsufficientResources("selected server is in maintenance or disabled", map[string]any{
			"serverId": serverID.String(),
		})
	}
	refreshed, err := s.repo.RecomputeAllocated(ctx, serverID)
	if err != nil {
		return err
	}
	c := toCandidate(refreshed)
	if excludeAppID != nil {
		pc, err := s.repo.GetPlacementContext(ctx, *excludeAppID)
		if err == nil && pc.TargetServerID != nil && *pc.TargetServerID == serverID {
			c.CPUAllocatedMillis = maxInt(0, c.CPUAllocatedMillis-pc.CPULimitMillis)
			c.MemoryAllocatedBytes = maxInt64(0, c.MemoryAllocatedBytes-pc.MemoryLimitBytes)
			c.DiskAllocatedBytes = maxInt64(0, c.DiskAllocatedBytes-pc.DiskBytes)
		}
	}
	if cpuMillis > 0 {
		if c.CPUTotalMillis <= 0 || c.CPUAllocatedMillis+cpuMillis > c.CPUTotalMillis {
			return apierror.InsufficientResources("insufficient CPU on selected server", map[string]any{
				"serverId": serverID.String(), "resource": "cpu",
			})
		}
	}
	if memBytes > 0 {
		if c.MemoryTotalBytes <= 0 || c.MemoryAllocatedBytes+memBytes > c.MemoryTotalBytes {
			return apierror.InsufficientResources("insufficient memory on selected server", map[string]any{
				"serverId": serverID.String(), "resource": "memory",
			})
		}
	}
	if diskBytes > 0 {
		if c.DiskTotalBytes <= 0 || c.DiskAllocatedBytes+diskBytes > c.DiskTotalBytes {
			return apierror.InsufficientResources("insufficient disk on selected server", map[string]any{
				"serverId": serverID.String(), "resource": "disk",
			})
		}
	}
	return nil
}

type PreviewPlacementInput struct {
	OrganizationID uuid.UUID
	CPUMillis      int
	MemoryBytes    int64
	DiskBytes      int64
	Policy         map[string]any
	ForcedServerID *uuid.UUID
}

type PreviewPlacementResult struct {
	ServerID uuid.UUID
	Score    float64
	Reason   string
}

func (s *Service) PreviewPlacement(ctx context.Context, actorID uuid.UUID, in PreviewPlacementInput) (PreviewPlacementResult, error) {
	if err := s.authz.RequirePermission(ctx, actorID, in.OrganizationID, rbac.ServerRead); err != nil {
		return PreviewPlacementResult{}, err
	}
	list, err := s.repo.ListActive(ctx, in.OrganizationID)
	if err != nil {
		return PreviewPlacementResult{}, err
	}
	cands := make([]placement.Candidate, 0, len(list))
	for _, srv := range list {
		refreshed, err := s.repo.RecomputeAllocated(ctx, srv.ID)
		if err != nil {
			return PreviewPlacementResult{}, err
		}
		cands = append(cands, toCandidate(refreshed))
	}
	req := placement.Request{
		CPUMillis:    in.CPUMillis,
		MemoryBytes:  in.MemoryBytes,
		DiskBytes:    in.DiskBytes,
		Policy:       placement.ParsePolicy(in.Policy),
		ForcedServer: in.ForcedServerID,
	}
	res, err := placement.Select(cands, req)
	if err != nil {
		if errors.Is(err, placement.ErrInsufficientResources) {
			return PreviewPlacementResult{}, apierror.InsufficientResources(err.Error(), nil)
		}
		return PreviewPlacementResult{}, err
	}
	return PreviewPlacementResult{ServerID: res.ServerID, Score: res.Score, Reason: res.Reason}, nil
}

// EnsureApplicationPlacement selects a server for the application (manual target or scheduler)
// and binds applications.target_server_id when needed.
func (s *Service) EnsureApplicationPlacement(ctx context.Context, appID uuid.UUID) (uuid.UUID, error) {
	pc, err := s.repo.GetPlacementContext(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return uuid.Nil, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return uuid.Nil, err
	}
	list, err := s.repo.ListActive(ctx, pc.OrganizationID)
	if err != nil {
		return uuid.Nil, err
	}
	cands := make([]placement.Candidate, 0, len(list))
	for _, srv := range list {
		refreshed, err := s.repo.RecomputeAllocated(ctx, srv.ID)
		if err != nil {
			return uuid.Nil, err
		}
		c := toCandidate(refreshed)
		if pc.TargetServerID != nil && c.ID == *pc.TargetServerID {
			c.CPUAllocatedMillis = maxInt(0, c.CPUAllocatedMillis-pc.CPULimitMillis)
			c.MemoryAllocatedBytes = maxInt64(0, c.MemoryAllocatedBytes-pc.MemoryLimitBytes)
			c.DiskAllocatedBytes = maxInt64(0, c.DiskAllocatedBytes-pc.DiskBytes)
		}
		cands = append(cands, c)
	}

	policy := placement.ParsePolicy(pc.PlacementPolicy)
	req := placement.Request{
		CPUMillis:    pc.CPULimitMillis,
		MemoryBytes:  pc.MemoryLimitBytes,
		DiskBytes:    pc.DiskBytes,
		Policy:       policy,
		ForcedServer: pc.TargetServerID,
	}
	res, err := placement.Select(cands, req)
	if err != nil {
		if errors.Is(err, placement.ErrInsufficientResources) {
			return uuid.Nil, apierror.InsufficientResources(err.Error(), map[string]any{
				"applicationId": appID.String(),
			})
		}
		return uuid.Nil, err
	}

	prev := pc.TargetServerID
	if prev == nil || *prev != res.ServerID {
		if err := s.repo.SetApplicationTarget(ctx, appID, res.ServerID); err != nil {
			return uuid.Nil, err
		}
		ids := []uuid.UUID{res.ServerID}
		if prev != nil {
			ids = append(ids, *prev)
		}
		if err := s.RefreshAllocated(ctx, ids...); err != nil {
			return uuid.Nil, err
		}
	}
	return res.ServerID, nil
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
