package agents_test

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
		url = "postgres://localhost/deploycore_b7_test?sslmode=disable"
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
		DELETE FROM server_heartbeats;
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

func TestAgentRegistrationAndHeartbeat(t *testing.T) {
	pool := testPool(t)
	cfg := config.Config{
		Env:                   "test",
		AuthTokenSecret:       "test-secret-0123456789abcdef01234567",
		AccessTokenTTL:        time.Minute,
		RefreshTokenTTL:       time.Hour,
		AuthRateLimitPerMin:   1000,
		CORSAllowedOrigins:    []string{"*"},
		AuthMinPasswordLength: 8,
		AgentRegistrationTTL:  15 * time.Minute,
		AgentHeartbeatRetain:  5,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := server.New(cfg, log, pool)

	ownerTok := register(t, srv, "owner-b7@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B7 Org", "b7-org")
	serverID := createServer(t, srv, ownerTok, orgID)

	// Issue registration token.
	tokRec := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/registration-token", nil, ownerTok)
	if tokRec.Code != http.StatusCreated {
		t.Fatalf("issue token status=%d body=%s", tokRec.Code, tokRec.Body.String())
	}
	var tokBody struct {
		RegistrationToken struct {
			Token     string `json:"token"`
			ExpiresAt string `json:"expiresAt"`
		} `json:"registrationToken"`
	}
	decode(t, tokRec, &tokBody)
	if tokBody.RegistrationToken.Token == "" {
		t.Fatal("expected token")
	}
	regToken := tokBody.RegistrationToken.Token

	// Register agent.
	regBody, _ := json.Marshal(map[string]string{
		"registrationToken": regToken,
		"agentVersion":      "1.0.0",
	})
	regRec := doJSON(t, srv, http.MethodPost, "/api/v1/agents/register", regBody, "")
	if regRec.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", regRec.Code, regRec.Body.String())
	}
	var reg struct {
		Agent struct {
			ID         string `json:"id"`
			ServerID   string `json:"serverId"`
			Credential string `json:"credential"`
		} `json:"agent"`
	}
	decode(t, regRec, &reg)
	if reg.Agent.Credential == "" || reg.Agent.ServerID != serverID {
		t.Fatalf("register result=%#v", reg.Agent)
	}

	// Replay protection.
	replay := doJSON(t, srv, http.MethodPost, "/api/v1/agents/register", regBody, "")
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("replay status=%d body=%s", replay.Code, replay.Body.String())
	}

	// Heartbeat updates server to ONLINE.
	hbBody, _ := json.Marshal(map[string]any{
		"agentVersion":    "1.0.1",
		"dockerStatus":    "ok",
		"cpuPercent":      12.5,
		"memoryUsedBytes": 1024,
		"diskUsedBytes":   2048,
		"load1":           0.2,
		"containerCount":  3,
		"uptimeSeconds":   99,
		"dockerVersion":   "24.0.0",
	})
	hb := doAuthed(t, srv, http.MethodPost, "/api/v1/agents/heartbeat", hbBody, reg.Agent.Credential)
	if hb.Code != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hb.Code, hb.Body.String())
	}

	get := doJSON(t, srv, http.MethodGet, "/api/v1/servers/"+serverID, nil, ownerTok)
	if get.Code != http.StatusOK {
		t.Fatalf("get server status=%d", get.Code)
	}
	var serverBody struct {
		Server struct {
			Status          string  `json:"status"`
			LastHeartbeatAt *string `json:"lastHeartbeatAt"`
			DockerVersion   *string `json:"dockerVersion"`
		} `json:"server"`
	}
	decode(t, get, &serverBody)
	if serverBody.Server.Status != "ONLINE" {
		t.Fatalf("status=%s", serverBody.Server.Status)
	}
	if serverBody.Server.LastHeartbeatAt == nil {
		t.Fatal("expected lastHeartbeatAt")
	}
	if serverBody.Server.DockerVersion == nil || *serverBody.Server.DockerVersion != "24.0.0" {
		t.Fatalf("dockerVersion=%v", serverBody.Server.DockerVersion)
	}

	var hbCount int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM server_heartbeats WHERE server_id = $1`, serverID).Scan(&hbCount); err != nil {
		t.Fatalf("hb count: %v", err)
	}
	if hbCount != 1 {
		t.Fatalf("heartbeat rows=%d", hbCount)
	}

	// Degraded on bad docker status.
	hbDegraded, _ := json.Marshal(map[string]any{"dockerStatus": "down", "agentVersion": "1.0.1"})
	d := doAuthed(t, srv, http.MethodPost, "/api/v1/agents/heartbeat", hbDegraded, reg.Agent.Credential)
	if d.Code != http.StatusOK {
		t.Fatalf("degraded hb status=%d", d.Code)
	}
	get2 := doJSON(t, srv, http.MethodGet, "/api/v1/servers/"+serverID, nil, ownerTok)
	decode(t, get2, &serverBody)
	if serverBody.Server.Status != "DEGRADED" {
		t.Fatalf("expected DEGRADED got %s", serverBody.Server.Status)
	}

	// Maintenance should not be overwritten by heartbeat.
	maint := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/maintenance", nil, ownerTok)
	if maint.Code != http.StatusOK {
		t.Fatalf("maintenance status=%d", maint.Code)
	}
	_ = doAuthed(t, srv, http.MethodPost, "/api/v1/agents/heartbeat", hbBody, reg.Agent.Credential)
	get3 := doJSON(t, srv, http.MethodGet, "/api/v1/servers/"+serverID, nil, ownerTok)
	decode(t, get3, &serverBody)
	if serverBody.Server.Status != "MAINTENANCE" {
		t.Fatalf("expected MAINTENANCE got %s", serverBody.Server.Status)
	}

	// Revoke open token: issue new then revoke before use.
	tok2 := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/registration-token", nil, ownerTok)
	decode(t, tok2, &tokBody)
	rev := doJSON(t, srv, http.MethodDelete, "/api/v1/servers/"+serverID+"/registration-token", nil, ownerTok)
	if rev.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", rev.Code, rev.Body.String())
	}
	regBody2, _ := json.Marshal(map[string]string{"registrationToken": tokBody.RegistrationToken.Token, "agentVersion": "2.0.0"})
	denied := doJSON(t, srv, http.MethodPost, "/api/v1/agents/register", regBody2, "")
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status=%d body=%s", denied.Code, denied.Body.String())
	}

	// Retention prune: insert more heartbeats than retain count.
	for i := 0; i < 8; i++ {
		body, _ := json.Marshal(map[string]any{"dockerStatus": "ok", "agentVersion": "1.0.1", "cpuPercent": float64(i)})
		rec := doAuthed(t, srv, http.MethodPost, "/api/v1/agents/heartbeat", body, reg.Agent.Credential)
		if rec.Code != http.StatusOK {
			t.Fatalf("hb loop %d status=%d", i, rec.Code)
		}
	}
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM server_heartbeats WHERE server_id = $1`, serverID).Scan(&hbCount); err != nil {
		t.Fatalf("hb count2: %v", err)
	}
	if hbCount > 5 {
		t.Fatalf("expected prune to <=5, got %d", hbCount)
	}

	// Revoke active agent credential — heartbeats must fail afterward.
	credRev := doJSON(t, srv, http.MethodDelete, "/api/v1/servers/"+serverID+"/agent-credential", nil, ownerTok)
	if credRev.Code != http.StatusOK {
		t.Fatalf("credential revoke status=%d body=%s", credRev.Code, credRev.Body.String())
	}
	deniedHB := doAuthed(t, srv, http.MethodPost, "/api/v1/agents/heartbeat", hbBody, reg.Agent.Credential)
	if deniedHB.Code != http.StatusUnauthorized {
		t.Fatalf("heartbeat after credential revoke status=%d body=%s", deniedHB.Code, deniedHB.Body.String())
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

func createServer(t *testing.T, srv *server.Server, token, orgID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"organizationId": orgID, "name": "node-1", "provider": "test", "hostname": "node-1",
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

func doAuthed(t *testing.T, srv *server.Server, method, path string, body []byte, credential string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSON(t, srv, method, path, body, credential)
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
}
