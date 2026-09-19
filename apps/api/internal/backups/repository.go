package backups

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
	ErrNotFound = errors.New("backups: not found")
	ErrConflict = errors.New("backups: conflict")
)

type DatabaseRef struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	ServerID       uuid.UUID
	Status         string
	BackupPolicy   map[string]any
	Name           string
}

type Repository interface {
	GetDatabase(ctx context.Context, id uuid.UUID) (DatabaseRef, error)
	CreateBackup(ctx context.Context, b Backup) (Backup, error)
	GetBackup(ctx context.Context, id uuid.UUID) (Backup, error)
	ListBackups(ctx context.Context, orgID uuid.UUID, resourceID *uuid.UUID, limit, offset int) ([]Backup, int64, error)
	UpdateBackup(ctx context.Context, b Backup) (Backup, error)
	SoftDeleteBackup(ctx context.Context, id uuid.UUID, at time.Time) error
	CreateRestore(ctx context.Context, r Restore) (Restore, error)
	GetRestore(ctx context.Context, id uuid.UUID) (Restore, error)
	ListRestores(ctx context.Context, orgID uuid.UUID, backupID, targetID *uuid.UUID, limit, offset int) ([]Restore, int64, error)
	UpdateRestore(ctx context.Context, r Restore) (Restore, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetDatabase(ctx context.Context, id uuid.UUID) (DatabaseRef, error) {
	var d DatabaseRef
	var policy []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, server_id, status, backup_policy, name
		FROM managed_databases WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&d.ID, &d.OrganizationID, &d.ServerID, &d.Status, &policy, &d.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return DatabaseRef{}, ErrNotFound
	}
	if err != nil {
		return DatabaseRef{}, err
	}
	_ = json.Unmarshal(policy, &d.BackupPolicy)
	if d.BackupPolicy == nil {
		d.BackupPolicy = map[string]any{}
	}
	return d, nil
}

func (r *PostgresRepository) CreateBackup(ctx context.Context, b Backup) (Backup, error) {
	meta, _ := json.Marshal(mapOrEmpty(b.Metadata))
	const q = `
		INSERT INTO backups (
			organization_id, server_id, resource_type, resource_id, type, status,
			destination_type, destination_uri, retention_until, job_id, command_id,
			last_error, metadata, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, organization_id, server_id, resource_type, resource_id, type, status,
			started_at, completed_at, duration_ms, size_bytes, checksum,
			destination_type, destination_uri, retention_until, job_id, command_id,
			last_error, metadata, created_by, created_at, updated_at, deleted_at`
	return scanBackup(r.pool.QueryRow(ctx, q,
		b.OrganizationID, b.ServerID, b.ResourceType, b.ResourceID, b.Type, b.Status,
		b.DestinationType, b.DestinationURI, b.RetentionUntil, b.JobID, b.CommandID,
		b.LastError, meta, b.CreatedBy,
	))
}

func (r *PostgresRepository) GetBackup(ctx context.Context, id uuid.UUID) (Backup, error) {
	const q = `
		SELECT id, organization_id, server_id, resource_type, resource_id, type, status,
			started_at, completed_at, duration_ms, size_bytes, checksum,
			destination_type, destination_uri, retention_until, job_id, command_id,
			last_error, metadata, created_by, created_at, updated_at, deleted_at
		FROM backups WHERE id = $1 AND deleted_at IS NULL`
	out, err := scanBackup(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Backup{}, ErrNotFound
	}
	return out, err
}

func (r *PostgresRepository) ListBackups(ctx context.Context, orgID uuid.UUID, resourceID *uuid.UUID, limit, offset int) ([]Backup, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM backups
		WHERE organization_id = $1 AND deleted_at IS NULL
		  AND ($2::uuid IS NULL OR resource_id = $2)`, orgID, resourceID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, server_id, resource_type, resource_id, type, status,
			started_at, completed_at, duration_ms, size_bytes, checksum,
			destination_type, destination_uri, retention_until, job_id, command_id,
			last_error, metadata, created_by, created_at, updated_at, deleted_at
		FROM backups
		WHERE organization_id = $1 AND deleted_at IS NULL
		  AND ($2::uuid IS NULL OR resource_id = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`, orgID, resourceID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Backup
	for rows.Next() {
		b, err := scanBackup(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) UpdateBackup(ctx context.Context, b Backup) (Backup, error) {
	meta, _ := json.Marshal(mapOrEmpty(b.Metadata))
	const q = `
		UPDATE backups SET
			status = $2, started_at = $3, completed_at = $4, duration_ms = $5,
			size_bytes = $6, checksum = $7, destination_uri = $8, retention_until = $9,
			job_id = $10, command_id = $11, last_error = $12, metadata = $13,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, server_id, resource_type, resource_id, type, status,
			started_at, completed_at, duration_ms, size_bytes, checksum,
			destination_type, destination_uri, retention_until, job_id, command_id,
			last_error, metadata, created_by, created_at, updated_at, deleted_at`
	out, err := scanBackup(r.pool.QueryRow(ctx, q,
		b.ID, b.Status, b.StartedAt, b.CompletedAt, b.DurationMs,
		b.SizeBytes, b.Checksum, b.DestinationURI, b.RetentionUntil,
		b.JobID, b.CommandID, b.LastError, meta,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Backup{}, ErrNotFound
	}
	return out, err
}

func (r *PostgresRepository) SoftDeleteBackup(ctx context.Context, id uuid.UUID, at time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE backups SET deleted_at = $2, status = 'DELETED', updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CreateRestore(ctx context.Context, rest Restore) (Restore, error) {
	meta, _ := json.Marshal(mapOrEmpty(rest.Metadata))
	const q = `
		INSERT INTO restore_operations (
			organization_id, backup_id, target_resource_type, target_resource_id, server_id,
			status, job_id, command_id, last_error, metadata, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, organization_id, backup_id, target_resource_type, target_resource_id, server_id,
			status, started_at, completed_at, duration_ms, job_id, command_id,
			validation_passed, last_error, metadata, created_by, created_at, updated_at`
	return scanRestore(r.pool.QueryRow(ctx, q,
		rest.OrganizationID, rest.BackupID, rest.TargetResourceType, rest.TargetResourceID, rest.ServerID,
		rest.Status, rest.JobID, rest.CommandID, rest.LastError, meta, rest.CreatedBy,
	))
}

func (r *PostgresRepository) GetRestore(ctx context.Context, id uuid.UUID) (Restore, error) {
	const q = `
		SELECT id, organization_id, backup_id, target_resource_type, target_resource_id, server_id,
			status, started_at, completed_at, duration_ms, job_id, command_id,
			validation_passed, last_error, metadata, created_by, created_at, updated_at
		FROM restore_operations WHERE id = $1`
	out, err := scanRestore(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Restore{}, ErrNotFound
	}
	return out, err
}

func (r *PostgresRepository) ListRestores(ctx context.Context, orgID uuid.UUID, backupID, targetID *uuid.UUID, limit, offset int) ([]Restore, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM restore_operations
		WHERE organization_id = $1
		  AND ($2::uuid IS NULL OR backup_id = $2)
		  AND ($3::uuid IS NULL OR target_resource_id = $3)`, orgID, backupID, targetID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, backup_id, target_resource_type, target_resource_id, server_id,
			status, started_at, completed_at, duration_ms, job_id, command_id,
			validation_passed, last_error, metadata, created_by, created_at, updated_at
		FROM restore_operations
		WHERE organization_id = $1
		  AND ($2::uuid IS NULL OR backup_id = $2)
		  AND ($3::uuid IS NULL OR target_resource_id = $3)
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5`, orgID, backupID, targetID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Restore
	for rows.Next() {
		item, err := scanRestore(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) UpdateRestore(ctx context.Context, rest Restore) (Restore, error) {
	meta, _ := json.Marshal(mapOrEmpty(rest.Metadata))
	const q = `
		UPDATE restore_operations SET
			status = $2, started_at = $3, completed_at = $4, duration_ms = $5,
			job_id = $6, command_id = $7, validation_passed = $8, last_error = $9,
			metadata = $10, updated_at = NOW()
		WHERE id = $1
		RETURNING id, organization_id, backup_id, target_resource_type, target_resource_id, server_id,
			status, started_at, completed_at, duration_ms, job_id, command_id,
			validation_passed, last_error, metadata, created_by, created_at, updated_at`
	out, err := scanRestore(r.pool.QueryRow(ctx, q,
		rest.ID, rest.Status, rest.StartedAt, rest.CompletedAt, rest.DurationMs,
		rest.JobID, rest.CommandID, rest.ValidationPassed, rest.LastError, meta,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Restore{}, ErrNotFound
	}
	return out, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanBackup(row scannable) (Backup, error) {
	var b Backup
	var meta []byte
	err := row.Scan(
		&b.ID, &b.OrganizationID, &b.ServerID, &b.ResourceType, &b.ResourceID, &b.Type, &b.Status,
		&b.StartedAt, &b.CompletedAt, &b.DurationMs, &b.SizeBytes, &b.Checksum,
		&b.DestinationType, &b.DestinationURI, &b.RetentionUntil, &b.JobID, &b.CommandID,
		&b.LastError, &meta, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt, &b.DeletedAt,
	)
	if err != nil {
		return Backup{}, err
	}
	_ = json.Unmarshal(meta, &b.Metadata)
	if b.Metadata == nil {
		b.Metadata = map[string]any{}
	}
	return b, nil
}

func scanRestore(row scannable) (Restore, error) {
	var r Restore
	var meta []byte
	err := row.Scan(
		&r.ID, &r.OrganizationID, &r.BackupID, &r.TargetResourceType, &r.TargetResourceID, &r.ServerID,
		&r.Status, &r.StartedAt, &r.CompletedAt, &r.DurationMs, &r.JobID, &r.CommandID,
		&r.ValidationPassed, &r.LastError, &meta, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return Restore{}, err
	}
	_ = json.Unmarshal(meta, &r.Metadata)
	if r.Metadata == nil {
		r.Metadata = map[string]any{}
	}
	return r, nil
}

func mapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
