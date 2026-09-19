package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/jackc/pgx/v5/pgxpool"
)

type captureNotifier struct {
	mu     sync.Mutex
	email  string
	token  string
	called int
}

func (c *captureNotifier) NotifyPasswordReset(_ context.Context, email, rawToken string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.called++
	c.email = email
	c.token = rawToken
	return nil
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_auth_test?sslmode=disable"
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
	// Isolate rows between tests.
	_, _ = pool.Exec(ctx, `
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

func TestAuthRegisterLoginMeRefreshLogout(t *testing.T) {
	pool := testPool(t)
	cfg := config.Config{
		Env:                   "test",
		AuthTokenSecret:       "test-secret-0123456789abcdef01234567",
		AccessTokenTTL:        time.Minute,
		RefreshTokenTTL:       time.Hour,
		PasswordResetTTL:      time.Hour,
		AuthRateLimitPerMin:   1000,
		CORSAllowedOrigins:    []string{"*"},
		AuthMinPasswordLength: 8,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := server.New(cfg, log, pool)

	email := "alice@example.com"
	password := "password123"

	regBody, _ := json.Marshal(map[string]string{
		"email": email, "password": password, "displayName": "Alice",
	})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/auth/register", regBody, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", rec.Code, rec.Body.String())
	}
	var reg apiAuthResponse
	decodeBody(t, rec, &reg)
	if reg.Tokens.AccessToken == "" || reg.Tokens.RefreshToken == "" {
		t.Fatalf("missing tokens: %#v", reg.Tokens)
	}
	if reg.User.Email != email {
		t.Fatalf("email = %s", reg.User.Email)
	}

	me := doJSON(t, srv, http.MethodGet, "/api/v1/auth/me", nil, reg.Tokens.AccessToken)
	if me.Code != http.StatusOK {
		t.Fatalf("me status=%d body=%s", me.Code, me.Body.String())
	}

	loginBody, _ := json.Marshal(map[string]string{"email": email, "password": password})
	loginRec := doJSON(t, srv, http.MethodPost, "/api/v1/auth/login", loginBody, "")
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var login apiAuthResponse
	decodeBody(t, loginRec, &login)

	oldRefresh := login.Tokens.RefreshToken
	refreshBody, _ := json.Marshal(map[string]string{"refreshToken": oldRefresh})
	refRec := doJSON(t, srv, http.MethodPost, "/api/v1/auth/refresh", refreshBody, "")
	if refRec.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", refRec.Code, refRec.Body.String())
	}
	var refreshed apiAuthResponse
	decodeBody(t, refRec, &refreshed)
	if refreshed.Tokens.RefreshToken == oldRefresh {
		t.Fatal("expected refresh token rotation")
	}

	// Old refresh must be rejected after rotation (reuse kills the family).
	reuse := doJSON(t, srv, http.MethodPost, "/api/v1/auth/refresh", refreshBody, "")
	if reuse.Code != http.StatusUnauthorized {
		t.Fatalf("reuse status=%d", reuse.Code)
	}
	// Family revoke: the previously valid rotated token must also die after reuse detection.
	newBody, _ := json.Marshal(map[string]string{"refreshToken": refreshed.Tokens.RefreshToken})
	familyDead := doJSON(t, srv, http.MethodPost, "/api/v1/auth/refresh", newBody, "")
	if familyDead.Code != http.StatusUnauthorized {
		t.Fatalf("family after reuse status=%d body=%s", familyDead.Code, familyDead.Body.String())
	}

	// Re-login for logout coverage.
	loginAgainBody, _ := json.Marshal(map[string]string{"email": "alice@example.com", "password": "password123"})
	loginAgainRec := doJSON(t, srv, http.MethodPost, "/api/v1/auth/login", loginAgainBody, "")
	if loginAgainRec.Code != http.StatusOK {
		t.Fatalf("re-login status=%d body=%s", loginAgainRec.Code, loginAgainRec.Body.String())
	}
	var loginAgain apiAuthResponse
	decodeBody(t, loginAgainRec, &loginAgain)

	logoutBody, _ := json.Marshal(map[string]string{"refreshToken": loginAgain.Tokens.RefreshToken})
	out := doJSON(t, srv, http.MethodPost, "/api/v1/auth/logout", logoutBody, loginAgain.Tokens.AccessToken)
	if out.Code != http.StatusOK {
		t.Fatalf("logout status=%d", out.Code)
	}
	me2 := doJSON(t, srv, http.MethodGet, "/api/v1/auth/me", nil, loginAgain.Tokens.AccessToken)
	if me2.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout status=%d", me2.Code)
	}
}

type apiAuthResponse struct {
	User struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	} `json:"user"`
	Tokens auth.TokenPair `json:"tokens"`
}

func TestAuthPasswordResetFlow(t *testing.T) {
	pool := testPool(t)
	notifier := &captureNotifier{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := auth.NewPostgresRepository(pool)
	svc := auth.NewService(repo, auth.ServiceConfig{
		AccessTokenSecret: []byte("test-secret-0123456789abcdef01234567"),
		AccessTokenTTL:    time.Minute,
		RefreshTokenTTL:   time.Hour,
		PasswordResetTTL:  time.Hour,
		MinPasswordLength: 8,
	}, log, notifier)
	handler := auth.NewHandler(svc, auth.NewRateLimiter(1000, time.Minute))
	mux := http.NewServeMux()
	handler.Mount(mux)

	_, err := svc.Register(context.Background(), auth.RegisterInput{
		Email: "bob@example.com", Password: "oldpassword", DisplayName: "Bob",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	forgotBody, _ := json.Marshal(map[string]string{"email": "bob@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewReader(forgotBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("forgot status=%d body=%s", rec.Code, rec.Body.String())
	}
	if notifier.token == "" {
		t.Fatal("expected reset token notification")
	}

	resetBody, _ := json.Marshal(map[string]string{
		"token": notifier.token, "newPassword": "newpassword1",
	})
	req = httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewReader(resetBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status=%d body=%s", rec.Code, rec.Body.String())
	}

	_, err = svc.Login(context.Background(), auth.LoginInput{
		Email: "bob@example.com", Password: "newpassword1",
	})
	if err != nil {
		t.Fatalf("login after reset: %v", err)
	}
}

func TestAuthDisabledAccount(t *testing.T) {
	pool := testPool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := auth.NewPostgresRepository(pool)
	svc := auth.NewService(repo, auth.ServiceConfig{
		AccessTokenSecret: []byte("test-secret-0123456789abcdef01234567"),
		AccessTokenTTL:    time.Minute,
		RefreshTokenTTL:   time.Hour,
		MinPasswordLength: 8,
	}, log, auth.NopPasswordResetNotifier{})

	res, err := svc.Register(context.Background(), auth.RegisterInput{
		Email: "disabled@example.com", Password: "password123", DisplayName: "D",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err = pool.Exec(context.Background(), `UPDATE users SET status = 'disabled' WHERE id = $1`, res.User.ID)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	_, err = svc.Login(context.Background(), auth.LoginInput{
		Email: "disabled@example.com", Password: "password123",
	})
	if err == nil {
		t.Fatal("expected disabled login failure")
	}
}

func TestProtectedRouteRequiresBearer(t *testing.T) {
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

	me := doJSON(t, srv, http.MethodGet, "/api/v1/auth/me", nil, "")
	if me.Code != http.StatusUnauthorized {
		t.Fatalf("no token status=%d", me.Code)
	}
	bad := doJSON(t, srv, http.MethodGet, "/api/v1/auth/me", nil, "not-a-jwt")
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad token status=%d", bad.Code)
	}

	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeBody(t, me, &env)
	if env.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("code=%s", env.Error.Code)
	}
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

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
}
