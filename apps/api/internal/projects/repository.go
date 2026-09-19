package projects

import (
	"context"
	"errors"
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
	CreateProject(ctx context.Context, orgID uuid.UUID, name, slug, description string, createdBy uuid.UUID) (Project, error)
	GetProject(ctx context.Context, id uuid.UUID) (Project, error)
	ListProjects(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Project, int64, error)
	UpdateProject(ctx context.Context, id uuid.UUID, name, slug, description *string) (Project, error)
	SoftDeleteProject(ctx context.Context, id uuid.UUID, at time.Time) error
	SoftDeleteEnvironmentsForProject(ctx context.Context, projectID uuid.UUID, at time.Time) error
	CountActiveApplicationsForProject(ctx context.Context, projectID uuid.UUID) (int64, error)

	CreateEnvironment(ctx context.Context, orgID, projectID uuid.UUID, name, slug, kind string) (Environment, error)
	GetEnvironment(ctx context.Context, id uuid.UUID) (Environment, error)
	ListEnvironments(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]Environment, int64, error)
	UpdateEnvironment(ctx context.Context, id uuid.UUID, name, slug, kind *string) (Environment, error)
	SoftDeleteEnvironment(ctx context.Context, id uuid.UUID, at time.Time) error
	CountActiveApplicationsForEnvironment(ctx context.Context, environmentID uuid.UUID) (int64, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateProject(ctx context.Context, orgID uuid.UUID, name, slug, description string, createdBy uuid.UUID) (Project, error) {
	const q = `
		INSERT INTO projects (organization_id, name, slug, description, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, organization_id, name, slug, description, created_by, created_at, updated_at`
	p, err := scanProject(r.pool.QueryRow(ctx, q, orgID, name, slug, description, createdBy))
	if isUniqueViolation(err) {
		return Project{}, ErrConflict
	}
	return p, err
}

func (r *PostgresRepository) GetProject(ctx context.Context, id uuid.UUID) (Project, error) {
	const q = `
		SELECT id, organization_id, name, slug, description, created_by, created_at, updated_at
		FROM projects WHERE id = $1 AND deleted_at IS NULL`
	p, err := scanProject(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

func (r *PostgresRepository) ListProjects(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Project, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM projects WHERE organization_id = $1 AND deleted_at IS NULL`, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, slug, description, created_by, created_at, updated_at
		FROM projects
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC
		LIMIT $2 OFFSET $3`, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) UpdateProject(ctx context.Context, id uuid.UUID, name, slug, description *string) (Project, error) {
	const q = `
		UPDATE projects
		SET name = COALESCE($2, name),
		    slug = COALESCE($3, slug),
		    description = COALESCE($4, description)
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, name, slug, description, created_by, created_at, updated_at`
	p, err := scanProject(r.pool.QueryRow(ctx, q, id, name, slug, description))
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Project{}, ErrConflict
	}
	return p, err
}

func (r *PostgresRepository) SoftDeleteProject(ctx context.Context, id uuid.UUID, at time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE projects SET deleted_at = $2
		WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SoftDeleteEnvironmentsForProject(ctx context.Context, projectID uuid.UUID, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE environments SET deleted_at = $2
		WHERE project_id = $1 AND deleted_at IS NULL`, projectID, at)
	return err
}

func (r *PostgresRepository) CountActiveApplicationsForProject(ctx context.Context, projectID uuid.UUID) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM applications
		WHERE project_id = $1 AND deleted_at IS NULL`, projectID).Scan(&n)
	return n, err
}

func (r *PostgresRepository) CreateEnvironment(ctx context.Context, orgID, projectID uuid.UUID, name, slug, kind string) (Environment, error) {
	const q = `
		INSERT INTO environments (organization_id, project_id, name, slug, kind)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, organization_id, project_id, name, slug, kind, created_at, updated_at`
	e, err := scanEnvironment(r.pool.QueryRow(ctx, q, orgID, projectID, name, slug, kind))
	if isUniqueViolation(err) {
		return Environment{}, ErrConflict
	}
	return e, err
}

func (r *PostgresRepository) GetEnvironment(ctx context.Context, id uuid.UUID) (Environment, error) {
	const q = `
		SELECT id, organization_id, project_id, name, slug, kind, created_at, updated_at
		FROM environments WHERE id = $1 AND deleted_at IS NULL`
	e, err := scanEnvironment(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Environment{}, ErrNotFound
	}
	return e, err
}

func (r *PostgresRepository) ListEnvironments(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]Environment, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM environments WHERE project_id = $1 AND deleted_at IS NULL`, projectID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, project_id, name, slug, kind, created_at, updated_at
		FROM environments
		WHERE project_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC
		LIMIT $2 OFFSET $3`, projectID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Environment
	for rows.Next() {
		e, err := scanEnvironment(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) UpdateEnvironment(ctx context.Context, id uuid.UUID, name, slug, kind *string) (Environment, error) {
	const q = `
		UPDATE environments
		SET name = COALESCE($2, name),
		    slug = COALESCE($3, slug),
		    kind = COALESCE($4, kind)
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, project_id, name, slug, kind, created_at, updated_at`
	e, err := scanEnvironment(r.pool.QueryRow(ctx, q, id, name, slug, kind))
	if errors.Is(err, pgx.ErrNoRows) {
		return Environment{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Environment{}, ErrConflict
	}
	return e, err
}

func (r *PostgresRepository) SoftDeleteEnvironment(ctx context.Context, id uuid.UUID, at time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE environments SET deleted_at = $2
		WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CountActiveApplicationsForEnvironment(ctx context.Context, environmentID uuid.UUID) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM applications
		WHERE environment_id = $1 AND deleted_at IS NULL`, environmentID).Scan(&n)
	return n, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanProject(row scannable) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Description, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func scanEnvironment(row scannable) (Environment, error) {
	var e Environment
	err := row.Scan(&e.ID, &e.OrganizationID, &e.ProjectID, &e.Name, &e.Slug, &e.Kind, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
