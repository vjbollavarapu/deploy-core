package agentcmd_test

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

	"github.com/deploycore/deploy-core/apps/api/internal/agentcmd"
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
		url = "postgres://localhost/deploycore_b8_test?sslmode=disable"
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
		DELETE FROM agent_commands;
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

func TestAgentCommandLifecycle(t *testing.T) {
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
		AgentHeartbeatRetain:  50,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := server.New(cfg, log, pool)

	ownerTok := register(t, srv, "owner-b8@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B8 Org", "b8-org")
	serverID := createServer(t, srv, ownerTok, orgID)
	agentCred := registerAgent(t, srv, ownerTok, serverID)

	// Reject shell-like payloads.
	badBody, _ := json.Marshal(map[string]any{
		"operation": "FETCH_LOGS",
		"payload":   map[string]any{"shell": "rm -rf /"},
	})
	bad := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/commands", badBody, ownerTok)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("shell payload status=%d body=%s", bad.Code, bad.Body.String())
	}
	nestedBad, _ := json.Marshal(map[string]any{
		"operation": "FETCH_LOGS",
		"payload": map[string]any{
			"opts": map[string]any{"nested": map[string]any{"exec": "id"}},
		},
	})
	nested := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/commands", nestedBad, ownerTok)
	if nested.Code != http.StatusBadRequest {
		t.Fatalf("nested exec payload status=%d body=%s", nested.Code, nested.Body.String())
	}

	// Reject unknown operations.
	unknown, _ := json.Marshal(map[string]any{"operation": "EXEC_SHELL", "payload": map[string]any{}})
	unk := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/commands", unknown, ownerTok)
	if unk.Code != http.StatusBadRequest {
		t.Fatalf("unknown op status=%d body=%s", unk.Code, unk.Body.String())
	}

	issueBody, _ := json.Marshal(map[string]any{
		"operation":     agentcmd.OpPullImage,
		"correlationId": "corr-1",
		"payload": map[string]any{
			"image": "ghcr.io/deploycore/demo:1.0.0",
		},
	})
	issued := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/commands", issueBody, ownerTok)
	if issued.Code != http.StatusCreated {
		t.Fatalf("issue status=%d body=%s", issued.Code, issued.Body.String())
	}
	var created struct {
		Command struct {
			ID            string         `json:"id"`
			SchemaVersion int            `json:"schemaVersion"`
			Status        string         `json:"status"`
			Payload       map[string]any `json:"payload"`
		} `json:"command"`
	}
	decode(t, issued, &created)
	if created.Command.Status != "pending" || created.Command.SchemaVersion != agentcmd.SchemaVersion {
		t.Fatalf("created=%#v", created.Command)
	}
	cmdID := created.Command.ID

	// Agent polls pending commands.
	poll := doJSON(t, srv, http.MethodGet, "/api/v1/agents/commands", nil, agentCred)
	if poll.Code != http.StatusOK {
		t.Fatalf("poll status=%d body=%s", poll.Code, poll.Body.String())
	}
	var pollBody struct {
		SchemaVersion int `json:"schemaVersion"`
		Commands      []struct {
			ID string `json:"id"`
		} `json:"commands"`
	}
	decode(t, poll, &pollBody)
	if pollBody.SchemaVersion != 1 || len(pollBody.Commands) != 1 || pollBody.Commands[0].ID != cmdID {
		t.Fatalf("poll=%#v", pollBody)
	}

	// Status transitions.
	acceptBody, _ := json.Marshal(map[string]string{"status": "accepted"})
	acc := doJSON(t, srv, http.MethodPost, "/api/v1/agents/commands/"+cmdID+"/status", acceptBody, agentCred)
	if acc.Code != http.StatusOK {
		t.Fatalf("accept status=%d body=%s", acc.Code, acc.Body.String())
	}
	runBody, _ := json.Marshal(map[string]string{"status": "running"})
	run := doJSON(t, srv, http.MethodPost, "/api/v1/agents/commands/"+cmdID+"/status", runBody, agentCred)
	if run.Code != http.StatusOK {
		t.Fatalf("running status=%d body=%s", run.Code, run.Body.String())
	}
	doneBody, _ := json.Marshal(map[string]any{
		"status": "completed",
		"result": map[string]any{"digest": "sha256:abc"},
	})
	done := doJSON(t, srv, http.MethodPost, "/api/v1/agents/commands/"+cmdID+"/status", doneBody, agentCred)
	if done.Code != http.StatusOK {
		t.Fatalf("completed status=%d body=%s", done.Code, done.Body.String())
	}

	get := doJSON(t, srv, http.MethodGet, "/api/v1/commands/"+cmdID, nil, ownerTok)
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d", get.Code)
	}
	var got struct {
		Command struct {
			Status string         `json:"status"`
			Result map[string]any `json:"result"`
		} `json:"command"`
	}
	decode(t, get, &got)
	if got.Command.Status != "completed" {
		t.Fatalf("final status=%s", got.Command.Status)
	}

	// Cancel path for a fresh pending command.
	issue2, _ := json.Marshal(map[string]any{
		"operation": agentcmd.OpFetchLogs,
		"payload":   map[string]any{"containerId": "abc", "tail": 100},
	})
	created2 := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/commands", issue2, ownerTok)
	decode(t, created2, &created)
	cancel := doJSON(t, srv, http.MethodPost, "/api/v1/commands/"+created.Command.ID+"/cancel", nil, ownerTok)
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", cancel.Code, cancel.Body.String())
	}
}

func TestAllowedOperationsCatalog(t *testing.T) {
	t.Parallel()
	ops := agentcmd.AllowedOperations()
	if len(ops) < 14 {
		t.Fatalf("expected full operation catalog, got %d", len(ops))
	}
	if !agentcmd.IsAllowedOperation(agentcmd.OpDeployRevision) {
		t.Fatal("DEPLOY_REVISION missing")
	}
	if agentcmd.IsAllowedOperation("EXEC_SHELL") {
		t.Fatal("EXEC_SHELL must not be allowed")
	}
}

func register(t *testing.T, srv *server.Server, email, password, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password, "displayName": name})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/auth/register", body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", rec.Code, rec.Body.String())
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
	body, _ := json.Marshal(map[string]any{"organizationId": orgID, "name": "cmd-node", "hostname": "cmd-node"})
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

func registerAgent(t *testing.T, srv *server.Server, ownerTok, serverID string) string {
	t.Helper()
	tokRec := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/registration-token", nil, ownerTok)
	if tokRec.Code != http.StatusCreated {
		t.Fatalf("reg token status=%d body=%s", tokRec.Code, tokRec.Body.String())
	}
	var tok struct {
		RegistrationToken struct {
			Token string `json:"token"`
		} `json:"registrationToken"`
	}
	decode(t, tokRec, &tok)
	body, _ := json.Marshal(map[string]string{"registrationToken": tok.RegistrationToken.Token, "agentVersion": "1.0.0"})
	reg := doJSON(t, srv, http.MethodPost, "/api/v1/agents/register", body, "")
	if reg.Code != http.StatusCreated {
		t.Fatalf("agent register status=%d body=%s", reg.Code, reg.Body.String())
	}
	var out struct {
		Agent struct {
			Credential string `json:"credential"`
		} `json:"agent"`
	}
	decode(t, reg, &out)
	return out.Agent.Credential
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
