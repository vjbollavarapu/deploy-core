package secrets

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type Repository interface {
	Create(ctx context.Context, meta Metadata, ciphertext, nonce []byte, actorID uuid.UUID) (Metadata, error)
	Get(ctx context.Context, id uuid.UUID) (Record, error)
	GetActiveByName(ctx context.Context, orgID uuid.UUID, scope string, projectID, environmentID, applicationID *uuid.UUID, name string) (Record, error)
	List(ctx context.Context, orgID uuid.UUID, scope *string, projectID, environmentID, applicationID *uuid.UUID, limit, offset int) ([]Metadata, int64, error)
	SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error
	ResolveProject(ctx context.Context, projectID uuid.UUID) (orgID uuid.UUID, err error)
	ResolveEnvironment(ctx context.Context, environmentID uuid.UUID) (orgID, projectID uuid.UUID, err error)
	ResolveApplication(ctx context.Context, applicationID uuid.UUID) (orgID, projectID, environmentID uuid.UUID, err error)
	OrgExists(ctx context.Context, orgID uuid.UUID) (bool, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, meta Metadata, ciphertext, nonce []byte, actorID uuid.UUID) (Metadata, error) {
	const q = `
		INSERT INTO secrets (
			organization_id, scope, project_id, environment_id, application_id,
			name, version, ciphertext, nonce, key_id, algorithm, created_by, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)
		RETURNING id, organization_id, scope, project_id, environment_id, application_id,
		          name, version, key_id, algorithm, created_by, updated_by, created_at, updated_at`
	m, err := scanMeta(r.pool.QueryRow(ctx, q,
		meta.OrganizationID, meta.Scope, meta.ProjectID, meta.EnvironmentID, meta.ApplicationID,
		meta.Name, meta.Version, ciphertext, nonce, meta.KeyID, meta.Algorithm, actorID,
	))
	if isUniqueViolation(err) {
		return Metadata{}, ErrConflict
	}
	return m, err
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Record, error) {
	const q = `
		SELECT id, organization_id, scope, project_id, environment_id, application_id,
		       name, version, key_id, algorithm, created_by, updated_by, created_at, updated_at,
		       ciphertext, nonce
		FROM secrets WHERE id = $1 AND deleted_at IS NULL`
	rec, err := scanRecord(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	return rec, err
}

func (r *PostgresRepository) GetActiveByName(ctx context.Context, orgID uuid.UUID, scope string, projectID, environmentID, applicationID *uuid.UUID, name string) (Record, error) {
	const q = `
		SELECT id, organization_id, scope, project_id, environment_id, application_id,
		       name, version, key_id, algorithm, created_by, updated_by, created_at, updated_at,
		       ciphertext, nonce
		FROM secrets
		WHERE organization_id = $1 AND scope = $2 AND name = $3 AND deleted_at IS NULL
		  AND project_id IS NOT DISTINCT FROM $4
		  AND environment_id IS NOT DISTINCT FROM $5
		  AND application_id IS NOT DISTINCT FROM $6`
	rec, err := scanRecord(r.pool.QueryRow(ctx, q, orgID, scope, name, projectID, environmentID, applicationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	return rec, err
}

func (r *PostgresRepository) List(ctx context.Context, orgID uuid.UUID, scope *string, projectID, environmentID, applicationID *uuid.UUID, limit, offset int) ([]Metadata, int64, error) {
	where := `WHERE organization_id = $1 AND deleted_at IS NULL`
	args := []any{orgID}
	n := 2
	if scope != nil {
		where += ` AND scope = $` + itoa(n)
		args = append(args, *scope)
		n++
	}
	if projectID != nil {
		where += ` AND project_id = $` + itoa(n)
		args = append(args, *projectID)
		n++
	}
	if environmentID != nil {
		where += ` AND environment_id = $` + itoa(n)
		args = append(args, *environmentID)
		n++
	}
	if applicationID != nil {
		where += ` AND application_id = $` + itoa(n)
		args = append(args, *applicationID)
		n++
	}

	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM secrets `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `
		SELECT id, organization_id, scope, project_id, environment_id, application_id,
		       name, version, key_id, algorithm, created_by, updated_by, created_at, updated_at
		FROM secrets ` + where + `
		ORDER BY name ASC LIMIT $` + itoa(n) + ` OFFSET $` + itoa(n+1)
	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Metadata
	for rows.Next() {
		m, err := scanMeta(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE secrets SET deleted_at = $2 WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ResolveProject(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	var orgID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT organization_id FROM projects WHERE id = $1 AND deleted_at IS NULL`, projectID).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return orgID, err
}

func (r *PostgresRepository) ResolveEnvironment(ctx context.Context, environmentID uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	var orgID, projectID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT organization_id, project_id FROM environments
		WHERE id = $1 AND deleted_at IS NULL`, environmentID).Scan(&orgID, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, ErrNotFound
	}
	return orgID, projectID, err
}

func (r *PostgresRepository) ResolveApplication(ctx context.Context, applicationID uuid.UUID) (uuid.UUID, uuid.UUID, uuid.UUID, error) {
	var orgID, projectID, environmentID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT organization_id, project_id, environment_id FROM applications
		WHERE id = $1 AND deleted_at IS NULL`, applicationID).Scan(&orgID, &projectID, &environmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, uuid.Nil, ErrNotFound
	}
	return orgID, projectID, environmentID, err
}

func (r *PostgresRepository) OrgExists(ctx context.Context, orgID uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM organizations WHERE id = $1 AND deleted_at IS NULL)`, orgID).Scan(&ok)
	return ok, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanMeta(row scannable) (Metadata, error) {
	var m Metadata
	err := row.Scan(
		&m.ID, &m.OrganizationID, &m.Scope, &m.ProjectID, &m.EnvironmentID, &m.ApplicationID,
		&m.Name, &m.Version, &m.KeyID, &m.Algorithm, &m.CreatedBy, &m.UpdatedBy, &m.CreatedAt, &m.UpdatedAt,
	)
	return m, err
}

func scanRecord(row scannable) (Record, error) {
	var rec Record
	err := row.Scan(
		&rec.ID, &rec.OrganizationID, &rec.Scope, &rec.ProjectID, &rec.EnvironmentID, &rec.ApplicationID,
		&rec.Name, &rec.Version, &rec.KeyID, &rec.Algorithm, &rec.CreatedBy, &rec.UpdatedBy, &rec.CreatedAt, &rec.UpdatedAt,
		&rec.Ciphertext, &rec.Nonce,
	)
	return rec, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
