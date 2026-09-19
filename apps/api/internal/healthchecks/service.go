package healthchecks

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/google/uuid"
)

type Service struct {
	repo  Repository
	authz *rbac.Authorizer
	log   *slog.Logger
	now   func() time.Time
}

func NewService(repo Repository, authz *rbac.Authorizer, log *slog.Logger) *Service {
	return &Service{repo: repo, authz: authz, log: log, now: time.Now}
}

func (s *Service) GetPolicy(ctx context.Context, appID uuid.UUID) (Policy, error) {
	raw, err := s.repo.LoadHealthCheckConfig(ctx, appID)
	if err != nil {
		return Policy{}, err
	}
	return ParsePolicy(raw)
}

func (s *Service) Get(ctx context.Context, actorID, appID uuid.UUID) (Status, Policy, error) {
	orgID, err := s.repo.GetApplicationOrg(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Status{}, Policy{}, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return Status{}, Policy{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ApplicationRead); err != nil {
		return Status{}, Policy{}, err
	}
	policy, err := s.GetPolicy(ctx, appID)
	if err != nil {
		return Status{}, Policy{}, err
	}
	st, err := s.repo.GetStatus(ctx, appID)
	if errors.Is(err, ErrNotFound) {
		return Status{
			ApplicationID:  appID,
			OrganizationID: orgID,
			State:          StateUnknown,
			UpdatedAt:      s.now().UTC(),
		}, policy, nil
	}
	return st, policy, err
}

func (s *Service) ListSamples(ctx context.Context, actorID, appID uuid.UUID, limit int) ([]Sample, error) {
	orgID, err := s.repo.GetApplicationOrg(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return nil, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ApplicationRead); err != nil {
		return nil, err
	}
	return s.repo.ListSamples(ctx, appID, limit)
}

// MarkStarting records that deployment health verification began.
func (s *Service) MarkStarting(ctx context.Context, orgID, appID uuid.UUID, revisionID, deploymentID *uuid.UUID, probeType string) (Status, error) {
	now := s.now().UTC()
	prev, err := s.repo.GetStatus(ctx, appID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Status{}, err
	}
	st := Status{
		ApplicationID:        appID,
		OrganizationID:       orgID,
		RevisionID:           revisionID,
		DeploymentID:         deploymentID,
		State:                StateStarting,
		ConsecutiveSuccesses: 0,
		ConsecutiveFailures:  0,
		LastMessage:          "health verification started",
		ProbeType:            probeType,
		UpdatedAt:            now,
	}
	if err == nil {
		st.LastSuccessAt = prev.LastSuccessAt
		st.LastFailureAt = prev.LastFailureAt
	}
	return s.repo.UpsertStatus(ctx, st)
}

// RecordProbe aggregates a probe result without requiring one DB write per raw
// agent tick beyond the aggregated status + a bounded sample row.
func (s *Service) RecordProbe(ctx context.Context, in ProbeInput) (Status, error) {
	now := s.now().UTC()
	prev, err := s.repo.GetStatus(ctx, in.ApplicationID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Status{}, err
	}
	st := Status{
		ApplicationID:  in.ApplicationID,
		OrganizationID: in.OrganizationID,
		RevisionID:     in.RevisionID,
		DeploymentID:   in.DeploymentID,
		ProbeType:      in.ProbeType,
		LastProbeAt:    &now,
		LastMessage:    in.Message,
	}
	if errors.Is(err, ErrNotFound) {
		prev = Status{State: StateUnknown}
	} else {
		st.ConsecutiveSuccesses = prev.ConsecutiveSuccesses
		st.ConsecutiveFailures = prev.ConsecutiveFailures
		st.LastSuccessAt = prev.LastSuccessAt
		st.LastFailureAt = prev.LastFailureAt
		if st.RevisionID == nil {
			st.RevisionID = prev.RevisionID
		}
		if st.DeploymentID == nil {
			st.DeploymentID = prev.DeploymentID
		}
		if st.ProbeType == "" {
			st.ProbeType = prev.ProbeType
		}
	}

	if in.Success {
		st.ConsecutiveSuccesses = prev.ConsecutiveSuccesses + 1
		st.ConsecutiveFailures = 0
		st.LastSuccessAt = &now
	} else {
		st.ConsecutiveFailures = prev.ConsecutiveFailures + 1
		st.ConsecutiveSuccesses = 0
		st.LastFailureAt = &now
	}

	policy := in.Policy
	if policy.Type == "" {
		policy = DefaultPolicy()
	}
	st.State = NextState(st.ConsecutiveSuccesses, st.ConsecutiveFailures, policy.SuccessThreshold(), policy.FailureThreshold(), prev.State)

	// Only persist a sample when state changes or counters hit thresholds — reduces write volume.
	shouldSample := st.State != prev.State ||
		st.ConsecutiveSuccesses == policy.SuccessThreshold() ||
		st.ConsecutiveFailures == policy.FailureThreshold() ||
		prev.State == StateUnknown || prev.State == StateStarting

	updated, err := s.repo.UpsertStatus(ctx, st)
	if err != nil {
		return Status{}, err
	}
	if shouldSample {
		_ = s.repo.InsertSample(ctx, Sample{
			OrganizationID: in.OrganizationID,
			ApplicationID:  in.ApplicationID,
			RevisionID:     in.RevisionID,
			DeploymentID:   in.DeploymentID,
			Success:        in.Success,
			ProbeType:      in.ProbeType,
			Message:        in.Message,
			LatencyMs:      in.LatencyMs,
			CreatedAt:      now,
		})
		_ = s.repo.TrimSamples(ctx, in.ApplicationID, MaxSamplesRetained)
	}
	return updated, nil
}

// ReportProbe is the authorized API path for probe ingestion.
func (s *Service) ReportProbe(ctx context.Context, actorID, appID uuid.UUID, success bool, message string, latencyMs *int, revisionID, deploymentID *uuid.UUID) (Status, error) {
	orgID, err := s.repo.GetApplicationOrg(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Status{}, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return Status{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ApplicationDeploy); err != nil {
		return Status{}, err
	}
	policy, err := s.GetPolicy(ctx, appID)
	if err != nil {
		return Status{}, apierror.Validation("invalid health check configuration", map[string]any{"healthCheck": err.Error()})
	}
	return s.RecordProbe(ctx, ProbeInput{
		OrganizationID: orgID,
		ApplicationID:  appID,
		RevisionID:     revisionID,
		DeploymentID:   deploymentID,
		Success:        success,
		ProbeType:      policy.Type,
		Message:        message,
		LatencyMs:      latencyMs,
		Policy:         policy,
	})
}
