package domains_test

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
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b18_test?sslmode=disable"
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
		DELETE FROM certificates;
		DELETE FROM domains;
		DELETE FROM application_configs;
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

func testServer(t *testing.T, pool *pgxpool.Pool) *server.Server {
	t.Helper()
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b18-domains"))
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
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return server.New(cfg, log, pool)
}

func TestDomainCRUDConflictAndRouting(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b18@example.com", "password123", "Owner")
	viewerTok := register(t, srv, "viewer-b18@example.com", "password123", "Viewer")
	orgID := createOrg(t, srv, ownerTok, "B18 Org", "b18-org")
	inviteViewer(t, srv, ownerTok, viewerTok, orgID, "viewer-b18@example.com")
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	appID := createApplication(t, srv, ownerTok, orgID, projectID, envID)

	body, _ := json.Marshal(map[string]any{
		"hostname": "API.Example.com", "forceHttps": true,
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/domains", body, ownerTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Domain struct {
			ID           string `json:"id"`
			Hostname     string `json:"hostname"`
			IsPrimary    bool   `json:"isPrimary"`
			InternalPort int    `json:"internalPort"`
			DNSStatus    string `json:"dnsStatus"`
			TLSStatus    string `json:"tlsStatus"`
			Routing      struct {
				Provider string            `json:"provider"`
				Labels   map[string]string `json:"labels"`
			} `json:"routing"`
		} `json:"domain"`
	}
	decode(t, rec, &created)
	if created.Domain.Hostname != "api.example.com" || !created.Domain.IsPrimary {
		t.Fatalf("domain=%#v", created.Domain)
	}
	if created.Domain.InternalPort != 8080 {
		t.Fatalf("port=%d want from app config", created.Domain.InternalPort)
	}
	if created.Domain.DNSStatus != "PENDING" || created.Domain.TLSStatus != "PENDING" {
		t.Fatalf("statuses dns=%s tls=%s", created.Domain.DNSStatus, created.Domain.TLSStatus)
	}
	if created.Domain.Routing.Provider != "traefik" || created.Domain.Routing.Labels["traefik.enable"] != "true" {
		t.Fatalf("routing=%#v", created.Domain.Routing)
	}

	// Conflict: same hostname on another app in org.
	app2 := createApplicationNamed(t, srv, ownerTok, orgID, projectID, envID, "api-2", "api-2")
	conflict := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+app2+"/domains", body, ownerTok)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	var cerr struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decode(t, conflict, &cerr)
	if cerr.Error.Code != "DOMAIN_ALREADY_ASSIGNED" {
		t.Fatalf("code=%s", cerr.Error.Code)
	}

	viewerCreate := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/domains",
		mustJSON(map[string]any{"hostname": "other.example.com"}), viewerTok)
	if viewerCreate.Code != http.StatusForbidden {
		t.Fatalf("viewer create=%d", viewerCreate.Code)
	}

	list := doJSON(t, srv, http.MethodGet, "/api/v1/applications/"+appID+"/domains", nil, viewerTok)
	if list.Code != http.StatusOK {
		t.Fatalf("viewer list=%d", list.Code)
	}

	// Second domain on same app; promote to primary.
	second := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/domains",
		mustJSON(map[string]any{"hostname": "www.example.com", "isPrimary": true}), ownerTok)
	if second.Code != http.StatusCreated {
		t.Fatalf("second status=%d body=%s", second.Code, second.Body.String())
	}
	var s2 struct {
		Domain struct {
			ID        string `json:"id"`
			IsPrimary bool   `json:"isPrimary"`
		} `json:"domain"`
	}
	decode(t, second, &s2)
	if !s2.Domain.IsPrimary {
		t.Fatal("second should be primary")
	}
	var primaryCount int
	_ = pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM domains WHERE application_id = $1 AND is_primary AND deleted_at IS NULL`, appID).
		Scan(&primaryCount)
	if primaryCount != 1 {
		t.Fatalf("primaryCount=%d", primaryCount)
	}

	patch := doJSON(t, srv, http.MethodPatch, "/api/v1/domains/"+created.Domain.ID,
		mustJSON(map[string]any{"dnsStatus": "VALID", "tlsStatus": "ISSUING"}), ownerTok)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patch.Code, patch.Body.String())
	}

	del := doJSON(t, srv, http.MethodDelete, "/api/v1/domains/"+created.Domain.ID, nil, ownerTok)
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", del.Code)
	}

	// Soft-delete frees hostname for reassignment.
	reuse := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+app2+"/domains", body, ownerTok)
	if reuse.Code != http.StatusCreated {
		t.Fatalf("reuse status=%d body=%s", reuse.Code, reuse.Body.String())
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
		t.Fatalf("invite status=%d", rec.Code)
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
		t.Fatalf("accept status=%d", acc.Code)
	}
}

func createProject(t *testing.T, srv *server.Server, token, orgID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"organizationId": orgID, "name": "Platform", "slug": "platform"})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/projects", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	decode(t, rec, &out)
	return out.Project.ID
}

func createEnvironment(t *testing.T, srv *server.Server, token, projectID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": "Production", "slug": "production", "kind": "production"})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/projects/"+projectID+"/environments", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create env status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Environment struct {
			ID string `json:"id"`
		} `json:"environment"`
	}
	decode(t, rec, &out)
	return out.Environment.ID
}

func createApplication(t *testing.T, srv *server.Server, token, orgID, projectID, envID string) string {
	t.Helper()
	return createApplicationNamed(t, srv, token, orgID, projectID, envID, "api", "api")
}

func createApplicationNamed(t *testing.T, srv *server.Server, token, orgID, projectID, envID, name, slug string) string {
	t.Helper()
	srvBody, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "name": "edge-" + slug, "provider": "hetzner",
		"region": "fsn1", "hostname": "edge-" + slug + ".local", "architecture": "amd64",
	})
	srec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", srvBody, token)
	if srec.Code != http.StatusCreated {
		t.Fatalf("create server status=%d body=%s", srec.Code, srec.Body.String())
	}
	var sOut struct {
		Server struct {
			ID string `json:"id"`
		} `json:"server"`
	}
	decode(t, srec, &sOut)
	port := 8080
	body, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"name": name, "slug": slug, "type": "API", "targetServerId": sOut.Server.ID,
		"config": map[string]any{"sourceType": "image", "imageReference": "ghcr.io/example/api:1", "internalPort": port},
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/applications", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create app status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Application struct {
			ID string `json:"id"`
		} `json:"application"`
	}
	decode(t, rec, &out)
	return out.Application.ID
}

func doJSON(t *testing.T, srv *server.Server, method, path string, body []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
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
