package variables

import (
	"context"
	"errors"
	"strconv"

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
	Create(ctx context.Context, in CreateInput, actorID uuid.UUID) (Variable, error)
	Get(ctx context.Context, id uuid.UUID) (Variable, error)
	List(ctx context.Context, orgID uuid.UUID, scope *string, projectID, environmentID, applicationID *uuid.UUID, limit, offset int) ([]Variable, int64, error)
	Update(ctx context.Context, id uuid.UUID, in UpdateInput, actorID uuid.UUID) (Variable, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ListForResolve(ctx context.Context, orgID uuid.UUID, projectID, environmentID, applicationID *uuid.UUID) ([]Variable, error)
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

func (r *PostgresRepository) Create(ctx context.Context, in CreateInput, actorID uuid.UUID) (Variable, error) {
	const q = `
		INSERT INTO environment_variables (
			organization_id, scope, project_id, environment_id, application_id,
			key, value, created_by, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
		RETURNING id, organization_id, scope, project_id, environment_id, application_id,
		          key, value, created_by, updated_by, created_at, updated_at`
	v, err := scanVar(r.pool.QueryRow(ctx, q,
		in.OrganizationID, in.Scope, in.ProjectID, in.EnvironmentID, in.ApplicationID,
		in.Key, in.Value, actorID,
	))
	if isUniqueViolation(err) {
		return Variable{}, ErrConflict
	}
	return v, err
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Variable, error) {
	const q = `
		SELECT id, organization_id, scope, project_id, environment_id, application_id,
		       key, value, created_by, updated_by, created_at, updated_at
		FROM environment_variables WHERE id = $1`
	v, err := scanVar(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Variable{}, ErrNotFound
	}
	return v, err
}

func (r *PostgresRepository) List(ctx context.Context, orgID uuid.UUID, scope *string, projectID, environmentID, applicationID *uuid.UUID, limit, offset int) ([]Variable, int64, error) {
	where := `WHERE organization_id = $1`
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
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM environment_variables `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := `
		SELECT id, organization_id, scope, project_id, environment_id, application_id,
		       key, value, created_by, updated_by, created_at, updated_at
		FROM environment_variables ` + where + `
		ORDER BY key ASC LIMIT $` + itoa(n) + ` OFFSET $` + itoa(n+1)
	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Variable
	for rows.Next() {
		v, err := scanVar(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, id uuid.UUID, in UpdateInput, actorID uuid.UUID) (Variable, error) {
	cur, err := r.Get(ctx, id)
	if err != nil {
		return Variable{}, err
	}
	key, value := cur.Key, cur.Value
	if in.Key != nil {
		key = *in.Key
	}
	if in.Value != nil {
		value = *in.Value
	}
	const q = `
		UPDATE environment_variables
		SET key = $2, value = $3, updated_by = $4
		WHERE id = $1
		RETURNING id, organization_id, scope, project_id, environment_id, application_id,
		          key, value, created_by, updated_by, created_at, updated_at`
	v, err := scanVar(r.pool.QueryRow(ctx, q, id, key, value, actorID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Variable{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Variable{}, ErrConflict
	}
	return v, err
}

func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM environment_variables WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListForResolve(ctx context.Context, orgID uuid.UUID, projectID, environmentID, applicationID *uuid.UUID) ([]Variable, error) {
	const q = `
		SELECT id, organization_id, scope, project_id, environment_id, application_id,
		       key, value, created_by, updated_by, created_at, updated_at
		FROM environment_variables
		WHERE organization_id = $1
		  AND (
		    scope = 'ORGANIZATION'
		    OR ($2::uuid IS NOT NULL AND scope = 'PROJECT' AND project_id = $2)
		    OR ($3::uuid IS NOT NULL AND scope = 'ENVIRONMENT' AND environment_id = $3)
		    OR ($4::uuid IS NOT NULL AND scope = 'APPLICATION' AND application_id = $4)
		  )
		ORDER BY CASE scope
		  WHEN 'ORGANIZATION' THEN 1
		  WHEN 'PROJECT' THEN 2
		  WHEN 'ENVIRONMENT' THEN 3
		  WHEN 'APPLICATION' THEN 4
		END, key ASC`
	rows, err := r.pool.Query(ctx, q, orgID, projectID, environmentID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Variable
	for rows.Next() {
		v, err := scanVar(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
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

func scanVar(row scannable) (Variable, error) {
	var v Variable
	err := row.Scan(
		&v.ID, &v.OrganizationID, &v.Scope, &v.ProjectID, &v.EnvironmentID, &v.ApplicationID,
		&v.Key, &v.Value, &v.CreatedBy, &v.UpdatedBy, &v.CreatedAt, &v.UpdatedAt,
	)
	return v, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
