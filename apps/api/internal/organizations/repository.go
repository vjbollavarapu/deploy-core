package organizations

import (
	"context"
	"errors"
	"strings"
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

type Repository interface {
	CreateOrganization(ctx context.Context, name, slug string, createdBy uuid.UUID) (Organization, error)
	GetOrganization(ctx context.Context, id uuid.UUID) (Organization, error)
	ListOrganizationsForUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Organization, int64, error)
	UpdateOrganization(ctx context.Context, id uuid.UUID, name, slug *string) (Organization, error)

	CreateMember(ctx context.Context, orgID, userID uuid.UUID, status string, invitedBy *uuid.UUID, joinedAt *time.Time) (uuid.UUID, error)
	GetMember(ctx context.Context, orgID, memberID uuid.UUID) (Member, error)
	GetMemberByUser(ctx context.Context, orgID, userID uuid.UUID) (Member, error)
	ListMembers(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Member, int64, error)
	DeleteMember(ctx context.Context, orgID, memberID uuid.UUID) error
	SetMemberStatus(ctx context.Context, memberID uuid.UUID, status string, joinedAt *time.Time) error
	ReplaceMemberRoles(ctx context.Context, memberID uuid.UUID, roleIDs []uuid.UUID) error
	CountActiveOwners(ctx context.Context, orgID uuid.UUID) (int, error)
	MemberHasRole(ctx context.Context, memberID uuid.UUID, roleKey string) (bool, error)

	GetSystemRolesByKeys(ctx context.Context, keys []string) ([]Role, error)
	GetSystemRoleID(ctx context.Context, key string) (uuid.UUID, error)

	FindUserIDByEmail(ctx context.Context, email string) (uuid.UUID, error)

	CreateInvitation(ctx context.Context, orgID uuid.UUID, email string, invitedBy *uuid.UUID, tokenHash string, expiresAt time.Time, roleIDs []uuid.UUID) (Invitation, error)
	GetInvitationByTokenHash(ctx context.Context, tokenHash string) (Invitation, error)
	ListInvitations(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Invitation, int64, error)
	MarkInvitationAccepted(ctx context.Context, id uuid.UUID, at time.Time) error
	RevokeOpenInvitationsForEmail(ctx context.Context, orgID uuid.UUID, email string, at time.Time) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateOrganization(ctx context.Context, name, slug string, createdBy uuid.UUID) (Organization, error) {
	const q = `
		INSERT INTO organizations (name, slug, created_by)
		VALUES ($1, $2, $3)
		RETURNING id, name, slug, status, created_by, created_at, updated_at`
	org, err := scanOrg(r.pool.QueryRow(ctx, q, name, slug, createdBy))
	if err != nil {
		if isUniqueViolation(err) {
			return Organization{}, ErrConflict
		}
		return Organization{}, err
	}
	return org, nil
}

func (r *PostgresRepository) GetOrganization(ctx context.Context, id uuid.UUID) (Organization, error) {
	const q = `
		SELECT id, name, slug, status, created_by, created_at, updated_at
		FROM organizations WHERE id = $1 AND deleted_at IS NULL`
	org, err := scanOrg(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	return org, err
}

func (r *PostgresRepository) ListOrganizationsForUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Organization, int64, error) {
	const countQ = `
		SELECT COUNT(*)
		FROM organizations o
		JOIN organization_members om ON om.organization_id = o.id
		WHERE om.user_id = $1 AND om.status IN ('active', 'invited') AND o.deleted_at IS NULL`
	var total int64
	if err := r.pool.QueryRow(ctx, countQ, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	const q = `
		SELECT o.id, o.name, o.slug, o.status, o.created_by, o.created_at, o.updated_at
		FROM organizations o
		JOIN organization_members om ON om.organization_id = o.id
		WHERE om.user_id = $1 AND om.status IN ('active', 'invited') AND o.deleted_at IS NULL
		ORDER BY o.name ASC
		LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Organization
	for rows.Next() {
		org, err := scanOrg(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, org)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) UpdateOrganization(ctx context.Context, id uuid.UUID, name, slug *string) (Organization, error) {
	const q = `
		UPDATE organizations
		SET name = COALESCE($2, name),
		    slug = COALESCE($3, slug)
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, name, slug, status, created_by, created_at, updated_at`
	org, err := scanOrg(r.pool.QueryRow(ctx, q, id, name, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Organization{}, ErrConflict
	}
	return org, err
}

func (r *PostgresRepository) CreateMember(ctx context.Context, orgID, userID uuid.UUID, status string, invitedBy *uuid.UUID, joinedAt *time.Time) (uuid.UUID, error) {
	const q = `
		INSERT INTO organization_members (organization_id, user_id, status, invited_by, joined_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, q, orgID, userID, status, invitedBy, joinedAt).Scan(&id)
	if isUniqueViolation(err) {
		return uuid.Nil, ErrConflict
	}
	return id, err
}

func (r *PostgresRepository) GetMember(ctx context.Context, orgID, memberID uuid.UUID) (Member, error) {
	const q = `
		SELECT om.id, om.organization_id, om.user_id, u.email, u.display_name, om.status,
		       om.invited_by, om.joined_at, om.created_at
		FROM organization_members om
		JOIN users u ON u.id = om.user_id
		WHERE om.organization_id = $1 AND om.id = $2`
	m, err := scanMember(r.pool.QueryRow(ctx, q, orgID, memberID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrNotFound
	}
	if err != nil {
		return Member{}, err
	}
	roles, err := r.memberRoles(ctx, m.ID)
	if err != nil {
		return Member{}, err
	}
	m.Roles = roles
	return m, nil
}

func (r *PostgresRepository) GetMemberByUser(ctx context.Context, orgID, userID uuid.UUID) (Member, error) {
	const q = `
		SELECT om.id, om.organization_id, om.user_id, u.email, u.display_name, om.status,
		       om.invited_by, om.joined_at, om.created_at
		FROM organization_members om
		JOIN users u ON u.id = om.user_id
		WHERE om.organization_id = $1 AND om.user_id = $2`
	m, err := scanMember(r.pool.QueryRow(ctx, q, orgID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrNotFound
	}
	if err != nil {
		return Member{}, err
	}
	roles, err := r.memberRoles(ctx, m.ID)
	if err != nil {
		return Member{}, err
	}
	m.Roles = roles
	return m, nil
}

func (r *PostgresRepository) ListMembers(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Member, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM organization_members WHERE organization_id = $1`, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	const q = `
		SELECT om.id, om.organization_id, om.user_id, u.email, u.display_name, om.status,
		       om.invited_by, om.joined_at, om.created_at
		FROM organization_members om
		JOIN users u ON u.id = om.user_id
		WHERE om.organization_id = $1
		ORDER BY u.email ASC
		LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range out {
		roles, err := r.memberRoles(ctx, out[i].ID)
		if err != nil {
			return nil, 0, err
		}
		out[i].Roles = roles
	}
	return out, total, nil
}

func (r *PostgresRepository) DeleteMember(ctx context.Context, orgID, memberID uuid.UUID) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM organization_members WHERE organization_id = $1 AND id = $2`, orgID, memberID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SetMemberStatus(ctx context.Context, memberID uuid.UUID, status string, joinedAt *time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE organization_members
		SET status = $2, joined_at = COALESCE($3, joined_at)
		WHERE id = $1`, memberID, status, joinedAt)
	return err
}

func (r *PostgresRepository) ReplaceMemberRoles(ctx context.Context, memberID uuid.UUID, roleIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM member_roles WHERE member_id = $1`, memberID); err != nil {
		return err
	}
	for _, roleID := range roleIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO member_roles (member_id, role_id) VALUES ($1, $2)`, memberID, roleID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) CountActiveOwners(ctx context.Context, orgID uuid.UUID) (int, error) {
	const q = `
		SELECT COUNT(DISTINCT om.id)
		FROM organization_members om
		JOIN member_roles mr ON mr.member_id = om.id
		JOIN roles r ON r.id = mr.role_id
		WHERE om.organization_id = $1
		  AND om.status = 'active'
		  AND r.key = 'owner'
		  AND r.organization_id IS NULL`
	var n int
	err := r.pool.QueryRow(ctx, q, orgID).Scan(&n)
	return n, err
}

func (r *PostgresRepository) MemberHasRole(ctx context.Context, memberID uuid.UUID, roleKey string) (bool, error) {
	const q = `
		SELECT EXISTS (
			SELECT 1 FROM member_roles mr
			JOIN roles r ON r.id = mr.role_id
			WHERE mr.member_id = $1 AND r.key = $2 AND r.organization_id IS NULL
		)`
	var ok bool
	err := r.pool.QueryRow(ctx, q, memberID, roleKey).Scan(&ok)
	return ok, err
}

func (r *PostgresRepository) GetSystemRolesByKeys(ctx context.Context, keys []string) ([]Role, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	const q = `
		SELECT id, key, name, description, is_system
		FROM roles
		WHERE organization_id IS NULL AND key = ANY($1)`
	rows, err := r.pool.Query(ctx, q, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.Key, &role.Name, &role.Description, &role.IsSystem); err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetSystemRoleID(ctx context.Context, key string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM roles WHERE organization_id IS NULL AND key = $1`, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return id, err
}

func (r *PostgresRepository) FindUserIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM users WHERE LOWER(email) = LOWER($1) AND deleted_at IS NULL`, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return id, err
}

func (r *PostgresRepository) CreateInvitation(ctx context.Context, orgID uuid.UUID, email string, invitedBy *uuid.UUID, tokenHash string, expiresAt time.Time, roleIDs []uuid.UUID) (Invitation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Invitation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO organization_invitations (organization_id, email, invited_by, token_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, organization_id, email, invited_by, expires_at, accepted_at, revoked_at, created_at`
	var inv Invitation
	err = tx.QueryRow(ctx, q, orgID, strings.ToLower(strings.TrimSpace(email)), invitedBy, tokenHash, expiresAt).Scan(
		&inv.ID, &inv.OrganizationID, &inv.Email, &inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.RevokedAt, &inv.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Invitation{}, ErrConflict
		}
		return Invitation{}, err
	}
	for _, roleID := range roleIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO organization_invitation_roles (invitation_id, role_id) VALUES ($1, $2)`, inv.ID, roleID); err != nil {
			return Invitation{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Invitation{}, err
	}
	roles, err := r.invitationRoles(ctx, inv.ID)
	if err != nil {
		return Invitation{}, err
	}
	inv.Roles = roles
	return inv, nil
}

func (r *PostgresRepository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (Invitation, error) {
	const q = `
		SELECT id, organization_id, email, invited_by, expires_at, accepted_at, revoked_at, created_at
		FROM organization_invitations WHERE token_hash = $1`
	var inv Invitation
	err := r.pool.QueryRow(ctx, q, tokenHash).Scan(
		&inv.ID, &inv.OrganizationID, &inv.Email, &inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.RevokedAt, &inv.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, ErrNotFound
	}
	if err != nil {
		return Invitation{}, err
	}
	roles, err := r.invitationRoles(ctx, inv.ID)
	if err != nil {
		return Invitation{}, err
	}
	inv.Roles = roles
	return inv, nil
}

func (r *PostgresRepository) ListInvitations(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Invitation, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM organization_invitations
		WHERE organization_id = $1 AND accepted_at IS NULL AND revoked_at IS NULL`, orgID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, email, invited_by, expires_at, accepted_at, revoked_at, created_at
		FROM organization_invitations
		WHERE organization_id = $1 AND accepted_at IS NULL AND revoked_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Invitation
	for rows.Next() {
		var inv Invitation
		if err := rows.Scan(&inv.ID, &inv.OrganizationID, &inv.Email, &inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.RevokedAt, &inv.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range out {
		roles, err := r.invitationRoles(ctx, out[i].ID)
		if err != nil {
			return nil, 0, err
		}
		out[i].Roles = roles
	}
	return out, total, nil
}

func (r *PostgresRepository) MarkInvitationAccepted(ctx context.Context, id uuid.UUID, at time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE organization_invitations SET accepted_at = $2
		WHERE id = $1 AND accepted_at IS NULL AND revoked_at IS NULL`, id, at)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) RevokeOpenInvitationsForEmail(ctx context.Context, orgID uuid.UUID, email string, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE organization_invitations SET revoked_at = $3
		WHERE organization_id = $1 AND LOWER(email) = LOWER($2)
		  AND accepted_at IS NULL AND revoked_at IS NULL`, orgID, email, at)
	return err
}

func (r *PostgresRepository) memberRoles(ctx context.Context, memberID uuid.UUID) ([]Role, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.id, r.key, r.name, r.description, r.is_system
		FROM member_roles mr
		JOIN roles r ON r.id = mr.role_id
		WHERE mr.member_id = $1
		ORDER BY r.name`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.Key, &role.Name, &role.Description, &role.IsSystem); err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) invitationRoles(ctx context.Context, invitationID uuid.UUID) ([]Role, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.id, r.key, r.name, r.description, r.is_system
		FROM organization_invitation_roles ir
		JOIN roles r ON r.id = ir.role_id
		WHERE ir.invitation_id = $1
		ORDER BY r.name`, invitationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.Key, &role.Name, &role.Description, &role.IsSystem); err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanOrg(row scannable) (Organization, error) {
	var o Organization
	err := row.Scan(&o.ID, &o.Name, &o.Slug, &o.Status, &o.CreatedBy, &o.CreatedAt, &o.UpdatedAt)
	return o, err
}

func scanMember(row scannable) (Member, error) {
	var m Member
	err := row.Scan(&m.ID, &m.OrganizationID, &m.UserID, &m.Email, &m.DisplayName, &m.Status, &m.InvitedBy, &m.JoinedAt, &m.CreatedAt)
	return m, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
