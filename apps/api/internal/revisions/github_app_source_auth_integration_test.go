package revisions_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	sourceAuthInstallToken = "ghs_source_auth_installation_token"
	sourceAuthWebhook      = "source-auth-webhook-marker"
	sourceAuthPAT          = "ghp_source_auth_pat_unchanged"
)

func TestGitHubAppSourceAuth(t *testing.T) {
	pool := testPool(t)
	var logs bytes.Buffer
	fake := newSourceAuthGitHub(t)
	app := loadSourceAuthApp(t)
	srv := sourceAuthGitHubServer(t, pool, fake.URL(), &logs, app.loaded)
	plain := sourceAuthServer(t, pool, io.Discard)
	offline := sourceAuthGitHubServer(t, pool, "http://127.0.0.1:1", io.Discard, app.loaded)

	ownerTok := register(t, srv, "owner-gha-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "App Source Org", "gas-"+uuid.NewString()[:8])
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	serverID := createNamedServer(t, srv, ownerTok, orgID, "gha-host")
	agentCred := registerAgent(t, srv, ownerTok, serverID)
	otherServer := createNamedServer(t, srv, ownerTok, orgID, "gha-other")
	otherAgent := registerAgent(t, srv, ownerTok, otherServer)
	appID := createWorkerApp(t, srv, ownerTok, orgID, projectID, envID, serverID)
	otherOwner := register(t, srv, "other-gha-"+uuid.NewString()+"@example.com", "password123", "Other")
	otherOrg := createOrg(t, srv, otherOwner, "Other App Org", "gao-"+uuid.NewString()[:8])

	patID := createGitConnection(t, srv, ownerTok, orgID, "github", sourceAuthPAT)
	patRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 1, map[string]any{
		"sourceType": "git", "repositoryUrl": "https://github.com/acme/pat", "gitBranch": "main",
		"gitConnectionId": patID,
	})
	patRes := sourceAuth(t, srv, patRev, agentCred)
	patBody := decodeMap(t, patRes)
	if patRes.Code != http.StatusOK || patRes.Header().Get("Cache-Control") != "no-store" ||
		patBody["scheme"] != "basic" || patBody["username"] != "x-access-token" || patBody["password"] != sourceAuthPAT {
		t.Fatalf("pat source-auth status=%d cache=%q body=%s", patRes.Code, patRes.Header().Get("Cache-Control"), patRes.Body.String())
	}
	if fake.calls() != 0 {
		t.Fatal("PAT source-auth called GitHub")
	}

	publicRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 2, map[string]any{
		"sourceType": "git", "repositoryUrl": "https://github.com/acme/public", "gitBranch": "main",
	})
	publicRes := sourceAuth(t, srv, publicRev, agentCred)
	if publicRes.Code != http.StatusOK || decodeMap(t, publicRes)["scheme"] != "none" || fake.calls() != 0 {
		t.Fatalf("public git status=%d body=%s", publicRes.Code, publicRes.Body.String())
	}
	imageRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 3, map[string]any{
		"sourceType": "image", "imageReference": "redis:7-alpine",
	})
	imageRes := sourceAuth(t, srv, imageRev, agentCred)
	if imageRes.Code != http.StatusOK || decodeMap(t, imageRes)["scheme"] != "none" || strings.Contains(imageRes.Body.String(), "password") {
		t.Fatalf("image source-auth status=%d body=%s", imageRes.Code, imageRes.Body.String())
	}

	connID := insertGitHubAppConnection(t, pool, orgID, 4242)
	privateRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 4, map[string]any{
		"sourceType": "git", "repositoryUrl": "https://github.com/verified-octo/api", "gitBranch": "main",
		"gitConnectionId": connID,
	})
	var before []byte
	if err := pool.QueryRow(context.Background(), `SELECT effective_config FROM revisions WHERE id = $1`, privateRev).Scan(&before); err != nil {
		t.Fatal(err)
	}
	authRes := sourceAuth(t, srv, privateRev, agentCred)
	if authRes.Code != http.StatusOK || authRes.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("github app status=%d cache=%q body=%s", authRes.Code, authRes.Header().Get("Cache-Control"), authRes.Body.String())
	}
	authBody := decodeMap(t, authRes)
	if authBody["scheme"] != "basic" || authBody["username"] != "x-access-token" || authBody["password"] != sourceAuthInstallToken {
		t.Fatalf("github app credential=%v", authBody)
	}
	for key := range authBody {
		switch key {
		case "revisionId", "scheme", "username", "password":
		default:
			t.Fatalf("unexpected source-auth field %s", key)
		}
	}
	var after []byte
	if err := pool.QueryRow(context.Background(), `SELECT effective_config FROM revisions WHERE id = $1`, privateRev).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || strings.Contains(string(after), sourceAuthInstallToken) {
		t.Fatal("revision effective_config changed or stored the installation token")
	}
	assertGitHubAppCredentialColumnsNull(t, pool, connID)
	assertSourceAuthMaterialAbsent(t, pool, logs.String(), app.marker, fake.jwts())
	if fake.posts() < 1 || fake.gets() < 1 || fake.repoLists() != 0 {
		t.Fatalf("github calls gets=%d posts=%d lists=%d", fake.gets(), fake.posts(), fake.repoLists())
	}

	calls := fake.calls()
	wrongServer := sourceAuth(t, srv, privateRev, otherAgent)
	if wrongServer.Code != http.StatusForbidden || wrongServer.Header().Get("Cache-Control") != "no-store" || fake.calls() != calls {
		t.Fatalf("wrong server status=%d calls=%d body=%s", wrongServer.Code, fake.calls(), wrongServer.Body.String())
	}
	assertNoTokens(t, "wrong server", wrongServer.Body.String(), []string{sourceAuthInstallToken, sourceAuthPAT})

	user := sourceAuth(t, srv, privateRev, ownerTok)
	if user.Code != http.StatusUnauthorized || user.Header().Get("Cache-Control") != "no-store" || fake.calls() != calls {
		t.Fatalf("user status=%d", user.Code)
	}

	otherConn := insertGitHubAppConnection(t, pool, otherOrg, 4243)
	outsideRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 5, map[string]any{
		"sourceType": "git", "gitConnectionId": otherConn,
	})
	outside := sourceAuth(t, srv, outsideRev, agentCred)
	if outside.Code != http.StatusForbidden || fake.calls() != calls {
		t.Fatalf("wrong org status=%d calls=%d body=%s", outside.Code, fake.calls(), outside.Body.String())
	}

	if _, err := pool.Exec(context.Background(), `UPDATE git_connections SET status = 'disabled' WHERE id = $1`, connID); err != nil {
		t.Fatal(err)
	}
	inactive := sourceAuth(t, srv, privateRev, agentCred)
	if inactive.Code != http.StatusConflict || fake.calls() != calls || strings.Contains(inactive.Body.String(), sourceAuthInstallToken) {
		t.Fatalf("inactive status=%d calls=%d body=%s", inactive.Code, fake.calls(), inactive.Body.String())
	}
	if _, err := pool.Exec(context.Background(), `UPDATE git_connections SET status = 'active' WHERE id = $1`, connID); err != nil {
		t.Fatal(err)
	}

	restoreCredentialConstraint(t, pool)
	if _, err := pool.Exec(context.Background(), `ALTER TABLE git_connections DROP CONSTRAINT git_connections_credential_mode_check`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE git_connections SET installation_id = NULL WHERE id = $1`, connID); err != nil {
		t.Fatal(err)
	}
	missing := sourceAuth(t, srv, privateRev, agentCred)
	if missing.Code != http.StatusBadRequest || fake.calls() != calls || strings.Contains(missing.Body.String(), sourceAuthInstallToken) {
		t.Fatalf("missing installation status=%d calls=%d body=%s", missing.Code, fake.calls(), missing.Body.String())
	}
	var missingStatus string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM git_connections WHERE id = $1`, connID).Scan(&missingStatus); err != nil || missingStatus != "error" {
		t.Fatalf("missing installation status=%s err=%v", missingStatus, err)
	}
	if _, err := pool.Exec(context.Background(), `DELETE FROM git_connections WHERE id = $1`, connID); err != nil {
		t.Fatal(err)
	}

	unconfiguredID := insertGitHubAppConnection(t, pool, orgID, 4250)
	unconfiguredRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 6, map[string]any{
		"sourceType": "git", "gitConnectionId": unconfiguredID,
	})
	unconfigured := sourceAuth(t, plain, unconfiguredRev, agentCred)
	if unconfigured.Code != http.StatusServiceUnavailable || fake.calls() != calls || strings.Contains(unconfigured.Body.String(), app.marker) {
		t.Fatalf("unconfigured status=%d body=%s", unconfigured.Code, unconfigured.Body.String())
	}
	assertConnectionStatus(t, pool, unconfiguredID, "active")

	unauthorizedID := insertGitHubAppConnection(t, pool, orgID, 4251)
	unauthorizedRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 7, map[string]any{
		"sourceType": "git", "gitConnectionId": unauthorizedID,
	})
	fake.setTokenStatus(http.StatusUnauthorized)
	unauthorized := sourceAuth(t, srv, unauthorizedRev, agentCred)
	if unauthorized.Code != http.StatusBadGateway || unauthorized.Header().Get("Cache-Control") != "no-store" || strings.Contains(unauthorized.Body.String(), sourceAuthInstallToken) {
		t.Fatalf("github 401 status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
	assertConnectionStatus(t, pool, unauthorizedID, "active")

	missingInstallID := insertGitHubAppConnection(t, pool, orgID, 4252)
	missingInstallRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 8, map[string]any{
		"sourceType": "git", "gitConnectionId": missingInstallID,
	})
	fake.setTokenStatus(0)
	fake.setInstallStatus(http.StatusNotFound)
	notFound := sourceAuth(t, srv, missingInstallRev, agentCred)
	if notFound.Code != http.StatusNotFound || strings.Contains(notFound.Body.String(), sourceAuthInstallToken) {
		t.Fatalf("github 404 status=%d body=%s", notFound.Code, notFound.Body.String())
	}
	assertConnectionStatus(t, pool, missingInstallID, "error")
	afterNotFound := fake.calls()
	again := sourceAuth(t, srv, missingInstallRev, agentCred)
	if again.Code != http.StatusConflict || fake.calls() != afterNotFound {
		t.Fatalf("repeat after 404 status=%d calls before=%d after=%d", again.Code, afterNotFound, fake.calls())
	}

	limitedID := insertGitHubAppConnection(t, pool, orgID, 4253)
	limitedRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 9, map[string]any{
		"sourceType": "git", "gitConnectionId": limitedID,
	})
	fake.setInstallStatus(0)
	fake.setTokenStatus(http.StatusTooManyRequests)
	limited := sourceAuth(t, srv, limitedRev, agentCred)
	if limited.Code != http.StatusTooManyRequests || strings.Contains(limited.Body.String(), sourceAuthInstallToken) {
		t.Fatalf("github 429 status=%d body=%s", limited.Code, limited.Body.String())
	}
	assertConnectionStatus(t, pool, limitedID, "active")

	malformedID := insertGitHubAppConnection(t, pool, orgID, 4254)
	malformedRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 10, map[string]any{
		"sourceType": "git", "gitConnectionId": malformedID,
	})
	fake.setTokenStatus(0)
	fake.setMalformed(true)
	malformed := sourceAuth(t, srv, malformedRev, agentCred)
	if malformed.Code != http.StatusBadGateway || strings.Contains(malformed.Body.String(), "ghs_malformed_body") || strings.Contains(malformed.Body.String(), sourceAuthInstallToken) {
		t.Fatalf("malformed status=%d body=%s", malformed.Code, malformed.Body.String())
	}
	assertConnectionStatus(t, pool, malformedID, "active")
	fake.setMalformed(false)

	suspendedID := insertGitHubAppConnection(t, pool, orgID, 4255)
	suspendedRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 11, map[string]any{
		"sourceType": "git", "gitConnectionId": suspendedID,
	})
	posts := fake.posts()
	fake.setSuspended(true)
	suspended := sourceAuth(t, srv, suspendedRev, agentCred)
	if suspended.Code != http.StatusConflict || fake.posts() != posts {
		t.Fatalf("suspended status=%d posts=%d body=%s", suspended.Code, fake.posts(), suspended.Body.String())
	}
	assertConnectionStatus(t, pool, suspendedID, "disabled")
	fake.setSuspended(false)

	offlineID := insertGitHubAppConnection(t, pool, orgID, 4256)
	offlineRev := insertSourceRevision(t, pool, orgID, appID, envID, serverID, 12, map[string]any{
		"sourceType": "git", "gitConnectionId": offlineID,
	})
	offlineRes := sourceAuth(t, offline, offlineRev, agentCred)
	if offlineRes.Code != http.StatusBadGateway || strings.Contains(offlineRes.Body.String(), sourceAuthInstallToken) || strings.Contains(offlineRes.Body.String(), "127.0.0.1") {
		t.Fatalf("network status=%d body=%s", offlineRes.Code, offlineRes.Body.String())
	}
	assertConnectionStatus(t, pool, offlineID, "active")
	assertSourceAuthMaterialAbsent(t, pool, logs.String(), app.marker, fake.jwts())
}

func insertGitHubAppConnection(t *testing.T, pool *pgxpool.Pool, orgID string, installationID int64) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO git_connections (
			organization_id, provider, auth_mode, installation_id,
			account_id, account_type, repository_selection,
			account_login, display_name, status
		) VALUES ($1, 'github', 'github_app', $2, '77', 'Organization', 'selected', 'verified-octo', 'verified-octo', 'active')
		RETURNING id::text`, orgID, installationID).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertGitHubAppCredentialColumnsNull(t *testing.T, pool *pgxpool.Pool, connectionID string) {
	t.Helper()
	var cipher, nonce, keyID *string
	if err := pool.QueryRow(context.Background(), `
		SELECT credential_ciphertext::text, credential_nonce::text, credential_key_id
		FROM git_connections WHERE id = $1`, connectionID).Scan(&cipher, &nonce, &keyID); err != nil {
		t.Fatal(err)
	}
	if cipher != nil || nonce != nil || keyID != nil {
		t.Fatal("github app source-auth persisted credential material")
	}
}

func assertConnectionStatus(t *testing.T, pool *pgxpool.Pool, connectionID, want string) {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM git_connections WHERE id = $1`, connectionID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != want {
		t.Fatalf("connection status=%s want=%s", status, want)
	}
}

func assertSourceAuthMaterialAbsent(t *testing.T, pool *pgxpool.Pool, logs, keyMarker string, jwts []string) {
	t.Helper()
	var stored string
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(string_agg(COALESCE(effective_config::text, '') || COALESCE(variable_snapshot::text, '') || COALESCE(secret_refs::text, ''), ''), '')
		FROM revisions`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	var commands, events, audit, connections string
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(string_agg(payload::text || COALESCE(error_message, '') || COALESCE(result::text, ''), ''), '')
		FROM agent_commands`).Scan(&commands); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(string_agg(message || metadata::text, ''), '') FROM deployment_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(string_agg(COALESCE(before_metadata::text, '') || COALESCE(after_metadata::text, ''), ''), '')
		FROM audit_logs`).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(string_agg(
			COALESCE(credential_ciphertext::text, '') || COALESCE(credential_nonce::text, '') ||
			COALESCE(credential_key_id, '') || COALESCE(metadata::text, '') || COALESCE(account_login, ''),
		''), '') FROM git_connections`).Scan(&connections); err != nil {
		t.Fatal(err)
	}
	joined := stored + commands + events + audit + connections + logs
	for _, secret := range append([]string{sourceAuthInstallToken, sourceAuthWebhook, "PRIVATE KEY", keyMarker}, jwts...) {
		if secret != "" && strings.Contains(joined, secret) {
			t.Fatalf("persisted or logged credential material")
		}
	}
}

func restoreCredentialConstraint(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM git_connections WHERE auth_mode = 'github_app' AND installation_id IS NULL`)
		_, _ = pool.Exec(ctx, `ALTER TABLE git_connections DROP CONSTRAINT IF EXISTS git_connections_credential_mode_check`)
		_, err := pool.Exec(ctx, `
			ALTER TABLE git_connections
			ADD CONSTRAINT git_connections_credential_mode_check
			CHECK (
				(
					auth_mode = 'pat'
					AND credential_ciphertext IS NOT NULL
					AND credential_nonce IS NOT NULL
					AND credential_key_id IS NOT NULL
					AND length(credential_key_id) > 0
				)
				OR
				(
					auth_mode = 'github_app'
					AND installation_id IS NOT NULL
					AND credential_ciphertext IS NULL
					AND credential_nonce IS NULL
					AND credential_key_id IS NULL
				)
			)`)
		if err != nil {
			t.Errorf("restore credential constraint: %v", err)
		}
	})
}

type sourceAuthApp struct {
	loaded config.Config
	marker string
}

func loadSourceAuthApp(t *testing.T) sourceAuthApp {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	body := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	path := filepath.Join(t.TempDir(), "github-app.pem")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_URL", "postgres://localhost/deploycore?sslmode=disable")
	t.Setenv("AUTH_TOKEN_SECRET", "test-secret-0123456789abcdef01234567")
	t.Setenv("SECRETS_PLATFORM_KEY", "")
	t.Setenv("GITHUB_APP_ID", "5150")
	t.Setenv("GITHUB_APP_SLUG", "deploycore")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_FILE", path)
	t.Setenv("GITHUB_APP_WEBHOOK_SECRET", sourceAuthWebhook)
	t.Setenv("GITHUB_APP_SETUP_BASE_URL", "https://deploycore.example.com")
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return sourceAuthApp{loaded: loaded, marker: string(body[40:80])}
}

func sourceAuthGitHubServer(t *testing.T, pool *pgxpool.Pool, gitHubBase string, logs io.Writer, loaded config.Config) *server.Server {
	t.Helper()
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-revision-runtime"))
	return server.New(config.Config{
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
		GitHubApp:             loaded.GitHubApp,
		GitHubAPIBaseURL:      gitHubBase,
	}, slog.New(slog.NewTextHandler(logs, nil)), pool)
}

type sourceAuthGitHub struct {
	mu          sync.Mutex
	server      *httptest.Server
	installCode int
	tokenCode   int
	malformed   bool
	suspended   bool
	getCount    int
	postCount   int
	listCount   int
	seenJWT     []string
}

func newSourceAuthGitHub(t *testing.T) *sourceAuthGitHub {
	t.Helper()
	f := &sourceAuthGitHub{}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *sourceAuthGitHub) URL() string { return f.server.URL }

func (f *sourceAuthGitHub) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getCount + f.postCount + f.listCount
}

func (f *sourceAuthGitHub) gets() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getCount
}

func (f *sourceAuthGitHub) posts() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.postCount
}

func (f *sourceAuthGitHub) repoLists() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCount
}

func (f *sourceAuthGitHub) jwts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.seenJWT))
	copy(out, f.seenJWT)
	return out
}

func (f *sourceAuthGitHub) setInstallStatus(code int) {
	f.mu.Lock()
	f.installCode = code
	f.mu.Unlock()
}

func (f *sourceAuthGitHub) setTokenStatus(code int) {
	f.mu.Lock()
	f.tokenCode = code
	f.mu.Unlock()
}

func (f *sourceAuthGitHub) setMalformed(v bool) {
	f.mu.Lock()
	f.malformed = v
	f.mu.Unlock()
}

func (f *sourceAuthGitHub) setSuspended(v bool) {
	f.mu.Lock()
	f.suspended = v
	f.mu.Unlock()
}

func (f *sourceAuthGitHub) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if strings.Count(bearer, ".") == 2 {
		f.seenJWT = append(f.seenJWT, bearer)
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/app/installations/"):
		f.getCount++
		if f.installCode != 0 {
			w.WriteHeader(f.installCode)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/app/installations/")
		suspended := "null"
		if f.suspended {
			suspended = `"2026-10-04T12:00:00Z"`
		}
		_, _ = w.Write([]byte(`{"id":` + id + `,"app_id":5150,"repository_selection":"selected","suspended_at":` + suspended + `,"account":{"login":"verified-octo","id":77,"type":"Organization"}}`))
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/access_tokens"):
		f.postCount++
		if f.tokenCode != 0 {
			if f.tokenCode == http.StatusTooManyRequests {
				w.Header().Set("X-RateLimit-Remaining", "0")
			}
			w.WriteHeader(f.tokenCode)
			return
		}
		if f.malformed {
			_, _ = w.Write([]byte(`{"leak":"ghs_malformed_body"}`))
			return
		}
		_, _ = w.Write([]byte(`{"token":"` + sourceAuthInstallToken + `","expires_at":"2026-10-04T13:00:00Z"}`))
	default:
		f.listCount++
		w.WriteHeader(http.StatusNotFound)
	}
}
