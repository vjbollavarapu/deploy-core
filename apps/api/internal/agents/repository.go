package agents

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
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type Repository interface {
	GetServerMeta(ctx context.Context, serverID uuid.UUID) (orgID uuid.UUID, status string, maintenance bool, err error)
	UpsertRegistrationToken(ctx context.Context, orgID, serverID uuid.UUID, tokenHash string, expiresAt time.Time) (Agent, error)
	RevokeRegistrationToken(ctx context.Context, serverID uuid.UUID, at time.Time) error
	RevokeAgentCredential(ctx context.Context, serverID uuid.UUID, at time.Time) error
	GetByRegistrationTokenHash(ctx context.Context, tokenHash string) (AgentRecord, error)
	CompleteRegistration(ctx context.Context, agentID uuid.UUID, credentialHash, agentVersion string, at time.Time) (Agent, error)
	GetByCredentialHash(ctx context.Context, credentialHash string) (AgentRecord, error)
	TouchAgent(ctx context.Context, agentID uuid.UUID, version string, at time.Time) error
	ApplyServerHeartbeat(ctx context.Context, serverID uuid.UUID, at time.Time, status string, dockerVersion *string) error
	InsertHeartbeat(ctx context.Context, orgID, serverID, agentID uuid.UUID, in HeartbeatInput, at time.Time) error
	PruneHeartbeats(ctx context.Context, serverID uuid.UUID, keep int) error
	MarkHeartbeatExpired(ctx context.Context, cutoff time.Time) ([]ExpiredServer, error)
}

type ExpiredServer struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	PreviousStatus string
}

type AgentRecord struct {
	Agent
	CredentialHash        string
	RegistrationTokenHash *string
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetServerMeta(ctx context.Context, serverID uuid.UUID) (uuid.UUID, string, bool, error) {
	var orgID uuid.UUID
	var status string
	var maintenance bool
	err := r.pool.QueryRow(ctx, `
		SELECT organization_id, status, maintenance_mode
		FROM servers WHERE id = $1 AND deleted_at IS NULL`, serverID).Scan(&orgID, &status, &maintenance)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", false, ErrNotFound
	}
	return orgID, status, maintenance, err
}

func (r *PostgresRepository) UpsertRegistrationToken(ctx context.Context, orgID, serverID uuid.UUID, tokenHash string, expiresAt time.Time) (Agent, error) {
	const q = `
		INSERT INTO server_agents (
			organization_id, server_id, agent_version, credential_hash,
			registration_token_hash, registration_expires_at,
			registration_used_at, registration_revoked_at, status
		) VALUES ($1, $2, '', '', $3, $4, NULL, NULL, 'pending')
		ON CONFLICT (server_id) DO UPDATE SET
			registration_token_hash = EXCLUDED.registration_token_hash,
			registration_expires_at = EXCLUDED.registration_expires_at,
			registration_used_at = NULL,
			registration_revoked_at = NULL,
			status = CASE
				WHEN server_agents.status = 'active' THEN server_agents.status
				ELSE 'pending'
			END,
			updated_at = NOW()
		RETURNING id, organization_id, server_id, agent_version, status,
		          registration_expires_at, registration_used_at, registration_revoked_at,
		          last_seen_at, created_at, updated_at`
	return scanAgent(r.pool.QueryRow(ctx, q, orgID, serverID, tokenHash, expiresAt))
}

func (r *PostgresRepository) RevokeRegistrationToken(ctx context.Context, serverID uuid.UUID, at time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE server_agents
		SET registration_revoked_at = $2,
		    registration_token_hash = NULL,
		    updated_at = NOW()
		WHERE server_id = $1
		  AND registration_token_hash IS NOT NULL
		  AND registration_used_at IS NULL
		  AND registration_revoked_at IS NULL`, serverID, at)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) RevokeAgentCredential(ctx context.Context, serverID uuid.UUID, at time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE server_agents
		SET credential_hash = '',
		    status = 'revoked',
		    updated_at = NOW()
		WHERE server_id = $1
		  AND status = 'active'
		  AND credential_hash <> ''`, serverID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	_ = at
	return nil
}

func (r *PostgresRepository) GetByRegistrationTokenHash(ctx context.Context, tokenHash string) (AgentRecord, error) {
	const q = `
		SELECT id, organization_id, server_id, agent_version, status,
		       registration_expires_at, registration_used_at, registration_revoked_at,
		       last_seen_at, created_at, updated_at,
		       credential_hash, registration_token_hash
		FROM server_agents
		WHERE registration_token_hash = $1`
	rec, err := scanAgentRecord(r.pool.QueryRow(ctx, q, tokenHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return AgentRecord{}, ErrNotFound
	}
	return rec, err
}

func (r *PostgresRepository) CompleteRegistration(ctx context.Context, agentID uuid.UUID, credentialHash, agentVersion string, at time.Time) (Agent, error) {
	const q = `
		UPDATE server_agents
		SET credential_hash = $2,
		    agent_version = $3,
		    registration_used_at = $4,
		    registration_token_hash = NULL,
		    status = 'active',
		    last_seen_at = $4,
		    updated_at = NOW()
		WHERE id = $1
		  AND registration_used_at IS NULL
		  AND registration_revoked_at IS NULL
		RETURNING id, organization_id, server_id, agent_version, status,
		          registration_expires_at, registration_used_at, registration_revoked_at,
		          last_seen_at, created_at, updated_at`
	a, err := scanAgent(r.pool.QueryRow(ctx, q, agentID, credentialHash, agentVersion, at))
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, ErrConflict
	}
	return a, err
}

func (r *PostgresRepository) GetByCredentialHash(ctx context.Context, credentialHash string) (AgentRecord, error) {
	const q = `
		SELECT id, organization_id, server_id, agent_version, status,
		       registration_expires_at, registration_used_at, registration_revoked_at,
		       last_seen_at, created_at, updated_at,
		       credential_hash, registration_token_hash
		FROM server_agents
		WHERE credential_hash = $1 AND status = 'active' AND credential_hash <> ''`
	rec, err := scanAgentRecord(r.pool.QueryRow(ctx, q, credentialHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return AgentRecord{}, ErrNotFound
	}
	return rec, err
}

func (r *PostgresRepository) TouchAgent(ctx context.Context, agentID uuid.UUID, version string, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE server_agents
		SET last_seen_at = $2,
		    agent_version = COALESCE(NULLIF($3, ''), agent_version),
		    updated_at = NOW()
		WHERE id = $1`, agentID, at, version)
	return err
}

func (r *PostgresRepository) ApplyServerHeartbeat(ctx context.Context, serverID uuid.UUID, at time.Time, status string, dockerVersion *string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE servers
		SET last_heartbeat_at = $2,
		    status = CASE
		        WHEN status IN ('DISABLED', 'MAINTENANCE') OR maintenance_mode THEN status
		        ELSE $3
		    END,
		    docker_version = COALESCE($4, docker_version),
		    updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, serverID, at, status, dockerVersion)
	return err
}

func (r *PostgresRepository) InsertHeartbeat(ctx context.Context, orgID, serverID, agentID uuid.UUID, in HeartbeatInput, at time.Time) error {
	payload, _ := json.Marshal(in)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO server_heartbeats (
			organization_id, server_id, agent_id, recorded_at, agent_version, docker_status,
			cpu_percent, memory_used_bytes, disk_used_bytes, load_1, container_count, uptime_seconds, payload
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		orgID, serverID, agentID, at, in.AgentVersion, in.DockerStatus,
		in.CPUPercent, in.MemoryUsedBytes, in.DiskUsedBytes, in.Load1, in.ContainerCount, in.UptimeSeconds, payload,
	)
	return err
}

func (r *PostgresRepository) PruneHeartbeats(ctx context.Context, serverID uuid.UUID, keep int) error {
	if keep <= 0 {
		keep = 50
	}
	_, err := r.pool.Exec(ctx, `
		DELETE FROM server_heartbeats
		WHERE server_id = $1
		  AND id NOT IN (
			SELECT id FROM server_heartbeats
			WHERE server_id = $1
			ORDER BY recorded_at DESC
			LIMIT $2
		  )`, serverID, keep)
	return err
}

func (r *PostgresRepository) MarkHeartbeatExpired(ctx context.Context, cutoff time.Time) ([]ExpiredServer, error) {
	rows, err := r.pool.Query(ctx, `
		WITH expired AS (
			SELECT id, organization_id, status AS previous_status
			FROM servers
			WHERE deleted_at IS NULL
			  AND maintenance_mode = FALSE
			  AND status IN ('ONLINE', 'DEGRADED')
			  AND (last_heartbeat_at IS NULL OR last_heartbeat_at < $1)
		)
		UPDATE servers s
		SET status = 'OFFLINE'
		FROM expired e
		WHERE s.id = e.id
		RETURNING e.id, e.organization_id, e.previous_status`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpiredServer
	for rows.Next() {
		var e ExpiredServer
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.PreviousStatus); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanAgent(row scannable) (Agent, error) {
	var a Agent
	err := row.Scan(
		&a.ID, &a.OrganizationID, &a.ServerID, &a.AgentVersion, &a.Status,
		&a.RegistrationExpiresAt, &a.RegistrationUsedAt, &a.RegistrationRevokedAt,
		&a.LastSeenAt, &a.CreatedAt, &a.UpdatedAt,
	)
	return a, err
}

func scanAgentRecord(row scannable) (AgentRecord, error) {
	var rec AgentRecord
	err := row.Scan(
		&rec.ID, &rec.OrganizationID, &rec.ServerID, &rec.AgentVersion, &rec.Status,
		&rec.RegistrationExpiresAt, &rec.RegistrationUsedAt, &rec.RegistrationRevokedAt,
		&rec.LastSeenAt, &rec.CreatedAt, &rec.UpdatedAt,
		&rec.CredentialHash, &rec.RegistrationTokenHash,
	)
	return rec, err
}
