package servers_test

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
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b6_test?sslmode=disable"
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
		DELETE FROM server_agents;
		DELETE FROM servers;
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

func TestServerRegistryLifecycle(t *testing.T) {
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

	ownerTok := register(t, srv, "owner-b6@example.com", "password123", "Owner")
	viewerTok := register(t, srv, "viewer-b6@example.com", "password123", "Viewer")
	orgID := createOrg(t, srv, ownerTok, "B6 Org", "b6-org")
	inviteViewer(t, srv, ownerTok, viewerTok, orgID, "viewer-b6@example.com")

	createBody, _ := json.Marshal(map[string]any{
		"organizationId": orgID,
		"name":           "edge-1",
		"provider":       "hetzner",
		"region":         "fsn1",
		"hostname":       "edge-1.local",
		"publicIp":       "203.0.113.10",
		"architecture":   "amd64",
		"cpuCores":       4,
		"memoryBytes":    8589934592,
		"labels":         map[string]string{"tier": "edge"},
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", createBody, ownerTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Server struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Name   string `json:"name"`
		} `json:"server"`
	}
	decode(t, rec, &created)
	if created.Server.Status != "OFFLINE" {
		t.Fatalf("initial status=%s", created.Server.Status)
	}
	serverID := created.Server.ID

	// Client cannot set status directly.
	badPatch, _ := json.Marshal(map[string]string{"status": "ONLINE"})
	bad := doJSON(t, srv, http.MethodPatch, "/api/v1/servers/"+serverID, badPatch, ownerTok)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("status patch status=%d body=%s", bad.Code, bad.Body.String())
	}

	// Viewer can read, cannot create.
	list := doJSON(t, srv, http.MethodGet, "/api/v1/servers?organizationId="+orgID, nil, viewerTok)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d", list.Code)
	}
	createDenied := doJSON(t, srv, http.MethodPost, "/api/v1/servers", createBody, viewerTok)
	if createDenied.Code != http.StatusForbidden {
		t.Fatalf("viewer create status=%d", createDenied.Code)
	}

	// Maintenance enter/exit.
	maint := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/maintenance", nil, ownerTok)
	if maint.Code != http.StatusOK {
		t.Fatalf("enter maintenance status=%d body=%s", maint.Code, maint.Body.String())
	}
	var maintBody struct {
		Server struct {
			Status          string `json:"status"`
			MaintenanceMode bool   `json:"maintenanceMode"`
		} `json:"server"`
	}
	decode(t, maint, &maintBody)
	if maintBody.Server.Status != "MAINTENANCE" || !maintBody.Server.MaintenanceMode {
		t.Fatalf("maintenance body=%#v", maintBody.Server)
	}

	exit := doJSON(t, srv, http.MethodDelete, "/api/v1/servers/"+serverID+"/maintenance", nil, ownerTok)
	if exit.Code != http.StatusOK {
		t.Fatalf("exit maintenance status=%d body=%s", exit.Code, exit.Body.String())
	}
	decode(t, exit, &maintBody)
	if maintBody.Server.Status != "OFFLINE" || maintBody.Server.MaintenanceMode {
		t.Fatalf("after exit=%#v", maintBody.Server)
	}

	// Disable via patch.
	dis, _ := json.Marshal(map[string]any{"disabled": true})
	disabled := doJSON(t, srv, http.MethodPatch, "/api/v1/servers/"+serverID, dis, ownerTok)
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	var disBody struct {
		Server struct {
			Status string `json:"status"`
		} `json:"server"`
	}
	decode(t, disabled, &disBody)
	if disBody.Server.Status != "DISABLED" {
		t.Fatalf("disabled status=%s", disBody.Server.Status)
	}
	maintDenied := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/maintenance", nil, ownerTok)
	if maintDenied.Code != http.StatusConflict {
		t.Fatalf("disabled maintenance status=%d", maintDenied.Code)
	}

	en, _ := json.Marshal(map[string]any{"disabled": false})
	enabled := doJSON(t, srv, http.MethodPatch, "/api/v1/servers/"+serverID, en, ownerTok)
	if enabled.Code != http.StatusOK {
		t.Fatalf("enable status=%d", enabled.Code)
	}
	decode(t, enabled, &disBody)
	if disBody.Server.Status != "OFFLINE" {
		t.Fatalf("re-enabled status=%s want OFFLINE", disBody.Server.Status)
	}

	// Delete protection with targeted application.
	_, err := pool.Exec(context.Background(), `
		INSERT INTO projects (id, organization_id, name, slug) VALUES
		  ('11111111-1111-1111-1111-111111111111', $1, 'p', 'p')`, orgID)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO environments (id, organization_id, project_id, name, slug) VALUES
		  ('22222222-2222-2222-2222-222222222222', $1, '11111111-1111-1111-1111-111111111111', 'e', 'e')`, orgID)
	if err != nil {
		t.Fatalf("seed env: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO applications (organization_id, project_id, environment_id, name, slug, type, target_server_id)
		VALUES ($1, '11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 'api', 'api', 'API', $2)`,
		orgID, serverID)
	if err != nil {
		t.Fatalf("seed app: %v", err)
	}
	delBlocked := doJSON(t, srv, http.MethodDelete, "/api/v1/servers/"+serverID, nil, ownerTok)
	if delBlocked.Code != http.StatusConflict {
		t.Fatalf("delete with apps status=%d body=%s", delBlocked.Code, delBlocked.Body.String())
	}

	_, _ = pool.Exec(context.Background(), `UPDATE applications SET deleted_at = NOW()`)
	del := doJSON(t, srv, http.MethodDelete, "/api/v1/servers/"+serverID, nil, ownerTok)
	if del.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", del.Code, del.Body.String())
	}
	gone := doJSON(t, srv, http.MethodGet, "/api/v1/servers/"+serverID, nil, ownerTok)
	if gone.Code != http.StatusNotFound {
		t.Fatalf("get deleted status=%d", gone.Code)
	}

	// Name reusable after soft delete.
	again := doJSON(t, srv, http.MethodPost, "/api/v1/servers", createBody, ownerTok)
	if again.Code != http.StatusCreated {
		t.Fatalf("recreate status=%d body=%s", again.Code, again.Body.String())
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

func inviteViewer(t *testing.T, srv *server.Server, ownerTok, viewerTok, orgID, email string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"email": email, "roleKeys": []string{"viewer"}})
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
