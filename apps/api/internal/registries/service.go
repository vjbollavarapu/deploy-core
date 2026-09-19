package registries

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/google/uuid"
)

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
}

func NewService(repo Repository, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger, cfg ServiceConfig) *Service {
	if cfg.KeyID == "" {
		cfg.KeyID = "platform:v1"
	}
	return &Service{repo: repo, authz: authz, audit: auditWriter, log: log, cfg: cfg, now: time.Now}
}

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, in CreateInput, meta AuditMeta) (Registry, error) {
	if err := s.authz.RequirePermission(ctx, actorID, in.OrganizationID, rbac.RegistryManage); err != nil {
		return Registry{}, err
	}
	providerName := strings.ToLower(strings.TrimSpace(in.Provider))
	provider, err := Lookup(providerName)
	if err != nil {
		return Registry{}, apierror.Validation("unsupported provider", map[string]any{"provider": providerName})
	}
	if !SupportedInitially(providerName) {
		return Registry{}, apierror.Validation("provider is reserved but not enabled yet", map[string]any{
			"provider": providerName,
			"supported": []string{ProviderGHCR, ProviderDockerHub, ProviderOCI},
		})
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Registry{}, apierror.Validation("name is required", nil)
	}
	url, err := provider.NormalizeURL(in.RegistryURL)
	if err != nil {
		return Registry{}, apierror.Validation("invalid registryUrl", map[string]any{"registryUrl": err.Error()})
	}
	if err := provider.ValidateCredentials(in.Credentials); err != nil {
		return Registry{}, apierror.Validation("invalid credentials", map[string]any{"credentials": err.Error()})
	}

	username := strings.TrimSpace(in.Username)
	if username == "" && in.Credentials != nil {
		username = strings.TrimSpace(in.Credentials.Username)
	}

	var cred *credentialBlob
	if in.Credentials != nil && hasSecret(in.Credentials) {
		blob, err := s.sealCredentials(in.Credentials)
		if err != nil {
			return Registry{}, err
		}
		cred = blob
	}

	reg, err := s.repo.Create(ctx, Registry{
		OrganizationID: in.OrganizationID,
		Name:           name,
		Provider:       providerName,
		RegistryURL:    url,
		Username:       username,
		Status:         StatusActive,
		Metadata:       in.Metadata,
	}, cred, actorID)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Registry{}, apierror.Conflict("registry name already exists in organization")
		}
		return Registry{}, err
	}
	s.writeAudit(ctx, &reg.OrganizationID, &actorID, "registry.create", "registry", reg.ID.String(), meta, nil, map[string]any{
		"name": reg.Name, "provider": reg.Provider, "registryUrl": reg.RegistryURL,
	})
	return reg, nil
}

func (s *Service) List(ctx context.Context, actorID, orgID uuid.UUID, limit, offset int) ([]Registry, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.RegistryRead); err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, orgID, limit, offset)
}

func (s *Service) Get(ctx context.Context, actorID, id uuid.UUID) (Registry, error) {
	reg, _, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Registry{}, apierror.NotFound("registry not found")
		}
		return Registry{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, reg.OrganizationID, rbac.RegistryRead); err != nil {
		return Registry{}, err
	}
	return reg, nil
}

func (s *Service) Update(ctx context.Context, actorID, id uuid.UUID, in UpdateInput, meta AuditMeta) (Registry, error) {
	before, _, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Registry{}, apierror.NotFound("registry not found")
		}
		return Registry{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.RegistryManage); err != nil {
		return Registry{}, err
	}
	provider, err := Lookup(before.Provider)
	if err != nil {
		return Registry{}, err
	}

	after := before
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" {
			return Registry{}, apierror.Validation("name is required", nil)
		}
		after.Name = n
	}
	if in.RegistryURL != nil {
		url, err := provider.NormalizeURL(*in.RegistryURL)
		if err != nil {
			return Registry{}, apierror.Validation("invalid registryUrl", map[string]any{"registryUrl": err.Error()})
		}
		after.RegistryURL = url
	}
	if in.Username != nil {
		after.Username = strings.TrimSpace(*in.Username)
	}
	if in.Status != nil {
		st := strings.ToLower(strings.TrimSpace(*in.Status))
		switch st {
		case StatusActive, StatusDisabled, StatusError:
			after.Status = st
		default:
			return Registry{}, apierror.Validation("invalid status", nil)
		}
	}
	if in.Metadata != nil {
		after.Metadata = in.Metadata
	}

	var cred *credentialBlob
	if in.ClearCredentials {
		// handled by repo flag
	} else if in.Credentials != nil && hasSecret(in.Credentials) {
		if err := provider.ValidateCredentials(in.Credentials); err != nil {
			return Registry{}, apierror.Validation("invalid credentials", map[string]any{"credentials": err.Error()})
		}
		if after.Username == "" {
			after.Username = strings.TrimSpace(in.Credentials.Username)
		}
		blob, err := s.sealCredentials(in.Credentials)
		if err != nil {
			return Registry{}, err
		}
		cred = blob
	}

	updated, err := s.repo.Update(ctx, id, after, cred, in.ClearCredentials)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Registry{}, apierror.Conflict("registry name already exists in organization")
		}
		return Registry{}, err
	}
	s.writeAudit(ctx, &updated.OrganizationID, &actorID, "registry.update", "registry", id.String(), meta,
		map[string]any{"status": before.Status, "name": before.Name},
		map[string]any{"status": updated.Status, "name": updated.Name, "hasCredentials": updated.HasCredentials},
	)
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) error {
	reg, _, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFound("registry not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, reg.OrganizationID, rbac.RegistryManage); err != nil {
		return err
	}
	if err := s.repo.SoftDelete(ctx, id, s.now().UTC()); err != nil {
		return err
	}
	s.writeAudit(ctx, &reg.OrganizationID, &actorID, "registry.delete", "registry", id.String(), meta,
		map[string]any{"name": reg.Name, "provider": reg.Provider}, nil,
	)
	return nil
}

// ResolveCredentials decrypts registry auth for agent pull/push operations.
// Never expose via public HTTP handlers.
func (s *Service) ResolveCredentials(ctx context.Context, id uuid.UUID) (Registry, Credentials, error) {
	reg, blob, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Registry{}, Credentials{}, apierror.NotFound("registry not found")
		}
		return Registry{}, Credentials{}, err
	}
	if blob == nil {
		return reg, Credentials{Username: reg.Username}, nil
	}
	plain, err := crypto.Open(s.cfg.PlatformKey, crypto.Envelope{
		KeyID: blob.KeyID, Nonce: blob.Nonce, Ciphertext: blob.Ciphertext,
	})
	if err != nil {
		return Registry{}, Credentials{}, err
	}
	var creds Credentials
	if err := json.Unmarshal(plain, &creds); err != nil {
		return Registry{}, Credentials{}, err
	}
	if creds.Username == "" {
		creds.Username = reg.Username
	}
	return reg, creds, nil
}

func (s *Service) sealCredentials(creds *Credentials) (*credentialBlob, error) {
	normalized := Credentials{
		Username: strings.TrimSpace(creds.Username),
		Password: strings.TrimSpace(creds.Password),
		Token:    strings.TrimSpace(creds.Token),
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	env, err := crypto.Seal(s.cfg.PlatformKey, s.cfg.KeyID, raw)
	if err != nil {
		return nil, err
	}
	return &credentialBlob{Ciphertext: env.Ciphertext, Nonce: env.Nonce, KeyID: env.KeyID}, nil
}

func hasSecret(c *Credentials) bool {
	if c == nil {
		return false
	}
	return strings.TrimSpace(c.Password) != "" || strings.TrimSpace(c.Token) != ""
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
