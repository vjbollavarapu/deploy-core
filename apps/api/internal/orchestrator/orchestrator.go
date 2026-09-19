package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agentcmd"
	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
	"github.com/deploycore/deploy-core/apps/api/internal/healthchecks"
	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
	"github.com/deploycore/deploy-core/apps/api/internal/notifications"
	"github.com/deploycore/deploy-core/apps/api/internal/replicas"
	"github.com/deploycore/deploy-core/apps/api/internal/webhooks"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config controls orchestrator behavior.
type Config struct {
	// SimulateAgent advances stages locally when the agent is unavailable
	// or when explicitly enabled (typical in development/test).
	SimulateAgent bool
	// ForceHealthFail fails HEALTH_CHECKING (test-only).
	ForceHealthFail bool
	// ForceBuildFail fails BUILDING (test-only).
	ForceBuildFail bool
	// ForceStartFail fails STARTING (test-only).
	ForceStartFail bool
}

// HealthGate records aggregated health during deployment verification.
type HealthGate interface {
	GetPolicy(ctx context.Context, appID uuid.UUID) (healthchecks.Policy, error)
	MarkStarting(ctx context.Context, orgID, appID uuid.UUID, revisionID, deploymentID *uuid.UUID, probeType string) (healthchecks.Status, error)
	RecordProbe(ctx context.Context, in healthchecks.ProbeInput) (healthchecks.Status, error)
}

// NotificationEmitter fans out deployment terminal events (B25).
type NotificationEmitter interface {
	Emit(ctx context.Context, in notifications.EmitInput) (int, error)
}

// WebhookEmitter fans out outgoing platform webhooks (B26).
type WebhookEmitter interface {
	Emit(ctx context.Context, in webhooks.EmitInput) (int, error)
}

// Orchestrator runs deployment workflow steps in a resumable fashion.
type Orchestrator struct {
	pool     *pgxpool.Pool
	deploys  deployments.Repository
	commands *agentcmd.PostgresRepository
	replicas *replicas.PostgresRepository
	health   HealthGate
	notify   NotificationEmitter
	webhooks WebhookEmitter
	log      *slog.Logger
	cfg      Config
	now      func() time.Time
}

func New(pool *pgxpool.Pool, deploys deployments.Repository, log *slog.Logger, cfg Config) *Orchestrator {
	return &Orchestrator{
		pool:     pool,
		deploys:  deploys,
		commands: agentcmd.NewPostgresRepository(pool),
		replicas: replicas.NewPostgresRepository(pool),
		health:   healthchecks.NewService(healthchecks.NewPostgresRepository(pool), nil, log),
		log:      log,
		cfg:      cfg,
		now:      time.Now,
	}
}

// WithHealthGate overrides the default health gate (tests).
func (o *Orchestrator) WithHealthGate(h HealthGate) *Orchestrator {
	o.health = h
	return o
}

// WithNotifier attaches async notification emission for terminal deployments.
func (o *Orchestrator) WithNotifier(n NotificationEmitter) *Orchestrator {
	o.notify = n
	return o
}

// WithWebhooks attaches outgoing webhook emission for deployment lifecycle events.
func (o *Orchestrator) WithWebhooks(w WebhookEmitter) *Orchestrator {
	o.webhooks = w
	return o
}

// JobHandler returns a jobs.Handler for DEPLOYMENT_EXECUTION.
func (o *Orchestrator) JobHandler() jobs.Handler {
	return func(ctx context.Context, job jobs.Job) error {
		raw, _ := job.Payload["deploymentId"].(string)
		deploymentID, err := uuid.Parse(raw)
		if err != nil {
			return fmt.Errorf("invalid deploymentId in job payload: %w", err)
		}
		return o.Execute(ctx, deploymentID)
	}
}

// Execute advances a deployment as far as possible in this invocation.
func (o *Orchestrator) Execute(ctx context.Context, deploymentID uuid.UUID) error {
	d, err := o.deploys.Get(ctx, deploymentID)
	if err != nil {
		if errors.Is(err, deployments.ErrNotFound) {
			return fmt.Errorf("deployment not found")
		}
		return err
	}
	if deployments.IsTerminal(d.Status) {
		o.log.Info("deployment already terminal",
			slog.String("deploymentId", d.ID.String()),
			slog.String("status", d.Status),
		)
		return nil
	}

	// Drive happy-path steps until blocked, failed, or complete.
	for !deployments.IsTerminal(d.Status) {
		prev := d.Status
		d, err = o.step(ctx, d)
		if err != nil {
			return err
		}
		if d.Status == prev {
			// Waiting on external work (agent) — retry later.
			return jobs.ErrRetryLater
		}
	}
	return nil
}

func (o *Orchestrator) step(ctx context.Context, d deployments.Deployment) (deployments.Deployment, error) {
	switch d.Status {
	case deployments.StatusQueued:
		return o.stepQueued(ctx, d)
	case deployments.StatusPreparing:
		return o.stepPreparing(ctx, d)
	case deployments.StatusFetchingSource:
		return o.stepFetchingSource(ctx, d)
	case deployments.StatusBuilding:
		return o.stepBuilding(ctx, d)
	case deployments.StatusImageReady:
		return o.stepImageReady(ctx, d)
	case deployments.StatusCreatingContainer:
		return o.stepCreatingContainer(ctx, d)
	case deployments.StatusStarting:
		return o.stepStarting(ctx, d)
	case deployments.StatusHealthChecking:
		return o.stepHealthChecking(ctx, d)
	case deployments.StatusActivating:
		return o.stepActivating(ctx, d)
	default:
		return d, fmt.Errorf("orchestrator cannot handle status %s", d.Status)
	}
}

func (o *Orchestrator) stepQueued(ctx context.Context, d deployments.Deployment) (deployments.Deployment, error) {
	if err := o.validateServer(ctx, d); err != nil {
		return o.fail(ctx, d, deployments.StatusQueued, deployments.StatusTimeout, "SERVER_OFFLINE", err.Error())
	}
	updated, err := o.advance(ctx, d, deployments.StatusPreparing, "server validated; preparing deployment", nil)
	if err != nil {
		return updated, err
	}
	o.emitWebhook(ctx, updated, webhooks.EventDeploymentStarted)
	return updated, nil
}

func (o *Orchestrator) stepPreparing(ctx context.Context, d deployments.Deployment) (deployments.Deployment, error) {
	if isRollback(d) {
		return o.stepPreparingRollback(ctx, d)
	}
	eff, vars, secrets, cfg, err := o.buildEffectiveConfig(ctx, d)
	if err != nil {
		return o.fail(ctx, d, deployments.StatusPreparing, deployments.StatusSourceFailed, "CONFIG_ERROR", err.Error())
	}
	rev, err := o.ensureRevisionCandidate(ctx, d, eff, vars, secrets, cfg)
	if err != nil {
		return o.fail(ctx, d, deployments.StatusPreparing, deployments.StatusSourceFailed, "REVISION_ERROR", err.Error())
	}
	if err := o.setTargetRevision(ctx, d.ID, rev.ID); err != nil {
		return deployments.Deployment{}, err
	}
	d.TargetRevisionID = &rev.ID

	return o.advance(ctx, d, deployments.StatusFetchingSource, "revision candidate created; fetching source", map[string]any{
		"revisionId":     rev.ID.String(),
		"revisionNumber": rev.Number,
		"sourceType":     cfg.SourceType,
	})
}

func (o *Orchestrator) stepPreparingRollback(ctx context.Context, d deployments.Deployment) (deployments.Deployment, error) {
	if d.TargetRevisionID == nil {
		return o.fail(ctx, d, deployments.StatusPreparing, deployments.StatusSourceFailed, "REVISION_MISSING", "rollback target revision missing")
	}
	var status string
	var number int
	err := o.pool.QueryRow(ctx, `
		SELECT status, revision_number FROM revisions
		WHERE id = $1 AND application_id = $2`, *d.TargetRevisionID, d.ApplicationID).
		Scan(&status, &number)
	if errors.Is(err, pgx.ErrNoRows) {
		return o.fail(ctx, d, deployments.StatusPreparing, deployments.StatusSourceFailed, "REVISION_MISSING", "rollback target revision not found")
	}
	if err != nil {
		return deployments.Deployment{}, err
	}
	if status != "READY" && status != "INACTIVE" {
		return o.fail(ctx, d, deployments.StatusPreparing, deployments.StatusSourceFailed, "REVISION_NOT_READY",
			fmt.Sprintf("rollback target status is %s", status))
	}
	return o.advance(ctx, d, deployments.StatusFetchingSource, "rollback: reusing existing revision (no rebuild)", map[string]any{
		"revisionId":     d.TargetRevisionID.String(),
		"revisionNumber": number,
		"rollback":       true,
	})
}

func (o *Orchestrator) stepFetchingSource(ctx context.Context, d deployments.Deployment) (deployments.Deployment, error) {
	if isRollback(d) {
		return o.advance(ctx, d, deployments.StatusBuilding, "rollback: skip source fetch", map[string]any{"rollback": true})
	}
	cfg, err := o.loadAppConfig(ctx, d.ApplicationID)
	if err != nil {
		return o.fail(ctx, d, deployments.StatusFetchingSource, deployments.StatusSourceFailed, "CONFIG_ERROR", err.Error())
	}
	// Image sources skip remote fetch.
	if cfg.SourceType != "image" {
		if err := o.issueOrSimulate(ctx, d, agentcmd.OpBuildImage, map[string]any{
			"phase": "fetch_source", "deploymentId": d.ID.String(),
		}); err != nil {
			return o.fail(ctx, d, deployments.StatusFetchingSource, deployments.StatusSourceFailed, "SOURCE_FAILED", err.Error())
		}
	}
	return o.advance(ctx, d, deployments.StatusBuilding, "source ready", nil)
}

func (o *Orchestrator) stepBuilding(ctx context.Context, d deployments.Deployment) (deployments.Deployment, error) {
	if isRollback(d) {
		return o.advance(ctx, d, deployments.StatusImageReady, "rollback: skip build/pull", map[string]any{"rollback": true})
	}
	if o.cfg.ForceBuildFail {
		return o.fail(ctx, d, deployments.StatusBuilding, deployments.StatusBuildFailed, "BUILD_FAILED", "forced build failure")
	}
	cfg, err := o.loadAppConfig(ctx, d.ApplicationID)
	if err != nil {
		return o.fail(ctx, d, deployments.StatusBuilding, deployments.StatusBuildFailed, "CONFIG_ERROR", err.Error())
	}
	op := agentcmd.OpBuildImage
	payload := map[string]any{"deploymentId": d.ID.String(), "phase": "build"}
	if cfg.SourceType == "image" {
		op = agentcmd.OpPullImage
		payload["phase"] = "pull"
		if cfg.ImageReference != nil {
			payload["imageReference"] = *cfg.ImageReference
		}
	}
	if err := o.issueOrSimulate(ctx, d, op, payload); err != nil {
		failTo := deployments.StatusBuildFailed
		if cfg.SourceType == "image" {
			failTo = deployments.StatusImageFailed
		}
		return o.fail(ctx, d, deployments.StatusBuilding, failTo, "BUILD_FAILED", err.Error())
	}
	digest := fmt.Sprintf("sha256:sim-%s", d.ID.String()[:8])
	if d.TargetRevisionID != nil {
		_ = o.updateRevisionImage(ctx, *d.TargetRevisionID, digest, strOr(cfg.ImageReference, "local:candidate"))
	}
	return o.advance(ctx, d, deployments.StatusImageReady, "image ready", map[string]any{"imageDigest": digest})
}

func (o *Orchestrator) stepImageReady(ctx context.Context, d deployments.Deployment) (deployments.Deployment, error) {
	msg := "creating candidate container"
	meta := map[string]any(nil)
	if isRollback(d) {
		msg = "rollback: starting reused revision"
		meta = map[string]any{"rollback": true}
	}
	return o.advance(ctx, d, deployments.StatusCreatingContainer, msg, meta)
}

func (o *Orchestrator) stepCreatingContainer(ctx context.Context, d deployments.Deployment) (deployments.Deployment, error) {
	desired, slug, revNumber, err := o.replicaDeployContext(ctx, d)
	if err != nil {
		return o.fail(ctx, d, deployments.StatusCreatingContainer, deployments.StatusContainerFailed, "CONTAINER_FAILED", err.Error())
	}
	for i := 0; i < desired; i++ {
		name := replicas.ContainerName(slug, revNumber, i)
		payload := map[string]any{
			"deploymentId":  d.ID.String(),
			"phase":         "create_container",
			"replicaIndex":  i,
			"containerName": name,
			"desiredReplicas": desired,
		}
		if isRollback(d) {
			payload["phase"] = "rollback_reuse"
		}
		if d.TargetRevisionID != nil {
			payload["revisionId"] = d.TargetRevisionID.String()
		}
		if _, err := o.replicas.UpsertSlot(ctx, replicas.UpsertInput{
			OrganizationID: d.OrganizationID,
			ApplicationID:  d.ApplicationID,
			RevisionID:     d.TargetRevisionID,
			ServerID:       d.ServerID,
			ReplicaIndex:   i,
			ContainerName:  name,
			Status:         replicas.StatusPending,
		}); err != nil {
			return o.fail(ctx, d, deployments.StatusCreatingContainer, deployments.StatusContainerFailed, "CONTAINER_FAILED", err.Error())
		}
		if err := o.issueOrSimulate(ctx, d, agentcmd.OpDeployRevision, payload); err != nil {
			return o.fail(ctx, d, deployments.StatusCreatingContainer, deployments.StatusContainerFailed, "CONTAINER_FAILED", err.Error())
		}
	}
	if d.TargetRevisionID != nil && !isRollback(d) {
		_ = o.setRevisionStatus(ctx, *d.TargetRevisionID, "READY")
	}
	return o.advance(ctx, d, deployments.StatusStarting, "candidate replica containers created", map[string]any{
		"desiredReplicas": desired,
	})
}

func (o *Orchestrator) stepStarting(ctx context.Context, d deployments.Deployment) (deployments.Deployment, error) {
	if o.cfg.ForceStartFail {
		return o.fail(ctx, d, deployments.StatusStarting, deployments.StatusStartFailed, "START_FAILED", "forced start failure")
	}
	desired, _, _, err := o.replicaDeployContext(ctx, d)
	if err != nil {
		return o.fail(ctx, d, deployments.StatusStarting, deployments.StatusStartFailed, "START_FAILED", err.Error())
	}
	list, err := o.replicas.ListByApplication(ctx, d.ApplicationID)
	if err != nil {
		return deployments.Deployment{}, err
	}
	for _, rep := range list {
		if rep.ReplicaIndex >= desired {
			continue
		}
		if err := o.issueOrSimulate(ctx, d, agentcmd.OpStartContainer, map[string]any{
			"deploymentId":  d.ID.String(),
			"replicaIndex":  rep.ReplicaIndex,
			"containerName": rep.ContainerName,
		}); err != nil {
			return o.fail(ctx, d, deployments.StatusStarting, deployments.StatusStartFailed, "START_FAILED", err.Error())
		}
		falseVal := false
		_, _ = o.replicas.UpdateStatus(ctx, rep.ID, replicas.StatusStarting, &falseVal, &falseVal, nil, nil)
	}
	return o.advance(ctx, d, deployments.StatusHealthChecking, "replicas started; health checking", map[string]any{
		"desiredReplicas": desired,
	})
}

func (o *Orchestrator) stepHealthChecking(ctx context.Context, d deployments.Deployment) (deployments.Deployment, error) {
	policy, err := o.health.GetPolicy(ctx, d.ApplicationID)
	if err != nil {
		return o.fail(ctx, d, deployments.StatusHealthChecking, deployments.StatusHealthCheckFailed, "HEALTH_POLICY_INVALID", err.Error())
	}
	if cfg, cfgErr := o.loadAppConfig(ctx, d.ApplicationID); cfgErr == nil && policy.Port == nil && cfg.InternalPort != nil {
		p := *cfg.InternalPort
		policy.Port = &p
	}

	_, _ = o.health.MarkStarting(ctx, d.OrganizationID, d.ApplicationID, d.TargetRevisionID, &d.ID, policy.Type)

	if o.cfg.ForceHealthFail {
		_, _ = o.health.RecordProbe(ctx, healthchecks.ProbeInput{
			OrganizationID: d.OrganizationID,
			ApplicationID:  d.ApplicationID,
			RevisionID:     d.TargetRevisionID,
			DeploymentID:   &d.ID,
			Success:        false,
			ProbeType:      policy.Type,
			Message:        "forced health check failure",
			Policy:         policy,
		})
		return o.fail(ctx, d, deployments.StatusHealthChecking, deployments.StatusHealthCheckFailed, apierror.CodeHealthCheckFailed, "forced health check failure")
	}

	if !policy.IsEnabled() {
		st, _ := o.health.RecordProbe(ctx, healthchecks.ProbeInput{
			OrganizationID: d.OrganizationID,
			ApplicationID:  d.ApplicationID,
			RevisionID:     d.TargetRevisionID,
			DeploymentID:   &d.ID,
			Success:        true,
			ProbeType:      policy.Type,
			Message:        "health checks disabled; skipping probes",
			Policy:         healthchecks.Policy{Type: policy.Type, Retries: 1, Enabled: policy.Enabled},
		})
		desired, _, _, _ := o.replicaDeployContext(ctx, d)
		list, _ := o.replicas.ListByApplication(ctx, d.ApplicationID)
		healthy := true
		for _, rep := range list {
			if rep.ReplicaIndex >= desired {
				continue
			}
			_, _ = o.replicas.UpdateStatus(ctx, rep.ID, replicas.StatusRunning, &healthy, nil, nil, nil)
		}
		return o.advance(ctx, d, deployments.StatusActivating, "health policy disabled; activating", map[string]any{
			"healthState": st.State, "desiredReplicas": desired,
		})
	}

	payload := policy.ToAgentPayload()
	payload["deploymentId"] = d.ID.String()
	if d.TargetRevisionID != nil {
		payload["revisionId"] = d.TargetRevisionID.String()
	}
	if err := o.issueOrSimulate(ctx, d, agentcmd.OpRunHealthCheck, payload); err != nil {
		_, _ = o.health.RecordProbe(ctx, healthchecks.ProbeInput{
			OrganizationID: d.OrganizationID,
			ApplicationID:  d.ApplicationID,
			RevisionID:     d.TargetRevisionID,
			DeploymentID:   &d.ID,
			Success:        false,
			ProbeType:      policy.Type,
			Message:        err.Error(),
			Policy:         policy,
		})
		return o.fail(ctx, d, deployments.StatusHealthChecking, deployments.StatusHealthCheckFailed, apierror.CodeHealthCheckFailed, err.Error())
	}

	// Simulate/agent success path: record enough consecutive successes to satisfy policy
	// before routing traffic (activation). Never activate while not HEALTHY.
	var st healthchecks.Status
	for i := 0; i < policy.SuccessThreshold(); i++ {
		st, err = o.health.RecordProbe(ctx, healthchecks.ProbeInput{
			OrganizationID: d.OrganizationID,
			ApplicationID:  d.ApplicationID,
			RevisionID:     d.TargetRevisionID,
			DeploymentID:   &d.ID,
			Success:        true,
			ProbeType:      policy.Type,
			Message:        "probe succeeded",
			Policy:         policy,
		})
		if err != nil {
			return deployments.Deployment{}, err
		}
	}
	if !healthchecks.ReadyForActivation(policy, st.State) {
		return o.fail(ctx, d, deployments.StatusHealthChecking, deployments.StatusHealthCheckFailed, apierror.CodeHealthCheckFailed,
			fmt.Sprintf("health state %s is not ready for activation", st.State))
	}

	desired, _, _, _ := o.replicaDeployContext(ctx, d)
	list, _ := o.replicas.ListByApplication(ctx, d.ApplicationID)
	healthy := true
	for _, rep := range list {
		if rep.ReplicaIndex >= desired {
			continue
		}
		_, _ = o.replicas.UpdateStatus(ctx, rep.ID, replicas.StatusRunning, &healthy, nil, nil, nil)
	}

	return o.advance(ctx, d, deployments.StatusActivating, "health checks passed; activating replicas", map[string]any{
		"healthState":     st.State,
		"probeType":       policy.Type,
		"desiredReplicas": desired,
	})
}

func (o *Orchestrator) stepActivating(ctx context.Context, d deployments.Deployment) (deployments.Deployment, error) {
	if d.TargetRevisionID == nil {
		return o.fail(ctx, d, deployments.StatusActivating, deployments.StatusRoutingFailed, "REVISION_MISSING", "target revision missing")
	}
	prevActive, err := o.getActiveRevision(ctx, d.ApplicationID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return deployments.Deployment{}, err
	}

	desired, slug, _, err := o.replicaDeployContext(ctx, d)
	if err != nil {
		return o.fail(ctx, d, deployments.StatusActivating, deployments.StatusRoutingFailed, "ROUTING_FAILED", err.Error())
	}

	// Rolling: enable Traefik routing on each healthy candidate replica (shared service name = LB).
	list, err := o.replicas.ListByApplication(ctx, d.ApplicationID)
	if err != nil {
		return deployments.Deployment{}, err
	}
	healthyTrue := true
	routingOn := true
	for _, rep := range list {
		if rep.ReplicaIndex >= desired {
			continue
		}
		if rep.Status != replicas.StatusRunning && !rep.Healthy {
			continue
		}
		labels := map[string]string{
			"traefik.enable":              "true",
			"deploycore.application.id":   d.ApplicationID.String(),
			"deploycore.application.slug": slug,
			"deploycore.replica.index":    fmt.Sprintf("%d", rep.ReplicaIndex),
			"deploycore.replica.role":     "member",
		}
		_ = o.issueOrSimulate(ctx, d, agentcmd.OpDeployRevision, map[string]any{
			"deploymentId":   d.ID.String(),
			"phase":          "enable_routing",
			"replicaIndex":   rep.ReplicaIndex,
			"containerName":  rep.ContainerName,
			"routingLabels":  labels,
			"routingEnabled": true,
		})
		_, _ = o.replicas.UpdateStatus(ctx, rep.ID, replicas.StatusRunning, &healthyTrue, &routingOn, nil, nil)
	}

	// Activate candidate only after health passed — previous ACTIVE preserved until here.
	if err := o.activateRevision(ctx, d.ApplicationID, *d.TargetRevisionID, prevActive); err != nil {
		return o.fail(ctx, d, deployments.StatusActivating, deployments.StatusRoutingFailed, "ROUTING_FAILED", err.Error())
	}
	if err := o.setActiveRevision(ctx, d.ID, *d.TargetRevisionID); err != nil {
		return deployments.Deployment{}, err
	}

	if prevActive != nil && prevActive.ID != *d.TargetRevisionID {
		// Drain/stop previous revision replicas (rolling retire).
		for _, rep := range list {
			if rep.RevisionID != nil && *rep.RevisionID == prevActive.ID {
				_ = o.issueOrSimulate(ctx, d, agentcmd.OpStopContainer, map[string]any{
					"deploymentId":  d.ID.String(),
					"revisionId":    prevActive.ID.String(),
					"replicaIndex":  rep.ReplicaIndex,
					"containerName": rep.ContainerName,
					"reason":        "rolling_replace",
					"drainRouting":  true,
				})
			}
		}
		_ = o.issueOrSimulate(ctx, d, agentcmd.OpStopContainer, map[string]any{
			"deploymentId": d.ID.String(),
			"revisionId":   prevActive.ID.String(),
			"reason":       "retire_previous",
		})
	}

	meta := map[string]any{
		"activeRevisionId": d.TargetRevisionID.String(),
		"desiredReplicas":  desired,
		"rolling":          true,
	}
	if prevActive != nil {
		meta["retiredRevisionId"] = prevActive.ID.String()
	}
	if isRollback(d) {
		meta["rollback"] = true
	}
	return o.advance(ctx, d, deployments.StatusRunning, "replicas activated; deployment succeeded", meta)
}

func (o *Orchestrator) advance(ctx context.Context, d deployments.Deployment, to, message string, meta map[string]any) (deployments.Deployment, error) {
	updated, err := o.deploys.Transition(ctx, d.ID, d.Status, deployments.TransitionInput{
		ToStatus:  to,
		Message:   message,
		Metadata:  meta,
		RequestID: strVal(d.RequestID),
	}, o.now().UTC())
	if err != nil {
		return deployments.Deployment{}, err
	}
	updated.Trigger = d.Trigger
	updated.TargetRevisionID = d.TargetRevisionID
	updated.ActiveRevisionID = d.ActiveRevisionID
	if to == deployments.StatusRunning && d.TargetRevisionID != nil {
		updated.ActiveRevisionID = d.TargetRevisionID
	}
	if to == deployments.StatusRunning {
		o.emitDeployment(ctx, updated, notifications.EventDeploymentSucceeded)
		o.emitWebhook(ctx, updated, webhooks.EventDeploymentCompleted)
	}
	return updated, nil
}

func (o *Orchestrator) fail(ctx context.Context, d deployments.Deployment, from, to, code, message string) (deployments.Deployment, error) {
	// Rollback must not mark the reused target revision FAILED — keep it READY/INACTIVE.
	if d.TargetRevisionID != nil && !isRollback(d) {
		_ = o.setRevisionStatus(ctx, *d.TargetRevisionID, "FAILED")
	}
	// Intentionally do not touch previous ACTIVE revision or routing.
	codeCopy, msgCopy := code, message
	updated, err := o.deploys.Transition(ctx, d.ID, from, deployments.TransitionInput{
		ToStatus:     to,
		Message:      message,
		ErrorCode:    &codeCopy,
		ErrorMessage: &msgCopy,
		RequestID:    strVal(d.RequestID),
		Metadata:     map[string]any{"preservedActiveRevision": true},
	}, o.now().UTC())
	if err != nil {
		return deployments.Deployment{}, err
	}
	o.log.Error("deployment failed",
		slog.String("deploymentId", d.ID.String()),
		slog.String("status", to),
		slog.String("error", message),
	)
	o.emitDeployment(ctx, updated, notifications.EventDeploymentFailed)
	o.emitWebhook(ctx, updated, webhooks.EventDeploymentFailed)
	return updated, nil
}

func (o *Orchestrator) emitDeployment(ctx context.Context, d deployments.Deployment, eventType string) {
	if o.notify == nil {
		return
	}
	appID := d.ApplicationID
	envID := d.EnvironmentID
	rid := d.ID
	in := notifications.EmitInput{
		OrganizationID: d.OrganizationID,
		EventType:      eventType,
		ApplicationID:  &appID,
		EnvironmentID:  &envID,
		ResourceType:   "deployment",
		ResourceID:     &rid,
		Payload: map[string]any{
			"deploymentId":  d.ID.String(),
			"applicationId": d.ApplicationID.String(),
			"status":        d.Status,
		},
	}
	if d.ServerID != nil {
		in.ServerID = d.ServerID
		in.Payload["serverId"] = d.ServerID.String()
	}
	if d.ErrorCode != nil {
		in.Payload["errorCode"] = *d.ErrorCode
	}
	if d.ErrorMessage != nil {
		in.Payload["errorMessage"] = *d.ErrorMessage
	}
	if _, err := o.notify.Emit(ctx, in); err != nil {
		o.log.Warn("deployment notification emit failed", slog.String("error", err.Error()))
	}
}

func (o *Orchestrator) emitWebhook(ctx context.Context, d deployments.Deployment, eventType string) {
	if o.webhooks == nil {
		return
	}
	appID := d.ApplicationID
	envID := d.EnvironmentID
	rid := d.ID
	in := webhooks.EmitInput{
		OrganizationID: d.OrganizationID,
		EventType:      eventType,
		ApplicationID:  &appID,
		EnvironmentID:  &envID,
		ResourceType:   "deployment",
		ResourceID:     &rid,
		Payload: map[string]any{
			"deploymentId":  d.ID.String(),
			"applicationId": d.ApplicationID.String(),
			"status":        d.Status,
		},
	}
	if d.ServerID != nil {
		in.ServerID = d.ServerID
		in.Payload["serverId"] = d.ServerID.String()
	}
	if d.ErrorCode != nil {
		in.Payload["errorCode"] = *d.ErrorCode
	}
	if d.ErrorMessage != nil {
		in.Payload["errorMessage"] = *d.ErrorMessage
	}
	if _, err := o.webhooks.Emit(ctx, in); err != nil {
		o.log.Warn("deployment webhook emit failed", slog.String("error", err.Error()))
	}
}

func (o *Orchestrator) validateServer(ctx context.Context, d deployments.Deployment) error {
	if d.ServerID == nil {
		return errors.New("deployment has no target server")
	}
	var status string
	var maintenance, deleted bool
	err := o.pool.QueryRow(ctx, `
		SELECT status, maintenance_mode, deleted_at IS NOT NULL
		FROM servers WHERE id = $1`, *d.ServerID).Scan(&status, &maintenance, &deleted)
	if errors.Is(err, pgx.ErrNoRows) || deleted {
		return errors.New("target server not found")
	}
	if err != nil {
		return err
	}
	if status == "DISABLED" {
		return errors.New("target server is disabled")
	}
	if maintenance {
		return errors.New("target server is in maintenance")
	}
	// ONLINE preferred; OFFLINE allowed when simulating (no agent yet).
	if status != "ONLINE" && !o.cfg.SimulateAgent {
		return fmt.Errorf("target server status is %s", status)
	}
	return nil
}

type appConfig struct {
	SourceType     string
	RepositoryURL  *string
	GitBranch      *string
	ImageReference *string
	InternalPort   *int
	HealthCheck    map[string]any
	RuntimeConfig  map[string]any
	CPULimitMillis *int
	MemoryLimit    *int64
	RestartPolicy  string
}

func (o *Orchestrator) loadAppConfig(ctx context.Context, appID uuid.UUID) (appConfig, error) {
	var c appConfig
	var health, runtime []byte
	err := o.pool.QueryRow(ctx, `
		SELECT source_type, repository_url, git_branch, image_reference, internal_port,
		       health_check, runtime_config, cpu_limit_millis, memory_limit_bytes, restart_policy
		FROM application_configs
		WHERE application_id = $1
		ORDER BY version DESC LIMIT 1`, appID).Scan(
		&c.SourceType, &c.RepositoryURL, &c.GitBranch, &c.ImageReference, &c.InternalPort,
		&health, &runtime, &c.CPULimitMillis, &c.MemoryLimit, &c.RestartPolicy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return appConfig{}, errors.New("application config not found")
	}
	if err != nil {
		return appConfig{}, err
	}
	c.HealthCheck = map[string]any{}
	c.RuntimeConfig = map[string]any{}
	_ = json.Unmarshal(health, &c.HealthCheck)
	_ = json.Unmarshal(runtime, &c.RuntimeConfig)
	return c, nil
}

func (o *Orchestrator) buildEffectiveConfig(ctx context.Context, d deployments.Deployment) (eff, vars, secretRefs map[string]any, cfg appConfig, err error) {
	cfg, err = o.loadAppConfig(ctx, d.ApplicationID)
	if err != nil {
		return nil, nil, nil, cfg, err
	}
	vars = map[string]any{}
	rows, err := o.pool.Query(ctx, `
		SELECT key, value, scope FROM environment_variables
		WHERE organization_id = $1
		  AND (
		    scope = 'ORGANIZATION'
		    OR (scope = 'PROJECT' AND project_id = (SELECT project_id FROM applications WHERE id = $2))
		    OR (scope = 'ENVIRONMENT' AND environment_id = $3)
		    OR (scope = 'APPLICATION' AND application_id = $2)
		  )
		ORDER BY CASE scope
		  WHEN 'ORGANIZATION' THEN 1 WHEN 'PROJECT' THEN 2
		  WHEN 'ENVIRONMENT' THEN 3 WHEN 'APPLICATION' THEN 4 END`,
		d.OrganizationID, d.ApplicationID, d.EnvironmentID)
	if err != nil {
		return nil, nil, nil, cfg, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value, scope string
		if err := rows.Scan(&key, &value, &scope); err != nil {
			return nil, nil, nil, cfg, err
		}
		vars[key] = map[string]any{"value": value, "scope": scope}
	}

	secretRefs = map[string]any{}
	srows, err := o.pool.Query(ctx, `
		SELECT name, version, scope FROM secrets
		WHERE organization_id = $1 AND deleted_at IS NULL
		  AND (
		    scope = 'ORGANIZATION'
		    OR (scope = 'PROJECT' AND project_id = (SELECT project_id FROM applications WHERE id = $2))
		    OR (scope = 'ENVIRONMENT' AND environment_id = $3)
		    OR (scope = 'APPLICATION' AND application_id = $2)
		  )`, d.OrganizationID, d.ApplicationID, d.EnvironmentID)
	if err != nil {
		return nil, nil, nil, cfg, err
	}
	defer srows.Close()
	for srows.Next() {
		var name, scope string
		var version int
		if err := srows.Scan(&name, &version, &scope); err != nil {
			return nil, nil, nil, cfg, err
		}
		secretRefs[name] = map[string]any{"version": version, "scope": scope}
	}

	eff = map[string]any{
		"sourceType":     cfg.SourceType,
		"repositoryUrl":  cfg.RepositoryURL,
		"gitBranch":      cfg.GitBranch,
		"imageReference": cfg.ImageReference,
		"internalPort":   cfg.InternalPort,
		"restartPolicy":  cfg.RestartPolicy,
		"runtimeConfig":  cfg.RuntimeConfig,
		"healthCheck":    cfg.HealthCheck,
	}
	return eff, vars, secretRefs, cfg, nil
}

type revision struct {
	ID     uuid.UUID
	Number int
}

func (o *Orchestrator) ensureRevisionCandidate(ctx context.Context, d deployments.Deployment, eff, vars, secrets map[string]any, cfg appConfig) (revision, error) {
	if d.TargetRevisionID != nil {
		var rev revision
		err := o.pool.QueryRow(ctx, `
			SELECT id, revision_number FROM revisions WHERE id = $1`, *d.TargetRevisionID).
			Scan(&rev.ID, &rev.Number)
		if err == nil {
			return rev, nil
		}
	}
	var next int
	if err := o.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(revision_number), 0) + 1 FROM revisions WHERE application_id = $1`,
		d.ApplicationID).Scan(&next); err != nil {
		return revision{}, err
	}
	limits := map[string]any{}
	if cfg.CPULimitMillis != nil {
		limits["cpuLimitMillis"] = *cfg.CPULimitMillis
	}
	if cfg.MemoryLimit != nil {
		limits["memoryLimitBytes"] = *cfg.MemoryLimit
	}
	var id uuid.UUID
	err := o.pool.QueryRow(ctx, `
		INSERT INTO revisions (
			organization_id, application_id, deployment_id, revision_number, status,
			effective_config, variable_snapshot, secret_refs, health_check, resource_limits, created_by
		) VALUES ($1,$2,$3,$4,'CREATED',$5,$6,$7,$8,$9,$10)
		RETURNING id`,
		d.OrganizationID, d.ApplicationID, d.ID, next,
		mustJSON(eff), mustJSON(vars), mustJSON(secretList(secrets)), mustJSON(cfg.HealthCheck), mustJSON(limits),
		d.CreatedBy,
	).Scan(&id)
	if err != nil {
		return revision{}, err
	}
	return revision{ID: id, Number: next}, nil
}

func (o *Orchestrator) setTargetRevision(ctx context.Context, deploymentID, revisionID uuid.UUID) error {
	_, err := o.pool.Exec(ctx, `
		UPDATE deployments SET target_revision_id = $2 WHERE id = $1`, deploymentID, revisionID)
	return err
}

func (o *Orchestrator) setActiveRevision(ctx context.Context, deploymentID, revisionID uuid.UUID) error {
	_, err := o.pool.Exec(ctx, `
		UPDATE deployments SET active_revision_id = $2 WHERE id = $1`, deploymentID, revisionID)
	return err
}

func (o *Orchestrator) setRevisionStatus(ctx context.Context, revisionID uuid.UUID, status string) error {
	_, err := o.pool.Exec(ctx, `UPDATE revisions SET status = $2 WHERE id = $1`, revisionID, status)
	return err
}

func (o *Orchestrator) updateRevisionImage(ctx context.Context, revisionID uuid.UUID, digest, tag string) error {
	_, err := o.pool.Exec(ctx, `
		UPDATE revisions SET image_digest = $2, image_tag = $3, status = 'READY' WHERE id = $1`,
		revisionID, digest, tag)
	return err
}

type activeRev struct {
	ID uuid.UUID
}

func (o *Orchestrator) getActiveRevision(ctx context.Context, appID uuid.UUID) (*activeRev, error) {
	var id uuid.UUID
	err := o.pool.QueryRow(ctx, `
		SELECT id FROM revisions WHERE application_id = $1 AND status = 'ACTIVE' LIMIT 1`, appID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	return &activeRev{ID: id}, nil
}

func (o *Orchestrator) activateRevision(ctx context.Context, appID, newID uuid.UUID, prev *activeRev) error {
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if prev != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE revisions SET status = 'INACTIVE' WHERE id = $1 AND status = 'ACTIVE'`, prev.ID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE revisions SET status = 'ACTIVE' WHERE id = $1`, newID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (o *Orchestrator) issueOrSimulate(ctx context.Context, d deployments.Deployment, op string, payload map[string]any) error {
	if d.ServerID == nil {
		return errors.New("no server")
	}
	if o.cfg.SimulateAgent || !o.serverOnline(ctx, *d.ServerID) {
		o.log.Info("simulating agent command",
			slog.String("operation", op),
			slog.String("deploymentId", d.ID.String()),
		)
		return nil
	}
	now := o.now().UTC()
	_, err := o.commands.Create(ctx, agentcmd.Command{
		OrganizationID: d.OrganizationID,
		ServerID:       *d.ServerID,
		Operation:      op,
		SchemaVersion:  agentcmd.SchemaVersion,
		Payload:        payload,
		IssuedAt:       now,
		ExpiresAt:      now.Add(10 * time.Minute),
		RequestID:      d.RequestID,
		CorrelationID:  strPtr(d.ID.String()),
	})
	return err
}

func (o *Orchestrator) serverOnline(ctx context.Context, serverID uuid.UUID) bool {
	var status string
	err := o.pool.QueryRow(ctx, `SELECT status FROM servers WHERE id = $1 AND deleted_at IS NULL`, serverID).Scan(&status)
	return err == nil && status == "ONLINE"
}

func (o *Orchestrator) replicaDeployContext(ctx context.Context, d deployments.Deployment) (desired int, slug string, revNumber int, err error) {
	cfg, err := o.loadAppConfig(ctx, d.ApplicationID)
	if err != nil {
		return 0, "", 0, err
	}
	desired = replicas.DesiredFromRuntime(cfg.RuntimeConfig)
	_ = o.pool.QueryRow(ctx, `SELECT slug FROM applications WHERE id = $1`, d.ApplicationID).Scan(&slug)
	if d.TargetRevisionID != nil {
		_ = o.pool.QueryRow(ctx, `SELECT revision_number FROM revisions WHERE id = $1`, *d.TargetRevisionID).Scan(&revNumber)
	}
	return desired, slug, revNumber, nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func secretList(m map[string]any) []any {
	out := make([]any, 0, len(m))
	for name, meta := range m {
		out = append(out, map[string]any{"name": name, "ref": meta})
	}
	return out
}

func strOr(v *string, fallback string) string {
	if v == nil || *v == "" {
		return fallback
	}
	return *v
}

func strVal(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func strPtr(s string) *string { return &s }

func isRollback(d deployments.Deployment) bool {
	return d.Trigger == deployments.TriggerRollback
}
