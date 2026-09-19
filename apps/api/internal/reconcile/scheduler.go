package reconcile

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
)

// Scheduler periodically enqueues DESIRED_STATE_RECONCILE with time-bucketed idempotency.
type Scheduler struct {
	queue *jobs.Queue
	cfg   Config
	log   *slog.Logger
	now   func() time.Time
}

func NewScheduler(queue *jobs.Queue, cfg Config, log *slog.Logger) *Scheduler {
	return &Scheduler{
		queue: queue,
		cfg:   cfg.withDefaults(),
		log:   log,
		now:   time.Now,
	}
}

// Run blocks until ctx is cancelled, enqueueing one sweep job per interval bucket.
func (s *Scheduler) Run(ctx context.Context) {
	if !s.cfg.Enabled || s.queue == nil {
		return
	}
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	s.enqueue(ctx) // immediate first tick
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.enqueue(ctx)
		}
	}
}

func (s *Scheduler) enqueue(ctx context.Context) {
	key := BucketKey(s.now(), s.cfg.Interval)
	maxAttempts := 3
	_, err := s.queue.Enqueue(ctx, jobs.EnqueueInput{
		Type:           jobs.TypeDesiredStateReconcile,
		IdempotencyKey: &key,
		MaxAttempts:    maxAttempts,
		Payload: map[string]any{
			"tick": key,
		},
	})
	if err != nil {
		if errors.Is(err, jobs.ErrConflict) {
			return // already queued/finished for this bucket
		}
		s.log.Warn("enqueue desired-state reconcile failed", slog.String("error", err.Error()))
		return
	}
	s.log.Debug("enqueued desired-state reconcile", slog.String("key", key))
}
