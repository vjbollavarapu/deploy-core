package applications

import (
	"context"
	"encoding/json"
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
	ResolveEnvironment(ctx context.Context, environmentID uuid.UUID) (orgID, projectID uuid.UUID, err error)
	ServerInOrg(ctx context.Context, orgID, serverID uuid.UUID) (bool, error)
	Create(ctx context.Context, in CreateInput, createdBy uuid.UUID) (Application, error)
	Get(ctx context.Context, id uuid.UUID) (Application, error)
	List(ctx context.Context, orgID uuid.UUID, projectID, environmentID *uuid.UUID, limit, offset int) ([]Application, int64, error)
	UpdateApp(ctx context.Context, id uuid.UUID, in UpdateInput) (Application, error)
	InsertConfig(ctx context.Context, orgID, appID uuid.UUID, version int, in ConfigInput, createdBy uuid.UUID) (Config, error)
	NextConfigVersion(ctx context.Context, appID uuid.UUID) (int, error)
	SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error
	CountActiveDeployments(ctx context.Context, appID uuid.UUID) (int64, error)
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
		SELECT organization_id, project_id FROM environments
		WHERE id = $1 AND deleted_at IS NULL`, environmentID).Scan(&orgID, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, ErrNotFound
	}
	return orgID, projectID, err
}

func (r *PostgresRepository) ServerInOrg(ctx context.Context, orgID, serverID uuid.UUID) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM servers WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
		)`, serverID, orgID).Scan(&ok)
	return ok, err
}

func (r *PostgresRepository) Create(ctx context.Context, in CreateInput, createdBy uuid.UUID) (Application, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Application{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const appQ = `
		INSERT INTO applications (
			organization_id, project_id, environment_id, name, slug, type,
			target_server_id, placement_policy, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, organization_id, project_id, environment_id, name, slug, type, status,
		          target_server_id, placement_policy, created_by, created_at, updated_at`
	app, err := scanApp(tx.QueryRow(ctx, appQ,
		in.OrganizationID, in.ProjectID, in.EnvironmentID, in.Name, in.Slug, in.Type,
		in.TargetServerID, mustJSON(mapOrEmpty(in.PlacementPolicy)), createdBy,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return Application{}, ErrConflict
		}
		return Application{}, err
	}

	cfg, err := insertConfigTx(ctx, tx, in.OrganizationID, app.ID, 1, in.Config, createdBy)
	if err != nil {
		return Application{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Application{}, err
	}
	app.Config = cfg
	return app, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Application, error) {
	const q = `
		SELECT id, organization_id, project_id, environment_id, name, slug, type, status,
		       target_server_id, placement_policy, created_by, created_at, updated_at
		FROM applications WHERE id = $1 AND deleted_at IS NULL`
	app, err := scanApp(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}
	if err != nil {
		return Application{}, err
	}
	cfg, err := r.latestConfig(ctx, id)
	if err != nil {
		return Application{}, err
	}
	app.Config = cfg
	return app, nil
}

func (r *PostgresRepository) List(ctx context.Context, orgID uuid.UUID, projectID, environmentID *uuid.UUID, limit, offset int) ([]Application, int64, error) {
	countQ := `SELECT COUNT(*) FROM applications WHERE organization_id = $1 AND deleted_at IS NULL`
	args := []any{orgID}
	argN := 2
	if projectID != nil {
		countQ += ` AND project_id = $` + itoa(argN)
		args = append(args, *projectID)
		argN++
	}
	if environmentID != nil {
		countQ += ` AND environment_id = $` + itoa(argN)
		args = append(args, *environmentID)
		argN++
	}
	var total int64
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQ := `
		SELECT id, organization_id, project_id, environment_id, name, slug, type, status,
		       target_server_id, placement_policy, created_by, created_at, updated_at
		FROM applications
		WHERE organization_id = $1 AND deleted_at IS NULL`
	listArgs := []any{orgID}
	n := 2
	if projectID != nil {
		listQ += ` AND project_id = $` + itoa(n)
		listArgs = append(listArgs, *projectID)
		n++
	}
	if environmentID != nil {
		listQ += ` AND environment_id = $` + itoa(n)
		listArgs = append(listArgs, *environmentID)
		n++
	}
	listQ += ` ORDER BY name ASC LIMIT $` + itoa(n) + ` OFFSET $` + itoa(n+1)
	listArgs = append(listArgs, limit, offset)

	rows, err := r.pool.Query(ctx, listQ, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Application
	for rows.Next() {
		app, err := scanApp(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, app)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range out {
		cfg, err := r.latestConfig(ctx, out[i].ID)
		if err != nil {
			return nil, 0, err
		}
		out[i].Config = cfg
	}
	return out, total, nil
}

func (r *PostgresRepository) UpdateApp(ctx context.Context, id uuid.UUID, in UpdateInput) (Application, error) {
	cur, err := r.Get(ctx, id)
	if err != nil {
		return Application{}, err
	}
	name, slug := cur.Name, cur.Slug
	if in.Name != nil {
		name = *in.Name
	}
	if in.Slug != nil {
		slug = *in.Slug
	}
	target := cur.TargetServerID
	if in.ClearTarget {
		target = nil
	} else if in.TargetServerID != nil {
		target = in.TargetServerID
	}
	placement := cur.PlacementPolicy
	if in.PlacementPolicy != nil {
		placement = in.PlacementPolicy
	}

	const q = `
		UPDATE applications
		SET name = $2, slug = $3, target_server_id = $4, placement_policy = $5
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, project_id, environment_id, name, slug, type, status,
		          target_server_id, placement_policy, created_by, created_at, updated_at`
	app, err := scanApp(r.pool.QueryRow(ctx, q, id, name, slug, target, mustJSON(mapOrEmpty(placement))))
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Application{}, ErrConflict
	}
	if err != nil {
		return Application{}, err
	}
	app.Config = cur.Config
	return app, nil
}

func (r *PostgresRepository) InsertConfig(ctx context.Context, orgID, appID uuid.UUID, version int, in ConfigInput, createdBy uuid.UUID) (Config, error) {
	return insertConfigTx(ctx, r.pool, orgID, appID, version, in, createdBy)
}

func (r *PostgresRepository) NextConfigVersion(ctx context.Context, appID uuid.UUID) (int, error) {
	var v int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1 FROM application_configs WHERE application_id = $1`, appID).Scan(&v)
	return v, err
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE applications SET deleted_at = $2 WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CountActiveDeployments(ctx context.Context, appID uuid.UUID) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM deployments
		WHERE application_id = $1 AND finished_at IS NULL
		  AND status NOT IN ('RUNNING', 'CANCELLED', 'TIMEOUT',
		                     'SOURCE_FAILED', 'BUILD_FAILED', 'IMAGE_FAILED', 'CONTAINER_FAILED',
		                     'START_FAILED', 'HEALTH_CHECK_FAILED', 'ROUTING_FAILED')`, appID).Scan(&n)
	return n, err
}

func (r *PostgresRepository) latestConfig(ctx context.Context, appID uuid.UUID) (Config, error) {
	const q = `
		SELECT id, version, source_type, repository_url, git_branch, dockerfile_path, build_context,
		       image_reference, internal_port, command, entrypoint, cpu_limit_millis, memory_limit_bytes,
		       restart_policy, health_check, runtime_config, auto_deploy_enabled, git_connection_id,
		       created_at, updated_at
		FROM application_configs
		WHERE application_id = $1
		ORDER BY version DESC
		LIMIT 1`
	cfg, err := scanConfig(r.pool.QueryRow(ctx, q, appID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Config{}, ErrNotFound
	}
	return cfg, err
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func insertConfigTx(ctx context.Context, q querier, orgID, appID uuid.UUID, version int, in ConfigInput, createdBy uuid.UUID) (Config, error) {
	restart := in.RestartPolicy
	if restart == "" {
		restart = "unless-stopped"
	}
	autoDeploy := false
	if in.AutoDeployEnabled != nil {
		autoDeploy = *in.AutoDeployEnabled
	}
	var gitConn any
	if in.ClearGitConnection {
		gitConn = nil
	} else if in.GitConnectionID != nil {
		gitConn = *in.GitConnectionID
	}
	const cfgQ = `
		INSERT INTO application_configs (
			organization_id, application_id, version, source_type, repository_url, git_branch,
			dockerfile_path, build_context, image_reference, internal_port, command, entrypoint,
			cpu_limit_millis, memory_limit_bytes, restart_policy, health_check, runtime_config,
			auto_deploy_enabled, git_connection_id, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		RETURNING id, version, source_type, repository_url, git_branch, dockerfile_path, build_context,
		          image_reference, internal_port, command, entrypoint, cpu_limit_millis, memory_limit_bytes,
		          restart_policy, health_check, runtime_config, auto_deploy_enabled, git_connection_id,
		          created_at, updated_at`
	return scanConfig(q.QueryRow(ctx, cfgQ,
		orgID, appID, version, in.SourceType, in.RepositoryURL, in.GitBranch,
		in.DockerfilePath, in.BuildContext, in.ImageReference, in.InternalPort, in.Command, in.Entrypoint,
		in.CPULimitMillis, in.MemoryLimitBytes, restart,
		mustJSON(mapOrEmpty(in.HealthCheck)), mustJSON(mapOrEmpty(in.RuntimeConfig)),
		autoDeploy, gitConn, createdBy,
	))
}

type scannable interface {
	Scan(dest ...any) error
}

func scanApp(row scannable) (Application, error) {
	var a Application
	var placement []byte
	err := row.Scan(
		&a.ID, &a.OrganizationID, &a.ProjectID, &a.EnvironmentID, &a.Name, &a.Slug, &a.Type, &a.Status,
		&a.TargetServerID, &placement, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return Application{}, err
	}
	a.PlacementPolicy = map[string]any{}
	if len(placement) > 0 {
		_ = json.Unmarshal(placement, &a.PlacementPolicy)
	}
	return a, nil
}

func scanConfig(row scannable) (Config, error) {
	var c Config
	var health, runtime []byte
	err := row.Scan(
		&c.ID, &c.Version, &c.SourceType, &c.RepositoryURL, &c.GitBranch, &c.DockerfilePath, &c.BuildContext,
		&c.ImageReference, &c.InternalPort, &c.Command, &c.Entrypoint, &c.CPULimitMillis, &c.MemoryLimitBytes,
		&c.RestartPolicy, &health, &runtime, &c.AutoDeployEnabled, &c.GitConnectionID, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return Config{}, err
	}
	c.HealthCheck = map[string]any{}
	c.RuntimeConfig = map[string]any{}
	if len(health) > 0 {
		_ = json.Unmarshal(health, &c.HealthCheck)
	}
	if len(runtime) > 0 {
		_ = json.Unmarshal(runtime, &c.RuntimeConfig)
	}
	return c, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
