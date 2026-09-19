package domains

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/google/uuid"
)

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

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, in CreateInput, meta AuditMeta) (Domain, error) {
	app, err := s.repo.GetApplication(ctx, in.ApplicationID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Domain{}, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return Domain{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, app.OrganizationID, rbac.DomainManage); err != nil {
		return Domain{}, err
	}

	hostname, err := NormalizeHostname(in.Hostname)
	if err != nil {
		return Domain{}, apierror.Validation("invalid hostname", map[string]any{"hostname": err.Error()})
	}

	port := 0
	if in.InternalPort != nil {
		port = *in.InternalPort
	} else if app.InternalPort != nil {
		port = *app.InternalPort
	}
	if port <= 0 || port > 65535 {
		return Domain{}, apierror.Validation("internalPort is required (1-65535)", nil)
	}

	forceHTTPS := true
	if in.ForceHTTPS != nil {
		forceHTTPS = *in.ForceHTTPS
	}
	isPrimary := false
	if in.IsPrimary != nil {
		isPrimary = *in.IsPrimary
	} else {
		existing, err := s.repo.ListByApplication(ctx, app.ID)
		if err != nil {
			return Domain{}, err
		}
		isPrimary = len(existing) == 0
	}

	d, err := s.repo.Create(ctx, Domain{
		OrganizationID: app.OrganizationID,
		ApplicationID:  app.ID,
		EnvironmentID:  app.EnvironmentID,
		Hostname:       hostname,
		InternalPort:   port,
		IsPrimary:      isPrimary,
		ForceHTTPS:     forceHTTPS,
		DNSStatus:      DNSPending,
		TLSStatus:      TLSPending,
	})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Domain{}, apierror.New(409, apierror.CodeDomainAlreadyAssigned, "hostname already assigned in organization")
		}
		return Domain{}, err
	}
	d.Routing = BuildRoutingConfig(app.Slug, d)
	s.writeAudit(ctx, &app.OrganizationID, &actorID, "domain.create", "domain", d.ID.String(), meta, nil, map[string]any{
		"hostname": d.Hostname, "applicationId": app.ID.String(), "isPrimary": d.IsPrimary,
	})
	return d, nil
}

func (s *Service) ListByApplication(ctx context.Context, actorID, appID uuid.UUID) ([]Domain, error) {
	app, err := s.repo.GetApplication(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return nil, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, app.OrganizationID, rbac.DomainRead); err != nil {
		return nil, err
	}
	items, err := s.repo.ListByApplication(ctx, appID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Routing = BuildRoutingConfig(app.Slug, items[i])
	}
	return items, nil
}

func (s *Service) Get(ctx context.Context, actorID, id uuid.UUID) (Domain, error) {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Domain{}, apierror.NotFound("domain not found")
		}
		return Domain{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, d.OrganizationID, rbac.DomainRead); err != nil {
		return Domain{}, err
	}
	app, err := s.repo.GetApplication(ctx, d.ApplicationID)
	if err != nil {
		return Domain{}, err
	}
	d.Routing = BuildRoutingConfig(app.Slug, d)
	return d, nil
}

func (s *Service) Update(ctx context.Context, actorID, id uuid.UUID, in UpdateInput, meta AuditMeta) (Domain, error) {
	before, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Domain{}, apierror.NotFound("domain not found")
		}
		return Domain{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.DomainManage); err != nil {
		return Domain{}, err
	}

	after := before
	if in.Hostname != nil {
		h, err := NormalizeHostname(*in.Hostname)
		if err != nil {
			return Domain{}, apierror.Validation("invalid hostname", map[string]any{"hostname": err.Error()})
		}
		after.Hostname = h
		if h != before.Hostname {
			after.DNSStatus = DNSPending
			after.TLSStatus = TLSPending
		}
	}
	if in.InternalPort != nil {
		if *in.InternalPort <= 0 || *in.InternalPort > 65535 {
			return Domain{}, apierror.Validation("internalPort must be between 1 and 65535", nil)
		}
		after.InternalPort = *in.InternalPort
	}
	if in.IsPrimary != nil {
		after.IsPrimary = *in.IsPrimary
	}
	if in.ForceHTTPS != nil {
		after.ForceHTTPS = *in.ForceHTTPS
	}
	if in.DNSStatus != nil {
		st := *in.DNSStatus
		switch st {
		case DNSPending, DNSValid, DNSInvalid:
			after.DNSStatus = st
		default:
			return Domain{}, apierror.Validation("invalid dnsStatus", nil)
		}
	}
	if in.TLSStatus != nil {
		st := *in.TLSStatus
		switch st {
		case TLSPending, TLSIssuing, TLSActive, TLSExpiring, TLSFailed:
			after.TLSStatus = st
		default:
			return Domain{}, apierror.Validation("invalid tlsStatus", nil)
		}
	}

	updated, err := s.repo.Update(ctx, after)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Domain{}, apierror.New(409, apierror.CodeDomainAlreadyAssigned, "hostname already assigned in organization")
		}
		return Domain{}, err
	}
	app, err := s.repo.GetApplication(ctx, updated.ApplicationID)
	if err != nil {
		return Domain{}, err
	}
	updated.Routing = BuildRoutingConfig(app.Slug, updated)
	s.writeAudit(ctx, &before.OrganizationID, &actorID, "domain.update", "domain", id.String(), meta,
		map[string]any{"hostname": before.Hostname, "isPrimary": before.IsPrimary, "dnsStatus": before.DNSStatus, "tlsStatus": before.TLSStatus},
		map[string]any{"hostname": updated.Hostname, "isPrimary": updated.IsPrimary, "dnsStatus": updated.DNSStatus, "tlsStatus": updated.TLSStatus},
	)
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) error {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFound("domain not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, d.OrganizationID, rbac.DomainManage); err != nil {
		return err
	}
	if err := s.repo.SoftDelete(ctx, id, s.now().UTC()); err != nil {
		return err
	}
	s.writeAudit(ctx, &d.OrganizationID, &actorID, "domain.delete", "domain", id.String(), meta,
		map[string]any{"hostname": d.Hostname, "applicationId": d.ApplicationID.String()}, nil,
	)
	return nil
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
