package healthchecks

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	GetApplicationOrg(ctx context.Context, appID uuid.UUID) (uuid.UUID, error)
	GetStatus(ctx context.Context, appID uuid.UUID) (Status, error)
	UpsertStatus(ctx context.Context, st Status) (Status, error)
	InsertSample(ctx context.Context, s Sample) error
	TrimSamples(ctx context.Context, appID uuid.UUID, keep int) error
	ListSamples(ctx context.Context, appID uuid.UUID, limit int) ([]Sample, error)
	LoadHealthCheckConfig(ctx context.Context, appID uuid.UUID) (map[string]any, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetApplicationOrg(ctx context.Context, appID uuid.UUID) (uuid.UUID, error) {
	var orgID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT organization_id FROM applications WHERE id = $1 AND deleted_at IS NULL`, appID).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return orgID, err
}

func (r *PostgresRepository) GetStatus(ctx context.Context, appID uuid.UUID) (Status, error) {
	const q = `
		SELECT application_id, organization_id, revision_id, deployment_id, state,
		       consecutive_successes, consecutive_failures, last_probe_at, last_success_at,
		       last_failure_at, last_message, probe_type, updated_at
		FROM application_health WHERE application_id = $1`
	st, err := scanStatus(r.pool.QueryRow(ctx, q, appID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{}, ErrNotFound
	}
	return st, err
}

func (r *PostgresRepository) UpsertStatus(ctx context.Context, st Status) (Status, error) {
	const q = `
		INSERT INTO application_health (
			application_id, organization_id, revision_id, deployment_id, state,
			consecutive_successes, consecutive_failures, last_probe_at, last_success_at,
			last_failure_at, last_message, probe_type
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (application_id) DO UPDATE SET
			organization_id = EXCLUDED.organization_id,
			revision_id = EXCLUDED.revision_id,
			deployment_id = EXCLUDED.deployment_id,
			state = EXCLUDED.state,
			consecutive_successes = EXCLUDED.consecutive_successes,
			consecutive_failures = EXCLUDED.consecutive_failures,
			last_probe_at = EXCLUDED.last_probe_at,
			last_success_at = EXCLUDED.last_success_at,
			last_failure_at = EXCLUDED.last_failure_at,
			last_message = EXCLUDED.last_message,
			probe_type = EXCLUDED.probe_type
		RETURNING application_id, organization_id, revision_id, deployment_id, state,
		          consecutive_successes, consecutive_failures, last_probe_at, last_success_at,
		          last_failure_at, last_message, probe_type, updated_at`
	return scanStatus(r.pool.QueryRow(ctx, q,
		st.ApplicationID, st.OrganizationID, st.RevisionID, st.DeploymentID, st.State,
		st.ConsecutiveSuccesses, st.ConsecutiveFailures, st.LastProbeAt, st.LastSuccessAt,
		st.LastFailureAt, st.LastMessage, st.ProbeType,
	))
}

func (r *PostgresRepository) InsertSample(ctx context.Context, s Sample) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO health_probe_samples (
			organization_id, application_id, revision_id, deployment_id,
			success, probe_type, message, latency_ms
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		s.OrganizationID, s.ApplicationID, s.RevisionID, s.DeploymentID,
		s.Success, s.ProbeType, s.Message, s.LatencyMs,
	)
	return err
}

func (r *PostgresRepository) TrimSamples(ctx context.Context, appID uuid.UUID, keep int) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM health_probe_samples
		WHERE application_id = $1
		  AND id NOT IN (
		    SELECT id FROM health_probe_samples
		    WHERE application_id = $1
		    ORDER BY created_at DESC
		    LIMIT $2
		  )`, appID, keep)
	return err
}

func (r *PostgresRepository) ListSamples(ctx context.Context, appID uuid.UUID, limit int) ([]Sample, error) {
	if limit <= 0 || limit > MaxSamplesRetained {
		limit = MaxSamplesRetained
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, application_id, revision_id, deployment_id,
		       success, probe_type, message, latency_ms, created_at
		FROM health_probe_samples
		WHERE application_id = $1
		ORDER BY created_at DESC
		LIMIT $2`, appID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Sample
	for rows.Next() {
		var s Sample
		if err := rows.Scan(
			&s.ID, &s.OrganizationID, &s.ApplicationID, &s.RevisionID, &s.DeploymentID,
			&s.Success, &s.ProbeType, &s.Message, &s.LatencyMs, &s.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) LoadHealthCheckConfig(ctx context.Context, appID uuid.UUID) (map[string]any, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT health_check FROM application_configs
		WHERE application_id = $1
		ORDER BY version DESC LIMIT 1`, appID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanStatus(row scannable) (Status, error) {
	var st Status
	err := row.Scan(
		&st.ApplicationID, &st.OrganizationID, &st.RevisionID, &st.DeploymentID, &st.State,
		&st.ConsecutiveSuccesses, &st.ConsecutiveFailures, &st.LastProbeAt, &st.LastSuccessAt,
		&st.LastFailureAt, &st.LastMessage, &st.ProbeType, &st.UpdatedAt,
	)
	return st, err
}
