package projects

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"

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
	now   func() time.Time
}

func NewService(repo Repository, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger) *Service {
	return &Service{repo: repo, authz: authz, audit: auditWriter, log: log, now: time.Now}
}

func (s *Service) CreateProject(ctx context.Context, actorID, orgID uuid.UUID, name, slug, description string, meta AuditMeta) (Project, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ProjectCreate); err != nil {
		return Project{}, err
	}
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	var errs validation.Errors
	validation.RequiredString(&errs, "name", name)
	validation.MaxLen(&errs, "name", name, 120)
	validation.MaxLen(&errs, "description", description, 2000)
	slug = strings.TrimSpace(slug)
	if slug == "" {
		slug = slugify(name)
	}
	if err := validateSlug(slug); err != nil {
		errs.Add("slug", err.Error())
	}
	if !errs.Empty() {
		return Project{}, apierror.Validation("invalid project", errs.Details())
	}

	p, err := s.repo.CreateProject(ctx, orgID, name, slug, description, actorID)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Project{}, apierror.Conflict("project slug already exists in organization")
		}
		return Project{}, err
	}
	s.writeAudit(ctx, &orgID, &actorID, "project.create", "project", p.ID.String(), meta, nil, map[string]any{
		"name": p.Name, "slug": p.Slug,
	})
	return p, nil
}

func (s *Service) ListProjects(ctx context.Context, actorID, orgID uuid.UUID, limit, offset int) ([]Project, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ProjectRead); err != nil {
		return nil, 0, err
	}
	return s.repo.ListProjects(ctx, orgID, limit, offset)
}

func (s *Service) GetProject(ctx context.Context, actorID, projectID uuid.UUID) (Project, error) {
	p, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Project{}, apierror.NotFoundCode(apierror.CodeProjectNotFound, "project not found")
		}
		return Project{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, p.OrganizationID, rbac.ProjectRead); err != nil {
		return Project{}, err
	}
	return p, nil
}

func (s *Service) UpdateProject(ctx context.Context, actorID, projectID uuid.UUID, name, slug, description *string, meta AuditMeta) (Project, error) {
	before, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Project{}, apierror.NotFoundCode(apierror.CodeProjectNotFound, "project not found")
		}
		return Project{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.ProjectUpdate); err != nil {
		return Project{}, err
	}

	var errs validation.Errors
	var namePtr, slugPtr, descPtr *string
	if name != nil {
		n := strings.TrimSpace(*name)
		validation.RequiredString(&errs, "name", n)
		validation.MaxLen(&errs, "name", n, 120)
		namePtr = &n
	}
	if slug != nil {
		sl := strings.TrimSpace(*slug)
		if err := validateSlug(sl); err != nil {
			errs.Add("slug", err.Error())
		}
		slugPtr = &sl
	}
	if description != nil {
		d := strings.TrimSpace(*description)
		validation.MaxLen(&errs, "description", d, 2000)
		descPtr = &d
	}
	if !errs.Empty() {
		return Project{}, apierror.Validation("invalid project", errs.Details())
	}

	p, err := s.repo.UpdateProject(ctx, projectID, namePtr, slugPtr, descPtr)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Project{}, apierror.Conflict("project slug already exists in organization")
		}
		if errors.Is(err, ErrNotFound) {
			return Project{}, apierror.NotFoundCode(apierror.CodeProjectNotFound, "project not found")
		}
		return Project{}, err
	}
	s.writeAudit(ctx, &before.OrganizationID, &actorID, "project.update", "project", projectID.String(), meta,
		map[string]any{"name": before.Name, "slug": before.Slug},
		map[string]any{"name": p.Name, "slug": p.Slug},
	)
	return p, nil
}

func (s *Service) DeleteProject(ctx context.Context, actorID, projectID uuid.UUID, meta AuditMeta) error {
	p, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFoundCode(apierror.CodeProjectNotFound, "project not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, p.OrganizationID, rbac.ProjectDelete); err != nil {
		return err
	}
	n, err := s.repo.CountActiveApplicationsForProject(ctx, projectID)
	if err != nil {
		return err
	}
	if n > 0 {
		return apierror.Conflict("project has active applications; delete or move them first")
	}
	at := s.now().UTC()
	if err := s.repo.SoftDeleteEnvironmentsForProject(ctx, projectID, at); err != nil {
		return err
	}
	if err := s.repo.SoftDeleteProject(ctx, projectID, at); err != nil {
		return err
	}
	s.writeAudit(ctx, &p.OrganizationID, &actorID, "project.delete", "project", projectID.String(), meta,
		map[string]any{"name": p.Name, "slug": p.Slug}, nil,
	)
	return nil
}

func (s *Service) CreateEnvironment(ctx context.Context, actorID, projectID uuid.UUID, name, slug, kind string, meta AuditMeta) (Environment, error) {
	p, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Environment{}, apierror.NotFoundCode(apierror.CodeProjectNotFound, "project not found")
		}
		return Environment{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, p.OrganizationID, rbac.ProjectUpdate); err != nil {
		return Environment{}, err
	}

	name = strings.TrimSpace(name)
	var errs validation.Errors
	validation.RequiredString(&errs, "name", name)
	validation.MaxLen(&errs, "name", name, 120)
	slug = strings.TrimSpace(slug)
	if slug == "" {
		slug = slugify(name)
	}
	if err := validateSlug(slug); err != nil {
		errs.Add("slug", err.Error())
	}
	kind = strings.TrimSpace(strings.ToLower(kind))
	if kind == "" {
		kind = KindCustom
	}
	if !validKind(kind) {
		errs.Add("kind", "must be one of production, staging, development, preview, custom")
	}
	if !errs.Empty() {
		return Environment{}, apierror.Validation("invalid environment", errs.Details())
	}

	env, err := s.repo.CreateEnvironment(ctx, p.OrganizationID, projectID, name, slug, kind)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Environment{}, apierror.Conflict("environment slug already exists in project")
		}
		return Environment{}, err
	}
	s.writeAudit(ctx, &p.OrganizationID, &actorID, "environment.create", "environment", env.ID.String(), meta, nil, map[string]any{
		"name": env.Name, "slug": env.Slug, "kind": env.Kind, "projectId": projectID.String(),
	})
	return env, nil
}

func (s *Service) ListEnvironments(ctx context.Context, actorID, projectID uuid.UUID, limit, offset int) ([]Environment, int64, error) {
	p, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, 0, apierror.NotFoundCode(apierror.CodeProjectNotFound, "project not found")
		}
		return nil, 0, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, p.OrganizationID, rbac.ProjectRead); err != nil {
		return nil, 0, err
	}
	return s.repo.ListEnvironments(ctx, projectID, limit, offset)
}

func (s *Service) GetEnvironment(ctx context.Context, actorID, environmentID uuid.UUID) (Environment, error) {
	env, err := s.repo.GetEnvironment(ctx, environmentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Environment{}, apierror.NotFoundCode(apierror.CodeEnvironmentNotFound, "environment not found")
		}
		return Environment{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, env.OrganizationID, rbac.ProjectRead); err != nil {
		return Environment{}, err
	}
	return env, nil
}

func (s *Service) UpdateEnvironment(ctx context.Context, actorID, environmentID uuid.UUID, name, slug, kind *string, meta AuditMeta) (Environment, error) {
	before, err := s.repo.GetEnvironment(ctx, environmentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Environment{}, apierror.NotFoundCode(apierror.CodeEnvironmentNotFound, "environment not found")
		}
		return Environment{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.ProjectUpdate); err != nil {
		return Environment{}, err
	}

	var errs validation.Errors
	var namePtr, slugPtr, kindPtr *string
	if name != nil {
		n := strings.TrimSpace(*name)
		validation.RequiredString(&errs, "name", n)
		validation.MaxLen(&errs, "name", n, 120)
		namePtr = &n
	}
	if slug != nil {
		sl := strings.TrimSpace(*slug)
		if err := validateSlug(sl); err != nil {
			errs.Add("slug", err.Error())
		}
		slugPtr = &sl
	}
	if kind != nil {
		k := strings.TrimSpace(strings.ToLower(*kind))
		if !validKind(k) {
			errs.Add("kind", "must be one of production, staging, development, preview, custom")
		}
		kindPtr = &k
	}
	if !errs.Empty() {
		return Environment{}, apierror.Validation("invalid environment", errs.Details())
	}

	env, err := s.repo.UpdateEnvironment(ctx, environmentID, namePtr, slugPtr, kindPtr)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Environment{}, apierror.Conflict("environment slug already exists in project")
		}
		if errors.Is(err, ErrNotFound) {
			return Environment{}, apierror.NotFoundCode(apierror.CodeEnvironmentNotFound, "environment not found")
		}
		return Environment{}, err
	}
	s.writeAudit(ctx, &before.OrganizationID, &actorID, "environment.update", "environment", environmentID.String(), meta,
		map[string]any{"name": before.Name, "slug": before.Slug, "kind": before.Kind},
		map[string]any{"name": env.Name, "slug": env.Slug, "kind": env.Kind},
	)
	return env, nil
}

func (s *Service) DeleteEnvironment(ctx context.Context, actorID, environmentID uuid.UUID, meta AuditMeta) error {
	env, err := s.repo.GetEnvironment(ctx, environmentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFoundCode(apierror.CodeEnvironmentNotFound, "environment not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, env.OrganizationID, rbac.ProjectDelete); err != nil {
		return err
	}
	n, err := s.repo.CountActiveApplicationsForEnvironment(ctx, environmentID)
	if err != nil {
		return err
	}
	if n > 0 {
		return apierror.Conflict("environment has active applications; delete or move them first")
	}
	if err := s.repo.SoftDeleteEnvironment(ctx, environmentID, s.now().UTC()); err != nil {
		return err
	}
	s.writeAudit(ctx, &env.OrganizationID, &actorID, "environment.delete", "environment", environmentID.String(), meta,
		map[string]any{"name": env.Name, "slug": env.Slug, "kind": env.Kind}, nil,
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

func validKind(kind string) bool {
	switch kind {
	case KindProduction, KindStaging, KindDevelopment, KindPreview, KindCustom:
		return true
	default:
		return false
	}
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func validateSlug(slug string) error {
	if slug == "" {
		return errors.New("is required")
	}
	if !slugPattern.MatchString(slug) {
		return errors.New("must be lowercase alphanumeric with optional hyphens")
	}
	return nil
}

func slugify(name string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if (r == ' ' || r == '-' || r == '_') && !lastHyphen && b.Len() > 0 {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "project"
	}
	if len(out) > 63 {
		out = strings.Trim(out[:63], "-")
	}
	return out
}
