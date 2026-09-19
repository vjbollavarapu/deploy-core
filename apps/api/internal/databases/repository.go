package databases

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
	ErrNotFound = errors.New("databases: not found")
	ErrConflict = errors.New("databases: conflict")
)

type Repository interface {
	ResolveEnvironment(ctx context.Context, environmentID uuid.UUID) (orgID, projectID uuid.UUID, err error)
	ServerInOrg(ctx context.Context, orgID, serverID uuid.UUID) (bool, error)
	Create(ctx context.Context, d Database, cred credentialBlob, createdBy uuid.UUID) (Database, error)
	Get(ctx context.Context, id uuid.UUID) (Database, *credentialBlob, error)
	List(ctx context.Context, orgID uuid.UUID, projectID, environmentID, serverID *uuid.UUID, limit, offset int) ([]Database, int64, error)
	Update(ctx context.Context, id uuid.UUID, d Database, cred *credentialBlob) (Database, error)
	SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error
	SetProvisionState(ctx context.Context, id uuid.UUID, status string, commandID *uuid.UUID, runtimeID *string, lastError string) (Database, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ResolveEnvironment(ctx context.Context, environmentID uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	var orgID, projectID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT p.organization_id, e.project_id
		FROM environments e
		JOIN projects p ON p.id = e.project_id
		WHERE e.id = $1 AND e.deleted_at IS NULL AND p.deleted_at IS NULL`, environmentID).
		Scan(&orgID, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, ErrNotFound
	}
	return orgID, projectID, err
}

func (r *PostgresRepository) ServerInOrg(ctx context.Context, orgID, serverID uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM servers
			WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
		)`, serverID, orgID).Scan(&ok)
	return ok, err
}

func (r *PostgresRepository) Create(ctx context.Context, d Database, cred credentialBlob, createdBy uuid.UUID) (Database, error) {
	policy, _ := json.Marshal(mapOrEmpty(d.BackupPolicy))
	const q = `
		INSERT INTO managed_databases (
			organization_id, project_id, environment_id, server_id, name,
			engine, engine_version, database_name, username,
			credential_ciphertext, credential_nonce, credential_key_id, credential_algorithm,
			storage_volume_name, volume_protected, cpu_millis, memory_bytes,
			status, backup_policy, created_by
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'AES-256-GCM',$13,$14,$15,$16,$17,$18,$19
		)
		RETURNING id, organization_id, project_id, environment_id, server_id, name,
			engine, engine_version, database_name, username,
			storage_volume_name, volume_protected, cpu_millis, memory_bytes,
			container_runtime_id, status, backup_policy, provision_command_id, last_error,
			created_by, created_at, updated_at, deleted_at,
			TRUE`
	out, err := scanDatabase(r.pool.QueryRow(ctx, q,
		d.OrganizationID, d.ProjectID, d.EnvironmentID, d.ServerID, d.Name,
		d.Engine, d.EngineVersion, d.DatabaseName, d.Username,
		cred.Ciphertext, cred.Nonce, cred.KeyID,
		d.StorageVolumeName, d.VolumeProtected, d.CPUMillis, d.MemoryBytes,
		d.Status, policy, createdBy,
	))
	if isUniqueViolation(err) {
		return Database{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Database, *credentialBlob, error) {
	const q = `
		SELECT id, organization_id, project_id, environment_id, server_id, name,
			engine, engine_version, database_name, username,
			storage_volume_name, volume_protected, cpu_millis, memory_bytes,
			container_runtime_id, status, backup_policy, provision_command_id, last_error,
			created_by, created_at, updated_at, deleted_at,
			TRUE,
			credential_ciphertext, credential_nonce, credential_key_id
		FROM managed_databases
		WHERE id = $1 AND deleted_at IS NULL`
	var d Database
	var policy []byte
	var ct, nonce []byte
	var keyID string
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&d.ID, &d.OrganizationID, &d.ProjectID, &d.EnvironmentID, &d.ServerID, &d.Name,
		&d.Engine, &d.EngineVersion, &d.DatabaseName, &d.Username,
		&d.StorageVolumeName, &d.VolumeProtected, &d.CPUMillis, &d.MemoryBytes,
		&d.ContainerRuntimeID, &d.Status, &policy, &d.ProvisionCommandID, &d.LastError,
		&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt,
		&d.HasCredential, &ct, &nonce, &keyID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Database{}, nil, ErrNotFound
	}
	if err != nil {
		return Database{}, nil, err
	}
	_ = json.Unmarshal(policy, &d.BackupPolicy)
	if d.BackupPolicy == nil {
		d.BackupPolicy = map[string]any{}
	}
	return d, &credentialBlob{Ciphertext: ct, Nonce: nonce, KeyID: keyID}, nil
}

func (r *PostgresRepository) List(ctx context.Context, orgID uuid.UUID, projectID, environmentID, serverID *uuid.UUID, limit, offset int) ([]Database, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM managed_databases
		WHERE organization_id = $1 AND deleted_at IS NULL
		  AND ($2::uuid IS NULL OR project_id = $2)
		  AND ($3::uuid IS NULL OR environment_id = $3)
		  AND ($4::uuid IS NULL OR server_id = $4)`,
		orgID, projectID, environmentID, serverID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, project_id, environment_id, server_id, name,
			engine, engine_version, database_name, username,
			storage_volume_name, volume_protected, cpu_millis, memory_bytes,
			container_runtime_id, status, backup_policy, provision_command_id, last_error,
			created_by, created_at, updated_at, deleted_at,
			TRUE
		FROM managed_databases
		WHERE organization_id = $1 AND deleted_at IS NULL
		  AND ($2::uuid IS NULL OR project_id = $2)
		  AND ($3::uuid IS NULL OR environment_id = $3)
		  AND ($4::uuid IS NULL OR server_id = $4)
		ORDER BY created_at DESC
		LIMIT $5 OFFSET $6`, orgID, projectID, environmentID, serverID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Database
	for rows.Next() {
		d, err := scanDatabase(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, id uuid.UUID, d Database, cred *credentialBlob) (Database, error) {
	policy, _ := json.Marshal(mapOrEmpty(d.BackupPolicy))
	var ct, nonce []byte
	var keyID *string
	clearCred := cred != nil
	if cred != nil {
		ct, nonce = cred.Ciphertext, cred.Nonce
		keyID = &cred.KeyID
	}
	const q = `
		UPDATE managed_databases SET
			name = $2,
			cpu_millis = $3,
			memory_bytes = $4,
			backup_policy = $5,
			status = $6,
			credential_ciphertext = CASE WHEN $7 THEN $8 ELSE credential_ciphertext END,
			credential_nonce = CASE WHEN $7 THEN $9 ELSE credential_nonce END,
			credential_key_id = CASE WHEN $7 THEN $10 ELSE credential_key_id END,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, project_id, environment_id, server_id, name,
			engine, engine_version, database_name, username,
			storage_volume_name, volume_protected, cpu_millis, memory_bytes,
			container_runtime_id, status, backup_policy, provision_command_id, last_error,
			created_by, created_at, updated_at, deleted_at,
			TRUE`
	out, err := scanDatabase(r.pool.QueryRow(ctx, q,
		id, d.Name, d.CPUMillis, d.MemoryBytes, policy, d.Status,
		clearCred, ct, nonce, keyID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Database{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Database{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error {
	// Soft-delete metadata only — never DROP the data volume here.
	tag, err := r.pool.Exec(ctx, `
		UPDATE managed_databases
		SET deleted_at = $2, status = 'DELETED', updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SetProvisionState(ctx context.Context, id uuid.UUID, status string, commandID *uuid.UUID, runtimeID *string, lastError string) (Database, error) {
	const q = `
		UPDATE managed_databases SET
			status = $2,
			provision_command_id = COALESCE($3, provision_command_id),
			container_runtime_id = COALESCE($4, container_runtime_id),
			last_error = $5,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, project_id, environment_id, server_id, name,
			engine, engine_version, database_name, username,
			storage_volume_name, volume_protected, cpu_millis, memory_bytes,
			container_runtime_id, status, backup_policy, provision_command_id, last_error,
			created_by, created_at, updated_at, deleted_at,
			TRUE`
	out, err := scanDatabase(r.pool.QueryRow(ctx, q, id, status, commandID, runtimeID, lastError))
	if errors.Is(err, pgx.ErrNoRows) {
		return Database{}, ErrNotFound
	}
	return out, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanDatabase(row scannable) (Database, error) {
	var d Database
	var policy []byte
	err := row.Scan(
		&d.ID, &d.OrganizationID, &d.ProjectID, &d.EnvironmentID, &d.ServerID, &d.Name,
		&d.Engine, &d.EngineVersion, &d.DatabaseName, &d.Username,
		&d.StorageVolumeName, &d.VolumeProtected, &d.CPUMillis, &d.MemoryBytes,
		&d.ContainerRuntimeID, &d.Status, &policy, &d.ProvisionCommandID, &d.LastError,
		&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.DeletedAt,
		&d.HasCredential,
	)
	if err != nil {
		return Database{}, err
	}
	_ = json.Unmarshal(policy, &d.BackupPolicy)
	if d.BackupPolicy == nil {
		d.BackupPolicy = map[string]any{}
	}
	return d, nil
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
