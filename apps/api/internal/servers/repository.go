package servers

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
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
	Create(ctx context.Context, in CreateInput, createdBy uuid.UUID) (Server, error)
	Get(ctx context.Context, id uuid.UUID) (Server, error)
	List(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Server, int64, error)
	ListActive(ctx context.Context, orgID uuid.UUID) ([]Server, error)
	Update(ctx context.Context, id uuid.UUID, in UpdateInput, status *string, maintenance *bool) (Server, error)
	SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error
	CountActiveApplications(ctx context.Context, serverID uuid.UUID) (int64, error)
	RecomputeAllocated(ctx context.Context, serverID uuid.UUID) (Server, error)
	SetApplicationTarget(ctx context.Context, appID, serverID uuid.UUID) error
	GetPlacementContext(ctx context.Context, appID uuid.UUID) (PlacementContext, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, in CreateInput, createdBy uuid.UUID) (Server, error) {
	labels := labelsOrEmpty(in.Labels)
	const q = `
		INSERT INTO servers (
			organization_id, name, provider, region, hostname,
			public_ip, private_ip, architecture, operating_system,
			cpu_cores, memory_bytes, disk_bytes, docker_version,
			status, maintenance_mode, labels, created_by
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'OFFLINE',FALSE,$14,$15
		)
		RETURNING id, organization_id, name, provider, region, hostname,
		          host(public_ip)::text, host(private_ip)::text, architecture, operating_system,
		          cpu_cores, memory_bytes, disk_bytes,
		          cpu_allocated_millis, memory_allocated_bytes, disk_allocated_bytes,
		          docker_version, status, maintenance_mode, last_heartbeat_at, labels,
		          created_by, created_at, updated_at`
	s, err := scanServer(r.pool.QueryRow(ctx, q,
		in.OrganizationID, in.Name, in.Provider, in.Region, in.Hostname,
		parseInet(in.PublicIP), parseInet(in.PrivateIP), in.Architecture, in.OperatingSystem,
		in.CPUCores, in.MemoryBytes, in.DiskBytes, in.DockerVersion,
		mustJSON(labels), createdBy,
	))
	if isUniqueViolation(err) {
		return Server{}, ErrConflict
	}
	return s, err
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Server, error) {
	const q = `
		SELECT id, organization_id, name, provider, region, hostname,
		       host(public_ip)::text, host(private_ip)::text, architecture, operating_system,
		       cpu_cores, memory_bytes, disk_bytes,
		       cpu_allocated_millis, memory_allocated_bytes, disk_allocated_bytes,
		       docker_version, status, maintenance_mode, last_heartbeat_at, labels,
		       created_by, created_at, updated_at
		FROM servers WHERE id = $1 AND deleted_at IS NULL`
	s, err := scanServer(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Server{}, ErrNotFound
	}
	return s, err
}

func (r *PostgresRepository) List(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Server, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM servers WHERE organization_id = $1 AND deleted_at IS NULL`, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, provider, region, hostname,
		       host(public_ip)::text, host(private_ip)::text, architecture, operating_system,
		       cpu_cores, memory_bytes, disk_bytes,
		       cpu_allocated_millis, memory_allocated_bytes, disk_allocated_bytes,
		       docker_version, status, maintenance_mode, last_heartbeat_at, labels,
		       created_by, created_at, updated_at
		FROM servers
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC
		LIMIT $2 OFFSET $3`, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Server
	for rows.Next() {
		s, err := scanServer(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, s)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) ListActive(ctx context.Context, orgID uuid.UUID) ([]Server, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, provider, region, hostname,
		       host(public_ip)::text, host(private_ip)::text, architecture, operating_system,
		       cpu_cores, memory_bytes, disk_bytes,
		       cpu_allocated_millis, memory_allocated_bytes, disk_allocated_bytes,
		       docker_version, status, maintenance_mode, last_heartbeat_at, labels,
		       created_by, created_at, updated_at
		FROM servers
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Server
	for rows.Next() {
		s, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, id uuid.UUID, in UpdateInput, status *string, maintenance *bool) (Server, error) {
	cur, err := r.Get(ctx, id)
	if err != nil {
		return Server{}, err
	}

	name := cur.Name
	if in.Name != nil {
		name = *in.Name
	}
	provider := cur.Provider
	if in.Provider != nil {
		provider = *in.Provider
	}
	region := cur.Region
	if in.Region != nil {
		region = *in.Region
	}
	hostname := cur.Hostname
	if in.Hostname != nil {
		hostname = *in.Hostname
	}
	arch := cur.Architecture
	if in.Architecture != nil {
		arch = *in.Architecture
	}
	osName := cur.OperatingSystem
	if in.OperatingSystem != nil {
		osName = *in.OperatingSystem
	}
	cpu := cur.CPUCores
	if in.CPUCores != nil {
		cpu = in.CPUCores
	}
	mem := cur.MemoryBytes
	if in.MemoryBytes != nil {
		mem = in.MemoryBytes
	}
	disk := cur.DiskBytes
	if in.DiskBytes != nil {
		disk = in.DiskBytes
	}
	docker := cur.DockerVersion
	if in.DockerVersion != nil {
		docker = in.DockerVersion
	}
	labels := cur.Labels
	if in.Labels != nil {
		labels = in.Labels
	}

	var publicIP, privateIP any
	switch {
	case in.ClearPublicIP:
		publicIP = nil
	case in.PublicIP != nil:
		publicIP = parseInet(in.PublicIP)
	default:
		publicIP = parseInet(cur.PublicIP)
	}
	switch {
	case in.ClearPrivateIP:
		privateIP = nil
	case in.PrivateIP != nil:
		privateIP = parseInet(in.PrivateIP)
	default:
		privateIP = parseInet(cur.PrivateIP)
	}

	newStatus := cur.Status
	if status != nil {
		newStatus = *status
	}
	maint := cur.MaintenanceMode
	if maintenance != nil {
		maint = *maintenance
	}

	const q = `
		UPDATE servers SET
			name = $2, provider = $3, region = $4, hostname = $5,
			public_ip = $6, private_ip = $7, architecture = $8, operating_system = $9,
			cpu_cores = $10, memory_bytes = $11, disk_bytes = $12, docker_version = $13,
			status = $14, maintenance_mode = $15, labels = $16
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, name, provider, region, hostname,
		          host(public_ip)::text, host(private_ip)::text, architecture, operating_system,
		          cpu_cores, memory_bytes, disk_bytes,
		          cpu_allocated_millis, memory_allocated_bytes, disk_allocated_bytes,
		          docker_version, status, maintenance_mode, last_heartbeat_at, labels,
		          created_by, created_at, updated_at`
	s, err := scanServer(r.pool.QueryRow(ctx, q,
		id, name, provider, region, hostname, publicIP, privateIP, arch, osName,
		cpu, mem, disk, docker, newStatus, maint, mustJSON(labelsOrEmpty(labels)),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Server{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Server{}, ErrConflict
	}
	return s, err
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE servers SET deleted_at = $2 WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CountActiveApplications(ctx context.Context, serverID uuid.UUID) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM applications
		WHERE target_server_id = $1 AND deleted_at IS NULL`, serverID).Scan(&n)
	return n, err
}

// RecomputeAllocated refreshes denormalized allocation counters from current application configs.
func (r *PostgresRepository) RecomputeAllocated(ctx context.Context, serverID uuid.UUID) (Server, error) {
	const q = `
		WITH latest AS (
			SELECT DISTINCT ON (ac.application_id)
				ac.application_id,
				COALESCE(ac.cpu_limit_millis, 0) *
					GREATEST(1, COALESCE((ac.runtime_config->>'desiredReplicas')::int, 1)) AS cpu_millis,
				COALESCE(ac.memory_limit_bytes, 0) *
					GREATEST(1, COALESCE((ac.runtime_config->>'desiredReplicas')::int, 1)) AS memory_bytes,
				COALESCE((ac.runtime_config->>'diskBytes')::bigint, 0) *
					GREATEST(1, COALESCE((ac.runtime_config->>'desiredReplicas')::int, 1)) AS disk_bytes
			FROM application_configs ac
			JOIN applications a ON a.id = ac.application_id
			WHERE a.target_server_id = $1 AND a.deleted_at IS NULL
			ORDER BY ac.application_id, ac.version DESC
		),
		sums AS (
			SELECT
				COALESCE(SUM(cpu_millis), 0)::int AS cpu_allocated_millis,
				COALESCE(SUM(memory_bytes), 0)::bigint AS memory_allocated_bytes,
				COALESCE(SUM(disk_bytes), 0)::bigint AS disk_allocated_bytes
			FROM latest
		)
		UPDATE servers s SET
			cpu_allocated_millis = sums.cpu_allocated_millis,
			memory_allocated_bytes = sums.memory_allocated_bytes,
			disk_allocated_bytes = sums.disk_allocated_bytes
		FROM sums
		WHERE s.id = $1 AND s.deleted_at IS NULL
		RETURNING s.id, s.organization_id, s.name, s.provider, s.region, s.hostname,
		          host(s.public_ip)::text, host(s.private_ip)::text, s.architecture, s.operating_system,
		          s.cpu_cores, s.memory_bytes, s.disk_bytes,
		          s.cpu_allocated_millis, s.memory_allocated_bytes, s.disk_allocated_bytes,
		          s.docker_version, s.status, s.maintenance_mode, s.last_heartbeat_at, s.labels,
		          s.created_by, s.created_at, s.updated_at`
	s, err := scanServer(r.pool.QueryRow(ctx, q, serverID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Server{}, ErrNotFound
	}
	return s, err
}

func (r *PostgresRepository) SetApplicationTarget(ctx context.Context, appID, serverID uuid.UUID) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE applications SET target_server_id = $2
		WHERE id = $1 AND deleted_at IS NULL`, appID, serverID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) GetPlacementContext(ctx context.Context, appID uuid.UUID) (PlacementContext, error) {
	var pc PlacementContext
	var policy []byte
	err := r.pool.QueryRow(ctx, `
		SELECT a.id, a.organization_id, a.target_server_id, a.placement_policy,
		       COALESCE(ac.cpu_limit_millis, 0) *
		         GREATEST(1, COALESCE((ac.runtime_config->>'desiredReplicas')::int, 1)),
		       COALESCE(ac.memory_limit_bytes, 0) *
		         GREATEST(1, COALESCE((ac.runtime_config->>'desiredReplicas')::int, 1)),
		       CASE
		         WHEN ac.runtime_config ? 'diskBytes'
		           THEN COALESCE((ac.runtime_config->>'diskBytes')::bigint, 0) *
		                GREATEST(1, COALESCE((ac.runtime_config->>'desiredReplicas')::int, 1))
		         ELSE 0
		       END
		FROM applications a
		LEFT JOIN LATERAL (
			SELECT cpu_limit_millis, memory_limit_bytes, runtime_config
			FROM application_configs
			WHERE application_id = a.id
			ORDER BY version DESC
			LIMIT 1
		) ac ON TRUE
		WHERE a.id = $1 AND a.deleted_at IS NULL`, appID).Scan(
		&pc.ApplicationID, &pc.OrganizationID, &pc.TargetServerID, &policy,
		&pc.CPULimitMillis, &pc.MemoryLimitBytes, &pc.DiskBytes,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlacementContext{}, ErrNotFound
	}
	if err != nil {
		return PlacementContext{}, err
	}
	pc.PlacementPolicy = map[string]any{}
	if len(policy) > 0 {
		_ = json.Unmarshal(policy, &pc.PlacementPolicy)
	}
	return pc, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanServer(row scannable) (Server, error) {
	var s Server
	var labels []byte
	err := row.Scan(
		&s.ID, &s.OrganizationID, &s.Name, &s.Provider, &s.Region, &s.Hostname,
		&s.PublicIP, &s.PrivateIP, &s.Architecture, &s.OperatingSystem,
		&s.CPUCores, &s.MemoryBytes, &s.DiskBytes,
		&s.CPUAllocatedMillis, &s.MemoryAllocatedBytes, &s.DiskAllocatedBytes,
		&s.DockerVersion, &s.Status, &s.MaintenanceMode, &s.LastHeartbeatAt, &labels,
		&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return Server{}, err
	}
	s.Labels = map[string]string{}
	if len(labels) > 0 {
		_ = json.Unmarshal(labels, &s.Labels)
	}
	return s, nil
}

func parseInet(ip *string) any {
	if ip == nil {
		return nil
	}
	v := strings.TrimSpace(*ip)
	if v == "" {
		return nil
	}
	addr, err := netip.ParseAddr(v)
	if err != nil {
		return nil
	}
	return addr
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
