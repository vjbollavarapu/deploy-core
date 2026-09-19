package audit_test

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

	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b27_test?sslmode=disable"
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
		TRUNCATE audit_logs;
		DELETE FROM jobs;
		DELETE FROM environments;
		DELETE FROM projects;
		DELETE FROM organization_invitation_roles;
		DELETE FROM organization_invitations;
		DELETE FROM member_roles;
		DELETE FROM organization_members;
		DELETE FROM password_reset_tokens;
		DELETE FROM sessions;
		DELETE FROM organizations;
		DELETE FROM users;`)
	return pool
}

func testServer(t *testing.T, pool *pgxpool.Pool) *server.Server {
	t.Helper()
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b27-audit"))
	cfg := config.Config{
		Env:                   "test",
		AuthTokenSecret:       "test-secret-0123456789abcdef01234567",
		AccessTokenTTL:        time.Minute,
		RefreshTokenTTL:       time.Hour,
		AuthRateLimitPerMin:   1000,
		CORSAllowedOrigins:    []string{"*"},
		AuthMinPasswordLength: 8,
		SecretsPlatformKey:    key,
		SecretsKeyID:          "platform:v1",
		JobWorkerEnabled:      false,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return server.New(cfg, log, pool)
}

func TestAuditAppendOnlyAndList(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	email := "owner-b27-" + uuid.NewString() + "@example.com"
	ownerTok := register(t, srv, email, "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B27 Org", "b27-org-"+uuid.NewString()[:8])

	// Role change is audited.
	inv := doJSON(t, srv, http.MethodPost, "/api/v1/organizations/"+orgID+"/invitations",
		mustJSON(map[string]any{"email": "member-" + uuid.NewString() + "@example.com", "roleKeys": []string{"viewer"}}), ownerTok)
	if inv.Code != http.StatusCreated {
		t.Fatalf("invite=%d %s", inv.Code, inv.Body.String())
	}

	list := doJSON(t, srv, http.MethodGet, "/api/v1/audit-logs?organizationId="+orgID+"&action=organization.invite", nil, ownerTok)
	if list.Code != http.StatusOK {
		t.Fatalf("list=%d %s", list.Code, list.Body.String())
	}
	var page struct {
		Items []struct {
			Action string `json:"action"`
			ID     string `json:"id"`
		} `json:"items"`
		TotalCount *int64 `json:"totalCount"`
	}
	decode(t, list, &page)
	if page.TotalCount == nil || *page.TotalCount < 1 {
		t.Fatalf("expected invite audit, got %#v", page)
	}

	// UPDATE must be rejected by trigger.
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		SELECT id FROM audit_logs WHERE organization_id = $1 LIMIT 1`, orgID).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(context.Background(), `UPDATE audit_logs SET action = 'tampered' WHERE id = $1`, id)
	if err == nil {
		t.Fatal("expected update to fail")
	}
	_, err = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE id = $1`, id)
	if err == nil {
		t.Fatal("expected delete to fail")
	}

	// Login failure audited (no org).
	bad := doJSON(t, srv, http.MethodPost, "/api/v1/auth/login",
		mustJSON(map[string]string{"email": email, "password": "wrong-password"}), "")
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad login=%d", bad.Code)
	}
	var failCount int
	err = pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM audit_logs WHERE action = 'auth.login.failure'`).Scan(&failCount)
	if err != nil {
		t.Fatal(err)
	}
	if failCount < 1 {
		t.Fatal("expected auth.login.failure audit")
	}

	if len(audit.KnownActions) < 10 {
		t.Fatalf("known actions too short: %d", len(audit.KnownActions))
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
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
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/organizations", mustJSON(map[string]string{
		"name": name, "slug": slug,
	}), token)
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

func doJSON(t *testing.T, srv *server.Server, method, path string, body []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.HTTPHandler().ServeHTTP(rec, r)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(dst); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
}
