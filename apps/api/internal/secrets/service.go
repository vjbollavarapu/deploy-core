package secrets

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/deploycore/deploy-core/apps/api/pkg/validation"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditMeta struct {
	IP        string
	UserAgent string
}

type ServiceConfig struct {
	PlatformKey []byte
	KeyID       string
}

type Service struct {
	repo  Repository
	authz *rbac.Authorizer
	audit *audit.Writer
	log   *slog.Logger
	cfg   ServiceConfig
	now   func() time.Time
	pool  *pgxpool.Pool // optional; used for rotate transaction
}

func NewService(repo Repository, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger, cfg ServiceConfig) *Service {
	if cfg.KeyID == "" {
		cfg.KeyID = "platform:v1"
	}
	return &Service{repo: repo, authz: authz, audit: auditWriter, log: log, cfg: cfg, now: time.Now}
}

// WithPool enables transactional rotate (soft-delete + insert).
func (s *Service) WithPool(pool *pgxpool.Pool) *Service {
	s.pool = pool
	return s
}

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, in CreateInput, meta AuditMeta) (Metadata, error) {
	normalized, err := s.normalizeScope(ctx, in)
	if err != nil {
		return Metadata{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, normalized.OrganizationID, rbac.SecretCreate); err != nil {
		return Metadata{}, err
	}
	if err := validateNameValue(normalized.Name, normalized.Value); err != nil {
		return Metadata{}, err
	}
	env, err := crypto.Seal(s.cfg.PlatformKey, s.cfg.KeyID, []byte(normalized.Value))
	if err != nil {
		return Metadata{}, err
	}
	m, err := s.repo.Create(ctx, Metadata{
		OrganizationID: normalized.OrganizationID,
		Scope:          normalized.Scope,
		ProjectID:      normalized.ProjectID,
		EnvironmentID:  normalized.EnvironmentID,
		ApplicationID:  normalized.ApplicationID,
		Name:           normalized.Name,
		Version:        1,
		KeyID:          env.KeyID,
		Algorithm:      "AES-256-GCM",
	}, env.Ciphertext, env.Nonce, actorID)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Metadata{}, apierror.Conflict("secret name already exists in scope")
		}
		return Metadata{}, err
	}
	s.writeAudit(ctx, &m.OrganizationID, &actorID, "secret.create", "secret", m.ID.String(), meta, nil, map[string]any{
		"name": m.Name, "scope": m.Scope, "version": m.Version,
	})
	return m, nil
}

func (s *Service) List(ctx context.Context, actorID, orgID uuid.UUID, scope *string, projectID, environmentID, applicationID *uuid.UUID, limit, offset int) ([]Metadata, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.SecretReadMetadata); err != nil {
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

func (s *Service) Get(ctx context.Context, actorID, id uuid.UUID) (Metadata, error) {
	rec, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Metadata{}, apierror.NotFoundCode(apierror.CodeSecretNotFound, "secret not found")
		}
		return Metadata{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, rec.OrganizationID, rbac.SecretReadMetadata); err != nil {
		return Metadata{}, err
	}
	return rec.Metadata, nil
}

// Rotate replaces the active secret value with a new version (soft-deletes prior row).
func (s *Service) Rotate(ctx context.Context, actorID, id uuid.UUID, value string, meta AuditMeta) (Metadata, error) {
	before, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Metadata{}, apierror.NotFoundCode(apierror.CodeSecretNotFound, "secret not found")
		}
		return Metadata{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.SecretUpdate); err != nil {
		return Metadata{}, err
	}
	if err := validateNameValue(before.Name, value); err != nil {
		return Metadata{}, err
	}
	env, err := crypto.Seal(s.cfg.PlatformKey, s.cfg.KeyID, []byte(value))
	if err != nil {
		return Metadata{}, err
	}

	next := Metadata{
		OrganizationID: before.OrganizationID,
		Scope:          before.Scope,
		ProjectID:      before.ProjectID,
		EnvironmentID:  before.EnvironmentID,
		ApplicationID:  before.ApplicationID,
		Name:           before.Name,
		Version:        before.Version + 1,
		KeyID:          env.KeyID,
		Algorithm:      "AES-256-GCM",
	}

	if s.pool != nil {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return Metadata{}, err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		ct, err := tx.Exec(ctx, `
			UPDATE secrets SET deleted_at = $2 WHERE id = $1 AND deleted_at IS NULL`, before.ID, s.now().UTC())
		if err != nil {
			return Metadata{}, err
		}
		if ct.RowsAffected() == 0 {
			return Metadata{}, apierror.NotFoundCode(apierror.CodeSecretNotFound, "secret not found")
		}
		const q = `
			INSERT INTO secrets (
				organization_id, scope, project_id, environment_id, application_id,
				name, version, ciphertext, nonce, key_id, algorithm, created_by, updated_by
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)
			RETURNING id, organization_id, scope, project_id, environment_id, application_id,
			          name, version, key_id, algorithm, created_by, updated_by, created_at, updated_at`
		m, err := scanMeta(tx.QueryRow(ctx, q,
			next.OrganizationID, next.Scope, next.ProjectID, next.EnvironmentID, next.ApplicationID,
			next.Name, next.Version, env.Ciphertext, env.Nonce, next.KeyID, next.Algorithm, actorID,
		))
		if err != nil {
			if isUniqueViolation(err) {
				return Metadata{}, apierror.Conflict("secret name already exists in scope")
			}
			return Metadata{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Metadata{}, err
		}
		s.writeAudit(ctx, &m.OrganizationID, &actorID, "secret.rotate", "secret", m.ID.String(), meta,
			map[string]any{"name": before.Name, "scope": before.Scope, "version": before.Version},
			map[string]any{"name": m.Name, "scope": m.Scope, "version": m.Version},
		)
		return m, nil
	}

	if err := s.repo.SoftDelete(ctx, before.ID, s.now().UTC()); err != nil {
		return Metadata{}, err
	}
	m, err := s.repo.Create(ctx, next, env.Ciphertext, env.Nonce, actorID)
	if err != nil {
		return Metadata{}, err
	}
	s.writeAudit(ctx, &m.OrganizationID, &actorID, "secret.rotate", "secret", m.ID.String(), meta,
		map[string]any{"name": before.Name, "scope": before.Scope, "version": before.Version},
		map[string]any{"name": m.Name, "scope": m.Scope, "version": m.Version},
	)
	return m, nil
}

func (s *Service) Delete(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) error {
	before, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFoundCode(apierror.CodeSecretNotFound, "secret not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.SecretDelete); err != nil {
		return err
	}
	if err := s.repo.SoftDelete(ctx, id, s.now().UTC()); err != nil {
		return err
	}
	s.writeAudit(ctx, &before.OrganizationID, &actorID, "secret.delete", "secret", id.String(), meta,
		map[string]any{"name": before.Name, "scope": before.Scope, "version": before.Version}, nil,
	)
	return nil
}

// DecryptPlaintext is for internal deployment use only — never exposed via HTTP in B10.
func (s *Service) DecryptPlaintext(ctx context.Context, id uuid.UUID) ([]byte, error) {
	rec, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return crypto.Open(s.cfg.PlatformKey, crypto.Envelope{
		KeyID:      rec.KeyID,
		Nonce:      rec.Nonce,
		Ciphertext: rec.Ciphertext,
	})
}

func (s *Service) normalizeScope(ctx context.Context, in CreateInput) (CreateInput, error) {
	in.Scope = strings.ToUpper(strings.TrimSpace(in.Scope))
	in.Name = strings.TrimSpace(in.Name)
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

var namePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_./-]*$`)

func validateNameValue(name, value string) error {
	var errs validation.Errors
	validation.RequiredString(&errs, "name", name)
	if name != "" && !namePattern.MatchString(name) {
		errs.Add("name", "must be a valid secret name")
	}
	validation.MaxLen(&errs, "name", name, 256)
	validation.RequiredString(&errs, "value", value)
	validation.MaxLen(&errs, "value", value, 65536)
	if !errs.Empty() {
		return apierror.Validation("invalid secret", errs.Details())
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
