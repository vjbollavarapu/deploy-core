package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// ErrRetryLater tells the worker to release the job for later without counting a failure.
var ErrRetryLater = errors.New("retry later")

type Queue struct {
	repo Repository
	log  *slog.Logger
	now  func() time.Time
}

func NewQueue(repo Repository, log *slog.Logger) *Queue {
	return &Queue{repo: repo, log: log, now: time.Now}
}

func (q *Queue) Enqueue(ctx context.Context, in EnqueueInput) (Job, error) {
	job, err := q.repo.Enqueue(ctx, in)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Job{}, fmt.Errorf("%w: duplicate idempotency key", ErrConflict)
		}
		return Job{}, err
	}
	return job, nil
}

func (q *Queue) Claim(ctx context.Context, owner string, leaseTTL time.Duration) (Job, error) {
	return q.repo.Claim(ctx, owner, leaseTTL, q.now().UTC())
}

func (q *Queue) Heartbeat(ctx context.Context, id uuid.UUID, owner string, leaseTTL time.Duration) error {
	return q.repo.Heartbeat(ctx, id, owner, leaseTTL, q.now().UTC())
}

func (q *Queue) MarkRunning(ctx context.Context, id uuid.UUID, owner string) (Job, error) {
	return q.repo.MarkRunning(ctx, id, owner, q.now().UTC())
}

func (q *Queue) Complete(ctx context.Context, id uuid.UUID, owner string) (Job, error) {
	return q.repo.Complete(ctx, id, owner, q.now().UTC())
}

func (q *Queue) Fail(ctx context.Context, id uuid.UUID, owner, errMsg string, retryDelay time.Duration) (Job, error) {
	return q.repo.Fail(ctx, id, owner, errMsg, retryDelay, q.now().UTC())
}

func (q *Queue) Release(ctx context.Context, id uuid.UUID, owner string, delay time.Duration) (Job, error) {
	return q.repo.Release(ctx, id, owner, delay, q.now().UTC())
}

func (q *Queue) Cancel(ctx context.Context, id uuid.UUID) (Job, error) {
	return q.repo.Cancel(ctx, id, q.now().UTC())
}

func (q *Queue) Get(ctx context.Context, id uuid.UUID) (Job, error) {
	return q.repo.Get(ctx, id)
}

func (q *Queue) ReclaimExpired(ctx context.Context) (int64, error) {
	return q.repo.ReclaimExpired(ctx, q.now().UTC())
}

// Handler processes a claimed job. Return ErrRetryLater to soft-release.
type Handler func(ctx context.Context, job Job) error

type Registry map[string]Handler

func (r Registry) Register(jobType string, h Handler) {
	r[jobType] = h
}

func (r Registry) Get(jobType string) (Handler, bool) {
	h, ok := r[jobType]
	return h, ok
}

// DefaultRegistry returns handlers for known job types.
// DEPLOYMENT_EXECUTION should be replaced by the orchestrator handler at wiring time.
func DefaultRegistry(log *slog.Logger) Registry {
	reg := Registry{}
	reg.Register(TypeDeploymentExecution, func(ctx context.Context, job Job) error {
		log.Warn("deployment execution handler not wired; deferring",
			slog.String("jobId", job.ID.String()),
		)
		return ErrRetryLater
	})
	reg.Register(TypeBackup, noopHandler(log, TypeBackup))
	reg.Register(TypeRestore, noopHandler(log, TypeRestore))
	reg.Register(TypeCertificateOperation, noopHandler(log, TypeCertificateOperation))
	reg.Register(TypeNotificationDelivery, noopHandler(log, TypeNotificationDelivery))
	reg.Register(TypeWebhookDelivery, noopHandler(log, TypeWebhookDelivery))
	reg.Register(TypeReplicasReconcile, noopHandler(log, TypeReplicasReconcile))
	reg.Register(TypeDesiredStateReconcile, noopHandler(log, TypeDesiredStateReconcile))
	return reg
}

func noopHandler(log *slog.Logger, typ string) Handler {
	return func(ctx context.Context, job Job) error {
		log.Info("job handled (noop)", slog.String("type", typ), slog.String("jobId", job.ID.String()))
		return nil
	}
}
