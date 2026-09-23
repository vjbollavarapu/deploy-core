package domains

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
	GetApplication(ctx context.Context, appID uuid.UUID) (ApplicationRef, error)
	Create(ctx context.Context, d Domain) (Domain, error)
	Get(ctx context.Context, id uuid.UUID) (Domain, error)
	List(ctx context.Context, orgID uuid.UUID) ([]Domain, error)
	ListByApplication(ctx context.Context, appID uuid.UUID) ([]Domain, error)
	Update(ctx context.Context, d Domain) (Domain, error)
	ClearPrimary(ctx context.Context, applicationID, exceptID uuid.UUID) error
	SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error
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
		SELECT a.id, a.organization_id, a.environment_id, a.slug,
		       (
		         SELECT c.internal_port FROM application_configs c
		         WHERE c.application_id = a.id
		         ORDER BY c.version DESC LIMIT 1
		       )
		FROM applications a
		WHERE a.id = $1 AND a.deleted_at IS NULL`, appID).
		Scan(&a.ID, &a.OrganizationID, &a.EnvironmentID, &a.Slug, &a.InternalPort)
	if errors.Is(err, pgx.ErrNoRows) {
		return ApplicationRef{}, ErrNotFound
	}
	return a, err
}

func (r *PostgresRepository) Create(ctx context.Context, d Domain) (Domain, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Domain{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if d.IsPrimary {
		if _, err := tx.Exec(ctx, `
			UPDATE domains SET is_primary = FALSE
			WHERE application_id = $1 AND is_primary AND deleted_at IS NULL`, d.ApplicationID); err != nil {
			return Domain{}, err
		}
	}

	const q = `
		INSERT INTO domains (
			organization_id, application_id, environment_id, hostname, internal_port,
			is_primary, force_https, dns_status, tls_status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, organization_id, application_id, environment_id, hostname, internal_port,
		          is_primary, force_https, dns_status, tls_status, created_at, updated_at`
	out, err := scanDomain(tx.QueryRow(ctx, q,
		d.OrganizationID, d.ApplicationID, d.EnvironmentID, d.Hostname, d.InternalPort,
		d.IsPrimary, d.ForceHTTPS, d.DNSStatus, d.TLSStatus,
	))
	if isUniqueViolation(err) {
		return Domain{}, ErrConflict
	}
	if err != nil {
		return Domain{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Domain{}, err
	}
	return out, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Domain, error) {
	const q = `
		SELECT id, organization_id, application_id, environment_id, hostname, internal_port,
		       is_primary, force_https, dns_status, tls_status, created_at, updated_at
		FROM domains WHERE id = $1 AND deleted_at IS NULL`
	d, err := scanDomain(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Domain{}, ErrNotFound
	}
	return d, err
}

func (r *PostgresRepository) List(ctx context.Context, orgID uuid.UUID) ([]Domain, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, application_id, environment_id, hostname, internal_port,
		       is_primary, force_https, dns_status, tls_status, created_at, updated_at
		FROM domains
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Domain
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListByApplication(ctx context.Context, appID uuid.UUID) ([]Domain, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, application_id, environment_id, hostname, internal_port,
		       is_primary, force_https, dns_status, tls_status, created_at, updated_at
		FROM domains
		WHERE application_id = $1 AND deleted_at IS NULL
		ORDER BY is_primary DESC, hostname ASC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Domain
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, d Domain) (Domain, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Domain{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if d.IsPrimary {
		if _, err := tx.Exec(ctx, `
			UPDATE domains SET is_primary = FALSE
			WHERE application_id = $1 AND id <> $2 AND is_primary AND deleted_at IS NULL`,
			d.ApplicationID, d.ID); err != nil {
			return Domain{}, err
		}
	}

	const q = `
		UPDATE domains SET
			hostname = $2, internal_port = $3, is_primary = $4, force_https = $5,
			dns_status = $6, tls_status = $7
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, application_id, environment_id, hostname, internal_port,
		          is_primary, force_https, dns_status, tls_status, created_at, updated_at`
	out, err := scanDomain(tx.QueryRow(ctx, q,
		d.ID, d.Hostname, d.InternalPort, d.IsPrimary, d.ForceHTTPS, d.DNSStatus, d.TLSStatus,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Domain{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Domain{}, ErrConflict
	}
	if err != nil {
		return Domain{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Domain{}, err
	}
	return out, nil
}

func (r *PostgresRepository) ClearPrimary(ctx context.Context, applicationID, exceptID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE domains SET is_primary = FALSE
		WHERE application_id = $1 AND id <> $2 AND is_primary AND deleted_at IS NULL`,
		applicationID, exceptID)
	return err
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE domains SET deleted_at = $2, is_primary = FALSE
		WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanDomain(row scannable) (Domain, error) {
	var d Domain
	err := row.Scan(
		&d.ID, &d.OrganizationID, &d.ApplicationID, &d.EnvironmentID, &d.Hostname, &d.InternalPort,
		&d.IsPrimary, &d.ForceHTTPS, &d.DNSStatus, &d.TLSStatus, &d.CreatedAt, &d.UpdatedAt,
	)
	return d, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
