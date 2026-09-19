package auth

import (
	"context"
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
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrInvalidState = errors.New("invalid state")
)

// Repository persists auth entities.
type Repository interface {
	CreateUser(ctx context.Context, email, passwordHash, displayName string) (UserRecord, error)
	GetUserByEmail(ctx context.Context, email string) (UserRecord, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (UserRecord, error)
	UpdateLastLogin(ctx context.Context, id uuid.UUID, at time.Time) error
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error

	CreateSession(ctx context.Context, userID uuid.UUID, refreshHash string, expiresAt time.Time, userAgent, ip *string, familyID *uuid.UUID) (Session, error)
	GetSessionByRefreshHash(ctx context.Context, refreshHash string) (Session, error)
	GetSessionByID(ctx context.Context, id uuid.UUID) (Session, error)
	RevokeSession(ctx context.Context, id uuid.UUID, at time.Time) error
	RevokeSessionAtomic(ctx context.Context, id uuid.UUID, at time.Time) (Session, error)
	RevokeSessionFamily(ctx context.Context, familyID uuid.UUID, at time.Time) error
	MarkSessionReplaced(ctx context.Context, oldID, newID uuid.UUID) error
	RevokeAllUserSessions(ctx context.Context, userID uuid.UUID, at time.Time) error

	CreatePasswordResetToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time, requestID *string) (PasswordResetToken, error)
	GetPasswordResetByHash(ctx context.Context, tokenHash string) (PasswordResetToken, error)
	MarkPasswordResetUsed(ctx context.Context, id uuid.UUID, at time.Time) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateUser(ctx context.Context, email, passwordHash, displayName string) (UserRecord, error) {
	email = normalizeEmail(email)
	const q = `
		INSERT INTO users (email, password_hash, display_name, status)
		VALUES ($1, $2, $3, 'active')
		RETURNING id, email, password_hash, display_name, status, mfa_enabled,
		          email_verified_at, last_login_at, created_at, updated_at, deleted_at`
	rec, err := scanUser(r.pool.QueryRow(ctx, q, email, passwordHash, displayName))
	if err != nil {
		if isUniqueViolation(err) {
			return UserRecord{}, ErrConflict
		}
		return UserRecord{}, err
	}
	return rec, nil
}

func (r *PostgresRepository) GetUserByEmail(ctx context.Context, email string) (UserRecord, error) {
	email = normalizeEmail(email)
	const q = `
		SELECT id, email, password_hash, display_name, status, mfa_enabled,
		       email_verified_at, last_login_at, created_at, updated_at, deleted_at
		FROM users
		WHERE LOWER(email) = LOWER($1) AND deleted_at IS NULL`
	rec, err := scanUser(r.pool.QueryRow(ctx, q, email))
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRecord{}, ErrNotFound
	}
	return rec, err
}

func (r *PostgresRepository) GetUserByID(ctx context.Context, id uuid.UUID) (UserRecord, error) {
	const q = `
		SELECT id, email, password_hash, display_name, status, mfa_enabled,
		       email_verified_at, last_login_at, created_at, updated_at, deleted_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL`
	rec, err := scanUser(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRecord{}, ErrNotFound
	}
	return rec, err
}

func (r *PostgresRepository) UpdateLastLogin(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET last_login_at = $2 WHERE id = $1 AND deleted_at IS NULL`, id, at)
	return err
}

func (r *PostgresRepository) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	ct, err := r.pool.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1 AND deleted_at IS NULL`, id, passwordHash)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) CreateSession(ctx context.Context, userID uuid.UUID, refreshHash string, expiresAt time.Time, userAgent, ip *string, familyID *uuid.UUID) (Session, error) {
	var ipAddr any
	if ip != nil && *ip != "" {
		if addr, err := netip.ParseAddr(strings.TrimSpace(*ip)); err == nil {
			ipAddr = addr
		}
	}
	id := uuid.New()
	family := id
	if familyID != nil && *familyID != uuid.Nil {
		family = *familyID
	}
	const q = `
		INSERT INTO sessions (id, user_id, refresh_token_hash, user_agent, ip_address, expires_at, family_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, family_id, replaced_by_session_id, refresh_token_hash, user_agent, host(ip_address)::text,
		          expires_at, revoked_at, created_at, updated_at`
	return scanSession(r.pool.QueryRow(ctx, q, id, userID, refreshHash, userAgent, ipAddr, expiresAt, family))
}

func (r *PostgresRepository) GetSessionByRefreshHash(ctx context.Context, refreshHash string) (Session, error) {
	const q = `
		SELECT id, user_id, family_id, replaced_by_session_id, refresh_token_hash, user_agent, host(ip_address)::text,
		       expires_at, revoked_at, created_at, updated_at
		FROM sessions WHERE refresh_token_hash = $1`
	s, err := scanSession(r.pool.QueryRow(ctx, q, refreshHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return s, err
}

func (r *PostgresRepository) GetSessionByID(ctx context.Context, id uuid.UUID) (Session, error) {
	const q = `
		SELECT id, user_id, family_id, replaced_by_session_id, refresh_token_hash, user_agent, host(ip_address)::text,
		       expires_at, revoked_at, created_at, updated_at
		FROM sessions WHERE id = $1`
	s, err := scanSession(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return s, err
}

func (r *PostgresRepository) RevokeSession(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = $2
		WHERE id = $1 AND revoked_at IS NULL`, id, at)
	return err
}

func (r *PostgresRepository) RevokeSessionAtomic(ctx context.Context, id uuid.UUID, at time.Time) (Session, error) {
	s, err := scanSession(r.pool.QueryRow(ctx, `
		UPDATE sessions SET revoked_at = $2
		WHERE id = $1 AND revoked_at IS NULL
		RETURNING id, user_id, family_id, replaced_by_session_id, refresh_token_hash, user_agent, host(ip_address)::text,
		          expires_at, revoked_at, created_at, updated_at`, id, at))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrConflict
	}
	return s, err
}

func (r *PostgresRepository) RevokeSessionFamily(ctx context.Context, familyID uuid.UUID, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = $2
		WHERE family_id = $1 AND revoked_at IS NULL`, familyID, at)
	return err
}

func (r *PostgresRepository) MarkSessionReplaced(ctx context.Context, oldID, newID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE sessions SET replaced_by_session_id = $2 WHERE id = $1`, oldID, newID)
	return err
}

func (r *PostgresRepository) RevokeAllUserSessions(ctx context.Context, userID uuid.UUID, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = $2
		WHERE user_id = $1 AND revoked_at IS NULL`, userID, at)
	return err
}

func (r *PostgresRepository) CreatePasswordResetToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time, requestID *string) (PasswordResetToken, error) {
	const q = `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at, request_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, token_hash, expires_at, used_at, request_id, created_at`
	var t PasswordResetToken
	err := r.pool.QueryRow(ctx, q, userID, tokenHash, expiresAt, requestID).Scan(
		&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.RequestID, &t.CreatedAt,
	)
	return t, err
}

func (r *PostgresRepository) GetPasswordResetByHash(ctx context.Context, tokenHash string) (PasswordResetToken, error) {
	const q = `
		SELECT id, user_id, token_hash, expires_at, used_at, request_id, created_at
		FROM password_reset_tokens WHERE token_hash = $1`
	var t PasswordResetToken
	err := r.pool.QueryRow(ctx, q, tokenHash).Scan(
		&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.RequestID, &t.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PasswordResetToken{}, ErrNotFound
	}
	return t, err
}

func (r *PostgresRepository) MarkPasswordResetUsed(ctx context.Context, id uuid.UUID, at time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE password_reset_tokens SET used_at = $2
		WHERE id = $1 AND used_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanUser(row scannable) (UserRecord, error) {
	var rec UserRecord
	err := row.Scan(
		&rec.ID, &rec.Email, &rec.PasswordHash, &rec.DisplayName, &rec.Status, &rec.MFAEnabled,
		&rec.EmailVerifiedAt, &rec.LastLoginAt, &rec.CreatedAt, &rec.UpdatedAt, &rec.DeletedAt,
	)
	return rec, err
}

func scanSession(row scannable) (Session, error) {
	var s Session
	err := row.Scan(
		&s.ID, &s.UserID, &s.FamilyID, &s.ReplacedBySessionID, &s.RefreshTokenHash, &s.UserAgent, &s.IPAddress,
		&s.ExpiresAt, &s.RevokedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	return s, err
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
