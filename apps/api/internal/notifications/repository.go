package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("notifications: not found")
	ErrConflict = errors.New("notifications: conflict")
)

type Repository interface {
	CreateChannel(ctx context.Context, ch Channel, cred *credentialBlob, createdBy uuid.UUID) (Channel, error)
	GetChannel(ctx context.Context, id uuid.UUID) (Channel, *credentialBlob, error)
	ListChannels(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Channel, int64, error)
	UpdateChannel(ctx context.Context, ch Channel, cred *credentialBlob, clearCred bool) (Channel, error)
	SoftDeleteChannel(ctx context.Context, id uuid.UUID, at time.Time) error

	CreatePolicy(ctx context.Context, p Policy, createdBy uuid.UUID) (Policy, error)
	GetPolicy(ctx context.Context, id uuid.UUID) (Policy, error)
	ListPolicies(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Policy, int64, error)
	UpdatePolicy(ctx context.Context, p Policy) (Policy, error)
	SoftDeletePolicy(ctx context.Context, id uuid.UUID, at time.Time) error
	ListEnabledPoliciesForEvent(ctx context.Context, orgID uuid.UUID, eventType string) ([]Policy, error)

	CreateDelivery(ctx context.Context, d Delivery) (Delivery, error)
	GetDelivery(ctx context.Context, id uuid.UUID) (Delivery, error)
	ListDeliveries(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Delivery, int64, error)
	UpdateDelivery(ctx context.Context, d Delivery) (Delivery, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateChannel(ctx context.Context, ch Channel, cred *credentialBlob, createdBy uuid.UUID) (Channel, error) {
	cfg, _ := json.Marshal(mapOrEmpty(ch.Config))
	var ct, nonce []byte
	var keyID *string
	if cred != nil {
		ct, nonce = cred.Ciphertext, cred.Nonce
		keyID = &cred.KeyID
	}
	const q = `
		INSERT INTO notification_channels (
			organization_id, name, type, config,
			credential_ciphertext, credential_nonce, credential_key_id, credential_algorithm,
			enabled, status, last_error, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,'AES-256-GCM',$8,$9,$10,$11)
		RETURNING id, organization_id, name, type, config, enabled, status, last_error,
			created_by, created_at, updated_at, deleted_at,
			(credential_ciphertext IS NOT NULL)`
	out, err := scanChannel(r.pool.QueryRow(ctx, q,
		ch.OrganizationID, ch.Name, ch.Type, cfg, ct, nonce, keyID,
		ch.Enabled, ch.Status, ch.LastError, createdBy,
	))
	if isUniqueViolation(err) {
		return Channel{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) GetChannel(ctx context.Context, id uuid.UUID) (Channel, *credentialBlob, error) {
	const q = `
		SELECT id, organization_id, name, type, config, enabled, status, last_error,
			created_by, created_at, updated_at, deleted_at,
			(credential_ciphertext IS NOT NULL),
			credential_ciphertext, credential_nonce, COALESCE(credential_key_id, '')
		FROM notification_channels WHERE id = $1 AND deleted_at IS NULL`
	var ch Channel
	var cfg []byte
	var ct, nonce []byte
	var keyID string
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&ch.ID, &ch.OrganizationID, &ch.Name, &ch.Type, &cfg, &ch.Enabled, &ch.Status, &ch.LastError,
		&ch.CreatedBy, &ch.CreatedAt, &ch.UpdatedAt, &ch.DeletedAt, &ch.HasCredential,
		&ct, &nonce, &keyID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Channel{}, nil, ErrNotFound
	}
	if err != nil {
		return Channel{}, nil, err
	}
	_ = json.Unmarshal(cfg, &ch.Config)
	if ch.Config == nil {
		ch.Config = map[string]any{}
	}
	var cred *credentialBlob
	if len(ct) > 0 {
		cred = &credentialBlob{Ciphertext: ct, Nonce: nonce, KeyID: keyID}
	}
	return ch, cred, nil
}

func (r *PostgresRepository) ListChannels(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Channel, int64, error) {
	limit, offset = pageBounds(limit, offset)
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM notification_channels
		WHERE organization_id = $1 AND deleted_at IS NULL`, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, type, config, enabled, status, last_error,
			created_by, created_at, updated_at, deleted_at,
			(credential_ciphertext IS NOT NULL)
		FROM notification_channels
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Channel
	for rows.Next() {
		ch, err := scanChannel(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, ch)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) UpdateChannel(ctx context.Context, ch Channel, cred *credentialBlob, clearCred bool) (Channel, error) {
	cfg, _ := json.Marshal(mapOrEmpty(ch.Config))
	var ct, nonce []byte
	var keyID *string
	setCred := cred != nil
	if cred != nil {
		ct, nonce = cred.Ciphertext, cred.Nonce
		keyID = &cred.KeyID
	}
	const q = `
		UPDATE notification_channels SET
			name = $2, config = $3, enabled = $4, status = $5, last_error = $6,
			credential_ciphertext = CASE
				WHEN $7 THEN NULL
				WHEN $8 THEN $9
				ELSE credential_ciphertext END,
			credential_nonce = CASE
				WHEN $7 THEN NULL
				WHEN $8 THEN $10
				ELSE credential_nonce END,
			credential_key_id = CASE
				WHEN $7 THEN NULL
				WHEN $8 THEN $11
				ELSE credential_key_id END,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, name, type, config, enabled, status, last_error,
			created_by, created_at, updated_at, deleted_at,
			(credential_ciphertext IS NOT NULL)`
	out, err := scanChannel(r.pool.QueryRow(ctx, q,
		ch.ID, ch.Name, cfg, ch.Enabled, ch.Status, ch.LastError,
		clearCred, setCred, ct, nonce, keyID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Channel{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Channel{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) SoftDeleteChannel(ctx context.Context, id uuid.UUID, at time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE notification_channels SET deleted_at = $2, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CreatePolicy(ctx context.Context, p Policy, createdBy uuid.UUID) (Policy, error) {
	events, _ := json.Marshal(p.EventTypes)
	resF, _ := json.Marshal(mapOrEmpty(p.ResourceFilters))
	envF, _ := json.Marshal(mapOrEmpty(p.EnvironmentFilters))
	chs, _ := json.Marshal(uuidStrings(p.ChannelIDs))
	const q = `
		INSERT INTO notification_policies (
			organization_id, name, event_types, resource_filters, environment_filters,
			channel_ids, enabled, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, organization_id, name, event_types, resource_filters, environment_filters,
			channel_ids, enabled, created_by, created_at, updated_at, deleted_at`
	out, err := scanPolicy(r.pool.QueryRow(ctx, q,
		p.OrganizationID, p.Name, events, resF, envF, chs, p.Enabled, createdBy,
	))
	if isUniqueViolation(err) {
		return Policy{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) GetPolicy(ctx context.Context, id uuid.UUID) (Policy, error) {
	const q = `
		SELECT id, organization_id, name, event_types, resource_filters, environment_filters,
			channel_ids, enabled, created_by, created_at, updated_at, deleted_at
		FROM notification_policies WHERE id = $1 AND deleted_at IS NULL`
	out, err := scanPolicy(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, ErrNotFound
	}
	return out, err
}

func (r *PostgresRepository) ListPolicies(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Policy, int64, error) {
	limit, offset = pageBounds(limit, offset)
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM notification_policies
		WHERE organization_id = $1 AND deleted_at IS NULL`, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, event_types, resource_filters, environment_filters,
			channel_ids, enabled, created_by, created_at, updated_at, deleted_at
		FROM notification_policies
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Policy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) UpdatePolicy(ctx context.Context, p Policy) (Policy, error) {
	events, _ := json.Marshal(p.EventTypes)
	resF, _ := json.Marshal(mapOrEmpty(p.ResourceFilters))
	envF, _ := json.Marshal(mapOrEmpty(p.EnvironmentFilters))
	chs, _ := json.Marshal(uuidStrings(p.ChannelIDs))
	const q = `
		UPDATE notification_policies SET
			name = $2, event_types = $3, resource_filters = $4, environment_filters = $5,
			channel_ids = $6, enabled = $7, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, name, event_types, resource_filters, environment_filters,
			channel_ids, enabled, created_by, created_at, updated_at, deleted_at`
	out, err := scanPolicy(r.pool.QueryRow(ctx, q,
		p.ID, p.Name, events, resF, envF, chs, p.Enabled,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Policy{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) SoftDeletePolicy(ctx context.Context, id uuid.UUID, at time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE notification_policies SET deleted_at = $2, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListEnabledPoliciesForEvent(ctx context.Context, orgID uuid.UUID, eventType string) ([]Policy, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, event_types, resource_filters, environment_filters,
			channel_ids, enabled, created_by, created_at, updated_at, deleted_at
		FROM notification_policies
		WHERE organization_id = $1 AND deleted_at IS NULL AND enabled = TRUE
		  AND event_types @> jsonb_build_array($2::text)`, orgID, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Policy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CreateDelivery(ctx context.Context, d Delivery) (Delivery, error) {
	payload, _ := json.Marshal(mapOrEmpty(d.Payload))
	const q = `
		INSERT INTO notification_deliveries (
			organization_id, policy_id, channel_id, event_type, payload, status,
			attempt_count, job_id, last_error
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, organization_id, policy_id, channel_id, event_type, payload, status,
			attempt_count, job_id, response_code, latency_ms, last_error, delivered_at,
			created_at, updated_at`
	return scanDelivery(r.pool.QueryRow(ctx, q,
		d.OrganizationID, d.PolicyID, d.ChannelID, d.EventType, payload, d.Status,
		d.AttemptCount, d.JobID, d.LastError,
	))
}

func (r *PostgresRepository) GetDelivery(ctx context.Context, id uuid.UUID) (Delivery, error) {
	const q = `
		SELECT id, organization_id, policy_id, channel_id, event_type, payload, status,
			attempt_count, job_id, response_code, latency_ms, last_error, delivered_at,
			created_at, updated_at
		FROM notification_deliveries WHERE id = $1`
	out, err := scanDelivery(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, ErrNotFound
	}
	return out, err
}

func (r *PostgresRepository) ListDeliveries(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Delivery, int64, error) {
	limit, offset = pageBounds(limit, offset)
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM notification_deliveries WHERE organization_id = $1`, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, policy_id, channel_id, event_type, payload, status,
			attempt_count, job_id, response_code, latency_ms, last_error, delivered_at,
			created_at, updated_at
		FROM notification_deliveries
		WHERE organization_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Delivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) UpdateDelivery(ctx context.Context, d Delivery) (Delivery, error) {
	payload, _ := json.Marshal(mapOrEmpty(d.Payload))
	const q = `
		UPDATE notification_deliveries SET
			status = $2, attempt_count = $3, job_id = $4, response_code = $5,
			latency_ms = $6, last_error = $7, delivered_at = $8, payload = $9,
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, organization_id, policy_id, channel_id, event_type, payload, status,
			attempt_count, job_id, response_code, latency_ms, last_error, delivered_at,
			created_at, updated_at`
	out, err := scanDelivery(r.pool.QueryRow(ctx, q,
		d.ID, d.Status, d.AttemptCount, d.JobID, d.ResponseCode,
		d.LatencyMs, d.LastError, d.DeliveredAt, payload,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, ErrNotFound
	}
	return out, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanChannel(row scannable) (Channel, error) {
	var ch Channel
	var cfg []byte
	err := row.Scan(
		&ch.ID, &ch.OrganizationID, &ch.Name, &ch.Type, &cfg, &ch.Enabled, &ch.Status, &ch.LastError,
		&ch.CreatedBy, &ch.CreatedAt, &ch.UpdatedAt, &ch.DeletedAt, &ch.HasCredential,
	)
	if err != nil {
		return Channel{}, err
	}
	_ = json.Unmarshal(cfg, &ch.Config)
	if ch.Config == nil {
		ch.Config = map[string]any{}
	}
	return ch, nil
}

func scanPolicy(row scannable) (Policy, error) {
	var p Policy
	var events, resF, envF, chs []byte
	err := row.Scan(
		&p.ID, &p.OrganizationID, &p.Name, &events, &resF, &envF, &chs,
		&p.Enabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)
	if err != nil {
		return Policy{}, err
	}
	_ = json.Unmarshal(events, &p.EventTypes)
	_ = json.Unmarshal(resF, &p.ResourceFilters)
	_ = json.Unmarshal(envF, &p.EnvironmentFilters)
	var ids []string
	_ = json.Unmarshal(chs, &ids)
	for _, s := range ids {
		if id, err := uuid.Parse(s); err == nil {
			p.ChannelIDs = append(p.ChannelIDs, id)
		}
	}
	if p.EventTypes == nil {
		p.EventTypes = []string{}
	}
	if p.ResourceFilters == nil {
		p.ResourceFilters = map[string]any{}
	}
	if p.EnvironmentFilters == nil {
		p.EnvironmentFilters = map[string]any{}
	}
	return p, nil
}

func scanDelivery(row scannable) (Delivery, error) {
	var d Delivery
	var payload []byte
	err := row.Scan(
		&d.ID, &d.OrganizationID, &d.PolicyID, &d.ChannelID, &d.EventType, &payload, &d.Status,
		&d.AttemptCount, &d.JobID, &d.ResponseCode, &d.LatencyMs, &d.LastError, &d.DeliveredAt,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return Delivery{}, err
	}
	_ = json.Unmarshal(payload, &d.Payload)
	if d.Payload == nil {
		d.Payload = map[string]any{}
	}
	return d, nil
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

func mapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
