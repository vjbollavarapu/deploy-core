package deployments_test

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
	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
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
		url = "postgres://localhost/deploycore_b11_test?sslmode=disable"
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
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b11-deployments"))
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

func TestDeploymentCreateQueueCancelIdempotency(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b11@example.com", "password123", "Owner")
	viewerTok := register(t, srv, "viewer-b11@example.com", "password123", "Viewer")
	orgID := createOrg(t, srv, ownerTok, "B11 Org", "b11-org")
	inviteViewer(t, srv, ownerTok, viewerTok, orgID, "viewer-b11@example.com")
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	appID := createApplication(t, srv, ownerTok, orgID, projectID, envID)

	body, _ := json.Marshal(map[string]any{"trigger": "manual", "idempotencyKey": "deploy-1"})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/deployments", body, ownerTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Deployment struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Events []struct {
				FromStatus *string `json:"fromStatus"`
				ToStatus   string  `json:"toStatus"`
			} `json:"events"`
		} `json:"deployment"`
	}
	decode(t, rec, &created)
	if created.Deployment.Status != deployments.StatusQueued {
		t.Fatalf("status=%s want QUEUED", created.Deployment.Status)
	}
	if len(created.Deployment.Events) < 2 {
		t.Fatalf("events=%d", len(created.Deployment.Events))
	}
	deployID := created.Deployment.ID

	// Job enqueued for later workers (B12).
	var jobType, jobStatus string
	err := pool.QueryRow(context.Background(), `
		SELECT type, status FROM jobs
		WHERE related_resource_type = 'deployment' AND related_resource_id = $1`, deployID).
		Scan(&jobType, &jobStatus)
	if err != nil {
		t.Fatalf("job: %v", err)
	}
	if jobType != "DEPLOYMENT_EXECUTION" || jobStatus != "queued" {
		t.Fatalf("job type=%s status=%s", jobType, jobStatus)
	}

	// Idempotent replay.
	again := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/deployments", body, ownerTok)
	if again.Code != http.StatusCreated {
		t.Fatalf("idempotent status=%d", again.Code)
	}
	var againBody struct {
		Deployment struct {
			ID string `json:"id"`
		} `json:"deployment"`
	}
	decode(t, again, &againBody)
	if againBody.Deployment.ID != deployID {
		t.Fatalf("idempotent id=%s want %s", againBody.Deployment.ID, deployID)
	}

	// Concurrent create without key blocked.
	other, _ := json.Marshal(map[string]string{"trigger": "api"})
	blocked := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/deployments", other, ownerTok)
	if blocked.Code != http.StatusConflict {
		t.Fatalf("concurrent create status=%d body=%s", blocked.Code, blocked.Body.String())
	}

	// Viewer can read, cannot create.
	list := doJSON(t, srv, http.MethodGet, "/api/v1/deployments?organizationId="+orgID+"&applicationId="+appID, nil, viewerTok)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d", list.Code)
	}
	viewerCreate := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/deployments", other, viewerTok)
	if viewerCreate.Code != http.StatusForbidden {
		t.Fatalf("viewer create status=%d", viewerCreate.Code)
	}

	// Cancel.
	cancel := doJSON(t, srv, http.MethodPost, "/api/v1/deployments/"+deployID+"/cancel", nil, ownerTok)
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", cancel.Code, cancel.Body.String())
	}
	var cancelled struct {
		Deployment struct {
			Status     string  `json:"status"`
			FinishedAt *string `json:"finishedAt"`
		} `json:"deployment"`
	}
	decode(t, cancel, &cancelled)
	if cancelled.Deployment.Status != deployments.StatusCancelled || cancelled.Deployment.FinishedAt == nil {
		t.Fatalf("cancelled=%#v", cancelled.Deployment)
	}

	// Terminal cancel rejected.
	againCancel := doJSON(t, srv, http.MethodPost, "/api/v1/deployments/"+deployID+"/cancel", nil, ownerTok)
	if againCancel.Code != http.StatusConflict {
		t.Fatalf("re-cancel status=%d", againCancel.Code)
	}

	// New deployment after cancel; advance transitions persist events.
	create2 := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/deployments", other, ownerTok)
	if create2.Code != http.StatusCreated {
		t.Fatalf("create2 status=%d body=%s", create2.Code, create2.Body.String())
	}
	var c2 struct {
		Deployment struct {
			ID string `json:"id"`
		} `json:"deployment"`
	}
	decode(t, create2, &c2)

	repo := deployments.NewPostgresRepository(pool)
	svc := deployments.NewService(repo, rbac.NewAuthorizer(pool), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	id := uuid.MustParse(c2.Deployment.ID)

	advanced, err := svc.Advance(context.Background(), id, deployments.StatusQueued, deployments.TransitionInput{
		ToStatus: deployments.StatusPreparing, Message: "worker acquired",
	})
	if err != nil {
		t.Fatalf("advance: %v", err)
	}
	if advanced.Status != deployments.StatusPreparing {
		t.Fatalf("status=%s", advanced.Status)
	}
	_, err = svc.Advance(context.Background(), id, deployments.StatusQueued, deployments.TransitionInput{
		ToStatus: deployments.StatusPreparing, Message: "stale",
	})
	if err == nil {
		t.Fatal("expected concurrent/stale transition conflict")
	}

	_, err = svc.Advance(context.Background(), id, deployments.StatusPreparing, deployments.TransitionInput{
		ToStatus: deployments.StatusRunning, Message: "skip",
	})
	if err == nil {
		t.Fatal("expected invalid transition")
	}
}

func TestRollbackEndpoint(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b15@example.com", "password123", "Owner")
	viewerTok := register(t, srv, "viewer-b15@example.com", "password123", "Viewer")
	orgID := createOrg(t, srv, ownerTok, "B15 Org", "b15-org")
	inviteViewer(t, srv, ownerTok, viewerTok, orgID, "viewer-b15@example.com")
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	appID := createApplication(t, srv, ownerTok, orgID, projectID, envID)

	ctx := context.Background()
	activeRev := uuid.New()
	inactiveRev := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO revisions (id, organization_id, application_id, revision_number, status, image_digest, image_tag)
		VALUES
		  ($1, $2, $3, 1, 'INACTIVE', 'sha256:old', 'v1'),
		  ($4, $2, $3, 2, 'ACTIVE', 'sha256:new', 'v2')`,
		inactiveRev, orgID, appID, activeRev)
	if err != nil {
		t.Fatalf("seed revisions: %v", err)
	}

	viewerRB := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/rollback",
		mustJSON(map[string]string{"targetRevisionId": inactiveRev.String()}), viewerTok)
	if viewerRB.Code != http.StatusForbidden {
		t.Fatalf("viewer rollback status=%d", viewerRB.Code)
	}

	activeRB := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/rollback",
		mustJSON(map[string]string{"targetRevisionId": activeRev.String()}), ownerTok)
	if activeRB.Code != http.StatusConflict {
		t.Fatalf("active target status=%d body=%s", activeRB.Code, activeRB.Body.String())
	}

	rec := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/rollback",
		mustJSON(map[string]string{"targetRevisionId": inactiveRev.String()}), ownerTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("rollback status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Deployment struct {
			ID               string  `json:"id"`
			Status           string  `json:"status"`
			Trigger          string  `json:"trigger"`
			TargetRevisionID *string `json:"targetRevisionId"`
		} `json:"deployment"`
	}
	decode(t, rec, &out)
	if out.Deployment.Status != deployments.StatusQueued || out.Deployment.Trigger != deployments.TriggerRollback {
		t.Fatalf("deployment=%#v", out.Deployment)
	}
	if out.Deployment.TargetRevisionID == nil || *out.Deployment.TargetRevisionID != inactiveRev.String() {
		t.Fatalf("target=%v", out.Deployment.TargetRevisionID)
	}

	var action string
	err = pool.QueryRow(ctx, `
		SELECT action FROM audit_logs
		WHERE action = 'deployment.rollback' AND resource_id = $1
		ORDER BY created_at DESC LIMIT 1`, out.Deployment.ID).Scan(&action)
	if err != nil || action != "deployment.rollback" {
		t.Fatalf("audit: %v action=%s", err, action)
	}

	missing := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/rollback",
		mustJSON(map[string]string{"targetRevisionId": uuid.New().String()}), ownerTok)
	if missing.Code != http.StatusConflict && missing.Code != http.StatusNotFound {
		// in-progress deployment exists → conflict; if cancelled would be not found
		t.Fatalf("second rollback status=%d body=%s", missing.Code, missing.Body.String())
	}
}

func TestDeploymentHTTPValidationAndErrorCodes(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)
	ownerTok := register(t, srv, "owner-val@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "Val Org", "val-org")
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	appID := createApplication(t, srv, ownerTok, orgID, projectID, envID)

	// Invalid application id → validation
	badApp := doJSON(t, srv, http.MethodPost, "/api/v1/applications/not-a-uuid/deployments",
		mustJSON(map[string]any{}), ownerTok)
	if badApp.Code != http.StatusBadRequest {
		t.Fatalf("bad app id status=%d", badApp.Code)
	}

	// Missing application → not found (valid UUID)
	missing := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+uuid.New().String()+"/deployments",
		mustJSON(map[string]any{}), ownerTok)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing app status=%d body=%s", missing.Code, missing.Body.String())
	}
	var missingEnv struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decode(t, missing, &missingEnv)
	if missingEnv.Error.Code != "APPLICATION_NOT_FOUND" {
		t.Fatalf("code=%s", missingEnv.Error.Code)
	}

	// Invalid rollback body
	badRB := doJSON(t, srv, http.MethodPost, "/api/v1/applications/"+appID+"/rollback",
		mustJSON(map[string]any{"targetRevisionId": "nope"}), ownerTok)
	if badRB.Code != http.StatusBadRequest {
		t.Fatalf("bad rollback status=%d body=%s", badRB.Code, badRB.Body.String())
	}

	// Unauthenticated
	unauth := doJSON(t, srv, http.MethodGet, "/api/v1/deployments?organizationId="+orgID, nil, "")
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", unauth.Code)
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
	srvBody, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "name": "edge-b11", "provider": "hetzner",
		"region": "fsn1", "hostname": "edge-b11.local", "architecture": "amd64",
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
