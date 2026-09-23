package metrics

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("metrics: not found")

type Repository interface {
	GetServerOrg(ctx context.Context, serverID uuid.UUID) (uuid.UUID, error)
	GetApplicationOrg(ctx context.Context, appID uuid.UUID) (uuid.UUID, error)
	GetApplicationPlacement(ctx context.Context, appID uuid.UUID) (orgID uuid.UUID, serverID *uuid.UUID, err error)
	UpsertServerSnapshot(ctx context.Context, snap ServerSnapshot) (ServerSnapshot, error)
	GetServerSnapshot(ctx context.Context, serverID uuid.UUID) (ServerSnapshot, error)
	UpsertContainerSnapshot(ctx context.Context, snap ContainerSnapshot) (ContainerSnapshot, error)
	ListContainerSnapshots(ctx context.Context, serverID uuid.UUID) ([]ContainerSnapshot, error)
	GetContainerByApplication(ctx context.Context, appID uuid.UUID) (ContainerSnapshot, error)
	LatestHeartbeatAsServer(ctx context.Context, serverID uuid.UUID) (ServerSnapshot, error)
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

func (r *PostgresRepository) GetApplicationOrg(ctx context.Context, appID uuid.UUID) (uuid.UUID, error) {
	var orgID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT organization_id FROM applications WHERE id = $1 AND deleted_at IS NULL`, appID).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return orgID, err
}

func (r *PostgresRepository) GetApplicationPlacement(ctx context.Context, appID uuid.UUID) (uuid.UUID, *uuid.UUID, error) {
	var orgID uuid.UUID
	var serverID *uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT organization_id, target_server_id FROM applications WHERE id = $1 AND deleted_at IS NULL`, appID).
		Scan(&orgID, &serverID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil, ErrNotFound
	}
	return orgID, serverID, err
}

func (r *PostgresRepository) UpsertServerSnapshot(ctx context.Context, snap ServerSnapshot) (ServerSnapshot, error) {
	if snap.Payload == nil {
		snap.Payload = map[string]any{}
	}
	payload, _ := json.Marshal(snap.Payload)
	if snap.Source == "" {
		snap.Source = SourceAgent
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO server_metric_snapshots (
			server_id, organization_id, recorded_at,
			cpu_percent, memory_used_bytes, memory_total_bytes,
			disk_used_bytes, disk_total_bytes,
			load_1, load_5, load_15, uptime_seconds,
			network_rx_bytes, network_tx_bytes, container_count,
			source, payload
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17
		)
		ON CONFLICT (server_id) DO UPDATE SET
			organization_id = EXCLUDED.organization_id,
			recorded_at = EXCLUDED.recorded_at,
			cpu_percent = EXCLUDED.cpu_percent,
			memory_used_bytes = EXCLUDED.memory_used_bytes,
			memory_total_bytes = COALESCE(EXCLUDED.memory_total_bytes, server_metric_snapshots.memory_total_bytes),
			disk_used_bytes = EXCLUDED.disk_used_bytes,
			disk_total_bytes = COALESCE(EXCLUDED.disk_total_bytes, server_metric_snapshots.disk_total_bytes),
			load_1 = EXCLUDED.load_1,
			load_5 = COALESCE(EXCLUDED.load_5, server_metric_snapshots.load_5),
			load_15 = COALESCE(EXCLUDED.load_15, server_metric_snapshots.load_15),
			uptime_seconds = EXCLUDED.uptime_seconds,
			network_rx_bytes = COALESCE(EXCLUDED.network_rx_bytes, server_metric_snapshots.network_rx_bytes),
			network_tx_bytes = COALESCE(EXCLUDED.network_tx_bytes, server_metric_snapshots.network_tx_bytes),
			container_count = EXCLUDED.container_count,
			source = EXCLUDED.source,
			payload = EXCLUDED.payload,
			updated_at = NOW()
		RETURNING server_id, organization_id, recorded_at,
			cpu_percent, memory_used_bytes, memory_total_bytes,
			disk_used_bytes, disk_total_bytes,
			load_1, load_5, load_15, uptime_seconds,
			network_rx_bytes, network_tx_bytes, container_count,
			source, payload, updated_at`,
		snap.ServerID, snap.OrganizationID, snap.RecordedAt,
		snap.CPUPercent, snap.MemoryUsedBytes, snap.MemoryTotalBytes,
		snap.DiskUsedBytes, snap.DiskTotalBytes,
		snap.Load1, snap.Load5, snap.Load15, snap.UptimeSeconds,
		snap.NetworkRxBytes, snap.NetworkTxBytes, snap.ContainerCount,
		snap.Source, payload,
	)
	return scanServer(row)
}

func (r *PostgresRepository) GetServerSnapshot(ctx context.Context, serverID uuid.UUID) (ServerSnapshot, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT server_id, organization_id, recorded_at,
			cpu_percent, memory_used_bytes, memory_total_bytes,
			disk_used_bytes, disk_total_bytes,
			load_1, load_5, load_15, uptime_seconds,
			network_rx_bytes, network_tx_bytes, container_count,
			source, payload, updated_at
		FROM server_metric_snapshots WHERE server_id = $1`, serverID)
	snap, err := scanServer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ServerSnapshot{}, ErrNotFound
	}
	return snap, err
}

func (r *PostgresRepository) UpsertContainerSnapshot(ctx context.Context, snap ContainerSnapshot) (ContainerSnapshot, error) {
	if snap.Payload == nil {
		snap.Payload = map[string]any{}
	}
	payload, _ := json.Marshal(snap.Payload)
	if snap.Status == "" {
		snap.Status = "unknown"
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO container_metric_snapshots (
			organization_id, server_id, application_id, container_id, container_name, recorded_at,
			cpu_percent, memory_used_bytes, memory_limit_bytes,
			network_rx_bytes, network_tx_bytes, restart_count, status, payload
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
		)
		ON CONFLICT (server_id, container_id) DO UPDATE SET
			organization_id = EXCLUDED.organization_id,
			application_id = COALESCE(EXCLUDED.application_id, container_metric_snapshots.application_id),
			container_name = EXCLUDED.container_name,
			recorded_at = EXCLUDED.recorded_at,
			cpu_percent = EXCLUDED.cpu_percent,
			memory_used_bytes = EXCLUDED.memory_used_bytes,
			memory_limit_bytes = EXCLUDED.memory_limit_bytes,
			network_rx_bytes = EXCLUDED.network_rx_bytes,
			network_tx_bytes = EXCLUDED.network_tx_bytes,
			restart_count = EXCLUDED.restart_count,
			status = EXCLUDED.status,
			payload = EXCLUDED.payload,
			updated_at = NOW()
		RETURNING id, organization_id, server_id, application_id, container_id, container_name, recorded_at,
			cpu_percent, memory_used_bytes, memory_limit_bytes,
			network_rx_bytes, network_tx_bytes, restart_count, status, payload, updated_at`,
		snap.OrganizationID, snap.ServerID, snap.ApplicationID, snap.ContainerID, snap.ContainerName, snap.RecordedAt,
		snap.CPUPercent, snap.MemoryUsedBytes, snap.MemoryLimitBytes,
		snap.NetworkRxBytes, snap.NetworkTxBytes, snap.RestartCount, snap.Status, payload,
	)
	return scanContainer(row)
}

func (r *PostgresRepository) ListContainerSnapshots(ctx context.Context, serverID uuid.UUID) ([]ContainerSnapshot, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, server_id, application_id, container_id, container_name, recorded_at,
			cpu_percent, memory_used_bytes, memory_limit_bytes,
			network_rx_bytes, network_tx_bytes, restart_count, status, payload, updated_at
		FROM container_metric_snapshots
		WHERE server_id = $1
		ORDER BY container_name ASC, container_id ASC`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ContainerSnapshot
	for rows.Next() {
		c, err := scanContainer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetContainerByApplication(ctx context.Context, appID uuid.UUID) (ContainerSnapshot, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, server_id, application_id, container_id, container_name, recorded_at,
			cpu_percent, memory_used_bytes, memory_limit_bytes,
			network_rx_bytes, network_tx_bytes, restart_count, status, payload, updated_at
		FROM container_metric_snapshots
		WHERE application_id = $1
		ORDER BY recorded_at DESC
		LIMIT 1`, appID)
	c, err := scanContainer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ContainerSnapshot{}, ErrNotFound
	}
	return c, err
}

func (r *PostgresRepository) LatestHeartbeatAsServer(ctx context.Context, serverID uuid.UUID) (ServerSnapshot, error) {
	var snap ServerSnapshot
	var payload []byte
	err := r.pool.QueryRow(ctx, `
		SELECT organization_id, server_id, recorded_at,
			cpu_percent, memory_used_bytes, disk_used_bytes, load_1, container_count, uptime_seconds, payload
		FROM server_heartbeats
		WHERE server_id = $1
		ORDER BY recorded_at DESC
		LIMIT 1`, serverID).Scan(
		&snap.OrganizationID, &snap.ServerID, &snap.RecordedAt,
		&snap.CPUPercent, &snap.MemoryUsedBytes, &snap.DiskUsedBytes, &snap.Load1,
		&snap.ContainerCount, &snap.UptimeSeconds, &payload,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ServerSnapshot{}, ErrNotFound
	}
	if err != nil {
		return ServerSnapshot{}, err
	}
	snap.Source = SourceHeartbeat
	snap.UpdatedAt = snap.RecordedAt
	_ = json.Unmarshal(payload, &snap.Payload)
	if snap.Payload == nil {
		snap.Payload = map[string]any{}
	}
	return snap, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanServer(row scannable) (ServerSnapshot, error) {
	var s ServerSnapshot
	var payload []byte
	err := row.Scan(
		&s.ServerID, &s.OrganizationID, &s.RecordedAt,
		&s.CPUPercent, &s.MemoryUsedBytes, &s.MemoryTotalBytes,
		&s.DiskUsedBytes, &s.DiskTotalBytes,
		&s.Load1, &s.Load5, &s.Load15, &s.UptimeSeconds,
		&s.NetworkRxBytes, &s.NetworkTxBytes, &s.ContainerCount,
		&s.Source, &payload, &s.UpdatedAt,
	)
	if err != nil {
		return ServerSnapshot{}, err
	}
	_ = json.Unmarshal(payload, &s.Payload)
	if s.Payload == nil {
		s.Payload = map[string]any{}
	}
	return s, nil
}

func scanContainer(row scannable) (ContainerSnapshot, error) {
	var c ContainerSnapshot
	var payload []byte
	err := row.Scan(
		&c.ID, &c.OrganizationID, &c.ServerID, &c.ApplicationID, &c.ContainerID, &c.ContainerName, &c.RecordedAt,
		&c.CPUPercent, &c.MemoryUsedBytes, &c.MemoryLimitBytes,
		&c.NetworkRxBytes, &c.NetworkTxBytes, &c.RestartCount, &c.Status, &payload, &c.UpdatedAt,
	)
	if err != nil {
		return ContainerSnapshot{}, err
	}
	_ = json.Unmarshal(payload, &c.Payload)
	if c.Payload == nil {
		c.Payload = map[string]any{}
	}
	return c, nil
}
