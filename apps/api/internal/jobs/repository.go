package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("job not found")
	ErrConflict      = errors.New("job conflict")
	ErrNotLeaseOwner = errors.New("not lease owner")
	ErrNotClaimable  = errors.New("job not claimable")
)

type Repository interface {
	Enqueue(ctx context.Context, in EnqueueInput) (Job, error)
	Claim(ctx context.Context, owner string, leaseTTL time.Duration, now time.Time) (Job, error)
	Heartbeat(ctx context.Context, id uuid.UUID, owner string, leaseTTL time.Duration, now time.Time) error
	MarkRunning(ctx context.Context, id uuid.UUID, owner string, now time.Time) (Job, error)
	Complete(ctx context.Context, id uuid.UUID, owner string, now time.Time) (Job, error)
	Fail(ctx context.Context, id uuid.UUID, owner, errMsg string, retryDelay time.Duration, now time.Time) (Job, error)
	Release(ctx context.Context, id uuid.UUID, owner string, delay time.Duration, now time.Time) (Job, error)
	Cancel(ctx context.Context, id uuid.UUID, now time.Time) (Job, error)
	Get(ctx context.Context, id uuid.UUID) (Job, error)
	ReclaimExpired(ctx context.Context, now time.Time) (int64, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Enqueue(ctx context.Context, in EnqueueInput) (Job, error) {
	if !validType(in.Type) {
		return Job{}, errors.New("invalid job type")
	}
	maxAttempts := in.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	available := time.Now().UTC()
	if in.AvailableAt != nil {
		available = in.AvailableAt.UTC()
	}
	const q = `
		INSERT INTO jobs (
			organization_id, type, status, payload, idempotency_key, max_attempts,
			available_at, request_id, correlation_id, related_resource_type, related_resource_id
		) VALUES ($1,$2,'queued',$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, organization_id, type, status, payload, idempotency_key, attempt_count, max_attempts,
		          available_at, leased_until, lease_owner, last_error, request_id, correlation_id,
		          related_resource_type, related_resource_id, created_at, updated_at, finished_at`
	job, err := scanJob(r.pool.QueryRow(ctx, q,
		in.OrganizationID, in.Type, mustJSON(mapOrEmpty(in.Payload)), in.IdempotencyKey, maxAttempts,
		available, in.RequestID, in.CorrelationID, in.RelatedResourceType, in.RelatedResourceID,
	))
	if isUniqueViolation(err) {
		return Job{}, ErrConflict
	}
	return job, err
}

func (r *PostgresRepository) Claim(ctx context.Context, owner string, leaseTTL time.Duration, now time.Time) (Job, error) {
	leaseUntil := now.Add(leaseTTL)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id FROM jobs
		WHERE available_at <= $1
		  AND (
		    status = 'queued'
		    OR (status IN ('leased', 'running') AND leased_until IS NOT NULL AND leased_until < $1)
		  )
		ORDER BY available_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1`, now).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}

	const upd = `
		UPDATE jobs
		SET status = 'leased',
		    lease_owner = $2,
		    leased_until = $3,
		    attempt_count = attempt_count + 1,
		    last_error = NULL,
		    finished_at = NULL
		WHERE id = $1
		RETURNING id, organization_id, type, status, payload, idempotency_key, attempt_count, max_attempts,
		          available_at, leased_until, lease_owner, last_error, request_id, correlation_id,
		          related_resource_type, related_resource_id, created_at, updated_at, finished_at`
	job, err := scanJob(tx.QueryRow(ctx, upd, id, owner, leaseUntil))
	if err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (r *PostgresRepository) Heartbeat(ctx context.Context, id uuid.UUID, owner string, leaseTTL time.Duration, now time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE jobs
		SET leased_until = $3
		WHERE id = $1 AND lease_owner = $2 AND status IN ('leased', 'running')`,
		id, owner, now.Add(leaseTTL))
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotLeaseOwner
	}
	return nil
}

func (r *PostgresRepository) MarkRunning(ctx context.Context, id uuid.UUID, owner string, now time.Time) (Job, error) {
	const q = `
		UPDATE jobs
		SET status = 'running', leased_until = COALESCE(leased_until, $3)
		WHERE id = $1 AND lease_owner = $2 AND status = 'leased'
		RETURNING id, organization_id, type, status, payload, idempotency_key, attempt_count, max_attempts,
		          available_at, leased_until, lease_owner, last_error, request_id, correlation_id,
		          related_resource_type, related_resource_id, created_at, updated_at, finished_at`
	job, err := scanJob(r.pool.QueryRow(ctx, q, id, owner, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotLeaseOwner
	}
	return job, err
}

func (r *PostgresRepository) Complete(ctx context.Context, id uuid.UUID, owner string, now time.Time) (Job, error) {
	const q = `
		UPDATE jobs
		SET status = 'succeeded',
		    finished_at = $3,
		    leased_until = NULL,
		    lease_owner = NULL,
		    last_error = NULL
		WHERE id = $1 AND lease_owner = $2 AND status IN ('leased', 'running')
		RETURNING id, organization_id, type, status, payload, idempotency_key, attempt_count, max_attempts,
		          available_at, leased_until, lease_owner, last_error, request_id, correlation_id,
		          related_resource_type, related_resource_id, created_at, updated_at, finished_at`
	job, err := scanJob(r.pool.QueryRow(ctx, q, id, owner, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotLeaseOwner
	}
	return job, err
}

func (r *PostgresRepository) Fail(ctx context.Context, id uuid.UUID, owner, errMsg string, retryDelay time.Duration, now time.Time) (Job, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var attempt, maxAttempts int
	var status string
	err = tx.QueryRow(ctx, `
		SELECT attempt_count, max_attempts, status FROM jobs
		WHERE id = $1 AND lease_owner = $2 AND status IN ('leased', 'running')
		FOR UPDATE`, id, owner).Scan(&attempt, &maxAttempts, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotLeaseOwner
	}
	if err != nil {
		return Job{}, err
	}

	nextStatus := StatusQueued
	var finished *time.Time
	availableAt := now.Add(retryDelay)
	if attempt >= maxAttempts {
		nextStatus = StatusDead
		finished = &now
		availableAt = now
	} else if nextStatus == StatusQueued && retryDelay < 0 {
		availableAt = now
	}

	const q = `
		UPDATE jobs
		SET status = $2,
		    available_at = $3,
		    finished_at = $4,
		    leased_until = NULL,
		    lease_owner = NULL,
		    last_error = $5
		WHERE id = $1
		RETURNING id, organization_id, type, status, payload, idempotency_key, attempt_count, max_attempts,
		          available_at, leased_until, lease_owner, last_error, request_id, correlation_id,
		          related_resource_type, related_resource_id, created_at, updated_at, finished_at`
	job, err := scanJob(tx.QueryRow(ctx, q, id, nextStatus, availableAt, finished, errMsg))
	if err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (r *PostgresRepository) Release(ctx context.Context, id uuid.UUID, owner string, delay time.Duration, now time.Time) (Job, error) {
	// Soft release: back to queued without consuming a failure; decrement attempt from claim.
	const q = `
		UPDATE jobs
		SET status = 'queued',
		    available_at = $3,
		    leased_until = NULL,
		    lease_owner = NULL,
		    attempt_count = GREATEST(attempt_count - 1, 0),
		    finished_at = NULL
		WHERE id = $1 AND lease_owner = $2 AND status IN ('leased', 'running')
		RETURNING id, organization_id, type, status, payload, idempotency_key, attempt_count, max_attempts,
		          available_at, leased_until, lease_owner, last_error, request_id, correlation_id,
		          related_resource_type, related_resource_id, created_at, updated_at, finished_at`
	job, err := scanJob(r.pool.QueryRow(ctx, q, id, owner, now.Add(delay)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotLeaseOwner
	}
	return job, err
}

func (r *PostgresRepository) Cancel(ctx context.Context, id uuid.UUID, now time.Time) (Job, error) {
	const q = `
		UPDATE jobs
		SET status = 'cancelled',
		    finished_at = $2,
		    leased_until = NULL,
		    lease_owner = NULL
		WHERE id = $1 AND status IN ('queued', 'leased', 'running')
		RETURNING id, organization_id, type, status, payload, idempotency_key, attempt_count, max_attempts,
		          available_at, leased_until, lease_owner, last_error, request_id, correlation_id,
		          related_resource_type, related_resource_id, created_at, updated_at, finished_at`
	job, err := scanJob(r.pool.QueryRow(ctx, q, id, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return job, err
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Job, error) {
	const q = `
		SELECT id, organization_id, type, status, payload, idempotency_key, attempt_count, max_attempts,
		       available_at, leased_until, lease_owner, last_error, request_id, correlation_id,
		       related_resource_type, related_resource_id, created_at, updated_at, finished_at
		FROM jobs WHERE id = $1`
	job, err := scanJob(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return job, err
}

func (r *PostgresRepository) ReclaimExpired(ctx context.Context, now time.Time) (int64, error) {
	ct, err := r.pool.Exec(ctx, `
		UPDATE jobs
		SET status = 'queued',
		    available_at = $1,
		    leased_until = NULL,
		    lease_owner = NULL,
		    last_error = COALESCE(last_error, 'lease expired')
		WHERE status IN ('leased', 'running')
		  AND leased_until IS NOT NULL
		  AND leased_until < $1`, now)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanJob(row scannable) (Job, error) {
	var j Job
	var payload []byte
	err := row.Scan(
		&j.ID, &j.OrganizationID, &j.Type, &j.Status, &payload, &j.IdempotencyKey, &j.AttemptCount, &j.MaxAttempts,
		&j.AvailableAt, &j.LeasedUntil, &j.LeaseOwner, &j.LastError, &j.RequestID, &j.CorrelationID,
		&j.RelatedResourceType, &j.RelatedResourceID, &j.CreatedAt, &j.UpdatedAt, &j.FinishedAt,
	)
	if err != nil {
		return Job{}, err
	}
	j.Payload = map[string]any{}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &j.Payload)
	}
	return j, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func validType(t string) bool {
	switch t {
	case TypeDeploymentExecution, TypeBackup, TypeRestore, TypeCertificateOperation, TypeNotificationDelivery, TypeWebhookDelivery, TypeReplicasReconcile, TypeDesiredStateReconcile:
		return true
	default:
		return false
	}
}
