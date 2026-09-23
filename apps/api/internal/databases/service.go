package databases

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/deploycore/deploy-core/apps/api/internal/agentcmd"
	"github.com/deploycore/deploy-core/apps/api/internal/agents"
	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

type ServiceConfig struct {
	PlatformKey []byte
	KeyID       string
}

type CommandStore interface {
	Create(ctx context.Context, cmd agentcmd.Command) (agentcmd.Command, error)
}

// VolumeRegistrar links a protected volume record for database-critical storage (B23).
type VolumeRegistrar interface {
	EnsureDatabaseVolume(ctx context.Context, orgID, serverID uuid.UUID, name string, databaseID uuid.UUID, createdBy *uuid.UUID) error
}

type Service struct {
	repo     Repository
	commands CommandStore
	volumes  VolumeRegistrar
	authz    *rbac.Authorizer
	audit    *audit.Writer
	log      *slog.Logger
	cfg      ServiceConfig
	now      func() time.Time
}

func NewService(repo Repository, commands CommandStore, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger, cfg ServiceConfig) *Service {
	if cfg.KeyID == "" {
		cfg.KeyID = "platform:v1"
	}
	return &Service{repo: repo, commands: commands, authz: authz, audit: auditWriter, log: log, cfg: cfg, now: time.Now}
}

func (s *Service) WithVolumeRegistrar(r VolumeRegistrar) *Service {
	s.volumes = r
	return s
}

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, in CreateInput, meta AuditMeta) (Database, error) {
	orgID, projectID, err := s.repo.ResolveEnvironment(ctx, in.EnvironmentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Database{}, apierror.NotFoundCode(apierror.CodeEnvironmentNotFound, "environment not found")
		}
		return Database{}, err
	}
	if in.ProjectID != uuid.Nil && in.ProjectID != projectID {
		return Database{}, apierror.Validation("projectId does not match environment", nil)
	}
	if in.OrganizationID != uuid.Nil && in.OrganizationID != orgID {
		return Database{}, apierror.Validation("organizationId does not match environment", nil)
	}
	in.OrganizationID = orgID
	in.ProjectID = projectID

	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.DatabaseCreate); err != nil {
		return Database{}, err
	}
	ok, err := s.repo.ServerInOrg(ctx, orgID, in.ServerID)
	if err != nil {
		return Database{}, err
	}
	if !ok {
		return Database{}, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found in organization")
	}

	name := strings.TrimSpace(in.Name)
	engine := strings.ToLower(strings.TrimSpace(in.Engine))
	if engine == "" {
		engine = EnginePostgreSQL
	}
	if engine != EnginePostgreSQL {
		return Database{}, apierror.Validation("only postgresql is supported initially", map[string]any{"engine": engine})
	}
	version := strings.TrimSpace(in.EngineVersion)
	if version == "" {
		version = "16"
	}
	dbName := strings.TrimSpace(in.DatabaseName)
	if dbName == "" {
		dbName = slugifyIdent(name)
	}
	username := strings.TrimSpace(in.Username)
	if username == "" {
		username = "deploycore"
	}
	volume := strings.TrimSpace(in.StorageVolume)
	if volume == "" {
		volume = "db-" + slugifyIdent(name) + "-data"
	}
	if err := validateCreate(name, dbName, username, volume, in.CPUMillis, in.MemoryBytes); err != nil {
		return Database{}, err
	}

	password := in.Password
	if strings.TrimSpace(password) == "" {
		password, err = crypto.RandomURLToken(24)
		if err != nil {
			return Database{}, apierror.Internal("could not generate database password")
		}
	}
	if len(password) < 8 {
		return Database{}, apierror.Validation("password must be at least 8 characters", map[string]any{"field": "password"})
	}
	env, err := crypto.Seal(s.cfg.PlatformKey, s.cfg.KeyID, []byte(password))
	if err != nil {
		return Database{}, err
	}

	policy := in.BackupPolicy
	if policy == nil {
		policy = map[string]any{"enabled": false, "retentionDays": 7}
	}

	d, err := s.repo.Create(ctx, Database{
		OrganizationID:    orgID,
		ProjectID:         projectID,
		EnvironmentID:     in.EnvironmentID,
		ServerID:          in.ServerID,
		Name:              name,
		Engine:            engine,
		EngineVersion:     version,
		DatabaseName:      dbName,
		Username:          username,
		StorageVolumeName: volume,
		VolumeProtected:   true,
		CPUMillis:         in.CPUMillis,
		MemoryBytes:       in.MemoryBytes,
		Status:            StatusPending,
		BackupPolicy:      policy,
	}, credentialBlob{Ciphertext: env.Ciphertext, Nonce: env.Nonce, KeyID: env.KeyID}, actorID)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Database{}, apierror.Conflict("database name or volume already exists")
		}
		return Database{}, err
	}

	// Creation must occur through agent commands — no local Docker work here.
	cmd, err := s.issueProvision(ctx, d, &actorID)
	if err != nil {
		_, _ = s.repo.SetProvisionState(ctx, d.ID, StatusFailed, nil, nil, err.Error())
		return Database{}, err
	}
	d, err = s.repo.SetProvisionState(ctx, d.ID, StatusProvisioning, &cmd.ID, nil, "")
	if err != nil {
		return Database{}, err
	}
	if s.volumes != nil {
		if err := s.volumes.EnsureDatabaseVolume(ctx, d.OrganizationID, d.ServerID, d.StorageVolumeName, d.ID, &actorID); err != nil && s.log != nil {
			s.log.Warn("failed to register protected database volume", slog.String("error", err.Error()))
		}
	}

	s.writeAudit(ctx, &orgID, &actorID, "database.create", "database", d.ID.String(), meta, nil, map[string]any{
		"name": d.Name, "engine": d.Engine, "engineVersion": d.EngineVersion,
		"serverId": d.ServerID.String(), "storageVolumeName": d.StorageVolumeName,
		"volumeProtected": true, "provisionCommandId": cmd.ID.String(),
	})
	return d, nil
}

func (s *Service) issueProvision(ctx context.Context, d Database, issuedBy *uuid.UUID) (agentcmd.Command, error) {
	if s.commands == nil {
		return agentcmd.Command{}, apierror.Internal("command store not configured")
	}
	now := s.now().UTC()
	reqID := requestid.FromContext(ctx)
	var reqPtr *string
	if reqID != "" {
		reqPtr = &reqID
	}
	payload := map[string]any{
		"databaseId":        d.ID.String(),
		"engine":            d.Engine,
		"engineVersion":     d.EngineVersion,
		"databaseName":      d.DatabaseName,
		"username":          d.Username,
		"storageVolumeName": d.StorageVolumeName,
		"volumeProtected":   d.VolumeProtected,
		// Password is never embedded — agent fetches via bootstrap endpoint.
	}
	if d.CPUMillis != nil {
		payload["cpuMillis"] = *d.CPUMillis
	}
	if d.MemoryBytes != nil {
		payload["memoryBytes"] = *d.MemoryBytes
	}
	return s.commands.Create(ctx, agentcmd.Command{
		OrganizationID: d.OrganizationID,
		ServerID:       d.ServerID,
		Operation:      protocol.OpProvisionDatabase,
		SchemaVersion:  protocol.SchemaVersion,
		Payload:        payload,
		IssuedAt:       now,
		ExpiresAt:      now.Add(30 * time.Minute),
		RequestID:      reqPtr,
		IssuedBy:       issuedBy,
	})
}

func (s *Service) List(ctx context.Context, actorID, orgID uuid.UUID, projectID, environmentID, serverID *uuid.UUID, limit, offset int) ([]Database, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.DatabaseRead); err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, orgID, projectID, environmentID, serverID, limit, offset)
}

func (s *Service) Get(ctx context.Context, actorID, id uuid.UUID) (Database, error) {
	d, _, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Database{}, apierror.NotFoundCode(apierror.CodeDatabaseNotFound, "database not found")
		}
		return Database{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, d.OrganizationID, rbac.DatabaseRead); err != nil {
		return Database{}, err
	}
	return d, nil
}

func (s *Service) RevealPassword(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) (string, Database, error) {
	d, cred, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", Database{}, apierror.NotFoundCode(apierror.CodeDatabaseNotFound, "database not found")
		}
		return "", Database{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, d.OrganizationID, rbac.DatabaseUpdate); err != nil {
		return "", Database{}, err
	}
	plain, err := crypto.Open(s.cfg.PlatformKey, crypto.Envelope{
		Ciphertext: cred.Ciphertext, Nonce: cred.Nonce, KeyID: cred.KeyID,
	})
	if err != nil {
		return "", Database{}, apierror.Internal("could not decrypt database credential")
	}
	s.writeAudit(ctx, &d.OrganizationID, &actorID, "database.credential.reveal", "database", d.ID.String(), meta, nil, map[string]any{
		"name": d.Name,
	})
	return string(plain), d, nil
}

func (s *Service) Update(ctx context.Context, actorID, id uuid.UUID, in UpdateInput, meta AuditMeta) (Database, error) {
	before, _, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Database{}, apierror.NotFoundCode(apierror.CodeDatabaseNotFound, "database not found")
		}
		return Database{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.DatabaseUpdate); err != nil {
		return Database{}, err
	}
	after := before
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return Database{}, apierror.Validation("name is required", nil)
		}
		after.Name = name
	}
	if in.CPUMillis != nil {
		if *in.CPUMillis <= 0 {
			return Database{}, apierror.Validation("cpuMillis must be positive", nil)
		}
		after.CPUMillis = in.CPUMillis
	}
	if in.MemoryBytes != nil {
		if *in.MemoryBytes <= 0 {
			return Database{}, apierror.Validation("memoryBytes must be positive", nil)
		}
		after.MemoryBytes = in.MemoryBytes
	}
	if in.BackupPolicy != nil {
		after.BackupPolicy = in.BackupPolicy
	}
	if in.Status != nil {
		// R16: PATCH status is CP-only and must not imply runtime stop/start succeeded.
		return Database{}, apierror.Validation(
			"database status cannot be changed via PATCH; use dedicated stop/start operations that issue Agent commands",
			map[string]any{"field": "status"},
		)
	}

	var cred *credentialBlob
	if in.RotatePassword != nil {
		pw := strings.TrimSpace(*in.RotatePassword)
		if pw == "" {
			pw, err = crypto.RandomURLToken(24)
			if err != nil {
				return Database{}, apierror.Internal("could not generate database password")
			}
		}
		if len(pw) < 8 {
			return Database{}, apierror.Validation("password must be at least 8 characters", nil)
		}
		env, err := crypto.Seal(s.cfg.PlatformKey, s.cfg.KeyID, []byte(pw))
		if err != nil {
			return Database{}, err
		}
		cred = &credentialBlob{Ciphertext: env.Ciphertext, Nonce: env.Nonce, KeyID: env.KeyID}
	}

	updated, err := s.repo.Update(ctx, id, after, cred)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Database{}, apierror.Conflict("database name already exists")
		}
		return Database{}, err
	}
	s.writeAudit(ctx, &updated.OrganizationID, &actorID, "database.update", "database", id.String(), meta,
		map[string]any{"name": before.Name, "status": before.Status},
		map[string]any{"name": updated.Name, "status": updated.Status, "passwordRotated": cred != nil},
	)
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) (DeleteResult, error) {
	d, _, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return DeleteResult{}, apierror.NotFoundCode(apierror.CodeDatabaseNotFound, "database not found")
		}
		return DeleteResult{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, d.OrganizationID, rbac.DatabaseUpdate); err != nil {
		return DeleteResult{}, err
	}
	// Soft-delete control-plane record only. Does NOT stop the Agent-managed
	// container or delete protected volumes (R15).
	if err := s.repo.SoftDelete(ctx, id, s.now().UTC()); err != nil {
		return DeleteResult{}, err
	}
	s.writeAudit(ctx, &d.OrganizationID, &actorID, "database.delete", "database", id.String(), meta,
		map[string]any{"name": d.Name, "storageVolumeName": d.StorageVolumeName, "volumeProtected": d.VolumeProtected},
		map[string]any{"status": StatusDeleted, "volumeDeleted": false, "runtimeStopped": false},
	)
	return DeleteResult{
		ID:             id,
		SoftDeleted:    true,
		RuntimeStopped: false,
		VolumeDeleted:  false,
		Message:        "control-plane record soft-deleted; managed container was not stopped and volume was not deleted",
	}, nil
}

// DeleteResult describes the truthful outcome of a database delete request.
type DeleteResult struct {
	ID             uuid.UUID
	SoftDeleted    bool
	RuntimeStopped bool
	VolumeDeleted  bool
	Message        string
}

// BootstrapForAgent returns provision secrets for the owning agent. Password is
// only available while provisioning (never logged).
func (s *Service) BootstrapForAgent(ctx context.Context, agent agents.Agent, id uuid.UUID) (BootstrapSecrets, error) {
	d, cred, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return BootstrapSecrets{}, apierror.NotFoundCode(apierror.CodeDatabaseNotFound, "database not found")
		}
		return BootstrapSecrets{}, err
	}
	if d.OrganizationID != agent.OrganizationID || d.ServerID != agent.ServerID {
		return BootstrapSecrets{}, apierror.Forbidden("database is outside agent scope")
	}
	switch d.Status {
	case StatusPending, StatusProvisioning, StatusRunning, StatusDegraded, StatusStopped:
	default:
		return BootstrapSecrets{}, apierror.Conflict("credentials only available for active databases")
	}
	plain, err := crypto.Open(s.cfg.PlatformKey, crypto.Envelope{
		Ciphertext: cred.Ciphertext, Nonce: cred.Nonce, KeyID: cred.KeyID,
	})
	if err != nil {
		return BootstrapSecrets{}, apierror.Internal("could not decrypt database credential")
	}
	return BootstrapSecrets{
		DatabaseID:        d.ID,
		Engine:            d.Engine,
		EngineVersion:     d.EngineVersion,
		DatabaseName:      d.DatabaseName,
		Username:          d.Username,
		Password:          string(plain),
		StorageVolumeName: d.StorageVolumeName,
		CPUMillis:         d.CPUMillis,
		MemoryBytes:       d.MemoryBytes,
	}, nil
}

// HandleCommandCompletion is wired from agentcmd when PROVISION_DATABASE finishes.
func (s *Service) HandleCommandCompletion(ctx context.Context, cmd agentcmd.Command) error {
	if cmd.Operation != protocol.OpProvisionDatabase {
		return nil
	}
	dbIDRaw, _ := cmd.Payload["databaseId"].(string)
	dbID, err := uuid.Parse(strings.TrimSpace(dbIDRaw))
	if err != nil {
		return nil
	}
	d, _, err := s.repo.Get(ctx, dbID)
	if err != nil {
		return nil
	}
	if d.ServerID != cmd.ServerID {
		return nil
	}

	switch cmd.Status {
	case protocol.StatusCompleted:
		var runtimeID *string
		if cmd.Result != nil {
			if v, ok := cmd.Result["containerRuntimeId"].(string); ok && strings.TrimSpace(v) != "" {
				v = strings.TrimSpace(v)
				runtimeID = &v
			} else if v, ok := cmd.Result["containerId"].(string); ok && strings.TrimSpace(v) != "" {
				v = strings.TrimSpace(v)
				runtimeID = &v
			} else if nested, ok := cmd.Result["database"].(map[string]any); ok {
				if v, ok := nested["containerId"].(string); ok && strings.TrimSpace(v) != "" {
					v = strings.TrimSpace(v)
					runtimeID = &v
				}
			}
		}
		_, err = s.repo.SetProvisionState(ctx, dbID, StatusRunning, &cmd.ID, runtimeID, "")
		return err
	case protocol.StatusFailed:
		msg := ""
		if cmd.ErrorMessage != nil {
			msg = *cmd.ErrorMessage
		}
		_, err = s.repo.SetProvisionState(ctx, dbID, StatusFailed, &cmd.ID, nil, msg)
		return err
	default:
		return nil
	}
}

func validateCreate(name, dbName, username, volume string, cpu *int, mem *int64) error {
	if name == "" {
		return apierror.Validation("name is required", map[string]any{"field": "name"})
	}
	if !identRe.MatchString(dbName) {
		return apierror.Validation("databaseName must be a simple identifier", map[string]any{"field": "databaseName"})
	}
	if !identRe.MatchString(username) {
		return apierror.Validation("username must be a simple identifier", map[string]any{"field": "username"})
	}
	if !volumeRe.MatchString(volume) {
		return apierror.Validation("storageVolume must be a valid volume name", map[string]any{"field": "storageVolume"})
	}
	if cpu != nil && *cpu <= 0 {
		return apierror.Validation("cpuMillis must be positive", nil)
	}
	if mem != nil && *mem <= 0 {
		return apierror.Validation("memoryBytes must be positive", nil)
	}
	return nil
}

var (
	identRe  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,62}$`)
	volumeRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
)

func slugifyIdent(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore && b.Len() > 0 {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "db"
	}
	if !unicode.IsLetter(rune(out[0])) {
		out = "db_" + out
	}
	if len(out) > 63 {
		out = out[:63]
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
		ActorType:      "user",
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
