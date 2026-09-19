package registries

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

type credentialBlob struct {
	Ciphertext []byte
	Nonce      []byte
	KeyID      string
}

type Repository interface {
	Create(ctx context.Context, r Registry, cred *credentialBlob, createdBy uuid.UUID) (Registry, error)
	Get(ctx context.Context, id uuid.UUID) (Registry, *credentialBlob, error)
	List(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Registry, int64, error)
	Update(ctx context.Context, id uuid.UUID, r Registry, cred *credentialBlob, clearCreds bool) (Registry, error)
	SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, reg Registry, cred *credentialBlob, createdBy uuid.UUID) (Registry, error) {
	meta, _ := json.Marshal(mapOrEmpty(reg.Metadata))
	var ct, nonce []byte
	var keyID *string
	if cred != nil {
		ct, nonce = cred.Ciphertext, cred.Nonce
		keyID = &cred.KeyID
	}
	const q = `
		INSERT INTO registries (
			organization_id, name, provider, registry_url, username,
			credential_ciphertext, credential_nonce, credential_key_id, credential_algorithm,
			status, metadata, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'AES-256-GCM',$9,$10,$11)
		RETURNING id, organization_id, name, provider, registry_url, username, status, metadata,
		          created_by, created_at, updated_at,
		          (credential_ciphertext IS NOT NULL)`
	out, err := scanRegistry(r.pool.QueryRow(ctx, q,
		reg.OrganizationID, reg.Name, reg.Provider, reg.RegistryURL, reg.Username,
		ct, nonce, keyID, reg.Status, meta, createdBy,
	))
	if isUniqueViolation(err) {
		return Registry{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Registry, *credentialBlob, error) {
	const q = `
		SELECT id, organization_id, name, provider, registry_url, username, status, metadata,
		       created_by, created_at, updated_at,
		       (credential_ciphertext IS NOT NULL),
		       credential_ciphertext, credential_nonce, COALESCE(credential_key_id, '')
		FROM registries
		WHERE id = $1 AND deleted_at IS NULL`
	var reg Registry
	var meta []byte
	var ct, nonce []byte
	var keyID string
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&reg.ID, &reg.OrganizationID, &reg.Name, &reg.Provider, &reg.RegistryURL, &reg.Username, &reg.Status, &meta,
		&reg.CreatedBy, &reg.CreatedAt, &reg.UpdatedAt, &reg.HasCredentials,
		&ct, &nonce, &keyID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Registry{}, nil, ErrNotFound
	}
	if err != nil {
		return Registry{}, nil, err
	}
	reg.Metadata = decodeMap(meta)
	var cred *credentialBlob
	if len(ct) > 0 {
		cred = &credentialBlob{Ciphertext: ct, Nonce: nonce, KeyID: keyID}
	}
	return reg, cred, nil
}

func (r *PostgresRepository) List(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Registry, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM registries WHERE organization_id = $1 AND deleted_at IS NULL`, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, provider, registry_url, username, status, metadata,
		       created_by, created_at, updated_at,
		       (credential_ciphertext IS NOT NULL)
		FROM registries
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Registry
	for rows.Next() {
		reg, err := scanRegistry(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, reg)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) Update(ctx context.Context, id uuid.UUID, reg Registry, cred *credentialBlob, clearCreds bool) (Registry, error) {
	meta, _ := json.Marshal(mapOrEmpty(reg.Metadata))
	if clearCreds {
		const q = `
			UPDATE registries SET
				name = $2, registry_url = $3, username = $4, status = $5, metadata = $6,
				credential_ciphertext = NULL, credential_nonce = NULL, credential_key_id = NULL
			WHERE id = $1 AND deleted_at IS NULL
			RETURNING id, organization_id, name, provider, registry_url, username, status, metadata,
			          created_by, created_at, updated_at,
			          (credential_ciphertext IS NOT NULL)`
		out, err := scanRegistry(r.pool.QueryRow(ctx, q, id, reg.Name, reg.RegistryURL, reg.Username, reg.Status, meta))
		if errors.Is(err, pgx.ErrNoRows) {
			return Registry{}, ErrNotFound
		}
		if isUniqueViolation(err) {
			return Registry{}, ErrConflict
		}
		return out, err
	}
	if cred != nil {
		const q = `
			UPDATE registries SET
				name = $2, registry_url = $3, username = $4, status = $5, metadata = $6,
				credential_ciphertext = $7, credential_nonce = $8, credential_key_id = $9,
				credential_algorithm = 'AES-256-GCM'
			WHERE id = $1 AND deleted_at IS NULL
			RETURNING id, organization_id, name, provider, registry_url, username, status, metadata,
			          created_by, created_at, updated_at,
			          (credential_ciphertext IS NOT NULL)`
		out, err := scanRegistry(r.pool.QueryRow(ctx, q,
			id, reg.Name, reg.RegistryURL, reg.Username, reg.Status, meta,
			cred.Ciphertext, cred.Nonce, cred.KeyID,
		))
		if errors.Is(err, pgx.ErrNoRows) {
			return Registry{}, ErrNotFound
		}
		if isUniqueViolation(err) {
			return Registry{}, ErrConflict
		}
		return out, err
	}
	const q = `
		UPDATE registries SET
			name = $2, registry_url = $3, username = $4, status = $5, metadata = $6
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, organization_id, name, provider, registry_url, username, status, metadata,
		          created_by, created_at, updated_at,
		          (credential_ciphertext IS NOT NULL)`
	out, err := scanRegistry(r.pool.QueryRow(ctx, q, id, reg.Name, reg.RegistryURL, reg.Username, reg.Status, meta))
	if errors.Is(err, pgx.ErrNoRows) {
		return Registry{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Registry{}, ErrConflict
	}
	return out, err
}

func (r *PostgresRepository) SoftDelete(ctx context.Context, id uuid.UUID, at time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE registries SET deleted_at = $2, status = 'disabled'
		WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRegistry(row scannable) (Registry, error) {
	var reg Registry
	var meta []byte
	err := row.Scan(
		&reg.ID, &reg.OrganizationID, &reg.Name, &reg.Provider, &reg.RegistryURL, &reg.Username, &reg.Status, &meta,
		&reg.CreatedBy, &reg.CreatedAt, &reg.UpdatedAt, &reg.HasCredentials,
	)
	if err != nil {
		return Registry{}, err
	}
	reg.Metadata = decodeMap(meta)
	return reg, nil
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

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
