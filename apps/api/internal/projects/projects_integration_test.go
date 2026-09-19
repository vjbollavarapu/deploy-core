package projects_test

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
		url = "postgres://localhost/deploycore_b5_test?sslmode=disable"
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
		DELETE FROM applications;
		DELETE FROM environments;
		DELETE FROM projects;
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

func TestProjectEnvironmentLifecycle(t *testing.T) {
	pool := testPool(t)
	cfg := config.Config{
		Env:                   "test",
		AuthTokenSecret:       "test-secret-0123456789abcdef01234567",
		AccessTokenTTL:        time.Minute,
		RefreshTokenTTL:       time.Hour,
		AuthRateLimitPerMin:   1000,
		CORSAllowedOrigins:    []string{"*"},
		AuthMinPasswordLength: 8,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := server.New(cfg, log, pool)

	ownerTok := register(t, srv, "owner-b5@example.com", "password123", "Owner")
	viewerTok := register(t, srv, "viewer-b5@example.com", "password123", "Viewer")

	orgID := createOrg(t, srv, ownerTok, "B5 Org", "b5-org")
	inviteAndAcceptViewer(t, srv, ownerTok, viewerTok, orgID)

	// Create project.
	body, _ := json.Marshal(map[string]string{
		"organizationId": orgID, "name": "Platform", "slug": "platform", "description": "core",
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/projects", body, ownerTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Project struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"project"`
	}
	decode(t, rec, &created)
	projectID := created.Project.ID

	// Duplicate slug conflict.
	dup := doJSON(t, srv, http.MethodPost, "/api/v1/projects", body, ownerTok)
	if dup.Code != http.StatusConflict {
		t.Fatalf("dup slug status=%d", dup.Code)
	}

	// Viewer can list/read, cannot create.
	list := doJSON(t, srv, http.MethodGet, "/api/v1/projects?organizationId="+orgID, nil, viewerTok)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	createDenied := doJSON(t, srv, http.MethodPost, "/api/v1/projects", body, viewerTok)
	if createDenied.Code != http.StatusForbidden {
		t.Fatalf("viewer create status=%d", createDenied.Code)
	}

	// Environments.
	envBody, _ := json.Marshal(map[string]string{"name": "Production", "slug": "production", "kind": "production"})
	envRec := doJSON(t, srv, http.MethodPost, "/api/v1/projects/"+projectID+"/environments", envBody, ownerTok)
	if envRec.Code != http.StatusCreated {
		t.Fatalf("create env status=%d body=%s", envRec.Code, envRec.Body.String())
	}
	var envCreated struct {
		Environment struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"environment"`
	}
	decode(t, envRec, &envCreated)
	envID := envCreated.Environment.ID
	if envCreated.Environment.Kind != "production" {
		t.Fatalf("kind=%s", envCreated.Environment.Kind)
	}

	dupEnv := doJSON(t, srv, http.MethodPost, "/api/v1/projects/"+projectID+"/environments", envBody, ownerTok)
	if dupEnv.Code != http.StatusConflict {
		t.Fatalf("dup env status=%d", dupEnv.Code)
	}

	patchEnv, _ := json.Marshal(map[string]string{"name": "Prod"})
	patched := doJSON(t, srv, http.MethodPatch, "/api/v1/environments/"+envID, patchEnv, ownerTok)
	if patched.Code != http.StatusOK {
		t.Fatalf("patch env status=%d body=%s", patched.Code, patched.Body.String())
	}

	// Deletion protection when applications exist.
	_, err := pool.Exec(context.Background(), `
		INSERT INTO applications (organization_id, project_id, environment_id, name, slug, type)
		VALUES ($1, $2, $3, 'api', 'api', 'API')`, orgID, projectID, envID)
	if err != nil {
		t.Fatalf("insert app: %v", err)
	}
	delEnv := doJSON(t, srv, http.MethodDelete, "/api/v1/environments/"+envID, nil, ownerTok)
	if delEnv.Code != http.StatusConflict {
		t.Fatalf("delete env with apps status=%d body=%s", delEnv.Code, delEnv.Body.String())
	}
	delProj := doJSON(t, srv, http.MethodDelete, "/api/v1/projects/"+projectID, nil, ownerTok)
	if delProj.Code != http.StatusConflict {
		t.Fatalf("delete project with apps status=%d body=%s", delProj.Code, delProj.Body.String())
	}

	_, err = pool.Exec(context.Background(), `UPDATE applications SET deleted_at = NOW() WHERE project_id = $1`, projectID)
	if err != nil {
		t.Fatalf("soft delete apps: %v", err)
	}

	delEnv2 := doJSON(t, srv, http.MethodDelete, "/api/v1/environments/"+envID, nil, ownerTok)
	if delEnv2.Code != http.StatusOK {
		t.Fatalf("delete env status=%d body=%s", delEnv2.Code, delEnv2.Body.String())
	}

	// Recreate env with same slug after soft delete.
	envRec2 := doJSON(t, srv, http.MethodPost, "/api/v1/projects/"+projectID+"/environments", envBody, ownerTok)
	if envRec2.Code != http.StatusCreated {
		t.Fatalf("recreate env status=%d body=%s", envRec2.Code, envRec2.Body.String())
	}

	delProj2 := doJSON(t, srv, http.MethodDelete, "/api/v1/projects/"+projectID, nil, ownerTok)
	if delProj2.Code != http.StatusOK {
		t.Fatalf("delete project status=%d body=%s", delProj2.Code, delProj2.Body.String())
	}
	getGone := doJSON(t, srv, http.MethodGet, "/api/v1/projects/"+projectID, nil, ownerTok)
	if getGone.Code != http.StatusNotFound {
		t.Fatalf("get deleted project status=%d", getGone.Code)
	}

	// Slug reusable after soft delete.
	rec2 := doJSON(t, srv, http.MethodPost, "/api/v1/projects", body, ownerTok)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("recreate project status=%d body=%s", rec2.Code, rec2.Body.String())
	}

	// Audit rows present.
	var auditCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'project.%' OR action LIKE 'environment.%'`).Scan(&auditCount); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditCount < 4 {
		t.Fatalf("expected audit events, got %d", auditCount)
	}
}

func register(t *testing.T, srv *server.Server, email, password, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password, "displayName": name})
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

func createOrg(t *testing.T, srv *server.Server, token, name, slug string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name, "slug": slug})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/organizations", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create org status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Organization struct {
			ID string `json:"id"`
		} `json:"organization"`
	}
	decode(t, rec, &out)
	return out.Organization.ID
}

func inviteAndAcceptViewer(t *testing.T, srv *server.Server, ownerTok, viewerTok, orgID string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"email": "viewer-b5@example.com", "roleKeys": []string{"viewer"}})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/organizations/"+orgID+"/invitations", body, ownerTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite status=%d body=%s", rec.Code, rec.Body.String())
	}
	var inv struct {
		Invitation struct {
			Token string `json:"token"`
		} `json:"invitation"`
	}
	decode(t, rec, &inv)
	accept, _ := json.Marshal(map[string]string{"token": inv.Invitation.Token})
	acc := doJSON(t, srv, http.MethodPost, "/api/v1/invitations/accept", accept, viewerTok)
	if acc.Code != http.StatusOK {
		t.Fatalf("accept status=%d body=%s", acc.Code, acc.Body.String())
	}
	_ = uuid.Nil
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
