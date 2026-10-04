package gitproviders

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type installState struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	UserID         uuid.UUID
	ExpiresAt      time.Time
	ConsumedAt     *time.Time
}

func (r *PostgresRepository) InsertInstallState(ctx context.Context, stateHash string, orgID, userID uuid.UUID, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO github_app_install_states (state_hash, organization_id, user_id, expires_at)
		VALUES ($1, $2, $3, $4)`, stateHash, orgID, userID, expiresAt)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (r *PostgresRepository) GetInstallState(ctx context.Context, stateHash string) (installState, error) {
	var st installState
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, user_id, expires_at, consumed_at
		FROM github_app_install_states
		WHERE state_hash = $1`, stateHash).Scan(&st.ID, &st.OrganizationID, &st.UserID, &st.ExpiresAt, &st.ConsumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return installState{}, ErrInstallStateNotFound
	}
	return st, err
}

func (r *PostgresRepository) ConsumeInstallState(ctx context.Context, stateHash string, userID, orgID uuid.UUID, now time.Time) (bool, error) {
	ct, err := r.pool.Exec(ctx, `
		UPDATE github_app_install_states
		SET consumed_at = $5
		WHERE state_hash = $1
		  AND user_id = $2
		  AND organization_id = $3
		  AND consumed_at IS NULL
		  AND expires_at > $4`, stateHash, userID, orgID, now, now)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() == 1, nil
}

func (r *PostgresRepository) FindLiveConnectionByInstallation(ctx context.Context, installationID int64) (Connection, error) {
	const q = `
		SELECT id, organization_id, provider, account_login, display_name, status, last_sync_at,
		       metadata, created_by, created_at, updated_at,
		       (webhook_secret_ciphertext IS NOT NULL),
		       auth_mode, installation_id, account_id, account_type, repository_selection
		FROM git_connections
		WHERE installation_id = $1 AND deleted_at IS NULL`
	out, err := scanConnection(r.pool.QueryRow(ctx, q, installationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, ErrNotFound
	}
	return out, err
}

func (r *PostgresRepository) CreateGitHubAppConnection(ctx context.Context, c Connection, createdBy uuid.UUID) (Connection, error) {
	meta, _ := json.Marshal(mapOrEmpty(c.Metadata))
	const q = `
		INSERT INTO git_connections (
			organization_id, provider, auth_mode, installation_id,
			account_id, account_type, repository_selection,
			account_login, display_name, status, metadata, created_by,
			credential_ciphertext, credential_nonce, credential_key_id
		) VALUES ($1,'github','github_app',$2,$3,$4,$5,$6,$7,$8,$9,$10,NULL,NULL,NULL)
		RETURNING id, organization_id, provider, account_login, display_name, status, last_sync_at,
		          metadata, created_by, created_at, updated_at,
		          (webhook_secret_ciphertext IS NOT NULL),
		          auth_mode, installation_id, account_id, account_type, repository_selection`
	out, err := scanConnection(r.pool.QueryRow(ctx, q,
		c.OrganizationID, c.InstallationID, c.AccountID, c.AccountType, c.RepositorySelection,
		c.AccountLogin, c.DisplayName, c.Status, meta, createdBy,
	))
	if isUniqueViolation(err) {
		return Connection{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) UpdateGitHubAppConnection(ctx context.Context, c Connection) (Connection, error) {
	meta, _ := json.Marshal(mapOrEmpty(c.Metadata))
	const q = `
		UPDATE git_connections
		SET account_login = $2,
		    display_name = $3,
		    account_id = $4,
		    account_type = $5,
		    repository_selection = $6,
		    status = $7,
		    metadata = $8
		WHERE id = $1 AND deleted_at IS NULL AND auth_mode = 'github_app'
		RETURNING id, organization_id, provider, account_login, display_name, status, last_sync_at,
		          metadata, created_by, created_at, updated_at,
		          (webhook_secret_ciphertext IS NOT NULL),
		          auth_mode, installation_id, account_id, account_type, repository_selection`
	out, err := scanConnection(r.pool.QueryRow(ctx, q,
		c.ID, c.AccountLogin, c.DisplayName, c.AccountID, c.AccountType, c.RepositorySelection, c.Status, meta,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, ErrNotFound
	}
	return out, err
}

func (r *PostgresRepository) SetConnectionStatus(ctx context.Context, id uuid.UUID, status string) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE git_connections
		SET status = $2
		WHERE id = $1 AND deleted_at IS NULL`, id, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ReconcileInstallationRepositories(ctx context.Context, orgID, connectionID uuid.UUID, repos []UpsertRepositoryInput, status string, syncedAt time.Time) ([]Repository, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	ids := make([]string, 0, len(repos))
	for _, repo := range repos {
		ids = append(ids, repo.ExternalID)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM git_repositories
		WHERE connection_id = $1
		  AND (external_id = '' OR NOT (external_id = ANY($2::text[])))`, connectionID, ids); err != nil {
		return nil, err
	}
	for _, in := range repos {
		meta, _ := json.Marshal(mapOrEmpty(in.Metadata))
		if _, err := tx.Exec(ctx, `
			INSERT INTO git_repositories (
				organization_id, connection_id, external_id, full_name, default_branch,
				clone_url, html_url, metadata, last_sync_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (connection_id, external_id) WHERE external_id <> ''
			DO UPDATE SET
				full_name = EXCLUDED.full_name,
				default_branch = EXCLUDED.default_branch,
				clone_url = EXCLUDED.clone_url,
				html_url = EXCLUDED.html_url,
				metadata = EXCLUDED.metadata,
				last_sync_at = EXCLUDED.last_sync_at`,
			orgID, connectionID, in.ExternalID, in.FullName, in.DefaultBranch,
			in.CloneURL, in.HTMLURL, meta, syncedAt,
		); err != nil {
			return nil, err
		}
	}
	ct, err := tx.Exec(ctx, `
		UPDATE git_connections
		SET status = $2, last_sync_at = $3
		WHERE id = $1 AND deleted_at IS NULL`, connectionID, status, syncedAt)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	rows, err := tx.Query(ctx, `
		SELECT id, organization_id, connection_id, external_id, full_name, default_branch,
		       clone_url, html_url, metadata, last_sync_at, created_at, updated_at
		FROM git_repositories
		WHERE connection_id = $1
		ORDER BY full_name ASC`, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Repository
	for rows.Next() {
		repo, err := scanRepository(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, repo)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}
