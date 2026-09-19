package logs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agents"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/google/uuid"
)

type Service struct {
	store    Store
	access   *PostgresAccessor
	authz    *rbac.Authorizer
	log      *slog.Logger
	cfg      Config
	pollEvery time.Duration
	now      func() time.Time
}

func NewService(store Store, access *PostgresAccessor, authz *rbac.Authorizer, log *slog.Logger, cfg Config) *Service {
	cfg = cfg.withDefaults()
	return &Service{
		store:     store,
		access:    access,
		authz:     authz,
		log:       log,
		cfg:       cfg,
		pollEvery: 750 * time.Millisecond,
		now:       time.Now,
	}
}

type StreamRequest struct {
	Kind          string
	ApplicationID *uuid.UUID
	DeploymentID  *uuid.UUID
	Since         *time.Time
	Cursor        string
	Limit         int
	Follow        bool
}

func (s *Service) QueryForUser(ctx context.Context, actorID uuid.UUID, req StreamRequest) ([]Entry, string, error) {
	orgID, appID, depID, kind, err := s.resolveScope(ctx, actorID, req, true)
	if err != nil {
		return nil, "", err
	}
	req.Kind = kind
	if kind == KindDeploymentEvents {
		return s.queryDeploymentEvents(ctx, orgID, appID, depID, req)
	}
	return s.store.Query(ctx, Query{
		OrganizationID: orgID,
		ApplicationID:  appID,
		DeploymentID:   depID,
		Kind:           kind,
		Since:          req.Since,
		Cursor:         req.Cursor,
		Limit:          req.Limit,
	})
}

// SubscribeForUser returns a channel of entries. Caller must cancel ctx / call unsubscribe on disconnect.
func (s *Service) SubscribeForUser(ctx context.Context, actorID uuid.UUID, req StreamRequest) (<-chan Entry, func(), error) {
	orgID, appID, depID, kind, err := s.resolveScope(ctx, actorID, req, true)
	if err != nil {
		return nil, nil, err
	}
	req.Kind = kind
	if kind == KindDeploymentEvents {
		return s.subscribeDeploymentEvents(ctx, orgID, appID, depID, req)
	}
	return s.store.Subscribe(ctx, Query{
		OrganizationID: orgID,
		ApplicationID:  appID,
		DeploymentID:   depID,
		Kind:           kind,
		Since:          req.Since,
		Cursor:         req.Cursor,
		Limit:          req.Limit,
		Follow:         true,
	})
}

type IngestLine struct {
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type IngestInput struct {
	Kind          string
	ApplicationID uuid.UUID
	DeploymentID  *uuid.UUID
	RevisionID    *uuid.UUID
	Entries       []IngestLine
}

func (s *Service) IngestFromAgent(ctx context.Context, agent agents.Agent, in IngestInput) (int, error) {
	kind := strings.TrimSpace(in.Kind)
	if kind != KindBuild && kind != KindRuntime {
		return 0, apierror.Validation("kind must be build or runtime", map[string]any{"field": "kind"})
	}
	if in.ApplicationID == uuid.Nil {
		return 0, apierror.Validation("applicationId is required", map[string]any{"field": "applicationId"})
	}
	if len(in.Entries) == 0 {
		return 0, apierror.Validation("entries required", map[string]any{"field": "entries"})
	}
	if len(in.Entries) > 500 {
		return 0, apierror.Validation("too many entries in one batch", map[string]any{"field": "entries", "max": 500})
	}
	orgID, err := s.access.GetApplicationOrg(ctx, in.ApplicationID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return 0, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return 0, err
	}
	if orgID != agent.OrganizationID {
		return 0, apierror.Forbidden("application is outside agent organization")
	}
	if in.DeploymentID != nil {
		d, err := s.access.GetDeployment(ctx, *in.DeploymentID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return 0, apierror.NotFoundCode(apierror.CodeDeploymentNotFound, "deployment not found")
			}
			return 0, err
		}
		if d.OrganizationID != orgID || d.ApplicationID != in.ApplicationID {
			return 0, apierror.Validation("deployment does not belong to application", nil)
		}
	}

	appID := in.ApplicationID
	now := s.now().UTC()
	batch := make([]Entry, 0, len(in.Entries))
	for _, line := range in.Entries {
		msg := strings.TrimRight(line.Message, "\r\n")
		if msg == "" {
			continue
		}
		stream := strings.ToLower(strings.TrimSpace(line.Stream))
		switch stream {
		case StreamStdout, StreamStderr, StreamSystem:
		case "":
			stream = StreamStdout
		default:
			return 0, apierror.Validation("invalid stream", map[string]any{"field": "stream", "allowed": []string{StreamStdout, StreamStderr, StreamSystem}})
		}
		ts := line.Timestamp.UTC()
		if ts.IsZero() {
			ts = now
		}
		batch = append(batch, Entry{
			Timestamp:      ts,
			OrganizationID: orgID,
			ApplicationID:  &appID,
			DeploymentID:   in.DeploymentID,
			RevisionID:     in.RevisionID,
			Kind:           kind,
			Stream:         stream,
			Message:        msg,
		})
	}
	if len(batch) == 0 {
		return 0, nil
	}
	if err := s.store.Append(ctx, batch); err != nil {
		return 0, err
	}
	return len(batch), nil
}

// AppendInternal lets the control plane publish build/runtime lines (e.g. simulated orchestrator).
func (s *Service) AppendInternal(ctx context.Context, entries []Entry) error {
	return s.store.Append(ctx, entries)
}

func (s *Service) resolveScope(ctx context.Context, actorID uuid.UUID, req StreamRequest, requireAuth bool) (orgID uuid.UUID, appID, depID *uuid.UUID, kind string, err error) {
	kind = strings.TrimSpace(req.Kind)
	switch kind {
	case KindBuild, KindRuntime, KindDeploymentEvents:
	default:
		return uuid.Nil, nil, nil, "", apierror.Validation("kind must be build, runtime, or deployment_events", map[string]any{"field": "kind"})
	}

	if req.DeploymentID != nil {
		d, err := s.access.GetDeployment(ctx, *req.DeploymentID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return uuid.Nil, nil, nil, "", apierror.NotFoundCode(apierror.CodeDeploymentNotFound, "deployment not found")
			}
			return uuid.Nil, nil, nil, "", err
		}
		if requireAuth {
			if err := s.authz.RequirePermission(ctx, actorID, d.OrganizationID, rbac.DeploymentRead); err != nil {
				return uuid.Nil, nil, nil, "", err
			}
		}
		app := d.ApplicationID
		dep := d.ID
		if req.ApplicationID != nil && *req.ApplicationID != app {
			return uuid.Nil, nil, nil, "", apierror.Validation("applicationId does not match deployment", nil)
		}
		return d.OrganizationID, &app, &dep, kind, nil
	}

	if req.ApplicationID == nil {
		return uuid.Nil, nil, nil, "", apierror.Validation("applicationId or deploymentId is required", nil)
	}
	if kind == KindDeploymentEvents {
		return uuid.Nil, nil, nil, "", apierror.Validation("deployment_events requires deploymentId", map[string]any{"field": "deploymentId"})
	}
	org, err := s.access.GetApplicationOrg(ctx, *req.ApplicationID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return uuid.Nil, nil, nil, "", apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return uuid.Nil, nil, nil, "", err
	}
	if requireAuth {
		if err := s.authz.RequirePermission(ctx, actorID, org, rbac.ApplicationRead); err != nil {
			return uuid.Nil, nil, nil, "", err
		}
	}
	return org, req.ApplicationID, nil, kind, nil
}

func (s *Service) queryDeploymentEvents(ctx context.Context, orgID uuid.UUID, appID, depID *uuid.UUID, req StreamRequest) ([]Entry, string, error) {
	if depID == nil || appID == nil {
		return nil, "", apierror.Validation("deploymentId is required", nil)
	}
	events, err := s.access.ListEvents(ctx, *depID)
	if err != nil {
		return nil, "", err
	}
	limit := req.Limit
	if limit <= 0 || limit > s.cfg.MaxQueryLimit {
		limit = s.cfg.MaxQueryLimit
	}
	skipUntilCursor := false
	if req.Cursor != "" {
		for _, e := range events {
			if eventToEntry(e, *appID).Cursor == req.Cursor {
				skipUntilCursor = true
				break
			}
		}
	}
	out := make([]Entry, 0, len(events))
	for _, e := range events {
		if e.OrganizationID != orgID {
			continue
		}
		entry := eventToEntry(e, *appID)
		if skipUntilCursor {
			if entry.Cursor == req.Cursor {
				skipUntilCursor = false
			}
			continue
		}
		if req.Since != nil && !entry.Timestamp.After(*req.Since) {
			continue
		}
		out = append(out, entry)
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	next := ""
	if len(out) > 0 {
		next = out[len(out)-1].Cursor
	}
	return out, next, nil
}

func (s *Service) subscribeDeploymentEvents(ctx context.Context, orgID uuid.UUID, appID, depID *uuid.UUID, req StreamRequest) (<-chan Entry, func(), error) {
	subCtx, cancel := context.WithCancel(ctx)
	out := make(chan Entry, s.cfg.SubscriberBuffer)

	go func() {
		defer close(out)
		cursor := req.Cursor
		// Initial snapshot
		entries, next, err := s.queryDeploymentEvents(subCtx, orgID, appID, depID, StreamRequest{
			Kind: KindDeploymentEvents, Since: req.Since, Cursor: cursor, Limit: req.Limit,
		})
		if err == nil {
			for _, e := range entries {
				select {
				case <-subCtx.Done():
					return
				case out <- e:
					cursor = e.Cursor
				}
			}
			if next != "" {
				cursor = next
			}
		}
		ticker := time.NewTicker(s.pollEvery)
		defer ticker.Stop()
		for {
			select {
			case <-subCtx.Done():
				return
			case <-ticker.C:
				entries, next, err := s.queryDeploymentEvents(subCtx, orgID, appID, depID, StreamRequest{
					Kind: KindDeploymentEvents, Cursor: cursor, Limit: s.cfg.MaxQueryLimit,
				})
				if err != nil {
					continue
				}
				for _, e := range entries {
					select {
					case <-subCtx.Done():
						return
					case out <- e:
						cursor = e.Cursor
					default:
						// Backpressure: skip this poll batch for slow clients.
						goto nextTick
					}
				}
				if next != "" {
					cursor = next
				}
			nextTick:
			}
		}
	}()

	return out, cancel, nil
}
