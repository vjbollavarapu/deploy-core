package volumes

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agentcmd"
	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/security"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

type CommandStore interface {
	Create(ctx context.Context, cmd agentcmd.Command) (agentcmd.Command, error)
}

type Service struct {
	repo     Repository
	commands CommandStore
	authz    *rbac.Authorizer
	audit    *audit.Writer
	log      *slog.Logger
	now      func() time.Time
}

func NewService(repo Repository, commands CommandStore, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger) *Service {
	return &Service{repo: repo, commands: commands, authz: authz, audit: auditWriter, log: log, now: time.Now}
}

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, in CreateInput, meta AuditMeta) (Volume, error) {
	orgID, err := s.repo.GetServerOrg(ctx, in.ServerID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Volume{}, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return Volume{}, err
	}
	if in.OrganizationID != uuid.Nil && in.OrganizationID != orgID {
		return Volume{}, apierror.Validation("organizationId does not match server", nil)
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ServerUpdate); err != nil {
		return Volume{}, err
	}

	name := strings.TrimSpace(in.Name)
	driver := strings.TrimSpace(in.Driver)
	if driver == "" {
		driver = DriverLocal
	}
	if !nameRe.MatchString(name) {
		return Volume{}, apierror.Validation("invalid volume name", map[string]any{"field": "name"})
	}
	labels := mapOrEmpty(in.Labels)
	labels["deploycore.managed"] = "true"
	labels["deploycore.owner"] = "platform"
	labels["deploycore.organization_id"] = orgID.String()

	v, err := s.repo.Create(ctx, Volume{
		OrganizationID: orgID,
		ServerID:       in.ServerID,
		Name:           name,
		Driver:         driver,
		MountPath:      strings.TrimSpace(in.MountPath),
		State:          StatePending,
		BackupPolicy:   in.BackupPolicy,
		Protected:      in.Protected,
		Labels:         labels,
	}, &actorID)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Volume{}, apierror.Conflict("volume name already exists on server")
		}
		return Volume{}, err
	}

	cmd, err := s.issue(ctx, v, protocol.OpCreateVolume, createVolumeCommandPayload(v), &actorID)
	if err != nil {
		_, _ = s.repo.SetState(ctx, v.ID, StateFailed, nil, nil, nil, err.Error())
		return Volume{}, err
	}
	v, err = s.repo.SetState(ctx, v.ID, StateCreating, &cmd.ID, nil, nil, "")
	if err != nil {
		return Volume{}, err
	}
	s.writeAudit(ctx, &orgID, &actorID, "volume.create", "volume", v.ID.String(), meta, nil, map[string]any{
		"name": v.Name, "serverId": v.ServerID.String(), "driver": v.Driver, "commandId": cmd.ID.String(),
	})
	return v, nil
}

// EnsureDatabaseVolume registers a protected, database-critical volume without
// issuing a separate CREATE_VOLUME when the database provisioner owns creation.
func (s *Service) EnsureDatabaseVolume(ctx context.Context, orgID, serverID uuid.UUID, name string, databaseID uuid.UUID, createdBy *uuid.UUID) (Volume, error) {
	name = strings.TrimSpace(name)
	if !nameRe.MatchString(name) {
		return Volume{}, apierror.Validation("invalid volume name", nil)
	}
	existing, err := s.repo.GetByServerName(ctx, serverID, name)
	if err == nil {
		rt := ResourceDatabase
		return s.repo.SetAttachment(ctx, existing.ID, &rt, &databaseID, existing.MountPath, StateAttached, nil)
	}
	if !errors.Is(err, ErrNotFound) {
		return Volume{}, err
	}
	rt := ResourceDatabase
	labels := map[string]any{
		"deploycore.managed":         "true",
		"deploycore.owner":           "platform",
		"deploycore.critical":        "database",
		"deploycore.organization_id": orgID.String(),
	}
	v, err := s.repo.Create(ctx, Volume{
		OrganizationID:       orgID,
		ServerID:             serverID,
		Name:                 name,
		Driver:               DriverLocal,
		MountPath:            "/var/lib/postgresql/data",
		State:                StateAttached,
		AttachedResourceType: &rt,
		AttachedResourceID:   &databaseID,
		BackupPolicy:         map[string]any{"enabled": false},
		Protected:            true,
		Labels:               labels,
	}, createdBy)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return s.repo.GetByServerName(ctx, serverID, name)
		}
		return Volume{}, err
	}
	return v, nil
}

func (s *Service) List(ctx context.Context, actorID, orgID uuid.UUID, serverID *uuid.UUID, limit, offset int) ([]Volume, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ServerRead); err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, orgID, serverID, limit, offset)
}

func (s *Service) Get(ctx context.Context, actorID, id uuid.UUID) (Volume, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Volume{}, apierror.NotFoundCode(apierror.CodeVolumeNotFound, "volume not found")
		}
		return Volume{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, v.OrganizationID, rbac.ServerRead); err != nil {
		return Volume{}, err
	}
	return v, nil
}

// RetryCreate requeues CREATE_VOLUME for a volume that failed before Docker created it.
func (s *Service) RetryCreate(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) (Volume, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Volume{}, apierror.NotFoundCode(apierror.CodeVolumeNotFound, "volume not found")
		}
		return Volume{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, v.OrganizationID, rbac.ServerUpdate); err != nil {
		return Volume{}, err
	}
	if v.AttachedResourceID != nil || v.State == StateAttached {
		return Volume{}, apierror.Conflict("cannot retry create for an attached volume")
	}
	if v.DockerName != nil && strings.TrimSpace(*v.DockerName) != "" {
		return Volume{}, apierror.Conflict("cannot retry create after a docker volume exists")
	}
	if v.State != StateFailed {
		return Volume{}, apierror.Conflict("volume create can only be retried from FAILED")
	}
	cmd, err := s.issue(ctx, v, protocol.OpCreateVolume, createVolumeCommandPayload(v), &actorID)
	if err != nil {
		return Volume{}, err
	}
	v, err = s.repo.SetState(ctx, v.ID, StateCreating, &cmd.ID, nil, nil, "")
	if err != nil {
		return Volume{}, err
	}
	s.writeAudit(ctx, &v.OrganizationID, &actorID, "volume.create.retry", "volume", v.ID.String(), meta,
		map[string]any{"state": StateFailed},
		map[string]any{"state": StateCreating, "commandId": cmd.ID.String()},
	)
	return v, nil
}

func (s *Service) Inspect(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) (Volume, error) {
	v, err := s.Get(ctx, actorID, id)
	if err != nil {
		return Volume{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, v.OrganizationID, rbac.ServerUpdate); err != nil {
		return Volume{}, err
	}
	cmd, err := s.issue(ctx, v, protocol.OpInspectVolume, map[string]any{
		"volumeId": v.ID.String(),
		"name":     v.Name,
	}, &actorID)
	if err != nil {
		return Volume{}, err
	}
	v, err = s.repo.SetState(ctx, v.ID, v.State, &cmd.ID, nil, nil, "")
	if err != nil {
		return Volume{}, err
	}
	s.writeAudit(ctx, &v.OrganizationID, &actorID, "volume.inspect", "volume", v.ID.String(), meta, nil, map[string]any{
		"commandId": cmd.ID.String(),
	})
	return v, nil
}

func (s *Service) Attach(ctx context.Context, actorID, id uuid.UUID, in AttachInput, meta AuditMeta) (Volume, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Volume{}, apierror.NotFoundCode(apierror.CodeVolumeNotFound, "volume not found")
		}
		return Volume{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, v.OrganizationID, rbac.ServerUpdate); err != nil {
		return Volume{}, err
	}
	rt := strings.ToLower(strings.TrimSpace(in.ResourceType))
	switch rt {
	case ResourceDatabase:
		ok, err := s.repo.DatabaseInOrg(ctx, v.OrganizationID, in.ResourceID)
		if err != nil {
			return Volume{}, err
		}
		if !ok {
			return Volume{}, apierror.NotFoundCode(apierror.CodeDatabaseNotFound, "database not found")
		}
	case ResourceApplication:
		ok, err := s.repo.ApplicationInOrg(ctx, v.OrganizationID, in.ResourceID)
		if err != nil {
			return Volume{}, err
		}
		if !ok {
			return Volume{}, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
	default:
		return Volume{}, apierror.Validation("resourceType must be database or application", nil)
	}
	mount := strings.TrimSpace(in.MountPath)
	if err := security.ValidateContainerMountPath(mount); err != nil {
		return Volume{}, apierror.Validation(err.Error(), map[string]any{"mountPath": err.Error()})
	}
	if v.AttachedResourceID != nil && v.AttachedResourceType != nil {
		same := strings.EqualFold(*v.AttachedResourceType, rt) && *v.AttachedResourceID == in.ResourceID
		if !same {
			return Volume{}, apierror.ConflictCode(apierror.CodeVolumeInUse, "volume is already attached")
		}
	}
	if in.ReadOnly != nil {
		if v.Labels == nil {
			v.Labels = map[string]any{}
		}
		v.Labels["readOnly"] = *in.ReadOnly
		v.Labels["deploycore.managed"] = "true"
		v.Labels["deploycore.owner"] = "platform"
		v.Labels["deploycore.organization_id"] = v.OrganizationID.String()
		v.MountPath = mount
		updated, err := s.repo.Update(ctx, v)
		if err != nil {
			return Volume{}, err
		}
		v = updated
	}

	containerID := strings.TrimSpace(in.ContainerID)
	var commandID *uuid.UUID
	if containerID != "" {
		cmd, err := s.issue(ctx, v, protocol.OpAttachVolume, map[string]any{
			"volumeId":     v.ID.String(),
			"name":         v.Name,
			"resourceType": rt,
			"resourceId":   in.ResourceID.String(),
			"mountPath":    mount,
			"containerId":  containerID,
			"readOnly":     v.Labels["readOnly"] == true,
		}, &actorID)
		if err != nil {
			return Volume{}, err
		}
		commandID = &cmd.ID
	}
	v, err = s.repo.SetAttachment(ctx, v.ID, &rt, &in.ResourceID, mount, StateAttached, commandID)
	if err != nil {
		return Volume{}, err
	}
	s.writeAudit(ctx, &v.OrganizationID, &actorID, "volume.attach", "volume", v.ID.String(), meta,
		nil, map[string]any{"resourceType": rt, "resourceId": in.ResourceID.String(), "mountPath": mount, "containerId": containerID},
	)
	return v, nil
}

func (s *Service) Detach(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) (Volume, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Volume{}, apierror.NotFoundCode(apierror.CodeVolumeNotFound, "volume not found")
		}
		return Volume{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, v.OrganizationID, rbac.ServerUpdate); err != nil {
		return Volume{}, err
	}
	if v.AttachedResourceID == nil {
		return Volume{}, apierror.Conflict("volume is not attached")
	}
	if v.Protected {
		return Volume{}, apierror.ConflictCode(apierror.CodeVolumeInUse, "cannot detach database-critical protected volume")
	}
	cmd, err := s.issue(ctx, v, protocol.OpDetachVolume, map[string]any{
		"volumeId": v.ID.String(),
		"name":     v.Name,
	}, &actorID)
	if err != nil {
		return Volume{}, err
	}
	v, err = s.repo.SetAttachment(ctx, v.ID, nil, nil, v.MountPath, StateReady, &cmd.ID)
	if err != nil {
		return Volume{}, err
	}
	s.writeAudit(ctx, &v.OrganizationID, &actorID, "volume.detach", "volume", v.ID.String(), meta, nil, map[string]any{
		"state": StateReady, "commandId": cmd.ID.String(),
	})
	return v, nil
}

func (s *Service) Delete(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) error {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFoundCode(apierror.CodeVolumeNotFound, "volume not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, v.OrganizationID, rbac.ServerUpdate); err != nil {
		return err
	}
	if v.Protected {
		return apierror.ConflictCode(apierror.CodeVolumeInUse, "cannot delete database-critical protected volume")
	}
	if v.AttachedResourceID != nil || v.State == StateAttached {
		return apierror.ConflictCode(apierror.CodeVolumeInUse, "cannot delete attached volume; detach first")
	}

	cmd, err := s.issue(ctx, v, protocol.OpRemoveVolume, map[string]any{
		"volumeId": v.ID.String(),
		"name":     v.Name,
	}, &actorID)
	if err != nil {
		return err
	}
	if _, err := s.repo.SetState(ctx, v.ID, StateDeleting, &cmd.ID, nil, nil, ""); err != nil {
		return err
	}
	s.writeAudit(ctx, &v.OrganizationID, &actorID, "volume.delete", "volume", v.ID.String(), meta,
		map[string]any{"name": v.Name, "state": v.State},
		map[string]any{"state": StateDeleting, "commandId": cmd.ID.String()},
	)
	return nil
}

func (s *Service) Update(ctx context.Context, actorID, id uuid.UUID, in UpdateInput, meta AuditMeta) (Volume, error) {
	before, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Volume{}, apierror.NotFoundCode(apierror.CodeVolumeNotFound, "volume not found")
		}
		return Volume{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.ServerUpdate); err != nil {
		return Volume{}, err
	}
	after := before
	if in.MountPath != nil {
		after.MountPath = strings.TrimSpace(*in.MountPath)
	}
	if in.BackupPolicy != nil {
		after.BackupPolicy = in.BackupPolicy
	}
	if in.Labels != nil {
		after.Labels = in.Labels
		after.Labels["deploycore.managed"] = "true"
		after.Labels["deploycore.owner"] = "platform"
		after.Labels["deploycore.organization_id"] = after.OrganizationID.String()
	}
	updated, err := s.repo.Update(ctx, after)
	if err != nil {
		return Volume{}, err
	}
	s.writeAudit(ctx, &updated.OrganizationID, &actorID, "volume.update", "volume", id.String(), meta, nil, map[string]any{
		"mountPath": updated.MountPath,
	})
	return updated, nil
}

func (s *Service) HandleCommandCompletion(ctx context.Context, cmd agentcmd.Command) error {
	switch cmd.Operation {
	case protocol.OpCreateVolume, protocol.OpRemoveVolume, protocol.OpInspectVolume,
		protocol.OpAttachVolume, protocol.OpDetachVolume:
	default:
		return nil
	}
	raw, _ := cmd.Payload["volumeId"].(string)
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		// Soft-deleted volume during REMOVE_VOLUME — ignore.
		return nil
	}
	if v.ServerID != cmd.ServerID {
		return nil
	}

	var dockerName *string
	var usage *int64
	if cmd.Result != nil {
		if n, ok := cmd.Result["dockerName"].(string); ok && strings.TrimSpace(n) != "" {
			n = strings.TrimSpace(n)
			dockerName = &n
		} else if n, ok := cmd.Result["name"].(string); ok && strings.TrimSpace(n) != "" {
			n = strings.TrimSpace(n)
			dockerName = &n
		}
		if u, ok := asInt64(cmd.Result["usageBytes"]); ok {
			usage = &u
		}
	}

	switch cmd.Operation {
	case protocol.OpCreateVolume:
		if cmd.Status == protocol.StatusCompleted {
			_, err = s.repo.SetState(ctx, id, StateReady, &cmd.ID, dockerName, usage, "")
		} else if cmd.Status == protocol.StatusFailed {
			msg := ""
			if cmd.ErrorMessage != nil {
				msg = *cmd.ErrorMessage
			}
			_, err = s.repo.SetState(ctx, id, StateFailed, &cmd.ID, nil, nil, msg)
		}
	case protocol.OpInspectVolume:
		if cmd.Status == protocol.StatusCompleted {
			_, err = s.repo.SetState(ctx, id, v.State, &cmd.ID, dockerName, usage, "")
		}
	case protocol.OpRemoveVolume:
		if cmd.Status == protocol.StatusCompleted {
			err = s.repo.SoftDelete(ctx, id, s.now().UTC())
		} else if cmd.Status == protocol.StatusFailed {
			msg := ""
			if cmd.ErrorMessage != nil {
				msg = *cmd.ErrorMessage
			}
			_, err = s.repo.SetState(ctx, id, StateFailed, &cmd.ID, nil, nil, msg)
		}
	}
	return err
}

func createVolumeCommandPayload(v Volume) map[string]any {
	return map[string]any{
		"name":           v.Name,
		"driver":         v.Driver,
		"labels":         dockerCommandLabels(v.Labels),
		"organizationId": v.OrganizationID.String(),
		"volumeId":       v.ID.String(),
	}
}

// dockerCommandLabels copies string Docker labels. readOnly and other non-string
// values stay on the control-plane volume row and are not sent to the agent.
func dockerCommandLabels(labels map[string]any) map[string]string {
	out := map[string]string{}
	for key, value := range labels {
		if key == "readOnly" {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out[key] = text
	}
	return out
}

func (s *Service) issue(ctx context.Context, v Volume, op string, payload map[string]any, issuedBy *uuid.UUID) (agentcmd.Command, error) {
	if s.commands == nil {
		return agentcmd.Command{}, apierror.Internal("command store not configured")
	}
	now := s.now().UTC()
	reqID := requestid.FromContext(ctx)
	var reqPtr *string
	if reqID != "" {
		reqPtr = &reqID
	}
	return s.commands.Create(ctx, agentcmd.Command{
		OrganizationID: v.OrganizationID,
		ServerID:       v.ServerID,
		Operation:      op,
		SchemaVersion:  protocol.SchemaVersion,
		Payload:        payload,
		IssuedAt:       now,
		ExpiresAt:      now.Add(15 * time.Minute),
		RequestID:      reqPtr,
		IssuedBy:       issuedBy,
	})
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

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
