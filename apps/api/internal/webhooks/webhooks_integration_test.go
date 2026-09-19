package webhooks_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/security"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/deploycore/deploy-core/apps/api/internal/webhooks"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b26_test?sslmode=disable"
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
		DELETE FROM outgoing_webhook_deliveries;
		DELETE FROM outgoing_webhooks;
		DELETE FROM jobs;
		DELETE FROM environments;
		DELETE FROM projects;
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
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b26-hooks"))
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

func TestOutgoingWebhookHMACDelivery(t *testing.T) {
	pool := testPool(t)
	security.AllowPrivateOutboundHosts.Store(true)
	t.Cleanup(func() { security.AllowPrivateOutboundHosts.Store(false) })

	var gotSig atomic.Value
	var gotEvent atomic.Value
	var gotBody atomic.Value
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotSig.Store(r.Header.Get("X-DeployCore-Signature"))
		gotEvent.Store(r.Header.Get("X-DeployCore-Event"))
		gotBody.Store(raw)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)

	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b26-hooks"))
	repo := webhooks.NewPostgresRepository(pool)
	queue := jobs.NewQueue(jobs.NewPostgresRepository(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc := webhooks.NewService(repo, queue, rbac.NewAuthorizer(pool), nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		webhooks.ServiceConfig{PlatformKey: key, KeyID: "platform:v1", HTTPClient: ts.Client()})

	srv := testServer(t, pool)
	ownerTok := register(t, srv, "owner-b26-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B26 Org", "b26-org-"+uuid.NewString()[:8])

	create := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/webhooks",
		mustJSON(map[string]any{
			"organizationId": orgID,
			"name":           "CI hook",
			"url":            ts.URL,
			"events":         []string{webhooks.EventBackupFailed},
			"secret":         "hook-secret-value",
		}), ownerTok)
	if create.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", create.Code, create.Body.String())
	}
	var created struct {
		Webhook struct {
			ID     string  `json:"id"`
			Secret *string `json:"secret"`
		} `json:"webhook"`
	}
	decode(t, create, &created)
	if created.Webhook.Secret == nil || *created.Webhook.Secret != "hook-secret-value" {
		t.Fatalf("secret once=%v", created.Webhook.Secret)
	}

	emit := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/webhooks/emit",
		mustJSON(map[string]any{
			"organizationId": orgID,
			"eventType":      webhooks.EventBackupFailed,
			"payload":        map[string]any{"backupId": uuid.NewString(), "password": "nope"},
		}), ownerTok)
	if emit.Code != http.StatusAccepted {
		t.Fatalf("emit=%d %s", emit.Code, emit.Body.String())
	}

	list := doJSON(t, srv, http.MethodGet, "/api/v1/integrations/webhooks/deliveries?organizationId="+orgID, nil, ownerTok)
	var delOut struct {
		Items []struct {
			ID     string         `json:"id"`
			Status string         `json:"status"`
			JobID  *string        `json:"jobId"`
			Payload map[string]any `json:"payload"`
		} `json:"items"`
	}
	decode(t, list, &delOut)
	if len(delOut.Items) < 1 || delOut.Items[0].JobID == nil {
		t.Fatalf("deliveries=%#v", delOut)
	}
	if _, ok := delOut.Items[0].Payload["password"]; ok {
		t.Fatal("password leaked")
	}

	job, err := queue.Claim(context.Background(), "b26-worker", 30*time.Second)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := svc.ProcessDeliveryJob(context.Background(), job); err != nil {
		t.Fatalf("process: %v", err)
	}
	_, _ = queue.Complete(context.Background(), job.ID, "b26-worker")

	sig, _ := gotSig.Load().(string)
	event, _ := gotEvent.Load().(string)
	body, _ := gotBody.Load().([]byte)
	if event != webhooks.EventBackupFailed {
		t.Fatalf("event header=%s", event)
	}
	mac := hmac.New(sha256.New, []byte("hook-secret-value"))
	_, _ = mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if sig != want {
		t.Fatalf("sig=%s want=%s body=%s", sig, want, string(body))
	}

	getList := doJSON(t, srv, http.MethodGet, "/api/v1/integrations/webhooks/"+created.Webhook.ID+"/deliveries", nil, ownerTok)
	decode(t, getList, &delOut)
	if delOut.Items[0].Status != webhooks.DeliveryDelivered {
		t.Fatalf("status=%s", delOut.Items[0].Status)
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
