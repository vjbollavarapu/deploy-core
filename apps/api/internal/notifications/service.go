package notifications

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/security"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/google/uuid"
)

type JobEnqueuer interface {
	Enqueue(ctx context.Context, in jobs.EnqueueInput) (jobs.Job, error)
}

type ServiceConfig struct {
	PlatformKey []byte
	KeyID       string
}

type Service struct {
	repo       Repository
	queue      JobEnqueuer
	authz      *rbac.Authorizer
	audit      *audit.Writer
	log        *slog.Logger
	cfg        ServiceConfig
	deliverers map[string]Deliverer
	now        func() time.Time
}

func NewService(repo Repository, queue JobEnqueuer, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger, cfg ServiceConfig) *Service {
	if cfg.KeyID == "" {
		cfg.KeyID = "platform:v1"
	}
	s := &Service{
		repo: repo, queue: queue, authz: authz, audit: auditWriter, log: log, cfg: cfg,
		deliverers: map[string]Deliverer{},
		now:        time.Now,
	}
	s.RegisterDeliverer(EmailDeliverer{Log: log})
	s.RegisterDeliverer(WebhookDeliverer{})
	for _, t := range ReservedChannelTypes() {
		s.RegisterDeliverer(ReservedDeliverer{ChannelType: t})
	}
	return s
}

func (s *Service) RegisterDeliverer(d Deliverer) {
	s.deliverers[d.Type()] = d
}

func (s *Service) CreateChannel(ctx context.Context, actorID uuid.UUID, in CreateChannelInput, meta AuditMeta) (Channel, error) {
	if err := s.authz.RequirePermission(ctx, actorID, in.OrganizationID, rbac.NotificationManage); err != nil {
		return Channel{}, err
	}
	typ := strings.ToUpper(strings.TrimSpace(in.Type))
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Channel{}, apierror.Validation("name is required", nil)
	}
	if !isKnownChannelType(typ) {
		return Channel{}, apierror.Validation("unsupported channel type", map[string]any{"type": typ})
	}
	if !isEnabledChannelType(typ) {
		return Channel{}, apierror.Validation("channel type is reserved but not enabled yet", map[string]any{
			"type": typ, "supported": EnabledChannelTypes(),
		})
	}
	if err := validateChannelConfig(typ, in.Config); err != nil {
		return Channel{}, err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	var cred *credentialBlob
	if strings.TrimSpace(in.Credential) != "" {
		env, err := crypto.Seal(s.cfg.PlatformKey, s.cfg.KeyID, []byte(in.Credential))
		if err != nil {
			return Channel{}, err
		}
		cred = &credentialBlob{Ciphertext: env.Ciphertext, Nonce: env.Nonce, KeyID: env.KeyID}
	}
	ch, err := s.repo.CreateChannel(ctx, Channel{
		OrganizationID: in.OrganizationID,
		Name:           name,
		Type:           typ,
		Config:         in.Config,
		Enabled:        enabled,
		Status:         ChannelStatusActive,
	}, cred, actorID)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Channel{}, apierror.Conflict("channel name already exists")
		}
		return Channel{}, err
	}
	s.writeAudit(ctx, &ch.OrganizationID, &actorID, "notification.channel.create", "notification_channel", ch.ID.String(), meta, nil, map[string]any{
		"name": ch.Name, "type": ch.Type,
	})
	return ch, nil
}

func (s *Service) ListChannels(ctx context.Context, actorID, orgID uuid.UUID, limit, offset int) ([]Channel, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.NotificationRead); err != nil {
		return nil, 0, err
	}
	return s.repo.ListChannels(ctx, orgID, limit, offset)
}

func (s *Service) GetChannel(ctx context.Context, actorID, id uuid.UUID) (Channel, error) {
	ch, _, err := s.repo.GetChannel(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Channel{}, apierror.NotFound("notification channel not found")
		}
		return Channel{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, ch.OrganizationID, rbac.NotificationRead); err != nil {
		return Channel{}, err
	}
	return ch, nil
}

func (s *Service) UpdateChannel(ctx context.Context, actorID, id uuid.UUID, in UpdateChannelInput, meta AuditMeta) (Channel, error) {
	before, _, err := s.repo.GetChannel(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Channel{}, apierror.NotFound("notification channel not found")
		}
		return Channel{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.NotificationManage); err != nil {
		return Channel{}, err
	}
	after := before
	if in.Name != nil {
		after.Name = strings.TrimSpace(*in.Name)
	}
	if in.Config != nil {
		if err := validateChannelConfig(after.Type, in.Config); err != nil {
			return Channel{}, err
		}
		after.Config = in.Config
	}
	if in.Enabled != nil {
		after.Enabled = *in.Enabled
	}
	if in.Status != nil {
		st := strings.ToUpper(strings.TrimSpace(*in.Status))
		switch st {
		case ChannelStatusActive, ChannelStatusDisabled:
			after.Status = st
		default:
			return Channel{}, apierror.Validation("invalid status", nil)
		}
	}
	var cred *credentialBlob
	if in.Credential != nil && strings.TrimSpace(*in.Credential) != "" {
		env, err := crypto.Seal(s.cfg.PlatformKey, s.cfg.KeyID, []byte(*in.Credential))
		if err != nil {
			return Channel{}, err
		}
		cred = &credentialBlob{Ciphertext: env.Ciphertext, Nonce: env.Nonce, KeyID: env.KeyID}
	}
	updated, err := s.repo.UpdateChannel(ctx, after, cred, in.ClearCredential)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Channel{}, apierror.Conflict("channel name already exists")
		}
		return Channel{}, err
	}
	s.writeAudit(ctx, &updated.OrganizationID, &actorID, "notification.channel.update", "notification_channel", id.String(), meta, nil, map[string]any{
		"name": updated.Name, "enabled": updated.Enabled,
	})
	return updated, nil
}

func (s *Service) DeleteChannel(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) error {
	ch, _, err := s.repo.GetChannel(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFound("notification channel not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, ch.OrganizationID, rbac.NotificationManage); err != nil {
		return err
	}
	if err := s.repo.SoftDeleteChannel(ctx, id, s.now().UTC()); err != nil {
		return err
	}
	s.writeAudit(ctx, &ch.OrganizationID, &actorID, "notification.channel.delete", "notification_channel", id.String(), meta,
		map[string]any{"name": ch.Name}, nil)
	return nil
}

func (s *Service) CreatePolicy(ctx context.Context, actorID uuid.UUID, in CreatePolicyInput, meta AuditMeta) (Policy, error) {
	if err := s.authz.RequirePermission(ctx, actorID, in.OrganizationID, rbac.NotificationManage); err != nil {
		return Policy{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Policy{}, apierror.Validation("name is required", nil)
	}
	events, err := normalizeEvents(in.EventTypes)
	if err != nil {
		return Policy{}, err
	}
	if len(in.ChannelIDs) == 0 {
		return Policy{}, apierror.Validation("channelIds required", nil)
	}
	for _, cid := range in.ChannelIDs {
		ch, _, err := s.repo.GetChannel(ctx, cid)
		if err != nil || ch.OrganizationID != in.OrganizationID {
			return Policy{}, apierror.Validation("channel not found in organization", map[string]any{"channelId": cid.String()})
		}
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	p, err := s.repo.CreatePolicy(ctx, Policy{
		OrganizationID:     in.OrganizationID,
		Name:               name,
		EventTypes:         events,
		ResourceFilters:    in.ResourceFilters,
		EnvironmentFilters: in.EnvironmentFilters,
		ChannelIDs:         in.ChannelIDs,
		Enabled:            enabled,
	}, actorID)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Policy{}, apierror.Conflict("policy name already exists")
		}
		return Policy{}, err
	}
	s.writeAudit(ctx, &p.OrganizationID, &actorID, "notification.policy.create", "notification_policy", p.ID.String(), meta, nil, map[string]any{
		"name": p.Name, "eventTypes": p.EventTypes,
	})
	return p, nil
}

func (s *Service) ListPolicies(ctx context.Context, actorID, orgID uuid.UUID, limit, offset int) ([]Policy, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.NotificationRead); err != nil {
		return nil, 0, err
	}
	return s.repo.ListPolicies(ctx, orgID, limit, offset)
}

func (s *Service) GetPolicy(ctx context.Context, actorID, id uuid.UUID) (Policy, error) {
	p, err := s.repo.GetPolicy(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Policy{}, apierror.NotFound("notification policy not found")
		}
		return Policy{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, p.OrganizationID, rbac.NotificationRead); err != nil {
		return Policy{}, err
	}
	return p, nil
}

func (s *Service) UpdatePolicy(ctx context.Context, actorID, id uuid.UUID, in UpdatePolicyInput, meta AuditMeta) (Policy, error) {
	before, err := s.repo.GetPolicy(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Policy{}, apierror.NotFound("notification policy not found")
		}
		return Policy{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.NotificationManage); err != nil {
		return Policy{}, err
	}
	after := before
	if in.Name != nil {
		after.Name = strings.TrimSpace(*in.Name)
	}
	if in.EventTypes != nil {
		events, err := normalizeEvents(in.EventTypes)
		if err != nil {
			return Policy{}, err
		}
		after.EventTypes = events
	}
	if in.ResourceFilters != nil {
		after.ResourceFilters = in.ResourceFilters
	}
	if in.EnvironmentFilters != nil {
		after.EnvironmentFilters = in.EnvironmentFilters
	}
	if in.ChannelIDs != nil {
		if len(in.ChannelIDs) == 0 {
			return Policy{}, apierror.Validation("channelIds required", nil)
		}
		after.ChannelIDs = in.ChannelIDs
	}
	if in.Enabled != nil {
		after.Enabled = *in.Enabled
	}
	updated, err := s.repo.UpdatePolicy(ctx, after)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Policy{}, apierror.Conflict("policy name already exists")
		}
		return Policy{}, err
	}
	s.writeAudit(ctx, &updated.OrganizationID, &actorID, "notification.policy.update", "notification_policy", id.String(), meta, nil, map[string]any{
		"name": updated.Name, "enabled": updated.Enabled,
	})
	return updated, nil
}

func (s *Service) DeletePolicy(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) error {
	p, err := s.repo.GetPolicy(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFound("notification policy not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, p.OrganizationID, rbac.NotificationManage); err != nil {
		return err
	}
	if err := s.repo.SoftDeletePolicy(ctx, id, s.now().UTC()); err != nil {
		return err
	}
	s.writeAudit(ctx, &p.OrganizationID, &actorID, "notification.policy.delete", "notification_policy", id.String(), meta,
		map[string]any{"name": p.Name}, nil)
	return nil
}

func (s *Service) ListDeliveries(ctx context.Context, actorID, orgID uuid.UUID, limit, offset int) ([]Delivery, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.NotificationRead); err != nil {
		return nil, 0, err
	}
	return s.repo.ListDeliveries(ctx, orgID, limit, offset)
}

// Emit matches policies and enqueues async NOTIFICATION_DELIVERY jobs.
func (s *Service) Emit(ctx context.Context, in EmitInput) (int, error) {
	eventType := strings.ToUpper(strings.TrimSpace(in.EventType))
	if !isKnownEvent(eventType) {
		return 0, apierror.Validation("unknown event type", map[string]any{"eventType": eventType, "known": KnownEvents()})
	}
	policies, err := s.repo.ListEnabledPoliciesForEvent(ctx, in.OrganizationID, eventType)
	if err != nil {
		return 0, err
	}
	payload := sanitizePayload(in.Payload)
	if payload == nil {
		payload = map[string]any{}
	}
	payload["eventType"] = eventType
	enqueued := 0
	for _, p := range policies {
		if !policyMatches(p, in) {
			continue
		}
		for _, channelID := range p.ChannelIDs {
			ch, _, err := s.repo.GetChannel(ctx, channelID)
			if err != nil || !ch.Enabled || ch.Status == ChannelStatusDisabled {
				continue
			}
			pid := p.ID
			d, err := s.repo.CreateDelivery(ctx, Delivery{
				OrganizationID: in.OrganizationID,
				PolicyID:       &pid,
				ChannelID:      channelID,
				EventType:      eventType,
				Payload:        payload,
				Status:         DeliveryPending,
			})
			if err != nil {
				continue
			}
			job, err := s.queue.Enqueue(ctx, jobs.EnqueueInput{
				OrganizationID:      &in.OrganizationID,
				Type:                jobs.TypeNotificationDelivery,
				Payload:             map[string]any{"deliveryId": d.ID.String()},
				MaxAttempts:         5,
				RelatedResourceType: strPtr("notification_delivery"),
				RelatedResourceID:   &d.ID,
			})
			if err != nil {
				d.Status = DeliveryFailed
				d.LastError = err.Error()
				_, _ = s.repo.UpdateDelivery(ctx, d)
				continue
			}
			d.JobID = &job.ID
			d.Status = DeliveryQueued
			_, _ = s.repo.UpdateDelivery(ctx, d)
			enqueued++
		}
	}
	return enqueued, nil
}

// EmitForActor is used by HTTP test endpoints with authz.
func (s *Service) EmitForActor(ctx context.Context, actorID uuid.UUID, in EmitInput, meta AuditMeta) (int, error) {
	if err := s.authz.RequirePermission(ctx, actorID, in.OrganizationID, rbac.NotificationManage); err != nil {
		return 0, err
	}
	n, err := s.Emit(ctx, in)
	if err != nil {
		return 0, err
	}
	s.writeAudit(ctx, &in.OrganizationID, &actorID, "notification.emit", "organization", in.OrganizationID.String(), meta, nil, map[string]any{
		"eventType": in.EventType, "enqueued": n,
	})
	return n, nil
}

func (s *Service) ProcessDeliveryJob(ctx context.Context, job jobs.Job) error {
	raw, _ := job.Payload["deliveryId"].(string)
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return errors.New("deliveryId missing")
	}
	d, err := s.repo.GetDelivery(ctx, id)
	if err != nil {
		return err
	}
	if d.Status == DeliveryDelivered || d.Status == DeliverySkipped {
		return nil
	}
	ch, cred, err := s.repo.GetChannel(ctx, d.ChannelID)
	if err != nil {
		d.Status = DeliveryFailed
		d.LastError = "channel missing"
		_, _ = s.repo.UpdateDelivery(ctx, d)
		return err
	}
	deliverer, ok := s.deliverers[ch.Type]
	if !ok {
		d.Status = DeliveryFailed
		d.LastError = "no deliverer for channel type"
		_, _ = s.repo.UpdateDelivery(ctx, d)
		return errors.New(d.LastError)
	}
	credential := ""
	if cred != nil {
		plain, err := crypto.Open(s.cfg.PlatformKey, crypto.Envelope{
			Ciphertext: cred.Ciphertext, Nonce: cred.Nonce, KeyID: cred.KeyID,
		})
		if err == nil {
			credential = string(plain)
		}
	}
	d.Status = DeliveryDelivering
	d.AttemptCount++
	_, _ = s.repo.UpdateDelivery(ctx, d)

	result, err := deliverer.Deliver(ctx, ch, d.EventType, d.Payload, credential)
	now := s.now().UTC()
	if result.ResponseCode != 0 {
		d.ResponseCode = &result.ResponseCode
	}
	if result.LatencyMs != 0 {
		d.LatencyMs = &result.LatencyMs
	}
	if err != nil {
		d.Status = DeliveryFailed
		d.LastError = err.Error()
		_, _ = s.repo.UpdateDelivery(ctx, d)
		return err
	}
	if result.Skipped {
		d.Status = DeliverySkipped
	} else {
		d.Status = DeliveryDelivered
		d.DeliveredAt = &now
		d.LastError = ""
	}
	_, err = s.repo.UpdateDelivery(ctx, d)
	return err
}

func policyMatches(p Policy, in EmitInput) bool {
	if apps, ok := asUUIDList(p.ResourceFilters["applicationIds"]); ok && len(apps) > 0 {
		if in.ApplicationID == nil || !containsUUID(apps, *in.ApplicationID) {
			return false
		}
	}
	if servers, ok := asUUIDList(p.ResourceFilters["serverIds"]); ok && len(servers) > 0 {
		if in.ServerID == nil || !containsUUID(servers, *in.ServerID) {
			return false
		}
	}
	if envs, ok := asUUIDList(p.EnvironmentFilters["environmentIds"]); ok && len(envs) > 0 {
		if in.EnvironmentID == nil || !containsUUID(envs, *in.EnvironmentID) {
			return false
		}
	}
	return true
}

func validateChannelConfig(typ string, cfg map[string]any) error {
	if cfg == nil {
		cfg = map[string]any{}
	}
	switch typ {
	case ChannelEmail:
		if _, ok := cfg["to"]; !ok {
			return apierror.Validation("email config.to is required", nil)
		}
	case ChannelWebhook:
		url, _ := cfg["url"].(string)
		if strings.TrimSpace(url) == "" {
			return apierror.Validation("webhook config.url is required", nil)
		}
		if err := security.ValidateOutboundURL(url); err != nil {
			return apierror.Validation("webhook config.url: "+err.Error(), nil)
		}
	}
	return nil
}

func normalizeEvents(events []string) ([]string, error) {
	if len(events) == 0 {
		return nil, apierror.Validation("eventTypes required", nil)
	}
	out := make([]string, 0, len(events))
	seen := map[string]bool{}
	for _, e := range events {
		e = strings.ToUpper(strings.TrimSpace(e))
		if !isKnownEvent(e) {
			return nil, apierror.Validation("unknown event type", map[string]any{"eventType": e})
		}
		if seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out, nil
}

func isKnownChannelType(t string) bool {
	for _, x := range append(EnabledChannelTypes(), ReservedChannelTypes()...) {
		if x == t {
			return true
		}
	}
	return false
}

func isEnabledChannelType(t string) bool {
	for _, x := range EnabledChannelTypes() {
		if x == t {
			return true
		}
	}
	return false
}

func isKnownEvent(e string) bool {
	for _, x := range KnownEvents() {
		if x == e {
			return true
		}
	}
	return false
}

func asUUIDList(v any) ([]uuid.UUID, bool) {
	arr, ok := v.([]any)
	if !ok {
		if ss, ok := v.([]string); ok {
			out := make([]uuid.UUID, 0, len(ss))
			for _, s := range ss {
				if id, err := uuid.Parse(s); err == nil {
					out = append(out, id)
				}
			}
			return out, true
		}
		return nil, false
	}
	out := make([]uuid.UUID, 0, len(arr))
	for _, item := range arr {
		s, _ := item.(string)
		if id, err := uuid.Parse(s); err == nil {
			out = append(out, id)
		}
	}
	return out, true
}

func containsUUID(list []uuid.UUID, id uuid.UUID) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

func strPtr(s string) *string { return &s }

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
