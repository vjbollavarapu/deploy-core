package audit

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("audit: not found")

// Record is a persisted audit_logs row (immutable after insert).
type Record struct {
	ID             uuid.UUID
	OrganizationID *uuid.UUID
	ActorUserID    *uuid.UUID
	ActorType      string
	Action         string
	ResourceType   string
	ResourceID     *string
	RequestID      *string
	IPAddress      *string
	UserAgent      *string
	Before         map[string]any
	After          map[string]any
	CreatedAt      time.Time
}

// ListFilter scopes audit queries. OrganizationID is required for org reads.
type ListFilter struct {
	OrganizationID uuid.UUID
	Action         string
	ResourceType   string
	ResourceID     string
	ActorUserID    *uuid.UUID
	Limit          int
	Offset         int
}

type Repository interface {
	List(ctx context.Context, f ListFilter) ([]Record, int64, error)
	Get(ctx context.Context, id uuid.UUID) (Record, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) List(ctx context.Context, f ListFilter) ([]Record, int64, error) {
	limit, offset := pageBounds(f.Limit, f.Offset)
	args := []any{f.OrganizationID}
	where := `organization_id = $1`
	n := 2
	if f.Action != "" {
		where += ` AND action = $` + strconv.Itoa(n)
		args = append(args, f.Action)
		n++
	}
	if f.ResourceType != "" {
		where += ` AND resource_type = $` + strconv.Itoa(n)
		args = append(args, f.ResourceType)
		n++
	}
	if f.ResourceID != "" {
		where += ` AND resource_id = $` + strconv.Itoa(n)
		args = append(args, f.ResourceID)
		n++
	}
	if f.ActorUserID != nil {
		where += ` AND actor_user_id = $` + strconv.Itoa(n)
		args = append(args, *f.ActorUserID)
		n++
	}

	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := `
		SELECT id, organization_id, actor_user_id, actor_type, action, resource_type, resource_id,
			request_id, ip_address::text, user_agent, before_metadata, after_metadata, created_at
		FROM audit_logs WHERE ` + where + `
		ORDER BY created_at DESC
		LIMIT $` + strconv.Itoa(n) + ` OFFSET $` + strconv.Itoa(n+1)
	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, rec)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Record, error) {
	const q = `
		SELECT id, organization_id, actor_user_id, actor_type, action, resource_type, resource_id,
			request_id, ip_address::text, user_agent, before_metadata, after_metadata, created_at
		FROM audit_logs WHERE id = $1`
	out, err := scanRecord(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	return out, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRecord(row scannable) (Record, error) {
	var rec Record
	var before, after []byte
	var ip *string
	err := row.Scan(
		&rec.ID, &rec.OrganizationID, &rec.ActorUserID, &rec.ActorType, &rec.Action, &rec.ResourceType, &rec.ResourceID,
		&rec.RequestID, &ip, &rec.UserAgent, &before, &after, &rec.CreatedAt,
	)
	if err != nil {
		return Record{}, err
	}
	rec.IPAddress = ip
	if len(before) > 0 {
		_ = json.Unmarshal(before, &rec.Before)
	}
	if len(after) > 0 {
		_ = json.Unmarshal(after, &rec.After)
	}
	return rec, nil
}

func pageBounds(limit, offset int) (int, int) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
