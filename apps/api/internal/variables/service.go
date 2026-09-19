package variables

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"

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

type Service struct {
	repo  Repository
	authz *rbac.Authorizer
	audit *audit.Writer
	log   *slog.Logger
}

func NewService(repo Repository, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger) *Service {
	return &Service{repo: repo, authz: authz, audit: auditWriter, log: log}
}

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, in CreateInput, meta AuditMeta) (Variable, error) {
	normalized, err := s.normalizeScope(ctx, in)
	if err != nil {
		return Variable{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, normalized.OrganizationID, rbac.ApplicationUpdate); err != nil {
		return Variable{}, err
	}
	if err := validateKeyValue(normalized.Key, normalized.Value); err != nil {
		return Variable{}, err
	}
	v, err := s.repo.Create(ctx, normalized, actorID)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Variable{}, apierror.Conflict("variable key already exists in scope")
		}
		return Variable{}, err
	}
	s.writeAudit(ctx, &v.OrganizationID, &actorID, "variable.create", "variable", v.ID.String(), meta, nil, map[string]any{
		"key": v.Key, "scope": v.Scope,
	})
	return v, nil
}

func (s *Service) List(ctx context.Context, actorID, orgID uuid.UUID, scope *string, projectID, environmentID, applicationID *uuid.UUID, limit, offset int) ([]Variable, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ApplicationRead); err != nil {
		return nil, 0, err
	}
	if scope != nil {
		sc := strings.ToUpper(strings.TrimSpace(*scope))
		if !validScope(sc) {
			return nil, 0, apierror.Validation("invalid scope", nil)
		}
		scope = &sc
	}
	return s.repo.List(ctx, orgID, scope, projectID, environmentID, applicationID, limit, offset)
}

func (s *Service) Get(ctx context.Context, actorID, id uuid.UUID) (Variable, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Variable{}, apierror.NotFound("variable not found")
		}
		return Variable{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, v.OrganizationID, rbac.ApplicationRead); err != nil {
		return Variable{}, err
	}
	return v, nil
}

func (s *Service) Update(ctx context.Context, actorID, id uuid.UUID, in UpdateInput, meta AuditMeta) (Variable, error) {
	before, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Variable{}, apierror.NotFound("variable not found")
		}
		return Variable{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.ApplicationUpdate); err != nil {
		return Variable{}, err
	}
	key, value := before.Key, before.Value
	if in.Key != nil {
		key = strings.TrimSpace(*in.Key)
		in.Key = &key
	}
	if in.Value != nil {
		value = *in.Value
	}
	if err := validateKeyValue(key, value); err != nil {
		return Variable{}, err
	}
	v, err := s.repo.Update(ctx, id, in, actorID)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Variable{}, apierror.Conflict("variable key already exists in scope")
		}
		return Variable{}, err
	}
	s.writeAudit(ctx, &v.OrganizationID, &actorID, "variable.update", "variable", v.ID.String(), meta,
		map[string]any{"key": before.Key, "scope": before.Scope},
		map[string]any{"key": v.Key, "scope": v.Scope},
	)
	return v, nil
}

func (s *Service) Delete(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) error {
	before, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFound("variable not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.ApplicationUpdate); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.writeAudit(ctx, &before.OrganizationID, &actorID, "variable.delete", "variable", id.String(), meta,
		map[string]any{"key": before.Key, "scope": before.Scope}, nil,
	)
	return nil
}

func (s *Service) Resolve(ctx context.Context, actorID, orgID uuid.UUID, projectID, environmentID, applicationID *uuid.UUID) ([]ResolvedEntry, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ApplicationRead); err != nil {
		return nil, err
	}
	resolvedOrg := orgID
	var err error
	if applicationID != nil {
		var pID, eID uuid.UUID
		resolvedOrg, pID, eID, err = s.repo.ResolveApplication(ctx, *applicationID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
			}
			return nil, err
		}
		projectID, environmentID = &pID, &eID
	} else if environmentID != nil {
		var pID uuid.UUID
		resolvedOrg, pID, err = s.repo.ResolveEnvironment(ctx, *environmentID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, apierror.NotFoundCode(apierror.CodeEnvironmentNotFound, "environment not found")
			}
			return nil, err
		}
		projectID = &pID
	} else if projectID != nil {
		resolvedOrg, err = s.repo.ResolveProject(ctx, *projectID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, apierror.NotFoundCode(apierror.CodeProjectNotFound, "project not found")
			}
			return nil, err
		}
	}
	if resolvedOrg != orgID {
		return nil, apierror.Validation("organizationId does not match resource", nil)
	}
	items, err := s.repo.ListForResolve(ctx, orgID, projectID, environmentID, applicationID)
	if err != nil {
		return nil, err
	}
	return mergeVariables(items), nil
}

func mergeVariables(items []Variable) []ResolvedEntry {
	type agg struct {
		scopes map[string]struct{}
		last   Variable
	}
	order := make([]string, 0)
	byKey := map[string]*agg{}
	for _, v := range items {
		a, ok := byKey[v.Key]
		if !ok {
			a = &agg{scopes: map[string]struct{}{}}
			byKey[v.Key] = a
			order = append(order, v.Key)
		}
		a.scopes[v.Scope] = struct{}{}
		a.last = v
	}
	out := make([]ResolvedEntry, 0, len(order))
	for _, key := range order {
		a := byKey[key]
		v := a.last
		out = append(out, ResolvedEntry{
			Key:            v.Key,
			Value:          v.Value,
			Scope:          v.Scope,
			SourceID:       v.ID,
			Overridden:     len(a.scopes) > 1,
			OrganizationID: v.OrganizationID,
			ProjectID:      v.ProjectID,
			EnvironmentID:  v.EnvironmentID,
			ApplicationID:  v.ApplicationID,
		})
	}
	return out
}

func (s *Service) normalizeScope(ctx context.Context, in CreateInput) (CreateInput, error) {
	in.Scope = strings.ToUpper(strings.TrimSpace(in.Scope))
	in.Key = strings.TrimSpace(in.Key)
	if !validScope(in.Scope) {
		return CreateInput{}, apierror.Validation("invalid scope", map[string]any{"scope": "must be ORGANIZATION, PROJECT, ENVIRONMENT, or APPLICATION"})
	}
	switch in.Scope {
	case ScopeOrganization:
		ok, err := s.repo.OrgExists(ctx, in.OrganizationID)
		if err != nil {
			return CreateInput{}, err
		}
		if !ok {
			return CreateInput{}, apierror.NotFoundCode(apierror.CodeOrganizationNotFound, "organization not found")
		}
		in.ProjectID, in.EnvironmentID, in.ApplicationID = nil, nil, nil
	case ScopeProject:
		if in.ProjectID == nil {
			return CreateInput{}, apierror.Validation("projectId is required", nil)
		}
		orgID, err := s.repo.ResolveProject(ctx, *in.ProjectID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return CreateInput{}, apierror.NotFoundCode(apierror.CodeProjectNotFound, "project not found")
			}
			return CreateInput{}, err
		}
		if in.OrganizationID != uuid.Nil && in.OrganizationID != orgID {
			return CreateInput{}, apierror.Validation("organizationId does not match project", nil)
		}
		in.OrganizationID = orgID
		in.EnvironmentID, in.ApplicationID = nil, nil
	case ScopeEnvironment:
		if in.EnvironmentID == nil {
			return CreateInput{}, apierror.Validation("environmentId is required", nil)
		}
		orgID, projectID, err := s.repo.ResolveEnvironment(ctx, *in.EnvironmentID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return CreateInput{}, apierror.NotFoundCode(apierror.CodeEnvironmentNotFound, "environment not found")
			}
			return CreateInput{}, err
		}
		if in.OrganizationID != uuid.Nil && in.OrganizationID != orgID {
			return CreateInput{}, apierror.Validation("organizationId does not match environment", nil)
		}
		in.OrganizationID = orgID
		in.ProjectID = &projectID
		in.ApplicationID = nil
	case ScopeApplication:
		if in.ApplicationID == nil {
			return CreateInput{}, apierror.Validation("applicationId is required", nil)
		}
		orgID, projectID, environmentID, err := s.repo.ResolveApplication(ctx, *in.ApplicationID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return CreateInput{}, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
			}
			return CreateInput{}, err
		}
		if in.OrganizationID != uuid.Nil && in.OrganizationID != orgID {
			return CreateInput{}, apierror.Validation("organizationId does not match application", nil)
		}
		in.OrganizationID = orgID
		in.ProjectID = &projectID
		in.EnvironmentID = &environmentID
	}
	return in, nil
}

var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateKeyValue(key, value string) error {
	var errs validation.Errors
	validation.RequiredString(&errs, "key", key)
	if key != "" && !keyPattern.MatchString(key) {
		errs.Add("key", "must match [A-Za-z_][A-Za-z0-9_]*")
	}
	validation.MaxLen(&errs, "key", key, 256)
	validation.MaxLen(&errs, "value", value, 8192)
	if !errs.Empty() {
		return apierror.Validation("invalid variable", errs.Details())
	}
	return nil
}

func validScope(s string) bool {
	switch s {
	case ScopeOrganization, ScopeProject, ScopeEnvironment, ScopeApplication:
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
