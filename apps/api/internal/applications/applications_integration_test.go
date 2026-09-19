package applications_test

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
		url = "postgres://localhost/deploycore_b9_test?sslmode=disable"
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
		DELETE FROM deployments;
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

func TestApplicationLifecycle(t *testing.T) {
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

	ownerTok := register(t, srv, "owner-b9@example.com", "password123", "Owner")
	viewerTok := register(t, srv, "viewer-b9@example.com", "password123", "Viewer")
	orgID := createOrg(t, srv, ownerTok, "B9 Org", "b9-org")
	inviteViewer(t, srv, ownerTok, viewerTok, orgID, "viewer-b9@example.com")

	projectID := createProject(t, srv, ownerTok, orgID, "Platform", "platform")
	envID := createEnvironment(t, srv, ownerTok, projectID, "Production", "production")
	serverID := createServer(t, srv, ownerTok, orgID, "edge-1")

	port := 8080
	repoURL := "https://github.com/example/api.git"
	branch := "main"
	createBody, _ := json.Marshal(map[string]any{
		"organizationId": orgID,
		"projectId":      projectID,
		"environmentId":  envID,
		"name":           "API Service",
		"slug":           "api-service",
		"type":           "API",
		"targetServerId": serverID,
		"config": map[string]any{
			"sourceType":     "git",
			"repositoryUrl":  repoURL,
			"gitBranch":      branch,
			"dockerfilePath": "Dockerfile",
			"internalPort":   port,
			"healthCheck":    map[string]any{"path": "/health"},
		},
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/applications", createBody, ownerTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Application struct {
			ID     string `json:"id"`
			Slug   string `json:"slug"`
			Status string `json:"status"`
			Config struct {
				Version    int    `json:"version"`
				SourceType string `json:"sourceType"`
			} `json:"config"`
		} `json:"application"`
	}
	decode(t, rec, &created)
	appID := created.Application.ID
	if created.Application.Slug != "api-service" || created.Application.Config.Version != 1 {
		t.Fatalf("created=%#v", created.Application)
	}
	if created.Application.Status != "draft" {
		t.Fatalf("status=%s", created.Application.Status)
	}

	// Duplicate slug conflict.
	dup := doJSON(t, srv, http.MethodPost, "/api/v1/applications", createBody, ownerTok)
	if dup.Code != http.StatusConflict {
		t.Fatalf("dup slug status=%d", dup.Code)
	}

	// Source-type validation: image source without imageReference.
	badImg, _ := json.Marshal(map[string]any{
		"environmentId":  envID,
		"name":           "Bad Image",
		"type":           "DOCKER_IMAGE",
		"targetServerId": serverID,
		"config":         map[string]any{"sourceType": "image"},
	})
	bad := doJSON(t, srv, http.MethodPost, "/api/v1/applications", badImg, ownerTok)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad image status=%d body=%s", bad.Code, bad.Body.String())
	}

	// API without internalPort.
	badPort, _ := json.Marshal(map[string]any{
		"environmentId":  envID,
		"name":           "No Port",
		"type":           "API",
		"targetServerId": serverID,
		"config": map[string]any{
			"sourceType":    "git",
			"repositoryUrl": repoURL,
			"gitBranch":     branch,
		},
	})
	noPort := doJSON(t, srv, http.MethodPost, "/api/v1/applications", badPort, ownerTok)
	if noPort.Code != http.StatusBadRequest {
		t.Fatalf("no port status=%d body=%s", noPort.Code, noPort.Body.String())
	}

	// Viewer can list/read, cannot create.
	list := doJSON(t, srv, http.MethodGet, "/api/v1/applications?organizationId="+orgID+"&environmentId="+envID, nil, viewerTok)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d", list.Code)
	}
	createDenied := doJSON(t, srv, http.MethodPost, "/api/v1/applications", createBody, viewerTok)
	if createDenied.Code != http.StatusForbidden {
		t.Fatalf("viewer create status=%d", createDenied.Code)
	}

	// Patch config creates a new version.
	imgRef := "ghcr.io/example/api:1.2.3"
	patch, _ := json.Marshal(map[string]any{
		"name": "API Service v2",
		"config": map[string]any{
			"sourceType":     "image",
			"imageReference": imgRef,
			"internalPort":   8080,
			"restartPolicy":  "always",
		},
	})
	// API type with image source is allowed (not DOCKER_IMAGE-only).
	patched := doJSON(t, srv, http.MethodPatch, "/api/v1/applications/"+appID, patch, ownerTok)
	if patched.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patched.Code, patched.Body.String())
	}
	var patchBody struct {
		Application struct {
			Name   string `json:"name"`
			Config struct {
				Version        int     `json:"version"`
				SourceType     string  `json:"sourceType"`
				ImageReference *string `json:"imageReference"`
			} `json:"config"`
		} `json:"application"`
	}
	decode(t, patched, &patchBody)
	if patchBody.Application.Name != "API Service v2" || patchBody.Application.Config.Version != 2 {
		t.Fatalf("patched=%#v", patchBody.Application)
	}
	if patchBody.Application.Config.SourceType != "image" || patchBody.Application.Config.ImageReference == nil || *patchBody.Application.Config.ImageReference != imgRef {
		t.Fatalf("config=%#v", patchBody.Application.Config)
	}

	// Soft-delete slug reuse.
	del := doJSON(t, srv, http.MethodDelete, "/api/v1/applications/"+appID, nil, ownerTok)
	if del.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", del.Code, del.Body.String())
	}
	gone := doJSON(t, srv, http.MethodGet, "/api/v1/applications/"+appID, nil, ownerTok)
	if gone.Code != http.StatusNotFound {
		t.Fatalf("get deleted status=%d", gone.Code)
	}
	again := doJSON(t, srv, http.MethodPost, "/api/v1/applications", createBody, ownerTok)
	if again.Code != http.StatusCreated {
		t.Fatalf("recreate status=%d body=%s", again.Code, again.Body.String())
	}
	var againBody struct {
		Application struct {
			ID string `json:"id"`
		} `json:"application"`
	}
	decode(t, again, &againBody)

	// Delete blocked while deployment in progress.
	_, err := pool.Exec(context.Background(), `
		INSERT INTO deployments (organization_id, application_id, environment_id, status)
		VALUES ($1, $2, $3, 'PENDING')`, orgID, againBody.Application.ID, envID)
	if err != nil {
		t.Fatalf("seed deployment: %v", err)
	}
	blocked := doJSON(t, srv, http.MethodDelete, "/api/v1/applications/"+againBody.Application.ID, nil, ownerTok)
	if blocked.Code != http.StatusConflict {
		t.Fatalf("delete with deployment status=%d body=%s", blocked.Code, blocked.Body.String())
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

func createProject(t *testing.T, srv *server.Server, token, orgID, name, slug string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"organizationId": orgID, "name": name, "slug": slug})
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

func createEnvironment(t *testing.T, srv *server.Server, token, projectID, name, slug string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name, "slug": slug, "kind": "production"})
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

func createServer(t *testing.T, srv *server.Server, token, orgID, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"organizationId": orgID,
		"name":           name,
		"provider":       "hetzner",
		"region":         "fsn1",
		"hostname":       name + ".local",
		"architecture":   "amd64",
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create server status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Server struct {
			ID string `json:"id"`
		} `json:"server"`
	}
	decode(t, rec, &out)
	return out.Server.ID
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
