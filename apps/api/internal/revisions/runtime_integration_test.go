package revisions_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRevisionRuntimeBootstrap(t *testing.T) {
	pool := testPool(t)
	srv := runtimeTestServer(t, pool)

	ownerTok := register(t, srv, "owner-rt-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "Runtime Org", "rt-"+uuid.NewString()[:8])
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	serverID := createNamedServer(t, srv, ownerTok, orgID, "rt-host")
	agentCred := registerAgent(t, srv, ownerTok, serverID)
	otherServer := createNamedServer(t, srv, ownerTok, orgID, "rt-other")
	otherAgent := registerAgent(t, srv, ownerTok, otherServer)

	otherOwner := register(t, srv, "other-rt-"+uuid.NewString()+"@example.com", "password123", "Other")
	otherOrg := createOrg(t, srv, otherOwner, "Other Org", "ot-"+uuid.NewString()[:8])
	otherOrgServer := createNamedServer(t, srv, otherOwner, otherOrg, "ot-host")
	otherOrgAgent := registerAgent(t, srv, otherOwner, otherOrgServer)

	appID := createWorkerApp(t, srv, ownerTok, orgID, projectID, envID, serverID)
	createVariable(t, srv, ownerTok, orgID, appID, "REDIS_URL", "redis://live:6379/0")
	secretID := createSecret(t, srv, ownerTok, orgID, appID, "APP_SECRET", "version-one-secret")
	rotate := doJSON(t, srv, http.MethodPatch, "/api/v1/secrets/"+secretID, mustJSON(map[string]string{"value": "version-two-secret"}), ownerTok)
	if rotate.Code != http.StatusOK {
		t.Fatalf("rotate status=%d body=%s", rotate.Code, rotate.Body.String())
	}

	revID := uuid.New()
	depID := uuid.New()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO deployments (id, organization_id, application_id, environment_id, server_id, status, trigger)
		VALUES ($1, $2, $3, $4, $5, 'QUEUED', 'manual')`,
		depID, orgID, appID, envID, serverID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO revisions (
			id, organization_id, application_id, deployment_id, revision_number, status,
			variable_snapshot, secret_refs
		) VALUES (
			$1, $2, $3, $4, 1, 'READY',
			$5::jsonb, $6::jsonb
		)`, revID, orgID, appID, depID,
		`{"REDIS_URL":{"value":"redis://redis:6379/1","scope":"APPLICATION"}}`,
		`[{"name":"APP_SECRET","ref":{"version":1,"scope":"APPLICATION"}}]`,
	); err != nil {
		t.Fatal(err)
	}

	boot := doJSON(t, srv, http.MethodGet, "/api/v1/agents/revisions/"+revID.String()+"/runtime", nil, agentCred)
	if boot.Code != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%s", boot.Code, boot.Body.String())
	}
	env := decodeEnv(t, boot)
	if env["REDIS_URL"] != "redis://redis:6379/1" {
		t.Fatalf("REDIS_URL = %q, want snapshot value", env["REDIS_URL"])
	}
	if env["APP_SECRET"] != "version-one-secret" {
		t.Fatalf("APP_SECRET = %q, want snapshotted version", env["APP_SECRET"])
	}

	wrongServer := doJSON(t, srv, http.MethodGet, "/api/v1/agents/revisions/"+revID.String()+"/runtime", nil, otherAgent)
	if wrongServer.Code != http.StatusForbidden {
		t.Fatalf("wrong server status=%d body=%s", wrongServer.Code, wrongServer.Body.String())
	}
	wrongOrg := doJSON(t, srv, http.MethodGet, "/api/v1/agents/revisions/"+revID.String()+"/runtime", nil, otherOrgAgent)
	if wrongOrg.Code != http.StatusForbidden {
		t.Fatalf("wrong org status=%d body=%s", wrongOrg.Code, wrongOrg.Body.String())
	}
	user := doJSON(t, srv, http.MethodGet, "/api/v1/agents/revisions/"+revID.String()+"/runtime", nil, ownerTok)
	if user.Code != http.StatusUnauthorized {
		t.Fatalf("user token status=%d body=%s", user.Code, user.Body.String())
	}

	operator := doJSON(t, srv, http.MethodGet, "/api/v1/revisions/"+revID.String(), nil, ownerTok)
	if operator.Code != http.StatusOK {
		t.Fatalf("operator get status=%d body=%s", operator.Code, operator.Body.String())
	}
	if strings.Contains(operator.Body.String(), "version-one-secret") || strings.Contains(operator.Body.String(), "version-two-secret") {
		t.Fatalf("operator revision response leaked secret plaintext: %s", operator.Body.String())
	}

	if _, err := pool.Exec(ctx, `
		UPDATE revisions SET secret_refs = $2::jsonb WHERE id = $1`,
		revID, `[{"name":"APP_SECRET","ref":{"version":9,"scope":"APPLICATION"}}]`); err != nil {
		t.Fatal(err)
	}
	missing := doJSON(t, srv, http.MethodGet, "/api/v1/agents/revisions/"+revID.String()+"/runtime", nil, agentCred)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing version status=%d body=%s", missing.Code, missing.Body.String())
	}
	if strings.Contains(missing.Body.String(), "version-one-secret") || strings.Contains(missing.Body.String(), "version-two-secret") {
		t.Fatalf("missing-version error leaked plaintext: %s", missing.Body.String())
	}
}

func runtimeTestServer(t *testing.T, pool *pgxpool.Pool) *server.Server {
	t.Helper()
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-revision-runtime"))
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
		AgentRegistrationTTL:  15 * time.Minute,
		AgentHeartbeatRetain:  50,
		JobWorkerEnabled:      false,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return server.New(cfg, log, pool)
}

func createNamedServer(t *testing.T, srv *server.Server, token, orgID, name string) string {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", mustJSON(map[string]any{
		"organizationId": orgID, "name": name, "provider": "hetzner",
		"region": "fsn1", "hostname": name + ".local", "architecture": "amd64",
	}), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("server status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Server struct {
			ID string `json:"id"`
		} `json:"server"`
	}
	decode(t, rec, &out)
	return out.Server.ID
}

func registerAgent(t *testing.T, srv *server.Server, ownerTok, serverID string) string {
	t.Helper()
	tokRec := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/registration-token", nil, ownerTok)
	if tokRec.Code != http.StatusCreated {
		t.Fatalf("reg token status=%d body=%s", tokRec.Code, tokRec.Body.String())
	}
	var tokOut struct {
		RegistrationToken struct {
			Token string `json:"token"`
		} `json:"registrationToken"`
	}
	decode(t, tokRec, &tokOut)
	reg := doJSON(t, srv, http.MethodPost, "/api/v1/agents/register", mustJSON(map[string]any{
		"registrationToken": tokOut.RegistrationToken.Token, "agentVersion": "1.0.0",
	}), "")
	if reg.Code != http.StatusCreated {
		t.Fatalf("register agent status=%d body=%s", reg.Code, reg.Body.String())
	}
	var out struct {
		Agent struct {
			Credential string `json:"credential"`
		} `json:"agent"`
	}
	decode(t, reg, &out)
	return out.Agent.Credential
}

func createWorkerApp(t *testing.T, srv *server.Server, token, orgID, projectID, envID, serverID string) string {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/applications", mustJSON(map[string]any{
		"organizationId": orgID,
		"projectId":      projectID,
		"environmentId":  envID,
		"name":           "Redis",
		"slug":           "redis",
		"type":           "WORKER",
		"targetServerId": serverID,
		"config": map[string]any{
			"sourceType":     "image",
			"imageReference": "redis:7-alpine",
		},
	}), token)
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

func createVariable(t *testing.T, srv *server.Server, token, orgID, appID, key, value string) {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/variables", mustJSON(map[string]any{
		"organizationId": orgID,
		"scope":          "APPLICATION",
		"applicationId":  appID,
		"key":            key,
		"value":          value,
	}), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create variable status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func createSecret(t *testing.T, srv *server.Server, token, orgID, appID, name, value string) string {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/secrets", mustJSON(map[string]any{
		"organizationId": orgID,
		"scope":          "APPLICATION",
		"applicationId":  appID,
		"name":           name,
		"value":          value,
	}), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create secret status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Secret struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		} `json:"secret"`
	}
	decode(t, rec, &out)
	if out.Secret.Version != 1 {
		t.Fatalf("secret version=%d", out.Secret.Version)
	}
	return out.Secret.ID
}

func decodeEnv(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var body struct {
		Env []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"env"`
	}
	decode(t, rec, &body)
	out := map[string]string{}
	for _, item := range body.Env {
		out[item.Name] = item.Value
	}
	return out
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
