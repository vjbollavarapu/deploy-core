package rbac

import (
	"context"
	"errors"

	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Authorizer evaluates organization-scoped permissions for the current user.
type Authorizer struct {
	pool *pgxpool.Pool
}

func NewAuthorizer(pool *pgxpool.Pool) *Authorizer {
	return &Authorizer{pool: pool}
}

// HasPermission reports whether userID holds permissionKey in organizationID
// via an active membership and assigned roles.
func (a *Authorizer) HasPermission(ctx context.Context, userID, organizationID uuid.UUID, permissionKey string) (bool, error) {
	const q = `
		SELECT EXISTS (
			SELECT 1
			FROM organization_members om
			JOIN member_roles mr ON mr.member_id = om.id
			JOIN role_permissions rp ON rp.role_id = mr.role_id
			JOIN permissions p ON p.id = rp.permission_id
			WHERE om.organization_id = $1
			  AND om.user_id = $2
			  AND om.status = 'active'
			  AND p.key = $3
		)`
	var ok bool
	if err := a.pool.QueryRow(ctx, q, organizationID, userID, permissionKey).Scan(&ok); err != nil {
		return false, err
	}
	return ok, nil
}

// RequirePermission returns FORBIDDEN when the user lacks the permission.
func (a *Authorizer) RequirePermission(ctx context.Context, userID, organizationID uuid.UUID, permissionKey string) error {
	ok, err := a.HasPermission(ctx, userID, organizationID, permissionKey)
	if err != nil {
		return err
	}
	if !ok {
		// Distinguish missing membership from missing permission when possible.
		member, mErr := a.ActiveMember(ctx, userID, organizationID)
		if mErr != nil {
			if errors.Is(mErr, ErrNotMember) {
				return apierror.Forbidden("not a member of this organization")
			}
			return mErr
		}
		_ = member
		return apierror.Forbidden("missing permission: " + permissionKey)
	}
	return nil
}

var ErrNotMember = errors.New("not a member")

type MemberRef struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Status string
}

func (a *Authorizer) ActiveMember(ctx context.Context, userID, organizationID uuid.UUID) (MemberRef, error) {
	const q = `
		SELECT id, user_id, status
		FROM organization_members
		WHERE organization_id = $1 AND user_id = $2 AND status = 'active'`
	var m MemberRef
	err := a.pool.QueryRow(ctx, q, organizationID, userID).Scan(&m.ID, &m.UserID, &m.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return MemberRef{}, ErrNotMember
	}
	return m, err
}
