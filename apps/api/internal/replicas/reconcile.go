package replicas

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agentcmd"
	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
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
		healthy := true
		routing := true
		_, _ = r.repo.UpdateStatus(ctx, slot.ID, StatusRunning, &healthy, &routing, nil, nil)
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
		"routingEnabled": true,
		"desiredAction":  "ensure_running",
		"routingLabels": map[string]string{
			"deploycore.application.id": app.ID.String(),
			"deploycore.replica.index":  fmt.Sprintf("%d", slot.ReplicaIndex),
			"traefik.enable":            "true",
		},
	}
	if revisionID != nil {
		payload["revisionId"] = revisionID.String()
	}
	_, err := r.commands.Create(ctx, agentcmd.Command{
		OrganizationID: app.OrganizationID,
		ServerID:       *app.ServerID,
		Operation:      agentcmd.OpDeployRevision,
		SchemaVersion:  agentcmd.SchemaVersion,
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
		Operation:      agentcmd.OpStartContainer,
		SchemaVersion:  agentcmd.SchemaVersion,
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
		Operation:      agentcmd.OpStopContainer,
		SchemaVersion:  agentcmd.SchemaVersion,
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
