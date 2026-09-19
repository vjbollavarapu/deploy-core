package webhooks

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
	ErrNotFound = errors.New("webhooks: not found")
	ErrConflict = errors.New("webhooks: conflict")
)

type Repository interface {
	Create(ctx context.Context, wh Webhook, secret secretBlob, createdBy uuid.UUID) (Webhook, error)
	Get(ctx context.Context, id uuid.UUID) (Webhook, *secretBlob, error)
	List(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Webhook, int64, error)
	Update(ctx context.Context, wh Webhook, secret *secretBlob) (Webhook, error)
	SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error
	ListEnabledForEvent(ctx context.Context, orgID uuid.UUID, eventType string) ([]Webhook, error)
	RecordSuccess(ctx context.Context, id uuid.UUID) error
	RecordFailure(ctx context.Context, id uuid.UUID, lastError string, disable bool, at time.Time) error
	MarkFailing(ctx context.Context, id uuid.UUID, lastError string) error

	CreateDelivery(ctx context.Context, d Delivery) (Delivery, error)
	GetDelivery(ctx context.Context, id uuid.UUID) (Delivery, error)
	ListDeliveries(ctx context.Context, orgID uuid.UUID, webhookID *uuid.UUID, limit, offset int) ([]Delivery, int64, error)
	UpdateDelivery(ctx context.Context, d Delivery) (Delivery, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, wh Webhook, secret secretBlob, createdBy uuid.UUID) (Webhook, error) {
	events, _ := json.Marshal(wh.Events)
	const q = `
		INSERT INTO outgoing_webhooks (
			organization_id, name, url, events,
			secret_ciphertext, secret_nonce, secret_key_id, secret_algorithm,
			enabled, status, consecutive_failures, failure_threshold, last_error, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,'AES-256-GCM',$8,$9,$10,$11,$12,$13)
		RETURNING id, organization_id, name, url, events, enabled, status,
			consecutive_failures, failure_threshold, last_error, disabled_at,
			created_by, created_at, updated_at, deleted_at`
	out, err := scanWebhook(r.pool.QueryRow(ctx, q,
		wh.OrganizationID, wh.Name, wh.URL, events,
		secret.Ciphertext, secret.Nonce, secret.KeyID,
		wh.Enabled, wh.Status, wh.ConsecutiveFailures, wh.FailureThreshold, wh.LastError, createdBy,
	))
	if isUniqueViolation(err) {
		return Webhook{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Webhook, *secretBlob, error) {
	const q = `
		SELECT id, organization_id, name, url, events, enabled, status,
			consecutive_failures, failure_threshold, last_error, disabled_at,
			created_by, created_at, updated_at, deleted_at,
			secret_ciphertext, secret_nonce, secret_key_id
		FROM outgoing_webhooks WHERE id = $1 AND deleted_at IS NULL`
	var wh Webhook
	var events []byte
	var secret secretBlob
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&wh.ID, &wh.OrganizationID, &wh.Name, &wh.URL, &events, &wh.Enabled, &wh.Status,
		&wh.ConsecutiveFailures, &wh.FailureThreshold, &wh.LastError, &wh.DisabledAt,
		&wh.CreatedBy, &wh.CreatedAt, &wh.UpdatedAt, &wh.DeletedAt,
		&secret.Ciphertext, &secret.Nonce, &secret.KeyID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Webhook{}, nil, ErrNotFound
	}
	if err != nil {
		return Webhook{}, nil, err
	}
	_ = json.Unmarshal(events, &wh.Events)
	if wh.Events == nil {
		wh.Events = []string{}
	}
	return wh, &secret, nil
}

func (r *PostgresRepository) List(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Webhook, int64, error) {
	limit, offset = pageBounds(limit, offset)
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM outgoing_webhooks
		WHERE organization_id = $1 AND deleted_at IS NULL`, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, url, events, enabled, status,
			consecutive_failures, failure_threshold, last_error, disabled_at,
			created_by, created_at, updated_at, deleted_at
		FROM outgoing_webhooks
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		wh, err := scanWebhook(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, wh)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, wh Webhook, secret *secretBlob) (Webhook, error) {
	events, _ := json.Marshal(wh.Events)
	var ct, nonce []byte
	var keyID *string
	setSecret := secret != nil
	if secret != nil {
		ct, nonce = secret.Ciphertext, secret.Nonce
		keyID = &secret.KeyID
	}
	const q = `
		UPDATE outgoing_webhooks SET
			name = $2, url = $3, events = $4, enabled = $5, status = $6,
			consecutive_failures = $7, failure_threshold = $8, last_error = $9, disabled_at = $10,
			secret_ciphertext = CASE WHEN $11 THEN $12 ELSE secret_ciphertext END,
			secret_nonce = CASE WHEN $11 THEN $13 ELSE secret_nonce END,
			secret_key_id = CASE WHEN $11 THEN $14 ELSE secret_key_id END,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, name, url, events, enabled, status,
			consecutive_failures, failure_threshold, last_error, disabled_at,
			created_by, created_at, updated_at, deleted_at`
	out, err := scanWebhook(r.pool.QueryRow(ctx, q,
		wh.ID, wh.Name, wh.URL, events, wh.Enabled, wh.Status,
		wh.ConsecutiveFailures, wh.FailureThreshold, wh.LastError, wh.DisabledAt,
		setSecret, ct, nonce, keyID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Webhook{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Webhook{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE outgoing_webhooks SET deleted_at = $2, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListEnabledForEvent(ctx context.Context, orgID uuid.UUID, eventType string) ([]Webhook, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, url, events, enabled, status,
			consecutive_failures, failure_threshold, last_error, disabled_at,
			created_by, created_at, updated_at, deleted_at
		FROM outgoing_webhooks
		WHERE organization_id = $1 AND deleted_at IS NULL AND enabled = TRUE
		  AND status <> 'DISABLED'
		  AND events @> jsonb_build_array($2::text)`, orgID, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		wh, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, wh)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) RecordSuccess(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE outgoing_webhooks SET
			consecutive_failures = 0, last_error = '', status = 'ACTIVE',
			disabled_at = NULL, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id)
	return err
}

func (r *PostgresRepository) RecordFailure(ctx context.Context, id uuid.UUID, lastError string, disable bool, at time.Time) error {
	if disable {
		_, err := r.pool.Exec(ctx, `
			UPDATE outgoing_webhooks SET
				consecutive_failures = consecutive_failures + 1,
				last_error = $2, status = 'DISABLED', enabled = FALSE,
				disabled_at = $3, updated_at = NOW()
			WHERE id = $1 AND deleted_at IS NULL`, id, lastError, at)
		return err
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE outgoing_webhooks SET
			consecutive_failures = consecutive_failures + 1,
			last_error = $2, status = 'FAILING', updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id, lastError)
	return err
}

func (r *PostgresRepository) MarkFailing(ctx context.Context, id uuid.UUID, lastError string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE outgoing_webhooks SET
			last_error = $2, status = 'FAILING', updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL AND status = 'ACTIVE'`, id, lastError)
	return err
}

func (r *PostgresRepository) CreateDelivery(ctx context.Context, d Delivery) (Delivery, error) {
	payload, _ := json.Marshal(mapOrEmpty(d.Payload))
	const q = `
		INSERT INTO outgoing_webhook_deliveries (
			organization_id, webhook_id, event_type, payload, status,
			attempt_count, job_id, last_error
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, organization_id, webhook_id, event_type, payload, status,
			attempt_count, job_id, response_code, latency_ms, last_error, delivered_at,
			created_at, updated_at`
	return scanDelivery(r.pool.QueryRow(ctx, q,
		d.OrganizationID, d.WebhookID, d.EventType, payload, d.Status,
		d.AttemptCount, d.JobID, d.LastError,
	))
}

func (r *PostgresRepository) GetDelivery(ctx context.Context, id uuid.UUID) (Delivery, error) {
	const q = `
		SELECT id, organization_id, webhook_id, event_type, payload, status,
			attempt_count, job_id, response_code, latency_ms, last_error, delivered_at,
			created_at, updated_at
		FROM outgoing_webhook_deliveries WHERE id = $1`
	out, err := scanDelivery(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, ErrNotFound
	}
	return out, err
}

func (r *PostgresRepository) ListDeliveries(ctx context.Context, orgID uuid.UUID, webhookID *uuid.UUID, limit, offset int) ([]Delivery, int64, error) {
	limit, offset = pageBounds(limit, offset)
	var total int64
	var err error
	if webhookID != nil {
		err = r.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM outgoing_webhook_deliveries
			WHERE organization_id = $1 AND webhook_id = $2`, orgID, *webhookID).Scan(&total)
	} else {
		err = r.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM outgoing_webhook_deliveries WHERE organization_id = $1`, orgID).Scan(&total)
	}
	if err != nil {
		return nil, 0, err
	}
	var rows pgx.Rows
	if webhookID != nil {
		rows, err = r.pool.Query(ctx, `
			SELECT id, organization_id, webhook_id, event_type, payload, status,
				attempt_count, job_id, response_code, latency_ms, last_error, delivered_at,
				created_at, updated_at
			FROM outgoing_webhook_deliveries
			WHERE organization_id = $1 AND webhook_id = $2
			ORDER BY created_at DESC LIMIT $3 OFFSET $4`, orgID, *webhookID, limit, offset)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT id, organization_id, webhook_id, event_type, payload, status,
				attempt_count, job_id, response_code, latency_ms, last_error, delivered_at,
				created_at, updated_at
			FROM outgoing_webhook_deliveries
			WHERE organization_id = $1
			ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, limit, offset)
	}
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
		UPDATE outgoing_webhook_deliveries SET
			status = $2, attempt_count = $3, job_id = $4, response_code = $5,
			latency_ms = $6, last_error = $7, delivered_at = $8, payload = $9,
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, organization_id, webhook_id, event_type, payload, status,
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

func scanWebhook(row scannable) (Webhook, error) {
	var wh Webhook
	var events []byte
	err := row.Scan(
		&wh.ID, &wh.OrganizationID, &wh.Name, &wh.URL, &events, &wh.Enabled, &wh.Status,
		&wh.ConsecutiveFailures, &wh.FailureThreshold, &wh.LastError, &wh.DisabledAt,
		&wh.CreatedBy, &wh.CreatedAt, &wh.UpdatedAt, &wh.DeletedAt,
	)
	if err != nil {
		return Webhook{}, err
	}
	_ = json.Unmarshal(events, &wh.Events)
	if wh.Events == nil {
		wh.Events = []string{}
	}
	return wh, nil
}

func scanDelivery(row scannable) (Delivery, error) {
	var d Delivery
	var payload []byte
	err := row.Scan(
		&d.ID, &d.OrganizationID, &d.WebhookID, &d.EventType, &payload, &d.Status,
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

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
