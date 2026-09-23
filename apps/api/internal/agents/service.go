package agents

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/notifications"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/webhooks"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/deploycore/deploy-core/apps/api/pkg/validation"
	"github.com/google/uuid"
)

type AuditMeta struct {
	IP        string
	UserAgent string
}

type ServiceConfig struct {
	RegistrationTokenTTL time.Duration
	HeartbeatRetainCount int
}

type Service struct {
	repo     Repository
	authz    *rbac.Authorizer
	audit    *audit.Writer
	log      *slog.Logger
	cfg      ServiceConfig
	now      func() time.Time
	metrics  MetricsRecorder
	notify   NotificationEmitter
	webhooks WebhookEmitter
}

// MetricsRecorder optionally mirrors heartbeat fields into current/summary snapshots (B21).
type MetricsRecorder interface {
	UpsertFromHeartbeat(ctx context.Context, orgID, serverID uuid.UUID, fields HeartbeatMetricFields, at time.Time) error
}

type NotificationEmitter interface {
	Emit(ctx context.Context, in notifications.EmitInput) (int, error)
}

type WebhookEmitter interface {
	Emit(ctx context.Context, in webhooks.EmitInput) (int, error)
}

type HeartbeatMetricFields struct {
	CPUPercent      *float64
	MemoryUsedBytes *int64
	DiskUsedBytes   *int64
	Load1           *float64
	ContainerCount  *int
	UptimeSeconds   *int64
}

func NewService(repo Repository, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger, cfg ServiceConfig) *Service {
	if cfg.RegistrationTokenTTL <= 0 {
		cfg.RegistrationTokenTTL = 15 * time.Minute
	}
	if cfg.HeartbeatRetainCount <= 0 {
		cfg.HeartbeatRetainCount = 50
	}
	return &Service{repo: repo, authz: authz, audit: auditWriter, log: log, cfg: cfg, now: time.Now}
}

func (s *Service) WithMetrics(m MetricsRecorder) *Service {
	s.metrics = m
	return s
}

func (s *Service) WithNotifier(n NotificationEmitter) *Service {
	s.notify = n
	return s
}

func (s *Service) WithWebhooks(w WebhookEmitter) *Service {
	s.webhooks = w
	return s
}

func (s *Service) IssueRegistrationToken(ctx context.Context, actorID, serverID uuid.UUID, meta AuditMeta) (RegistrationTokenResult, error) {
	orgID, status, _, err := s.repo.GetServerMeta(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return RegistrationTokenResult{}, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return RegistrationTokenResult{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ServerUpdate); err != nil {
		return RegistrationTokenResult{}, err
	}
	if status == "DISABLED" {
		return RegistrationTokenResult{}, apierror.Conflict("cannot issue registration token for disabled server")
	}

	raw, err := crypto.RandomURLToken(32)
	if err != nil {
		return RegistrationTokenResult{}, apierror.Internal("could not issue registration token")
	}
	expires := s.now().UTC().Add(s.cfg.RegistrationTokenTTL)
	agent, err := s.repo.UpsertRegistrationToken(ctx, orgID, serverID, crypto.HashTokenSHA256(raw), expires)
	if err != nil {
		return RegistrationTokenResult{}, err
	}
	s.writeAudit(ctx, &orgID, &actorID, "server.agent.registration_token.issue", "server_agent", agent.ID.String(), meta, nil, map[string]any{
		"serverId": serverID.String(), "expiresAt": expires.Format(time.RFC3339Nano),
	})
	return RegistrationTokenResult{
		ServerID:  serverID,
		AgentID:   agent.ID,
		Token:     raw,
		ExpiresAt: expires,
	}, nil
}

func (s *Service) RevokeRegistrationToken(ctx context.Context, actorID, serverID uuid.UUID, meta AuditMeta) error {
	orgID, _, _, err := s.repo.GetServerMeta(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ServerUpdate); err != nil {
		return err
	}
	if err := s.repo.RevokeRegistrationToken(ctx, serverID, s.now().UTC()); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFound("no open registration token")
		}
		return err
	}
	s.writeAudit(ctx, &orgID, &actorID, "server.agent.registration_token.revoke", "server", serverID.String(), meta, nil, nil)
	return nil
}

func (s *Service) RevokeAgentCredential(ctx context.Context, actorID, serverID uuid.UUID, meta AuditMeta) error {
	orgID, _, _, err := s.repo.GetServerMeta(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ServerUpdate); err != nil {
		return err
	}
	if err := s.repo.RevokeAgentCredential(ctx, serverID, s.now().UTC()); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFound("no active agent credential")
		}
		return err
	}
	s.writeAudit(ctx, &orgID, &actorID, "server.agent.credential.revoke", "server", serverID.String(), meta, nil, nil)
	return nil
}

func (s *Service) Register(ctx context.Context, rawToken, agentVersion string, meta AuditMeta) (RegisterResult, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return RegisterResult{}, apierror.Validation("invalid registration token", map[string]any{
			"fields": validation.Errors{{Field: "registrationToken", Message: "is required"}},
		})
	}
	agentVersion = strings.TrimSpace(agentVersion)

	rec, err := s.repo.GetByRegistrationTokenHash(ctx, crypto.HashTokenSHA256(rawToken))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return RegisterResult{}, apierror.Unauthorized("invalid or expired registration token")
		}
		return RegisterResult{}, err
	}
	now := s.now().UTC()
	if rec.RegistrationUsedAt != nil || rec.RegistrationRevokedAt != nil {
		return RegisterResult{}, apierror.Unauthorized("invalid or expired registration token")
	}
	if rec.RegistrationExpiresAt == nil || rec.RegistrationExpiresAt.Before(now) {
		return RegisterResult{}, apierror.Unauthorized("invalid or expired registration token")
	}

	_, status, _, err := s.repo.GetServerMeta(ctx, rec.ServerID)
	if err != nil {
		return RegisterResult{}, apierror.Unauthorized("invalid or expired registration token")
	}
	if status == "DISABLED" {
		return RegisterResult{}, apierror.Forbidden("server is disabled")
	}

	cred, err := crypto.RandomURLToken(48)
	if err != nil {
		return RegisterResult{}, apierror.Internal("could not create agent credential")
	}
	agent, err := s.repo.CompleteRegistration(ctx, rec.ID, crypto.HashTokenSHA256(cred), agentVersion, now)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return RegisterResult{}, apierror.Unauthorized("invalid or expired registration token")
		}
		return RegisterResult{}, err
	}

	s.writeAudit(ctx, &agent.OrganizationID, nil, "server.agent.register", "server_agent", agent.ID.String(), meta, nil, map[string]any{
		"serverId": agent.ServerID.String(), "agentVersion": agentVersion,
	})
	return RegisterResult{AgentID: agent.ID, ServerID: agent.ServerID, Credential: cred}, nil
}

func (s *Service) AuthenticateCredential(ctx context.Context, rawCredential string) (Agent, error) {
	rawCredential = strings.TrimSpace(rawCredential)
	if rawCredential == "" {
		return Agent{}, apierror.Unauthorized("not authenticated")
	}
	rec, err := s.repo.GetByCredentialHash(ctx, crypto.HashTokenSHA256(rawCredential))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Agent{}, apierror.Unauthorized("not authenticated")
		}
		return Agent{}, err
	}
	if rec.Status != AgentStatusActive {
		return Agent{}, apierror.Unauthorized("not authenticated")
	}
	return rec.Agent, nil
}

func (s *Service) ExpireStaleHeartbeats(ctx context.Context, ttl time.Duration) (int, error) {
	if ttl <= 0 {
		ttl = 90 * time.Second
	}
	cutoff := s.now().UTC().Add(-ttl)
	expired, err := s.repo.MarkHeartbeatExpired(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	for _, e := range expired {
		s.emitServerEvent(ctx, e.OrganizationID, e.ID, notifications.EventServerOffline, e.PreviousStatus, "OFFLINE")
	}
	return len(expired), nil
}

func (s *Service) Heartbeat(ctx context.Context, agent Agent, in HeartbeatInput) error {
	now := s.now().UTC()
	at := now
	if in.Timestamp != nil && !in.Timestamp.After(now.Add(2*time.Minute)) {
		at = in.Timestamp.UTC()
	}

	_, status, maintenance, err := s.repo.GetServerMeta(ctx, agent.ServerID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return err
	}
	if status == "DISABLED" {
		return apierror.Forbidden("server is disabled")
	}

	derived := deriveStatus(in, maintenance, status)
	if err := s.repo.TouchAgent(ctx, agent.ID, in.AgentVersion, at); err != nil {
		return err
	}
	if err := s.repo.ApplyServerHeartbeat(ctx, agent.ServerID, at, derived, in.DockerVersion); err != nil {
		return err
	}
	if err := s.repo.InsertHeartbeat(ctx, agent.OrganizationID, agent.ServerID, agent.ID, in, at); err != nil {
		return err
	}
	_ = s.repo.PruneHeartbeats(ctx, agent.ServerID, s.cfg.HeartbeatRetainCount)
	if s.metrics != nil {
		if err := s.metrics.UpsertFromHeartbeat(ctx, agent.OrganizationID, agent.ServerID, HeartbeatMetricFields{
			CPUPercent:      in.CPUPercent,
			MemoryUsedBytes: in.MemoryUsedBytes,
			DiskUsedBytes:   in.DiskUsedBytes,
			Load1:           in.Load1,
			ContainerCount:  in.ContainerCount,
			UptimeSeconds:   in.UptimeSeconds,
		}, at); err != nil && s.log != nil {
			s.log.Warn("metrics snapshot from heartbeat failed", slog.String("error", err.Error()))
		}
	}
	if s.notify != nil && derived != status {
		switch derived {
		case "DEGRADED":
			s.emitServerEvent(ctx, agent.OrganizationID, agent.ServerID, notifications.EventServerDegraded, status, derived)
		case "OFFLINE":
			s.emitServerEvent(ctx, agent.OrganizationID, agent.ServerID, notifications.EventServerOffline, status, derived)
		}
	}
	return nil
}

func (s *Service) emitServerEvent(ctx context.Context, orgID, serverID uuid.UUID, eventType, from, to string) {
	sid := serverID
	if s.notify != nil {
		if _, err := s.notify.Emit(ctx, notifications.EmitInput{
			OrganizationID: orgID,
			EventType:      eventType,
			ServerID:       &sid,
			ResourceType:   "server",
			ResourceID:     &sid,
			Payload: map[string]any{
				"serverId":   serverID.String(),
				"fromStatus": from,
				"toStatus":   to,
			},
		}); err != nil && s.log != nil {
			s.log.Warn("server notification emit failed", slog.String("error", err.Error()), slog.String("eventType", eventType))
		}
	}
	if s.webhooks != nil && eventType == notifications.EventServerOffline {
		if _, err := s.webhooks.Emit(ctx, webhooks.EmitInput{
			OrganizationID: orgID,
			EventType:      webhooks.EventServerOffline,
			ServerID:       &sid,
			ResourceType:   "server",
			ResourceID:     &sid,
			Payload: map[string]any{
				"serverId":   serverID.String(),
				"fromStatus": from,
				"toStatus":   to,
			},
		}); err != nil && s.log != nil {
			s.log.Warn("server webhook emit failed", slog.String("error", err.Error()))
		}
	}
}

func deriveStatus(in HeartbeatInput, maintenance bool, current string) string {
	if maintenance || current == "MAINTENANCE" {
		return "MAINTENANCE"
	}
	if current == "DISABLED" {
		return "DISABLED"
	}
	docker := strings.ToLower(strings.TrimSpace(in.DockerStatus))
	// Accept agent wire values ("ONLINE") and protocol examples ("running"/"ok"/"healthy").
	switch docker {
	case "", "ok", "running", "healthy", "online":
		if in.CPUPercent != nil && *in.CPUPercent >= 95 {
			return "DEGRADED"
		}
		return "ONLINE"
	case "degraded", "warn", "warning":
		return "DEGRADED"
	default:
		// offline / error / unavailable / unknown → agent reachable but not fully healthy
		return "DEGRADED"
	}
}

func (s *Service) writeAudit(ctx context.Context, orgID, actorID *uuid.UUID, action, resourceType, resourceID string, meta AuditMeta, before, after map[string]any) {
	if s.audit == nil {
		return
	}
	entry := audit.Entry{
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
	}
	if actorID == nil {
		entry.ActorType = "agent"
	}
	if err := s.audit.Write(ctx, entry); err != nil {
		s.log.Error("audit write failed", slog.String("error", err.Error()), slog.String("action", action))
	}
}
