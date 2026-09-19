package jobs_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b12_test?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, url)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.NewMigrator(pool).Up(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM jobs`)
	return pool
}

func TestClaimSkipLockedAndComplete(t *testing.T) {
	pool := testPool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := jobs.NewQueue(jobs.NewPostgresRepository(pool), log)

	job, err := q.Enqueue(context.Background(), jobs.EnqueueInput{
		Type:    jobs.TypeBackup,
		Payload: map[string]any{"target": "db-1"},
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	claimed, err := q.Claim(context.Background(), "worker-a", 10*time.Second)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != job.ID || claimed.Status != jobs.StatusLeased || claimed.AttemptCount != 1 {
		t.Fatalf("claimed=%#v", claimed)
	}

	// Second worker cannot claim the same leased job.
	_, err = q.Claim(context.Background(), "worker-b", 10*time.Second)
	if !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("second claim err=%v", err)
	}

	if _, err := q.MarkRunning(context.Background(), claimed.ID, "worker-a"); err != nil {
		t.Fatalf("running: %v", err)
	}
	if err := q.Heartbeat(context.Background(), claimed.ID, "worker-a", 10*time.Second); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	done, err := q.Complete(context.Background(), claimed.ID, "worker-a")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if done.Status != jobs.StatusSucceeded || done.FinishedAt == nil {
		t.Fatalf("done=%#v", done)
	}
}

func TestFailRetriesThenDead(t *testing.T) {
	pool := testPool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := jobs.NewQueue(jobs.NewPostgresRepository(pool), log)

	job, err := q.Enqueue(context.Background(), jobs.EnqueueInput{
		Type:        jobs.TypeRestore,
		MaxAttempts: 2,
		Payload:     map[string]any{},
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	c1, err := q.Claim(context.Background(), "w", time.Minute)
	if err != nil {
		t.Fatalf("claim1: %v", err)
	}
	failed, err := q.Fail(context.Background(), c1.ID, "w", "boom", time.Millisecond)
	if err != nil {
		t.Fatalf("fail1: %v", err)
	}
	if failed.Status != jobs.StatusQueued {
		t.Fatalf("status=%s", failed.Status)
	}

	time.Sleep(5 * time.Millisecond)
	c2, err := q.Claim(context.Background(), "w", time.Minute)
	if err != nil {
		t.Fatalf("claim2: %v", err)
	}
	if c2.ID != job.ID || c2.AttemptCount != 2 {
		t.Fatalf("c2=%#v", c2)
	}
	dead, err := q.Fail(context.Background(), c2.ID, "w", "boom again", time.Millisecond)
	if err != nil {
		t.Fatalf("fail2: %v", err)
	}
	if dead.Status != jobs.StatusDead {
		t.Fatalf("status=%s", dead.Status)
	}
}

func TestCancelAndIdempotency(t *testing.T) {
	pool := testPool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := jobs.NewQueue(jobs.NewPostgresRepository(pool), log)

	key := "backup-once"
	job, err := q.Enqueue(context.Background(), jobs.EnqueueInput{
		Type:           jobs.TypeBackup,
		IdempotencyKey: &key,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	_, err = q.Enqueue(context.Background(), jobs.EnqueueInput{
		Type:           jobs.TypeBackup,
		IdempotencyKey: &key,
	})
	if !errors.Is(err, jobs.ErrConflict) {
		t.Fatalf("dup err=%v", err)
	}

	cancelled, err := q.Cancel(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Status != jobs.StatusCancelled {
		t.Fatalf("status=%s", cancelled.Status)
	}
}

func TestExpiredLeaseReclaim(t *testing.T) {
	pool := testPool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := jobs.NewPostgresRepository(pool)
	q := jobs.NewQueue(repo, log)

	job, err := q.Enqueue(context.Background(), jobs.EnqueueInput{Type: jobs.TypeNotificationDelivery})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed, err := q.Claim(context.Background(), "stale-worker", time.Millisecond)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	n, err := q.ReclaimExpired(context.Background())
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if n < 1 {
		t.Fatalf("reclaimed=%d", n)
	}
	again, err := q.Claim(context.Background(), "fresh-worker", time.Minute)
	if err != nil {
		t.Fatalf("reclaim claim: %v", err)
	}
	if again.ID != job.ID || again.ID != claimed.ID {
		t.Fatalf("again=%s claimed=%s job=%s", again.ID, claimed.ID, job.ID)
	}
}

func TestWorkerProcessesHandler(t *testing.T) {
	pool := testPool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := jobs.NewQueue(jobs.NewPostgresRepository(pool), log)

	var ran atomic.Int32
	reg := jobs.Registry{}
	reg.Register(jobs.TypeCertificateOperation, func(ctx context.Context, job jobs.Job) error {
		ran.Add(1)
		return nil
	})

	_, err := q.Enqueue(context.Background(), jobs.EnqueueInput{
		Type:    jobs.TypeCertificateOperation,
		Payload: map[string]any{"domain": "example.com"},
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := jobs.NewWorker(q, reg, jobs.WorkerConfig{
		WorkerID:     "test-worker",
		LeaseTTL:     5 * time.Second,
		PollInterval: 20 * time.Millisecond,
	}, log)
	go w.Run(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ran.Load() >= 1 {
			cancel()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("worker did not process job")
}

func TestWorkerRetryLaterReleases(t *testing.T) {
	pool := testPool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := jobs.NewQueue(jobs.NewPostgresRepository(pool), log)

	job, err := q.Enqueue(context.Background(), jobs.EnqueueInput{Type: jobs.TypeDeploymentExecution})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	reg := jobs.Registry{}
	reg.Register(jobs.TypeDeploymentExecution, func(ctx context.Context, job jobs.Job) error {
		return jobs.ErrRetryLater
	})
	w := jobs.NewWorker(q, reg, jobs.WorkerConfig{
		WorkerID:     "defer-worker",
		LeaseTTL:     time.Second,
		PollInterval: 10 * time.Millisecond,
		DeferDelay:   50 * time.Millisecond,
	}, log)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	// processOne path via short Run
	go w.Run(ctx)
	<-ctx.Done()

	got, err := q.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != jobs.StatusQueued {
		t.Fatalf("status=%s want queued after defer", got.Status)
	}
}
