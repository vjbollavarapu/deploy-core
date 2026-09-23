package notifications_test

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
	"github.com/deploycore/deploy-core/apps/api/internal/jobs"
	"github.com/deploycore/deploy-core/apps/api/internal/notifications"
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
		url = "postgres://localhost/deploycore_b25_test?sslmode=disable"
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
	_, err = pool.Exec(ctx, `
		DELETE FROM notification_deliveries;
		DELETE FROM notification_policies;
		DELETE FROM notification_channels;
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
	if err != nil {
		// Shared TEST_DATABASE_URL: prior packages may leave FK rows. Force-clear jobs at minimum.
		if _, jerr := pool.Exec(ctx, `DELETE FROM jobs`); jerr != nil {
			t.Fatalf("cleanup jobs: %v (batch: %v)", jerr, err)
		}
	}
	// Always ensure no leased/queued jobs remain for Claim isolation.
	if _, err := pool.Exec(ctx, `DELETE FROM jobs`); err != nil {
		t.Fatalf("cleanup jobs: %v", err)
	}
	return pool
}

func testServer(t *testing.T, pool *pgxpool.Pool) *server.Server {
	t.Helper()
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b25-notify"))
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

func TestNotificationChannelPolicyEmitAndDeliver(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b25-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B25 Org", "b25-org-"+uuid.NewString()[:8])

	bad := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/notifications/channels",
		mustJSON(map[string]any{
			"organizationId": orgID, "name": "Slack", "type": "SLACK",
			"config": map[string]any{"webhookUrl": "https://example.com"},
		}), ownerTok)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("reserved channel status=%d body=%s", bad.Code, bad.Body.String())
	}

	chRec := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/notifications/channels",
		mustJSON(map[string]any{
			"organizationId": orgID, "name": "Ops Email", "type": "EMAIL",
			"config": map[string]any{"to": []string{"ops@example.com"}},
		}), ownerTok)
	if chRec.Code != http.StatusCreated {
		t.Fatalf("create channel=%d %s", chRec.Code, chRec.Body.String())
	}
	var chOut struct {
		Channel struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"channel"`
	}
	decode(t, chRec, &chOut)
	if chOut.Channel.Type != notifications.ChannelEmail {
		t.Fatalf("type=%s", chOut.Channel.Type)
	}

	polRec := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/notifications/policies",
		mustJSON(map[string]any{
			"organizationId": orgID,
			"name":           "Backup failures",
			"eventTypes":     []string{notifications.EventBackupFailed},
			"channelIds":     []string{chOut.Channel.ID},
		}), ownerTok)
	if polRec.Code != http.StatusCreated {
		t.Fatalf("create policy=%d %s", polRec.Code, polRec.Body.String())
	}

	emit := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/notifications/emit",
		mustJSON(map[string]any{
			"organizationId": orgID,
			"eventType":      notifications.EventBackupFailed,
			"payload":        map[string]any{"backupId": uuid.NewString(), "password": "nope"},
		}), ownerTok)
	if emit.Code != http.StatusAccepted {
		t.Fatalf("emit=%d %s", emit.Code, emit.Body.String())
	}
	var emitOut struct {
		Enqueued int `json:"enqueued"`
	}
	decode(t, emit, &emitOut)
	if emitOut.Enqueued != 1 {
		t.Fatalf("enqueued=%d", emitOut.Enqueued)
	}

	list := doJSON(t, srv, http.MethodGet, "/api/v1/integrations/notifications/deliveries?organizationId="+orgID, nil, ownerTok)
	if list.Code != http.StatusOK {
		t.Fatalf("list deliveries=%d %s", list.Code, list.Body.String())
	}
	var delOut struct {
		Items []struct {
			ID        string         `json:"id"`
			Status    string         `json:"status"`
			EventType string         `json:"eventType"`
			Payload   map[string]any `json:"payload"`
			JobID     *string        `json:"jobId"`
		} `json:"items"`
		TotalCount *int64 `json:"totalCount"`
	}
	decode(t, list, &delOut)
	if delOut.TotalCount == nil || *delOut.TotalCount < 1 || len(delOut.Items) < 1 {
		t.Fatalf("deliveries=%#v", delOut)
	}
	d := delOut.Items[0]
	if d.Status != notifications.DeliveryQueued || d.JobID == nil {
		t.Fatalf("delivery=%#v", d)
	}
	if _, ok := d.Payload["password"]; ok {
		t.Fatal("password leaked into delivery payload")
	}

	var jobType string
	err := pool.QueryRow(context.Background(), `SELECT type FROM jobs WHERE id = $1`, *d.JobID).Scan(&jobType)
	if err != nil {
		t.Fatal(err)
	}
	if jobType != jobs.TypeNotificationDelivery {
		t.Fatalf("job type=%s", jobType)
	}

	repo := notifications.NewPostgresRepository(pool)
	queue := jobs.NewQueue(jobs.NewPostgresRepository(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b25-notify"))
	svc := notifications.NewService(repo, queue, rbac.NewAuthorizer(pool), nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		notifications.ServiceConfig{PlatformKey: key, KeyID: "platform:v1"})
	job, err := queue.Claim(context.Background(), "test-worker", 30*time.Second)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if job.ID == uuid.Nil {
		t.Fatal("expected NOTIFICATION_DELIVERY job")
	}
	wantID, parseErr := uuid.Parse(*d.JobID)
	if parseErr != nil {
		t.Fatalf("delivery jobId: %v", parseErr)
	}
	// Shared test DB may contain leftover DEPLOYMENT_EXECUTION rows; drain until ours.
	for attempt := 0; job.ID != wantID && attempt < 32; attempt++ {
		if job.Type != jobs.TypeNotificationDelivery {
			_, _ = queue.Complete(context.Background(), job.ID, "test-worker")
		}
		job, err = queue.Claim(context.Background(), "test-worker", 30*time.Second)
		if err != nil {
			t.Fatalf("claim retry: %v", err)
		}
	}
	if job.ID != wantID {
		t.Fatalf("claimed unexpected job type=%s id=%s (wanted NOTIFICATION_DELIVERY %s)", job.Type, job.ID, *d.JobID)
	}
	if err := svc.ProcessDeliveryJob(context.Background(), job); err != nil {
		t.Fatalf("process: %v", err)
	}
	_, _ = queue.Complete(context.Background(), job.ID, "test-worker")

	getList := doJSON(t, srv, http.MethodGet, "/api/v1/integrations/notifications/deliveries?organizationId="+orgID, nil, ownerTok)
	decode(t, getList, &delOut)
	if delOut.Items[0].Status != notifications.DeliveryDelivered {
		t.Fatalf("after process status=%s", delOut.Items[0].Status)
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
