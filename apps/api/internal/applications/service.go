package applications

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/healthchecks"
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
	repo     Repository
	authz    *rbac.Authorizer
	audit    *audit.Writer
	log      *slog.Logger
	now      func() time.Time
	capacity CapacityTracker
}

// CapacityTracker refreshes denormalized server allocation and validates capacity.
type CapacityTracker interface {
	RefreshAllocated(ctx context.Context, serverIDs ...uuid.UUID) error
	ValidateTargetCapacity(ctx context.Context, orgID, serverID uuid.UUID, cpuMillis int, memBytes, diskBytes int64, excludeAppID *uuid.UUID) error
}

func NewService(repo Repository, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger) *Service {
	return &Service{repo: repo, authz: authz, audit: auditWriter, log: log, now: time.Now}
}

func (s *Service) WithCapacity(tracker CapacityTracker) *Service {
	s.capacity = tracker
	return s
}

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, in CreateInput, meta AuditMeta) (Application, error) {
	orgID, projectID, err := s.repo.ResolveEnvironment(ctx, in.EnvironmentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Application{}, apierror.NotFoundCode(apierror.CodeEnvironmentNotFound, "environment not found")
		}
		return Application{}, err
	}
	if in.ProjectID != uuid.Nil && in.ProjectID != projectID {
		return Application{}, apierror.Validation("projectId does not match environment", nil)
	}
	if in.OrganizationID != uuid.Nil && in.OrganizationID != orgID {
		return Application{}, apierror.Validation("organizationId does not match environment", nil)
	}
	in.OrganizationID = orgID
	in.ProjectID = projectID

	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ApplicationCreate); err != nil {
		return Application{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if strings.TrimSpace(in.Slug) == "" {
		in.Slug = slugify(in.Name)
	} else {
		in.Slug = strings.TrimSpace(in.Slug)
	}
	in.Type = strings.TrimSpace(in.Type)
	in.Config.SourceType = strings.ToLower(strings.TrimSpace(in.Config.SourceType))
	if err := validateCreate(in); err != nil {
		return Application{}, err
	}
	if in.TargetServerID != nil {
		ok, err := s.repo.ServerInOrg(ctx, orgID, *in.TargetServerID)
		if err != nil {
			return Application{}, err
		}
		if !ok {
			return Application{}, apierror.NotFoundCode(apierror.CodeServerNotFound, "target server not found in organization")
		}
		if err := s.validateCapacity(ctx, orgID, *in.TargetServerID, in.Config, nil); err != nil {
			return Application{}, err
		}
	}

	app, err := s.repo.Create(ctx, in, actorID)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Application{}, apierror.Conflict("application slug already exists in environment")
		}
		return Application{}, err
	}
	if app.TargetServerID != nil {
		s.refreshCapacity(ctx, *app.TargetServerID)
	}
	s.writeAudit(ctx, &orgID, &actorID, "application.create", "application", app.ID.String(), meta, nil, map[string]any{
		"name": app.Name, "slug": app.Slug, "type": app.Type, "sourceType": app.Config.SourceType,
	})
	return app, nil
}

func (s *Service) List(ctx context.Context, actorID, orgID uuid.UUID, projectID, environmentID *uuid.UUID, limit, offset int) ([]Application, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ApplicationRead); err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, orgID, projectID, environmentID, limit, offset)
}

func (s *Service) Get(ctx context.Context, actorID, appID uuid.UUID) (Application, error) {
	app, err := s.repo.Get(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Application{}, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return Application{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, app.OrganizationID, rbac.ApplicationRead); err != nil {
		return Application{}, err
	}
	return app, nil
}

func (s *Service) Update(ctx context.Context, actorID, appID uuid.UUID, in UpdateInput, meta AuditMeta) (Application, error) {
	before, err := s.repo.Get(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Application{}, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return Application{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.ApplicationUpdate); err != nil {
		return Application{}, err
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
	}
	if in.Slug != nil {
		sl := strings.TrimSpace(*in.Slug)
		in.Slug = &sl
	}
	if in.Config != nil {
		in.Config.SourceType = strings.ToLower(strings.TrimSpace(in.Config.SourceType))
		if in.Config.SourceType == "" {
			in.Config.SourceType = before.Config.SourceType
		}
		if in.Config.AutoDeployEnabled == nil {
			v := before.Config.AutoDeployEnabled
			in.Config.AutoDeployEnabled = &v
		}
		if in.Config.GitConnectionID == nil && !in.Config.ClearGitConnection {
			in.Config.GitConnectionID = before.Config.GitConnectionID
		}
	}
	if err := validateUpdate(before.Type, in); err != nil {
		return Application{}, err
	}

	nextTarget := before.TargetServerID
	if in.ClearTarget {
		nextTarget = nil
	} else if in.TargetServerID != nil {
		nextTarget = in.TargetServerID
	}
	nextPlacement := before.PlacementPolicy
	if in.PlacementPolicy != nil {
		nextPlacement = in.PlacementPolicy
	}
	if nextTarget == nil && len(nextPlacement) == 0 {
		return Application{}, apierror.Validation("target server or placement policy is required", map[string]any{
			"targetServerId": "target server or placement policy is required",
		})
	}

	if in.TargetServerID != nil && !in.ClearTarget {
		ok, err := s.repo.ServerInOrg(ctx, before.OrganizationID, *in.TargetServerID)
		if err != nil {
			return Application{}, err
		}
		if !ok {
			return Application{}, apierror.NotFoundCode(apierror.CodeServerNotFound, "target server not found in organization")
		}
	}

	cfgForCap := before.Config
	if in.Config != nil {
		cfgForCap = Config{
			CPULimitMillis:   in.Config.CPULimitMillis,
			MemoryLimitBytes: in.Config.MemoryLimitBytes,
			RuntimeConfig:    in.Config.RuntimeConfig,
		}
		if cfgForCap.CPULimitMillis == nil {
			cfgForCap.CPULimitMillis = before.Config.CPULimitMillis
		}
		if cfgForCap.MemoryLimitBytes == nil {
			cfgForCap.MemoryLimitBytes = before.Config.MemoryLimitBytes
		}
		if cfgForCap.RuntimeConfig == nil {
			cfgForCap.RuntimeConfig = before.Config.RuntimeConfig
		}
	}
	if nextTarget != nil {
		exclude := &appID
		if err := s.validateCapacityFromConfig(ctx, before.OrganizationID, *nextTarget, cfgForCap, exclude); err != nil {
			return Application{}, err
		}
	}

	app, err := s.repo.UpdateApp(ctx, appID, in)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Application{}, apierror.Conflict("application slug already exists in environment")
		}
		return Application{}, err
	}

	if in.Config != nil {
		ver, err := s.repo.NextConfigVersion(ctx, appID)
		if err != nil {
			return Application{}, err
		}
		cfg, err := s.repo.InsertConfig(ctx, before.OrganizationID, appID, ver, *in.Config, actorID)
		if err != nil {
			return Application{}, err
		}
		app.Config = cfg
	} else {
		app.Config = before.Config
	}

	refreshIDs := make([]uuid.UUID, 0, 2)
	if before.TargetServerID != nil {
		refreshIDs = append(refreshIDs, *before.TargetServerID)
	}
	if app.TargetServerID != nil {
		refreshIDs = append(refreshIDs, *app.TargetServerID)
	}
	s.refreshCapacity(ctx, refreshIDs...)

	s.writeAudit(ctx, &before.OrganizationID, &actorID, "application.update", "application", appID.String(), meta,
		map[string]any{"name": before.Name, "slug": before.Slug, "configVersion": before.Config.Version},
		map[string]any{"name": app.Name, "slug": app.Slug, "configVersion": app.Config.Version},
	)
	return app, nil
}

func (s *Service) Delete(ctx context.Context, actorID, appID uuid.UUID, meta AuditMeta) error {
	app, err := s.repo.Get(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, app.OrganizationID, rbac.ApplicationDelete); err != nil {
		return err
	}
	n, err := s.repo.CountActiveDeployments(ctx, appID)
	if err != nil {
		return err
	}
	if n > 0 {
		return apierror.Conflict("application has in-progress deployments; cancel or wait for them first")
	}
	if err := s.repo.SoftDelete(ctx, appID, s.now().UTC()); err != nil {
		return err
	}
	if app.TargetServerID != nil {
		s.refreshCapacity(ctx, *app.TargetServerID)
	}
	s.writeAudit(ctx, &app.OrganizationID, &actorID, "application.delete", "application", appID.String(), meta,
		map[string]any{"name": app.Name, "slug": app.Slug}, nil,
	)
	return nil
}

func validateCreate(in CreateInput) error {
	var errs validation.Errors
	validation.RequiredString(&errs, "name", strings.TrimSpace(in.Name))
	validation.MaxLen(&errs, "name", in.Name, 120)
	if err := validateSlug(in.Slug); err != nil {
		errs.Add("slug", err.Error())
	}
	if !validType(in.Type) {
		errs.Add("type", "must be a supported application type")
	}
	if in.TargetServerID == nil && len(in.PlacementPolicy) == 0 {
		errs.Add("targetServerId", "target server or placement policy is required")
	}
	validateConfigFields(&errs, in.Type, in.Config)
	if !errs.Empty() {
		return apierror.Validation("invalid application", errs.Details())
	}
	return nil
}

func validateUpdate(appType string, in UpdateInput) error {
	var errs validation.Errors
	if in.Name != nil {
		validation.RequiredString(&errs, "name", strings.TrimSpace(*in.Name))
		validation.MaxLen(&errs, "name", *in.Name, 120)
	}
	if in.Slug != nil {
		if err := validateSlug(strings.TrimSpace(*in.Slug)); err != nil {
			errs.Add("slug", err.Error())
		}
	}
	if in.Config != nil {
		validateConfigFields(&errs, appType, *in.Config)
	}
	if !errs.Empty() {
		return apierror.Validation("invalid application", errs.Details())
	}
	return nil
}

func validateConfigFields(errs *validation.Errors, appType string, cfg ConfigInput) {
	src := strings.TrimSpace(strings.ToLower(cfg.SourceType))
	if src == "" {
		errs.Add("sourceType", "is required")
		return
	}
	switch src {
	case SourceGit, SourceImage, SourceCompose, SourceUpload:
	default:
		errs.Add("sourceType", "must be git, image, compose, or upload")
		return
	}

	switch appType {
	case TypeDockerImage:
		if src != SourceImage {
			errs.Add("sourceType", "DOCKER_IMAGE applications require sourceType=image")
		}
	case TypeCompose:
		if src != SourceCompose && src != SourceGit {
			errs.Add("sourceType", "DOCKER_COMPOSE applications require sourceType=compose or git")
		}
	}

	switch src {
	case SourceGit:
		if strEmpty(cfg.RepositoryURL) {
			errs.Add("repositoryUrl", "is required for git source")
		}
		if strEmpty(cfg.GitBranch) {
			errs.Add("gitBranch", "is required for git source")
		}
	case SourceImage:
		if strEmpty(cfg.ImageReference) {
			errs.Add("imageReference", "is required for image source")
		}
	case SourceCompose:
		if strEmpty(cfg.RepositoryURL) && strEmpty(cfg.BuildContext) {
			errs.Add("repositoryUrl", "repositoryUrl or buildContext is required for compose source")
		}
	case SourceUpload:
		if strEmpty(cfg.BuildContext) {
			errs.Add("buildContext", "is required for upload source")
		}
	}

	needsPort := appType == TypeWebService || appType == TypeAPI || appType == TypeStaticSite
	if needsPort && (cfg.InternalPort == nil || *cfg.InternalPort <= 0 || *cfg.InternalPort > 65535) {
		errs.Add("internalPort", "is required for this application type (1-65535)")
	}
	if cfg.InternalPort != nil && (*cfg.InternalPort <= 0 || *cfg.InternalPort > 65535) {
		errs.Add("internalPort", "must be between 1 and 65535")
	}
	if cfg.CPULimitMillis != nil && *cfg.CPULimitMillis <= 0 {
		errs.Add("cpuLimitMillis", "must be greater than 0")
	}
	if cfg.MemoryLimitBytes != nil && *cfg.MemoryLimitBytes <= 0 {
		errs.Add("memoryLimitBytes", "must be greater than 0")
	}
	if cfg.RestartPolicy != "" {
		switch cfg.RestartPolicy {
		case "no", "always", "on-failure", "unless-stopped":
		default:
			errs.Add("restartPolicy", "must be no, always, on-failure, or unless-stopped")
		}
	}
	if cfg.HealthCheck != nil {
		if err := healthchecks.ValidateMap(cfg.HealthCheck); err != nil {
			errs.Add("healthCheck", err.Error())
		}
	}
	if cfg.RuntimeConfig != nil {
		if v, ok := cfg.RuntimeConfig["desiredReplicas"]; ok {
			n := 0
			switch t := v.(type) {
			case float64:
				n = int(t)
			case int:
				n = t
			case int64:
				n = int(t)
			default:
				errs.Add("runtimeConfig.desiredReplicas", "must be an integer")
				n = -1
			}
			if n != -1 && (n < 1 || n > 20) {
				errs.Add("runtimeConfig.desiredReplicas", "must be between 1 and 20")
			}
		}
	}
}

func validType(t string) bool {
	switch t {
	case TypeWebService, TypeAPI, TypeWorker, TypeScheduledJob, TypeStaticSite, TypeCompose, TypeDockerImage:
		return true
	default:
		return false
	}
}

func strEmpty(v *string) bool {
	return v == nil || strings.TrimSpace(*v) == ""
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
		return "app"
	}
	if len(out) > 63 {
		out = strings.Trim(out[:63], "-")
	}
	return out
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

func (s *Service) refreshCapacity(ctx context.Context, serverIDs ...uuid.UUID) {
	if s.capacity == nil || len(serverIDs) == 0 {
		return
	}
	if err := s.capacity.RefreshAllocated(ctx, serverIDs...); err != nil {
		s.log.Error("capacity refresh failed", slog.String("error", err.Error()))
	}
}

func (s *Service) validateCapacity(ctx context.Context, orgID, serverID uuid.UUID, cfg ConfigInput, excludeAppID *uuid.UUID) error {
	if s.capacity == nil {
		return nil
	}
	cpu, mem, disk := limitsFromConfigInput(cfg)
	return s.capacity.ValidateTargetCapacity(ctx, orgID, serverID, cpu, mem, disk, excludeAppID)
}

func (s *Service) validateCapacityFromConfig(ctx context.Context, orgID, serverID uuid.UUID, cfg Config, excludeAppID *uuid.UUID) error {
	if s.capacity == nil {
		return nil
	}
	cpu, mem, disk := limitsFromConfig(cfg)
	return s.capacity.ValidateTargetCapacity(ctx, orgID, serverID, cpu, mem, disk, excludeAppID)
}

func limitsFromConfigInput(cfg ConfigInput) (cpu int, mem, disk int64) {
	if cfg.CPULimitMillis != nil {
		cpu = *cfg.CPULimitMillis
	}
	if cfg.MemoryLimitBytes != nil {
		mem = *cfg.MemoryLimitBytes
	}
	disk = diskFromRuntime(cfg.RuntimeConfig)
	desired := desiredReplicasFromRuntime(cfg.RuntimeConfig)
	return cpu * desired, mem * int64(desired), disk * int64(desired)
}

func limitsFromConfig(cfg Config) (cpu int, mem, disk int64) {
	if cfg.CPULimitMillis != nil {
		cpu = *cfg.CPULimitMillis
	}
	if cfg.MemoryLimitBytes != nil {
		mem = *cfg.MemoryLimitBytes
	}
	disk = diskFromRuntime(cfg.RuntimeConfig)
	desired := desiredReplicasFromRuntime(cfg.RuntimeConfig)
	return cpu * desired, mem * int64(desired), disk * int64(desired)
}

func desiredReplicasFromRuntime(runtime map[string]any) int {
	if runtime == nil {
		return 1
	}
	switch v := runtime["desiredReplicas"].(type) {
	case float64:
		if int(v) < 1 {
			return 1
		}
		return int(v)
	case int:
		if v < 1 {
			return 1
		}
		return v
	case int64:
		if int(v) < 1 {
			return 1
		}
		return int(v)
	default:
		return 1
	}
}

func diskFromRuntime(runtime map[string]any) int64 {
	if runtime == nil {
		return 0
	}
	switch v := runtime["diskBytes"].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	default:
		return 0
	}
}
