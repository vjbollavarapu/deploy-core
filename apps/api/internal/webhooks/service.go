package webhooks

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
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
	HTTPClient  *http.Client
}

type Service struct {
	repo   Repository
	queue  JobEnqueuer
	authz  *rbac.Authorizer
	audit  *audit.Writer
	log    *slog.Logger
	cfg    ServiceConfig
	now    func() time.Time
	client *http.Client
}

func NewService(repo Repository, queue JobEnqueuer, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger, cfg ServiceConfig) *Service {
	if cfg.KeyID == "" {
		cfg.KeyID = "platform:v1"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Service{repo: repo, queue: queue, authz: authz, audit: auditWriter, log: log, cfg: cfg, now: time.Now, client: client}
}

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, in CreateInput, meta AuditMeta) (Webhook, error) {
	if err := s.authz.RequirePermission(ctx, actorID, in.OrganizationID, rbac.WebhookManage); err != nil {
		return Webhook{}, err
	}
	name := strings.TrimSpace(in.Name)
	url := strings.TrimSpace(in.URL)
	if name == "" {
		return Webhook{}, apierror.Validation("name is required", nil)
	}
	if err := validateURL(url); err != nil {
		return Webhook{}, err
	}
	events, err := normalizeEvents(in.Events)
	if err != nil {
		return Webhook{}, err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	threshold := 5
	if in.FailureThreshold != nil {
		threshold = *in.FailureThreshold
		if threshold < 1 || threshold > 100 {
			return Webhook{}, apierror.Validation("failureThreshold must be 1-100", nil)
		}
	}
	plain := strings.TrimSpace(in.Secret)
	if plain == "" {
		plain, err = randomSecret(32)
		if err != nil {
			return Webhook{}, err
		}
	}
	env, err := crypto.Seal(s.cfg.PlatformKey, s.cfg.KeyID, []byte(plain))
	if err != nil {
		return Webhook{}, err
	}
	wh, err := s.repo.Create(ctx, Webhook{
		OrganizationID:   in.OrganizationID,
		Name:             name,
		URL:              url,
		Events:           events,
		Enabled:          enabled,
		Status:           StatusActive,
		FailureThreshold: threshold,
	}, secretBlob{Ciphertext: env.Ciphertext, Nonce: env.Nonce, KeyID: env.KeyID}, actorID)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Webhook{}, apierror.Conflict("webhook name already exists")
		}
		return Webhook{}, err
	}
	wh.SecretPlain = &plain
	s.writeAudit(ctx, &wh.OrganizationID, &actorID, "webhook.create", "outgoing_webhook", wh.ID.String(), meta, nil, map[string]any{
		"name": wh.Name, "url": wh.URL, "events": wh.Events,
	})
	return wh, nil
}

func (s *Service) List(ctx context.Context, actorID, orgID uuid.UUID, limit, offset int) ([]Webhook, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.WebhookRead); err != nil {
		return nil, 0, err
	}
	return s.repo.List(ctx, orgID, limit, offset)
}

func (s *Service) Get(ctx context.Context, actorID, id uuid.UUID) (Webhook, error) {
	wh, _, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Webhook{}, apierror.NotFound("webhook not found")
		}
		return Webhook{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, wh.OrganizationID, rbac.WebhookRead); err != nil {
		return Webhook{}, err
	}
	return wh, nil
}

func (s *Service) Update(ctx context.Context, actorID, id uuid.UUID, in UpdateInput, meta AuditMeta) (Webhook, error) {
	before, _, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Webhook{}, apierror.NotFound("webhook not found")
		}
		return Webhook{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.WebhookManage); err != nil {
		return Webhook{}, err
	}
	after := before
	if in.Name != nil {
		after.Name = strings.TrimSpace(*in.Name)
	}
	if in.URL != nil {
		if err := validateURL(strings.TrimSpace(*in.URL)); err != nil {
			return Webhook{}, err
		}
		after.URL = strings.TrimSpace(*in.URL)
	}
	if in.Events != nil {
		events, err := normalizeEvents(in.Events)
		if err != nil {
			return Webhook{}, err
		}
		after.Events = events
	}
	if in.FailureThreshold != nil {
		if *in.FailureThreshold < 1 || *in.FailureThreshold > 100 {
			return Webhook{}, apierror.Validation("failureThreshold must be 1-100", nil)
		}
		after.FailureThreshold = *in.FailureThreshold
	}
	if in.Enabled != nil {
		after.Enabled = *in.Enabled
		if after.Enabled {
			after.Status = StatusActive
			after.DisabledAt = nil
			after.ConsecutiveFailures = 0
			after.LastError = ""
		} else {
			after.Status = StatusDisabled
			now := s.now().UTC()
			after.DisabledAt = &now
		}
	}
	var secret *secretBlob
	var plain *string
	if in.RotateSecret {
		p, err := randomSecret(32)
		if err != nil {
			return Webhook{}, err
		}
		env, err := crypto.Seal(s.cfg.PlatformKey, s.cfg.KeyID, []byte(p))
		if err != nil {
			return Webhook{}, err
		}
		secret = &secretBlob{Ciphertext: env.Ciphertext, Nonce: env.Nonce, KeyID: env.KeyID}
		plain = &p
	}
	updated, err := s.repo.Update(ctx, after, secret)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Webhook{}, apierror.Conflict("webhook name already exists")
		}
		return Webhook{}, err
	}
	updated.SecretPlain = plain
	s.writeAudit(ctx, &updated.OrganizationID, &actorID, "webhook.update", "outgoing_webhook", id.String(), meta, nil, map[string]any{
		"name": updated.Name, "enabled": updated.Enabled,
	})
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) error {
	wh, _, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFound("webhook not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, wh.OrganizationID, rbac.WebhookManage); err != nil {
		return err
	}
	if err := s.repo.SoftDelete(ctx, id, s.now().UTC()); err != nil {
		return err
	}
	s.writeAudit(ctx, &wh.OrganizationID, &actorID, "webhook.delete", "outgoing_webhook", id.String(), meta,
		map[string]any{"name": wh.Name}, nil)
	return nil
}

func (s *Service) ListDeliveries(ctx context.Context, actorID, orgID uuid.UUID, webhookID *uuid.UUID, limit, offset int) ([]Delivery, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.WebhookRead); err != nil {
		return nil, 0, err
	}
	return s.repo.ListDeliveries(ctx, orgID, webhookID, limit, offset)
}

// Emit matches enabled webhooks subscribed to the event and enqueues WEBHOOK_DELIVERY jobs.
func (s *Service) Emit(ctx context.Context, in EmitInput) (int, error) {
	eventType := strings.TrimSpace(in.EventType)
	if !isKnownEvent(eventType) {
		return 0, apierror.Validation("unknown event type", map[string]any{"eventType": eventType, "known": KnownEvents()})
	}
	hooks, err := s.repo.ListEnabledForEvent(ctx, in.OrganizationID, eventType)
	if err != nil {
		return 0, err
	}
	payload := sanitizePayload(in.Payload)
	if payload == nil {
		payload = map[string]any{}
	}
	payload["event"] = eventType
	enqueued := 0
	for _, wh := range hooks {
		d, err := s.repo.CreateDelivery(ctx, Delivery{
			OrganizationID: in.OrganizationID,
			WebhookID:      wh.ID,
			EventType:      eventType,
			Payload:        payload,
			Status:         DeliveryPending,
		})
		if err != nil {
			continue
		}
		job, err := s.queue.Enqueue(ctx, jobs.EnqueueInput{
			OrganizationID:      &in.OrganizationID,
			Type:                jobs.TypeWebhookDelivery,
			Payload:             map[string]any{"deliveryId": d.ID.String()},
			MaxAttempts:         8,
			RelatedResourceType: strPtr("outgoing_webhook_delivery"),
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
	return enqueued, nil
}

func (s *Service) EmitForActor(ctx context.Context, actorID uuid.UUID, in EmitInput, meta AuditMeta) (int, error) {
	if err := s.authz.RequirePermission(ctx, actorID, in.OrganizationID, rbac.WebhookManage); err != nil {
		return 0, err
	}
	n, err := s.Emit(ctx, in)
	if err != nil {
		return 0, err
	}
	s.writeAudit(ctx, &in.OrganizationID, &actorID, "webhook.emit", "organization", in.OrganizationID.String(), meta, nil, map[string]any{
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
	wh, secret, err := s.repo.Get(ctx, d.WebhookID)
	if err != nil {
		d.Status = DeliveryFailed
		d.LastError = "webhook missing"
		_, _ = s.repo.UpdateDelivery(ctx, d)
		return err
	}
	if !wh.Enabled || wh.Status == StatusDisabled {
		d.Status = DeliverySkipped
		d.LastError = "webhook disabled"
		_, _ = s.repo.UpdateDelivery(ctx, d)
		return nil
	}
	plain, err := crypto.Open(s.cfg.PlatformKey, crypto.Envelope{
		Ciphertext: secret.Ciphertext, Nonce: secret.Nonce, KeyID: secret.KeyID,
	})
	if err != nil {
		d.Status = DeliveryFailed
		d.LastError = "secret unwrap failed"
		_, _ = s.repo.UpdateDelivery(ctx, d)
		return err
	}

	d.Status = DeliveryDelivering
	d.AttemptCount++
	_, _ = s.repo.UpdateDelivery(ctx, d)

	result, err := postSigned(ctx, s.client, wh.URL, d.EventType, d.ID, d.Payload, plain)
	if result.ResponseCode != 0 {
		d.ResponseCode = &result.ResponseCode
	}
	if result.LatencyMs != 0 {
		d.LatencyMs = &result.LatencyMs
	}
	now := s.now().UTC()
	if err != nil {
		d.Status = DeliveryFailed
		d.LastError = err.Error()
		_, _ = s.repo.UpdateDelivery(ctx, d)
		// Count toward disable threshold only when job retries are exhausted.
		if job.AttemptCount >= job.MaxAttempts {
			disable := wh.ConsecutiveFailures+1 >= wh.FailureThreshold
			_ = s.repo.RecordFailure(ctx, wh.ID, err.Error(), disable, now)
			if disable {
				s.writeAudit(ctx, &wh.OrganizationID, nil, "webhook.auto_disable", "outgoing_webhook", wh.ID.String(), AuditMeta{}, nil, map[string]any{
					"consecutiveFailures": wh.ConsecutiveFailures + 1,
					"failureThreshold":    wh.FailureThreshold,
					"lastError":           err.Error(),
				})
				if s.log != nil {
					s.log.Warn("outgoing webhook auto-disabled after repeated failures",
						slog.String("webhookId", wh.ID.String()),
						slog.String("error", err.Error()),
					)
				}
			}
		} else {
			_ = s.repo.MarkFailing(ctx, wh.ID, err.Error())
		}
		return err
	}
	d.Status = DeliveryDelivered
	d.DeliveredAt = &now
	d.LastError = ""
	if _, err := s.repo.UpdateDelivery(ctx, d); err != nil {
		return err
	}
	return s.repo.RecordSuccess(ctx, wh.ID)
}

func validateURL(raw string) error {
	if err := security.ValidateOutboundURL(raw); err != nil {
		return apierror.Validation(err.Error(), nil)
	}
	return nil
}

func normalizeEvents(events []string) ([]string, error) {
	if len(events) == 0 {
		return nil, apierror.Validation("events required", nil)
	}
	out := make([]string, 0, len(events))
	seen := map[string]bool{}
	for _, e := range events {
		e = strings.TrimSpace(e)
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

func isKnownEvent(e string) bool {
	for _, x := range KnownEvents() {
		if x == e {
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
		entry.ActorType = "system"
	}
	if err := s.audit.Write(ctx, entry); err != nil && s.log != nil {
		s.log.Error("audit write failed", slog.String("error", err.Error()), slog.String("action", action))
	}
}
