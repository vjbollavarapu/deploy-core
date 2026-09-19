package revisions

import (
	"context"
	"errors"
	"strings"

	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/google/uuid"
)

type Service struct {
	repo  Repository
	authz *rbac.Authorizer
}

func NewService(repo Repository, authz *rbac.Authorizer) *Service {
	return &Service{repo: repo, authz: authz}
}

func (s *Service) ListByApplication(ctx context.Context, actorID, applicationID uuid.UUID, status *string, limit, offset int) ([]Revision, int64, error) {
	orgID, err := s.repo.GetApplicationOrg(ctx, applicationID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, 0, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return nil, 0, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ApplicationRead); err != nil {
		return nil, 0, err
	}
	if status != nil {
		st := strings.ToUpper(strings.TrimSpace(*status))
		if st == "" {
			status = nil
		} else if !validStatus(st) {
			return nil, 0, apierror.Validation("invalid status", map[string]any{
				"status": "must be CREATED, READY, ACTIVE, INACTIVE, FAILED, or ARCHIVED",
			})
		} else {
			status = &st
		}
	}
	return s.repo.ListByApplication(ctx, applicationID, status, limit, offset)
}

func (s *Service) Get(ctx context.Context, actorID, revisionID uuid.UUID) (Revision, error) {
	rev, err := s.repo.Get(ctx, revisionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Revision{}, apierror.NotFoundCode(apierror.CodeRevisionNotFound, "revision not found")
		}
		return Revision{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, rev.OrganizationID, rbac.ApplicationRead); err != nil {
		return Revision{}, err
	}
	return rev, nil
}

func validStatus(s string) bool {
	switch s {
	case StatusCreated, StatusReady, StatusActive, StatusInactive, StatusFailed, StatusArchived:
		return true
	default:
		return false
	}
}
