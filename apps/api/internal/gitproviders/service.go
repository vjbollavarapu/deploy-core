package gitproviders

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
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

// DeploymentQueuer creates queued deployments for matching auto-deploy applications.
type DeploymentQueuer interface {
	CountActive(ctx context.Context, appID uuid.UUID) (int64, error)
	CreateQueued(ctx context.Context, orgID, appID, envID uuid.UUID, serverID *uuid.UUID, trigger string, idempotencyKey, correlationID *string, requestID string, createdBy uuid.UUID, now time.Time) (deployments.Deployment, error)
}

type Service struct {
	repo    RepositoryStore
	deploys DeploymentQueuer
	authz   *rbac.Authorizer
	audit   *audit.Writer
	log     *slog.Logger
	cfg     ServiceConfig
	now     func() time.Time
}

func NewService(repo RepositoryStore, deploys DeploymentQueuer, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger, cfg ServiceConfig) *Service {
	if cfg.KeyID == "" {
		cfg.KeyID = "platform:v1"
	}
	return &Service{repo: repo, deploys: deploys, authz: authz, audit: auditWriter, log: log, cfg: cfg, now: time.Now}
}

func (s *Service) CreateConnection(ctx context.Context, actorID uuid.UUID, in CreateConnectionInput, meta AuditMeta) (Connection, error) {
	if err := s.authz.RequirePermission(ctx, actorID, in.OrganizationID, rbac.GitConnectionManage); err != nil {
		return Connection{}, err
	}
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if _, err := Lookup(provider); err != nil {
		return Connection{}, apierror.Validation("unsupported provider", map[string]any{"provider": provider})
	}
	token := strings.TrimSpace(in.AccessToken)
	if token == "" {
		return Connection{}, apierror.Validation("accessToken is required", nil)
	}
	credEnv, err := crypto.Seal(s.cfg.PlatformKey, s.cfg.KeyID, []byte(token))
	if err != nil {
		return Connection{}, err
	}
	webhookPlain := ""
	if in.WebhookSecret != nil && strings.TrimSpace(*in.WebhookSecret) != "" {
		webhookPlain = strings.TrimSpace(*in.WebhookSecret)
	} else {
		webhookPlain, err = randomSecret(32)
		if err != nil {
			return Connection{}, err
		}
	}
	whEnv, err := crypto.Seal(s.cfg.PlatformKey, s.cfg.KeyID, []byte(webhookPlain))
	if err != nil {
		return Connection{}, err
	}
	hash := sha256Hex(webhookPlain)
	sec := connectionSecrets{
		CredentialCiphertext: credEnv.Ciphertext,
		CredentialNonce:      credEnv.Nonce,
		CredentialKeyID:      credEnv.KeyID,
		WebhookCiphertext:    whEnv.Ciphertext,
		WebhookNonce:         whEnv.Nonce,
		WebhookKeyID:         whEnv.KeyID,
		WebhookSecretHash:    &hash,
	}
	c, err := s.repo.CreateConnection(ctx, Connection{
		OrganizationID: in.OrganizationID,
		Provider:       provider,
		AccountLogin:   strings.TrimSpace(in.AccountLogin),
		DisplayName:    strings.TrimSpace(in.DisplayName),
		Status:         StatusActive,
		Metadata:       in.Metadata,
	}, sec, actorID)
	if err != nil {
		return Connection{}, err
	}
	c.WebhookSecretPlain = &webhookPlain
	s.writeAudit(ctx, &c.OrganizationID, &actorID, "git_connection.create", "git_connection", c.ID.String(), meta, nil, map[string]any{
		"provider": c.Provider, "accountLogin": c.AccountLogin,
	})
	return c, nil
}

func (s *Service) ListConnections(ctx context.Context, actorID, orgID uuid.UUID, limit, offset int) ([]Connection, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.GitConnectionRead); err != nil {
		return nil, 0, err
	}
	return s.repo.ListConnections(ctx, orgID, limit, offset)
}

func (s *Service) GetConnection(ctx context.Context, actorID, id uuid.UUID) (Connection, error) {
	c, _, err := s.repo.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Connection{}, apierror.NotFound("git connection not found")
		}
		return Connection{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, c.OrganizationID, rbac.GitConnectionRead); err != nil {
		return Connection{}, err
	}
	return c, nil
}

func (s *Service) UpdateConnection(ctx context.Context, actorID, id uuid.UUID, in UpdateConnectionInput, meta AuditMeta) (Connection, error) {
	before, _, err := s.repo.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Connection{}, apierror.NotFound("git connection not found")
		}
		return Connection{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, before.OrganizationID, rbac.GitConnectionManage); err != nil {
		return Connection{}, err
	}
	after := before
	if in.AccountLogin != nil {
		after.AccountLogin = strings.TrimSpace(*in.AccountLogin)
	}
	if in.DisplayName != nil {
		after.DisplayName = strings.TrimSpace(*in.DisplayName)
	}
	if in.Status != nil {
		st := strings.ToLower(strings.TrimSpace(*in.Status))
		switch st {
		case StatusActive, StatusDisabled, StatusError, StatusRevoked:
			after.Status = st
		default:
			return Connection{}, apierror.Validation("invalid status", nil)
		}
	}
	if in.Metadata != nil {
		after.Metadata = in.Metadata
	}

	var sec *connectionSecrets
	var plainWebhook *string
	if (in.AccessToken != nil && strings.TrimSpace(*in.AccessToken) != "") ||
		(in.WebhookSecret != nil && strings.TrimSpace(*in.WebhookSecret) != "") {
		sec = &connectionSecrets{}
		if in.AccessToken != nil && strings.TrimSpace(*in.AccessToken) != "" {
			env, err := crypto.Seal(s.cfg.PlatformKey, s.cfg.KeyID, []byte(strings.TrimSpace(*in.AccessToken)))
			if err != nil {
				return Connection{}, err
			}
			sec.CredentialCiphertext = env.Ciphertext
			sec.CredentialNonce = env.Nonce
			sec.CredentialKeyID = env.KeyID
		}
		if in.WebhookSecret != nil && strings.TrimSpace(*in.WebhookSecret) != "" {
			plain := strings.TrimSpace(*in.WebhookSecret)
			env, err := crypto.Seal(s.cfg.PlatformKey, s.cfg.KeyID, []byte(plain))
			if err != nil {
				return Connection{}, err
			}
			hash := sha256Hex(plain)
			sec.WebhookCiphertext = env.Ciphertext
			sec.WebhookNonce = env.Nonce
			sec.WebhookKeyID = env.KeyID
			sec.WebhookSecretHash = &hash
			plainWebhook = &plain
		}
	}

	updated, err := s.repo.UpdateConnection(ctx, id, after, sec)
	if err != nil {
		return Connection{}, err
	}
	updated.WebhookSecretPlain = plainWebhook
	s.writeAudit(ctx, &updated.OrganizationID, &actorID, "git_connection.update", "git_connection", id.String(), meta,
		map[string]any{"status": before.Status}, map[string]any{"status": updated.Status},
	)
	return updated, nil
}

func (s *Service) DeleteConnection(ctx context.Context, actorID, id uuid.UUID, meta AuditMeta) error {
	c, _, err := s.repo.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFound("git connection not found")
		}
		return err
	}
	if err := s.authz.RequirePermission(ctx, actorID, c.OrganizationID, rbac.GitConnectionManage); err != nil {
		return err
	}
	if err := s.repo.SoftDeleteConnection(ctx, id, s.now().UTC()); err != nil {
		return err
	}
	s.writeAudit(ctx, &c.OrganizationID, &actorID, "git_connection.delete", "git_connection", id.String(), meta,
		map[string]any{"provider": c.Provider}, nil,
	)
	return nil
}

func (s *Service) SyncConnection(ctx context.Context, actorID, id uuid.UUID, repos []UpsertRepositoryInput, meta AuditMeta) (Connection, []Repository, error) {
	c, _, err := s.repo.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Connection{}, nil, apierror.NotFound("git connection not found")
		}
		return Connection{}, nil, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, c.OrganizationID, rbac.GitConnectionManage); err != nil {
		return Connection{}, nil, err
	}
	now := s.now().UTC()
	var out []Repository
	for _, in := range repos {
		in.FullName = strings.TrimSpace(in.FullName)
		if in.FullName == "" {
			continue
		}
		if in.DefaultBranch == "" {
			in.DefaultBranch = "main"
		}
		repo, err := s.repo.UpsertRepository(ctx, c.OrganizationID, c.ID, in, now)
		if err != nil {
			return Connection{}, nil, err
		}
		out = append(out, repo)
	}
	if err := s.repo.MarkSynced(ctx, id, now); err != nil {
		return Connection{}, nil, err
	}
	c.LastSyncAt = &now
	s.writeAudit(ctx, &c.OrganizationID, &actorID, "git_connection.sync", "git_connection", id.String(), meta, nil, map[string]any{
		"repositoryCount": len(out),
	})
	return c, out, nil
}

func (s *Service) ListRepositories(ctx context.Context, actorID, connectionID uuid.UUID, limit, offset int) ([]Repository, int64, error) {
	c, _, err := s.repo.GetConnection(ctx, connectionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, 0, apierror.NotFound("git connection not found")
		}
		return nil, 0, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, c.OrganizationID, rbac.GitConnectionRead); err != nil {
		return nil, 0, err
	}
	return s.repo.ListRepositories(ctx, connectionID, limit, offset)
}

type WebhookResult struct {
	Status        string
	Duplicate     bool
	Ignored       bool
	DeploymentIDs []uuid.UUID
	Message       string
}

// HandleWebhook verifies the provider signature, deduplicates delivery IDs, and
// optionally queues deployments for matching auto-deploy applications.
func (s *Service) HandleWebhook(ctx context.Context, connectionID uuid.UUID, headers map[string][]string, body []byte) (WebhookResult, error) {
	c, sec, err := s.repo.GetConnection(ctx, connectionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return WebhookResult{}, apierror.NotFound("git connection not found")
		}
		return WebhookResult{}, err
	}
	if c.Status != StatusActive {
		return WebhookResult{Status: DeliveryIgnored, Ignored: true, Message: "connection not active"}, nil
	}
	provider, err := Lookup(c.Provider)
	if err != nil {
		return WebhookResult{}, err
	}
	if len(sec.WebhookCiphertext) == 0 {
		return WebhookResult{}, apierror.New(401, "WEBHOOK_SECRET_MISSING", "webhook secret not configured")
	}
	secret, err := crypto.Open(s.cfg.PlatformKey, crypto.Envelope{
		KeyID: sec.WebhookKeyID, Nonce: sec.WebhookNonce, Ciphertext: sec.WebhookCiphertext,
	})
	if err != nil {
		return WebhookResult{}, err
	}
	hdr := http.Header(headers)
	if err := provider.VerifyWebhook(hdr, body, secret); err != nil {
		return WebhookResult{}, apierror.New(401, "WEBHOOK_SIGNATURE_INVALID", "invalid webhook signature")
	}

	event, err := provider.ParsePushEvent(hdr, body)
	if errors.Is(err, ErrIgnoredEvent) {
		delivery, ierr := s.repo.InsertDelivery(ctx, WebhookDelivery{
			OrganizationID: c.OrganizationID,
			ConnectionID:   c.ID,
			Provider:       c.Provider,
			DeliveryID:     event.DeliveryID,
			EventType:      event.EventType,
			Status:         DeliveryIgnored,
		})
		if errors.Is(ierr, ErrConflict) {
			return WebhookResult{Status: DeliveryDuplicate, Duplicate: true, Message: "duplicate delivery"}, nil
		}
		if ierr != nil {
			return WebhookResult{}, ierr
		}
		_ = delivery
		return WebhookResult{Status: DeliveryIgnored, Ignored: true, Message: "event ignored"}, nil
	}
	if err != nil {
		return WebhookResult{}, apierror.Validation("invalid webhook payload", nil)
	}

	delivery, err := s.repo.InsertDelivery(ctx, WebhookDelivery{
		OrganizationID:     c.OrganizationID,
		ConnectionID:       c.ID,
		Provider:           c.Provider,
		DeliveryID:         event.DeliveryID,
		EventType:          event.EventType,
		RepositoryFullName: event.RepositoryFullName,
		Branch:             event.Branch,
		CommitSHA:          event.CommitSHA,
		Status:             DeliveryReceived,
	})
	if errors.Is(err, ErrConflict) {
		return WebhookResult{Status: DeliveryDuplicate, Duplicate: true, Message: "duplicate delivery"}, nil
	}
	if err != nil {
		return WebhookResult{}, err
	}

	if event.Deleted || event.CommitSHA == "" || event.CommitSHA == strings.Repeat("0", 40) {
		msg := "push deleted or empty commit"
		_ = s.repo.UpdateDelivery(ctx, delivery.ID, DeliveryIgnored, nil, &msg)
		return WebhookResult{Status: DeliveryIgnored, Ignored: true, Message: msg}, nil
	}

	connID := c.ID
	apps, err := s.repo.FindAutoDeployApps(ctx, c.OrganizationID, &connID)
	if err != nil {
		return WebhookResult{}, err
	}
	repoKey := NormalizeRepoKey(event.RepositoryFullName)
	var deployIDs []uuid.UUID
	for _, app := range apps {
		if app.RepositoryURL == nil || !repoMatches(*app.RepositoryURL, repoKey, event) {
			continue
		}
		if app.GitBranch == nil || !BranchMatches(*app.GitBranch, event.Branch) {
			continue
		}
		if s.deploys == nil {
			continue
		}
		active, err := s.deploys.CountActive(ctx, app.ApplicationID)
		if err != nil {
			return WebhookResult{}, err
		}
		if active > 0 {
			s.log.Info("skip auto-deploy; deployment in progress",
				slog.String("applicationId", app.ApplicationID.String()),
			)
			continue
		}
		idem := "git_push:" + event.DeliveryID + ":" + app.ApplicationID.String()
		corr := event.CommitSHA
		d, err := s.deploys.CreateQueued(ctx, app.OrganizationID, app.ApplicationID, app.EnvironmentID, app.ServerID,
			deployments.TriggerGitPush, &idem, &corr, requestid.FromContext(ctx), uuid.Nil, s.now().UTC())
		if err != nil {
			if errors.Is(err, deployments.ErrConflict) {
				continue
			}
			msg := err.Error()
			_ = s.repo.UpdateDelivery(ctx, delivery.ID, DeliveryFailed, deployIDs, &msg)
			return WebhookResult{}, err
		}
		deployIDs = append(deployIDs, d.ID)
	}

	status := DeliveryProcessed
	msg := ""
	if len(deployIDs) == 0 {
		status = DeliveryIgnored
		msg = "no matching auto-deploy applications"
	}
	var errMsg *string
	if msg != "" {
		errMsg = &msg
	}
	_ = s.repo.UpdateDelivery(ctx, delivery.ID, status, deployIDs, errMsg)
	s.writeAudit(ctx, &c.OrganizationID, nil, "git_webhook.push", "git_connection", c.ID.String(), AuditMeta{}, nil, map[string]any{
		"deliveryId":  event.DeliveryID,
		"repository":  event.RepositoryFullName,
		"branch":      event.Branch,
		"commitSha":   event.CommitSHA,
		"deployments": len(deployIDs),
	})
	return WebhookResult{
		Status:        status,
		Ignored:       status == DeliveryIgnored,
		DeploymentIDs: deployIDs,
		Message:       msg,
	}, nil
}

func repoMatches(configuredURL, eventFullName string, event PushEvent) bool {
	cfg := NormalizeRepoKey(configuredURL)
	if cfg == "" {
		return false
	}
	if cfg == eventFullName || cfg == NormalizeRepoKey(event.RepositoryFullName) {
		return true
	}
	if event.CloneURL != "" && cfg == NormalizeRepoKey(event.CloneURL) {
		return true
	}
	if event.HTMLURL != "" && cfg == NormalizeRepoKey(event.HTMLURL) {
		return true
	}
	return false
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

func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
