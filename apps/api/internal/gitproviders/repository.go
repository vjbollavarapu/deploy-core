package gitproviders

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
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type connectionSecrets struct {
	CredentialCiphertext []byte
	CredentialNonce      []byte
	CredentialKeyID      string
	WebhookCiphertext    []byte
	WebhookNonce         []byte
	WebhookKeyID         string
	WebhookSecretHash    *string
}

type autoDeployApp struct {
	ApplicationID  uuid.UUID
	OrganizationID uuid.UUID
	EnvironmentID  uuid.UUID
	ServerID       *uuid.UUID
	RepositoryURL  *string
	GitBranch      *string
}

type RepositoryStore interface {
	CreateConnection(ctx context.Context, c Connection, sec connectionSecrets, createdBy uuid.UUID) (Connection, error)
	GetConnection(ctx context.Context, id uuid.UUID) (Connection, connectionSecrets, error)
	ListConnections(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Connection, int64, error)
	UpdateConnection(ctx context.Context, id uuid.UUID, c Connection, sec *connectionSecrets) (Connection, error)
	SoftDeleteConnection(ctx context.Context, id uuid.UUID, at time.Time) error
	MarkSynced(ctx context.Context, id uuid.UUID, at time.Time) error

	UpsertRepository(ctx context.Context, orgID, connectionID uuid.UUID, in UpsertRepositoryInput, syncedAt time.Time) (Repository, error)
	ListRepositories(ctx context.Context, connectionID uuid.UUID, limit, offset int) ([]Repository, int64, error)

	InsertDelivery(ctx context.Context, d WebhookDelivery) (WebhookDelivery, error)
	UpdateDelivery(ctx context.Context, id uuid.UUID, status string, deploymentIDs []uuid.UUID, errMsg *string) error

	FindAutoDeployApps(ctx context.Context, orgID uuid.UUID, connectionID *uuid.UUID) ([]autoDeployApp, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateConnection(ctx context.Context, c Connection, sec connectionSecrets, createdBy uuid.UUID) (Connection, error) {
	const q = `
		INSERT INTO git_connections (
			organization_id, provider, account_login, display_name,
			credential_ciphertext, credential_nonce, credential_key_id, credential_algorithm,
			webhook_secret_ciphertext, webhook_secret_nonce, webhook_secret_key_id, webhook_secret_hash,
			status, metadata, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,'AES-256-GCM',$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, organization_id, provider, account_login, display_name, status, last_sync_at,
		          metadata, created_by, created_at, updated_at,
		          (webhook_secret_ciphertext IS NOT NULL)`
	meta, _ := json.Marshal(mapOrEmpty(c.Metadata))
	out, err := scanConnection(r.pool.QueryRow(ctx, q,
		c.OrganizationID, c.Provider, c.AccountLogin, c.DisplayName,
		sec.CredentialCiphertext, sec.CredentialNonce, sec.CredentialKeyID,
		nullBytes(sec.WebhookCiphertext), nullBytes(sec.WebhookNonce), nullStr(sec.WebhookKeyID), sec.WebhookSecretHash,
		c.Status, meta, createdBy,
	))
	return out, err
}

func (r *PostgresRepository) GetConnection(ctx context.Context, id uuid.UUID) (Connection, connectionSecrets, error) {
	const q = `
		SELECT id, organization_id, provider, account_login, display_name, status, last_sync_at,
		       metadata, created_by, created_at, updated_at,
		       (webhook_secret_ciphertext IS NOT NULL),
		       credential_ciphertext, credential_nonce, credential_key_id,
		       webhook_secret_ciphertext, webhook_secret_nonce, COALESCE(webhook_secret_key_id, ''),
		       webhook_secret_hash
		FROM git_connections
		WHERE id = $1 AND deleted_at IS NULL`
	var c Connection
	var meta []byte
	var sec connectionSecrets
	var whCT, whNonce []byte
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&c.ID, &c.OrganizationID, &c.Provider, &c.AccountLogin, &c.DisplayName, &c.Status, &c.LastSyncAt,
		&meta, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.HasWebhookSecret,
		&sec.CredentialCiphertext, &sec.CredentialNonce, &sec.CredentialKeyID,
		&whCT, &whNonce, &sec.WebhookKeyID, &sec.WebhookSecretHash,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, connectionSecrets{}, ErrNotFound
	}
	if err != nil {
		return Connection{}, connectionSecrets{}, err
	}
	c.Metadata = decodeMap(meta)
	sec.WebhookCiphertext = whCT
	sec.WebhookNonce = whNonce
	return c, sec, nil
}

func (r *PostgresRepository) ListConnections(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Connection, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM git_connections WHERE organization_id = $1 AND deleted_at IS NULL`, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, provider, account_login, display_name, status, last_sync_at,
		       metadata, created_by, created_at, updated_at,
		       (webhook_secret_ciphertext IS NOT NULL)
		FROM git_connections
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Connection
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) UpdateConnection(ctx context.Context, id uuid.UUID, c Connection, sec *connectionSecrets) (Connection, error) {
	meta, _ := json.Marshal(mapOrEmpty(c.Metadata))
	if sec == nil {
		const q = `
			UPDATE git_connections SET
				account_login = $2, display_name = $3, status = $4, metadata = $5
			WHERE id = $1 AND deleted_at IS NULL
			RETURNING id, organization_id, provider, account_login, display_name, status, last_sync_at,
			          metadata, created_by, created_at, updated_at,
			          (webhook_secret_ciphertext IS NOT NULL)`
		out, err := scanConnection(r.pool.QueryRow(ctx, q, id, c.AccountLogin, c.DisplayName, c.Status, meta))
		if errors.Is(err, pgx.ErrNoRows) {
			return Connection{}, ErrNotFound
		}
		return out, err
	}
	const q = `
		UPDATE git_connections SET
			account_login = $2, display_name = $3, status = $4, metadata = $5,
			credential_ciphertext = COALESCE($6, credential_ciphertext),
			credential_nonce = COALESCE($7, credential_nonce),
			credential_key_id = COALESCE($8, credential_key_id),
			webhook_secret_ciphertext = COALESCE($9, webhook_secret_ciphertext),
			webhook_secret_nonce = COALESCE($10, webhook_secret_nonce),
			webhook_secret_key_id = COALESCE($11, webhook_secret_key_id),
			webhook_secret_hash = COALESCE($12, webhook_secret_hash)
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, provider, account_login, display_name, status, last_sync_at,
		          metadata, created_by, created_at, updated_at,
		          (webhook_secret_ciphertext IS NOT NULL)`
	out, err := scanConnection(r.pool.QueryRow(ctx, q,
		id, c.AccountLogin, c.DisplayName, c.Status, meta,
		nullBytes(sec.CredentialCiphertext), nullBytes(sec.CredentialNonce), nullStrPtr(sec.CredentialKeyID),
		nullBytes(sec.WebhookCiphertext), nullBytes(sec.WebhookNonce), nullStrPtr(sec.WebhookKeyID), sec.WebhookSecretHash,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, ErrNotFound
	}
	return out, err
}

func (r *PostgresRepository) SoftDeleteConnection(ctx context.Context, id uuid.UUID, at time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE git_connections SET deleted_at = $2, status = 'revoked'
		WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkSynced(ctx context.Context, id uuid.UUID, at time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE git_connections SET last_sync_at = $2 WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) UpsertRepository(ctx context.Context, orgID, connectionID uuid.UUID, in UpsertRepositoryInput, syncedAt time.Time) (Repository, error) {
	meta, _ := json.Marshal(mapOrEmpty(in.Metadata))
	const q = `
		INSERT INTO git_repositories (
			organization_id, connection_id, external_id, full_name, default_branch,
			clone_url, html_url, metadata, last_sync_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (connection_id, full_name) DO UPDATE SET
			external_id = EXCLUDED.external_id,
			default_branch = EXCLUDED.default_branch,
			clone_url = EXCLUDED.clone_url,
			html_url = EXCLUDED.html_url,
			metadata = EXCLUDED.metadata,
			last_sync_at = EXCLUDED.last_sync_at
		RETURNING id, organization_id, connection_id, external_id, full_name, default_branch,
		          clone_url, html_url, metadata, last_sync_at, created_at, updated_at`
	return scanRepository(r.pool.QueryRow(ctx, q,
		orgID, connectionID, in.ExternalID, in.FullName, in.DefaultBranch,
		in.CloneURL, in.HTMLURL, meta, syncedAt,
	))
}

func (r *PostgresRepository) ListRepositories(ctx context.Context, connectionID uuid.UUID, limit, offset int) ([]Repository, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM git_repositories WHERE connection_id = $1`, connectionID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, connection_id, external_id, full_name, default_branch,
		       clone_url, html_url, metadata, last_sync_at, created_at, updated_at
		FROM git_repositories
		WHERE connection_id = $1
		ORDER BY full_name ASC
		LIMIT $2 OFFSET $3`, connectionID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Repository
	for rows.Next() {
		repo, err := scanRepository(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, repo)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) InsertDelivery(ctx context.Context, d WebhookDelivery) (WebhookDelivery, error) {
	ids, _ := json.Marshal(d.DeploymentIDs)
	const q = `
		INSERT INTO git_webhook_deliveries (
			organization_id, connection_id, provider, delivery_id, event_type,
			repository_full_name, branch, commit_sha, status, deployment_ids, error_message
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, organization_id, connection_id, provider, delivery_id, event_type,
		          repository_full_name, branch, commit_sha, status, deployment_ids, error_message, created_at`
	out, err := scanDelivery(r.pool.QueryRow(ctx, q,
		d.OrganizationID, d.ConnectionID, d.Provider, d.DeliveryID, d.EventType,
		d.RepositoryFullName, d.Branch, d.CommitSHA, d.Status, ids, d.ErrorMessage,
	))
	if isUniqueViolation(err) {
		return WebhookDelivery{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) UpdateDelivery(ctx context.Context, id uuid.UUID, status string, deploymentIDs []uuid.UUID, errMsg *string) error {
	ids, _ := json.Marshal(deploymentIDs)
	_, err := r.pool.Exec(ctx, `
		UPDATE git_webhook_deliveries
		SET status = $2, deployment_ids = $3, error_message = $4
		WHERE id = $1`, id, status, ids, errMsg)
	return err
}

func (r *PostgresRepository) FindAutoDeployApps(ctx context.Context, orgID uuid.UUID, connectionID *uuid.UUID) ([]autoDeployApp, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (a.id)
			a.id, a.organization_id, a.environment_id, a.target_server_id,
			c.repository_url, c.git_branch
		FROM applications a
		JOIN application_configs c ON c.application_id = a.id
		WHERE a.organization_id = $1
		  AND a.deleted_at IS NULL
		  AND c.auto_deploy_enabled = TRUE
		  AND c.source_type = 'git'
		  AND ($2::uuid IS NULL OR c.git_connection_id IS NULL OR c.git_connection_id = $2)
		ORDER BY a.id, c.version DESC`, orgID, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []autoDeployApp
	for rows.Next() {
		var a autoDeployApp
		if err := rows.Scan(&a.ApplicationID, &a.OrganizationID, &a.EnvironmentID, &a.ServerID, &a.RepositoryURL, &a.GitBranch); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanConnection(row scannable) (Connection, error) {
	var c Connection
	var meta []byte
	err := row.Scan(
		&c.ID, &c.OrganizationID, &c.Provider, &c.AccountLogin, &c.DisplayName, &c.Status, &c.LastSyncAt,
		&meta, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.HasWebhookSecret,
	)
	if err != nil {
		return Connection{}, err
	}
	c.Metadata = decodeMap(meta)
	return c, nil
}

func scanRepository(row scannable) (Repository, error) {
	var repo Repository
	var meta []byte
	err := row.Scan(
		&repo.ID, &repo.OrganizationID, &repo.ConnectionID, &repo.ExternalID, &repo.FullName, &repo.DefaultBranch,
		&repo.CloneURL, &repo.HTMLURL, &meta, &repo.LastSyncAt, &repo.CreatedAt, &repo.UpdatedAt,
	)
	if err != nil {
		return Repository{}, err
	}
	repo.Metadata = decodeMap(meta)
	return repo, nil
}

func scanDelivery(row scannable) (WebhookDelivery, error) {
	var d WebhookDelivery
	var ids []byte
	err := row.Scan(
		&d.ID, &d.OrganizationID, &d.ConnectionID, &d.Provider, &d.DeliveryID, &d.EventType,
		&d.RepositoryFullName, &d.Branch, &d.CommitSHA, &d.Status, &ids, &d.ErrorMessage, &d.CreatedAt,
	)
	if err != nil {
		return WebhookDelivery{}, err
	}
	_ = json.Unmarshal(ids, &d.DeploymentIDs)
	if d.DeploymentIDs == nil {
		d.DeploymentIDs = []uuid.UUID{}
	}
	return d, nil
}

func decodeMap(b []byte) map[string]any {
	m := map[string]any{}
	if len(b) > 0 {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func mapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func nullBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullStrPtr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
