package agentcmd

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/deploycore/deploy-core/packages/protocol-go"
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
	GetServerOrg(ctx context.Context, serverID uuid.UUID) (uuid.UUID, string, error)
	Create(ctx context.Context, cmd Command) (Command, error)
	Get(ctx context.Context, id uuid.UUID) (Command, error)
	ListForServer(ctx context.Context, orgID, serverID uuid.UUID, limit, offset int) ([]Command, int64, error)
	ListPendingForServer(ctx context.Context, serverID uuid.UUID, limit int, now time.Time) ([]Command, error)
	ExpirePending(ctx context.Context, serverID uuid.UUID, now time.Time) error
	// ExpireStaleInFlight marks accepted/running commands past expires_at as failed
	// and returns the updated rows so completion hooks can run.
	ExpireStaleInFlight(ctx context.Context, serverID uuid.UUID, now time.Time) ([]Command, error)
	UpdateStatus(ctx context.Context, id, serverID uuid.UUID, fromStatuses []string, toStatus string, result map[string]any, errCode, errMsg *string, at time.Time) (Command, error)
	Cancel(ctx context.Context, id, orgID uuid.UUID, at time.Time) (Command, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetServerOrg(ctx context.Context, serverID uuid.UUID) (uuid.UUID, string, error) {
	var orgID uuid.UUID
	var status string
	err := r.pool.QueryRow(ctx, `
		SELECT organization_id, status FROM servers WHERE id = $1 AND deleted_at IS NULL`, serverID).Scan(&orgID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", ErrNotFound
	}
	return orgID, status, err
}

func (r *PostgresRepository) Create(ctx context.Context, cmd Command) (Command, error) {
	const q = `
		INSERT INTO agent_commands (
			organization_id, server_id, operation, schema_version, payload, status,
			issued_at, expires_at, request_id, correlation_id, issued_by
		) VALUES ($1,$2,$3,$4,$5,'pending',$6,$7,$8,$9,$10)
		RETURNING id, organization_id, server_id, operation, schema_version, payload, status,
		          issued_at, expires_at, request_id, correlation_id, issued_by,
		          result, error_code, error_message, accepted_at, started_at, finished_at,
		          created_at, updated_at`
	out, err := scanCommand(r.pool.QueryRow(ctx, q,
		cmd.OrganizationID, cmd.ServerID, cmd.Operation, cmd.SchemaVersion, payloadJSON(cmd.Payload),
		cmd.IssuedAt, cmd.ExpiresAt, cmd.RequestID, cmd.CorrelationID, cmd.IssuedBy,
	))
	if isCheckViolation(err) {
		return Command{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Command, error) {
	const q = `
		SELECT id, organization_id, server_id, operation, schema_version, payload, status,
		       issued_at, expires_at, request_id, correlation_id, issued_by,
		       result, error_code, error_message, accepted_at, started_at, finished_at,
		       created_at, updated_at
		FROM agent_commands WHERE id = $1`
	cmd, err := scanCommand(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Command{}, ErrNotFound
	}
	return cmd, err
}

func (r *PostgresRepository) ListForServer(ctx context.Context, orgID, serverID uuid.UUID, limit, offset int) ([]Command, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM agent_commands WHERE organization_id = $1 AND server_id = $2`, orgID, serverID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, server_id, operation, schema_version, payload, status,
		       issued_at, expires_at, request_id, correlation_id, issued_by,
		       result, error_code, error_message, accepted_at, started_at, finished_at,
		       created_at, updated_at
		FROM agent_commands
		WHERE organization_id = $1 AND server_id = $2
		ORDER BY issued_at DESC
		LIMIT $3 OFFSET $4`, orgID, serverID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Command
	for rows.Next() {
		cmd, err := scanCommand(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, cmd)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) ListPendingForServer(ctx context.Context, serverID uuid.UUID, limit int, now time.Time) ([]Command, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, server_id, operation, schema_version, payload, status,
		       issued_at, expires_at, request_id, correlation_id, issued_by,
		       result, error_code, error_message, accepted_at, started_at, finished_at,
		       created_at, updated_at
		FROM agent_commands
		WHERE server_id = $1 AND status = 'pending' AND expires_at > $2
		ORDER BY issued_at ASC
		LIMIT $3`, serverID, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Command
	for rows.Next() {
		cmd, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cmd)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ExpirePending(ctx context.Context, serverID uuid.UUID, now time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE agent_commands
		SET status = 'expired', finished_at = $2,
		    error_code = COALESCE(error_code, 'COMMAND_EXPIRED'),
		    error_message = COALESCE(error_message, 'command expired before acceptance')
		WHERE server_id = $1 AND status = 'pending' AND expires_at <= $2`, serverID, now)
	return err
}

// ExpireStaleInFlight fails accepted/running commands whose expires_at has passed.
// Without this, Agent disappearance can leave commands RUNNING indefinitely (I8).
func (r *PostgresRepository) ExpireStaleInFlight(ctx context.Context, serverID uuid.UUID, now time.Time) ([]Command, error) {
	code := "COMMAND_EXPIRED"
	msg := "command expired while accepted or running"
	rows, err := r.pool.Query(ctx, `
		UPDATE agent_commands
		SET status = 'failed',
		    finished_at = $2,
		    error_code = $3,
		    error_message = $4
		WHERE server_id = $1
		  AND status IN ('accepted', 'running')
		  AND expires_at <= $2
		RETURNING id, organization_id, server_id, operation, schema_version, payload, status,
		          issued_at, expires_at, request_id, correlation_id, issued_by,
		          result, error_code, error_message, accepted_at, started_at, finished_at,
		          created_at, updated_at`, serverID, now, code, msg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Command
	for rows.Next() {
		cmd, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cmd)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpdateStatus(ctx context.Context, id, serverID uuid.UUID, fromStatuses []string, toStatus string, result map[string]any, errCode, errMsg *string, at time.Time) (Command, error) {
	var acceptedAt, startedAt, finishedAt *time.Time
	switch toStatus {
	case protocol.StatusAccepted:
		acceptedAt = &at
	case protocol.StatusRunning:
		startedAt = &at
	case protocol.StatusCompleted, protocol.StatusFailed:
		finishedAt = &at
	}

	const q = `
		UPDATE agent_commands
		SET status = $4,
		    result = COALESCE($5, result),
		    error_code = COALESCE($6, error_code),
		    error_message = COALESCE($7, error_message),
		    accepted_at = COALESCE($8, accepted_at),
		    started_at = COALESCE($9, started_at),
		    finished_at = COALESCE($10, finished_at)
		WHERE id = $1 AND server_id = $2 AND status = ANY($3)
		RETURNING id, organization_id, server_id, operation, schema_version, payload, status,
		          issued_at, expires_at, request_id, correlation_id, issued_by,
		          result, error_code, error_message, accepted_at, started_at, finished_at,
		          created_at, updated_at`

	var resultJSON any
	if result != nil {
		resultJSON = payloadJSON(result)
	}

	cmd, err := scanCommand(r.pool.QueryRow(ctx, q,
		id, serverID, fromStatuses, toStatus, resultJSON, errCode, errMsg, acceptedAt, startedAt, finishedAt,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Command{}, ErrConflict
	}
	return cmd, err
}

func (r *PostgresRepository) Cancel(ctx context.Context, id, orgID uuid.UUID, at time.Time) (Command, error) {
	const q = `
		UPDATE agent_commands
		SET status = 'cancelled', finished_at = $3
		WHERE id = $1 AND organization_id = $2 AND status IN ('pending', 'accepted')
		RETURNING id, organization_id, server_id, operation, schema_version, payload, status,
		          issued_at, expires_at, request_id, correlation_id, issued_by,
		          result, error_code, error_message, accepted_at, started_at, finished_at,
		          created_at, updated_at`
	cmd, err := scanCommand(r.pool.QueryRow(ctx, q, id, orgID, at))
	if errors.Is(err, pgx.ErrNoRows) {
		return Command{}, ErrConflict
	}
	return cmd, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanCommand(row scannable) (Command, error) {
	var c Command
	var payload, result []byte
	err := row.Scan(
		&c.ID, &c.OrganizationID, &c.ServerID, &c.Operation, &c.SchemaVersion, &payload, &c.Status,
		&c.IssuedAt, &c.ExpiresAt, &c.RequestID, &c.CorrelationID, &c.IssuedBy,
		&result, &c.ErrorCode, &c.ErrorMessage, &c.AcceptedAt, &c.StartedAt, &c.FinishedAt,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return Command{}, err
	}
	c.Payload = map[string]any{}
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &c.Payload)
	}
	if len(result) > 0 {
		c.Result = map[string]any{}
		_ = json.Unmarshal(result, &c.Result)
	}
	return c, nil
}

func isCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514"
}
