package replicas

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("not found")
)

type Repository interface {
	ListByApplication(ctx context.Context, appID uuid.UUID) ([]Replica, error)
	ListNeedingRestart(ctx context.Context, now time.Time, limit int) ([]Replica, error)
	ListApplicationIDsForReconcile(ctx context.Context, limit int) ([]uuid.UUID, error)
	GetByIndex(ctx context.Context, appID uuid.UUID, index int) (Replica, error)
	UpsertSlot(ctx context.Context, in UpsertInput) (Replica, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, healthy, routing *bool, containerID, lastError *string) (Replica, error)
	RecordRestartAttempt(ctx context.Context, id uuid.UUID, nextRestartAt time.Time) error
	ResetRestartBackoff(ctx context.Context, id uuid.UUID) error
	TouchReconcile(ctx context.Context, id uuid.UUID, at time.Time) error
	DeleteAboveIndex(ctx context.Context, appID uuid.UUID, maxIndexExclusive int) (int64, error)
	MarkStopped(ctx context.Context, appID uuid.UUID, indexes []int) error
	CountHealthy(ctx context.Context, appID uuid.UUID) (int, error)
	GetApplicationMeta(ctx context.Context, appID uuid.UUID) (AppMeta, error)
	GetLatestConfigRuntime(ctx context.Context, appID uuid.UUID) (map[string]any, ConfigMeta, error)
	InsertConfigVersion(ctx context.Context, orgID, appID, actorID uuid.UUID, runtime map[string]any) error
	GetRestartPolicy(ctx context.Context, appID uuid.UUID) (string, error)
}

type UpsertInput struct {
	OrganizationID uuid.UUID
	ApplicationID  uuid.UUID
	RevisionID     *uuid.UUID
	ServerID       *uuid.UUID
	ReplicaIndex   int
	ContainerName  string
	Status         string
}

type AppMeta struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Slug           string
	ServerID       *uuid.UUID
}

type ConfigMeta struct {
	Version          int
	SourceType       string
	RepositoryURL    *string
	GitBranch        *string
	DockerfilePath   *string
	BuildContext     *string
	ImageReference   *string
	InternalPort     *int
	Command          *string
	Entrypoint       *string
	CPULimitMillis   *int
	MemoryLimitBytes *int64
	RestartPolicy    string
	HealthCheck      map[string]any
	RuntimeConfig    map[string]any
	AutoDeploy       bool
	GitConnectionID  *uuid.UUID
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ListByApplication(ctx context.Context, appID uuid.UUID) ([]Replica, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, application_id, revision_id, server_id, replica_index,
		       container_name, container_id, status, healthy, routing_enabled,
		       last_probe_at, last_error,
		       restart_attempt_count, next_restart_at, last_reconcile_at, observed_exit_code,
		       created_at, updated_at
		FROM application_replicas
		WHERE application_id = $1
		ORDER BY replica_index ASC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Replica
	for rows.Next() {
		rep, err := scanReplica(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListNeedingRestart(ctx context.Context, now time.Time, limit int) ([]Replica, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, application_id, revision_id, server_id, replica_index,
		       container_name, container_id, status, healthy, routing_enabled,
		       last_probe_at, last_error,
		       restart_attempt_count, next_restart_at, last_reconcile_at, observed_exit_code,
		       created_at, updated_at
		FROM application_replicas
		WHERE status IN ('FAILED', 'UNHEALTHY', 'STOPPED')
		  AND (next_restart_at IS NULL OR next_restart_at <= $1)
		ORDER BY updated_at ASC
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Replica
	for rows.Next() {
		rep, err := scanReplica(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListApplicationIDsForReconcile(ctx context.Context, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT a.id
		FROM applications a
		WHERE a.deleted_at IS NULL
		  AND a.target_server_id IS NOT NULL
		  AND EXISTS (
		    SELECT 1 FROM application_configs ac WHERE ac.application_id = a.id
		  )
		ORDER BY a.updated_at ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetByIndex(ctx context.Context, appID uuid.UUID, index int) (Replica, error) {
	rep, err := scanReplica(r.pool.QueryRow(ctx, `
		SELECT id, organization_id, application_id, revision_id, server_id, replica_index,
		       container_name, container_id, status, healthy, routing_enabled,
		       last_probe_at, last_error,
		       restart_attempt_count, next_restart_at, last_reconcile_at, observed_exit_code,
		       created_at, updated_at
		FROM application_replicas
		WHERE application_id = $1 AND replica_index = $2`, appID, index))
	if errors.Is(err, pgx.ErrNoRows) {
		return Replica{}, ErrNotFound
	}
	return rep, err
}

func (r *PostgresRepository) UpsertSlot(ctx context.Context, in UpsertInput) (Replica, error) {
	status := in.Status
	if status == "" {
		status = StatusPending
	}
	return scanReplica(r.pool.QueryRow(ctx, `
		INSERT INTO application_replicas (
			organization_id, application_id, revision_id, server_id, replica_index,
			container_name, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (application_id, replica_index) DO UPDATE SET
			revision_id = EXCLUDED.revision_id,
			server_id = EXCLUDED.server_id,
			container_name = EXCLUDED.container_name,
			status = EXCLUDED.status,
			healthy = FALSE,
			routing_enabled = FALSE,
			last_error = '',
			updated_at = NOW()
		RETURNING id, organization_id, application_id, revision_id, server_id, replica_index,
		          container_name, container_id, status, healthy, routing_enabled,
		          last_probe_at, last_error,
		          restart_attempt_count, next_restart_at, last_reconcile_at, observed_exit_code,
		          created_at, updated_at`,
		in.OrganizationID, in.ApplicationID, in.RevisionID, in.ServerID, in.ReplicaIndex,
		in.ContainerName, status,
	))
}

func (r *PostgresRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, healthy, routing *bool, containerID, lastError *string) (Replica, error) {
	cur, err := scanReplica(r.pool.QueryRow(ctx, `
		SELECT id, organization_id, application_id, revision_id, server_id, replica_index,
		       container_name, container_id, status, healthy, routing_enabled,
		       last_probe_at, last_error,
		       restart_attempt_count, next_restart_at, last_reconcile_at, observed_exit_code,
		       created_at, updated_at
		FROM application_replicas WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Replica{}, ErrNotFound
	}
	if err != nil {
		return Replica{}, err
	}
	if status != "" {
		cur.Status = status
	}
	if healthy != nil {
		cur.Healthy = *healthy
	}
	if routing != nil {
		cur.RoutingEnabled = *routing
	}
	if containerID != nil {
		cur.ContainerID = containerID
	}
	if lastError != nil {
		cur.LastError = *lastError
	}
	var probe any
	if cur.Healthy || status == StatusUnhealthy || status == StatusRunning {
		now := time.Now().UTC()
		probe = now
		cur.LastProbeAt = &now
	}
	return scanReplica(r.pool.QueryRow(ctx, `
		UPDATE application_replicas SET
			status = $2, healthy = $3, routing_enabled = $4,
			container_id = $5, last_error = $6, last_probe_at = COALESCE($7, last_probe_at),
			restart_attempt_count = CASE WHEN $3 = TRUE AND $2 = 'RUNNING' THEN 0 ELSE restart_attempt_count END,
			next_restart_at = CASE WHEN $3 = TRUE AND $2 = 'RUNNING' THEN NULL ELSE next_restart_at END
		WHERE id = $1
		RETURNING id, organization_id, application_id, revision_id, server_id, replica_index,
		          container_name, container_id, status, healthy, routing_enabled,
		          last_probe_at, last_error,
		          restart_attempt_count, next_restart_at, last_reconcile_at, observed_exit_code,
		          created_at, updated_at`,
		id, cur.Status, cur.Healthy, cur.RoutingEnabled, cur.ContainerID, cur.LastError, probe,
	))
}

func (r *PostgresRepository) RecordRestartAttempt(ctx context.Context, id uuid.UUID, nextRestartAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE application_replicas SET
			restart_attempt_count = restart_attempt_count + 1,
			next_restart_at = $2,
			last_reconcile_at = NOW()
		WHERE id = $1`, id, nextRestartAt)
	return err
}

func (r *PostgresRepository) ResetRestartBackoff(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE application_replicas SET
			restart_attempt_count = 0,
			next_restart_at = NULL,
			last_reconcile_at = NOW()
		WHERE id = $1`, id)
	return err
}

func (r *PostgresRepository) TouchReconcile(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE application_replicas SET last_reconcile_at = $2 WHERE id = $1`, id, at)
	return err
}

func (r *PostgresRepository) GetRestartPolicy(ctx context.Context, appID uuid.UUID) (string, error) {
	var policy string
	err := r.pool.QueryRow(ctx, `
		SELECT restart_policy FROM application_configs
		WHERE application_id = $1
		ORDER BY version DESC LIMIT 1`, appID).Scan(&policy)
	if errors.Is(err, pgx.ErrNoRows) {
		return "unless-stopped", nil
	}
	if err != nil {
		return "", err
	}
	if policy == "" {
		return "unless-stopped", nil
	}
	return policy, nil
}

func (r *PostgresRepository) DeleteAboveIndex(ctx context.Context, appID uuid.UUID, maxIndexExclusive int) (int64, error) {
	ct, err := r.pool.Exec(ctx, `
		DELETE FROM application_replicas
		WHERE application_id = $1 AND replica_index >= $2`, appID, maxIndexExclusive)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

func (r *PostgresRepository) MarkStopped(ctx context.Context, appID uuid.UUID, indexes []int) error {
	if len(indexes) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE application_replicas SET
			status = $2, healthy = FALSE, routing_enabled = FALSE, last_error = 'scale_down'
		WHERE application_id = $1 AND replica_index = ANY($3)`,
		appID, StatusStopped, indexes)
	return err
}

func (r *PostgresRepository) CountHealthy(ctx context.Context, appID uuid.UUID) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM application_replicas
		WHERE application_id = $1 AND healthy = TRUE AND status = $2`,
		appID, StatusRunning).Scan(&n)
	return n, err
}

func (r *PostgresRepository) GetApplicationMeta(ctx context.Context, appID uuid.UUID) (AppMeta, error) {
	var m AppMeta
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, slug, target_server_id
		FROM applications WHERE id = $1 AND deleted_at IS NULL`, appID).Scan(
		&m.ID, &m.OrganizationID, &m.Slug, &m.ServerID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AppMeta{}, ErrNotFound
	}
	return m, err
}

func (r *PostgresRepository) GetLatestConfigRuntime(ctx context.Context, appID uuid.UUID) (map[string]any, ConfigMeta, error) {
	var meta ConfigMeta
	var health, runtime []byte
	err := r.pool.QueryRow(ctx, `
		SELECT version, source_type, repository_url, git_branch, dockerfile_path, build_context,
		       image_reference, internal_port, command, entrypoint, cpu_limit_millis, memory_limit_bytes,
		       restart_policy, health_check, runtime_config, auto_deploy_enabled, git_connection_id
		FROM application_configs
		WHERE application_id = $1
		ORDER BY version DESC LIMIT 1`, appID).Scan(
		&meta.Version, &meta.SourceType, &meta.RepositoryURL, &meta.GitBranch, &meta.DockerfilePath, &meta.BuildContext,
		&meta.ImageReference, &meta.InternalPort, &meta.Command, &meta.Entrypoint, &meta.CPULimitMillis, &meta.MemoryLimitBytes,
		&meta.RestartPolicy, &health, &runtime, &meta.AutoDeploy, &meta.GitConnectionID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ConfigMeta{}, ErrNotFound
	}
	if err != nil {
		return nil, ConfigMeta{}, err
	}
	meta.HealthCheck = map[string]any{}
	meta.RuntimeConfig = map[string]any{}
	_ = jsonUnmarshal(health, &meta.HealthCheck)
	_ = jsonUnmarshal(runtime, &meta.RuntimeConfig)
	return meta.RuntimeConfig, meta, nil
}

func (r *PostgresRepository) InsertConfigVersion(ctx context.Context, orgID, appID, actorID uuid.UUID, runtime map[string]any) error {
	_, meta, err := r.GetLatestConfigRuntime(ctx, appID)
	if err != nil {
		return err
	}
	next := meta.Version + 1
	_, err = r.pool.Exec(ctx, `
		INSERT INTO application_configs (
			organization_id, application_id, version, source_type, repository_url, git_branch,
			dockerfile_path, build_context, image_reference, internal_port, command, entrypoint,
			cpu_limit_millis, memory_limit_bytes, restart_policy, health_check, runtime_config,
			auto_deploy_enabled, git_connection_id, created_by
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20
		)`,
		orgID, appID, next, meta.SourceType, meta.RepositoryURL, meta.GitBranch,
		meta.DockerfilePath, meta.BuildContext, meta.ImageReference, meta.InternalPort, meta.Command, meta.Entrypoint,
		meta.CPULimitMillis, meta.MemoryLimitBytes, meta.RestartPolicy, mustJSON(meta.HealthCheck), mustJSON(runtime),
		meta.AutoDeploy, meta.GitConnectionID, actorID,
	)
	return err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanReplica(row scannable) (Replica, error) {
	var r Replica
	err := row.Scan(
		&r.ID, &r.OrganizationID, &r.ApplicationID, &r.RevisionID, &r.ServerID, &r.ReplicaIndex,
		&r.ContainerName, &r.ContainerID, &r.Status, &r.Healthy, &r.RoutingEnabled,
		&r.LastProbeAt, &r.LastError,
		&r.RestartAttemptCount, &r.NextRestartAt, &r.LastReconcileAt, &r.ObservedExitCode,
		&r.CreatedAt, &r.UpdatedAt,
	)
	return r, err
}

func mustJSON(v any) []byte {
	if v == nil {
		v = map[string]any{}
	}
	b, _ := json.Marshal(v)
	return b
}

func jsonUnmarshal(b []byte, dst any) error {
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, dst)
}
