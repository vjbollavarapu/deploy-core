package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

type WorkerConfig struct {
	WorkerID       string
	LeaseTTL       time.Duration
	PollInterval   time.Duration
	RetryBaseDelay time.Duration
	DeferDelay     time.Duration
	Enabled        bool
}

func (c WorkerConfig) withDefaults() WorkerConfig {
	if c.WorkerID == "" {
		c.WorkerID = "worker-1"
	}
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = 30 * time.Second
	}
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if c.RetryBaseDelay <= 0 {
		c.RetryBaseDelay = 5 * time.Second
	}
	if c.DeferDelay <= 0 {
		c.DeferDelay = time.Minute
	}
	return c
}

type Worker struct {
	queue    *Queue
	handlers Registry
	cfg      WorkerConfig
	log      *slog.Logger
}

func NewWorker(queue *Queue, handlers Registry, cfg WorkerConfig, log *slog.Logger) *Worker {
	return &Worker{
		queue:    queue,
		handlers: handlers,
		cfg:      cfg.withDefaults(),
		log:      log,
	}
}

// Run polls and processes jobs until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	w.cfg = w.cfg.withDefaults()
	w.log.Info("job worker started",
		slog.String("workerId", w.cfg.WorkerID),
		slog.Duration("leaseTTL", w.cfg.LeaseTTL),
		slog.Duration("pollInterval", w.cfg.PollInterval),
	)
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			w.log.Info("job worker stopped")
			return
		}
		if _, err := w.queue.ReclaimExpired(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.log.Error("reclaim expired leases failed", slog.String("error", err.Error()))
		}
		processed, err := w.processOne(ctx)
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrNotFound) {
			w.log.Error("job processing failed", slog.String("error", err.Error()))
		}
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			w.log.Info("job worker stopped")
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) processOne(ctx context.Context) (bool, error) {
	job, err := w.queue.Claim(ctx, w.cfg.WorkerID, w.cfg.LeaseTTL)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	handler, ok := w.handlers.Get(job.Type)
	if !ok {
		_, _ = w.queue.Fail(ctx, job.ID, w.cfg.WorkerID, "no handler registered for job type", w.cfg.RetryBaseDelay)
		return true, nil
	}

	if _, err := w.queue.MarkRunning(ctx, job.ID, w.cfg.WorkerID); err != nil {
		return true, err
	}

	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	go w.heartbeatLoop(hbCtx, job.ID)

	runErr := handler(ctx, job)
	hbCancel()

	if runErr != nil {
		if errors.Is(runErr, ErrRetryLater) {
			_, err := w.queue.Release(ctx, job.ID, w.cfg.WorkerID, w.cfg.DeferDelay)
			return true, err
		}
		delay := backoff(w.cfg.RetryBaseDelay, job.AttemptCount)
		_, err := w.queue.Fail(ctx, job.ID, w.cfg.WorkerID, runErr.Error(), delay)
		return true, err
	}

	_, err = w.queue.Complete(ctx, job.ID, w.cfg.WorkerID)
	return true, err
}

func (w *Worker) heartbeatLoop(ctx context.Context, jobID uuid.UUID) {
	interval := w.cfg.LeaseTTL / 3
	if interval < time.Second {
		interval = time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := w.queue.Heartbeat(ctx, jobID, w.cfg.WorkerID, w.cfg.LeaseTTL); err != nil {
				w.log.Warn("job heartbeat failed",
					slog.String("jobId", jobID.String()),
					slog.String("error", err.Error()),
				)
				return
			}
		}
	}
}

func backoff(base time.Duration, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := base
	for i := 1; i < attempt && d < 5*time.Minute; i++ {
		d *= 2
	}
	if d > 5*time.Minute {
		d = 5 * time.Minute
	}
	return d
}
