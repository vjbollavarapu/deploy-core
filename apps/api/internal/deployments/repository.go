package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrInvalidTransition = errors.New("invalid transition")
	ErrTerminal          = errors.New("terminal state")
)

type ApplicationRef struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  uuid.UUID
	TargetServerID *uuid.UUID
	Status         string
}

type Repository interface {
	GetApplication(ctx context.Context, appID uuid.UUID) (ApplicationRef, error)
	CountActive(ctx context.Context, appID uuid.UUID) (int64, error)
	GetByIdempotency(ctx context.Context, appID uuid.UUID, key string) (Deployment, error)
	CreateQueued(ctx context.Context, orgID, appID, envID uuid.UUID, serverID *uuid.UUID, trigger string, idempotencyKey, correlationID *string, requestID string, createdBy uuid.UUID, now time.Time) (Deployment, error)
	Get(ctx context.Context, id uuid.UUID) (Deployment, error)
	List(ctx context.Context, orgID uuid.UUID, applicationID *uuid.UUID, status *string, limit, offset int) ([]Deployment, int64, error)
	ListEvents(ctx context.Context, deploymentID uuid.UUID) ([]Event, error)
	Transition(ctx context.Context, id uuid.UUID, from string, in TransitionInput, now time.Time) (Deployment, error)
	AdminReconcile(ctx context.Context, id uuid.UUID, from, to, message, requestID string, now time.Time) (Deployment, error)
	EnqueueExecutionJob(ctx context.Context, orgID, deploymentID uuid.UUID, idempotencyKey, requestID, correlationID *string) error
	CancelQueuedJob(ctx context.Context, deploymentID uuid.UUID) error
	SetApplicationStatus(ctx context.Context, appID uuid.UUID, status string) error
	GetRevisionForRollback(ctx context.Context, applicationID, revisionID uuid.UUID) (status string, number int, err error)
	SetTargetRevision(ctx context.Context, deploymentID, revisionID uuid.UUID) error
	GetActiveRevisionID(ctx context.Context, applicationID uuid.UUID) (*uuid.UUID, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetApplication(ctx context.Context, appID uuid.UUID) (ApplicationRef, error) {
	var a ApplicationRef
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, project_id, environment_id, target_server_id, status
		FROM applications WHERE id = $1 AND deleted_at IS NULL`, appID).Scan(
		&a.ID, &a.OrganizationID, &a.ProjectID, &a.EnvironmentID, &a.TargetServerID, &a.Status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ApplicationRef{}, ErrNotFound
	}
	return a, err
}

func (r *PostgresRepository) CountActive(ctx context.Context, appID uuid.UUID) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM deployments
		WHERE application_id = $1 AND finished_at IS NULL
		  AND status NOT IN (
		    'RUNNING', 'CANCELLED', 'TIMEOUT',
		    'SOURCE_FAILED', 'BUILD_FAILED', 'IMAGE_FAILED', 'CONTAINER_FAILED',
		    'START_FAILED', 'HEALTH_CHECK_FAILED', 'ROUTING_FAILED'
		  )`, appID).Scan(&n)
	return n, err
}

func (r *PostgresRepository) GetByIdempotency(ctx context.Context, appID uuid.UUID, key string) (Deployment, error) {
	const q = `
		SELECT id, organization_id, application_id, environment_id, server_id, status, trigger,
		       idempotency_key, request_id, correlation_id, created_by, started_at, finished_at,
		       error_code, error_message, active_revision_id, target_revision_id, created_at, updated_at
		FROM deployments WHERE application_id = $1 AND idempotency_key = $2`
	d, err := scanDeployment(r.pool.QueryRow(ctx, q, appID, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, ErrNotFound
	}
	return d, err
}

func (r *PostgresRepository) CreateQueued(ctx context.Context, orgID, appID, envID uuid.UUID, serverID *uuid.UUID, trigger string, idempotencyKey, correlationID *string, requestID string, createdBy uuid.UUID, now time.Time) (Deployment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Deployment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var reqID *string
	if requestID != "" {
		reqID = &requestID
	}

	var createdByArg any
	if createdBy != uuid.Nil {
		createdByArg = createdBy
	}

	const insertQ = `
		INSERT INTO deployments (
			organization_id, application_id, environment_id, server_id, status, trigger,
			idempotency_key, request_id, correlation_id, created_by, started_at
		) VALUES ($1,$2,$3,$4,'PENDING',$5,$6,$7,$8,$9,$10)
		RETURNING id, organization_id, application_id, environment_id, server_id, status, trigger,
		          idempotency_key, request_id, correlation_id, created_by, started_at, finished_at,
		          error_code, error_message, active_revision_id, target_revision_id, created_at, updated_at`
	d, err := scanDeployment(tx.QueryRow(ctx, insertQ,
		orgID, appID, envID, serverID, trigger, idempotencyKey, reqID, correlationID, createdByArg, now,
	))
	if isUniqueViolation(err) {
		return Deployment{}, ErrConflict
	}
	if err != nil {
		return Deployment{}, err
	}

	if err := insertEventTx(ctx, tx, orgID, d.ID, nil, StatusPending, "deployment created", nil, reqID); err != nil {
		return Deployment{}, err
	}
	from := StatusPending
	if err := insertEventTx(ctx, tx, orgID, d.ID, &from, StatusQueued, "queued for execution", nil, reqID); err != nil {
		return Deployment{}, err
	}

	const updQ = `
		UPDATE deployments SET status = $2
		WHERE id = $1 AND status = $3
		RETURNING id, organization_id, application_id, environment_id, server_id, status, trigger,
		          idempotency_key, request_id, correlation_id, created_by, started_at, finished_at,
		          error_code, error_message, active_revision_id, target_revision_id, created_at, updated_at`
	d, err = scanDeployment(tx.QueryRow(ctx, updQ, d.ID, StatusQueued, StatusPending))
	if err != nil {
		return Deployment{}, err
	}

	if err := enqueueJobTx(ctx, tx, orgID, d.ID, idempotencyKey, reqID, correlationID); err != nil {
		return Deployment{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE applications SET status = 'deploying' WHERE id = $1 AND deleted_at IS NULL`, appID); err != nil {
		return Deployment{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Deployment{}, err
	}
	return d, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Deployment, error) {
	const q = `
		SELECT id, organization_id, application_id, environment_id, server_id, status, trigger,
		       idempotency_key, request_id, correlation_id, created_by, started_at, finished_at,
		       error_code, error_message, active_revision_id, target_revision_id, created_at, updated_at
		FROM deployments WHERE id = $1`
	d, err := scanDeployment(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, ErrNotFound
	}
	return d, err
}

func (r *PostgresRepository) List(ctx context.Context, orgID uuid.UUID, applicationID *uuid.UUID, status *string, limit, offset int) ([]Deployment, int64, error) {
	where := `WHERE organization_id = $1`
	args := []any{orgID}
	n := 2
	if applicationID != nil {
		where += ` AND application_id = $` + itoa(n)
		args = append(args, *applicationID)
		n++
	}
	if status != nil {
		where += ` AND status = $` + itoa(n)
		args = append(args, *status)
		n++
	}
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM deployments `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `
		SELECT id, organization_id, application_id, environment_id, server_id, status, trigger,
		       idempotency_key, request_id, correlation_id, created_by, started_at, finished_at,
		       error_code, error_message, active_revision_id, target_revision_id, created_at, updated_at
		FROM deployments ` + where + `
		ORDER BY created_at DESC LIMIT $` + itoa(n) + ` OFFSET $` + itoa(n+1)
	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Deployment
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) ListEvents(ctx context.Context, deploymentID uuid.UUID) ([]Event, error) {
	const q = `
		SELECT id, organization_id, deployment_id, from_status, to_status, message, metadata, request_id, created_at
		FROM deployment_events WHERE deployment_id = $1 ORDER BY created_at ASC, id ASC`
	rows, err := r.pool.Query(ctx, q, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) Transition(ctx context.Context, id uuid.UUID, from string, in TransitionInput, now time.Time) (Deployment, error) {
	if err := ValidateTransition(from, in.ToStatus); err != nil {
		return Deployment{}, ErrInvalidTransition
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Deployment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var curStatus string
	var orgID, appID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT status, organization_id, application_id FROM deployments WHERE id = $1 FOR UPDATE`, id).
		Scan(&curStatus, &orgID, &appID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, ErrNotFound
	}
	if err != nil {
		return Deployment{}, err
	}
	if IsTerminal(curStatus) {
		return Deployment{}, ErrTerminal
	}
	if curStatus != from {
		return Deployment{}, ErrConflict
	}

	var finishedAt *time.Time
	if IsTerminal(in.ToStatus) {
		t := now
		finishedAt = &t
	}
	var reqID *string
	if in.RequestID != "" {
		reqID = &in.RequestID
	}

	const upd = `
		UPDATE deployments
		SET status = $2,
		    finished_at = COALESCE($3, finished_at),
		    error_code = COALESCE($4, error_code),
		    error_message = COALESCE($5, error_message)
		WHERE id = $1 AND status = $6
		RETURNING id, organization_id, application_id, environment_id, server_id, status, trigger,
		          idempotency_key, request_id, correlation_id, created_by, started_at, finished_at,
		          error_code, error_message, active_revision_id, target_revision_id, created_at, updated_at`
	d, err := scanDeployment(tx.QueryRow(ctx, upd, id, in.ToStatus, finishedAt, in.ErrorCode, in.ErrorMessage, from))
	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, ErrConflict
	}
	if err != nil {
		return Deployment{}, err
	}
	if err := insertEventTx(ctx, tx, orgID, id, &from, in.ToStatus, in.Message, in.Metadata, reqID); err != nil {
		return Deployment{}, err
	}

	if IsTerminal(in.ToStatus) {
		appStatus := "failed"
		if in.ToStatus == StatusRunning {
			appStatus = "running"
		} else if in.ToStatus == StatusCancelled {
			appStatus = "ready"
		}
		if _, err := tx.Exec(ctx, `
			UPDATE applications SET status = $2 WHERE id = $1 AND deleted_at IS NULL`, appID, appStatus); err != nil {
			return Deployment{}, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE jobs SET status = 'cancelled', finished_at = $2
			WHERE related_resource_type = 'deployment' AND related_resource_id = $1
			  AND status IN ('queued', 'leased')`, id, now); err != nil {
			return Deployment{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Deployment{}, err
	}
	return d, nil
}

func (r *PostgresRepository) AdminReconcile(ctx context.Context, id uuid.UUID, from, to, message, requestID string, now time.Time) (Deployment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Deployment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var curStatus string
	var orgID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT status, organization_id FROM deployments WHERE id = $1 FOR UPDATE`, id).
		Scan(&curStatus, &orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, ErrNotFound
	}
	if err != nil {
		return Deployment{}, err
	}
	if curStatus != from {
		return Deployment{}, ErrConflict
	}

	var finishedAt *time.Time
	if IsTerminal(to) {
		t := now
		finishedAt = &t
	} else {
		finishedAt = nil
	}
	var reqID *string
	if requestID != "" {
		reqID = &requestID
	}

	const upd = `
		UPDATE deployments
		SET status = $2,
		    finished_at = CASE WHEN $3::boolean THEN $4 ELSE NULL END,
		    error_code = NULL,
		    error_message = NULL
		WHERE id = $1 AND status = $5
		RETURNING id, organization_id, application_id, environment_id, server_id, status, trigger,
		          idempotency_key, request_id, correlation_id, created_by, started_at, finished_at,
		          error_code, error_message, active_revision_id, target_revision_id, created_at, updated_at`
	d, err := scanDeployment(tx.QueryRow(ctx, upd, id, to, IsTerminal(to), finishedAt, from))
	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, ErrConflict
	}
	if err != nil {
		return Deployment{}, err
	}
	meta := map[string]any{"administrative": true}
	if err := insertEventTx(ctx, tx, orgID, id, &from, to, message, meta, reqID); err != nil {
		return Deployment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Deployment{}, err
	}
	return d, nil
}

func (r *PostgresRepository) EnqueueExecutionJob(ctx context.Context, orgID, deploymentID uuid.UUID, idempotencyKey, requestID, correlationID *string) error {
	return enqueueJobTx(ctx, r.pool, orgID, deploymentID, idempotencyKey, requestID, correlationID)
}

func (r *PostgresRepository) CancelQueuedJob(ctx context.Context, deploymentID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE jobs SET status = 'cancelled', finished_at = NOW()
		WHERE related_resource_type = 'deployment' AND related_resource_id = $1
		  AND status IN ('queued', 'leased')`, deploymentID)
	return err
}

func (r *PostgresRepository) SetApplicationStatus(ctx context.Context, appID uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE applications SET status = $2 WHERE id = $1 AND deleted_at IS NULL`, appID, status)
	return err
}

func (r *PostgresRepository) GetRevisionForRollback(ctx context.Context, applicationID, revisionID uuid.UUID) (string, int, error) {
	var status string
	var number int
	err := r.pool.QueryRow(ctx, `
		SELECT status, revision_number FROM revisions
		WHERE id = $1 AND application_id = $2`, revisionID, applicationID).Scan(&status, &number)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, ErrNotFound
		}
		return "", 0, err
	}
	return status, number, nil
}

func (r *PostgresRepository) SetTargetRevision(ctx context.Context, deploymentID, revisionID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE deployments SET target_revision_id = $2 WHERE id = $1`, deploymentID, revisionID)
	return err
}

func (r *PostgresRepository) GetActiveRevisionID(ctx context.Context, applicationID uuid.UUID) (*uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM revisions
		WHERE application_id = $1 AND status = 'ACTIVE'
		ORDER BY number DESC LIMIT 1`, applicationID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &id, nil
}

type querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func enqueueJobTx(ctx context.Context, q querier, orgID, deploymentID uuid.UUID, idempotencyKey, requestID, correlationID *string) error {
	payload, _ := json.Marshal(map[string]any{"deploymentId": deploymentID.String()})
	var jobKey *string
	if idempotencyKey != nil && *idempotencyKey != "" {
		k := "deployment:" + *idempotencyKey
		jobKey = &k
	}
	_, err := q.Exec(ctx, `
		INSERT INTO jobs (
			organization_id, type, status, payload, idempotency_key, request_id, correlation_id,
			related_resource_type, related_resource_id
		) VALUES ($1, 'DEPLOYMENT_EXECUTION', 'queued', $2, $3, $4, $5, 'deployment', $6)`,
		orgID, payload, jobKey, requestID, correlationID, deploymentID)
	if isUniqueViolation(err) {
		return nil
	}
	return err
}

func insertEventTx(ctx context.Context, q querier, orgID, deploymentID uuid.UUID, from *string, to, message string, metadata map[string]any, requestID *string) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	meta, _ := json.Marshal(metadata)
	_, err := q.Exec(ctx, `
		INSERT INTO deployment_events (
			organization_id, deployment_id, from_status, to_status, message, metadata, request_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7)`, orgID, deploymentID, from, to, message, meta, requestID)
	return err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanDeployment(row scannable) (Deployment, error) {
	var d Deployment
	err := row.Scan(
		&d.ID, &d.OrganizationID, &d.ApplicationID, &d.EnvironmentID, &d.ServerID, &d.Status, &d.Trigger,
		&d.IdempotencyKey, &d.RequestID, &d.CorrelationID, &d.CreatedBy, &d.StartedAt, &d.FinishedAt,
		&d.ErrorCode, &d.ErrorMessage, &d.ActiveRevisionID, &d.TargetRevisionID, &d.CreatedAt, &d.UpdatedAt,
	)
	return d, err
}

func scanEvent(row scannable) (Event, error) {
	var e Event
	var meta []byte
	err := row.Scan(
		&e.ID, &e.OrganizationID, &e.DeploymentID, &e.FromStatus, &e.ToStatus, &e.Message, &meta, &e.RequestID, &e.CreatedAt,
	)
	if err != nil {
		return Event{}, err
	}
	e.Metadata = map[string]any{}
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &e.Metadata)
	}
	return e, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
