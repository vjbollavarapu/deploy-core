package revisions_test

import (
	"bytes"
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

func TestRevisionSourceAuth(t *testing.T) {
	pool := testPool(t)
	var logs bytes.Buffer
	srv := sourceAuthServer(t, pool, &logs)

	const (
		original = "ghp_source_auth_original_token"
		rotated  = "ghp_source_auth_rotated_token"
		otherTok = "ghp_source_auth_other_org_token"
		inactive = "ghp_source_auth_inactive_token"
		gitlab   = "glpat_source_auth_should_not_return"
		bucket   = "bb_source_auth_should_not_return"
		generic  = "generic_source_auth_should_not_return"
	)
	tokens := []string{original, rotated, otherTok, inactive, gitlab, bucket, generic}

	ownerTok := register(t, srv, "owner-src-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "Source Org", "src-"+uuid.NewString()[:8])
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	serverID := createNamedServer(t, srv, ownerTok, orgID, "src-host")
	agentCred := registerAgent(t, srv, ownerTok, serverID)
	otherServer := createNamedServer(t, srv, ownerTok, orgID, "src-other")
	otherAgent := registerAgent(t, srv, ownerTok, otherServer)
	appID := createWorkerApp(t, srv, ownerTok, orgID, projectID, envID, serverID)

	otherOwner := register(t, srv, "other-src-"+uuid.NewString()+"@example.com", "password123", "Other")
	otherOrg := createOrg(t, srv, otherOwner, "Other Source Org", "os-"+uuid.NewString()[:8])

	githubID := createGitConnection(t, srv, ownerTok, orgID, "github", original)
	otherID := createGitConnection(t, srv, otherOwner, otherOrg, "github", otherTok)
	inactiveID := createGitConnection(t, srv, ownerTok, orgID, "github", inactive)
	gitlabID := createGitConnection(t, srv, ownerTok, orgID, "gitlab", gitlab)
	bitbucketID := createGitConnection(t, srv, ownerTok, orgID, "bitbucket", bucket)
	genericID := createGitConnection(t, srv, ownerTok, orgID, "generic", generic)

	publicRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 1, map[string]any{
		"sourceType": "git", "repositoryUrl": "https://github.com/acme/public", "gitBranch": "main",
	})
	publicRes := sourceAuth(t, srv, publicRev, agentCred)
	if publicRes.Code != http.StatusOK || publicRes.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("public status=%d cache=%q body=%s", publicRes.Code, publicRes.Header().Get("Cache-Control"), publicRes.Body.String())
	}
	publicBody := decodeMap(t, publicRes)
	if publicBody["scheme"] != "none" {
		t.Fatalf("public scheme=%v", publicBody["scheme"])
	}
	if _, ok := publicBody["password"]; ok || strings.Contains(publicRes.Body.String(), "password") {
		t.Fatalf("public response included credentials: %s", publicRes.Body.String())
	}

	privateRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 2, map[string]any{
		"sourceType": "git", "repositoryUrl": "https://github.com/acme/private", "gitBranch": "main",
		"gitConnectionId": githubID,
	})
	var before []byte
	if err := pool.QueryRow(context.Background(), `SELECT effective_config FROM revisions WHERE id = $1`, privateRev).Scan(&before); err != nil {
		t.Fatal(err)
	}
	authRes := sourceAuth(t, srv, privateRev, agentCred)
	if authRes.Code != http.StatusOK || authRes.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("auth status=%d cache=%q body=%s", authRes.Code, authRes.Header().Get("Cache-Control"), authRes.Body.String())
	}
	authBody := decodeMap(t, authRes)
	if authBody["scheme"] != "basic" || authBody["username"] != "x-access-token" || authBody["password"] != original {
		t.Fatal("github basic auth was not returned to the owning agent")
	}

	wrong := sourceAuth(t, srv, privateRev, otherAgent)
	if wrong.Code != http.StatusForbidden || wrong.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("wrong agent status=%d cache=%q", wrong.Code, wrong.Header().Get("Cache-Control"))
	}
	assertNoTokens(t, "wrong agent", wrong.Body.String(), tokens)

	user := sourceAuth(t, srv, privateRev, ownerTok)
	if user.Code != http.StatusUnauthorized || user.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("user status=%d cache=%q", user.Code, user.Header().Get("Cache-Control"))
	}
	assertNoTokens(t, "user", user.Body.String(), tokens)

	outsideRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 3, map[string]any{
		"sourceType": "git", "gitConnectionId": otherID,
	})
	outside := sourceAuth(t, srv, outsideRev, agentCred)
	if outside.Code != http.StatusForbidden {
		t.Fatalf("outside org status=%d body=%s", outside.Code, outside.Body.String())
	}
	assertNoTokens(t, "outside org", outside.Body.String(), tokens)

	inactiveRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 4, map[string]any{
		"sourceType": "git", "gitConnectionId": inactiveID,
	})
	for _, status := range []string{"disabled", "revoked", "error"} {
		patchConnection(t, srv, ownerTok, inactiveID, map[string]any{"status": status})
		got := sourceAuth(t, srv, inactiveRev, agentCred)
		if got.Code != http.StatusConflict {
			t.Fatalf("status %s returned %d body=%s", status, got.Code, got.Body.String())
		}
		assertNoTokens(t, status, got.Body.String(), tokens)
	}

	for _, item := range []struct {
		id       string
		provider string
	}{
		{gitlabID, "gitlab"},
		{bitbucketID, "bitbucket"},
		{genericID, "generic"},
	} {
		rev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, nextRevisionNumber(t, pool, appID), map[string]any{
			"sourceType": "git", "gitConnectionId": item.id,
		})
		got := sourceAuth(t, srv, rev, agentCred)
		if got.Code != http.StatusBadRequest || !strings.Contains(got.Body.String(), "unsupported git provider") {
			t.Fatalf("%s status=%d body=%s", item.provider, got.Code, got.Body.String())
		}
		assertNoTokens(t, item.provider, got.Body.String(), tokens)
	}

	patchConnection(t, srv, ownerTok, githubID, map[string]any{"accessToken": rotated})
	var after []byte
	if err := pool.QueryRow(context.Background(), `SELECT effective_config FROM revisions WHERE id = $1`, privateRev).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || strings.Contains(string(after), original) || strings.Contains(string(after), rotated) {
		t.Fatalf("historical revision changed: before=%s after=%s", before, after)
	}
	rotatedRes := sourceAuth(t, srv, privateRev, agentCred)
	if rotatedRes.Code != http.StatusOK {
		t.Fatalf("rotated status=%d body=%s", rotatedRes.Code, rotatedRes.Body.String())
	}
	rotatedBody := decodeMap(t, rotatedRes)
	if rotatedBody["password"] != rotated || rotatedBody["username"] != "x-access-token" {
		t.Fatal("rotation did not resolve the current credential")
	}
	if strings.Contains(rotatedRes.Body.String(), original) {
		t.Fatal("rotated response included the previous token")
	}

	var stored string
	if err := pool.QueryRow(context.Background(), `
		SELECT effective_config::text || variable_snapshot::text || secret_refs::text
		     || COALESCE((SELECT string_agg(message || metadata::text, ' ') FROM deployment_events WHERE organization_id = $1::uuid), '')
		     || COALESCE((SELECT string_agg(payload::text || COALESCE(error_message, '') || COALESCE(result::text, ''), ' ') FROM agent_commands WHERE organization_id = $1::uuid), '')
		     || COALESCE((SELECT string_agg(COALESCE(before_metadata::text, '') || COALESCE(after_metadata::text, ''), ' ') FROM audit_logs WHERE organization_id = $1::uuid), '')
		FROM revisions WHERE id = $2`, orgID, privateRev).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	assertNoTokens(t, "stored records", stored, tokens)
	assertNoTokens(t, "api logs", logs.String(), tokens)
}

func sourceAuthServer(t *testing.T, pool *pgxpool.Pool, logs io.Writer) *server.Server {
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
	return server.New(cfg, slog.New(slog.NewTextHandler(logs, nil)), pool)
}

func createGitConnection(t *testing.T, srv *server.Server, token, orgID, provider, accessToken string) string {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/git/connections", mustJSON(map[string]any{
		"organizationId": orgID,
		"provider":       provider,
		"accountLogin":   provider + "-account",
		"displayName":    provider,
		"accessToken":    accessToken,
	}), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s connection status=%d body=%s", provider, rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), accessToken) {
		t.Fatalf("%s connection response echoed the access token", provider)
	}
	var out struct {
		Connection struct {
			ID string `json:"id"`
		} `json:"connection"`
	}
	decode(t, rec, &out)
	return out.Connection.ID
}

func patchConnection(t *testing.T, srv *server.Server, token, connectionID string, body map[string]any) {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPatch, "/api/v1/integrations/git/connections/"+connectionID, mustJSON(body), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch connection status=%d body=%s", rec.Code, rec.Body.String())
	}
	if tokenValue, ok := body["accessToken"].(string); ok && strings.Contains(rec.Body.String(), tokenValue) {
		t.Fatal("connection update echoed the access token")
	}
}

func insertSourceRevision(t *testing.T, pool *pgxpool.Pool, orgID, appID, envID, serverID string, number int, cfg map[string]any) string {
	t.Helper()
	depID := uuid.New()
	revID := uuid.New()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO deployments (id, organization_id, application_id, environment_id, server_id, status, trigger)
		VALUES ($1, $2, $3, $4, $5, 'QUEUED', 'manual')`,
		depID, orgID, appID, envID, serverID); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO revisions (
			id, organization_id, application_id, deployment_id, revision_number, status, effective_config
		) VALUES ($1, $2, $3, $4, $5, 'READY', $6::jsonb)`,
		revID, orgID, appID, depID, number, raw); err != nil {
		t.Fatal(err)
	}
	return revID.String()
}

func nextRevisionNumber(t *testing.T, pool *pgxpool.Pool, appID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(MAX(revision_number), 0) + 1 FROM revisions WHERE application_id = $1`, appID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func sourceAuth(t *testing.T, srv *server.Server, revisionID, credential string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSON(t, srv, http.MethodGet, "/api/v1/agents/revisions/"+revisionID+"/source-auth", nil, credential)
}

func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertNoTokens(t *testing.T, label, raw string, tokens []string) {
	t.Helper()
	for _, token := range tokens {
		if token != "" && strings.Contains(raw, token) {
			t.Fatalf("%s contained a git credential", label)
		}
	}
}
