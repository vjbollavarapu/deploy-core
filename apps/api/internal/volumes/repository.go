package volumes

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
	ErrNotFound = errors.New("volumes: not found")
	ErrConflict = errors.New("volumes: conflict")
)

type Repository interface {
	GetServerOrg(ctx context.Context, serverID uuid.UUID) (uuid.UUID, error)
	ApplicationInOrg(ctx context.Context, orgID, appID uuid.UUID) (bool, error)
	DatabaseInOrg(ctx context.Context, orgID, dbID uuid.UUID) (bool, error)
	Create(ctx context.Context, v Volume, createdBy *uuid.UUID) (Volume, error)
	Get(ctx context.Context, id uuid.UUID) (Volume, error)
	GetByServerName(ctx context.Context, serverID uuid.UUID, name string) (Volume, error)
	List(ctx context.Context, orgID uuid.UUID, serverID *uuid.UUID, limit, offset int) ([]Volume, int64, error)
	Update(ctx context.Context, v Volume) (Volume, error)
	SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error
	SetState(ctx context.Context, id uuid.UUID, state string, commandID *uuid.UUID, dockerName *string, usageBytes *int64, lastError string) (Volume, error)
	SetAttachment(ctx context.Context, id uuid.UUID, resourceType *string, resourceID *uuid.UUID, mountPath string, state string, commandID *uuid.UUID) (Volume, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetServerOrg(ctx context.Context, serverID uuid.UUID) (uuid.UUID, error) {
	var orgID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT organization_id FROM servers WHERE id = $1 AND deleted_at IS NULL`, serverID).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return orgID, err
}

func (r *PostgresRepository) ApplicationInOrg(ctx context.Context, orgID, appID uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM applications
			WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
		)`, appID, orgID).Scan(&ok)
	return ok, err
}

func (r *PostgresRepository) DatabaseInOrg(ctx context.Context, orgID, dbID uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM managed_databases
			WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
		)`, dbID, orgID).Scan(&ok)
	return ok, err
}

func (r *PostgresRepository) Create(ctx context.Context, v Volume, createdBy *uuid.UUID) (Volume, error) {
	policy, _ := json.Marshal(mapOrEmpty(v.BackupPolicy))
	labels, _ := json.Marshal(mapOrEmpty(v.Labels))
	const q = `
		INSERT INTO volumes (
			organization_id, server_id, name, driver, mount_path, state,
			attached_resource_type, attached_resource_id, backup_policy, protected,
			docker_name, usage_bytes, labels, last_command_id, last_error, created_by
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16
		)
		RETURNING id, organization_id, server_id, name, driver, mount_path, state,
			attached_resource_type, attached_resource_id, backup_policy, protected,
			docker_name, usage_bytes, labels, last_command_id, last_error,
			created_by, created_at, updated_at, deleted_at`
	out, err := scanVolume(r.pool.QueryRow(ctx, q,
		v.OrganizationID, v.ServerID, v.Name, v.Driver, v.MountPath, v.State,
		v.AttachedResourceType, v.AttachedResourceID, policy, v.Protected,
		v.DockerName, v.UsageBytes, labels, v.LastCommandID, v.LastError, createdBy,
	))
	if isUniqueViolation(err) {
		return Volume{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Volume, error) {
	const q = `
		SELECT id, organization_id, server_id, name, driver, mount_path, state,
			attached_resource_type, attached_resource_id, backup_policy, protected,
			docker_name, usage_bytes, labels, last_command_id, last_error,
			created_by, created_at, updated_at, deleted_at
		FROM volumes WHERE id = $1 AND deleted_at IS NULL`
	out, err := scanVolume(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Volume{}, ErrNotFound
	}
	return out, err
}

func (r *PostgresRepository) GetByServerName(ctx context.Context, serverID uuid.UUID, name string) (Volume, error) {
	const q = `
		SELECT id, organization_id, server_id, name, driver, mount_path, state,
			attached_resource_type, attached_resource_id, backup_policy, protected,
			docker_name, usage_bytes, labels, last_command_id, last_error,
			created_by, created_at, updated_at, deleted_at
		FROM volumes WHERE server_id = $1 AND name = $2 AND deleted_at IS NULL`
	out, err := scanVolume(r.pool.QueryRow(ctx, q, serverID, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return Volume{}, ErrNotFound
	}
	return out, err
}

func (r *PostgresRepository) List(ctx context.Context, orgID uuid.UUID, serverID *uuid.UUID, limit, offset int) ([]Volume, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM volumes
		WHERE organization_id = $1 AND deleted_at IS NULL
		  AND ($2::uuid IS NULL OR server_id = $2)`, orgID, serverID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, server_id, name, driver, mount_path, state,
			attached_resource_type, attached_resource_id, backup_policy, protected,
			docker_name, usage_bytes, labels, last_command_id, last_error,
			created_by, created_at, updated_at, deleted_at
		FROM volumes
		WHERE organization_id = $1 AND deleted_at IS NULL
		  AND ($2::uuid IS NULL OR server_id = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`, orgID, serverID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Volume
	for rows.Next() {
		v, err := scanVolume(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, v Volume) (Volume, error) {
	policy, _ := json.Marshal(mapOrEmpty(v.BackupPolicy))
	labels, _ := json.Marshal(mapOrEmpty(v.Labels))
	const q = `
		UPDATE volumes SET
			mount_path = $2,
			backup_policy = $3,
			labels = $4,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, server_id, name, driver, mount_path, state,
			attached_resource_type, attached_resource_id, backup_policy, protected,
			docker_name, usage_bytes, labels, last_command_id, last_error,
			created_by, created_at, updated_at, deleted_at`
	out, err := scanVolume(r.pool.QueryRow(ctx, q, v.ID, v.MountPath, policy, labels))
	if errors.Is(err, pgx.ErrNoRows) {
		return Volume{}, ErrNotFound
	}
	return out, err
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE volumes
		SET deleted_at = $2, state = 'DELETED', updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SetState(ctx context.Context, id uuid.UUID, state string, commandID *uuid.UUID, dockerName *string, usageBytes *int64, lastError string) (Volume, error) {
	const q = `
		UPDATE volumes SET
			state = $2,
			last_command_id = COALESCE($3, last_command_id),
			docker_name = COALESCE($4, docker_name),
			usage_bytes = COALESCE($5, usage_bytes),
			last_error = $6,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, server_id, name, driver, mount_path, state,
			attached_resource_type, attached_resource_id, backup_policy, protected,
			docker_name, usage_bytes, labels, last_command_id, last_error,
			created_by, created_at, updated_at, deleted_at`
	out, err := scanVolume(r.pool.QueryRow(ctx, q, id, state, commandID, dockerName, usageBytes, lastError))
	if errors.Is(err, pgx.ErrNoRows) {
		return Volume{}, ErrNotFound
	}
	return out, err
}

func (r *PostgresRepository) SetAttachment(ctx context.Context, id uuid.UUID, resourceType *string, resourceID *uuid.UUID, mountPath string, state string, commandID *uuid.UUID) (Volume, error) {
	const q = `
		UPDATE volumes SET
			attached_resource_type = $2,
			attached_resource_id = $3,
			mount_path = COALESCE(NULLIF($4, ''), mount_path),
			state = $5,
			last_command_id = COALESCE($6, last_command_id),
			last_error = '',
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, server_id, name, driver, mount_path, state,
			attached_resource_type, attached_resource_id, backup_policy, protected,
			docker_name, usage_bytes, labels, last_command_id, last_error,
			created_by, created_at, updated_at, deleted_at`
	out, err := scanVolume(r.pool.QueryRow(ctx, q, id, resourceType, resourceID, mountPath, state, commandID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Volume{}, ErrNotFound
	}
	return out, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanVolume(row scannable) (Volume, error) {
	var v Volume
	var policy, labels []byte
	err := row.Scan(
		&v.ID, &v.OrganizationID, &v.ServerID, &v.Name, &v.Driver, &v.MountPath, &v.State,
		&v.AttachedResourceType, &v.AttachedResourceID, &policy, &v.Protected,
		&v.DockerName, &v.UsageBytes, &labels, &v.LastCommandID, &v.LastError,
		&v.CreatedBy, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
	)
	if err != nil {
		return Volume{}, err
	}
	_ = json.Unmarshal(policy, &v.BackupPolicy)
	_ = json.Unmarshal(labels, &v.Labels)
	if v.BackupPolicy == nil {
		v.BackupPolicy = map[string]any{}
	}
	if v.Labels == nil {
		v.Labels = map[string]any{}
	}
	return v, nil
}

func mapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
