package revisions

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	GetApplicationOrg(ctx context.Context, applicationID uuid.UUID) (uuid.UUID, error)
	Get(ctx context.Context, id uuid.UUID) (Revision, error)
	ListByApplication(ctx context.Context, applicationID uuid.UUID, status *string, limit, offset int) ([]Revision, int64, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetApplicationOrg(ctx context.Context, applicationID uuid.UUID) (uuid.UUID, error) {
	var orgID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT organization_id FROM applications WHERE id = $1 AND deleted_at IS NULL`, applicationID).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return orgID, err
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Revision, error) {
	const q = `
		SELECT id, organization_id, application_id, deployment_id, revision_number, status,
		       commit_sha, image_digest, image_tag, effective_config, variable_snapshot,
		       secret_refs, health_check, resource_limits, created_by, created_at, updated_at
		FROM revisions WHERE id = $1`
	rev, err := scanRevision(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Revision{}, ErrNotFound
	}
	return rev, err
}

func (r *PostgresRepository) ListByApplication(ctx context.Context, applicationID uuid.UUID, status *string, limit, offset int) ([]Revision, int64, error) {
	where := `WHERE application_id = $1`
	args := []any{applicationID}
	n := 2
	if status != nil {
		where += ` AND status = $` + itoa(n)
		args = append(args, *status)
		n++
	}
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM revisions `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `
		SELECT id, organization_id, application_id, deployment_id, revision_number, status,
		       commit_sha, image_digest, image_tag, effective_config, variable_snapshot,
		       secret_refs, health_check, resource_limits, created_by, created_at, updated_at
		FROM revisions ` + where + `
		ORDER BY revision_number DESC
		LIMIT $` + itoa(n) + ` OFFSET $` + itoa(n+1)
	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Revision
	for rows.Next() {
		rev, err := scanRevision(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, rev)
	}
	return out, total, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRevision(row scannable) (Revision, error) {
	var rev Revision
	var eff, vars, secrets, health, limits []byte
	err := row.Scan(
		&rev.ID, &rev.OrganizationID, &rev.ApplicationID, &rev.DeploymentID, &rev.RevisionNumber, &rev.Status,
		&rev.CommitSHA, &rev.ImageDigest, &rev.ImageTag, &eff, &vars,
		&secrets, &health, &limits, &rev.CreatedBy, &rev.CreatedAt, &rev.UpdatedAt,
	)
	if err != nil {
		return Revision{}, err
	}
	rev.EffectiveConfig = map[string]any{}
	rev.VariableSnapshot = map[string]any{}
	rev.HealthCheck = map[string]any{}
	rev.ResourceLimits = map[string]any{}
	rev.SecretRefs = []any{}
	_ = json.Unmarshal(eff, &rev.EffectiveConfig)
	_ = json.Unmarshal(vars, &rev.VariableSnapshot)
	_ = json.Unmarshal(health, &rev.HealthCheck)
	_ = json.Unmarshal(limits, &rev.ResourceLimits)
	_ = json.Unmarshal(secrets, &rev.SecretRefs)
	if rev.SecretRefs == nil {
		rev.SecretRefs = []any{}
	}
	return rev, nil
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
