package revisions_test

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
		url = "postgres://localhost/deploycore_b14_test?sslmode=disable"
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
		DELETE FROM deployment_events;
		DELETE FROM jobs;
		UPDATE deployments SET active_revision_id = NULL, target_revision_id = NULL;
		DELETE FROM revisions;
		DELETE FROM deployments;
		DELETE FROM application_configs;
		DELETE FROM applications;
		DELETE FROM environments;
		DELETE FROM projects;
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

func TestRevisionListAndGetImmutable(t *testing.T) {
	pool := testPool(t)
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b14-revisions!!"))
	cfg := config.Config{
		Env:                       "test",
		AuthTokenSecret:           "test-secret-0123456789abcdef01234567",
		AccessTokenTTL:            time.Minute,
		RefreshTokenTTL:           time.Hour,
		AuthRateLimitPerMin:       1000,
		CORSAllowedOrigins:        []string{"*"},
		AuthMinPasswordLength:     8,
		SecretsPlatformKey:        key,
		SecretsKeyID:              "platform:v1",
		JobWorkerEnabled:          false,
		OrchestratorSimulateAgent: true,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := server.New(cfg, log, pool)

	ownerTok := register(t, srv, "owner-b14@example.com", "password123", "Owner")
	viewerTok := register(t, srv, "viewer-b14@example.com", "password123", "Viewer")
	orgID := createOrg(t, srv, ownerTok, "B14 Org", "b14-org")
	inviteViewer(t, srv, ownerTok, viewerTok, orgID, "viewer-b14@example.com")
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	appID := createApplication(t, srv, ownerTok, orgID, projectID, envID)

	// Seed immutable revisions (as orchestrator would).
	var rev1, rev2 string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO revisions (
			organization_id, application_id, revision_number, status,
			image_digest, image_tag, effective_config, variable_snapshot, secret_refs
		) VALUES ($1, $2, 1, 'INACTIVE', 'sha256:aaa', 'v1', '{"sourceType":"image"}', '{"LOG_LEVEL":{"value":"info"}}', '[]')
		RETURNING id`, orgID, appID).Scan(&rev1)
	if err != nil {
		t.Fatalf("seed rev1: %v", err)
	}
	err = pool.QueryRow(context.Background(), `
		INSERT INTO revisions (
			organization_id, application_id, revision_number, status,
			image_digest, image_tag, effective_config, variable_snapshot, secret_refs
		) VALUES ($1, $2, 2, 'ACTIVE', 'sha256:bbb', 'v2', '{"sourceType":"image","internalPort":8080}', '{}', '[{"name":"DB_PASSWORD"}]')
		RETURNING id`, orgID, appID).Scan(&rev2)
	if err != nil {
		t.Fatalf("seed rev2: %v", err)
	}

	list := doJSON(t, srv, http.MethodGet, "/api/v1/applications/"+appID+"/revisions", nil, viewerTok)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var page struct {
		Items []struct {
			ID             string `json:"id"`
			RevisionNumber int    `json:"revisionNumber"`
			Status         string `json:"status"`
		} `json:"items"`
		TotalCount *int64 `json:"totalCount"`
	}
	decode(t, list, &page)
	if page.TotalCount == nil || *page.TotalCount != 2 || len(page.Items) != 2 {
		t.Fatalf("page=%#v", page)
	}
	if page.Items[0].RevisionNumber != 2 || page.Items[0].Status != "ACTIVE" {
		t.Fatalf("newest=%#v", page.Items[0])
	}

	activeOnly := doJSON(t, srv, http.MethodGet, "/api/v1/applications/"+appID+"/revisions?status=ACTIVE", nil, ownerTok)
	if activeOnly.Code != http.StatusOK {
		t.Fatalf("filter status=%d", activeOnly.Code)
	}
	decode(t, activeOnly, &page)
	if len(page.Items) != 1 || page.Items[0].ID != rev2 {
		t.Fatalf("active filter=%#v", page.Items)
	}

	get := doJSON(t, srv, http.MethodGet, "/api/v1/revisions/"+rev2, nil, viewerTok)
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}
	var body struct {
		Revision struct {
			ID              string         `json:"id"`
			ImageDigest     *string        `json:"imageDigest"`
			EffectiveConfig map[string]any `json:"effectiveConfig"`
			SecretRefs      []any          `json:"secretRefs"`
		} `json:"revision"`
	}
	decode(t, get, &body)
	if body.Revision.ID != rev2 || body.Revision.ImageDigest == nil || *body.Revision.ImageDigest != "sha256:bbb" {
		t.Fatalf("get=%#v", body.Revision)
	}
	if body.Revision.EffectiveConfig["internalPort"] == nil {
		t.Fatalf("effectiveConfig=%#v", body.Revision.EffectiveConfig)
	}
	if len(body.Revision.SecretRefs) != 1 {
		t.Fatalf("secretRefs=%#v", body.Revision.SecretRefs)
	}

	// No mutation endpoint — PATCH should 404/405 (not mounted).
	patch := doJSON(t, srv, http.MethodPatch, "/api/v1/revisions/"+rev2, []byte(`{"status":"ARCHIVED"}`), ownerTok)
	if patch.Code != http.StatusMethodNotAllowed && patch.Code != http.StatusNotFound {
		t.Fatalf("unexpected mutate status=%d", patch.Code)
	}

	_ = rev1
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
		t.Fatalf("create project status=%d", rec.Code)
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
		t.Fatalf("create env status=%d", rec.Code)
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
	srvBody, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "name": "edge-b14", "provider": "hetzner",
		"region": "fsn1", "hostname": "edge-b14.local", "architecture": "amd64",
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
		"name": "api", "slug": "api", "type": "API", "targetServerId": sOut.Server.ID,
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
