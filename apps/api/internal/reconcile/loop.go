package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agents"
	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
	"github.com/deploycore/deploy-core/apps/api/internal/replicas"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ServerExpirer marks servers offline when heartbeats expire.
type ServerExpirer interface {
	ExpireStaleHeartbeats(ctx context.Context, ttl time.Duration) (int, error)
}

// ReplicaReconciler aligns observed replica slots with desiredReplicas.
type ReplicaReconciler interface {
	Reconcile(ctx context.Context, appID uuid.UUID, replaceIndex int) error
}

// Loop runs one desired-state reconciliation sweep.
type Loop struct {
	pool       *pgxpool.Pool
	agents     ServerExpirer
	replicas   replicas.Repository
	reconciler ReplicaReconciler
	log        *slog.Logger
	cfg        Config
	now        func() time.Time
}

func NewLoop(pool *pgxpool.Pool, agentSvc *agents.Service, replicaRepo replicas.Repository, replicaRec *replicas.Reconciler, log *slog.Logger, cfg Config) *Loop {
	return &Loop{
		pool:       pool,
		agents:     agentSvc,
		replicas:   replicaRepo,
		reconciler: replicaRec,
		log:        log,
		cfg:        cfg.withDefaults(),
		now:        time.Now,
	}
}

func (l *Loop) JobHandler() jobs.Handler {
	return func(ctx context.Context, job jobs.Job) error {
		return l.Sweep(ctx)
	}
}

// Sweep runs servers → apps → restart-policy phases idempotently.
func (l *Loop) Sweep(ctx context.Context) error {
	servers, err := l.SweepServers(ctx)
	if err != nil {
		return err
	}
	apps, err := l.SweepApps(ctx)
	if err != nil {
		return err
	}
	restarts, err := l.SweepRestarts(ctx)
	if err != nil {
		return err
	}
	l.log.Info("desired-state reconcile sweep complete",
		slog.Int("serversOffline", servers),
		slog.Int("appsReconciled", apps),
		slog.Int("restarts", restarts),
	)
	return nil
}

func (l *Loop) SweepServers(ctx context.Context) (int, error) {
	if l.agents == nil {
		return 0, nil
	}
	return l.agents.ExpireStaleHeartbeats(ctx, l.cfg.HeartbeatTTL)
}

func (l *Loop) SweepApps(ctx context.Context) (int, error) {
	if l.reconciler == nil || l.replicas == nil {
		return 0, nil
	}
	ids, err := l.replicas.ListApplicationIDsForReconcile(ctx, l.cfg.MaxAppActionsPerTick)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, appID := range ids {
		if err := l.reconcileAppIfNeeded(ctx, appID); err != nil {
			l.log.Warn("app reconcile failed",
				slog.String("applicationId", appID.String()),
				slog.String("error", err.Error()),
			)
			continue
		}
		n++
	}
	return n, nil
}

func (l *Loop) reconcileAppIfNeeded(ctx context.Context, appID uuid.UUID) error {
	runtime, _, err := l.replicas.GetLatestConfigRuntime(ctx, appID)
	if err != nil {
		return err
	}
	desired := replicas.DesiredFromRuntime(runtime)
	list, err := l.replicas.ListByApplication(ctx, appID)
	if err != nil {
		return err
	}
	meta, err := l.replicas.GetApplicationMeta(ctx, appID)
	if err != nil {
		return err
	}
	if meta.ServerID != nil {
		var status string
		_ = l.pool.QueryRow(ctx, `
			SELECT status FROM servers WHERE id = $1 AND deleted_at IS NULL`, *meta.ServerID).Scan(&status)
		if status == "OFFLINE" || status == "DISABLED" || status == "MAINTENANCE" {
			return nil // skip until server is eligible
		}
	}

	need := false
	byIndex := map[int]replicas.Replica{}
	healthy := 0
	for _, rep := range list {
		byIndex[rep.ReplicaIndex] = rep
		if rep.Healthy && rep.Status == replicas.StatusRunning {
			healthy++
		}
		if rep.ReplicaIndex >= desired {
			need = true
		}
		if rep.Status == replicas.StatusUnhealthy {
			need = true
		}
	}
	for i := 0; i < desired; i++ {
		rep, ok := byIndex[i]
		if !ok || rep.Status == replicas.StatusStopped || rep.Status == replicas.StatusFailed {
			need = true
			break
		}
	}
	if healthy < desired {
		need = true
	}
	if !need {
		return nil
	}
	return l.reconciler.Reconcile(ctx, appID, -1)
}

func (l *Loop) SweepRestarts(ctx context.Context) (int, error) {
	if l.replicas == nil || l.reconciler == nil {
		return 0, nil
	}
	now := l.now().UTC()
	list, err := l.replicas.ListNeedingRestart(ctx, now, l.cfg.MaxRestartActionsPerTick)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, rep := range list {
		if rep.RestartAttemptCount >= l.cfg.RestartMaxAttempts {
			continue
		}
		// Only restart slots still within desired range.
		runtime, _, err := l.replicas.GetLatestConfigRuntime(ctx, rep.ApplicationID)
		if err != nil {
			continue
		}
		desired := replicas.DesiredFromRuntime(runtime)
		if rep.ReplicaIndex >= desired {
			continue
		}
		policy, err := l.replicas.GetRestartPolicy(ctx, rep.ApplicationID)
		if err != nil {
			continue
		}
		if !shouldRestart(policy, rep) {
			continue
		}
		meta, err := l.replicas.GetApplicationMeta(ctx, rep.ApplicationID)
		if err != nil {
			continue
		}
		if meta.ServerID != nil {
			var status string
			_ = l.pool.QueryRow(ctx, `
				SELECT status FROM servers WHERE id = $1 AND deleted_at IS NULL`, *meta.ServerID).Scan(&status)
			if status == "OFFLINE" || status == "DISABLED" || status == "MAINTENANCE" {
				continue
			}
		}

		backoff := restartBackoff(l.cfg.RestartBackoffBase, rep.RestartAttemptCount)
		next := now.Add(backoff)
		if err := l.replicas.RecordRestartAttempt(ctx, rep.ID, next); err != nil {
			l.log.Warn("record restart attempt failed", slog.String("error", err.Error()))
			continue
		}
		if err := l.reconciler.Reconcile(ctx, rep.ApplicationID, rep.ReplicaIndex); err != nil {
			l.log.Warn("restart reconcile failed",
				slog.String("applicationId", rep.ApplicationID.String()),
				slog.Int("replicaIndex", rep.ReplicaIndex),
				slog.String("error", err.Error()),
			)
			continue
		}
		n++
	}
	return n, nil
}

func shouldRestart(policy string, rep replicas.Replica) bool {
	switch policy {
	case "no":
		return false
	case "always":
		return rep.Status == replicas.StatusFailed || rep.Status == replicas.StatusUnhealthy ||
			(rep.Status == replicas.StatusStopped && !intentionalStop(rep))
	case "on-failure":
		if rep.Status == replicas.StatusFailed || rep.Status == replicas.StatusUnhealthy {
			return true
		}
		if rep.ObservedExitCode != nil && *rep.ObservedExitCode != 0 {
			return true
		}
		return false
	case "unless-stopped":
		if intentionalStop(rep) {
			return false
		}
		return rep.Status == replicas.StatusFailed || rep.Status == replicas.StatusUnhealthy ||
			rep.Status == replicas.StatusStopped
	default:
		return rep.Status == replicas.StatusFailed || rep.Status == replicas.StatusUnhealthy
	}
}

// ShouldRestartForTest exports shouldRestart for unit tests.
func ShouldRestartForTest(policy string, rep replicas.Replica) bool {
	return shouldRestart(policy, rep)
}

func intentionalStop(rep replicas.Replica) bool {
	// Scale-down and drain leave last_error markers; treat explicit STOPPING/STOPPED with scale_down as intentional.
	return rep.Status == replicas.StatusStopping ||
		rep.LastError == "scale_down" ||
		rep.LastError == "retire_previous" ||
		rep.LastError == "rolling_replace"
}

func restartBackoff(base time.Duration, attempts int) time.Duration {
	if attempts <= 0 {
		return base
	}
	mult := math.Pow(2, float64(attempts))
	d := time.Duration(float64(base) * mult)
	if d > 5*time.Minute {
		return 5 * time.Minute
	}
	return d
}

// BucketKey builds a time-bucketed idempotency key for sweep jobs.
func BucketKey(now time.Time, interval time.Duration) string {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	bucket := now.UTC().Unix() / int64(interval.Seconds())
	return fmt.Sprintf("desired-state-reconcile:%d", bucket)
}
