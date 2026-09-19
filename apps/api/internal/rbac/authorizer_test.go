package rbac_test

import (
	"context"
	"os"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b33_rbac_test?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, url)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.NewMigrator(pool).Up(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := rbac.EnsureSeeded(ctx, pool); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, _ = pool.Exec(ctx, `
		DELETE FROM member_roles;
		DELETE FROM organization_members;
		DELETE FROM organizations;
		DELETE FROM users;`)
	return pool
}

func addUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, email, password_hash, display_name)
		VALUES ($1, $2, 'x', 'U')`, id, id.String()+"@example.com")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	return id
}

func addOrg(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO organizations (id, name, slug) VALUES ($1, 'Org', $2)`,
		id, "org-"+id.String()[:8])
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	return id
}

func assignRole(t *testing.T, pool *pgxpool.Pool, orgID, userID uuid.UUID, roleKey string) {
	t.Helper()
	ctx := context.Background()
	var memberID, roleID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO organization_members (organization_id, user_id, status)
		VALUES ($1, $2, 'active') RETURNING id`, orgID, userID).Scan(&memberID)
	if err != nil {
		t.Fatalf("member: %v", err)
	}
	err = pool.QueryRow(ctx, `
		SELECT id FROM roles WHERE key = $1 AND is_system = true`, roleKey).Scan(&roleID)
	if err != nil {
		t.Fatalf("role %s: %v", roleKey, err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO member_roles (member_id, role_id) VALUES ($1, $2)`, memberID, roleID)
	if err != nil {
		t.Fatalf("member_roles: %v", err)
	}
}

func TestAuthorizerOwnerVsViewer(t *testing.T) {
	pool := testPool(t)
	authz := rbac.NewAuthorizer(pool)
	ctx := context.Background()

	orgID := addOrg(t, pool)
	ownerID := addUser(t, pool)
	viewerID := addUser(t, pool)
	assignRole(t, pool, orgID, ownerID, rbac.RoleOwner)
	assignRole(t, pool, orgID, viewerID, rbac.RoleViewer)

	ok, err := authz.HasPermission(ctx, ownerID, orgID, rbac.DeploymentCreate)
	if err != nil || !ok {
		t.Fatalf("owner deploy create: ok=%v err=%v", ok, err)
	}
	ok, err = authz.HasPermission(ctx, viewerID, orgID, rbac.DeploymentCreate)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("viewer must not create deployments")
	}
	ok, err = authz.HasPermission(ctx, viewerID, orgID, rbac.DeploymentRead)
	if err != nil || !ok {
		t.Fatalf("viewer read: ok=%v err=%v", ok, err)
	}

	if err := authz.RequirePermission(ctx, viewerID, orgID, rbac.ServerDelete); err == nil {
		t.Fatal("expected forbidden")
	} else if apiErr, ok := apierror.AsAPIError(err); !ok || apiErr.Code != apierror.CodeForbidden {
		t.Fatalf("expected FORBIDDEN got %#v", err)
	}

	stranger := addUser(t, pool)
	if err := authz.RequirePermission(ctx, stranger, orgID, rbac.OrganizationRead); err == nil {
		t.Fatal("stranger should be forbidden")
	}
}
