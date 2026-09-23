package replicas

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agentcmd"
	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Reconciler brings observed replica slots in line with desiredReplicas.
// Used for scale up/down and unhealthy replacement (B29). Full control-plane
// reconciliation loop is B30.
type Reconciler struct {
	pool     *pgxpool.Pool
	repo     Repository
	commands *agentcmd.PostgresRepository
	log      *slog.Logger
	simulate bool
	now      func() time.Time
}

func NewReconciler(pool *pgxpool.Pool, repo Repository, log *slog.Logger, simulateAgent bool) *Reconciler {
	return &Reconciler{
		pool:     pool,
		repo:     repo,
		commands: agentcmd.NewPostgresRepository(pool),
		log:      log,
		simulate: simulateAgent,
		now:      time.Now,
	}
}

func (r *Reconciler) JobHandler() jobs.Handler {
	return func(ctx context.Context, job jobs.Job) error {
		raw, _ := job.Payload["applicationId"].(string)
		appID, err := uuid.Parse(raw)
		if err != nil {
			return fmt.Errorf("invalid applicationId: %w", err)
		}
		replaceIdx := -1
		if v, ok := job.Payload["replaceIndex"].(float64); ok {
			replaceIdx = int(v)
		}
		return r.Reconcile(ctx, appID, replaceIdx)
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, appID uuid.UUID, replaceIndex int) error {
	// Do not scale/replace while a deployment is mid-flight — that would bypass
	// health/activation gates and can disturb zero-downtime handoff (I5).
	if active, err := r.hasInFlightDeployment(ctx, appID); err != nil {
		return err
	} else if active {
		r.log.Info("skipping replica reconcile; deployment in progress",
			slog.String("applicationId", appID.String()),
		)
		return nil
	}

	app, err := r.repo.GetApplicationMeta(ctx, appID)
	if err != nil {
		return err
	}
	runtime, _, err := r.repo.GetLatestConfigRuntime(ctx, appID)
	if err != nil {
		return err
	}
	desired := DesiredFromRuntime(runtime)
	list, err := r.repo.ListByApplication(ctx, appID)
	if err != nil {
		return err
	}
	byIndex := map[int]Replica{}
	for _, rep := range list {
		byIndex[rep.ReplicaIndex] = rep
	}

	var activeRev *uuid.UUID
	var revNumber int
	_ = r.pool.QueryRow(ctx, `
		SELECT id, revision_number FROM revisions
		WHERE application_id = $1 AND status = 'ACTIVE'
		ORDER BY revision_number DESC LIMIT 1`, appID).Scan(&activeRev, &revNumber)

	for i := 0; i < desired; i++ {
		rep, ok := byIndex[i]
		needReplace := replaceIndex == i || (ok && rep.Status == StatusUnhealthy)
		if ok && !needReplace && rep.Status != StatusStopped && rep.Status != StatusFailed {
			continue
		}
		name := ContainerName(app.Slug, revNumber, i)
		if app.ServerID != nil {
			if open, err := r.hasOpenLifecycleCommand(ctx, *app.ServerID, appID, name); err != nil {
				return err
			} else if open {
				r.log.Info("skipping replica ensure; lifecycle command already open",
					slog.String("applicationId", appID.String()),
					slog.String("containerName", name),
				)
				continue
			}
		}
		slot, err := r.repo.UpsertSlot(ctx, UpsertInput{
			OrganizationID: app.OrganizationID,
			ApplicationID:  appID,
			RevisionID:     activeRev,
			ServerID:       app.ServerID,
			ReplicaIndex:   i,
			ContainerName:  name,
			Status:         StatusPending,
		})
		if err != nil {
			return err
		}
		if err := r.issueReplicaLifecycle(ctx, app, slot, activeRev, "scale_up"); err != nil {
			r.log.Warn("replica ensure failed", slog.String("error", err.Error()), slog.Int("index", i))
			msg := err.Error()
			_, _ = r.repo.UpdateStatus(ctx, slot.ID, StatusFailed, nil, nil, nil, &msg)
			continue
		}
		// Commands issued asynchronously — do not claim healthy/routed until
		// a deployment activation path (or future command completion) confirms.
		healthy := false
		routing := false
		_, _ = r.repo.UpdateStatus(ctx, slot.ID, StatusStarting, &healthy, &routing, nil, nil)
	}

	var stopIndexes []int
	for _, rep := range list {
		if rep.ReplicaIndex < desired {
			continue
		}
		stopIndexes = append(stopIndexes, rep.ReplicaIndex)
		if err := r.issueStop(ctx, app, rep, "scale_down"); err != nil {
			r.log.Warn("replica scale-down stop failed", slog.String("error", err.Error()))
		}
	}
	if len(stopIndexes) > 0 {
		_ = r.repo.MarkStopped(ctx, appID, stopIndexes)
		_, _ = r.repo.DeleteAboveIndex(ctx, appID, desired)
	}
	return nil
}

func (r *Reconciler) hasInFlightDeployment(ctx context.Context, appID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM deployments
			WHERE application_id = $1
			  AND status NOT IN (
				'RUNNING','CANCELLED','TIMEOUT',
				'SOURCE_FAILED','BUILD_FAILED','IMAGE_FAILED',
				'CONTAINER_FAILED','START_FAILED','HEALTH_CHECK_FAILED','ROUTING_FAILED'
			  )
		)`, appID).Scan(&exists)
	return exists, err
}

// hasOpenLifecycleCommand prevents reconcile from enqueueing duplicate DEPLOY/START
// while a prior command is still pending/accepted/running (I8 R1 recovery).
func (r *Reconciler) hasOpenLifecycleCommand(ctx context.Context, serverID, appID uuid.UUID, containerName string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM agent_commands
			WHERE server_id = $1
			  AND status IN ('pending', 'accepted', 'running')
			  AND operation IN ('DEPLOY_REVISION', 'START_CONTAINER')
			  AND payload->>'applicationId' = $2
			  AND (
				payload->>'containerName' = $3
				OR payload->>'containerName' IS NULL
			  )
		)`, serverID, appID.String(), containerName).Scan(&exists)
	return exists, err
}

func (r *Reconciler) issueReplicaLifecycle(ctx context.Context, app AppMeta, slot Replica, revisionID *uuid.UUID, reason string) error {
	if app.ServerID == nil || r.simulate {
		return nil
	}
	now := r.now().UTC()
	payload := map[string]any{
		"applicationId":  app.ID.String(),
		"replicaIndex":   slot.ReplicaIndex,
		"containerName":  slot.ContainerName,
		"reason":         reason,
		"routingEnabled": false, // production routing only after deployment activation
		"desiredAction":  "ensure_running",
		"routingLabels": map[string]string{
			"deploycore.application.id": app.ID.String(),
			"deploycore.replica.index":  fmt.Sprintf("%d", slot.ReplicaIndex),
			"traefik.enable":            "false",
		},
	}
	if revisionID != nil {
		payload["revisionId"] = revisionID.String()
	}
	_, err := r.commands.Create(ctx, agentcmd.Command{
		OrganizationID: app.OrganizationID,
		ServerID:       *app.ServerID,
		Operation:      protocol.OpDeployRevision,
		SchemaVersion:  protocol.SchemaVersion,
		Payload:        payload,
		IssuedAt:       now,
		ExpiresAt:      now.Add(10 * time.Minute),
	})
	if err != nil {
		return err
	}
	_, err = r.commands.Create(ctx, agentcmd.Command{
		OrganizationID: app.OrganizationID,
		ServerID:       *app.ServerID,
		Operation:      protocol.OpStartContainer,
		SchemaVersion:  protocol.SchemaVersion,
		Payload: map[string]any{
			"applicationId": app.ID.String(),
			"replicaIndex":  slot.ReplicaIndex,
			"containerName": slot.ContainerName,
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(10 * time.Minute),
	})
	return err
}

func (r *Reconciler) issueStop(ctx context.Context, app AppMeta, rep Replica, reason string) error {
	if app.ServerID == nil || r.simulate {
		return nil
	}
	now := r.now().UTC()
	_, err := r.commands.Create(ctx, agentcmd.Command{
		OrganizationID: app.OrganizationID,
		ServerID:       *app.ServerID,
		Operation:      protocol.OpStopContainer,
		SchemaVersion:  protocol.SchemaVersion,
		Payload: map[string]any{
			"applicationId": app.ID.String(),
			"replicaIndex":  rep.ReplicaIndex,
			"containerName": rep.ContainerName,
			"reason":        reason,
			"drainRouting":  true,
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(10 * time.Minute),
	})
	return err
}
