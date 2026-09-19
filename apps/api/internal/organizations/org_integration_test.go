package organizations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b4_test?sslmode=disable"
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
		t.Fatalf("rbac seed: %v", err)
	}
	_, _ = pool.Exec(ctx, `
		DELETE FROM organization_invitation_roles;
		DELETE FROM organization_invitations;
		DELETE FROM member_roles;
		DELETE FROM organization_members;
		TRUNCATE audit_logs;
		DELETE FROM password_reset_tokens;
		DELETE FROM sessions;
		DELETE FROM organizations;
		DELETE FROM users;`)
	return pool
}

func TestOrganizationOwnerLifecycleAndRBAC(t *testing.T) {
	pool := testPool(t)
	cfg := testConfig()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := server.New(cfg, log, pool)

	ownerTok := register(t, srv, "owner@example.com", "password123", "Owner")
	viewerTok := register(t, srv, "viewer@example.com", "password123", "Viewer")

	// Create organization as owner.
	orgBody, _ := json.Marshal(map[string]string{"name": "Acme Platform", "slug": "acme"})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/organizations", orgBody, ownerTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create org status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Organization struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"organization"`
	}
	decode(t, rec, &created)
	orgID := created.Organization.ID
	if created.Organization.Slug != "acme" {
		t.Fatalf("slug=%s", created.Organization.Slug)
	}

	// Owner can read org.
	get := doJSON(t, srv, http.MethodGet, "/api/v1/organizations/"+orgID, nil, ownerTok)
	if get.Code != http.StatusOK {
		t.Fatalf("get org status=%d", get.Code)
	}

	// Non-member forbidden.
	denied := doJSON(t, srv, http.MethodGet, "/api/v1/organizations/"+orgID, nil, viewerTok)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("non-member status=%d body=%s", denied.Code, denied.Body.String())
	}

	// Invite viewer as viewer role.
	invBody, _ := json.Marshal(map[string]any{
		"email": "viewer@example.com", "roleKeys": []string{"viewer"},
	})
	invRec := doJSON(t, srv, http.MethodPost, "/api/v1/organizations/"+orgID+"/invitations", invBody, ownerTok)
	if invRec.Code != http.StatusCreated {
		t.Fatalf("invite status=%d body=%s", invRec.Code, invRec.Body.String())
	}
	var inv struct {
		Invitation struct {
			Token string `json:"token"`
		} `json:"invitation"`
	}
	decode(t, invRec, &inv)
	if inv.Invitation.Token == "" {
		t.Fatal("expected invitation token")
	}

	acceptBody, _ := json.Marshal(map[string]string{"token": inv.Invitation.Token})
	acc := doJSON(t, srv, http.MethodPost, "/api/v1/invitations/accept", acceptBody, viewerTok)
	if acc.Code != http.StatusOK {
		t.Fatalf("accept status=%d body=%s", acc.Code, acc.Body.String())
	}

	// Viewer can read organization but cannot invite.
	get2 := doJSON(t, srv, http.MethodGet, "/api/v1/organizations/"+orgID, nil, viewerTok)
	if get2.Code != http.StatusOK {
		t.Fatalf("viewer get status=%d", get2.Code)
	}
	inviteDenied := doJSON(t, srv, http.MethodPost, "/api/v1/organizations/"+orgID+"/invitations", invBody, viewerTok)
	if inviteDenied.Code != http.StatusForbidden {
		t.Fatalf("viewer invite status=%d body=%s", inviteDenied.Code, inviteDenied.Body.String())
	}

	// Authorizer unit-level checks for seeded permissions.
	authz := rbac.NewAuthorizer(pool)
	ownerID := userIDFromMe(t, srv, ownerTok)
	viewerID := userIDFromMe(t, srv, viewerTok)
	orgUUID := uuid.MustParse(orgID)

	assertPerm(t, authz, ownerID, orgUUID, rbac.OrganizationUpdate, true)
	assertPerm(t, authz, ownerID, orgUUID, rbac.ServerCreate, true)
	assertPerm(t, authz, viewerID, orgUUID, rbac.OrganizationRead, true)
	assertPerm(t, authz, viewerID, orgUUID, rbac.MemberInvite, false)
	assertPerm(t, authz, viewerID, orgUUID, rbac.ServerCreate, false)
	assertPerm(t, authz, viewerID, orgUUID, rbac.AuditRead, true)

	// List members.
	members := doJSON(t, srv, http.MethodGet, "/api/v1/organizations/"+orgID+"/members", nil, ownerTok)
	if members.Code != http.StatusOK {
		t.Fatalf("members status=%d", members.Code)
	}
	var page struct {
		Items []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Roles []struct {
				Key string `json:"key"`
			} `json:"roles"`
		} `json:"items"`
		TotalCount int64 `json:"totalCount"`
	}
	decode(t, members, &page)
	if page.TotalCount != 2 {
		t.Fatalf("total=%d", page.TotalCount)
	}

	var viewerMemberID string
	for _, m := range page.Items {
		if m.Email == "viewer@example.com" {
			viewerMemberID = m.ID
		}
	}
	if viewerMemberID == "" {
		t.Fatal("viewer member missing")
	}

	// Promote viewer to developer.
	roleBody, _ := json.Marshal(map[string]any{"roleKeys": []string{"developer"}})
	promoted := doJSON(t, srv, http.MethodPatch, "/api/v1/organizations/"+orgID+"/members/"+viewerMemberID, roleBody, ownerTok)
	if promoted.Code != http.StatusOK {
		t.Fatalf("promote status=%d body=%s", promoted.Code, promoted.Body.String())
	}
	assertPerm(t, authz, viewerID, orgUUID, rbac.ApplicationDeploy, true)
	assertPerm(t, authz, viewerID, orgUUID, rbac.ServerCreate, false)

	// Cannot remove last owner.
	var ownerMemberID string
	for _, m := range page.Items {
		if m.Email == "owner@example.com" {
			ownerMemberID = m.ID
		}
	}
	delOwner := doJSON(t, srv, http.MethodDelete, "/api/v1/organizations/"+orgID+"/members/"+ownerMemberID, nil, ownerTok)
	if delOwner.Code != http.StatusConflict {
		t.Fatalf("delete last owner status=%d body=%s", delOwner.Code, delOwner.Body.String())
	}

	// Owner can update org.
	patchBody, _ := json.Marshal(map[string]string{"name": "Acme Renamed"})
	patched := doJSON(t, srv, http.MethodPatch, "/api/v1/organizations/"+orgID, patchBody, ownerTok)
	if patched.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patched.Code, patched.Body.String())
	}

	// Developer still cannot update org settings.
	patchDenied := doJSON(t, srv, http.MethodPatch, "/api/v1/organizations/"+orgID, patchBody, viewerTok)
	if patchDenied.Code != http.StatusForbidden {
		t.Fatalf("developer patch status=%d", patchDenied.Code)
	}
}

func TestRBACRoleMatrixSeeded(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	var permCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM permissions`).Scan(&permCount); err != nil {
		t.Fatalf("count permissions: %v", err)
	}
	if permCount < 30 {
		t.Fatalf("expected seeded permissions, got %d", permCount)
	}

	roles := []string{"owner", "administrator", "devops", "developer", "support", "viewer"}
	for _, key := range roles {
		var n int
		err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM role_permissions rp
			JOIN roles r ON r.id = rp.role_id
			WHERE r.organization_id IS NULL AND r.key = $1`, key).Scan(&n)
		if err != nil {
			t.Fatalf("role %s: %v", key, err)
		}
		if n == 0 {
			t.Fatalf("role %s has no permissions", key)
		}
	}
}

func testConfig() config.Config {
	return config.Config{
		Env:                   "test",
		AuthTokenSecret:       "test-secret-0123456789abcdef01234567",
		AccessTokenTTL:        time.Minute,
		RefreshTokenTTL:       time.Hour,
		AuthRateLimitPerMin:   1000,
		CORSAllowedOrigins:    []string{"*"},
		AuthMinPasswordLength: 8,
	}
}

func register(t *testing.T, srv *server.Server, email, password, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email": email, "password": password, "displayName": name,
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/auth/register", body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s status=%d body=%s", email, rec.Code, rec.Body.String())
	}
	var out struct {
		Tokens auth.TokenPair `json:"tokens"`
	}
	decode(t, rec, &out)
	return out.Tokens.AccessToken
}

func userIDFromMe(t *testing.T, srv *server.Server, token string) uuid.UUID {
	t.Helper()
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/auth/me", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("me status=%d", rec.Code)
	}
	var out struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	decode(t, rec, &out)
	return uuid.MustParse(out.User.ID)
}

func assertPerm(t *testing.T, authz *rbac.Authorizer, userID, orgID uuid.UUID, perm string, want bool) {
	t.Helper()
	got, err := authz.HasPermission(context.Background(), userID, orgID, perm)
	if err != nil {
		t.Fatalf("HasPermission(%s): %v", perm, err)
	}
	if got != want {
		t.Fatalf("permission %s: got %v want %v", perm, got, want)
	}
}

func doJSON(t *testing.T, srv *server.Server, method, path string, body []byte, access string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
	rec := httptest.NewRecorder()
	srv.HTTPHandler().ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
}
