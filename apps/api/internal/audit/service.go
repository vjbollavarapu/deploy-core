package audit

import (
	"context"
	"errors"
	"log/slog"

	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/google/uuid"
)

// Service exposes read-only audit access. Mutations are intentionally unsupported.
type Service struct {
	repo  Repository
	authz *rbac.Authorizer
	log   *slog.Logger
}

func NewService(repo Repository, authz *rbac.Authorizer, log *slog.Logger) *Service {
	return &Service{repo: repo, authz: authz, log: log}
}

func (s *Service) List(ctx context.Context, actorID uuid.UUID, f ListFilter) ([]Record, int64, error) {
	if f.OrganizationID == uuid.Nil {
		return nil, 0, apierror.Validation("organizationId is required", nil)
	}
	if err := s.authz.RequirePermission(ctx, actorID, f.OrganizationID, rbac.AuditRead); err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, f)
}

func (s *Service) Get(ctx context.Context, actorID, id uuid.UUID) (Record, error) {
	rec, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Record{}, apierror.NotFound("audit event not found")
		}
		return Record{}, err
	}
	if rec.OrganizationID == nil {
		return Record{}, apierror.NotFound("audit event not found")
	}
	if err := s.authz.RequirePermission(ctx, actorID, *rec.OrganizationID, rbac.AuditRead); err != nil {
		return Record{}, err
	}
	return rec, nil
}
